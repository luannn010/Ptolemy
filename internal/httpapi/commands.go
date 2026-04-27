package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/luannn010/ptolemy/internal/action"
	"github.com/luannn010/ptolemy/internal/approval"
	"github.com/luannn010/ptolemy/internal/command"
	"github.com/luannn010/ptolemy/internal/logs"
	"github.com/luannn010/ptolemy/internal/memory"
	"github.com/luannn010/ptolemy/internal/policy"
	"github.com/luannn010/ptolemy/internal/session"
	"github.com/luannn010/ptolemy/internal/terminal"
	"github.com/rs/zerolog/log"
)

const maxOutputSize = 10000 // 10KB

type CommandHandler struct {
	sessionStore  *session.Store
	commandStore  *command.Store
	actionStore   *action.Store
	logStore      *logs.Store
	approvalStore *approval.Store
	runner        *terminal.TmuxRunner
}

func NewCommandHandler(
	sessionStore *session.Store,
	commandStore *command.Store,
	actionStore *action.Store,
	logStore *logs.Store,
	approvalStore *approval.Store,
	runner *terminal.TmuxRunner,
) *CommandHandler {
	return &CommandHandler{
		sessionStore:  sessionStore,
		commandStore:  commandStore,
		actionStore:   actionStore,
		logStore:      logStore,
		approvalStore: approvalStore,
		runner:        runner,
	}
}

func (h *CommandHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Post("/", h.runCommand)
	r.Get("/", h.listCommands)

	return r
}

func (h *CommandHandler) runCommand(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")

	sess, err := h.sessionStore.Get(r.Context(), sessionID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, session.ErrSessionNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	if sess.Status != session.StatusOpen {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "session is not open",
		})
		return
	}

	var req command.RunCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid JSON body",
		})
		return
	}

	if req.Command == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "command is required",
		})
		return
	}

	if req.CWD == "" {
		req.CWD = sess.Workspace
	}

	mem, err := memory.LoadWorkspaceMemory(sess.Workspace)
	if err != nil {
		log.Warn().
			Err(err).
			Str("session_id", sessionID).
			Msg("failed to load memory for execution")
	} else {
		log.Info().
			Str("session_id", sessionID).
			Int("global_files", len(mem.Global)).
			Int("project_files", len(mem.Project)).
			Msg("memory loaded for execution")
	}
	decision := policy.CheckCommand(req.Command)

	if decision.Mode == policy.ModeDeny {
		_, _ = h.logStore.Create(r.Context(), logs.Log{
			SessionID: sessionID,
			Level:     "warn",
			Message:   "command denied by policy",
			Metadata:  `{"command":` + strconv.Quote(req.Command) + `}`,
		})

		writeJSON(w, http.StatusForbidden, map[string]any{
			"error":       "policy_denied",
			"reason":      decision.Reason,
			"action_type": decision.ActionType,
		})
		return
	}

	if decision.Mode == policy.ModeAsk {
		approvalItem, err := h.approvalStore.Create(r.Context(), approval.Approval{
			SessionID:  sessionID,
			ActionType: decision.ActionType,
			Payload:    req.Command,
			Status:     "pending",
			Reason:     decision.Reason,
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusForbidden, map[string]any{
			"error":       "approval_required",
			"reason":      decision.Reason,
			"action_type": decision.ActionType,
			"approval_id": approvalItem.ID,
		})
		return
	}
	act, err := h.actionStore.Create(r.Context(), action.Action{
		SessionID: sessionID,
		Type:      "command.exec",
		Input:     req.Command,
		Status:    "pending",
		Metadata:  "{}",
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	result := h.runner.Run(r.Context(), sessionID, req.Command, req.CWD, req.Timeout)

	if len(result.Output) > maxOutputSize {
		result.Output = result.Output[:maxOutputSize] + "\n...[truncated]"
	}

	if len(result.ErrorOutput) > maxOutputSize {
		result.ErrorOutput = result.ErrorOutput[:maxOutputSize] + "\n...[truncated]"
	}

	status := "success"
	if result.ExitCode != 0 {
		status = "failed"
	}

	combinedOutput := result.Output
	if result.ErrorOutput != "" {
		combinedOutput += "\n" + result.ErrorOutput
	}

	if err := h.actionStore.UpdateResult(r.Context(), act.ID, combinedOutput, status); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	_, _ = h.logStore.Create(r.Context(), logs.Log{
		SessionID: sessionID,
		ActionID:  act.ID,
		Level:     "info",
		Message:   "command executed",
		Metadata:  "{}",
	})

	log.Info().
		Str("session_id", sessionID).
		Str("action_id", act.ID).
		Str("command", req.Command).
		Int("exit_code", result.ExitCode).
		Int64("duration_ms", result.DurationMS).
		Msg("command executed")

	logItem, err := h.commandStore.Create(r.Context(), command.CommandLog{
		SessionID:   sessionID,
		Command:     req.Command,
		CWD:         req.CWD,
		ExitCode:    result.ExitCode,
		Output:      result.Output,
		ErrorOutput: result.ErrorOutput,
		DurationMS:  result.DurationMS,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, logItem)
}

func (h *CommandHandler) listCommands(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")

	commandLogs, err := h.commandStore.ListBySession(r.Context(), sessionID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, commandLogs)
}
