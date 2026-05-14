package navigator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type FileIndexEntry struct {
	Purpose        string   `json:"purpose"`
	LastSeenCommit string   `json:"last_seen_commit"`
	Tags           []string `json:"tags"`
}

type SymbolIndexEntry struct {
	File    string `json:"file"`
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
}

type KBUpdateResult struct {
	Workspace      string   `json:"workspace"`
	ChangedFiles   []string `json:"changed_files"`
	UpdatedFiles   []string `json:"updated_files"`
	RemovedFiles   []string `json:"removed_files"`
	UpdatedSymbols []string `json:"updated_symbols"`
	RemovedSymbols []string `json:"removed_symbols"`
	PackID         string   `json:"pack_id,omitempty"`
	CommitSHA      string   `json:"commit_sha,omitempty"`
}

type KBResetResult struct {
	Workspace    string   `json:"workspace"`
	RemovedFiles []string `json:"removed_files"`
	UpdatedFiles []string `json:"updated_files"`
}

func buildFileIndex(root string, tree FileTree) map[string]FileIndexEntry {
	index := map[string]FileIndexEntry{}
	for _, entry := range tree.Files {
		if entry.IsDir {
			continue
		}
		index[entry.Path] = FileIndexEntry{
			Purpose:        purposeForPath(entry.Path),
			LastSeenCommit: lastSeenCommit(root, entry.Path),
			Tags:           tagsForPath(entry.Path),
		}
	}
	return index
}

func buildSymbolIndex(root string, tree FileTree) map[string]SymbolIndexEntry {
	index := map[string]SymbolIndexEntry{}
	for _, entry := range tree.Files {
		if entry.IsDir || filepath.Ext(entry.Path) != ".go" || strings.HasSuffix(entry.Path, "_test.go") {
			continue
		}
		mergeSymbolEntries(index, readGoSymbols(root, entry.Path))
	}
	return index
}

func readGoSymbols(root string, rel string) map[string]SymbolIndexEntry {
	full := filepath.Join(root, filepath.FromSlash(rel))
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, full, nil, parser.SkipObjectResolution)
	if err != nil {
		return map[string]SymbolIndexEntry{}
	}

	out := map[string]SymbolIndexEntry{}
	for _, decl := range parsed.Decls {
		switch typed := decl.(type) {
		case *ast.FuncDecl:
			name := typed.Name.Name
			kind := "func"
			if typed.Recv != nil && len(typed.Recv.List) > 0 {
				kind = "method"
			}
			out[name] = SymbolIndexEntry{
				File:    rel,
				Kind:    kind,
				Summary: summarizeFunc(name, rel, typed),
			}
		case *ast.GenDecl:
			switch typed.Tok {
			case token.TYPE:
				for _, spec := range typed.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					out[typeSpec.Name.Name] = SymbolIndexEntry{
						File:    rel,
						Kind:    kindForType(typeSpec.Type),
						Summary: summarizeType(typeSpec.Name.Name, rel, typeSpec.Type),
					}
				}
			case token.CONST, token.VAR:
				kind := strings.ToLower(typed.Tok.String())
				for _, spec := range typed.Specs {
					valueSpec, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for _, name := range valueSpec.Names {
						out[name.Name] = SymbolIndexEntry{
							File:    rel,
							Kind:    kind,
							Summary: fmt.Sprintf("%s declared in %s", kind, rel),
						}
					}
				}
			}
		}
	}

	return out
}

func mergeSymbolEntries(target map[string]SymbolIndexEntry, source map[string]SymbolIndexEntry) {
	for key, value := range source {
		target[key] = value
	}
}

func removeSymbolsForFile(index map[string]SymbolIndexEntry, rel string) []string {
	removed := []string{}
	for key, value := range index {
		if value.File != rel {
			continue
		}
		delete(index, key)
		removed = append(removed, key)
	}
	sort.Strings(removed)
	return removed
}

