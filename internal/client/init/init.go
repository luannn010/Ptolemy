package clientinit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/luannn010/ptolemy/internal/bootstrap"
	clientconfig "github.com/luannn010/ptolemy/internal/client/config"
)

type Options struct {
	Workspace   string
	ServerURL   string
	ProjectName string
	Force       bool
}

type Result struct {
	Created []string
	Skipped []string
}

func Initialize(opts Options) (Result, error) {
	workspace := opts.Workspace
	if strings.TrimSpace(workspace) == "" {
		workspace = "."
	}

	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return Result{}, fmt.Errorf("resolve workspace: %w", err)
	}

	projectName := opts.ProjectName
	if strings.TrimSpace(projectName) == "" {
		projectName = filepath.Base(absWorkspace)
	}

	cfg := clientconfig.Default(projectName, opts.ServerURL)
	payload := bootstrap.Build(bootstrap.Request{
		ServerURL:   cfg.ServerURL,
		ProjectName: cfg.ProjectName,
	})

	result := Result{Created: []string{}, Skipped: []string{}}
	root := filepath.Join(absWorkspace, payload.RootPath)

	dirs := []string{
		filepath.Join(root, "context"),
		filepath.Join(root, "tasks"),
		filepath.Join(root, "memory"),
		filepath.Join(root, "cache"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return result, err
		}
	}

	for _, name := range payload.TaskFolders {
		if err := os.MkdirAll(filepath.Join(root, "tasks", name), 0o755); err != nil {
			return result, err
		}
	}
	for _, name := range payload.CacheFolders {
		if err := os.MkdirAll(filepath.Join(root, "cache", name), 0o755); err != nil {
			return result, err
		}
	}

	for _, name := range payload.ContextFiles {
		path := filepath.Join(root, "context", name)
		if err := writeManagedFile(path, defaultMarkdownContent(name), opts.Force, &result); err != nil {
			return result, err
		}
	}
	for _, name := range payload.MemoryFiles {
		path := filepath.Join(root, "memory", name)
		if err := writeManagedFile(path, defaultMarkdownContent(name), opts.Force, &result); err != nil {
			return result, err
		}
	}

	clientConfigPath := filepath.Join(root, payload.ClientConfigPath)
	if err := writeManagedFile(clientConfigPath, cfg.YAML(), opts.Force, &result); err != nil {
		return result, err
	}

	if err := ensureGitignoreEntry(filepath.Join(absWorkspace, ".gitignore"), ".ptolemy/"); err != nil {
		return result, err
	}
	result.Created = append(result.Created, ".gitignore")

	return result, nil
}

func writeManagedFile(path string, content string, force bool, result *Result) error {
	if existing, err := os.ReadFile(path); err == nil {
		if !force {
			result.Skipped = append(result.Skipped, path)
			return nil
		}
		if string(existing) == content {
			result.Skipped = append(result.Skipped, path)
			return nil
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err
	}
	result.Created = append(result.Created, path)
	return nil
}

func ensureGitignoreEntry(path string, entry string) error {
	content := ""
	if data, err := os.ReadFile(path); err == nil {
		content = string(data)
	} else if !os.IsNotExist(err) {
		return err
	}

	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == entry {
			return nil
		}
	}

	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += entry + "\n"
	return os.WriteFile(path, []byte(content), 0o644)
}

func defaultMarkdownContent(name string) string {
	title := strings.TrimSuffix(name, filepath.Ext(name))
	title = strings.ReplaceAll(title, "-", " ")
	title = strings.ReplaceAll(title, "_", " ")
	title = strings.Title(title)
	return "# " + title + "\n"
}
