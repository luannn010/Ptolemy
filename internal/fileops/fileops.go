package fileops

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type FileOps struct {
	BaseDir string
}

type DirEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

func New(baseDir string) *FileOps {
	return &FileOps{
		BaseDir: filepath.Clean(baseDir),
	}
}

func (f *FileOps) Resolve(path string) (string, error) {
	if f.BaseDir == "" {
		return "", fmt.Errorf("base dir is required")
	}

	full := filepath.Join(f.BaseDir, path)
	clean := filepath.Clean(full)

	rel, err := filepath.Rel(f.BaseDir, clean)
	if err != nil {
		return "", err
	}

	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("path escapes base directory")
	}

	return clean, nil
}

func (f *FileOps) ReadFile(path string) (string, error) {
	full, err := f.Resolve(path)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func (f *FileOps) WriteFile(path string, content string) error {
	full, err := f.Resolve(path)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}

	return os.WriteFile(full, []byte(content), 0o644)
}

func (f *FileOps) ListDirectory(path string) ([]DirEntry, error) {
	full, err := f.Resolve(path)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(full)
	if err != nil {
		return nil, err
	}

	result := make([]DirEntry, 0, len(entries))

	for _, entry := range entries {
		relPath := filepath.Join(path, entry.Name())

		result = append(result, DirEntry{
			Name:  entry.Name(),
			Path:  relPath,
			IsDir: entry.IsDir(),
		})
	}

	return result, nil
}

func (f *FileOps) Search(query string) (string, error) {
	if query == "" {
		return "", fmt.Errorf("query is required")
	}

	cmd := exec.Command("rg", "--line-number", "--hidden", "--glob", "!.git", query, f.BaseDir)
	out, err := cmd.CombinedOutput()

	if err != nil {
		// ripgrep returns exit code 1 when no matches are found.
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return string(out), nil
		}
		return string(out), err
	}

	return string(out), nil
}

func (f *FileOps) ApplyPatch(path string, newContent string) error {
	return f.WriteFile(path, newContent)
}
