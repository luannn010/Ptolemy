package navigator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var _ = filepath.Join // ensure import not pruned across edits

func TestIgnoredDirsExposesDefaults(t *testing.T) {
	dirs := IgnoredDirs()
	if len(dirs) == 0 {
		t.Fatalf("expected ignored dirs")
	}
	want := map[string]bool{".git": false, "node_modules": false, "dist": false, "vendor": false}
	for _, d := range dirs {
		if _, ok := want[d]; ok {
			want[d] = true
		}
	}
	for d, found := range want {
		if !found {
			t.Fatalf("expected %s in IgnoredDirs(), got %v", d, dirs)
		}
	}
}

func TestAppendSessionNote(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "go.mod", "module test\n")
	if _, err := IndexWorkspace(root); err != nil {
		t.Fatalf("index: %v", err)
	}
	if _, err := StartTaskSession(root, "S1", "task body"); err != nil {
		t.Fatalf("start task: %v", err)
	}
	sess, err := AppendSessionNote(root, "S1", "a note")
	if err != nil {
		t.Fatalf("AppendSessionNote: %v", err)
	}
	if sess.ID != "s1" && sess.ID != "S1" {
		t.Fatalf("unexpected session id %q", sess.ID)
	}
	// Walk session directory and ensure at least one file contains the note.
	root2 := filepath.Join(root, sess.Path)
	found := false
	_ = filepath.Walk(root2, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		data, _ := os.ReadFile(p)
		if strings.Contains(string(data), "a note") {
			found = true
		}
		return nil
	})
	if !found {
		t.Fatalf("note not persisted under %s", root2)
	}
}

func TestRecordFileRead(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "go.mod", "module test\n")
	writeTestFile(t, root, "main.go", "package main\n")
	if _, err := IndexWorkspace(root); err != nil {
		t.Fatalf("index: %v", err)
	}
	if _, err := StartTaskSession(root, "S2", "task body"); err != nil {
		t.Fatalf("start task: %v", err)
	}
	if err := RecordFileRead(root, "S2", "main.go"); err != nil {
		t.Fatalf("RecordFileRead: %v", err)
	}
}

func TestReadContext(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "go.mod", "module test\n")
	if _, err := IndexWorkspace(root); err != nil {
		t.Fatalf("index: %v", err)
	}
	files, err := ReadContext(root)
	if err != nil {
		t.Fatalf("ReadContext: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("expected context files after IndexWorkspace")
	}
	// At least one of the seeded KB files should be present with content.
	seen := false
	for _, f := range files {
		if strings.HasSuffix(f.Path, "PTOLEMY.md") && f.Content != "" {
			seen = true
			break
		}
	}
	if !seen {
		t.Fatalf("PTOLEMY.md not in context: %+v", files)
	}
}

func TestStartTaskSession_PersistsSessionFile(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "go.mod", "module test\n")
	if _, err := IndexWorkspace(root); err != nil {
		t.Fatalf("index: %v", err)
	}
	sess, err := StartTaskSession(root, "task-X", "what to do")
	if err != nil {
		t.Fatalf("StartTaskSession: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, sess.Path)); err != nil {
		t.Fatalf("session file missing: %v", err)
	}
}

func TestCommandsArchitectureEnvConventions_GenerateMarkdown(t *testing.T) {
	root := t.TempDir()
	// Touch a few hint files so each generator has something to emit.
	writeTestFile(t, root, "Makefile", "run:\n\tgo run .\n")
	writeTestFile(t, root, "package.json", "{}\n")
	writeTestFile(t, root, "main.go", "package main\n")
	writeTestFile(t, root, ".env.example", "KEY=value\n")

	// These are unexported helpers but exercised via IndexWorkspace, which
	// composes them. Run an index and confirm the resulting KB markdown
	// contains some of the expected sections.
	if _, err := IndexWorkspace(root); err != nil {
		t.Fatalf("index: %v", err)
	}
	for _, p := range []string{
		filepath.Join(".ptolemy", "kb", "PROJECT_MAP.md"),
		filepath.Join(".ptolemy", "kb", "WORKFLOWS.md"),
		filepath.Join(".ptolemy", "kb", "DECISIONS.md"),
	} {
		full := filepath.Join(root, p)
		data, err := os.ReadFile(full)
		if err != nil {
			t.Fatalf("expected %s to exist: %v", p, err)
		}
		if len(data) == 0 {
			t.Fatalf("%s is empty", p)
		}
	}
}
