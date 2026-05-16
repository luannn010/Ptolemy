package agentloop

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/luannn010/ptolemy/internal/action"
	"github.com/luannn010/ptolemy/internal/navigator"
	"github.com/luannn010/ptolemy/internal/tasks"
)

type childExecutionFacts struct {
	filesInspected   []string
	filesChanged     []string
	commandsRun      []string
	decisions        []string
	errors           []string
	validationResult string
}

type childExecutionResult struct {
	Summary        tasks.ProcessSummary
	ShouldPause    bool
	ShouldFinalize bool
	FailureReason  string
}

func (s *Service) executeTaskRun(ctx context.Context, run Run, task tasks.Task) (Run, error) {
	workspace := runWorkspace(run, task)
	if workspace == "" {
		return Run{}, fmt.Errorf("workspace is required")
	}

	if tasks.ClassifyTaskBody(task.Body) == tasks.SizeLarge {
		return s.executeProcessRun(ctx, run, task, workspace)
	}

	run.Status = StatusRunning
	run.CurrentPhase = "reasoning_loop"
	run.MaxSteps = s.limits.WithMaxSteps(run.MaxSteps).MaxSteps
	updated, err := s.runStore.UpdateRun(ctx, run)
	if err != nil {
		return Run{}, err
	}
	run = updated

	result, err := s.executeScopedTask(ctx, &run, task, nil, nil, nil)
	if err != nil {
		run.Status = StatusFailed
		run.LastError = err.Error()
		return s.runStore.UpdateRun(ctx, run)
	}

	return s.finishRun(ctx, run, task, result)
}

func (s *Service) executeProcessRun(ctx context.Context, run Run, task tasks.Task, workspace string) (Run, error) {
	manifest, paths, err := s.loadOrCreateProcessManifest(workspace, task)
	if err != nil {
		run.Status = StatusFailed
		run.LastError = err.Error()
		return s.runStore.UpdateRun(ctx, run)
	}

	run.Status = StatusRunning
	run.CurrentPhase = "task_process"
	run.MaxSteps = totalProcessSteps(manifest.ChildTasks)
	updated, err := s.runStore.UpdateRun(ctx, run)
	if err != nil {
		return Run{}, err
	}
	run = updated

	for i := range manifest.ChildTasks {
		child := &manifest.ChildTasks[i]
		if child.Status == tasks.ProcessStatusDone || child.Status == tasks.ProcessStatusSkipped {
			continue
		}

		manifest.Status = tasks.ProcessStatusRunning
		manifest.CurrentChildTaskID = child.ID
		child.Status = tasks.ProcessStatusRunning
		if err := persistProcessState(paths, manifest); err != nil {
			run.Status = StatusFailed
			run.LastError = err.Error()
			return s.runStore.UpdateRun(ctx, run)
		}

		run.CurrentPhase = "child_task:" + child.ID
		if run, err = s.runStore.UpdateRun(ctx, run); err != nil {
			return Run{}, err
		}

		previousSummaries, err := s.loadProcessSummaryContext(workspace, manifest, child.ID)
		if err != nil {
			run.Status = StatusFailed
			run.LastError = err.Error()
			return s.runStore.UpdateRun(ctx, run)
		}

		scopedTask := task
		scopedTask.ID = child.ID
		scopedTask.Body = child.Body
		scopedTask.AllowedFiles = append([]string{}, child.AllowedFiles...)
		scopedTask.Validation = append([]string{}, child.ValidationCommands...)

		result, execErr := s.executeScopedTask(ctx, &run, scopedTask, &manifest, child, previousSummaries)
		if execErr != nil {
			child.Status = tasks.ProcessStatusFailed
			child.FailureReason = execErr.Error()
			manifest.Status = tasks.ProcessStatusFailed
			_ = persistProcessState(paths, manifest)
			run.Status = StatusFailed
			run.LastError = execErr.Error()
			return s.runStore.UpdateRun(ctx, run)
		}

		summaryPath, err := tasks.WriteProcessSummary(paths, result.Summary)
		if err != nil {
			run.Status = StatusFailed
			run.LastError = err.Error()
			return s.runStore.UpdateRun(ctx, run)
		}

		child.ResultSummaryPath = summaryPath
		child.FilesChanged = append([]string{}, result.Summary.FilesChanged...)
		child.CommandsRun = append([]string{}, result.Summary.CommandsRun...)

		if result.ShouldPause {
			child.Status = tasks.ProcessStatusPending
			manifest.Status = tasks.ProcessStatusPending
			if err := persistProcessState(paths, manifest); err != nil {
				run.Status = StatusFailed
				run.LastError = err.Error()
				return s.runStore.UpdateRun(ctx, run)
			}
			run.Status = StatusPaused
			return s.runStore.UpdateRun(ctx, run)
		}

		if result.FailureReason != "" {
			child.Status = tasks.ProcessStatusFailed
			child.FailureReason = result.FailureReason
			manifest.Status = tasks.ProcessStatusFailed
			_ = persistProcessState(paths, manifest)
			run.Status = StatusFailed
			run.LastError = result.FailureReason
			return s.runStore.UpdateRun(ctx, run)
		}

		child.Status = tasks.ProcessStatusDone
		child.FailureReason = ""
		manifest.Status = tasks.ProcessStatusRunning
		if err := persistProcessState(paths, manifest); err != nil {
			run.Status = StatusFailed
			run.LastError = err.Error()
			return s.runStore.UpdateRun(ctx, run)
		}
	}

	manifest.Status = tasks.ProcessStatusDone
	manifest.CurrentChildTaskID = ""
	if err := persistProcessState(paths, manifest); err != nil {
		run.Status = StatusFailed
		run.LastError = err.Error()
		return s.runStore.UpdateRun(ctx, run)
	}

	run.Status = StatusCompleted
	run.CurrentPhase = "finalizing"
	return s.runStore.UpdateRun(ctx, run)
}

