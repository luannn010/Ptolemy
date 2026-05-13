package navigator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildFileTreeIgnoresHeavyDirectories(t *testing.T) {
	root := t.TempDir()

	writeTestFile(t, root, "main.go", "package main\n")
	writeTestFile(t, root, "node_modules/pkg/index.js", "ignored\n")
	writeTestFile(t, root, ".git/config", "ignored\n")
	writeTestFile(t, root, "dist/app.js", "ignored\n")

	tree, err := BuildFileTree(root)
	if err != nil {
		t.Fatalf("build file tree: %v", err)
	}

	if !treeHasPath(tree, "main.go") {
		t.Fatalf("expected useful file in tree: %+v", tree.Files)
	}

	for _, ignored := range []string{"node_modules/pkg/index.js", ".git/config", "dist/app.js"} {
		if treeHasPath(tree, ignored) {
			t.Fatalf("expected %s to be ignored: %+v", ignored, tree.Files)
		}
	}
}

func TestIndexWorkspaceBootstrapsContextAndFileTree(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "go.mod", "module test\n")
	if err := os.Mkdir(filepath.Join(root, "internal"), 0o755); err != nil {
		t.Fatalf("create internal dir: %v", err)
	}

	result, err := IndexWorkspace(root)
	if err != nil {
		t.Fatalf("index workspace: %v", err)
	}

	if result.FileCount == 0 {
		t.Fatalf("expected indexed files")
	}

	for _, rel := range []string{
		".ptolemy/PTOLEMY.md",
		".ptolemy/context/project-map.md",
		".ptolemy/context/commands.md",
		".ptolemy/context/architecture.md",
		".ptolemy/context/env.md",
		".ptolemy/context/conventions.md",
		".ptolemy/index/file-tree.json",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected %s to exist: %v", rel, err)
		}
	}

	data, err := os.ReadFile(filepath.Join(root, ".ptolemy", "index", "file-tree.json"))
	if err != nil {
		t.Fatalf("read file tree: %v", err)
	}

	var tree FileTree
	if err := json.Unmarshal(data, &tree); err != nil {
		t.Fatalf("file tree should be valid JSON: %v", err)
	}
	if !treeHasPath(tree, "go.mod") {
		t.Fatalf("expected go.mod in file tree: %+v", tree.Files)
	}
}

func TestReadContextReturnsPtolemyAndMarkdownContext(t *testing.T) {
	root := t.TempDir()
	if _, err := IndexWorkspace(root); err != nil {
		t.Fatalf("index workspace: %v", err)
	}

	files, err := ReadContext(root)
	if err != nil {
		t.Fatalf("read context: %v", err)
	}

	if len(files) < 2 {
		t.Fatalf("expected context files, got %d", len(files))
	}
	if files[0].Path != ".ptolemy/PTOLEMY.md" {
		t.Fatalf("expected PTOLEMY.md first, got %s", files[0].Path)
	}
}

func TestTaskSessionNotesAndFilesRead(t *testing.T) {
	root := t.TempDir()

	session, err := StartTaskSession(root, "booking-api-fix", "Fix booking API")
	if err != nil {
		t.Fatalf("start task session: %v", err)
	}

	if session.ID != "booking-api-fix" {
		t.Fatalf("unexpected session id: %s", session.ID)
	}

	if _, err := AppendSessionNote(root, session.ID, "Read router"); err != nil {
		t.Fatalf("append note: %v", err)
	}

	noteData, err := os.ReadFile(filepath.Join(root, ".ptolemy", "sessions", session.ID, "notes.md"))
	if err != nil {
		t.Fatalf("read notes: %v", err)
	}
	if !strings.Contains(string(noteData), "Read router") {
		t.Fatalf("expected note content, got %s", string(noteData))
	}

	if err := RecordFileRead(root, session.ID, "internal/httpapi/router.go"); err != nil {
		t.Fatalf("record file read: %v", err)
	}
	if err := RecordFileRead(root, session.ID, "internal/httpapi/router.go"); err != nil {
		t.Fatalf("record duplicate file read: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, ".ptolemy", "sessions", session.ID, "files-read.json"))
	if err != nil {
		t.Fatalf("read files-read: %v", err)
	}

	var files []string
	if err := json.Unmarshal(data, &files); err != nil {
		t.Fatalf("files-read should be valid JSON: %v", err)
	}
	if len(files) != 1 || files[0] != "internal/httpapi/router.go" {
		t.Fatalf("unexpected files-read content: %v", files)
	}
}

func writeTestFile(t *testing.T, root string, rel string, content string) {
	t.Helper()

	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func treeHasPath(tree FileTree, path string) bool {
	for _, file := range tree.Files {
		if file.Path == path {
			return true
		}
	}
	return false
}
