package agentloop

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/luannn010/ptolemy/internal/action"
	"github.com/luannn010/ptolemy/internal/brain"
	"github.com/luannn010/ptolemy/internal/navigator"
	"github.com/luannn010/ptolemy/internal/tasks"
)

const maxPromptPreviewChars = 6000

type PromptManifestItem struct {
	ID                  string `json:"id"`
	Title               string `json:"title"`
	Phase               string `json:"phase"`
	Status              string `json:"status"`
	MaxSteps            int    `json:"max_steps"`
	EstimatedBodyTokens int    `json:"estimated_body_tokens"`
	Warning             string `json:"warning,omitempty"`
}

type PromptState struct {
	CurrentTaskID             string               `json:"current_task_id"`
	CurrentTaskTitle          string               `json:"current_task_title"`
	CurrentTaskPhase          string               `json:"current_task_phase"`
	EstimatedPromptTokens     int                  `json:"estimated_prompt_tokens"`
	MaxContextTokens          int                  `json:"max_context_tokens"`
	TargetInputTokens         int                  `json:"target_input_tokens"`
	HardMaxInputTokens        int                  `json:"hard_max_input_tokens"`
	IncludedContextFiles      []string             `json:"included_context_files"`
	TrimmedContextFiles       []string             `json:"trimmed_context_files"`
	PreviousChildSummaries    []string             `json:"previous_child_summaries"`
	BrainTimeout              string               `json:"brain_timeout"`
	ManifestProgress          []PromptManifestItem `json:"manifest_progress"`
	RawPromptPreview          string               `json:"raw_prompt_preview"`
	RawPromptPreviewTruncated bool                 `json:"raw_prompt_preview_truncated"`
	Error                     string               `json:"error,omitempty"`
}

type RunInspection struct {
	Run              Run          `json:"run"`
	WaitingOn        string       `json:"waiting_on"`
	LatestActionType string       `json:"latest_action_type,omitempty"`
	Prompt           *PromptState `json:"prompt,omitempty"`
}

func (s *Service) InspectRun(ctx context.Context, runID string) (RunInspection, error) {
	run, err := s.runStore.GetRun(ctx, runID)
	if err != nil {
		return RunInspection{}, err
	}

	inspection := RunInspection{Run: run}

	task, err := s.loadTask(run)
	if err != nil {
		inspection.WaitingOn = waitingOnForRun(run, nil, nil)
		inspection.Prompt = &PromptState{
			CurrentTaskID:      firstNonEmpty(run.TaskID, "direct-task"),
			CurrentTaskPhase:   run.CurrentPhase,
			MaxContextTokens:   MaxContextTokens,
			TargetInputTokens:  TargetMaxInputTokens,
			HardMaxInputTokens: HardMaxInputTokens,
			BrainTimeout:       s.brainTimeoutString(),
			Error:              err.Error(),
		}
		return inspection, nil
	}

	var actions []action.Action
	if s.actionStore != nil {
		actions, _ = s.actionStore.ListByRun(ctx, run.ID)
	}
	latestAction := latestAction(actions)
	inspection.WaitingOn = waitingOnForRun(run, latestAction, task.Validation)
	if latestAction != nil {
		inspection.LatestActionType = latestAction.Type
	}

	workspace := runWorkspace(run, task)
	promptState := &PromptState{
		CurrentTaskID:      task.ID,
		CurrentTaskTitle:   taskTitle(task, nil),
		CurrentTaskPhase:   run.CurrentPhase,
		MaxContextTokens:   MaxContextTokens,
		TargetInputTokens:  TargetMaxInputTokens,
		HardMaxInputTokens: HardMaxInputTokens,
		BrainTimeout:       s.brainTimeoutString(),
	}
	inspection.Prompt = promptState

	if strings.TrimSpace(workspace) == "" {
		promptState.Error = "workspace is not available for prompt inspection"
		return inspection, nil
	}

	observations := []Observation{}
	if s.runStore != nil {
		observations, _ = s.runStore.ListObservations(ctx, run.ID)
	}

	manifest, child, summaries, err := inspectProcessState(workspace, task, run)
	if err != nil {
		promptState.Error = err.Error()
		return inspection, nil
	}

	if child != nil {
		promptState.CurrentTaskID = child.ID
		promptState.CurrentTaskTitle = firstNonEmpty(child.Title, taskTitle(task, child))
		promptState.CurrentTaskPhase = firstNonEmpty(child.Phase, run.CurrentPhase)
	}

	promptState.ManifestProgress = manifestProgress(manifest)

	assembly, err := buildPromptAssembly(workspace, task, manifest, child, summaries, observations)
	if err != nil {
		promptState.PreviousChildSummaries = contextFilePaths(summaries)
		promptState.Error = err.Error()
		return inspection, nil
	}

	promptState.EstimatedPromptTokens = assembly.EstimatedInputTokens
	promptState.IncludedContextFiles = append([]string{}, assembly.IncludedContext...)
	promptState.TrimmedContextFiles = append([]string{}, assembly.TrimmedContext...)
	promptState.PreviousChildSummaries = append([]string{}, assembly.PreviousSummaries...)
	promptState.RawPromptPreview, promptState.RawPromptPreviewTruncated = promptPreview(assembly.Messages)

	return inspection, nil
}

