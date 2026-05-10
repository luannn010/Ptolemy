package httpapi

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/luannn010/ptolemy/internal/action"
	"github.com/luannn010/ptolemy/internal/agentloop"
	"github.com/luannn010/ptolemy/internal/command"
	"github.com/luannn010/ptolemy/internal/packstudio"
)

//go:embed ui/*
var packStudioAssets embed.FS

type PackStudioHandler struct {
	service *packstudio.Service
	assets  fs.FS
}

func NewPackStudioHandler(service *packstudio.Service) *PackStudioHandler {
	sub, err := fs.Sub(packStudioAssets, "ui")
	if err != nil {
		sub = packStudioAssets
	}
	return &PackStudioHandler{service: service, assets: sub}
}

func (h *PackStudioHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.serveShell)
	r.Get("/studio", h.serveShell)
	r.Get("/overview", h.serveShell)
	r.Get("/runs/{id}", h.serveShell)
	r.Handle("/assets/*", http.StripPrefix("/ui/assets/", http.FileServer(http.FS(h.assets))))

	r.Get("/api/overview", h.overview)
	r.Get("/api/packs", h.listPacks)
	r.Get("/api/packs/{id}", h.getPack)
	r.Get("/api/packs/{id}/plan", h.planPack)
	r.Post("/api/packs", h.createPack)
	r.Get("/api/programs", h.listPrograms)
	r.Get("/api/programs/{id}", h.getProgram)
	r.Post("/api/programs", h.createProgram)
	r.Get("/api/program-runs", h.listProgramRuns)
	r.Post("/api/program-runs", h.createProgramRun)
	r.Get("/api/program-runs/{id}", h.getProgramRun)
	r.Get("/api/program-runs/{id}/tree", h.getProgramRun)
	r.Get("/api/program-runs/{id}/events", h.getRunEvents)
	r.Post("/api/program-runs/{id}/cancel", h.cancelProgramRun)
	r.Get("/api/program-runs/{id}/events/stream", h.streamRunEvents)
	r.Get("/api/program-runs/{id}/terminal/stream", h.streamTerminal)
	r.Get("/api/artifact", h.getArtifact)
	return r
}

func (h *PackStudioHandler) serveShell(w http.ResponseWriter, r *http.Request) {
	data, err := fs.ReadFile(h.assets, "index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func (h *PackStudioHandler) overview(w http.ResponseWriter, r *http.Request) {
	packs, packErr := packstudio.ListPacks(h.service.Root())
	programs, programErr := packstudio.ListPrograms(h.service.Root())
	runs, runErr := h.service.Store().ListProgramRuns(r.Context(), 20)
	if packErr != nil || programErr != nil || runErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"packs_error":    errorString(packErr),
			"programs_error": errorString(programErr),
			"runs_error":     errorString(runErr),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"packs":    packs,
		"programs": programs,
		"runs":     runs,
	})
}

func (h *PackStudioHandler) listPacks(w http.ResponseWriter, r *http.Request) {
	items, err := packstudio.ListPacks(h.service.Root())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *PackStudioHandler) getPack(w http.ResponseWriter, r *http.Request) {
	detail, err := packstudio.GetPack(h.service.Root(), chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h *PackStudioHandler) planPack(w http.ResponseWriter, r *http.Request) {
	detail, err := packstudio.GetPack(h.service.Root(), chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	ordered := make([]string, 0, len(detail.Tasks))
	for _, task := range detail.Tasks {
		ordered = append(ordered, task.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"pack_id": detail.ID,
		"tasks":   ordered,
	})
}

func (h *PackStudioHandler) createPack(w http.ResponseWriter, r *http.Request) {
	var input packstudio.CreatePackInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	detail, err := h.service.CreatePack(input)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, detail)
}

func (h *PackStudioHandler) listPrograms(w http.ResponseWriter, r *http.Request) {
	items, err := packstudio.ListPrograms(h.service.Root())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *PackStudioHandler) getProgram(w http.ResponseWriter, r *http.Request) {
	definition, validationErrs, err := packstudio.GetProgram(h.service.Root(), chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"program":           definition,
		"validation_errors": validationErrs,
	})
}

func (h *PackStudioHandler) createProgram(w http.ResponseWriter, r *http.Request) {
	var input packstudio.CreateProgramInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	definition, err := h.service.CreateProgram(input)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, definition)
}

func (h *PackStudioHandler) createProgramRun(w http.ResponseWriter, r *http.Request) {
	var input packstudio.StartProgramRunInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	run, err := h.service.StartProgramRun(r.Context(), input)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

func (h *PackStudioHandler) listProgramRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := h.service.Store().ListProgramRuns(r.Context(), 50)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (h *PackStudioHandler) getProgramRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	detail, err := h.service.BuildProgramRunDetail(r.Context(), runID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, packstudio.ErrProgramRunNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	task, actions, observations, commandLogs, loadErr := h.service.CurrentTaskDetail(r.Context(), runID)
	if loadErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": loadErr.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"detail":       detail,
		"current_task": task,
		"actions":      actions,
		"observations": observations,
		"command_logs": commandLogs,
	})
}

