package shellcmd

import (
	"reflect"
	"testing"
)

func TestBuildUsesPowerShellOnWindows(t *testing.T) {
	gotName, gotArgs := Build("windows", "", "Get-Location")

	if gotName != PowerShellExe {
		t.Fatalf("name = %q", gotName)
	}

	wantArgs := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", "Get-Location"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("args = %#v, want %#v", gotArgs, wantArgs)
	}
}

func TestBuildUsesBashOnUnix(t *testing.T) {
	gotName, gotArgs := Build("linux", "", "pwd")

	if gotName != BashExe {
		t.Fatalf("name = %q", gotName)
	}

	wantArgs := []string{"-lc", "pwd"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("args = %#v, want %#v", gotArgs, wantArgs)
	}
}

func TestBuildKeepsPowerShellArgsForExplicitPowerShell(t *testing.T) {
	gotName, gotArgs := Build("linux", "powershell.exe", "Get-Location")

	if gotName != PowerShellExe {
		t.Fatalf("name = %q", gotName)
	}

	wantArgs := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", "Get-Location"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("args = %#v, want %#v", gotArgs, wantArgs)
	}
}

func TestBuildUsesLoginShellArgsForCustomShell(t *testing.T) {
	gotName, gotArgs := Build("windows", "/bin/zsh", "pwd")

	if gotName != "/bin/zsh" {
		t.Fatalf("name = %q", gotName)
	}

	wantArgs := []string{"-lc", "pwd"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("args = %#v, want %#v", gotArgs, wantArgs)
	}
}
