package inspect

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type Snapshot struct {
	Workspace     string   `json:"workspace"`
	OS            string   `json:"os"`
	Arch          string   `json:"arch"`
	IsWSL         bool     `json:"is_wsl"`
	GitBranch     string   `json:"git_branch"`
	GitStatus     string   `json:"git_status"`
	DetectedFiles []string `json:"detected_files"`
	ProjectTypes  []string `json:"project_types"`
}

func InspectWorkspace(workspace string) Snapshot {
	abs, err := filepath.Abs(workspace)
	if err != nil {
		abs = workspace
	}

	s := Snapshot{
		Workspace: abs,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		IsWSL:     detectWSL(),
	}

	s.GitBranch = runCommand(abs, "git", "branch", "--show-current")
	s.GitStatus = runCommand(abs, "git", "status", "--short")

	s.DetectedFiles = detectFiles(abs)
	s.ProjectTypes = detectProjectTypes(s.DetectedFiles)

	return s
}

func detectWSL() bool {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}

	content := strings.ToLower(string(data))
	return strings.Contains(content, "microsoft") || strings.Contains(content, "wsl")
}

func detectFiles(workspace string) []string {
	candidates := []string{
		"go.mod",
		"go.sum",
		"Makefile",
		"README.md",
		"package.json",
		"pnpm-lock.yaml",
		"yarn.lock",
		"package-lock.json",
		"requirements.txt",
		"pyproject.toml",
		"pom.xml",
		"build.gradle",
		"docker-compose.yml",
		"Dockerfile",
	}

	var found []string
	for _, file := range candidates {
		if _, err := os.Stat(filepath.Join(workspace, file)); err == nil {
			found = append(found, file)
		}
	}

	if _, err := os.Stat(filepath.Join(workspace, "internal")); err == nil {
		found = append(found, "internal/")
	}

	if _, err := os.Stat(filepath.Join(workspace, "cmd")); err == nil {
		found = append(found, "cmd/")
	}

	if _, err := os.Stat(filepath.Join(workspace, "docs")); err == nil {
		found = append(found, "docs/")
	}

	return found
}

func detectProjectTypes(files []string) []string {
	seen := map[string]bool{}

	for _, file := range files {
		switch file {
		case "go.mod":
			seen["go"] = true
		case "package.json":
			seen["node"] = true
		case "requirements.txt", "pyproject.toml":
			seen["python"] = true
		case "pom.xml", "build.gradle":
			seen["java"] = true
		case "docker-compose.yml", "Dockerfile":
			seen["docker"] = true
		}
	}

	var types []string
	for projectType := range seen {
		types = append(types, projectType)
	}

	return types
}

func runCommand(dir string, name string, args ...string) string {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(out))
}
