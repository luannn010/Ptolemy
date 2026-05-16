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

type InitContextOptions struct {
	IncludeTaskPack bool     `json:"include_task_pack"`
	Presets         []string `json:"presets"`
	IncludeGlobs    []string `json:"include_globs"`
	ExcludeGlobs    []string `json:"exclude_globs"`
}

type InitContextResult struct {
	Workspace      string   `json:"workspace"`
	ConfigPath     string   `json:"config_path"`
	GeneratedFiles []string `json:"generated_files"`
}

func InitContext(workspace string, opts InitContextOptions) (InitContextResult, error) {
	root, err := cleanWorkspace(workspace)
	if err != nil {
		return InitContextResult{}, err
	}
	if err := ensureLayout(root); err != nil {
		return InitContextResult{}, err
	}
	contextRoot := filepath.Join(root, ".ptolemy", "context")
	indexRoot := filepath.Join(contextRoot, "index")
	if err := os.MkdirAll(indexRoot, 0o755); err != nil {
		return InitContextResult{}, err
	}

	tree, err := BuildFileTree(root)
	if err != nil {
		return InitContextResult{}, err
	}
	files := selectFiles(tree, opts)
	mapIndex := buildMapIndex(tree)
	fileIndex := buildSimpleFileIndex(files)
	routes := buildRoutesIndex(files)
	api := buildAPIIndex(routes)

	if err := writeJSONFile(filepath.Join(indexRoot, "map-index.json"), mapIndex); err != nil {
		return InitContextResult{}, err
	}
	if err := writeJSONFile(filepath.Join(indexRoot, "file-index.json"), fileIndex); err != nil {
		return InitContextResult{}, err
	}
	if err := writeJSONFile(filepath.Join(indexRoot, "routes.json"), routes); err != nil {
		return InitContextResult{}, err
	}
	if err := writeJSONFile(filepath.Join(indexRoot, "api.json"), api); err != nil {
		return InitContextResult{}, err
	}

	writeMD := func(name, body string) error {
		return os.WriteFile(filepath.Join(indexRoot, name), []byte(body), 0o644)
	}
	_ = writeMD("map-index.md", markdownMapIndex(mapIndex))
	_ = writeMD("file-index.md", markdownFileIndex(fileIndex))
	_ = writeMD("routes.md", markdownRoutes(routes))
	_ = writeMD("api.md", markdownAPI(api))

	hub := buildFilesHub(opts)
	if err := os.WriteFile(filepath.Join(contextRoot, "files.md"), []byte(hub), 0o644); err != nil {
		return InitContextResult{}, err
	}

	if err := writeJSONFile(filepath.Join(contextRoot, "init-config.json"), opts); err != nil {
		return InitContextResult{}, err
	}
	if opts.IncludeTaskPack {
		_ = bootstrapTaskPackTemplate(root)
	}

	return InitContextResult{
		Workspace:  root,
		ConfigPath: ".ptolemy/context/init-config.json",
		GeneratedFiles: []string{
			".ptolemy/context/init-config.json",
			".ptolemy/context/files.md",
			".ptolemy/context/index/map-index.json",
			".ptolemy/context/index/map-index.md",
			".ptolemy/context/index/file-index.json",
			".ptolemy/context/index/file-index.md",
			".ptolemy/context/index/routes.json",
			".ptolemy/context/index/routes.md",
			".ptolemy/context/index/api.json",
			".ptolemy/context/index/api.md",
		},
	}, nil
}

