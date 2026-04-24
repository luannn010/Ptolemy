package terminal

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

type TmuxRunner struct{}

func NewTmuxRunner() *TmuxRunner {
	return &TmuxRunner{}
}

func tmuxSessionName(sessionID string) string {
	clean := strings.ReplaceAll(sessionID, "-", "_")
	return "ptolemy_" + clean
}

func (r *TmuxRunner) EnsureSession(ctx context.Context, sessionID string, cwd string) error {
	name := tmuxSessionName(sessionID)

	check := exec.CommandContext(ctx, "tmux", "has-session", "-t", name)
	if err := check.Run(); err == nil {
		return nil
	}

	args := []string{"new-session", "-d", "-s", name}
	if cwd != "" {
		args = append(args, "-c", cwd)
	}

	cmd := exec.CommandContext(ctx, "tmux", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create tmux session: %w: %s", err, string(out))
	}

	return nil
}

func (r *TmuxRunner) Run(ctx context.Context, sessionID string, command string, cwd string, timeoutSeconds int) Result {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}

	start := time.Now()
	name := tmuxSessionName(sessionID)

	if err := r.EnsureSession(ctx, sessionID, cwd); err != nil {
		return Result{
			ExitCode:    1,
			ErrorOutput: err.Error(),
			DurationMS:  time.Since(start).Milliseconds(),
		}
	}

	runID := uuid.NewString()
	startMarker := "__PTOLEMY_START_" + runID + "__"
	endMarker := "__PTOLEMY_END_" + runID + "__"

	scriptPath := filepath.Join(os.TempDir(), "ptolemy-"+runID+".sh")

	script := fmt.Sprintf(`#!/usr/bin/env bash
	cd %q || exit 1
	echo %q
	(
	%s
	)
	exit_code=$?
	echo "%s:${exit_code}"
	`, cwd, startMarker, command, endMarker)

	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		return Result{
			ExitCode:    1,
			ErrorOutput: fmt.Sprintf("write temp script: %v", err),
			DurationMS:  time.Since(start).Milliseconds(),
		}
	}

	_ = exec.CommandContext(ctx, "tmux", "clear-history", "-t", name).Run()

	sendLine := fmt.Sprintf("bash %q; rm -f %q", scriptPath, scriptPath)

	sendCmd := exec.CommandContext(ctx, "tmux", "send-keys", "-t", name, sendLine, "C-m")
	if out, err := sendCmd.CombinedOutput(); err != nil {
		return Result{
			ExitCode:    1,
			ErrorOutput: fmt.Sprintf("send tmux command: %v: %s", err, string(out)),
			DurationMS:  time.Since(start).Milliseconds(),
		}
	}

	deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)

	for {
		output := r.capture(ctx, name)

		if strings.Contains(output, endMarker+":") {
			exitCode := extractMarkedExitCode(output, endMarker)
			cleanOutput := extractMarkedOutput(output, startMarker, endMarker)

			return Result{
				ExitCode:   exitCode,
				Output:     cleanOutput,
				DurationMS: time.Since(start).Milliseconds(),
			}
		}

		if time.Now().After(deadline) {
			return Result{
				ExitCode:    124,
				Output:      extractMarkedOutput(output, startMarker, endMarker),
				ErrorOutput: fmt.Sprintf("command timed out after %d seconds", timeoutSeconds),
				DurationMS:  time.Since(start).Milliseconds(),
			}
		}

		time.Sleep(200 * time.Millisecond)
	}
}

func (r *TmuxRunner) capture(ctx context.Context, name string) string {
	cmd := exec.CommandContext(ctx, "tmux", "capture-pane", "-p", "-t", name)
	out, _ := cmd.Output()
	return string(out)
}

func extractMarkedExitCode(output string, endMarker string) int {
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, endMarker+":") {
			var code int
			_, _ = fmt.Sscanf(line, endMarker+":%d", &code)
			return code
		}
	}

	return 1
}

func extractMarkedOutput(output string, startMarker string, endMarker string) string {
	lines := strings.Split(output, "\n")
	capturing := false
	cleaned := []string{}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == startMarker {
			capturing = true
			continue
		}

		if strings.HasPrefix(trimmed, endMarker+":") {
			break
		}

		if capturing {
			cleaned = append(cleaned, line)
		}
	}

	return strings.TrimRight(strings.Join(cleaned, "\n"), "\n") + "\n"
}
func KillSession(sessionID string) {
	name := tmuxSessionName(sessionID)
	_ = exec.Command("tmux", "kill-session", "-t", name).Run()
}
