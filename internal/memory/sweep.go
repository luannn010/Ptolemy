package memory

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
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
