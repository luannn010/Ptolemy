package policy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/luannn010/ptolemy/internal/domain"
)

func TestDefaultRuleset_HasBrainRules(t *testing.T) {
	got := map[string]domain.Effect{}
	for _, r := range DefaultRuleset().Rules {
		got[r.ID] = r.Effect
	}
	want := map[string]domain.Effect{
		"allow-brain-wake":      domain.EffectAllow,
		"allow-brain-status":    domain.EffectAllow,
		"allow-brain-models":    domain.EffectAllow,
		"allow-brain-resume":    domain.EffectAllow,
		"allow-brain-hibernate": domain.EffectAllow,
		"ask-brain-load":        domain.EffectAsk,
		"ask-brain-stop":        domain.EffectAsk,
	}
	for id, eff := range want {
		if got[id] != eff {
			t.Fatalf("DefaultRuleset rule %q: got %q want %q", id, got[id], eff)
		}
	}
	for _, gone := range []string{"ask-brain-switch", "allow-brain-autounload"} {
		if _, ok := got[gone]; ok {
			t.Fatalf("DefaultRuleset rule %q should have been removed", gone)
		}
	}
	for _, id := range []string{"deny-policy-write", "deny-secret-cmd"} {
		if _, ok := got[id]; !ok {
			t.Fatalf("DefaultRuleset must keep self-protection rule %q", id)
		}
	}
}

func TestLoadRuleset_FileMissingFallsBackToDefault(t *testing.T) {
	rs := LoadRuleset(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if len(rs.Rules) == 0 {
		t.Fatalf("missing file must yield DefaultRuleset, got empty rules")
	}
	if rs.Rules[0].ID != "deny-policy-write" {
		t.Fatalf("expected default ruleset's first rule, got %q", rs.Rules[0].ID)
	}
}

func TestLoadRuleset_InvalidJSONFallsBackToDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	rs := LoadRuleset(path)
	if len(rs.Rules) == 0 || rs.Rules[0].ID != "deny-policy-write" {
		t.Fatalf("invalid JSON must yield DefaultRuleset")
	}
}

func TestLoadRuleset_EmptyRulesArrayFallsBackToDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(path, []byte(`{"rules":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	rs := LoadRuleset(path)
	if len(rs.Rules) == 0 || rs.Rules[0].ID != "deny-policy-write" {
		t.Fatalf("empty rules must yield DefaultRuleset")
	}
}

func TestLoadRuleset_ValidFileParses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ok.json")
	body := `{"rules":[{"id":"only","contains":"frob","effect":"deny","reason":"r"}]}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	rs := LoadRuleset(path)
	if len(rs.Rules) != 1 || rs.Rules[0].ID != "only" {
		t.Fatalf("expected single custom rule, got %+v", rs)
	}
	if rs.Rules[0].Effect != domain.EffectDeny {
		t.Fatalf("effect must round-trip via JSON")
	}
}

func TestErrDenied_Error(t *testing.T) {
	e := ErrDenied{RuleID: "x", Reason: "y"}
	if e.Error() != "denied: y" {
		t.Fatalf("unexpected Error(): %q", e.Error())
	}
}

func TestErrNeedsConfirmation_Error(t *testing.T) {
	e := ErrNeedsConfirmation{PendingID: "p", Reason: "r"}
	if e.Error() != "needs confirmation" {
		t.Fatalf("unexpected Error(): %q", e.Error())
	}
}
