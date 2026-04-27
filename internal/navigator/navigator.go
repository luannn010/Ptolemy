package navigator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var ignoredDirs = map[string]bool{
	".git":         true,
	".next":        true,
	"__pycache__":  true,
	"build":        true,
	"coverage":     true,
	"dist":         true,
	"node_modules": true,
	"vendor":       true,
}

type FileEntry struct {
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size,omitempty"`
}

type FileTree struct {
	Workspace   string      `json:"workspace"`
	GeneratedAt time.Time   `json:"generated_at"`
	Files       []FileEntry `json:"files"`
}

type IndexResult struct {
	Workspace    string   `json:"workspace"`
	FileCount    int      `json:"file_count"`
	ContextFiles []string `json:"context_files"`
	IndexFiles   []string `json:"index_files"`
}

type ContextFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type TaskSession struct {
	ID    string   `json:"id"`
	Path  string   `json:"path"`
	Files []string `json:"files"`
}

func IndexWorkspace(workspace string) (IndexResult, error) {
	root, err := cleanWorkspace(workspace)
	if err != nil {
		return IndexResult{}, err
	}

	if err := ensureLayout(root); err != nil {
		return IndexResult{}, err
	}

	contextFiles, err := ensureContextFiles(root)
	if err != nil {
		return IndexResult{}, err
	}

	tree, err := BuildFileTree(root)
	if err != nil {
		return IndexResult{}, err
	}

	indexPath := filepath.Join(root, ".ptolemy", "index", "file-tree.json")
	data, err := json.MarshalIndent(tree, "", "  ")
	if err != nil {
		return IndexResult{}, err
	}
	data = append(data, '\n')

	if err := os.WriteFile(indexPath, data, 0o644); err != nil {
		return IndexResult{}, err
	}

	return IndexResult{
		Workspace:    root,
		FileCount:    len(tree.Files),
		ContextFiles: contextFiles,
		IndexFiles:   []string{".ptolemy/index/file-tree.json"},
	}, nil
}

func BuildFileTree(workspace string) (FileTree, error) {
	root, err := cleanWorkspace(workspace)
	if err != nil {
		return FileTree{}, err
	}

	tree := FileTree{
		Workspace:   root,
		GeneratedAt: time.Now().UTC(),
	}

	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		if d.IsDir() {
			if shouldSkipDir(rel, d.Name()) {
				return filepath.SkipDir
			}
			tree.Files = append(tree.Files, FileEntry{Path: rel + "/", IsDir: true})
			return nil
		}

		if shouldSkipFile(rel) {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		tree.Files = append(tree.Files, FileEntry{
			Path: rel,
			Size: info.Size(),
		})
		return nil
	})
	if err != nil {
		return FileTree{}, err
	}

	sort.Slice(tree.Files, func(i, j int) bool {
		return tree.Files[i].Path < tree.Files[j].Path
	})

	return tree, nil
}

func ReadContext(workspace string) ([]ContextFile, error) {
	root, err := cleanWorkspace(workspace)
	if err != nil {
		return nil, err
	}

	paths := []string{".ptolemy/PTOLEMY.md"}
	contextRoot := filepath.Join(root, ".ptolemy", "context")

	entries, err := os.ReadDir(contextRoot)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		paths = append(paths, filepath.ToSlash(filepath.Join(".ptolemy", "context", entry.Name())))
	}

	sort.Strings(paths)
	files := make([]ContextFile, 0, len(paths))
	for _, rel := range paths {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		files = append(files, ContextFile{Path: rel, Content: string(data)})
	}

	return files, nil
}