func (s *Service) executeScopedTask(ctx context.Context, run *Run, task tasks.Task, manifest *tasks.ProcessManifest, child *tasks.ProcessChildTask, summaries []navigator.ContextFile) (childExecutionResult, error) {
	workspace := runWorkspace(*run, task)
	observations := []Observation{}
	facts := childExecutionFacts{}
	run.NoProgressCount = 0

	maxSteps := run.MaxSteps
	if child != nil {
		maxSteps = s.limits.WithMaxSteps(child.MaxSteps).MaxSteps
	}
	if maxSteps == 0 {
		maxSteps = s.limits.WithMaxSteps(0).MaxSteps
	}

	for localStep := 1; localStep <= maxSteps; localStep++ {
		run.CurrentStep++

		assembly, err := buildPromptAssembly(workspace, task, manifest, child, summaries, observations)
		if err != nil {
			return childExecutionResult{
				Summary:       summarizeExecution(task, child, facts, tasks.ProcessStatusFailed, err.Error()),
				FailureReason: err.Error(),
			}, nil
		}
		s.logPromptAssembly(child, assembly)

		reply, err := s.brain.Chat(ctx, assembly.Messages)
		if err != nil {
			return childExecutionResult{
				Summary:       summarizeExecution(task, child, facts, tasks.ProcessStatusFailed, err.Error()),
				FailureReason: err.Error(),
			}, nil
		}

		envelope, err := action.ValidateSingleJSONAction(reply)
		if err != nil {
			run.InvalidJSONCount++
			run.LastError = err.Error()
			observation := Observation{
				RunID:      run.ID,
				Step:       run.CurrentStep,
				Source:     "brain.invalid_json",
				Summary:    summarizeInvalidJSON(err),
				RawPayload: reply,
			}
			created, createErr := s.runStore.CreateObservation(ctx, observation)
			if createErr == nil {
				observations = append(observations, created)
			}
			if run.InvalidJSONCount >= s.limits.MaxInvalidJSON {
				run.CurrentPhase = "invalid_model_output"
				return childExecutionResult{
					Summary:       summarizeExecution(task, child, facts, tasks.ProcessStatusFailed, err.Error()),
					FailureReason: err.Error(),
				}, nil
			}
			if _, updateErr := s.runStore.UpdateRun(ctx, *run); updateErr != nil {
				return childExecutionResult{}, updateErr
			}
			continue
		}

		taskScope := ActionTask{
			ID:           task.ID,
			Branch:       task.Branch,
			AllowedFiles: task.AllowedFiles,
			Validation:   task.Validation,
			Workspace:    workspace,
		}

		recordedAction, err := s.actionStore.Create(ctx, action.Action{
			AgentRunID:    run.ID,
			SessionID:     run.SessionID,
			Type:          envelope.Action,
			Input:         marshalJSON(envelope),
			Status:        "pending",
			Metadata:      "{}",
			Title:         envelope.Title,
			Purpose:       envelope.Purpose,
			ReasoningStep: envelope.ReasoningStep,
			Target:        firstNonEmpty(envelope.Target, envelope.Path, envelope.Command),
		})
		if err != nil {
			return childExecutionResult{}, err
		}

		result, execErr := s.registry.Execute(ctx, *run, taskScope, envelope)
		if execErr != nil {
			_ = s.actionStore.UpdateResult(ctx, recordedAction.ID, execErr.Error(), "failed")
			return childExecutionResult{
				Summary:       summarizeExecution(task, child, facts, tasks.ProcessStatusFailed, execErr.Error()),
				FailureReason: execErr.Error(),
			}, nil
		}

		collectExecutionFacts(&facts, task, envelope, result)
		if !result.Progressed {
			run.NoProgressCount++
		} else {
			run.NoProgressCount = 0
		}

		_ = s.actionStore.UpdateResult(ctx, recordedAction.ID, result.Output, result.Status)
		observation, obsErr := s.runStore.CreateObservation(ctx, Observation{
			RunID:        run.ID,
			ActionID:     recordedAction.ID,
			Step:         run.CurrentStep,
			Source:       envelope.Action,
			Summary:      result.Summary,
			ArtifactPath: result.ArtifactPath,
			RawPayload:   result.Output,
		})
		if obsErr == nil {
			observations = append(observations, observation)
		}

		if result.ShouldFinalize {
			return childExecutionResult{
				Summary:        summarizeExecution(task, child, facts, tasks.ProcessStatusDone, ""),
				ShouldFinalize: true,
			}, nil
		}

		if run.NoProgressCount >= s.limits.MaxNoProgressSteps {
			return childExecutionResult{
				Summary:       summarizeExecution(task, child, facts, tasks.ProcessStatusFailed, "no progress limit reached"),
				FailureReason: "no progress limit reached",
			}, nil
		}

		if _, updateErr := s.runStore.UpdateRun(ctx, *run); updateErr != nil {
			return childExecutionResult{}, updateErr
		}

		if !result.ShouldContinue {
			return childExecutionResult{
				Summary:     summarizeExecution(task, child, facts, tasks.ProcessStatusPending, ""),
				ShouldPause: true,
			}, nil
		}
	}

	return childExecutionResult{
		Summary:       summarizeExecution(task, child, facts, tasks.ProcessStatusFailed, "max steps reached"),
		FailureReason: "max steps reached",
	}, nil
}

