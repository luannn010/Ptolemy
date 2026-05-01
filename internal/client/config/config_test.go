package config

import (
	"runtime"
	"strings"
	"testing"

	"github.com/luannn010/ptolemy/internal/shellcmd"
)

func TestDefaultConfigYAML(t *testing.T) {
	cfg := Default("demo-project", "https://ptolemy.example")
	yaml := cfg.YAML()

	if !strings.Contains(yaml, `server_url: "https://ptolemy.example"`) {
		t.Fatalf("unexpected yaml: %s", yaml)
	}
	if !strings.Contains(yaml, `project_name: "demo-project"`) {
		t.Fatalf("unexpected yaml: %s", yaml)
	}
	if cfg.Shell != shellcmd.DefaultProgram(runtime.GOOS) {
		t.Fatalf("shell = %q", cfg.Shell)
	}
}