func StartTaskSession(workspace string, sessionID string, task string) (TaskSession, error) {
	root, err := cleanWorkspace(workspace)
	if err != nil {
		return TaskSession{}, err
	}
	if strings.TrimSpace(sessionID) == "" {
		sessionID = "session-" + time.Now().UTC().Format("20060102T150405Z")
	}
	sessionID = safeSessionID(sessionID)
	if sessionID == "" {
		return TaskSession{}, fmt.Errorf("session_id contains no safe characters")
	}

	sessionRoot := filepath.Join(root, ".ptolemy", "sessions", sessionID)
	if err := os.MkdirAll(sessionRoot, 0o755); err != nil {
		return TaskSession{}, err
	}

	files := map[string]string{
		"task.md":         strings.TrimSpace(task) + "\n",
		"notes.md":        "# Notes\n",
		"files-read.json": "[]\n",
		"changes.md":      "# Changes\n",
		"test-results.md": "# Test Results\n",
	}

	created := make([]string, 0, len(files))
	for name, content := range files {
		rel := filepath.ToSlash(filepath.Join(".ptolemy", "sessions", sessionID, name))
		full := filepath.Join(sessionRoot, name)
		if _, err := os.Stat(full); err == nil {
			created = append(created, rel)
			continue
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			return TaskSession{}, err
		}
		created = append(created, rel)
	}
	sort.Strings(created)

	return TaskSession{
		ID:    sessionID,
		Path:  filepath.ToSlash(filepath.Join(".ptolemy", "sessions", sessionID)),
		Files: created,
	}, nil
}

func AppendSessionNote(workspace string, sessionID string, note string) (TaskSession, error) {
	root, err := cleanWorkspace(workspace)
	if err != nil {
		return TaskSession{}, err
	}
	sessionID = safeSessionID(sessionID)
	if sessionID == "" {
		return TaskSession{}, fmt.Errorf("session_id is required")
	}
	if strings.TrimSpace(note) == "" {
		return TaskSession{}, fmt.Errorf("note is required")
	}

	sessionRoot := filepath.Join(root, ".ptolemy", "sessions", sessionID)
	if err := os.MkdirAll(sessionRoot, 0o755); err != nil {
		return TaskSession{}, err
	}

	notePath := filepath.Join(sessionRoot, "notes.md")
	entry := fmt.Sprintf("\n## %s\n\n%s\n", time.Now().UTC().Format(time.RFC3339), strings.TrimSpace(note))
	if err := appendFile(notePath, entry); err != nil {
		return TaskSession{}, err
	}

	return TaskSession{
		ID:   sessionID,
		Path: filepath.ToSlash(filepath.Join(".ptolemy", "sessions", sessionID)),
		Files: []string{
			filepath.ToSlash(filepath.Join(".ptolemy", "sessions", sessionID, "notes.md")),
		},
	}, nil
}

