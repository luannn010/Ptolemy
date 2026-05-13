package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/luannn010/ptolemy/internal/tasks"
)

type runInboxRequest struct {
	Dir      string `json:"dir"`
	MaxBatch int    `json:"max_batch"`
}

type runInboxResponse struct {
	OK        bool     `json:"ok"`
	Completed []string `json:"completed"`
	Failed    []string `json:"failed"`
	Blocked   []string `json:"blocked,omitempty"`
	Error     string   `json:"error,omitempty"`
}

type inboxExecutor struct{}

func (e inboxExecutor) Execute(task tasks.Task) error {
	return http.ErrNotSupported
}

type TasksHandler struct{}

func NewTasksHandler() *TasksHandler { return &TasksHandler{} }

func (h *TasksHandler) RunInbox(w http.ResponseWriter, r *http.Request) {
	var req runInboxRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid json"})
		return
	}
	if req.Dir == "" {
		req.Dir = "docs/tasks/inbox"
	}

	taskList, err := tasks.ScanInbox(req.Dir)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, runInboxResponse{OK: false, Error: err.Error()})
		return
	}

	state := tasks.NewMemoryStateStore()
	runner := tasks.BatchRunner{State: state, Executor: inboxExecutor{}, MaxBatch: req.MaxBatch}
	runErr := runner.RunInbox(taskList)

	resp := runInboxResponse{OK: runErr == nil}
	for _, t := range taskList {
		if s, ok := state.Get(t.ID); ok {
			if s == tasks.StatusCompleted {
				resp.Completed = append(resp.Completed, t.ID)
			}
			if s == tasks.StatusFailed {
				resp.Failed = append(resp.Failed, t.ID)
			}
		}
	}
	for _, t := range tasks.BlockedTasks(taskList, state) {
		resp.Blocked = append(resp.Blocked, t.ID)
	}
	if runErr != nil {
		resp.OK = false
		resp.Error = runErr.Error()
		writeJSON(w, http.StatusOK, resp)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
