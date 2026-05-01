package clientscan

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func Run(workspace string) error {
	if strings.TrimSpace(workspace) == "" {
		workspace = "."
	}
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return err
	}

	files, err := collectFiles(absWorkspace)
	if err != nil {
		return err
	}

	memoryDir := filepath.Join(absWorkspace, ".ptolemy", "memory")
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(memoryDir, "codebase-map.md"), []byte(renderCodebaseMap(files)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(memoryDir, "dependency-map.md"), []byte(renderDependencyMap(absWorkspace, files)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(memoryDir, "recent-changes.md"), []byte(renderRecentChanges(absWorkspace)), 0o644); err != nil {
		return err
	}

	return nil
}

func collectFiles(root string) ([]string, error) {
	out := []string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
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
		relSlash := filepath.ToSlash(rel)
		base := filepath.Base(path)

		if d.IsDir() {
			if base == ".git" || base == ".ptolemy" || base == "node_modules" || base == "vendor" || base == "dist" || base == "build" {
				return filepath.SkipDir
			}
			return nil
		}

		if isBinaryExt(filepath.Ext(base)) {
			return nil
		}

		out = append(out, relSlash)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

func isBinaryExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".png", ".jpg", ".jpeg", ".gif", ".pdf", ".zip", ".tar", ".gz", ".exe", ".dll", ".so", ".bin":
		return true
	default:
		return false
	}
}

func renderCodebaseMap(files []string) string {
	var b strings.Builder
	b.WriteString("# Codebase Map\n\n")
	if len(files) == 0 {
		b.WriteString("- no files found\n")
		return b.String()
	}
	for _, f := range files {
		b.WriteString("- `")
		b.WriteString(f)
		b.WriteString("`\n")
	}
	return b.String()
}

func renderDependencyMap(workspace string, files []string) string {
	var manifests []string
	for _, f := range files {
		base := filepath.Base(f)
		if base == "go.mod" || base == "package.json" || base == "pyproject.toml" || base == "requirements.txt" {
			manifests = append(manifests, f)
		}
	}

	var b strings.Builder
	b.WriteString("# Dependency Map\n\n")
	if len(manifests) == 0 {
		b.WriteString("- no supported dependency manifests found\n")
		return b.String()
	}
	for _, m := range manifests {
		b.WriteString("- `")
		b.WriteString(m)
		b.WriteString("`\n")
		abs := filepath.Join(workspace, filepath.FromSlash(m))
		content, err := os.ReadFile(abs)
		if err != nil {
			continue
		}
		preview := string(content)
		if len(preview) > 400 {
			preview = preview[:400]
		}
		b.WriteString("```text\n")
		b.WriteString(preview)
		if !strings.HasSuffix(preview, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("```\n")
	}
	return b.String()
}

func renderRecentChanges(workspace string) string {
	var b strings.Builder
	b.WriteString("# Recent Changes\n\n")

	status := runGit(workspace, "status", "--short")
	log := runGit(workspace, "log", "--oneline", "-n", "5")

	b.WriteString("## Git Status\n")
	if strings.TrimSpace(status) == "" {
		b.WriteString("- unavailable or clean\n")
	} else {
		b.WriteString("```text\n")
		b.WriteString(status)
		if !strings.HasSuffix(status, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("```\n")
	}

	b.WriteString("\n## Recent Commits\n")
	if strings.TrimSpace(log) == "" {
		b.WriteString("- unavailable\n")
	} else {
		b.WriteString("```text\n")
		b.WriteString(log)
		if !strings.HasSuffix(log, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("```\n")
	}

	return b.String()
}

func runGit(workspace string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = workspace
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Sprintf("git %s failed: %v", strings.Join(args, " "), err)
	}
	return out.String()
}
