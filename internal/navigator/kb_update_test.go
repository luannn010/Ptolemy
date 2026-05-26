package navigator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func writeGoFile(t *testing.T, root, rel, body string) {
	t.Helper()
	writeTestFile(t, root, rel, body)
}

func TestUpdateKnowledgeBase_AddsSymbolsAndAppendsChangelog(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "go.mod", "module example.com/x\n")
	writeGoFile(t, root, "service.go", `package x

import "fmt"

type Server struct{ name string }

type Handler interface { Handle() }

func New(name string) *Server { return &Server{name: name} }

func (s *Server) Greet() string { return fmt.Sprintf("hi %s", s.name) }
`)

	if _, err := IndexWorkspace(root); err != nil {
		t.Fatalf("IndexWorkspace: %v", err)
	}

	res, err := UpdateKnowledgeBase(root, []string{"./service.go", "service.go"}, "pack-1", []string{"task-1"}, "deadbeef")
	if err != nil {
		t.Fatalf("UpdateKnowledgeBase: %v", err)
	}
	if len(res.UpdatedFiles) == 0 || res.UpdatedFiles[0] != "service.go" {
		t.Fatalf("expected service.go to be updated, got %+v", res.UpdatedFiles)
	}
	if res.PackID != "pack-1" || res.CommitSHA != "deadbeef" {
		t.Fatalf("metadata not preserved: %+v", res)
	}

	wantSyms := map[string]bool{"Server": false, "Handler": false, "New": false, "Greet": false}
	for _, sym := range res.UpdatedSymbols {
		if _, ok := wantSyms[sym]; ok {
			wantSyms[sym] = true
		}
	}
	missing := []string{}
	for s, found := range wantSyms {
		if !found {
			missing = append(missing, s)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("symbols missing from UpdateKnowledgeBase result: %v (got %+v)", missing, res.UpdatedSymbols)
	}

	// Symbol index file contains those symbols
	data, err := os.ReadFile(filepath.Join(root, ".ptolemy", "kb", "SYMBOL_INDEX.json"))
	if err != nil {
		t.Fatalf("read symbol index: %v", err)
	}
	var idx map[string]map[string]string
	if err := json.Unmarshal(data, &idx); err != nil {
		t.Fatalf("symbol index json: %v", err)
	}
	if _, ok := idx["Server"]; !ok {
		t.Fatalf("Server should be in symbol index: %v", idx)
	}

	// Daily changelog was appended somewhere under .ptolemy/kb/changelog/.
	changelogDir := filepath.Join(root, ".ptolemy", "kb", "changelog")
	entries, err := os.ReadDir(changelogDir)
	if err != nil || len(entries) == 0 {
		t.Fatalf("expected changelog files under %s, got err=%v entries=%d", changelogDir, err, len(entries))
	}
	logBytes, err := os.ReadFile(filepath.Join(changelogDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("read changelog: %v", err)
	}
	if !strings.Contains(string(logBytes), "pack-1") {
		t.Fatalf("expected pack-1 in changelog, got %q", string(logBytes))
	}
}

func TestUpdateKnowledgeBase_DeletedFileMovesToRemoved(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "go.mod", "module x\n")
	writeGoFile(t, root, "doomed.go", "package x\nfunc Doomed() {}\n")
	if _, err := IndexWorkspace(root); err != nil {
		t.Fatalf("IndexWorkspace: %v", err)
	}

	if err := os.Remove(filepath.Join(root, "doomed.go")); err != nil {
		t.Fatalf("remove: %v", err)
	}

	res, err := UpdateKnowledgeBase(root, []string{"doomed.go"}, "", nil, "")
	if err != nil {
		t.Fatalf("UpdateKnowledgeBase: %v", err)
	}
	if len(res.RemovedFiles) == 0 || res.RemovedFiles[0] != "doomed.go" {
		t.Fatalf("expected doomed.go in RemovedFiles, got %+v", res.RemovedFiles)
	}
}

func TestUpdateKnowledgeBase_EmptyChangedFallsBackToIndex(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "go.mod", "module x\n")
	if _, err := IndexWorkspace(root); err != nil {
		t.Fatalf("index: %v", err)
	}

	res, err := UpdateKnowledgeBase(root, nil, "", nil, "")
	if err != nil {
		t.Fatalf("UpdateKnowledgeBase: %v", err)
	}
	if len(res.UpdatedFiles) == 0 {
		t.Fatalf("expected UpdatedFiles to list KB-level files: %+v", res)
	}
}

func TestNormalizeChangedFiles_DedupesAndStrips(t *testing.T) {
	got := normalizeChangedFiles([]string{
		"a.go", "./a.go", "  a.go  ", ".git/HEAD", "", ".", "b.go",
	})
	wantSet := map[string]bool{"a.go": true, "b.go": true}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %v", got)
	}
	for _, g := range got {
		if !wantSet[g] {
			t.Fatalf("unexpected entry %q in %v", g, got)
		}
	}
}