func writeKnowledgeBase(root string, tree FileTree, fileIndex map[string]FileIndexEntry, symbolIndex map[string]SymbolIndexEntry) error {
	kbRoot := filepath.Join(root, ".ptolemy", "kb")

	projectMapContent := projectMap(root)
	if err := os.WriteFile(filepath.Join(kbRoot, "PROJECT_MAP.md"), []byte(projectMapContent), 0o644); err != nil {
		return err
	}

	if err := ensureSeedFile(filepath.Join(kbRoot, "WORKFLOWS.md"), seedWorkflows(root)); err != nil {
		return err
	}
	if err := ensureSeedFile(filepath.Join(kbRoot, "DECISIONS.md"), "# Decisions\n\nRecord durable architectural decisions here.\n"); err != nil {
		return err
	}
	if err := ensureSeedFile(filepath.Join(kbRoot, "CHANGELOG.md"), "# KB Changelog\n"); err != nil {
		return err
	}

	if err := writeJSONFile(filepath.Join(kbRoot, "FILE_INDEX.json"), fileIndex); err != nil {
		return err
	}
	if err := writeJSONFile(filepath.Join(kbRoot, "SYMBOL_INDEX.json"), symbolIndex); err != nil {
		return err
	}
	if err := writeJSONFile(filepath.Join(root, ".ptolemy", "index", "file-tree.json"), tree); err != nil {
		return err
	}

	return nil
}

func UpdateKnowledgeBase(workspace string, changedFiles []string, packID string, completedTaskIDs []string, commitSHA string) (KBUpdateResult, error) {
	root, err := cleanWorkspace(workspace)
	if err != nil {
		return KBUpdateResult{}, err
	}
	if err := ensureLayout(root); err != nil {
		return KBUpdateResult{}, err
	}

	normalized := normalizeChangedFiles(changedFiles)
	if len(normalized) == 0 {
		result, err := IndexWorkspace(root)
		if err != nil {
			return KBUpdateResult{}, err
		}
		return KBUpdateResult{
			Workspace:    result.Workspace,
			ChangedFiles: normalized,
			UpdatedFiles: []string{".ptolemy/kb/PROJECT_MAP.md", ".ptolemy/kb/FILE_INDEX.json", ".ptolemy/kb/SYMBOL_INDEX.json"},
			PackID:       packID,
			CommitSHA:    commitSHA,
		}, nil
	}

	tree, err := BuildFileTree(root)
	if err != nil {
		return KBUpdateResult{}, err
	}

	fileIndex, _ := readFileIndex(filepath.Join(root, ".ptolemy", "kb", "FILE_INDEX.json"))
	symbolIndex, _ := readSymbolIndex(filepath.Join(root, ".ptolemy", "kb", "SYMBOL_INDEX.json"))
	if len(fileIndex) == 0 {
		fileIndex = buildFileIndex(root, tree)
	}
	if len(symbolIndex) == 0 {
		symbolIndex = buildSymbolIndex(root, tree)
	}

	result := KBUpdateResult{
		Workspace:    root,
		ChangedFiles: normalized,
		UpdatedFiles: []string{},
		RemovedFiles: []string{},
		PackID:       packID,
		CommitSHA:    commitSHA,
	}

	for _, rel := range normalized {
		deleteRemoved := false
		full := filepath.Join(root, filepath.FromSlash(rel))
		info, statErr := os.Stat(full)
		if statErr != nil || info.IsDir() || shouldSkipFile(rel) {
			delete(fileIndex, rel)
			result.RemovedFiles = append(result.RemovedFiles, rel)
			result.RemovedSymbols = append(result.RemovedSymbols, removeSymbolsForFile(symbolIndex, rel)...)
			deleteRemoved = true
		}
		if deleteRemoved {
			continue
		}

		fileIndex[rel] = FileIndexEntry{
			Purpose:        purposeForPath(rel),
			LastSeenCommit: lastSeenCommit(root, rel),
			Tags:           tagsForPath(rel),
		}
		result.UpdatedFiles = append(result.UpdatedFiles, rel)

		result.RemovedSymbols = append(result.RemovedSymbols, removeSymbolsForFile(symbolIndex, rel)...)
		if filepath.Ext(rel) == ".go" && !strings.HasSuffix(rel, "_test.go") {
			symbols := readGoSymbols(root, rel)
			mergeSymbolEntries(symbolIndex, symbols)
			for name := range symbols {
				result.UpdatedSymbols = append(result.UpdatedSymbols, name)
			}
		}
	}

	if err := writeKnowledgeBase(root, tree, fileIndex, symbolIndex); err != nil {
		return KBUpdateResult{}, err
	}
	if packID != "" {
		if err := appendKBChangelog(root, packID, completedTaskIDs, normalized, commitSHA); err != nil {
			return KBUpdateResult{}, err
		}
	}

	sort.Strings(result.UpdatedFiles)
	sort.Strings(result.RemovedFiles)
	sort.Strings(result.UpdatedSymbols)
	sort.Strings(result.RemovedSymbols)

	return result, nil
}

