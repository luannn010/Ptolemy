package terminal

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

type Result struct {
	ExitCode    int
	Output      string
	ErrorOutput string
	DurationMS  int64
}

type Runner struct{}

func NewRunner() *Runner {
	return &Runner{}
}

func (r *Runner) Run(ctx context.Context, command string, cwd string, timeoutSeconds int) Result {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}

	start := time.Now()

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "bash", "-lc", command)

	if cwd != "" {
		cmd.Dir = cwd
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	duration := time.Since(start).Milliseconds()

	result := Result{
		ExitCode:    0,
		Output:      stdout.String(),
		ErrorOutput: stderr.String(),
		DurationMS:  duration,
	}

	if runCtx.Err() == context.DeadlineExceeded {
		result.ExitCode = 124
		result.ErrorOutput += fmt.Sprintf("\ncommand timed out after %d seconds", timeoutSeconds)
		return result
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = 1
			result.ErrorOutput += err.Error()
		}
	}

	return result
}
