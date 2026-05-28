package memory

import (
	"context"
	"testing"
	"time"
)

func gcTestConfig() GCConfig {
	return GCConfig{
		SweepEnabled:     true,
		SweepInterval:    time.Hour,
		DecayLambda:      0.05,
		ArchiveThreshold: 0.1,
		PurgeEnabled:     false,
		PurgeGraceDays:   30,
	}
}

func TestSweeper_ArchivesDecayedProjectRow_LeavesGlobalUntouched(t *testing.T) {
	conn := freshDB(t)
	s := NewPgStore(conn)
	old := time.Now().UTC().Add(-365 * 24 * time.Hour) // very old, unaccessed
	if err := s.Upsert(context.Background(), []Chunk{
		{ID: "proj", Content: "p", Embedding: []float32{1, 0, 0, 0}, PublishedAt: old},
		{ID: "glob", Content: "g", Embedding: []float32{1, 0, 0, 0}, PublishedAt: old},
	}); err != nil {
		t.Fatal(err)
	}
	// proj → project, both last_accessed long ago, access_count 0.
	if _, err := conn.Exec(context.Background(), `
		UPDATE chunks SET scope='project', last_accessed_at=$1 WHERE id='proj'`, old); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(context.Background(), `
		UPDATE chunks SET last_accessed_at=$1 WHERE id='glob'`, old); err != nil {
		t.Fatal(err)
	}

	sw := &Sweeper{conn: conn, cfg: gcTestConfig()}
	if err := sw.sweepOnce(context.Background()); err != nil {
		t.Fatalf("sweepOnce: %v", err)
	}

	var projStatus, globStatus string
	_ = conn.QueryRow(context.Background(), `SELECT status FROM chunks WHERE id='proj'`).Scan(&projStatus)
	_ = conn.QueryRow(context.Background(), `SELECT status FROM chunks WHERE id='glob'`).Scan(&globStatus)
	if projStatus != "archived" {
		t.Fatalf("project row should be archived, got %q", projStatus)
	}
	if globStatus != "active" {
		t.Fatalf("global row must be UNTOUCHED, got %q", globStatus)
	}
	// Audit row for the archive.
	var auditCount int
	_ = conn.QueryRow(context.Background(),
		`SELECT count(*) FROM chunk_audit WHERE chunk_id='proj' AND new_status='archived' AND reason='decay'`,
	).Scan(&auditCount)
	if auditCount != 1 {
		t.Fatalf("expected 1 archive audit row for proj, got %d", auditCount)
	}
	// Global row has NO audit entry.
	var globAudit int
	_ = conn.QueryRow(context.Background(),
		`SELECT count(*) FROM chunk_audit WHERE chunk_id='glob'`).Scan(&globAudit)
	if globAudit != 0 {
		t.Fatalf("global row must not be audited, got %d", globAudit)
	}
}

func TestSweeper_ArchiveIsReversible(t *testing.T) {
	conn := freshDB(t)
	_, err := conn.Exec(context.Background(), `
		INSERT INTO chunks (id, content, embedding, metadata, published_at, scope, status, archived_at)
		VALUES ('a', 'x', NULL, '{}', now(), 'project', 'archived', now())`)
	if err != nil {
		t.Fatal(err)
	}
	// One UPDATE restores it.
	if _, err := conn.Exec(context.Background(),
		`UPDATE chunks SET status='active', archived_at=NULL WHERE id='a'`); err != nil {
		t.Fatal(err)
	}
	var status string
	_ = conn.QueryRow(context.Background(), `SELECT status FROM chunks WHERE id='a'`).Scan(&status)
	if status != "active" {
		t.Fatalf("archive should be reversible by one UPDATE, got %q", status)
	}
}

func TestSweeper_PurgeDead_GatedAndGraced(t *testing.T) {
	conn := freshDB(t)
	old := time.Now().UTC().Add(-40 * 24 * time.Hour)
	recent := time.Now().UTC().Add(-1 * 24 * time.Hour)
	mustInsertDead := func(id string, deadAt time.Time) {
		if _, err := conn.Exec(context.Background(), `
			INSERT INTO chunks (id, content, embedding, metadata, published_at, scope, status, dead_at)
			VALUES ($1, 'x', NULL, '{}', now(), 'project', 'dead', $2)`, id, deadAt); err != nil {
			t.Fatal(err)
		}
	}
	mustInsertDead("old_dead", old)
	mustInsertDead("recent_dead", recent)

	// Gate OFF: nothing deleted.
	swOff := &Sweeper{conn: conn, cfg: gcTestConfig()} // PurgeEnabled=false
	if err := swOff.sweepOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = conn.QueryRow(context.Background(), `SELECT count(*) FROM chunks WHERE status='dead'`).Scan(&n)
	if n != 2 {
		t.Fatalf("purge disabled: both dead rows should survive, got %d", n)
	}

	// Gate ON, 30-day grace: only old_dead (40d) is deleted, recent_dead (1d) survives.
	cfg := gcTestConfig()
	cfg.PurgeEnabled = true
	swOn := &Sweeper{conn: conn, cfg: cfg}
	if err := swOn.sweepOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	var oldExists, recentExists bool
	_ = conn.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM chunks WHERE id='old_dead')`).Scan(&oldExists)
	_ = conn.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM chunks WHERE id='recent_dead')`).Scan(&recentExists)
	if oldExists {
		t.Fatalf("old_dead (40d) should be purged")
	}
	if !recentExists {
		t.Fatalf("recent_dead (1d) should survive the 30-day grace")
	}
	var purgeAudit int
	_ = conn.QueryRow(context.Background(),
		`SELECT count(*) FROM chunk_audit WHERE chunk_id='old_dead' AND new_status='purged'`).Scan(&purgeAudit)
	if purgeAudit != 1 {
		t.Fatalf("expected purge audit row for old_dead, got %d", purgeAudit)
	}
}
