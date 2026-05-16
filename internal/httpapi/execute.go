package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/luannn010/ptolemy/internal/executor"
)

type ExecuteHandler struct {
	exec *executor.Executor
}

func NewExecuteHandler(exec *executor.Executor) *ExecuteHandler {
	return &ExecuteHandler{exec: exec}
}

func (h *ExecuteHandler) Handle(w http.ResponseWriter, r *http.Request) {
	var req executor.ExecuteRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid JSON",
		})
		return
	}

	if req.SessionID == "" || req.Command == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "session_id and command required",
		})
		return
	}

	resp, err := h.exec.Run(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}
