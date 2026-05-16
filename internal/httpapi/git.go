package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/luannn010/ptolemy/internal/gitops"
	"github.com/luannn010/ptolemy/internal/session"
)

type GitHandler struct {
	sessionStore *session.Store
}

func NewGitHandler(sessionStore *session.Store) *GitHandler {
	return &GitHandler{sessionStore: sessionStore}
}

type gitRequest struct {
	SessionID string `json:"session_id"`
	Branch    string `json:"branch"`
	Message   string `json:"message"`
	Remote    string `json:"remote"`
}

func (h *GitHandler) gitForSession(w http.ResponseWriter, r *http.Request, sessionID string) (*gitops.GitOps, bool) {
	if sessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session_id is required"})
		return nil, false
	}

	sess, err := h.sessionStore.Get(r.Context(), sessionID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return nil, false
	}

	return gitops.New(sess.Workspace), true
}

func (h *GitHandler) Status(w http.ResponseWriter, r *http.Request) {
	var req gitRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	git, ok := h.gitForSession(w, r, req.SessionID)
	if !ok {
		return
	}

	writeJSON(w, http.StatusOK, git.Status(r.Context()))
}

func (h *GitHandler) Diff(w http.ResponseWriter, r *http.Request) {
	var req gitRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	git, ok := h.gitForSession(w, r, req.SessionID)
	if !ok {
		return
	}

	writeJSON(w, http.StatusOK, git.Diff(r.Context()))
}

func (h *GitHandler) Log(w http.ResponseWriter, r *http.Request) {
	var req gitRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	git, ok := h.gitForSession(w, r, req.SessionID)
	if !ok {
		return
	}

	writeJSON(w, http.StatusOK, git.Log(r.Context()))
}

func (h *GitHandler) Checkout(w http.ResponseWriter, r *http.Request) {
	var req gitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	git, ok := h.gitForSession(w, r, req.SessionID)
	if !ok {
		return
	}

	writeJSON(w, http.StatusOK, git.Checkout(r.Context(), req.Branch))
}

func (h *GitHandler) CreateBranch(w http.ResponseWriter, r *http.Request) {
	var req gitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	git, ok := h.gitForSession(w, r, req.SessionID)
	if !ok {
		return
	}

	writeJSON(w, http.StatusOK, git.CreateBranch(r.Context(), req.Branch))
}

func (h *GitHandler) Commit(w http.ResponseWriter, r *http.Request) {
	var req gitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	git, ok := h.gitForSession(w, r, req.SessionID)
	if !ok {
		return
	}

	writeJSON(w, http.StatusOK, git.CommitConventional(r.Context(), req.Message))
}

func (h *GitHandler) Push(w http.ResponseWriter, r *http.Request) {
	var req gitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	git, ok := h.gitForSession(w, r, req.SessionID)
	if !ok {
		return
	}

	writeJSON(w, http.StatusOK, git.Push(r.Context(), req.Remote, req.Branch))
}
