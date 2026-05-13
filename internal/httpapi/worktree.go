package httpapi

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/luannn010/ptolemy/internal/session"
	"github.com/luannn010/ptolemy/internal/worktree"
)

type WorktreeHandler struct {
	sessionStore *session.Store
}

func NewWorktreeHandler(sessionStore *session.Store) *WorktreeHandler {
	return &WorktreeHandler{sessionStore: sessionStore}
}

type worktreeRequest struct {
	SessionID string `json:"session_id"`
	Name      string `json:"name"`
	Branch    string `json:"branch"`
}

func (h *WorktreeHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req worktreeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}

	sess, err := h.sessionStore.Get(r.Context(), req.SessionID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	repoRoot := resolveMainRepoRoot(sess.Workspace)

	manager := worktree.NewManager(
		repoRoot,
		filepath.Join(repoRoot, ".ptolemy-worktrees"),
	)

	result := manager.Create(r.Context(), req.Name, req.Branch)
	if result.Success {
		sess.Workspace = result.Worktree
		_, _ = h.sessionStore.Update(r.Context(), sess)
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *WorktreeHandler) List(w http.ResponseWriter, r *http.Request) {
	var req worktreeRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	sess, err := h.sessionStore.Get(r.Context(), req.SessionID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	repoRoot := resolveMainRepoRoot(sess.Workspace)

	manager := worktree.NewManager(
		repoRoot,
		filepath.Join(repoRoot, ".ptolemy-worktrees"),
	)

	result := manager.List(r.Context())
	writeJSON(w, http.StatusOK, result)
}

func (h *WorktreeHandler) Remove(w http.ResponseWriter, r *http.Request) {
	var req worktreeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}

	sess, err := h.sessionStore.Get(r.Context(), req.SessionID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	repoRoot := resolveMainRepoRoot(sess.Workspace)

	manager := worktree.NewManager(
		repoRoot,
		filepath.Join(repoRoot, ".ptolemy-worktrees"),
	)

	result := manager.Remove(r.Context(), req.Name)
	writeJSON(w, http.StatusOK, result)
}

func resolveMainRepoRoot(workspace string) string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = workspace

	out, err := cmd.Output()
	if err != nil {
		return workspace
	}

	root := strings.TrimSpace(string(out))

	if strings.Contains(root, ".ptolemy-worktrees") {
		return filepath.Dir(filepath.Dir(root))
	}

	return root
}