func (h *PackStudioHandler) getRunEvents(w http.ResponseWriter, r *http.Request) {
	events, err := h.service.Store().ListEvents(r.Context(), chi.URLParam(r, "id"), 400)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (h *PackStudioHandler) cancelProgramRun(w http.ResponseWriter, r *http.Request) {
	if err := h.service.CancelProgramRun(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *PackStudioHandler) streamRunEvents(w http.ResponseWriter, r *http.Request) {
	programRunID := chi.URLParam(r, "id")
	prepareSSE(w)
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	lastCount := 0
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			events, err := h.service.Store().ListEvents(r.Context(), programRunID, 400)
			if err != nil {
				writeSSEEvent(w, "error", map[string]string{"error": err.Error()})
				flusher.Flush()
				return
			}
			if len(events) <= lastCount {
				writeSSEEvent(w, "heartbeat", map[string]string{"status": "ok"})
				flusher.Flush()
				continue
			}
			for _, event := range events[lastCount:] {
				writeSSEEvent(w, "event", event)
			}
			lastCount = len(events)
			flusher.Flush()
		}
	}
}

func (h *PackStudioHandler) streamTerminal(w http.ResponseWriter, r *http.Request) {
	programRunID := chi.URLParam(r, "id")
	prepareSSE(w)
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	lastSessionID := ""
	lastSnapshot := ""
	ticker := time.NewTicker(700 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			task, _, _, _, err := h.service.CurrentTaskDetail(r.Context(), programRunID)
			if err != nil {
				writeSSEEvent(w, "error", map[string]string{"error": err.Error()})
				flusher.Flush()
				return
			}
			if task == nil || strings.TrimSpace(task.SessionID) == "" {
				writeSSEEvent(w, "heartbeat", map[string]string{"status": "waiting"})
				flusher.Flush()
				continue
			}
			if task.SessionID != lastSessionID {
				lastSessionID = task.SessionID
				lastSnapshot = ""
				writeSSEEvent(w, "meta", map[string]string{
					"session_id": task.SessionID,
					"task_id":    task.TaskID,
					"phase":      task.Status,
				})
			}
			snapshot, err := h.service.Runner().CaptureSession(context.Background(), task.SessionID)
			if err != nil {
				writeSSEEvent(w, "meta", map[string]string{
					"session_id": task.SessionID,
					"task_id":    task.TaskID,
					"phase":      "waiting-for-pane",
				})
				writeSSEEvent(w, "terminal", map[string]string{
					"mode":       "replace",
					"content":    "Waiting for live terminal output.\nThe agent session exists, but no tmux pane is available to capture yet.\nThis usually means the run has not executed a shell command yet, or the session bootstrap failed.\n",
					"session_id": task.SessionID,
					"task_id":    task.TaskID,
				})
				flusher.Flush()
				continue
			}
			if snapshot == lastSnapshot {
				writeSSEEvent(w, "heartbeat", map[string]string{"status": "unchanged"})
				flusher.Flush()
				continue
			}

			mode := "replace"
			content := snapshot
			if strings.HasPrefix(snapshot, lastSnapshot) {
				mode = "append"
				content = snapshot[len(lastSnapshot):]
			}
			lastSnapshot = snapshot
			writeSSEEvent(w, "terminal", map[string]string{
				"mode":       mode,
				"content":    content,
				"session_id": task.SessionID,
				"task_id":    task.TaskID,
			})
			flusher.Flush()
		}
	}
}

func (h *PackStudioHandler) getArtifact(w http.ResponseWriter, r *http.Request) {
	rawPath := strings.TrimSpace(r.URL.Query().Get("path"))
	if rawPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path is required"})
		return
	}
	cleanPath := filepath.Clean(rawPath)
	if !filepath.IsAbs(cleanPath) {
		cleanPath = filepath.Join(h.service.Root(), cleanPath)
	}
	allowedRoots := []string{
		filepath.Join(h.service.Root(), ".state", "agent-runs"),
		filepath.Join(h.service.Root(), ".state", "pack-studio"),
	}
	if !isSafeArtifactPath(cleanPath, allowedRoots) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "artifact path is outside allowed roots"})
		return
	}
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(data)
}

func prepareSSE(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
}

func writeSSEEvent(w http.ResponseWriter, event string, payload any) {
	data, _ := json.Marshal(payload)
	_, _ = w.Write([]byte("event: " + event + "\n"))
	_, _ = w.Write([]byte("data: " + string(data) + "\n\n"))
}

func isSafeArtifactPath(path string, roots []string) bool {
	path = filepath.Clean(path)
	for _, root := range roots {
		root = filepath.Clean(root)
		if path == root || strings.HasPrefix(path, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func commandLogsForSession(store *command.Store, ctx context.Context, sessionID string) []command.CommandLog {
	if sessionID == "" {
		return []command.CommandLog{}
	}
	logs, err := store.ListBySession(ctx, sessionID)
	if err != nil {
		return []command.CommandLog{}
	}
	return logs
}

func actionsForRun(store *action.Store, ctx context.Context, runID string) []action.Action {
	if runID == "" {
		return []action.Action{}
	}
	actions, err := store.ListByRun(ctx, runID)
	if err != nil {
		return []action.Action{}
	}
	return actions
}

func observationsForRun(store *agentloop.Store, ctx context.Context, runID string) []agentloop.Observation {
	if runID == "" {
		return []agentloop.Observation{}
	}
	observations, err := store.ListObservations(ctx, runID)
	if err != nil {
		return []agentloop.Observation{}
	}
	return observations
}