func inspectProcessState(workspace string, task tasks.Task, run Run) (*tasks.ProcessManifest, *tasks.ProcessChildTask, []navigator.ContextFile, error) {
	if strings.TrimSpace(workspace) == "" {
		return nil, nil, nil, nil
	}

	candidate := tasks.BuildProcessManifest(task)
	paths := tasks.ProcessPathsForWorkspace(workspace, candidate.PackID)
	manifest := &candidate
	if _, err := os.Stat(paths.ManifestPath); err == nil {
		loaded, loadErr := tasks.LoadProcessManifest(paths)
		if loadErr != nil {
			return nil, nil, nil, loadErr
		}
		manifest = &loaded
	} else if tasks.ClassifyTaskBody(task.Body) != tasks.SizeLarge {
		return nil, nil, nil, nil
	}

	child := currentChildTask(manifest, run.CurrentPhase)
	summaries := []navigator.ContextFile{}
	if child != nil {
		loadedSummaries, err := loadProcessSummaryContextFromManifest(workspace, *manifest, child.ID)
		if err != nil {
			return manifest, child, nil, err
		}
		summaries = loadedSummaries
	}
	return manifest, child, summaries, nil
}

func currentChildTask(manifest *tasks.ProcessManifest, currentPhase string) *tasks.ProcessChildTask {
	if manifest == nil {
		return nil
	}

	childID := strings.TrimSpace(manifest.CurrentChildTaskID)
	if childID == "" && strings.HasPrefix(currentPhase, "child_task:") {
		childID = strings.TrimPrefix(currentPhase, "child_task:")
	}

	if childID != "" {
		for i := range manifest.ChildTasks {
			if manifest.ChildTasks[i].ID == childID {
				return &manifest.ChildTasks[i]
			}
		}
	}

	for i := range manifest.ChildTasks {
		if manifest.ChildTasks[i].Status == tasks.ProcessStatusRunning {
			return &manifest.ChildTasks[i]
		}
	}

	for i := range manifest.ChildTasks {
		if manifest.ChildTasks[i].Status == "" || manifest.ChildTasks[i].Status == tasks.ProcessStatusPending {
			return &manifest.ChildTasks[i]
		}
	}
	return nil
}

func loadProcessSummaryContextFromManifest(workspace string, manifest tasks.ProcessManifest, beforeChildID string) ([]navigator.ContextFile, error) {
	summaries := []navigator.ContextFile{}
	for _, child := range manifest.ChildTasks {
		if child.ID == beforeChildID {
			break
		}
		if strings.TrimSpace(child.ResultSummaryPath) == "" {
			continue
		}
		fullPath := filepath.Join(workspace, filepath.FromSlash(child.ResultSummaryPath))
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, navigator.ContextFile{
			Path:    child.ResultSummaryPath,
			Content: string(data),
		})
	}
	return summaries, nil
}

func manifestProgress(manifest *tasks.ProcessManifest) []PromptManifestItem {
	if manifest == nil {
		return nil
	}
	items := make([]PromptManifestItem, 0, len(manifest.ChildTasks))
	for _, child := range manifest.ChildTasks {
		items = append(items, PromptManifestItem{
			ID:                  child.ID,
			Title:               child.Title,
			Phase:               child.Phase,
			Status:              firstNonEmpty(child.Status, tasks.ProcessStatusPending),
			MaxSteps:            child.MaxSteps,
			EstimatedBodyTokens: child.EstimatedBodyTokens,
			Warning:             child.Warning,
		})
	}
	return items
}

func promptPreview(messages []brain.Message) (string, bool) {
	if len(messages) == 0 {
		return "", false
	}

	var builder strings.Builder
	for _, message := range messages {
		if strings.TrimSpace(message.Content) == "" {
			continue
		}
		builder.WriteString(strings.ToUpper(message.Role))
		builder.WriteString(":\n")
		builder.WriteString(message.Content)
		builder.WriteString("\n\n")
	}

	content := strings.TrimSpace(builder.String())
	if len(content) <= maxPromptPreviewChars {
		return content, false
	}
	return strings.TrimSpace(content[:maxPromptPreviewChars]) + "\n\n...[truncated]", true
}

func latestAction(actions []action.Action) *action.Action {
	if len(actions) == 0 {
		return nil
	}
	return &actions[len(actions)-1]
}

func waitingOnForRun(run Run, latest *action.Action, validation []string) string {
	switch run.Status {
	case StatusPaused:
		return "paused"
	case StatusFailed:
		return "failure"
	case StatusCompleted:
		return "finalize"
	case StatusCancelled:
		return "finalize"
	}

	if run.CurrentPhase == "finalizing" {
		return "finalize"
	}

	if latest != nil && latest.Status == "pending" {
		if latest.Type == "run_command" && isValidationCommand(latest.Target, validation) {
			return "validation"
		}
		return "tool_execution"
	}

	return "brain_response"
}

func isValidationCommand(command string, validation []string) bool {
	trimmed := strings.TrimSpace(command)
	for _, item := range validation {
		if strings.TrimSpace(item) == trimmed {
			return true
		}
	}
	return false
}
