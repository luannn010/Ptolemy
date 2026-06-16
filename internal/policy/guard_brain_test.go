package policy

import (
	"context"
	"errors"
	"testing"

	"github.com/luannn010/ptolemy/internal/brain"
)

// stubRawBrain records calls for the new controller verb set.
type stubRawBrain struct {
	loaded  *brain.Spec
	resumed int
	ensured int
	hibern  int
	stopped int
	listed  int
	status  brain.Status
}

func (s *stubRawBrain) ResolveSpec(sp brain.Spec) brain.Spec        { return sp }
func (s *stubRawBrain) Load(_ context.Context, sp brain.Spec) error { s.loaded = &sp; return nil }
func (s *stubRawBrain) Resume(context.Context) error                { s.resumed++; return nil }
func (s *stubRawBrain) EnsureAwake(context.Context) error           { s.ensured++; return nil }
func (s *stubRawBrain) Hibernate(context.Context) error             { s.hibern++; return nil }
func (s *stubRawBrain) Stop(context.Context) error                  { s.stopped++; return nil }
func (s *stubRawBrain) Status() brain.Status                        { return s.status }
func (s *stubRawBrain) ListModels() ([]brain.DiscoveredModel, error) {
	s.listed++
	return nil, nil
}

func brainRuleset() Ruleset {
	return Ruleset{Rules: []Rule{
		{ID: "deny-destructive-rm", Contains: "rm -rf", Effect: "deny", Reason: "destructive"},
		{ID: "allow-brain-wake", Contains: "brain wake", Effect: "allow", Reason: "auto"},
		{ID: "allow-brain-status", Contains: "brain status", Effect: "allow", Reason: "read"},
		{ID: "allow-brain-models", Contains: "brain models", Effect: "allow", Reason: "read"},
		{ID: "allow-brain-resume", Contains: "brain resume", Effect: "allow", Reason: "resume"},
		{ID: "allow-brain-hibernate", Contains: "brain hibernate", Effect: "allow", Reason: "idle"},
		{ID: "ask-brain-load", Contains: "brain load", Effect: "ask", Channel: "oob", Reason: "custom launch"},
		{ID: "ask-brain-stop", Contains: "brain stop", Effect: "ask", Channel: "oob", Reason: "manual stop"},
	}}
}

func TestGuardedBrain_LoadIsAsk_RawNotCalledUntilApproved(t *testing.T) {
	s := openTestStore(t)
	raw := &stubRawBrain{}
	appr := NewApprovals()
	g := NewGuardedBrain(NewEngine(brainRuleset()), appr, raw, s.DB)

	err := g.Load(context.Background(), "s1", brain.Spec{Binary: "/b", GGUF: "/g"}, CallOpts{})
	var needs ErrNeedsConfirmation
	if !errors.As(err, &needs) {
		t.Fatalf("load must need confirmation, got %v", err)
	}
	if raw.loaded != nil {
		t.Fatal("raw.Load must NOT run before approval")
	}

	if !appr.Approve(needs.PendingID) {
		t.Fatal("approve failed")
	}
	if err := g.Load(context.Background(), "s1", brain.Spec{Binary: "/b", GGUF: "/g"}, CallOpts{ConfirmToken: needs.PendingID}); err != nil {
		t.Fatalf("approved retry: %v", err)
	}
	if raw.loaded == nil || raw.loaded.GGUF != "/g" {
		t.Fatalf("raw.Load must run after approval: %+v", raw.loaded)
	}
}

func TestGuardedBrain_AllowVerbs(t *testing.T) {
	s := openTestStore(t)
	raw := &stubRawBrain{status: brain.Status{Running: true, GGUF: "/g"}}
	g := NewGuardedBrain(NewEngine(brainRuleset()), NewApprovals(), raw, s.DB)
	ctx := context.Background()

	if err := g.Resume(ctx, "s1", CallOpts{}); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if err := g.EnsureAwake(ctx, "s1", CallOpts{}); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if err := g.Hibernate(ctx, "s1", CallOpts{}); err != nil {
		t.Fatalf("hibernate: %v", err)
	}
	if _, err := g.ListModels(ctx, "s1", CallOpts{}); err != nil {
		t.Fatalf("models: %v", err)
	}
	st, err := g.Status(ctx, "s1", CallOpts{})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if raw.resumed != 1 || raw.ensured != 1 || raw.hibern != 1 || raw.listed != 1 || st.GGUF != "/g" {
		t.Fatalf("allow verbs must call raw: %+v st=%+v", raw, st)
	}
	// each allow op is audited
	var n int
	if err := s.DB.QueryRow(`SELECT count(*) FROM policy_decisions WHERE program='brain'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("expected 5 audited brain decisions, got %d", n)
	}
}

func TestGuardedBrain_StopIsAsk(t *testing.T) {
	s := openTestStore(t)
	raw := &stubRawBrain{}
	g := NewGuardedBrain(NewEngine(brainRuleset()), NewApprovals(), raw, s.DB)
	err := g.Stop(context.Background(), "s1", CallOpts{})
	var needs ErrNeedsConfirmation
	if !errors.As(err, &needs) {
		t.Fatalf("stop must need confirmation, got %v", err)
	}
	if raw.stopped != 0 {
		t.Fatal("raw.Stop must not run before approval")
	}
}

func TestGuardedBrain_DenyTokenInSpecFieldIsDenied(t *testing.T) {
	s := openTestStore(t)
	raw := &stubRawBrain{}
	g := NewGuardedBrain(NewEngine(brainRuleset()), NewApprovals(), raw, s.DB)
	// "rm -rf" smuggled as a flag value -> deny rule trips on the full argv.
	err := g.Load(context.Background(), "s1",
		brain.Spec{Binary: "/b", GGUF: "/g", Args: []string{"--x", "rm -rf /"}}, CallOpts{})
	var denied ErrDenied
	if !errors.As(err, &denied) {
		t.Fatalf("destructive token in a spec field must be denied, got %v", err)
	}
	if raw.loaded != nil {
		t.Fatal("denied load must not run")
	}
}
