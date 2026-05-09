package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/luannn010/ptolemy/internal/action"
	"github.com/luannn010/ptolemy/internal/agentloop"
)

type AgentRunsHandler struct {
	store       *agentloop.Store
	actionStore *action.Store
	service     *agentloop.Service
}

type agentRunRequest struct {
	SessionID       string `json:"session_id"`
	TaskID          string `json:"task_id"`
	TaskFile        string `json:"task_file"`
	Workspace       string `json:"workspace"`
	Branch          string `json:"branch"`
	WorktreePath    string `json:"worktree_path"`
	FinalizationMode string `json:"finalization_mode"`
	MaxSteps        int    `json:"max_steps"`
	CurrentPhase    string `json:"current_phase"`
	FinalReportPath string `json:"final_report_path"`
	LastError       string `json:"last_error"`
	AutoStart       bool   `json:"auto_start"`
}

func NewAgentRunsHandler(store *agentloop.Store, actionStore *action.Store, service *agentloop.Service) *AgentRunsHandler {
	return &AgentRunsHandler{
		store:       store,
		actionStore: actionStore,
		service:     service,
	}
}

func (h *AgentRunsHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.createRun)
	r.Get("/{id}", h.getRun)
	r.Get("/{id}/actions", h.listActions)
	r.Get("/{id}/observations", h.listObservations)
	r.Post("/{id}/resume", h.resumeRun)
	r.Post("/{id}/cancel", h.cancelRun)
	return r
}

func (h *AgentRunsHandler) createRun(w http.ResponseWriter, r *http.Request) {
	var req agentRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	run, err := h.store.CreateRun(r.Context(), agentloop.Run{
		SessionID:       req.SessionID,
		TaskID:          req.TaskID,
		TaskFile:        req.TaskFile,
		Workspace:       req.Workspace,
		Branch:          req.Branch,
		WorktreePath:    req.WorktreePath,
		FinalizationMode: req.FinalizationMode,
		Status:          agentloop.StatusPending,
		MaxSteps:        req.MaxSteps,
		CurrentPhase:    req.CurrentPhase,
		FinalReportPath: req.FinalReportPath,
		LastError:       req.LastError,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if req.AutoStart && h.service != nil {
		run, err = h.service.StartOrResume(r.Context(), run.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}

	writeJSON(w, http.StatusCreated, run)
}

func (h *AgentRunsHandler) getRun(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	run, err := h.store.GetRun(r.Context(), id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, agentloop.ErrRunNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, run)
}

func (h *AgentRunsHandler) resumeRun(w http.ResponseWriter, r *http.Request) {
	if h.service != nil {
		run, err := h.service.StartOrResume(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, run)
		return
	}
	h.updateRunStatus(w, r, agentloop.StatusRunning)
}

func (h *AgentRunsHandler) cancelRun(w http.ResponseWriter, r *http.Request) {
	h.updateRunStatus(w, r, agentloop.StatusCancelled)
}

func (h *AgentRunsHandler) updateRunStatus(w http.ResponseWriter, r *http.Request, status string) {
	id := chi.URLParam(r, "id")

	run, err := h.store.GetRun(r.Context(), id)
	if err != nil {
		httpStatus := http.StatusInternalServerError
		if errors.Is(err, agentloop.ErrRunNotFound) {
			httpStatus = http.StatusNotFound
		}
		writeJSON(w, httpStatus, map[string]string{"error": err.Error()})
		return
	}

	run.Status = status
	updated, err := h.store.UpdateRun(r.Context(), run)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

func (h *AgentRunsHandler) listActions(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")

	if _, err := h.store.GetRun(r.Context(), runID); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, agentloop.ErrRunNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	actions, err := h.actionStore.ListByRun(r.Context(), runID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, actions)
}

func (h *AgentRunsHandler) listObservations(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")

	if _, err := h.store.GetRun(r.Context(), runID); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, agentloop.ErrRunNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	observations, err := h.store.ListObservations(r.Context(), runID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, observations)
}