func RecordFileRead(workspace string, sessionID string, path string) error {
	root, err := cleanWorkspace(workspace)
	if err != nil {
		return err
	}
	sessionID = safeSessionID(sessionID)
	if sessionID == "" || strings.TrimSpace(path) == "" {
		return nil
	}

	sessionRoot := filepath.Join(root, ".ptolemy", "sessions", sessionID)
	if err := os.MkdirAll(sessionRoot, 0o755); err != nil {
		return err
	}

	target := filepath.Join(sessionRoot, "files-read.json")
	var files []string
	if data, err := os.ReadFile(target); err == nil {
		_ = json.Unmarshal(data, &files)
	}

	cleaned := filepath.ToSlash(filepath.Clean(path))
	for _, existing := range files {
		if existing == cleaned {
			return nil
		}
	}
	files = append(files, cleaned)
	sort.Strings(files)

	data, err := json.MarshalIndent(files, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(target, data, 0o644)
}

func IgnoredDirs() []string {
	items := make([]string, 0, len(ignoredDirs))
	for item := range ignoredDirs {
		items = append(items, item)
	}
	sort.Strings(items)
	return items
}

func ensureLayout(root string) error {
	dirs := []string{
		filepath.Join(root, ".ptolemy"),
		filepath.Join(root, ".ptolemy", "context"),
		filepath.Join(root, ".ptolemy", "index"),
		filepath.Join(root, ".ptolemy", "sessions"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func ensureContextFiles(root string) ([]string, error) {
	files := map[string]string{
		".ptolemy/PTOLEMY.md":              ptolemyGuide(),
		".ptolemy/context/project-map.md":  projectMap(root),
		".ptolemy/context/commands.md":     commands(root),
		".ptolemy/context/architecture.md": architecture(root),
		".ptolemy/context/env.md":          envNotes(root),
		".ptolemy/context/conventions.md":  conventions(),
	}

	created := make([]string, 0, len(files))
	for rel, content := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if _, err := os.Stat(full); err == nil {
			created = append(created, rel)
			continue
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			return nil, err
		}
		created = append(created, rel)
	}
	sort.Strings(created)
	return created, nil
}

func ptolemyGuide() string {
	return "# Ptolemy\n\n" +
		"Ptolemy is a codebase navigator, not a whole-codebase reader.\n\n" +
		"Golden rule:\n\n" +
		"```text\n" +
		"Search first.\n" +
		"Read small.\n" +
		"Edit targeted.\n" +
		"Test immediately.\n" +
		"Summarise changes.\n" +
		"Update memory only after confirmed change.\n" +
		"```\n\n" +
		"Default workflow:\n\n" +
		"1. Read this file.\n" +
		"2. Read `.ptolemy/context/project-map.md`.\n" +
		"3. Search by keyword or symbol.\n" +
		"4. Read only top relevant files.\n" +
		"5. Make small changes.\n" +
		"6. Run targeted tests.\n" +
		"7. Save session notes.\n"
}

func projectMap(root string) string {
	var parts []string
	for _, dir := range []string{"cmd", "internal", "docs", "configs", "deploy"} {
		if isDir(filepath.Join(root, dir)) {
			parts = append(parts, "- `"+dir+"/`")
		}
	}
	if len(parts) == 0 {
		parts = append(parts, "- Project structure has not been summarized yet.")
	}

	return "# Project Map\n\nUseful top-level areas:\n\n" + strings.Join(parts, "\n") + "\n"
}

func commands(root string) string {
	lines := []string{"# Commands", ""}
	if exists(filepath.Join(root, "Makefile")) {
		lines = append(lines, "- `make test` - run the project test suite if the Makefile target is available.")
		lines = append(lines, "- `make build` - build the worker binary if the Makefile target is available.")
	}
	if exists(filepath.Join(root, "go.mod")) {
		lines = append(lines, "- `/usr/local/go/bin/go test ./...` - run all Go tests in WSL.")
		LinesGoRun := "- `/usr/local/go/bin/go run ./cmd/workerd` - start the worker daemon locally."
		lines = append(lines, LinesGoRun)
	}
	if exists(filepath.Join(root, "package.json")) {
		lines = append(lines, "- Use the package manager lockfile to choose the Node command before installing or running scripts.")
	}
	if len(lines) == 2 {
		lines = append(lines, "- Add project-specific build, test, and run commands after discovery.")
	}
	return strings.Join(lines, "\n") + "\n"
}

func architecture(root string) string {
	lines := []string{"# Architecture", ""}
	if exists(filepath.Join(root, "go.mod")) {
		lines = append(lines, "- Go project.")
	}
	if isDir(filepath.Join(root, "cmd")) {
		lines = append(lines, "- `cmd/` contains executable entrypoints.")
	}
	if isDir(filepath.Join(root, "internal")) {
		lines = append(lines, "- `internal/` contains application packages.")
	}
	lines = append(lines, "- Keep this file high-level. Do not paste whole source files here.")
	return strings.Join(lines, "\n") + "\n"
}

func envNotes(root string) string {
	lines := []string{"# Environment", ""}
	if exists(filepath.Join(root, ".env.example")) {
		lines = append(lines, "- `.env.example` exists. Use it as the safe reference for required variables.")
	}
	if exists(filepath.Join(root, ".env")) {
		lines = append(lines, "- `.env` exists locally. Do not copy secrets into context files.")
	}
	lines = append(lines, "- Heavy/generated folders are skipped during indexing.")
	return strings.Join(lines, "\n") + "\n"
}

func conventions() string {
	return "# Conventions\n\n" +
		"- Search before reading full files.\n" +
		"- Use Level 3 full-file reads only when editing or debugging that file.\n" +
		"- Prefer small, reversible changes.\n" +
		"- Run targeted tests immediately after edits.\n" +
		"- Keep `.ptolemy/context` concise and reusable.\n"
}

func shouldSkipDir(rel string, name string) bool {
	if ignoredDirs[name] {
		return true
	}
	return rel == ".ptolemy/index" || rel == ".ptolemy/sessions"
}

func shouldSkipFile(rel string) bool {
	return rel == ".ptolemy/index/file-tree.json"
}

func cleanWorkspace(workspace string) (string, error) {
	if strings.TrimSpace(workspace) == "" {
		workspace = "."
	}
	root, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace is not a directory: %s", workspace)
	}
	return filepath.Clean(root), nil
}

func safeSessionID(id string) string {
	id = strings.TrimSpace(id)
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		}
	}
	return b.String()
}

func appendFile(path string, content string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(content)
	return err
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
