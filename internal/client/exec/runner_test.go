package clientexec

import (
	"errors"
	"runtime"
	"testing"

	"github.com/luannn010/ptolemy/internal/shellcmd"
)

func TestRunCapturesStdoutStderrAndExitCode(t *testing.T) {
	runner, err := New(Options{
		Workspace: t.TempDir(),
		Shell:     shellcmd.DefaultProgram(runtime.GOOS),
	})
	if err != nil {
		t.Fatal(err)
	}

	command := `echo "hello"; echo "oops" >&2; exit 3`
	if runtime.GOOS == "windows" {
		command = `Write-Output "hello"; [Console]::Error.WriteLine("oops"); exit 3`
	}

	result, err := runner.Run(command)
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
		Shell:          shellcmd.DefaultProgram(runtime.GOOS),
		TimeoutSeconds: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	command := `sleep 2`
	if runtime.GOOS == "windows" {
		command = `Start-Sleep -Seconds 2`
	}

	result, err := runner.Run(command)
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
		Shell:     shellcmd.DefaultProgram(runtime.GOOS),
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
