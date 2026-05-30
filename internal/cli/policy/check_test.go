package policy

import (
	"bytes"
	"strings"
	"testing"

	"github.com/luannn010/ptolemy/internal/domain"
	"github.com/luannn010/ptolemy/internal/policy"
)

// runCheck must print exactly one verdict line per sample command, in order,
// and must not depend on the working directory (the engine is injected).
func TestRunCheckPrintsOneLinePerSample(t *testing.T) {
	eng := policy.NewEngine(policy.Ruleset{Rules: []policy.Rule{
		{ID: "allow-build-test", Contains: "go test", Effect: domain.EffectAllow, Reason: "ok"},
		{ID: "ask-push-cmd", Contains: "git push", Effect: domain.EffectAsk, Channel: domain.ChannelOOB, Reason: "confirm"},
	}})
	samples := []string{"go test ./...", "git push origin main", "echo hi"}

	var buf bytes.Buffer
	if err := runCheck(&buf, eng, samples); err != nil {
		t.Fatalf("runCheck returned error: %v", err)
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != len(samples) {
		t.Fatalf("got %d lines, want %d\noutput:\n%s", len(lines), len(samples), buf.String())
	}

	// The allow rule should surface its effect+ruleID; the unmatched sample
	// falls through to the fail-safe default (ask/oob).
	if !strings.Contains(lines[0], "go test ./...") || !strings.Contains(lines[0], string(domain.EffectAllow)) {
		t.Errorf("line 0 missing command or allow effect: %q", lines[0])
	}
	if !strings.Contains(lines[1], string(domain.EffectAsk)) {
		t.Errorf("line 1 missing ask effect: %q", lines[1])
	}
	if !strings.Contains(lines[2], "default") {
		t.Errorf("line 2 (unmatched) should hit default rule: %q", lines[2])
	}
}
