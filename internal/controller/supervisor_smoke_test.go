package controller

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/luannn010/ptolemy/internal/domain"
	"github.com/luannn010/ptolemy/internal/policy"
	"github.com/luannn010/ptolemy/internal/store"
	"github.com/luannn010/ptolemy/internal/worktree"
)

func setupRepoForSmoke(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "chore: initial commit")
	return dir
}

func TestSupervisorWithRealGuardedWorktree(t *testing.T) {
	if testing.Short() {
		t.Skip("smoke test requires real git; skipped under -short")
	}
	repo := setupRepoForSmoke(t)
	wtDir := filepath.Join(repo, ".worktrees")

	// Real DB + a sessions row (policy_decisions FKs to sessions(id)).
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if _, err := st.DB.Exec(`INSERT INTO sessions(id,name,status,workspace,description,created_at,updated_at)
		VALUES('smoke','n','open','.','','x','x')`); err != nil {
		t.Fatal(err)
	}

	// Real worktree manager wrapped by the real guard. DefaultRuleset has no
	// worktree rule (would hit the Ask/OOB fail-safe), so use an allow-worktree
	// ruleset for the test.
	mgr := worktree.NewManager(repo, wtDir)
	rs := policy.Ruleset{Rules: []policy.Rule{
		{ID: "allow-worktree", Contains: "worktree", Effect: domain.EffectAllow, Reason: "test allows worktree ops"},
	}}
	engine := policy.NewEngine(rs)
	approvals := policy.NewApprovals()
	guarded := policy.NewGuardedWorktree(engine, approvals, mgr, repo, st.DB)

	bus := NewBus()
	defer bus.Close()
	s := New(Deps{
		Worktree: guarded,
		Runner:   &fakeRunner{outcome: Outcome{Passed: true, Detail: "green"}},
		Bus:      bus,
		Config:   Config{MaxWorkers: 2},
	})

	id, err := s.Spawn(context.Background(), "smoke", WorkSpec{Name: "smoke-worker"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	w, _ := s.Worker(id)
	if w.State != StateStage1Passed {
		t.Fatalf("got %+v", w)
	}
	if _, err := os.Stat(w.Worktree); err != nil {
		t.Fatalf("expected worktree dir on disk: %v", err)
	}
	s.Shutdown(context.Background())
}
