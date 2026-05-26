package policy

import (
	"testing"

	"github.com/luannn010/ptolemy/internal/domain"
)

func decideShell(t *testing.T, e *Engine, line string) domain.Decision {
	t.Helper()
	toks := TokenizeShell(line)
	if len(toks) == 0 {
		return e.Authorize(domain.Intent{Kind: "shell.run"})
	}
	return e.Authorize(domain.Intent{Kind: "shell.run", Program: toks[0], Args: toks[1:]})
}

func TestAuthorizeBypassSuite(t *testing.T) {
	e := NewEngine(DefaultRuleset())

	cases := []struct {
		name   string
		line   string
		effect domain.Effect
		rule   string
	}{
		{"allow-build-test", "go test ./...", domain.EffectAllow, "allow-build-test"},
		{"ask-push", "git push origin main", domain.EffectAsk, "ask-push-cmd"},
		{"ask-push-double-space", "git  push  origin  main", domain.EffectAsk, "ask-push-cmd"},
		{"ask-push-tab", "git\tpush\torigin\tmain", domain.EffectAsk, "ask-push-cmd"},
		{"deny-force-push", "git push --force origin main", domain.EffectDeny, "deny-destructive"},
		{"deny-secret", "cat ./.env", domain.EffectDeny, "deny-secret-cmd"},
		{"deny-compound-secret", "go build && cat ./.env", domain.EffectDeny, "deny-secret-cmd"},
		{"deny-compound-semicolon", "go test ./...; cat .env", domain.EffectDeny, "deny-secret-cmd"},
		{"deny-compound-pipe", "ls | grep secret > .env && cat .env", domain.EffectDeny, "deny-secret-cmd"},
		{"deny-newline-injection", "go test ./...\ncat .env", domain.EffectDeny, "deny-secret-cmd"},
		{"ask-network", "curl http://example.com", domain.EffectAsk, "ask-network"},
		{"deny-rm-rf", "rm -rf /", domain.EffectDeny, "deny-destructive-rm"},
		{"default-ask", "frobnicate --all", domain.EffectAsk, "default"},
		{"deny-policy-write", "vim .ptolemy/policy.json", domain.EffectDeny, "deny-policy-write"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := decideShell(t, e, tc.line)
			if d.Effect != tc.effect {
				t.Fatalf("effect: got %q want %q (rule=%s reason=%s)", d.Effect, tc.effect, d.RuleID, d.Reason)
			}
			if d.RuleID != tc.rule {
				t.Fatalf("rule: got %q want %q", d.RuleID, tc.rule)
			}
		})
	}
}

func TestAuthorizeTargetSmuggling(t *testing.T) {
	e := NewEngine(DefaultRuleset())
	// A file.read intent whose program looks innocent must still be caught
	// when the target is a secret file — the engine appends Targets to the
	// matched string for exactly this reason.
	d := e.Authorize(domain.Intent{
		Kind:    "file.read",
		Program: "read",
		Targets: []string{"./.env"},
	})
	if d.Effect != domain.EffectDeny {
		t.Fatalf("expected deny via target, got %q (rule=%s)", d.Effect, d.RuleID)
	}
}

func TestAuthorizeMostRestrictiveWins(t *testing.T) {
	e := NewEngine(DefaultRuleset())
	// `go test` would allow, but `cat .env` in the same compound must deny.
	d := decideShell(t, e, "go test ./... && cat .env")
	if d.Effect != domain.EffectDeny {
		t.Fatalf("most-restrictive lost: got %q (rule=%s)", d.Effect, d.RuleID)
	}
}

func TestTokenizeShellQuotes(t *testing.T) {
	toks := TokenizeShell(`git commit -m "hello world" --author='x y'`)
	want := []string{"git", "commit", "-m", "hello world", "--author=x y"}
	if len(toks) != len(want) {
		t.Fatalf("len: got %v want %v", toks, want)
	}
	for i := range toks {
		if toks[i] != want[i] {
			t.Fatalf("tok[%d]: got %q want %q", i, toks[i], want[i])
		}
	}
}

func TestSplitCompoundQuotesPreserveSeparators(t *testing.T) {
	segs := SplitCompound(`echo "a && b" ; echo c`)
	if len(segs) != 2 {
		t.Fatalf("expected 2 segments, got %d: %v", len(segs), segs)
	}
	if segs[0] != `echo "a && b"` {
		t.Fatalf("seg[0]: %q", segs[0])
	}
}

func TestGlobToRegexp(t *testing.T) {
	re := GlobToRegexp("*.env")
	if re == nil || !re.MatchString(".env") || !re.MatchString("prod.env") {
		t.Fatalf("glob *.env should match .env files")
	}
	if re.MatchString("env.txt") {
		t.Fatalf("glob *.env must not match env.txt")
	}
}
