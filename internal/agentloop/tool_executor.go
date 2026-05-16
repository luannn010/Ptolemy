package agentloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/luannn010/ptolemy/internal/action"
	"github.com/luannn010/ptolemy/internal/approval"
	"github.com/luannn010/ptolemy/internal/fileops"
	"github.com/luannn010/ptolemy/internal/policy"
	"github.com/luannn010/ptolemy/internal/session"
	"github.com/luannn010/ptolemy/internal/terminal"
)

var (
	ErrEmptyCommand       = fmt.Errorf("command is required")
	ErrEmptyPath          = fmt.Errorf("path is required")
	ErrEmptyMarker        = fmt.Errorf("marker is required")
	ErrEmptyOldBlock      = fmt.Errorf("old block is required")
	ErrEmptyContent       = fmt.Errorf("content is required")
	ErrUnsafeWorkspace    = fmt.Errorf("workspace is required")
	ErrPathNotInTaskScope = fmt.Errorf("path is outside allowed_files")
)

type ToolExecutor struct {
	sessionStore  *session.Store
	approvalStore *approval.Store
	runner        *terminal.TmuxRunner
	artifactsDir  string
}

func NewToolExecutor(sessionStore *session.Store, approvalStore *approval.Store, runner *terminal.TmuxRunner, artifactsDir string) *ToolExecutor {
	return &ToolExecutor{
		sessionStore:  sessionStore,
		approvalStore: approvalStore,
		runner:        runner,
		artifactsDir:  artifactsDir,
	}
}

func (e *ToolExecutor) RegisterAll(registry *ToolRegistry) {
	registry.Register("read_file", e.readFile)
	registry.Register("write_file", e.writeFile)
	registry.Register("replace_block", e.replaceBlock)
	registry.Register("insert_after", e.insertAfter)
	registry.Register("run_command", e.runCommand)
	registry.Register("ask_approval", e.askApproval)
	registry.Register("explain", e.explain)
}

func (e *ToolExecutor) readFile(ctx context.Context, run Run, task ActionTask, envelope *action.ActionEnvelope) (ExecutionResult, error) {
	ops, cleanPath, err := e.opsForPath(task, envelope.Path)
	if err != nil {
		return ExecutionResult{}, err
	}
	content, err := ops.ReadFile(cleanPath)
	if err != nil {
		return ExecutionResult{}, err
	}
	artifactPath := e.saveArtifact(run.ID, run.CurrentStep, "read-file", content)
	return ExecutionResult{Status: "success", Output: content, Summary: fmt.Sprintf("read file %s", cleanPath), ArtifactPath: artifactPath, Progressed: true, ShouldContinue: true}, nil
}

func (e *ToolExecutor) writeFile(ctx context.Context, run Run, task ActionTask, envelope *action.ActionEnvelope) (ExecutionResult, error) {
	ops, cleanPath, err := e.opsForPath(task, envelope.Path)
	if err != nil {
		return ExecutionResult{}, err
	}
	if strings.TrimSpace(envelope.Content) == "" {
		return ExecutionResult{}, ErrEmptyContent
	}
	if err := ops.WriteFile(cleanPath, envelope.Content); err != nil {
		return ExecutionResult{}, err
	}
	artifactPath := e.saveArtifact(run.ID, run.CurrentStep, "write-file", envelope.Content)
	return ExecutionResult{Status: "success", Output: envelope.Content, Summary: fmt.Sprintf("wrote file %s", cleanPath), ArtifactPath: artifactPath, Progressed: true, ShouldContinue: true}, nil
}

func (e *ToolExecutor) replaceBlock(ctx context.Context, run Run, task ActionTask, envelope *action.ActionEnvelope) (ExecutionResult, error) {
	ops, cleanPath, err := e.opsForPath(task, envelope.Path)
	if err != nil {
		return ExecutionResult{}, err
	}
	if strings.TrimSpace(envelope.Old) == "" {
		return ExecutionResult{}, ErrEmptyOldBlock
	}
	if err := ops.ReplaceBlock(cleanPath, envelope.Old, envelope.New); err != nil {
		if errors.Is(err, fileops.ErrOldBlockNotFound) {
			content, readErr := ops.ReadFile(cleanPath)
			if readErr != nil {
				return ExecutionResult{}, readErr
			}
			return ExecutionResult{Status: "failed", Output: content, Summary: fmt.Sprintf("replace block old text not found in %s", cleanPath), ShouldContinue: true}, nil
		}
		return ExecutionResult{}, err
	}
	updated, err := ops.ReadFile(cleanPath)
	if err != nil {
		return ExecutionResult{}, err
	}
	artifactPath := e.saveArtifact(run.ID, run.CurrentStep, "replace-block", updated)
	return ExecutionResult{Status: "success", Output: updated, Summary: fmt.Sprintf("replaced block in %s", cleanPath), ArtifactPath: artifactPath, Progressed: true, ShouldContinue: true}, nil
}

