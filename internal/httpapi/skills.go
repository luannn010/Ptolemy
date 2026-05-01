package httpapi

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/luannn010/ptolemy/internal/skills"
)

type SkillsHandler struct {
	registry *skills.Registry
}

func NewSkillsHandler(registry *skills.Registry) *SkillsHandler {
	return &SkillsHandler{registry: registry}
}

func (h *SkillsHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.listSkills)
	r.Get("/*", h.getSkill)
	return r
}

func (h *SkillsHandler) listSkills(w http.ResponseWriter, r *http.Request) {
	items, err := h.registry.List()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *SkillsHandler) getSkill(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "*")
	doc, err := h.registry.Get(id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, skills.ErrSkillNotFound) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, doc)
}
