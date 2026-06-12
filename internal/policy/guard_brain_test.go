package policy

import (
	"context"
	"errors"
	"testing"

	"github.com/luannn010/ptolemy/internal/brain"
)

type fakeBrain struct {
	ensure, wake, stop, unload, switched int
	lastModel                            string
	status                               brain.Status
}

func (f *fakeBrain) EnsureAwake(_ context.Context) error      { f.ensure++; return nil }
func (f *fakeBrain) Wake(_ context.Context, m string) error   { f.wake++; f.lastModel = m; return nil }
func (f *fakeBrain) Switch(_ context.Context, m string) error { f.switched++; f.lastModel = m; return nil }
func (f *fakeBrain) Stop(_ context.Context) error             { f.stop++; return nil }
func (f *fakeBrain) Unload(_ context.Context) error           { f.unload++; return nil }
func (f *fakeBrain) Status() brain.Status                     { return f.status }

func brainRuleset() Ruleset {
	return Ruleset{Rules: []Rule{
		{ID: "allow-brain-wake", Contains: "brain wake", Effect: "allow", Reason: "auto"},
		{ID: "allow-brain-status", Contains: "brain status", Effect: "allow", Reason: "read"},
		{ID: "allow-brain-autounload", Contains: "brain autounload", Effect: "allow", Reason: "idle"},
		{ID: "ask-brain-switch", Contains: "brain switch", Effect: "ask", Channel: "oob", Reason: "manual swap"},
		{ID: "ask-brain-stop", Contains: "brain stop", Effect: "ask", Channel: "oob", Reason: "manual stop"},
	}}
}

func TestGuardedBrain_WakeAllowed(t *testing.T) {
	s := openTestStore(t)
	fb := &fakeBrain{}
	g := NewGuardedBrain(NewEngine(brainRuleset()), NewApprovals(), fb, s.DB)

	if err := g.Wake(context.Background(), "s1", "qwen9b", CallOpts{}); err != nil {
		t.Fatalf("wake: %v", err)
	}
	if fb.wake != 1 || fb.lastModel != "qwen9b" {
		t.Fatalf("raw wake not called: %+v", fb)
	}
	// the allow decision is audited
	var n int
	if err := s.DB.QueryRow(`SELECT count(*) FROM policy_decisions WHERE program='brain'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected one audited brain decision, got %d", n)
	}
}

func TestGuardedBrain_EnsureAndAutounloadAndStatusAllowed(t *testing.T) {
	s := openTestStore(t)
	fb := &fakeBrain{status: brain.Status{Running: true, Model: "qwen9b"}}
	g := NewGuardedBrain(NewEngine(brainRuleset()), NewApprovals(), fb, s.DB)

	if err := g.EnsureAwake(context.Background(), "s1", CallOpts{}); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if err := g.Unload(context.Background(), "s1", CallOpts{}); err != nil {
		t.Fatalf("unload: %v", err)
	}
	st, err := g.Status(context.Background(), "s1", CallOpts{})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if fb.ensure != 1 || fb.unload != 1 || st.Model != "qwen9b" {
		t.Fatalf("allowed ops wrong: %+v st=%+v", fb, st)
	}
}

func TestGuardedBrain_SwitchAsksThenConfirms(t *testing.T) {
	s := openTestStore(t)
	fb := &fakeBrain{}
	approvals := NewApprovals()
	g := NewGuardedBrain(NewEngine(brainRuleset()), approvals, fb, s.DB)

	err := g.Switch(context.Background(), "s1", "qwen4b", CallOpts{})
	var needs ErrNeedsConfirmation
	if !errors.As(err, &needs) {
		t.Fatalf("expected needs-confirmation, got %v", err)
	}
	if fb.switched != 0 {
		t.Fatal("switch must not run before approval")
	}
	// approve out-of-band, then retry with the token
	if !approvals.Approve(needs.PendingID) {
		t.Fatal("approve failed")
	}
	if err := g.Switch(context.Background(), "s1", "qwen4b", CallOpts{ConfirmToken: needs.PendingID}); err != nil {
		t.Fatalf("confirmed switch: %v", err)
	}
	if fb.switched != 1 || fb.lastModel != "qwen4b" {
		t.Fatalf("switch should run after confirmation: %+v", fb)
	}
}

func TestGuardedBrain_StopDeniedDoesNotRun(t *testing.T) {
	s := openTestStore(t)
	fb := &fakeBrain{}
	rs := Ruleset{Rules: []Rule{{ID: "deny-brain-stop", Contains: "brain stop", Effect: "deny", Reason: "no"}}}
	g := NewGuardedBrain(NewEngine(rs), NewApprovals(), fb, s.DB)

	err := g.Stop(context.Background(), "s1", CallOpts{})
	var denied ErrDenied
	if !errors.As(err, &denied) {
		t.Fatalf("expected ErrDenied, got %v", err)
	}
	if fb.stop != 0 {
		t.Fatal("stop must not run when denied")
	}
}
