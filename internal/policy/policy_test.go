package policy

import "testing"

func TestCheckCommandAllowsSafeCommand(t *testing.T) {
	decision := CheckCommand("echo hello")

	if decision.Mode != ModeAllow {
		t.Fatalf("expected allow, got %s", decision.Mode)
	}
}

func TestCheckCommandAsksForGitPush(t *testing.T) {
	decision := CheckCommand("git push origin main")

	if decision.Mode != ModeAsk {
		t.Fatalf("expected ask, got %s", decision.Mode)
	}

	if decision.ActionType != "git.push" {
		t.Fatalf("expected git.push, got %s", decision.ActionType)
	}
}

func TestCheckCommandAsksForRmRf(t *testing.T) {
	decision := CheckCommand("rm -rf tmp")

	if decision.Mode != ModeAsk {
		t.Fatalf("expected ask, got %s", decision.Mode)
	}

	if decision.ActionType != "filesystem.delete_recursive" {
		t.Fatalf("expected filesystem.delete_recursive, got %s", decision.ActionType)
	}
}

func TestCheckCommandDeniesEnvRead(t *testing.T) {
	decision := CheckCommand("cat .env")

	if decision.Mode != ModeDeny {
		t.Fatalf("expected deny, got %s", decision.Mode)
	}
}
