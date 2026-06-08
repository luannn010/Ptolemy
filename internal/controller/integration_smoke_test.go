package controller

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/luannn010/ptolemy/internal/domain"
	"github.com/luannn010/ptolemy/internal/gitops"
	"github.com/luannn010/ptolemy/internal/policy"
	"github.com/luannn010/ptolemy/internal/store"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestIntegrateWithRealGuardedGitMerge(t *testing.T) {
	if testing.Short() {
		t.Skip("smoke test requires real git; skipped under -short")
	}
	repo := t.TempDir()
	gitRun(t, repo, "init", "-b", "main")
	gitRun(t, repo, "config", "user.email", "test@example.com")
	gitRun(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-m", "chore: base")

	// A feature branch with one extra commit, ready to merge into main.
	gitRun(t, repo, "checkout", "-b", "feature/x")
	if err := os.WriteFile(filepath.Join(repo, "feature.txt"), []byte("work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-m", "feat: x")
	gitRun(t, repo, "checkout", "main")

	// Real DB + sessions row (policy_decisions FKs to sessions(id)).
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if _, err := st.DB.Exec(`INSERT INTO sessions(id,name,status,workspace,description,created_at,updated_at)
		VALUES('smoke','n','open','.','','x','x')`); err != nil {
		t.Fatal(err)
	}

	// Real GuardedGit with an allow-git ruleset (DefaultRuleset would Ask on merge).
	raw := gitops.New(repo)
	rs := policy.Ruleset{Rules: []policy.Rule{
		{ID: "allow-git", Contains: "git", Effect: domain.EffectAllow, Reason: "test allows git ops"},
	}}
	guarded := policy.NewGuardedGit(policy.NewEngine(rs), policy.NewApprovals(), raw, repo, st.DB)

	// Drive integrate() via a one-worker supervisor whose Stage-1 passes and
	// whose worker branch is feature/x.
	bus := NewBus()
	defer bus.Close()
	s := New(Deps{
		Worktree: &fakeWorktree{},
		Runner:   &fakeRunner{outcome: Outcome{Passed: true}},
		Bus:      bus,
		Config:   Config{MaxWorkers: 1, BaseBranch: "main"},
		Lock:     newFakeLock(),
		Stage2:   &fakeStage2{outcome: Outcome{Passed: true}},
		Merger:   guarded,
	})
	id, _ := s.Spawn(context.Background(), "smoke", WorkSpec{Name: "x", Branch: "feature/x"})
	if err := s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	w, _ := s.Worker(id)
	if w.State != StateMerged {
		t.Fatalf("got %+v", w)
	}
	// feature.txt should now exist on main (merge succeeded).
	if _, err := os.Stat(filepath.Join(repo, "feature.txt")); err != nil {
		t.Fatalf("expected merged file on main: %v", err)
	}
}

func TestPgLockSerializesWhenDatabaseURLSet(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL unset; skipping Postgres advisory-lock smoke")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool: %v", err)
	}
	defer pool.Close()

	lock := NewPgLock(pool, 0x70746C6D, time.Minute) // arbitrary fixed key
	rel1, err := lock.Acquire(ctx)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	// Second acquire must block until rel1; prove with a short-ctx try that fails.
	tctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	rel2, err := lock.Acquire(tctx)
	if err == nil {
		rel2()
		rel1()
		t.Fatal("expected second acquire to block while lock held")
	}

	rel1()
	// Now it should be acquirable.
	rel3, err := lock.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	rel3()
}
