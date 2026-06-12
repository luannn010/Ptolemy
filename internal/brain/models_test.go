package brain

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleRegistry = `{
  "models": [
    {"name": "qwen9b", "binary": "/bin/llama-server", "gguf": "/m/qwen9b.gguf",
     "args": ["--ctx-size", "32768", "-ngl", "999"]},
    {"name": "qwen4b", "binary": "/bin/llama-server", "gguf": "/m/qwen4b.gguf",
     "args": ["--ctx-size", "16384"]}
  ]
}`

func writeRegistry(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "brain-models.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadRegistry_GetKnownModel(t *testing.T) {
	reg, err := LoadRegistry(writeRegistry(t, sampleRegistry))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	m, err := reg.Get("qwen9b")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if m.GGUF != "/m/qwen9b.gguf" || m.Binary != "/bin/llama-server" {
		t.Fatalf("model fields wrong: %+v", m)
	}
	if len(m.Args) != 4 || m.Args[0] != "--ctx-size" {
		t.Fatalf("args wrong: %v", m.Args)
	}
}

func TestLoadRegistry_UnknownModel(t *testing.T) {
	reg, err := LoadRegistry(writeRegistry(t, sampleRegistry))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Get("llama70b"); err == nil {
		t.Fatal("expected error for unknown model")
	}
}

func TestLoadRegistry_MissingFile(t *testing.T) {
	if _, err := LoadRegistry(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadRegistry_EmptyName(t *testing.T) {
	if _, err := LoadRegistry(writeRegistry(t, `{"models":[{"name":"","gguf":"x"}]}`)); err == nil {
		t.Fatal("expected error for empty model name")
	}
}
