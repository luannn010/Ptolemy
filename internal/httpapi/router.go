package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/luannn010/ptolemy/internal/command"
	"github.com/luannn010/ptolemy/internal/session"
	"github.com/luannn010/ptolemy/internal/terminal"
	"github.com/rs/zerolog/log"
)

type healthResponse struct {
	Status    string `json:"status"`
	Service   string `json:"service"`
	Timestamp string `json:"timestamp"`
}

func NewRouter(
	sessionStore *session.Store,
	commandStore *command.Store,
	runner *terminal.Runner,
) http.Handler {
	r := chi.NewRouter()

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		log.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Msg("health check called")

		writeJSON(w, http.StatusOK, healthResponse{
			Status:    "ok",
			Service:   "workerd",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
	})

	sessionHandler := NewSessionHandler(sessionStore)
	r.Mount("/sessions", sessionHandler.Routes())

	commandHandler := NewCommandHandler(sessionStore, commandStore, runner)
	r.Mount("/sessions/{id}/commands", commandHandler.Routes())

	return r
}