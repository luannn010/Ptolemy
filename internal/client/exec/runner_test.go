package clientexec

import (
	"errors"
	"testing"
)

func TestRunCapturesStdoutStderrAndExitCode(t *testing.T) {
	runner, err := New(Options{
		Workspace: t.TempDir(),
		Shell:     "/bin/bash",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := runner.Run(`echo "hello"; echo "oops" >&2; exit 3`)
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != "hello\n" {
		t.Fatalf("stdout = %q", result.Stdout)
	}
	if result.Stderr != "oops\n" {
		t.Fatalf("stderr = %q", result.Stderr)
	}
	if result.ExitCode != 3 {
		t.Fatalf("exit code = %d", result.ExitCode)
	}
	if result.TimedOut {
		t.Fatal("did not expect timeout")
	}
}

func TestRunTimeout(t *testing.T) {
	runner, err := New(Options{
		Workspace:      t.TempDir(),
		Shell:          "/bin/bash",
		TimeoutSeconds: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := runner.Run(`sleep 2`)
	if err != nil {
		t.Fatal(err)
	}
	if !result.TimedOut {
		t.Fatal("expected timeout")
	}
}

func TestRunPolicyHook(t *testing.T) {
	runner, err := New(Options{
		Workspace: t.TempDir(),
		Shell:     "/bin/bash",
		Policy: func(command string) error {
			return errors.New("blocked")
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = runner.Run(`echo hi`)
	if !errors.Is(err, ErrDeniedByPolicy) {
		t.Fatalf("expected ErrDeniedByPolicy, got %v", err)
	}
}
