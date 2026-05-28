package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// Sweeper runs the GC's off-hot-path maintenance. Phase 4 passes: archiveDecayed
// (project rows below the decay threshold) and the gated purgeDead. Dedup and
// supersession resolution are Phase 5.
type Sweeper struct {
	conn *pgx.Conn
	cfg  GCConfig
}

func NewSweeper(conn *pgx.Conn, cfg GCConfig) *Sweeper {
	return &Sweeper{conn: conn, cfg: cfg}
}

// sweepOnce runs the Phase 4 passes in order. Directly callable in tests.
func (s *Sweeper) sweepOnce(ctx context.Context) error {
	if err := s.archiveDecayed(ctx); err != nil {
		return fmt.Errorf("archive pass: %w", err)
	}
	if err := s.purgeDead(ctx); err != nil {
		return fmt.Errorf("purge pass: %w", err)
	}
	return nil
}

// archiveDecayed archives project rows whose decay score is below the threshold.
// The scope='project' clause IS the firewall — global rows are structurally
// unreachable. Each transition is audited.
func (s *Sweeper) archiveDecayed(ctx context.Context) error {
	_, err := s.conn.Exec(ctx, `
		WITH moved AS (
		  UPDATE chunks SET status='archived', archived_at=now()
		   WHERE scope='project' AND status='active' AND NOT pinned
		     AND importance * exp(-$1::float8 * extract(epoch FROM now()-last_accessed_at)/86400
		                          / (1 + access_count)) < $2::float8
		  RETURNING id
		)
		INSERT INTO chunk_audit(chunk_id, old_status, new_status, reason)
		SELECT id, 'active', 'archived', 'decay' FROM moved
	`, s.cfg.DecayLambda, s.cfg.ArchiveThreshold)
	return err
}

// purgeDead deletes rows that have been 'dead' for at least PurgeGraceDays.
// The ONLY destructive op; a no-op unless PurgeEnabled. Audited before delete,
// inside a transaction so audit + delete are atomic.
func (s *Sweeper) purgeDead(ctx context.Context) error {
	if !s.cfg.PurgeEnabled {
		return nil
	}
	tx, err := s.conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin purge tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO chunk_audit(chunk_id, old_status, new_status, reason)
		SELECT id, 'dead', 'purged', 'purge' FROM chunks
		 WHERE status='dead' AND dead_at <= now() - make_interval(days => $1::int)
	`, int(s.cfg.PurgeGraceDays)); err != nil {
		return fmt.Errorf("purge audit: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM chunks
		 WHERE status='dead' AND dead_at <= now() - make_interval(days => $1::int)
	`, int(s.cfg.PurgeGraceDays)); err != nil {
		return fmt.Errorf("purge delete: %w", err)
	}
	return tx.Commit(ctx)
}

