package policy

import (
	"context"
	"testing"

	"github.com/luannn010/ptolemy/internal/store"
	"github.com/luannn010/ptolemy/internal/terminal"
)

type fakeRunner struct {
	calls int
	res   terminal.Result
}

func (f *fakeRunner) Run(_ context.Context, _ string, _ string, _ int) terminal.Result {
	f.calls++
	return f.res
}

func TestGuardedRunnerDenyDoesNotExecute(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/db.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, _ = s.DB.Exec(`INSERT INTO sessions(id,name,status,workspace,description,created_at,updated_at) VALUES('s1','n','open','.','','x','x')`)

	r := &fakeRunner{}
	g := NewGuardedRunner(NewEngine(DefaultRuleset()), NewApprovals(), r, s.DB)

	_, runErr := g.Run(context.Background(), "s1", "cat ./.env", ".", 5)
	if runErr == nil {
		t.Fatalf("expected deny error")
	}
	if r.calls != 0 {
		t.Fatalf("expected no execution")
	}
}

func TestGuardedRunnerAllowExecutes(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/db.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, _ = s.DB.Exec(`INSERT INTO sessions(id,name,status,workspace,description,created_at,updated_at) VALUES('s1','n','open','.','','x','x')`)

	r := &fakeRunner{res: terminal.Result{ExitCode: 0}}
	g := NewGuardedRunner(NewEngine(DefaultRuleset()), NewApprovals(), r, s.DB)

	_, runErr := g.Run(context.Background(), "s1", "go test ./...", ".", 5)
	if runErr != nil {
		t.Fatalf("unexpected error: %v", runErr)
	}
	if r.calls != 1 {
		t.Fatalf("expected one execution")
	}
}
