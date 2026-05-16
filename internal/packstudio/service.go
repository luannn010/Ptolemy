package packstudio

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/luannn010/ptolemy/internal/action"
	"github.com/luannn010/ptolemy/internal/agentloop"
	"github.com/luannn010/ptolemy/internal/command"
	"github.com/luannn010/ptolemy/internal/tasks"
	"github.com/luannn010/ptolemy/internal/terminal"
)

type Service struct {
	root          string
	store         *Store
	agentRunStore *agentloop.Store
	agentService  *agentloop.Service
	actionStore   *action.Store
	commandStore  *command.Store
	runner        *terminal.TmuxRunner

	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func NewService(root string, store *Store, agentRunStore *agentloop.Store, agentService *agentloop.Service, actionStore *action.Store, commandStore *command.Store, runner *terminal.TmuxRunner) *Service {
	return &Service{
		root:          root,
		store:         store,
		agentRunStore: agentRunStore,
		agentService:  agentService,
		actionStore:   actionStore,
		commandStore:  commandStore,
		runner:        runner,
		cancels:       map[string]context.CancelFunc{},
	}
}

func (s *Service) Recover(ctx context.Context) error {
	return s.store.MarkRunningRunsFailed(ctx, "interrupted by workerd restart")
}

func (s *Service) Root() string {
	return s.root
}

func (s *Service) Store() *Store {
	return s.store
}

func (s *Service) CommandStore() *command.Store {
	return s.commandStore
}

func (s *Service) Runner() *terminal.TmuxRunner {
	return s.runner
}

func (s *Service) CreatePack(input CreatePackInput) (PackDetail, error) {
	return WritePack(s.root, input)
}

func (s *Service) CreateProgram(input CreateProgramInput) (ProgramDefinition, error) {
	return WriteProgram(s.root, input)
}

type StartProgramRunInput struct {
	ProgramID string `json:"program_id"`
	PackID    string `json:"pack_id"`
	Workspace string `json:"workspace"`
}

func (s *Service) StartProgramRun(ctx context.Context, input StartProgramRunInput) (ProgramRun, error) {
	active, err := s.store.HasActiveProgramRun(ctx)
	if err != nil {
		return ProgramRun{}, err
	}
	if active {
		return ProgramRun{}, fmt.Errorf("another program run is already active")
	}

	program, packDetails, err := s.resolveProgram(input)
	if err != nil {
		return ProgramRun{}, err
	}

	totalTasks := 0
	for _, item := range packDetails {
		totalTasks += len(item.Tasks)
	}

	workspace := strings.TrimSpace(input.Workspace)
	if workspace == "" {
		workspace = s.root
	}

	programRun, err := s.store.CreateProgramRun(ctx, ProgramRun{
		ProgramID:       program.ID,
		ProgramName:     program.Name,
		Mode:            resolveRunMode(input),
		Status:          StatusPlanning,
		Workspace:       workspace,
		TotalPacks:      len(packDetails),
		TotalTasks:      totalTasks,
		PercentComplete: 0,
	})
	if err != nil {
		return ProgramRun{}, err
	}

	for i, detail := range packDetails {
		packRun, packErr := s.store.CreatePackRun(ctx, PackRun{
			ProgramRunID: programRun.ID,
			PackID:       detail.ID,
			PackName:     detail.Name,
			PackPath:     detail.Path,
			Status:       StatusPending,
			Position:     i,
			TotalTasks:   len(detail.Tasks),
		})
		if packErr != nil {
			return ProgramRun{}, packErr
		}
		for j, taskDetail := range detail.Tasks {
			taskChecklist := taskDetail.Checklist
			if len(taskChecklist) == 0 {
				taskChecklist = statusChecklist(taskDetail.Status)
			}
			_, taskErr := s.store.CreatePackRunTask(ctx, PackRunTask{
				PackRunID:       packRun.ID,
				TaskID:          taskDetail.ID,
				Title:           taskDetail.Title,
				TaskPath:        taskDetail.Path,
				Branch:          taskDetail.Branch,
				Status:          StatusPending,
				Position:        j,
				ExecutionGroup:  "sequential",
				DependsOnRaw:    marshalStrings(taskDetail.DependsOn),
				ValidationRaw:   marshalStrings(taskDetail.Validation),
				AllowedFilesRaw: marshalStrings(taskDetail.AllowedFiles),
				ChecklistRaw:    marshalChecklist(taskChecklist),
			})
			if taskErr != nil {
				return ProgramRun{}, taskErr
			}
		}
	}

	_, _ = s.store.CreateEvent(ctx, RunEvent{
		ProgramRunID: programRun.ID,
		EventType:    "program.created",
		Message:      fmt.Sprintf("Created program run for %s", program.Name),
	})

	runCtx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancels[programRun.ID] = cancel
	s.mu.Unlock()

	go s.executeProgram(runCtx, programRun.ID, program, workspace)
	return programRun, nil
}

func (s *Service) CancelProgramRun(ctx context.Context, programRunID string) error {
	s.mu.Lock()
	cancel := s.cancels[programRunID]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	run, err := s.store.GetProgramRun(ctx, programRunID)
	if err != nil {
		return err
	}
	run.Status = StatusCancelled
	run.LastError = "cancelled by operator"
	run.FinishedAt = time.Now().UTC()
	if _, err := s.store.UpdateProgramRun(ctx, run); err != nil {
		return err
	}
	_, _ = s.store.CreateEvent(ctx, RunEvent{
		ProgramRunID: programRunID,
		EventType:    "program.cancelled",
		Message:      "Program run cancelled by operator",
	})
	return nil
}

func (s *Service) BuildProgramRunDetail(ctx context.Context, programRunID string) (ProgramRunDetail, error) {
	programRun, err := s.store.GetProgramRun(ctx, programRunID)
	if err != nil {
		return ProgramRunDetail{}, err
	}

	detail := ProgramRunDetail{ProgramRun: programRun}
	if programRun.ProgramID != "" {
		if programDef, _, getErr := GetProgram(s.root, programRun.ProgramID); getErr == nil {
			detail.Program = &programDef
		}
	}

	packRuns, err := s.store.ListPackRunsByProgram(ctx, programRunID)
	if err != nil {
		return ProgramRunDetail{}, err
	}
	detail.Packs = make([]PackRunDetail, 0, len(packRuns))
	for _, packRun := range packRuns {
		taskRows, taskErr := s.store.ListPackRunTasks(ctx, packRun.ID)
		if taskErr != nil {
			return ProgramRunDetail{}, taskErr
		}
		packDetail := PackRunDetail{PackRun: packRun, Tasks: make([]RunTaskDetail, 0, len(taskRows))}
		for _, taskRow := range taskRows {
			taskDetail := RunTaskDetail{
				PackRunTask:  taskRow,
				DependsOn:    unmarshalStrings(taskRow.DependsOnRaw),
				Validation:   unmarshalStrings(taskRow.ValidationRaw),
				AllowedFiles: unmarshalStrings(taskRow.AllowedFilesRaw),
				Checklist:    applyStatusToChecklist(unmarshalChecklist(taskRow.ChecklistRaw), taskRow.Status),
			}
			if taskRow.AgentRunID != "" {
				if agentRun, getErr := s.agentRunStore.GetRun(ctx, taskRow.AgentRunID); getErr == nil {
					taskDetail.SessionID = firstNonEmpty(agentRun.SessionID, taskDetail.SessionID)
				}
			}
			packDetail.Tasks = append(packDetail.Tasks, taskDetail)
		}
		detail.Packs = append(detail.Packs, packDetail)
	}

	events, err := s.store.ListEvents(ctx, programRunID, 200)
	if err != nil {
		return ProgramRunDetail{}, err
	}
	detail.Events = events
	return detail, nil
}

func (s *Service) CurrentTaskDetail(ctx context.Context, programRunID string) (*RunTaskDetail, []action.Action, []agentloop.Observation, []command.CommandLog, error) {
	runDetail, err := s.BuildProgramRunDetail(ctx, programRunID)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	for _, pack := range runDetail.Packs {
		for _, task := range pack.Tasks {
			if task.Status != StatusRunning && task.Status != StatusWaitingOnAgent {
				continue
			}
			actions, observations, logs, fetchErr := s.loadTaskRuntime(ctx, &task)
			return &task, actions, observations, logs, fetchErr
		}
	}

	for i := len(runDetail.Packs) - 1; i >= 0; i-- {
		tasks := runDetail.Packs[i].Tasks
		for j := len(tasks) - 1; j >= 0; j-- {
			task := tasks[j]
			if task.AgentRunID == "" {
				continue
			}
			actions, observations, logs, fetchErr := s.loadTaskRuntime(ctx, &task)
			return &task, actions, observations, logs, fetchErr
		}
	}
	return nil, []action.Action{}, []agentloop.Observation{}, []command.CommandLog{}, nil
}

func (s *Service) BuildRunOperatorState(ctx context.Context, programRun ProgramRun, task *RunTaskDetail, actions []action.Action) RunOperatorState {
	terminal := s.BuildTerminalState(ctx, programRun, task)
	terminal.Snapshot = ""
	state := RunOperatorState{
		Terminal: terminal,
	}

	if task == nil || task.AgentRunID == "" || s.agentService == nil {
		return state
	}

	inspection, err := s.agentService.InspectRun(ctx, task.AgentRunID)
	if err == nil {
		if inspection.WaitingOn == "" {
			inspection.WaitingOn = waitingOnFromRunAndActions(programRun, task, actions)
		}
		state.AgentRun = &inspection
	}
	return state
}

func (s *Service) BuildTerminalState(ctx context.Context, programRun ProgramRun, task *RunTaskDetail) TerminalState {
	state := TerminalState{
		Status:      "no_session",
		Message:     "Waiting for an agent session to start.",
		Source:      "none",
		LastUpdated: time.Now().UTC(),
	}

	if task == nil {
		if programRun.Status == StatusFailed {
			state.Status = "failed"
			state.Message = firstNonEmpty(programRun.LastError, "This run failed before a live terminal session was captured.")
		} else if programRun.Status == StatusCompleted {
			state.Status = "completed"
			state.Message = "This run completed before a terminal session was attached."
		}
		return state
	}

	state.TaskID = task.TaskID
	state.TaskTitle = task.Title
	state.SessionID = task.SessionID
	state.Phase = task.Status
	state.Source = terminalSource(programRun, task)

	if strings.TrimSpace(task.SessionID) == "" {
		state.Status = terminalLifecycleStatus(programRun, task, false)
		state.Message = terminalLifecycleMessage(state.Status, programRun, task)
		return state
	}

	hasSession := s.runner != nil && s.runner.HasSession(ctx, task.SessionID)
	state.HasSession = hasSession
	if !hasSession {
		state.Status = terminalLifecycleStatus(programRun, task, false)
		state.Message = terminalLifecycleMessage(state.Status, programRun, task)
		return state
	}

	snapshot, err := s.runner.CaptureSession(ctx, task.SessionID)
	if err != nil {
		state.Status = terminalLifecycleStatus(programRun, task, false)
		state.Message = terminalLifecycleMessage(state.Status, programRun, task)
		return state
	}

	state.CaptureAvailable = true
	state.Snapshot = snapshot
	state.SessionBootstrapped = strings.Contains(snapshot, "[ptolemy] terminal session ready")
	state.Status = terminalLifecycleStatus(programRun, task, true)
	state.Message = terminalLifecycleMessage(state.Status, programRun, task)
	return state
}

func (s *Service) loadTaskRuntime(ctx context.Context, task *RunTaskDetail) ([]action.Action, []agentloop.Observation, []command.CommandLog, error) {
	actions := []action.Action{}
	observations := []agentloop.Observation{}
	logs := []command.CommandLog{}
	if task.AgentRunID != "" && s.actionStore != nil {
		if loadedActions, err := s.actionStore.ListByRun(ctx, task.AgentRunID); err == nil {
			actions = loadedActions
		}
	}
	if task.AgentRunID != "" && s.agentRunStore != nil {
		if loadedObservations, err := s.agentRunStore.ListObservations(ctx, task.AgentRunID); err == nil {
			observations = loadedObservations
		}
		if agentRun, err := s.agentRunStore.GetRun(ctx, task.AgentRunID); err == nil {
			task.SessionID = firstNonEmpty(task.SessionID, agentRun.SessionID)
		}
	}
	if task.SessionID != "" && s.commandStore != nil {
		if loadedLogs, err := s.commandStore.ListBySession(ctx, task.SessionID); err == nil {
			logs = loadedLogs
		}
	}
	return actions, observations, logs, nil
}

func waitingOnFromRunAndActions(programRun ProgramRun, task *RunTaskDetail, actions []action.Action) string {
	switch programRun.Status {
	case StatusFailed:
		return "failure"
	case StatusCompleted, StatusCancelled:
		return "finalize"
	}

	if task == nil {
		return "brain_response"
	}

	for i := len(actions) - 1; i >= 0; i-- {
		if actions[i].Status != "pending" {
			continue
		}
		if actions[i].Type == "run_command" && containsCommand(task.Validation, actions[i].Target) {
			return "validation"
		}
		return "tool_execution"
	}

	if task.Status == StatusWaitingOnAgent || task.Status == StatusRunning {
		return "brain_response"
	}
	if task.Status == StatusFailed {
		return "failure"
	}
	return "finalize"
}

func containsCommand(commands []string, target string) bool {
	trimmed := strings.TrimSpace(target)
	for _, command := range commands {
		if strings.TrimSpace(command) == trimmed {
			return true
		}
	}
	return false
}

func terminalSource(programRun ProgramRun, task *RunTaskDetail) string {
	if task == nil {
		return "none"
	}
	if task.TaskID == programRun.CurrentTaskID && (task.Status == StatusRunning || task.Status == StatusWaitingOnAgent) {
		return "current"
	}
	return "latest"
}

func terminalLifecycleStatus(programRun ProgramRun, task *RunTaskDetail, captureAvailable bool) string {
	switch {
	case task == nil:
		return "no_session"
	case programRun.Status == StatusFailed || task.Status == StatusFailed:
		return "failed"
	case programRun.Status == StatusCompleted || task.Status == StatusCompleted:
		if captureAvailable {
			return "completed"
		}
		return "no_session"
	case programRun.Status == StatusCancelled:
		return "completed"
	case task.SessionID == "":
		return "no_session"
	case !captureAvailable:
		return "bootstrapping"
	default:
		return "running"
	}
}

func terminalLifecycleMessage(status string, programRun ProgramRun, task *RunTaskDetail) string {
	switch status {
	case "failed":
		return firstNonEmpty(task.LastError, programRun.LastError, "The run failed. Review the terminal and recent events for the last successful output.")
	case "completed":
		return "Command execution completed. This pane is showing the most recent terminal snapshot for the run."
	case "running":
		return "Live terminal capture is active for the current agent session."
	case "bootstrapping":
		return "The tmux session exists, but live pane output is not available yet. The session may still be bootstrapping."
	default:
		if task != nil && strings.TrimSpace(task.SessionID) != "" {
			return "The agent session has been created, but no tmux output has been captured yet."
		}
		return "No tmux session is attached to this run yet."
	}
}

func (s *Service) resolveProgram(input StartProgramRunInput) (ProgramDefinition, []PackDetail, error) {
	if strings.TrimSpace(input.ProgramID) != "" {
		program, validationErrs, err := GetProgram(s.root, input.ProgramID)
		if err != nil {
			return ProgramDefinition{}, nil, err
		}
		if len(validationErrs) > 0 {
			return ProgramDefinition{}, nil, fmt.Errorf("%s", strings.Join(validationErrs, "; "))
		}
		packs := []PackDetail{}
		for _, ref := range orderedProgramPacks(program) {
			detail, detailErr := GetPack(s.root, ref.PackID)
			if detailErr != nil {
				return ProgramDefinition{}, nil, detailErr
			}
			if !detail.Valid {
				return ProgramDefinition{}, nil, fmt.Errorf("invalid pack %s: %s", detail.ID, strings.Join(detail.ValidationErrors, "; "))
			}
			packs = append(packs, detail)
		}
		return program, packs, nil
	}

	if strings.TrimSpace(input.PackID) == "" {
		return ProgramDefinition{}, nil, fmt.Errorf("program_id or pack_id is required")
	}

	detail, err := GetPack(s.root, input.PackID)
	if err != nil {
		return ProgramDefinition{}, nil, err
	}
	if !detail.Valid {
		return ProgramDefinition{}, nil, fmt.Errorf("invalid pack %s: %s", detail.ID, strings.Join(detail.ValidationErrors, "; "))
	}
	return ProgramDefinition{
		ID:          detail.ID,
		Name:        detail.Name,
		Description: detail.Goal,
		Packs:       []ProgramPackRef{{PackID: detail.ID, Order: 0}},
	}, []PackDetail{detail}, nil
}

func orderedProgramPacks(program ProgramDefinition) []ProgramPackRef {
	remaining := append([]ProgramPackRef{}, program.Packs...)
	done := map[string]bool{}
	out := make([]ProgramPackRef, 0, len(remaining))
	for len(remaining) > 0 {
		progressed := false
		for i, ref := range remaining {
			ready := true
			for _, dep := range ref.DependsOn {
				if !done[dep] {
					ready = false
					break
				}
			}
			if !ready {
				continue
			}
			out = append(out, ref)
			done[ref.PackID] = true
			remaining = slices.Delete(remaining, i, i+1)
			progressed = true
			break
		}
		if !progressed {
			return program.Packs
		}
	}
	return out
}

func resolveRunMode(input StartProgramRunInput) string {
	if strings.TrimSpace(input.ProgramID) != "" {
		return "program"
	}
	return "pack"
}

func (s *Service) executeProgram(ctx context.Context, programRunID string, program ProgramDefinition, workspace string) {
	defer s.clearCancel(programRunID)

	run, err := s.store.GetProgramRun(ctx, programRunID)
	if err != nil {
		return
	}
	run.Status = StatusRunning
	run.StartedAt = time.Now().UTC()
	if _, err := s.store.UpdateProgramRun(ctx, run); err != nil {
		return
	}
	_, _ = s.store.CreateEvent(ctx, RunEvent{
		ProgramRunID: programRunID,
		EventType:    "program.started",
		Message:      fmt.Sprintf("Started %s", program.Name),
	})

	packRuns, err := s.store.ListPackRunsByProgram(ctx, programRunID)
	if err != nil {
		return
	}

	completedTasks := 0
	for _, packRun := range packRuns {
		select {
		case <-ctx.Done():
			s.failProgram(ctx, programRunID, "cancelled by operator")
			return
		default:
		}

		run.CurrentPackID = packRun.PackID
		run.CurrentTaskID = ""
		if _, err := s.store.UpdateProgramRun(ctx, run); err != nil {
			return
		}

		packRun.Status = StatusRunning
		packRun.StartedAt = time.Now().UTC()
		if _, err := s.store.UpdatePackRun(ctx, packRun); err != nil {
			s.failProgram(ctx, programRunID, err.Error())
			return
		}
		_, _ = s.store.CreateEvent(ctx, RunEvent{
			ProgramRunID: programRunID,
			PackRunID:    packRun.ID,
			EventType:    "pack.started",
			Message:      fmt.Sprintf("Started pack %s", packRun.PackName),
		})

		taskRows, err := s.store.ListPackRunTasks(ctx, packRun.ID)
		if err != nil {
			s.failProgram(ctx, programRunID, err.Error())
			return
		}

		for _, taskRow := range taskRows {
			select {
			case <-ctx.Done():
				s.failProgram(ctx, programRunID, "cancelled by operator")
				return
			default:
			}

			run.Status = StatusWaitingOnAgent
			run.CurrentTaskID = taskRow.TaskID
			if _, err := s.store.UpdateProgramRun(ctx, run); err != nil {
				s.failProgram(ctx, programRunID, err.Error())
				return
			}

			packRun.CurrentTaskID = taskRow.TaskID
			packRun.Status = StatusWaitingOnAgent
			if _, err := s.store.UpdatePackRun(ctx, packRun); err != nil {
				s.failProgram(ctx, programRunID, err.Error())
				return
			}

			taskRow.Status = StatusRunning
			taskRow.StartedAt = time.Now().UTC()
			if _, err := s.store.UpdatePackRunTask(ctx, taskRow); err != nil {
				s.failProgram(ctx, programRunID, err.Error())
				return
			}
			_, _ = s.store.CreateEvent(ctx, RunEvent{
				ProgramRunID:  programRunID,
				PackRunID:     packRun.ID,
				PackRunTaskID: taskRow.ID,
				EventType:     "task.started",
				Message:       fmt.Sprintf("Started task %s", taskRow.TaskID),
			})

			agentRun, runErr := s.startAgentRun(ctx, workspace, taskRow)
			if runErr != nil {
				taskRow.Status = StatusFailed
				taskRow.LastError = runErr.Error()
				taskRow.FinishedAt = time.Now().UTC()
				_, _ = s.store.UpdatePackRunTask(ctx, taskRow)
				s.failPack(ctx, packRun, taskRow.TaskID, runErr.Error())
				s.failProgram(ctx, programRunID, runErr.Error())
				return
			}

			taskRow.AgentRunID = agentRun.ID
			taskRow.SessionID = agentRun.SessionID
			taskRow.FinishedAt = time.Now().UTC()
			if agentRun.Status == agentloop.StatusCompleted {
				taskRow.Status = StatusCompleted
				_ = tasks.UpdateTaskStatusFile(taskRow.TaskPath, tasks.StatusCompleted)
				completedTasks++
			} else if agentRun.Status == agentloop.StatusCancelled {
				taskRow.Status = StatusCancelled
				taskRow.LastError = firstNonEmpty(agentRun.LastError, "cancelled")
				_ = tasks.UpdateTaskStatusFile(taskRow.TaskPath, tasks.StatusFailed)
			} else {
				taskRow.Status = StatusFailed
				taskRow.LastError = firstNonEmpty(agentRun.LastError, "agent run failed")
				_ = tasks.UpdateTaskStatusFile(taskRow.TaskPath, tasks.StatusFailed)
			}
			if _, err := s.store.UpdatePackRunTask(ctx, taskRow); err != nil {
				s.failProgram(ctx, programRunID, err.Error())
				return
			}
			_, _ = s.store.CreateEvent(ctx, RunEvent{
				ProgramRunID:  programRunID,
				PackRunID:     packRun.ID,
				PackRunTaskID: taskRow.ID,
				AgentRunID:    taskRow.AgentRunID,
				SessionID:     taskRow.SessionID,
				EventType:     "task.finished",
				Message:       fmt.Sprintf("Task %s %s", taskRow.TaskID, taskRow.Status),
			})

			if taskRow.Status != StatusCompleted {
				s.failPack(ctx, packRun, taskRow.TaskID, taskRow.LastError)
				s.failProgram(ctx, programRunID, taskRow.LastError)
				return
			}

			packRun.CompletedTasks++
			packRun.PercentComplete = percent(packRun.CompletedTasks, packRun.TotalTasks)
			packRun.CurrentTaskID = taskRow.TaskID
			if _, err := s.store.UpdatePackRun(ctx, packRun); err != nil {
				s.failProgram(ctx, programRunID, err.Error())
				return
			}
			run.CompletedTasks = completedTasks
			run.PercentComplete = percent(run.CompletedTasks, run.TotalTasks)
			run.Status = StatusRunning
			if _, err := s.store.UpdateProgramRun(ctx, run); err != nil {
				s.failProgram(ctx, programRunID, err.Error())
				return
			}
		}

		packRun.Status = StatusCompleted
		packRun.FinishedAt = time.Now().UTC()
		packRun.PercentComplete = 100
		if _, err := s.store.UpdatePackRun(ctx, packRun); err != nil {
			s.failProgram(ctx, programRunID, err.Error())
			return
		}
		_, _ = s.store.CreateEvent(ctx, RunEvent{
			ProgramRunID: programRunID,
			PackRunID:    packRun.ID,
			EventType:    "pack.finished",
			Message:      fmt.Sprintf("Completed pack %s", packRun.PackName),
		})
		run.CompletedPacks++
	}

	run.Status = StatusCompleted
	run.FinishedAt = time.Now().UTC()
	run.PercentComplete = 100
	run.CompletedTasks = run.TotalTasks
	if _, err := s.store.UpdateProgramRun(ctx, run); err != nil {
		return
	}
	_, _ = s.store.CreateEvent(ctx, RunEvent{
		ProgramRunID: programRunID,
		EventType:    "program.finished",
		Message:      fmt.Sprintf("Completed %s", program.Name),
	})
}

func (s *Service) startAgentRun(ctx context.Context, workspace string, taskRow PackRunTask) (agentloop.Run, error) {
	data, err := os.ReadFile(taskRow.TaskPath)
	if err != nil {
		return agentloop.Run{}, err
	}
	task, err := tasks.ParseTaskMarkdown(taskRow.TaskPath, data)
	if err != nil {
		return agentloop.Run{}, err
	}
	_ = tasks.UpdateTaskStatusFile(taskRow.TaskPath, tasks.StatusRunning)
	run, err := s.agentRunStore.CreateRun(ctx, agentloop.Run{
		TaskID:           task.ID,
		TaskFile:         relativeToRoot(workspace, taskRow.TaskPath),
		Workspace:        workspace,
		Branch:           task.Branch,
		Status:           agentloop.StatusPending,
		MaxSteps:         8,
		CurrentPhase:     "pack_studio",
		FinalizationMode: "none",
	})
	if err != nil {
		return agentloop.Run{}, err
	}
	finished, err := s.agentService.StartOrResume(ctx, run.ID)
	if err != nil {
		return agentloop.Run{}, err
	}
	return finished, nil
}

func (s *Service) failPack(ctx context.Context, packRun PackRun, taskID string, message string) {
	packRun.Status = StatusFailed
	packRun.CurrentTaskID = taskID
	packRun.LastError = message
	packRun.FinishedAt = time.Now().UTC()
	_, _ = s.store.UpdatePackRun(ctx, packRun)
	_, _ = s.store.CreateEvent(ctx, RunEvent{
		ProgramRunID: packRun.ProgramRunID,
		PackRunID:    packRun.ID,
		EventType:    "pack.failed",
		Message:      fmt.Sprintf("Pack %s failed: %s", packRun.PackName, message),
	})
}

func (s *Service) failProgram(ctx context.Context, programRunID string, message string) {
	run, err := s.store.GetProgramRun(ctx, programRunID)
	if err != nil {
		return
	}
	run.Status = StatusFailed
	run.LastError = message
	run.FinishedAt = time.Now().UTC()
	_, _ = s.store.UpdateProgramRun(ctx, run)
	_, _ = s.store.CreateEvent(ctx, RunEvent{
		ProgramRunID: programRunID,
		EventType:    "program.failed",
		Message:      fmt.Sprintf("Program failed: %s", message),
	})
}

func (s *Service) clearCancel(programRunID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cancels, programRunID)
}

func percent(done int, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(done) / float64(total) * 100
}

func relativeToRoot(root string, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}