func selectFiles(tree FileTree, opts InitContextOptions) []string {
	out := []string{}
	for _, entry := range tree.Files {
		if entry.IsDir {
			continue
		}
		if matchesScope(entry.Path, opts) {
			out = append(out, entry.Path)
		}
	}
	sort.Strings(out)
	return out
}
func matchesScope(path string, opts InitContextOptions) bool {
	for _, ex := range opts.ExcludeGlobs {
		if ok, _ := filepath.Match(ex, path); ok {
			return false
		}
	}
	if len(opts.IncludeGlobs) > 0 {
		for _, in := range opts.IncludeGlobs {
			if ok, _ := filepath.Match(in, path); ok {
				return true
			}
		}
		return false
	}
	if len(opts.Presets) == 0 {
		return true
	}
	for _, p := range opts.Presets {
		switch strings.ToLower(strings.TrimSpace(p)) {
		case "core":
			if strings.HasPrefix(path, "cmd/") || strings.HasPrefix(path, "internal/") {
				return true
			}
		case "api":
			if strings.Contains(path, "api") || strings.Contains(path, "httpapi") {
				return true
			}
		case "routes":
			if strings.Contains(path, "router") || strings.Contains(path, "route") {
				return true
			}
		case "docs":
			if strings.HasPrefix(path, "docs/") {
				return true
			}
		case "tasks":
			if strings.Contains(path, "task") {
				return true
			}
		}
	}
	return false
}

func buildMapIndex(tree FileTree) map[string]any {
	dirs := map[string]int{}
	for _, e := range tree.Files {
		if !e.IsDir {
			continue
		}
		top := strings.Split(strings.TrimSuffix(e.Path, "/"), "/")[0]
		dirs[top]++
	}
	return map[string]any{"generated_at": time.Now().UTC().Format(time.RFC3339), "top_level": dirs}
}
func buildSimpleFileIndex(files []string) []map[string]string {
	out := make([]map[string]string, 0, len(files))
	for _, f := range files {
		out = append(out, map[string]string{"path": f, "ext": filepath.Ext(f)})
	}
	return out
}
func buildRoutesIndex(files []string) []map[string]string {
	out := []map[string]string{}
	for _, f := range files {
		l := strings.ToLower(f)
		if strings.Contains(l, "router") || strings.Contains(l, "route") || strings.Contains(l, "httpapi") {
			out = append(out, map[string]string{"file": f, "hint": "route-related filename"})
		}
	}
	return out
}
func buildAPIIndex(routes []map[string]string) []map[string]string {
	out := []map[string]string{}
	for _, r := range routes {
		out = append(out, map[string]string{"file": r["file"], "signature": "unknown (needs parser support)"})
	}
	return out
}
func markdownMapIndex(v map[string]any) string    { b, _ := json.MarshalIndent(v, "", "  "); return "# Map Index\n\n```json\n" + string(b) + "\n```\n" }
func markdownFileIndex(v []map[string]string) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return "# File Index\n\n```json\n" + string(b) + "\n```\n"
}
func markdownRoutes(v []map[string]string) string { b, _ := json.MarshalIndent(v, "", "  "); return "# Routes\n\n```json\n" + string(b) + "\n```\n" }
func markdownAPI(v []map[string]string) string    { b, _ := json.MarshalIndent(v, "", "  "); return "# API\n\n```json\n" + string(b) + "\n```\n" }
func buildFilesHub(opts InitContextOptions) string {
	return "# Context Files\n\n" +
		"Selected presets: `" + strings.Join(opts.Presets, ",") + "`\n\n" +
		"Includes: `" + strings.Join(opts.IncludeGlobs, ",") + "`\n\n" +
		"Excludes: `" + strings.Join(opts.ExcludeGlobs, ",") + "`\n\n" +
		"- [map-index](./index/map-index.md)\n" +
		"- [file-index](./index/file-index.md)\n" +
		"- [routes](./index/routes.md)\n" +
		"- [api](./index/api.md)\n\n" +
		"Refresh: run `ptolemy init` again.\n"
}
func bootstrapTaskPackTemplate(root string) error {
	dateDir := time.Now().Format("02-01-2006")
	dest := filepath.Join(root, ".ptolemy", "tasks", "packs", dateDir, "starter-pack")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	readme := filepath.Join(dest, "README.md")
	if _, err := os.Stat(readme); err == nil {
		return nil
	}
	return os.WriteFile(readme, []byte("# Starter Task Pack\n"), 0o644)
}

