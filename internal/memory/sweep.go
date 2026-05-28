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
