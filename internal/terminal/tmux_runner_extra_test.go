package terminal

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func requireTmux(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available; skipping live tmux test")
	}
}

func TestTmuxEnsureBootstrapRunCapture_LiveTmux(t *testing.T) {
	requireTmux(t)
	tr := NewTmuxRunner()
	sess := "extra_" + strings.ReplaceAll(t.Name(), "/", "_")

	defer KillSession(sess)

	if err := tr.EnsureSession(context.Background(), sess, t.TempDir()); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	if err := tr.BootstrapSession(context.Background(), sess, t.TempDir()); err != nil {
		t.Fatalf("BootstrapSession: %v", err)
	}
	if !tr.HasSession(context.Background(), sess) {
		t.Fatalf("HasSession should be true after EnsureSession")
	}

	res := tr.Run(context.Background(), sess, "echo tmux-ok", t.TempDir(), 5)
	if res.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d (err=%q)", res.ExitCode, res.ErrorOutput)
	}
	if !strings.Contains(res.Output, "tmux-ok") {
		t.Fatalf("expected tmux-ok in output: %q", res.Output)
	}

	out, err := tr.CaptureSession(context.Background(), sess)
	if err != nil {
		t.Fatalf("CaptureSession: %v", err)
	}
	if out == "" {
		t.Fatalf("expected non-empty capture")
	}
}

func TestTmuxRunner_HasSessionFalseForUnknown(t *testing.T) {
	requireTmux(t)
	tr := NewTmuxRunner()
	if tr.HasSession(context.Background(), "definitely-not-a-real-session") {
		t.Fatalf("HasSession must be false for a session that was never created")
	}
}

func TestTmuxRunner_KillNonexistentSessionDoesNotPanic(t *testing.T) {
	requireTmux(t)
	// Killing a session that doesn't exist returns nil — the helper swallows the error.
	KillSession("nope_" + t.Name())
}

func TestTmuxRunner_RunFallsBackWhenTmuxMissing(t *testing.T) {
	orig := lookPath
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	defer func() { lookPath = orig }()

	tr := NewTmuxRunner()
	res := tr.Run(context.Background(), "fb", "echo fallback-ok", t.TempDir(), 5)
	if !strings.Contains(res.Output, "fallback-ok") {
		t.Fatalf("expected fallback runner output, got %q", res.Output)
	}
}