func (s *Service) finishRun(ctx context.Context, run Run, task tasks.Task, result childExecutionResult) (Run, error) {
	if result.ShouldPause {
		run.Status = StatusPaused
		return s.runStore.UpdateRun(ctx, run)
	}
	if result.FailureReason != "" {
		run.Status = StatusFailed
		run.LastError = result.FailureReason
		return s.runStore.UpdateRun(ctx, run)
	}
	if result.ShouldFinalize {
		run.CurrentPhase = "finalizing"
		if s.finalizer != nil && run.FinalizationMode != "none" {
			reportPath, finalErr := s.finalizer.Finalize(ctx, run, task)
			run.FinalReportPath = reportPath
			if finalErr != nil {
				run.Status = StatusFailed
				run.LastError = finalErr.Error()
				return s.runStore.UpdateRun(ctx, run)
			}
		}
		run.Status = StatusCompleted
		return s.runStore.UpdateRun(ctx, run)
	}
	run.Status = StatusFailed
	run.LastError = "task exited without finalize signal"
	return s.runStore.UpdateRun(ctx, run)
}

func (s *Service) loadOrCreateProcessManifest(workspace string, task tasks.Task) (tasks.ProcessManifest, tasks.ProcessPaths, error) {
	manifest := tasks.BuildProcessManifest(task)
	paths := tasks.ProcessPathsForWorkspace(workspace, manifest.PackID)
	if _, err := os.Stat(paths.ManifestPath); err == nil {
		loaded, loadErr := tasks.LoadProcessManifest(paths)
		return loaded, paths, loadErr
	}
	paths, err := tasks.EnsureProcessFiles(workspace, manifest)
	if err != nil {
		return tasks.ProcessManifest{}, tasks.ProcessPaths{}, err
	}
	return manifest, paths, nil
}

