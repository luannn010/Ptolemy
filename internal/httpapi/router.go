package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/luannn010/ptolemy/internal/action"
	"github.com/luannn010/ptolemy/internal/command"
	"github.com/luannn010/ptolemy/internal/executor"
	"github.com/luannn010/ptolemy/internal/logs"
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
	actionStore *action.Store,
	logStore *logs.Store,
	runner *terminal.TmuxRunner,
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

	exec := executor.NewExecutor(sessionStore, commandStore, runner)
	executeHandler := NewExecuteHandler(exec)
	r.Post("/execute", executeHandler.Handle)

	sessionHandler := NewSessionHandler(sessionStore)
	r.Mount("/sessions", sessionHandler.Routes())

	commandHandler := NewCommandHandler(
		sessionStore,
		commandStore,
		actionStore,
		logStore,
		runner,
	)
	r.Mount("/sessions/{id}/commands", commandHandler.Routes())

	fileHandler := NewFileHandler(sessionStore)

	r.Post("/file/read", fileHandler.Read)
	r.Post("/file/write", fileHandler.Write)
	r.Post("/file/list", fileHandler.List)
	r.Post("/file/search", fileHandler.Search)
	r.Post("/file/apply", fileHandler.Apply)

	gitHandler := NewGitHandler(sessionStore)

	r.Post("/git/status", gitHandler.Status)
	r.Post("/git/diff", gitHandler.Diff)
	r.Post("/git/log", gitHandler.Log)
	r.Post("/git/checkout", gitHandler.Checkout)
	r.Post("/git/branch", gitHandler.CreateBranch)
	r.Post("/git/commit", gitHandler.Commit)
	r.Post("/git/push", gitHandler.Push)

	worktreeHandler := NewWorktreeHandler(sessionStore)

	r.Post("/worktree/create", worktreeHandler.Create)
	r.Post("/worktree/list", worktreeHandler.List)
	r.Post("/worktree/remove", worktreeHandler.Remove)

	return r
}
