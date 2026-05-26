package shellcmd

import (
	"context"
	"strings"
	"testing"
)

func TestDefaultProgram(t *testing.T) {
	if got := DefaultProgram("windows"); got != PowerShellExe {
		t.Fatalf("windows default: got %q want %q", got, PowerShellExe)
	}
	if got := DefaultProgram("linux"); got != BashExe {
		t.Fatalf("linux default: got %q want %q", got, BashExe)
	}
	if got := DefaultProgram("darwin"); got != BashExe {
		t.Fatalf("darwin default: got %q want %q", got, BashExe)
	}
}

func TestBuild_DefaultsByGOOS(t *testing.T) {
	name, args := Build("linux", "", "echo hi")
	if name != BashExe {
		t.Fatalf("empty program on linux must default to bash, got %q", name)
	}
	if len(args) < 2 || args[0] != "-lc" || args[len(args)-1] != "echo hi" {
		t.Fatalf("bash args malformed: %v", args)
	}
}

func TestBuild_PowerShell(t *testing.T) {
	_, args := Build("windows", "powershell.exe", "Get-Process")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-NoProfile") || !strings.Contains(joined, "-ExecutionPolicy") {
		t.Fatalf("expected NoProfile + ExecutionPolicy in args: %v", args)
	}
}

func TestBuild_Pwsh(t *testing.T) {
	_, args := Build("linux", "/usr/local/bin/pwsh", "Get-Date")
	if args[0] != "-NoProfile" || args[len(args)-1] != "Get-Date" {
		t.Fatalf("pwsh args malformed: %v", args)
	}
}

func TestCommand_NotNil(t *testing.T) {
	cmd := Command(context.Background(), "echo hi")
	if cmd == nil {
		t.Fatalf("Command returned nil")
	}
	if cmd.Path == "" {
		t.Fatalf("command path must be set")
	}
}

func TestCommandForProgram_ExplicitBash(t *testing.T) {
	cmd := CommandForProgram(context.Background(), "linux", "bash", "echo hi")
	if cmd == nil {
		t.Fatalf("nil cmd")
	}
	if cmd.Args[len(cmd.Args)-1] != "echo hi" {
		t.Fatalf("last arg should be the command string, got %v", cmd.Args)
	}
}
