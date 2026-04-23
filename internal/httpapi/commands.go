package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/luannn010/ptolemy/internal/command"
	"github.com/luannn010/ptolemy/internal/session"
	"github.com/luannn010/ptolemy/internal/terminal"
)

type CommandHandler struct {
	sessionStore *session.Store
	commandStore *command.Store
	runner       *terminal.Runner
}

func NewCommandHandler(
	sessionStore *session.Store,
	commandStore *command.Store,
	runner *terminal.Runner,
) *CommandHandler {
	return &CommandHandler{
		sessionStore: sessionStore,
		commandStore: commandStore,
		runner: runner,
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

	result := h.runner.Run(r.Context(), req.Command, req.CWD, req.Timeout)

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

	logs, err := h.commandStore.ListBySession(r.Context(), sessionID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, logs)
}