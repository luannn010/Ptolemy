package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/luannn010/ptolemy/internal/navigator"
	"github.com/luannn010/ptolemy/internal/session"
)

type NavigatorHandler struct {
	sessionStore *session.Store
}

type navigatorRequest struct {
	SessionID     string `json:"session_id"`
	Workspace     string `json:"workspace"`
	TaskSessionID string `json:"task_session_id"`
	Task          string `json:"task"`
	Note          string `json:"note"`
}

func NewNavigatorHandler(sessionStore *session.Store) *NavigatorHandler {
	return &NavigatorHandler{sessionStore: sessionStore}
}

func (h *NavigatorHandler) IndexWorkspace(w http.ResponseWriter, r *http.Request) {
	var req navigatorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	workspace, ok := h.workspaceForRequest(w, r, req)
	if !ok {
		return
	}

	result, err := navigator.IndexWorkspace(workspace)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *NavigatorHandler) ReadContext(w http.ResponseWriter, r *http.Request) {
	var req navigatorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	workspace, ok := h.workspaceForRequest(w, r, req)
	if !ok {
		return
	}

	files, err := navigator.ReadContext(workspace)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"workspace": workspace,
		"files":     files,
	})
}

func (h *NavigatorHandler) StartTaskSession(w http.ResponseWriter, r *http.Request) {
	var req navigatorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	workspace, ok := h.workspaceForRequest(w, r, req)
	if !ok {
		return
	}

	result, err := navigator.StartTaskSession(workspace, req.TaskSessionID, req.Task)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *NavigatorHandler) AppendSessionNote(w http.ResponseWriter, r *http.Request) {
	var req navigatorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	workspace, ok := h.workspaceForRequest(w, r, req)
	if !ok {
		return
	}

	result, err := navigator.AppendSessionNote(workspace, req.TaskSessionID, req.Note)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *NavigatorHandler) workspaceForRequest(w http.ResponseWriter, r *http.Request, req navigatorRequest) (string, bool) {
	if req.SessionID == "" {
		if req.Workspace == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session_id or workspace is required"})
			return "", false
		}
		return req.Workspace, true
	}

	sess, err := h.sessionStore.Get(r.Context(), req.SessionID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, session.ErrSessionNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return "", false
	}

	return sess.Workspace, true
}