func (e *ToolExecutor) insertAfter(ctx context.Context, run Run, task ActionTask, envelope *action.ActionEnvelope) (ExecutionResult, error) {
	ops, cleanPath, err := e.opsForPath(task, envelope.Path)
	if err != nil {
		return ExecutionResult{}, err
	}
	if strings.TrimSpace(envelope.Marker) == "" {
		return ExecutionResult{}, ErrEmptyMarker
	}
	if strings.TrimSpace(envelope.Content) == "" {
		return ExecutionResult{}, ErrEmptyContent
	}
	if err := ops.InsertAfter(cleanPath, envelope.Marker, envelope.Content); err != nil {
		if errors.Is(err, fileops.ErrMarkerNotFound) {
			content, readErr := ops.ReadFile(cleanPath)
			if readErr != nil {
				return ExecutionResult{}, readErr
			}
			return ExecutionResult{Status: "failed", Output: content, Summary: fmt.Sprintf("insert marker not found in %s", cleanPath), ShouldContinue: true}, nil
		}
		return ExecutionResult{}, err
	}
	updated, err := ops.ReadFile(cleanPath)
	if err != nil {
		return ExecutionResult{}, err
	}
	artifactPath := e.saveArtifact(run.ID, run.CurrentStep, "insert-after", updated)
	return ExecutionResult{Status: "success", Output: updated, Summary: fmt.Sprintf("inserted content after marker in %s", cleanPath), ArtifactPath: artifactPath, Progressed: true, ShouldContinue: true}, nil
}

func (e *ToolExecutor) runCommand(ctx context.Context, run Run, task ActionTask, envelope *action.ActionEnvelope) (ExecutionResult, error) {
	if strings.TrimSpace(envelope.Command) == "" {
		return ExecutionResult{}, ErrEmptyCommand
	}
	decision := policy.CheckCommand(envelope.Command)
	if decision.Mode == policy.ModeDeny {
		return ExecutionResult{Status: "denied", Output: decision.Reason, Summary: fmt.Sprintf("command denied: %s", decision.Reason), ShouldContinue: false}, nil
	}
	if decision.Mode == policy.ModeAsk {
		return e.askApproval(ctx, run, task, envelope)
	}
	if strings.TrimSpace(run.SessionID) == "" {
		return ExecutionResult{}, ErrUnsafeWorkspace
	}
	result := e.runner.Run(ctx, run.SessionID, envelope.Command, task.Workspace, 120)
	output := strings.TrimSpace(result.Output)
	if result.ErrorOutput != "" {
		output = strings.TrimSpace(output + "\n" + result.ErrorOutput)
	}
	artifactPath := e.saveArtifact(run.ID, run.CurrentStep, "command-output", output)
	return ExecutionResult{
		Status:         statusFromExitCode(result.ExitCode),
		Output:         output,
		Summary:        fmt.Sprintf("ran command %q (exit=%d)", envelope.Command, result.ExitCode),
		ArtifactPath:   artifactPath,
		Progressed:     result.ExitCode == 0,
		ShouldContinue: true,
	}, nil
}

func (e *ToolExecutor) askApproval(ctx context.Context, run Run, task ActionTask, envelope *action.ActionEnvelope) (ExecutionResult, error) {
	item, err := e.approvalStore.Create(ctx, approval.Approval{
		SessionID:  run.SessionID,
		ActionType: "agentloop.approval",
		Payload:    firstNonEmpty(envelope.Command, envelope.Path, envelope.Target),
		Status:     "pending",
		Reason:     firstNonEmpty(envelope.Reason, "approval required"),
	})
	if err != nil {
		return ExecutionResult{}, err
	}
	return ExecutionResult{Status: "approval_required", Output: item.Reason, Summary: "approval required", ApprovalID: item.ID, ShouldContinue: false}, nil
}

func (e *ToolExecutor) explain(ctx context.Context, run Run, task ActionTask, envelope *action.ActionEnvelope) (ExecutionResult, error) {
	message := strings.TrimSpace(firstNonEmpty(envelope.Reason, envelope.Content))
	if message == "" {
		message = "task marked complete"
	}
	return ExecutionResult{Status: "success", Output: message, Summary: message, Progressed: true, ShouldContinue: false, ShouldFinalize: true}, nil
}

func (e *ToolExecutor) opsForPath(task ActionTask, path string) (*fileops.FileOps, string, error) {
	if strings.TrimSpace(task.Workspace) == "" {
		return nil, "", ErrUnsafeWorkspace
	}
	cleanPath := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	if cleanPath == "." || cleanPath == "" {
		return nil, "", ErrEmptyPath
	}
	if !isAllowedPath(task.AllowedFiles, cleanPath) {
		return nil, "", ErrPathNotInTaskScope
	}
	return fileops.New(task.Workspace), cleanPath, nil
}

func isAllowedPath(allowedFiles []string, candidate string) bool {
	candidate = filepath.ToSlash(filepath.Clean(candidate))
	for _, allowed := range allowedFiles {
		if filepath.ToSlash(filepath.Clean(strings.TrimSpace(allowed))) == candidate {
			return true
		}
	}
	return false
}

func (e *ToolExecutor) saveArtifact(runID string, step int, label string, content string) string {
	if strings.TrimSpace(content) == "" || strings.TrimSpace(e.artifactsDir) == "" {
		return ""
	}
	dir := filepath.Join(e.artifactsDir, runID)
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, fmt.Sprintf("step-%03d-%s-%d.txt", step, label, time.Now().UTC().UnixNano()))
	_ = os.WriteFile(path, []byte(content), 0o644)
	return path
}

func statusFromExitCode(exitCode int) string {
	if exitCode == 0 {
		return "success"
	}
	return "failed"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func marshalJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(data)
}