// dedupRecent collapses normalized-content-identical near-duplicates among active rows
// created within DedupLookback, within scope. Trigram similarity (>= DedupThreshold) only
// PREFILTERS candidate pairs; the collapse decision is normalized-content EQUALITY, so a
// contradiction (different content) is never collapsed. No-op unless DedupEnabled.
func (s *Sweeper) dedupRecent(ctx context.Context) error {
	if !s.cfg.DedupEnabled {
		return nil
	}
	// Recent active rows, newest first so we keep the established (older) row as survivor.
	rows, err := s.conn.Query(ctx, `
		SELECT id, content, scope, access_count, created_at
		FROM chunks
		WHERE status='active' AND created_at >= now() - make_interval(secs => $1::float8)
		ORDER BY created_at DESC
	`, s.cfg.DedupLookback.Seconds())
	if err != nil {
		return fmt.Errorf("dedup recent scan: %w", err)
	}
	type row struct {
		id, content, scope string
		accessCount        int
		createdAt          time.Time
	}
	var recent []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.content, &r.scope, &r.accessCount, &r.createdAt); err != nil {
			rows.Close()
			return err
		}
		recent = append(recent, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	dead := map[string]bool{}
	for _, r := range recent {
		if dead[r.id] {
			continue
		}
		// Trigram candidates in the same scope (explicit similarity() comparison; independent of pg_trgm.similarity_threshold).
		crows, err := s.conn.Query(ctx, `
			SELECT id, content, access_count, created_at
			FROM chunks
			WHERE status='active' AND scope=$1 AND id <> $2
			  AND similarity(content, $3) >= $4::float4
		`, r.scope, r.id, r.content, s.cfg.DedupThreshold)
		if err != nil {
			return fmt.Errorf("dedup candidates: %w", err)
		}
		type cand struct {
			id, content string
			accessCount int
			createdAt   time.Time
		}
		var cands []cand
		for crows.Next() {
			var c cand
			if err := crows.Scan(&c.id, &c.content, &c.accessCount, &c.createdAt); err != nil {
				crows.Close()
				return err
			}
			cands = append(cands, c)
		}
		crows.Close()
		if err := crows.Err(); err != nil {
			return err
		}

		for _, c := range cands {
			if dead[c.id] {
				continue
			}
			if normalizeContent(r.content) != normalizeContent(c.content) {
				continue // similar but not identical → keep both (safe fallback / contradiction)
			}
			// Survivor: higher access_count; tie-break older created_at.
			survID := r.id
			loseID := c.id
			if c.accessCount > r.accessCount || (c.accessCount == r.accessCount && c.createdAt.Before(r.createdAt)) {
				survID = c.id
				loseID = r.id
			}
			if err := s.collapseDuplicate(ctx, survID, loseID); err != nil {
				return err
			}
			dead[loseID] = true
			if loseID == r.id {
				break // r itself is gone; stop pairing it
			}
		}
	}
	return nil
}

// collapseDuplicate reinforces the survivor and marks the loser dead 'duplicate', audited,
// in one transaction.
func (s *Sweeper) collapseDuplicate(ctx context.Context, survivorID, loserID string) error {
	tx, err := s.conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin dedup tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Kill the loser first. If it's no longer active (a concurrent supersede/archive
	// raced us between the candidate scan and now), skip the survivor reinforce + audit
	// so we never record a transition that didn't happen.
	ct, err := tx.Exec(ctx,
		`UPDATE chunks SET status='dead', dead_at=now() WHERE id=$1 AND status='active'`, loserID)
	if err != nil {
		return fmt.Errorf("kill loser: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return nil // loser already gone; nothing to collapse (deferred Rollback is a no-op)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE chunks SET access_count=access_count+1, last_accessed_at=now() WHERE id=$1`, survivorID); err != nil {
		return fmt.Errorf("reinforce survivor: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO chunk_audit(chunk_id, old_status, new_status, reason) VALUES ($1,'active','dead','duplicate')`, loserID); err != nil {
		return fmt.Errorf("dedup audit: %w", err)
	}
	return tx.Commit(ctx)
}

// Run ticks every cfg.SweepInterval and runs a sweep. Per-tick-tolerant: a
// failed sweep is logged at Error and the loop continues to the next interval.
// Returns when ctx is cancelled.
func (s *Sweeper) Run(ctx context.Context) {
	log.Info().Dur("interval", s.cfg.SweepInterval).Msg("memory sweep loop started")
	runLoop(ctx, s.cfg.SweepInterval, s.sweepOnce)
	log.Info().Msg("memory sweep loop stopped")
}

// runLoop is the testable core: tick on the interval, tolerate per-tick errors,
// stop on ctx cancellation. Extracted so the loop can be unit-tested without a DB.
func runLoop(ctx context.Context, interval time.Duration, tick func(context.Context) error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := tick(ctx); err != nil {
				log.Error().Err(err).Msg("memory sweep tick failed; retrying next interval")
			}
		}
	}
}
