package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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
	SessionID  string `json:"session_id"`
	Branch     string `json:"branch"`
	Message    string `json:"message"`
	Remote     string `json:"remote"`
	Base       string `json:"base"`
	Head       string `json:"head"`
	Title      string `json:"title"`
	BodyPath   string `json:"body_path"`
	OutputPath string `json:"output_path"`
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

func (h *GitHandler) GeneratePRBody(w http.ResponseWriter, r *http.Request) {
	var req gitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.SessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session_id is required"})
		return
	}

	sess, err := h.sessionStore.Get(r.Context(), req.SessionID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	git := gitops.New(sess.Workspace)
	currentBranch := strings.TrimSpace(git.CurrentBranch(r.Context()).Output)
	if currentBranch == "" {
		currentBranch = "unknown"
	}
	base := strings.TrimSpace(req.Base)
	if base == "" {
		base = "main"
	}

	templatePath := filepath.Join(sess.Workspace, ".github", "pull_request_template.md")
	templateData, readErr := os.ReadFile(templatePath)
	if readErr != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "pull request template not found at .github/pull_request_template.md"})
		return
	}

	outputPath := strings.TrimSpace(req.OutputPath)
	if outputPath == "" {
		outputPath = filepath.ToSlash(filepath.Join(".github", "PRs", fmt.Sprintf("%s.md", strings.ReplaceAll(currentBranch, "/", "-"))))
	}
	absoluteOutputPath := filepath.Join(sess.Workspace, filepath.FromSlash(outputPath))
	if err := os.MkdirAll(filepath.Dir(absoluteOutputPath), 0o755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	changed := strings.TrimSpace(git.ChangedFiles(r.Context()).Output)
	diffStat := strings.TrimSpace(gitops.New(sess.Workspace).Diff(r.Context()).Output)
	if idx := strings.Index(diffStat, "--- DIFF EXCERPT ---"); idx >= 0 {
		diffStat = strings.TrimSpace(diffStat[:idx])
	}

	generated := strings.TrimSpace(string(templateData)) +
		"\n\n---\n\n" +
		"### Autofill Context (Local)\n" +
		fmt.Sprintf("- Branch: `%s`\n", currentBranch) +
		fmt.Sprintf("- Base: `%s`\n", base) +
		"- Changed files:\n" +
		formatIndentedList(changed) +
		"\n- Diff stat:\n" +
		formatIndentedList(diffStat) +
		"\n- Note: Edit sections above before opening or updating the PR.\n"

	if err := os.WriteFile(absoluteOutputPath, []byte(generated), 0o644); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"template_path": ".github/pull_request_template.md",
		"output_path":   filepath.ToSlash(outputPath),
		"branch":        currentBranch,
		"base":          base,
		"changed_files": strings.Fields(changed),
		"success":       true,
	})
}

func (h *GitHandler) CreatePR(w http.ResponseWriter, r *http.Request) {
	var req gitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.SessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session_id is required"})
		return
	}

	sess, err := h.sessionStore.Get(r.Context(), req.SessionID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	git := gitops.New(sess.Workspace)
	base := strings.TrimSpace(req.Base)
	if base == "" {
		base = "main"
	}
	head := strings.TrimSpace(req.Head)
	if head == "" {
		head = strings.TrimSpace(git.CurrentBranch(r.Context()).Output)
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = head
	}

	bodyPath := strings.TrimSpace(req.BodyPath)
	if bodyPath == "" {
		bodyPath = filepath.ToSlash(filepath.Join(".github", "PRs", fmt.Sprintf("%s.md", strings.ReplaceAll(head, "/", "-"))))
	}

	absoluteBodyPath := filepath.Join(sess.Workspace, filepath.FromSlash(bodyPath))
	if _, err := os.Stat(absoluteBodyPath); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body file does not exist: " + bodyPath})
		return
	}

	result := git.CreatePullRequest(r.Context(), base, head, title, absoluteBodyPath)
	writeJSON(w, http.StatusOK, map[string]any{
		"result":    result,
		"body_path": filepath.ToSlash(bodyPath),
		"success":   result.Success,
	})
}

func formatIndentedList(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "  - (none)\n"
	}

	lines := strings.Split(trimmed, "\n")
	var b strings.Builder
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		b.WriteString("  - ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		return "  - (none)\n"
	}
	return b.String()
}
