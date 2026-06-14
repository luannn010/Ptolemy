package brain

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverModels_FindsGGUFRecursivelyAndIgnoresOthers(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.gguf"), "x")
	mustWrite(t, filepath.Join(root, "sub", "b.GGUF"), "yy")
	mustWrite(t, filepath.Join(root, "notes.txt"), "ignore")

	got, err := DiscoverModels(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 gguf, got %d (%+v)", len(got), got)
	}
	if got[0].Name != "a.gguf" || got[0].Size != 1 {
		t.Fatalf("unexpected first model: %+v", got[0])
	}
}

func TestDiscoverModels_MissingOrEmptyRoot(t *testing.T) {
	got, err := DiscoverModels(filepath.Join(t.TempDir(), "nope"))
	if err != nil || len(got) != 0 {
		t.Fatalf("missing root: got %v err %v", got, err)
	}
	if got, err := DiscoverModels(""); err != nil || len(got) != 0 {
		t.Fatalf("empty root: got %v err %v", got, err)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
