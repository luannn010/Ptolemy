package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/luannn010/ptolemy/internal/fileops"
	"github.com/luannn010/ptolemy/internal/navigator"
	"github.com/luannn010/ptolemy/internal/session"
)

type FileHandler struct {
	sessionStore *session.Store
}

func NewFileHandler(sessionStore *session.Store) *FileHandler {
	return &FileHandler{sessionStore: sessionStore}
}

type fileRequest struct {
	SessionID     string `json:"session_id"`
	TaskSessionID string `json:"task_session_id"`
	Path          string `json:"path"`
	Content       string `json:"content"`
	Query         string `json:"query"`
}

// func (h *FileHandler) opsForSession(r *http.Request, sessionID string) (*fileops.FileOps, bool) {
// 	sess, err := h.sessionStore.Get(r.Context(), sessionID)
// 	if err != nil {
// 		writeJSON(nilSafeWriter{}, http.StatusNotFound, map[string]string{"error": err.Error()})
// 		return nil, false
// 	}

// 	return fileops.New(sess.Workspace), true
// }

func (h *FileHandler) Read(w http.ResponseWriter, r *http.Request) {
	var req fileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	ops, ok := h.getOps(w, r, req.SessionID)
	if !ok {
		return
	}

	content, err := ops.ReadFile(req.Path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.TaskSessionID != "" {
		if err := navigator.RecordFileRead(ops.BaseDir, req.TaskSessionID, req.Path); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"path":    req.Path,
		"content": content,
	})
}

func (h *FileHandler) Write(w http.ResponseWriter, r *http.Request) {
	var req fileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	ops, ok := h.getOps(w, r, req.SessionID)
	if !ok {
		return
	}

	if err := ops.WriteFile(req.Path, req.Content); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"path":    req.Path,
		"written": true,
	})
}

func (h *FileHandler) List(w http.ResponseWriter, r *http.Request) {
	var req fileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	ops, ok := h.getOps(w, r, req.SessionID)
	if !ok {
		return
	}

	entries, err := ops.ListDirectory(req.Path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"path":    req.Path,
		"entries": entries,
	})
}

func (h *FileHandler) Search(w http.ResponseWriter, r *http.Request) {
	var req fileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	ops, ok := h.getOps(w, r, req.SessionID)
	if !ok {
		return
	}

	result, err := ops.Search(req.Query)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error(), "output": result})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"query":  req.Query,
		"result": result,
	})
}

func (h *FileHandler) Apply(w http.ResponseWriter, r *http.Request) {
	var req fileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	ops, ok := h.getOps(w, r, req.SessionID)
	if !ok {
		return
	}

	if err := ops.ApplyPatch(req.Path, req.Content); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"path":    req.Path,
		"applied": true,
	})
}

func (h *FileHandler) getOps(w http.ResponseWriter, r *http.Request, sessionID string) (*fileops.FileOps, bool) {
	if sessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session_id is required"})
		return nil, false
	}

	sess, err := h.sessionStore.Get(r.Context(), sessionID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return nil, false
	}

	return fileops.New(sess.Workspace), true
}
