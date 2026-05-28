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
		UPDATE chunks SET scope='project', subject_id='userT', last_accessed_at=$1 WHERE id='proj'`, old); err != nil {
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
		INSERT INTO chunks (id, content, embedding, metadata, published_at, scope, subject_id, status, archived_at)
		VALUES ('a', 'x', NULL, '{}', now(), 'project', 'userT', 'archived', now())`)
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

func TestRunLoop_ContinuesAfterTickError(t *testing.T) {
	calls := 0
	tick := func(_ context.Context) error {
		calls++
		if calls < 3 {
			return context.DeadlineExceeded // simulate a failing tick
		}
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runLoop(ctx, time.Millisecond, tick)
		close(done)
	}()
	// Let several ticks fire, then cancel.
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runLoop did not return after cancel")
	}
	if calls < 3 {
		t.Fatalf("loop should have continued past failing ticks, got %d calls", calls)
	}
}

func TestRunLoop_StopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	returned := make(chan struct{})
	go func() {
		runLoop(ctx, time.Millisecond, func(_ context.Context) error { return nil })
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("runLoop should return promptly on a cancelled ctx")
	}
}

func dedupCfg(enabled bool) GCConfig {
	return GCConfig{
		DedupEnabled:     enabled,
		DedupThreshold:   0.5,
		DedupLookback:    24 * time.Hour,
		DecayLambda:      0.05,
		ArchiveThreshold: 0.1,
	}
}

