package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/luannn010/ptolemy/internal/bootstrap"
)

type BootstrapHandler struct{}

func NewBootstrapHandler() *BootstrapHandler {
	return &BootstrapHandler{}
}

func (h *BootstrapHandler) Handle(w http.ResponseWriter, r *http.Request) {
	req := bootstrap.Request{}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
	}

	writeJSON(w, http.StatusOK, bootstrap.Build(req))
}
