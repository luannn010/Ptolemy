package gitops

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type GitOps struct {
	RepoPath string
}

type Result struct {
	Command    string `json:"command"`
	RepoPath   string `json:"repo_path"`
	ExitCode   int    `json:"exit_code"`
	Output     string `json:"output"`
	DurationMS int64  `json:"duration_ms"`
	Success    bool   `json:"success"`
}

func New(repoPath string) *GitOps {
	return &GitOps{RepoPath: repoPath}
}

func (g *GitOps) Status(ctx context.Context) Result {
	return g.run(ctx, "git status --short")
}

func (g *GitOps) Diff(ctx context.Context) Result {
	return g.run(ctx, "git diff")
}

func (g *GitOps) Log(ctx context.Context) Result {
	return g.run(ctx, "git log --oneline -n 20")
}

func (g *GitOps) Checkout(ctx context.Context, branch string) Result {
	return g.run(ctx, fmt.Sprintf("git checkout %s", shellQuote(branch)))
}

func (g *GitOps) CreateBranch(ctx context.Context, branch string) Result {
	return g.run(ctx, fmt.Sprintf("git checkout -b %s", shellQuote(branch)))
}

func (g *GitOps) CommitConventional(ctx context.Context, message string) Result {
	if !isConventionalCommit(message) {
		return Result{
			Command:  "git commit",
			RepoPath: g.RepoPath,
			ExitCode: 1,
			Output:   "invalid conventional commit message",
			Success:  false,
		}
	}

	return g.run(ctx, fmt.Sprintf("git add . && git commit -m %s", shellQuote(message)))
}

func (g *GitOps) Push(ctx context.Context, remote string, branch string) Result {
	if remote == "" {
		remote = "origin"
	}

	return g.run(ctx, fmt.Sprintf("git push %s %s", shellQuote(remote), shellQuote(branch)))
}

func (g *GitOps) run(ctx context.Context, command string) Result {
	start := time.Now()

	runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "bash", "-lc", command)
	cmd.Dir = g.RepoPath

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()

	result := Result{
		Command:    command,
		RepoPath:   g.RepoPath,
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

func isConventionalCommit(message string) bool {
	pattern := `^(feat|fix|docs|style|refactor|test|chore|perf|ci|build|revert)(\([a-zA-Z0-9._-]+\))?: .+`
	ok, _ := regexp.MatchString(pattern, message)
	return ok
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