func TestSweeper_Dedup_ContradictionPairBothSurvive(t *testing.T) {
	conn := freshDB(t)
	ctx := context.Background()
	s := NewPgStore(conn)
	now := time.Now().UTC()
	// High trigram similarity but NOT normalized-equal: a contradiction.
	subj := "userT"
	_ = s.Upsert(ctx, []Chunk{
		{ID: "c8088", Content: "the service runs on port 8088", Embedding: []float32{1, 0, 0, 0}, PublishedAt: now, Scope: "project", SubjectID: &subj},
		{ID: "c1089", Content: "the service runs on port 1089", Embedding: []float32{0, 1, 0, 0}, PublishedAt: now, Scope: "project", SubjectID: &subj},
	})
	sw := NewSweeper(conn, dedupCfg(true))
	if err := sw.dedupRecent(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(ctx, []string{"c8088", "c1089"})
	for _, c := range got {
		if c.Status != "active" {
			t.Errorf("contradiction row %s status=%q, want active (never collapsed)", c.ID, c.Status)
		}
	}
	if len(got) != 2 {
		t.Fatalf("expected both contradiction rows to survive, got %d", len(got))
	}
}

func TestSweeper_Dedup_NearIdenticalCollapses(t *testing.T) {
	conn := freshDB(t)
	ctx := context.Background()
	old := time.Now().UTC().Add(-time.Hour)
	newer := time.Now().UTC()
	// Normalized-equal (differ only by whitespace). Older = survivor.
	_, _ = conn.Exec(ctx, `INSERT INTO chunks (id, content, scope, subject_id, status, created_at, access_count)
	  VALUES ('keep','user prefers tabs','project','userT','active',$1,3),
	         ('drop','user   prefers   tabs','project','userT','active',$2,0)`, old, newer)
	sw := NewSweeper(conn, dedupCfg(true))
	if err := sw.dedupRecent(ctx); err != nil {
		t.Fatal(err)
	}
	var keepStatus, dropStatus string
	_ = conn.QueryRow(ctx, `SELECT status FROM chunks WHERE id='keep'`).Scan(&keepStatus)
	_ = conn.QueryRow(ctx, `SELECT status FROM chunks WHERE id='drop'`).Scan(&dropStatus)
	if keepStatus != "active" {
		t.Errorf("survivor keep status=%q, want active", keepStatus)
	}
	if dropStatus != "dead" {
		t.Errorf("loser drop status=%q, want dead", dropStatus)
	}
	var ac int
	_ = conn.QueryRow(ctx, `SELECT access_count FROM chunks WHERE id='keep'`).Scan(&ac)
	if ac < 4 {
		t.Errorf("survivor access_count=%d, want reinforced (>3)", ac)
	}
	var auditN int
	_ = conn.QueryRow(ctx, `SELECT count(*) FROM chunk_audit WHERE chunk_id='drop' AND new_status='dead' AND reason='duplicate'`).Scan(&auditN)
	if auditN != 1 {
		t.Errorf("expected 1 dead/duplicate audit row for drop, got %d", auditN)
	}
}

func TestSweeper_Dedup_GatedOffIsNoOp(t *testing.T) {
	conn := freshDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	_, _ = conn.Exec(ctx, `INSERT INTO chunks (id, content, scope, subject_id, status, created_at)
	  VALUES ('a','same text','project','userT','active',$1),('b','same text','project','userT','active',$1)`, now)
	sw := NewSweeper(conn, dedupCfg(false)) // gated OFF
	if err := sw.dedupRecent(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = conn.QueryRow(ctx, `SELECT count(*) FROM chunks WHERE status='active'`).Scan(&n)
	if n != 2 {
		t.Errorf("gated-off dedup changed rows: active=%d, want 2", n)
	}
}

func TestSweeper_Dedup_SurvivorByAccessCount(t *testing.T) {
	conn := freshDB(t)
	ctx := context.Background()
	older := time.Now().UTC().Add(-time.Hour)
	newer := time.Now().UTC()
	// Normalized-equal content; the NEWER row has the higher access_count and must win
	// (proves the access_count branch, not just the created_at tie-break).
	if _, err := conn.Exec(ctx, `INSERT INTO chunks (id, content, scope, subject_id, status, created_at, access_count)
	  VALUES ('old','user prefers spaces','project','userT','active',$1,0),
	         ('new','user prefers spaces','project','userT','active',$2,5)`, older, newer); err != nil {
		t.Fatal(err)
	}
	sw := NewSweeper(conn, dedupCfg(true))
	if err := sw.dedupRecent(ctx); err != nil {
		t.Fatal(err)
	}
	var oldStatus, newStatus string
	var newAC int
	_ = conn.QueryRow(ctx, `SELECT status FROM chunks WHERE id='old'`).Scan(&oldStatus)
	_ = conn.QueryRow(ctx, `SELECT status, access_count FROM chunks WHERE id='new'`).Scan(&newStatus, &newAC)
	if newStatus != "active" {
		t.Errorf("higher-access_count row 'new' should survive, got %q", newStatus)
	}
	if oldStatus != "dead" {
		t.Errorf("lower-access_count row 'old' should die, got %q", oldStatus)
	}
	if newAC < 6 {
		t.Errorf("survivor access_count=%d, want reinforced to >=6", newAC)
	}
}

func TestSweeper_CollapseDuplicate_LoserAlreadyDeadIsNoOp(t *testing.T) {
	conn := freshDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := conn.Exec(ctx, `INSERT INTO chunks (id, content, scope, subject_id, status, created_at, access_count)
	  VALUES ('surv','x','project','userT','active',$1,0),
	         ('los','x','project','userT','dead',$1,0)`, now); err != nil {
		t.Fatal(err)
	}
	sw := NewSweeper(conn, dedupCfg(true))
	if err := sw.collapseDuplicate(ctx, "surv", "los"); err != nil {
		t.Fatal(err)
	}
	var survAC int
	_ = conn.QueryRow(ctx, `SELECT access_count FROM chunks WHERE id='surv'`).Scan(&survAC)
	if survAC != 0 {
		t.Errorf("survivor must NOT be reinforced when loser already dead, got access_count=%d", survAC)
	}
	var auditN int
	_ = conn.QueryRow(ctx, `SELECT count(*) FROM chunk_audit WHERE chunk_id='los' AND reason='duplicate'`).Scan(&auditN)
	if auditN != 0 {
		t.Errorf("no false audit row should be written when loser already dead, got %d", auditN)
	}
}

func TestSweeper_PurgeDead_GatedAndGraced(t *testing.T) {
	conn := freshDB(t)
	old := time.Now().UTC().Add(-40 * 24 * time.Hour)
	recent := time.Now().UTC().Add(-1 * 24 * time.Hour)
	mustInsertDead := func(id string, deadAt time.Time) {
		if _, err := conn.Exec(context.Background(), `
			INSERT INTO chunks (id, content, embedding, metadata, published_at, scope, subject_id, status, dead_at)
			VALUES ($1, 'x', NULL, '{}', now(), 'project', 'userT', 'dead', $2)`, id, deadAt); err != nil {
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
