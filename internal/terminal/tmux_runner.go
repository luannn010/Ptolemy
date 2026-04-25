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

	runID := strings.ReplaceAll(uuid.NewString(), "-", "_")
	startMarker := "__PTOLEMY_START_" + runID + "__"
	exitMarker := "__PTOLEMY_EXIT_" + runID + "__"
	endMarker := "__PTOLEMY_END_" + runID + "__"

	if cwd == "" {
		cwd = "."
	}

	scriptPath := filepath.Join(os.TempDir(), "ptolemy-"+runID+".sh")

	// 🔥 FIXED SCRIPT (important part)
	script := fmt.Sprintf(`#!/usr/bin/env bash
set +e
set +o errexit

cd %q
cd_status=$?

echo %q

if [ "$cd_status" -ne 0 ]; then
  echo "failed to cd into workspace: %q"
  echo "%s:$cd_status"
  echo %q
  exit 0
fi

(
%s
)

exit_code=$?

echo "%s:$exit_code"
echo %q
`, cwd, startMarker, cwd, exitMarker, endMarker, command, exitMarker, endMarker)

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

	var output string

	for {
		output = r.capture(ctx, name)

		if strings.Contains(output, endMarker) {
			exitCode := extractMarkedExitCode(output, exitMarker)
			cleanOutput := extractMarkedOutput(output, startMarker, exitMarker)

			return Result{
				ExitCode:   exitCode,
				Output:     cleanOutput,
				DurationMS: time.Since(start).Milliseconds(),
			}
		}

		if time.Now().After(deadline) {
			return Result{
				ExitCode:    124,
				Output:      extractMarkedOutput(output, startMarker, exitMarker),
				ErrorOutput: fmt.Sprintf("command timed out after %d seconds", timeoutSeconds),
				DurationMS:  time.Since(start).Milliseconds(),
			}
		}

		time.Sleep(200 * time.Millisecond)
	}
}

func (r *TmuxRunner) capture(ctx context.Context, name string) string {
	cmd := exec.CommandContext(ctx, "tmux", "capture-pane", "-p", "-S", "-", "-t", name)
	out, _ := cmd.Output()
	return string(out)
}

func extractMarkedExitCode(output string, exitMarker string) int {
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, exitMarker+":") {
			var code int
			_, _ = fmt.Sscanf(line, exitMarker+":%d", &code)
			return code
		}
	}

	return 1
}

func extractMarkedOutput(output string, startMarker string, exitMarker string) string {
	lines := strings.Split(output, "\n")
	capturing := false
	cleaned := []string{}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == startMarker {
			capturing = true
			continue
		}

		if strings.HasPrefix(trimmed, exitMarker+":") {
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
