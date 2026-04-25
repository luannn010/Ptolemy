package worktree

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Manager struct {
	RepoPath    string
	WorktreeDir string
}

type Result struct {
	Command    string `json:"command"`
	RepoPath   string `json:"repo_path"`
	Worktree   string `json:"worktree"`
	Branch     string `json:"branch"`
	ExitCode   int    `json:"exit_code"`
	Output     string `json:"output"`
	DurationMS int64  `json:"duration_ms"`
	Success    bool   `json:"success"`
}

func NewManager(repoPath string, worktreeDir string) *Manager {
	return &Manager{
		RepoPath:    filepath.Clean(repoPath),
		WorktreeDir: filepath.Clean(worktreeDir),
	}
}

func (m *Manager) Create(ctx context.Context, name string, branch string) Result {
	if name == "" {
		return fail("create_worktree", m.RepoPath, "", branch, "name is required")
	}

	if branch == "" {
		branch = "worktree/" + sanitize(name)
	}

	worktreePath := filepath.Join(m.WorktreeDir, sanitize(name))

	if err := os.MkdirAll(m.WorktreeDir, 0o755); err != nil {
		return fail("mkdir worktree dir", m.RepoPath, worktreePath, branch, err.Error())
	}

	cmd := fmt.Sprintf(
		"git worktree add -b %s %s",
		shellQuote(branch),
		shellQuote(worktreePath),
	)

	result := m.run(ctx, cmd)
	result.Worktree = worktreePath
	result.Branch = branch

	return result
}

func (m *Manager) Remove(ctx context.Context, name string) Result {
	if name == "" {
		return fail("remove_worktree", m.RepoPath, "", "", "name is required")
	}

	worktreePath := filepath.Join(m.WorktreeDir, sanitize(name))

	cmd := fmt.Sprintf("git worktree remove --force %s", shellQuote(worktreePath))

	result := m.run(ctx, cmd)
	result.Worktree = worktreePath

	return result
}

func (m *Manager) List(ctx context.Context) Result {
	return m.run(ctx, "git worktree list")
}

func (m *Manager) run(ctx context.Context, command string) Result {
	start := time.Now()

	runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "bash", "-lc", command)
	cmd.Dir = m.RepoPath

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()

	result := Result{
		Command:    command,
		RepoPath:   m.RepoPath,
		ExitCode:   0,
		Output:     out.String(),
		DurationMS: time.Since(start).Milliseconds(),
		Success:    true,
	}

	if runCtx.Err() == context.DeadlineExceeded {
		result.ExitCode = 124
		result.Output += "\ncommand timed out"
		result.Success = false
		return result
	}

	if err != nil {
		result.Success = false
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = 1
			result.Output += err.Error()
		}
	}

	return result
}

func fail(command, repoPath, worktreePath, branch, message string) Result {
	return Result{
		Command:  command,
		RepoPath: repoPath,
		Worktree: worktreePath,
		Branch:   branch,
		ExitCode: 1,
		Output:   message,
		Success:  false,
	}
}

func sanitize(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, " ", "-")
	value = strings.ReplaceAll(value, "/", "-")
	value = strings.ReplaceAll(value, "\\", "-")
	return value
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
