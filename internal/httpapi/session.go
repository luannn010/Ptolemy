package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/luannn010/ptolemy/internal/session"
)

type SessionHandler struct {
	store *session.Store
}

func NewSessionHandler(store *session.Store) *SessionHandler {
	return &SessionHandler{store: store}
}

func (h *SessionHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Post("/", h.createSession)
	r.Get("/", h.listSessions)
	r.Get("/{id}", h.getSession)
	r.Post("/{id}/close", h.closeSession)

	return r
}

func (h *SessionHandler) createSession(w http.ResponseWriter, r *http.Request) {
	var req session.CreateSessionRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid JSON body",
		})
		return
	}

	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "name is required",
		})
		return
	}

	if req.Workspace == "" {
		req.Workspace = "."
	}

	sess, err := h.store.Create(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusCreated, sess)
}

func (h *SessionHandler) listSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := h.store.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, sessions)
}

func (h *SessionHandler) getSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	sess, err := h.store.Get(r.Context(), id)
	if err != nil {
		status := http.StatusInternalServerError

		if errors.Is(err, session.ErrSessionNotFound) {
			status = http.StatusNotFound
		}

		writeJSON(w, status, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, sess)
}

func (h *SessionHandler) closeSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	sess, err := h.store.CloseSession(r.Context(), id)
	if err != nil {
		status := http.StatusInternalServerError

		if errors.Is(err, session.ErrSessionNotFound) {
			status = http.StatusNotFound
		}

		writeJSON(w, status, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, sess)
}