func (s *Service) loadProcessSummaryContext(workspace string, manifest tasks.ProcessManifest, beforeChildID string) ([]navigator.ContextFile, error) {
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

func collectExecutionFacts(facts *childExecutionFacts, task tasks.Task, envelope *action.ActionEnvelope, result ExecutionResult) {
	switch envelope.Action {
	case "read_file":
		facts.filesInspected = append(facts.filesInspected, envelope.Path)
	case "write_file", "replace_block", "insert_after":
		facts.filesChanged = append(facts.filesChanged, envelope.Path)
	case "run_command":
		facts.commandsRun = append(facts.commandsRun, envelope.Command)
		for _, validation := range task.Validation {
			if strings.TrimSpace(validation) == strings.TrimSpace(envelope.Command) {
				facts.validationResult = result.Status
				break
			}
		}
	case "explain":
		if strings.TrimSpace(result.Summary) != "" {
			facts.decisions = append(facts.decisions, result.Summary)
		}
	}

	if result.Status == "failed" || result.Status == "denied" {
		facts.errors = append(facts.errors, strings.TrimSpace(result.Summary))
	}
}

func summarizeExecution(task tasks.Task, child *tasks.ProcessChildTask, facts childExecutionFacts, status string, failure string) tasks.ProcessSummary {
	title := taskTitle(task, child)
	nextStep := "none"
	if status != tasks.ProcessStatusDone {
		nextStep = "address the blocker and rerun the current child task"
	}
	if child != nil && status == tasks.ProcessStatusDone {
		nextStep = "continue to the next child task"
	}

	errors := append([]string{}, facts.errors...)
	if strings.TrimSpace(failure) != "" {
		errors = append(errors, failure)
	}

	return tasks.ProcessSummary{
		ChildTaskID:         task.ID,
		Title:               title,
		Status:              status,
		FilesInspected:      dedupeReferencedPaths(facts.filesInspected),
		FilesChanged:        dedupeReferencedPaths(facts.filesChanged),
		CommandsRun:         dedupeReferencedPaths(facts.commandsRun),
		ValidationResult:    firstNonEmpty(facts.validationResult, "not run"),
		ImportantDecisions:  dedupeReferencedPaths(facts.decisions),
		ErrorsBlockers:      dedupeReferencedPaths(errors),
		NextRecommendedStep: nextStep,
	}
}

func persistProcessState(paths tasks.ProcessPaths, manifest tasks.ProcessManifest) error {
	if err := tasks.WriteProcessManifest(paths.ManifestPath, manifest); err != nil {
		return err
	}
	return tasks.WriteProcessTodo(paths.TodoPath, manifest)
}

func totalProcessSteps(children []tasks.ProcessChildTask) int {
	total := 0
	for _, child := range children {
		total += child.MaxSteps
	}
	if total == 0 {
		return DefaultLimits().MaxSteps
	}
	return total
}

func taskTitle(task tasks.Task, child *tasks.ProcessChildTask) string {
	if child != nil && strings.TrimSpace(child.Title) != "" {
		return child.Title
	}
	for _, line := range strings.Split(task.Body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			return strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		}
	}
	return task.ID
}

func (s *Service) logPromptAssembly(child *tasks.ProcessChildTask, assembly PromptAssembly) {
	childID := "<direct>"
	if child != nil && strings.TrimSpace(child.ID) != "" {
		childID = child.ID
	}
	log.Printf(
		"agentloop child_task=%s estimated_prompt_tokens=%d max_context_tokens=%d included_context=%s trimmed_context=%s brain_timeout=%s",
		childID,
		assembly.EstimatedInputTokens,
		MaxContextTokens,
		strings.Join(assembly.IncludedContext, ","),
		strings.Join(assembly.TrimmedContext, ","),
		s.brainTimeoutString(),
	)
}
