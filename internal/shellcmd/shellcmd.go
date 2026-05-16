package shellcmd

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	BashExe       = "bash"
	PowerShellExe = "powershell.exe"
)

func DefaultProgram(goos string) string {
	if goos == "windows" {
		return PowerShellExe
	}
	return BashExe
}

func Build(goos string, program string, command string) (string, []string) {
	name := strings.TrimSpace(program)
	if name == "" {
		name = DefaultProgram(goos)
	}

	switch strings.ToLower(filepath.Base(name)) {
	case "powershell.exe", "powershell":
		return name, []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", command}
	case "pwsh.exe", "pwsh":
		return name, []string{"-NoProfile", "-Command", command}
	default:
		return name, []string{"-lc", command}
	}
}

func Command(ctx context.Context, command string) *exec.Cmd {
	return CommandForProgram(ctx, runtime.GOOS, "", command)
}

func CommandForProgram(ctx context.Context, goos string, program string, command string) *exec.Cmd {
	name, args := Build(goos, program, command)
	return exec.CommandContext(ctx, name, args...)
}