func normalizeChangedFiles(paths []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, raw := range paths {
		cleaned := filepath.ToSlash(filepath.Clean(strings.TrimSpace(raw)))
		if cleaned == "." || cleaned == "" || strings.HasPrefix(cleaned, ".git/") {
			continue
		}
		if strings.HasPrefix(cleaned, "./") {
			cleaned = strings.TrimPrefix(cleaned, "./")
		}
		if seen[cleaned] {
			continue
		}
		seen[cleaned] = true
		out = append(out, cleaned)
	}
	sort.Strings(out)
	return out
}

func readFileIndex(path string) (map[string]FileIndexEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]FileIndexEntry{}, err
	}
	index := map[string]FileIndexEntry{}
	if err := json.Unmarshal(data, &index); err != nil {
		return map[string]FileIndexEntry{}, err
	}
	return index, nil
}

func readSymbolIndex(path string) (map[string]SymbolIndexEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]SymbolIndexEntry{}, err
	}
	index := map[string]SymbolIndexEntry{}
	if err := json.Unmarshal(data, &index); err != nil {
		return map[string]SymbolIndexEntry{}, err
	}
	return index, nil
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func ensureSeedFile(path string, content string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func ResetKnowledgeBase(workspace string) (KBResetResult, error) {
	root, err := cleanWorkspace(workspace)
	if err != nil {
		return KBResetResult{}, err
	}
	if err := ensureLayout(root); err != nil {
		return KBResetResult{}, err
	}

	result := KBResetResult{Workspace: root, RemovedFiles: []string{}, UpdatedFiles: []string{}}
	indexResult, err := IndexWorkspace(root)
	if err != nil {
		return KBResetResult{}, err
	}
	result.UpdatedFiles = append(result.UpdatedFiles, indexResult.ContextFiles...)
	result.UpdatedFiles = append(result.UpdatedFiles, indexResult.IndexFiles...)
	sort.Strings(result.RemovedFiles)
	sort.Strings(result.UpdatedFiles)

	if err := appendKBDailyLog(root, "kb_reset", "", nil, nil, "rebuilt canonical KB files in place"); err != nil {
		return KBResetResult{}, err
	}
	return result, nil
}

func appendKBChangelog(root string, packID string, completedTaskIDs []string, changedFiles []string, commitSHA string) error {
	return appendKBDailyLog(root, "kb_update", packID, completedTaskIDs, changedFiles, commitSHA)
}

func appendKBDailyLog(root string, operation string, packID string, completedTaskIDs []string, changedFiles []string, commitSHA string) error {
	dayStamp := time.Now().UTC().Format("020106")
	changelogPath := filepath.Join(root, ".ptolemy", "kb", "changelog", "CHANGELOG-"+dayStamp+".md")
	var builder strings.Builder

	builder.WriteString("\n## ")
	builder.WriteString(time.Now().UTC().Format("2006-01-02 15:04:05"))
	builder.WriteString("\n- Operation: ")
	builder.WriteString(operation)
	builder.WriteString("\n")
	if strings.TrimSpace(packID) != "" {
		builder.WriteString("- Pack: ")
		builder.WriteString(packID)
		builder.WriteString("\n")
	}
	if strings.TrimSpace(commitSHA) != "" {
		builder.WriteString("- Commit: `")
		builder.WriteString(commitSHA)
		builder.WriteString("`\n")
	}
	if len(completedTaskIDs) > 0 {
		builder.WriteString("- Completed tasks: ")
		builder.WriteString(strings.Join(completedTaskIDs, ", "))
		builder.WriteString("\n")
	}
	if len(changedFiles) > 0 {
		builder.WriteString("- Affected files:\n")
		for _, file := range changedFiles {
			builder.WriteString("  - ")
			builder.WriteString(file)
			builder.WriteString("\n")
		}
	} else {
		builder.WriteString("- Affected files: none listed\n")
	}

	if err := os.MkdirAll(filepath.Dir(changelogPath), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(changelogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(builder.String())
	return err
}

func lastSeenCommit(root string, rel string) string {
	cmd := exec.Command("git", "log", "-n", "1", "--format=%H", "--", rel)
	cmd.Dir = root
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(stdout.String())
}

func purposeForPath(rel string) string {
	switch {
	case strings.HasPrefix(rel, "cmd/") && filepath.Base(rel) == "main.go":
		return fmt.Sprintf("Entrypoint for %s", strings.TrimSuffix(strings.TrimPrefix(filepath.Dir(rel), "cmd/"), "/"))
	case strings.HasPrefix(rel, "internal/"):
		parts := strings.Split(rel, "/")
		if len(parts) >= 2 {
			return fmt.Sprintf("Implementation for the %s package", parts[1])
		}
	case strings.HasPrefix(rel, "docs/workflows/"):
		return "Workflow documentation"
	case strings.HasPrefix(rel, "docs/tasks/"):
		return "Task planning or execution document"
	case strings.HasSuffix(rel, "_test.go"):
		return "Go tests for package behavior"
	case strings.HasSuffix(rel, ".go"):
		return "Go source file"
	case strings.HasSuffix(rel, ".md"):
		return "Project documentation"
	}
	return "Project file"
}

func tagsForPath(rel string) []string {
	tags := []string{}
	add := func(value string) {
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			return
		}
		for _, existing := range tags {
			if existing == value {
				return
			}
		}
		tags = append(tags, value)
	}

	ext := strings.TrimPrefix(filepath.Ext(rel), ".")
	if ext != "" {
		add(ext)
	}
	for _, part := range strings.Split(rel, "/") {
		part = strings.TrimSuffix(part, filepath.Ext(part))
		part = strings.ReplaceAll(part, "_", "-")
		part = strings.ReplaceAll(part, ".", "-")
		if part == "" {
			continue
		}
		add(part)
	}
	if strings.HasSuffix(rel, "_test.go") {
		add("test")
	}
	sort.Strings(tags)
	return tags
}

func kindForType(node ast.Expr) string {
	switch node.(type) {
	case *ast.StructType:
		return "struct"
	case *ast.InterfaceType:
		return "interface"
	default:
		return "type"
	}
}

func summarizeType(name string, rel string, node ast.Expr) string {
	switch node.(type) {
	case *ast.StructType:
		return fmt.Sprintf("Struct declared in %s", rel)
	case *ast.InterfaceType:
		return fmt.Sprintf("Interface declared in %s", rel)
	default:
		return fmt.Sprintf("Type declared in %s", rel)
	}
}

func summarizeFunc(name string, rel string, decl *ast.FuncDecl) string {
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		return fmt.Sprintf("Method %s implemented in %s", name, rel)
	}
	return fmt.Sprintf("Function %s implemented in %s", name, rel)
}

func seedWorkflows(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "WORKFLOWS.md"))
	if err != nil {
		return "# Workflows\n\nMirror or summarize repo workflow guidance here when needed.\n"
	}
	return string(data)
}
