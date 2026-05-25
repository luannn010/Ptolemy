package httpapi

import (
	"context"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/luannn010/ptolemy/internal/action"
	"github.com/luannn010/ptolemy/internal/agentloop"
	"github.com/luannn010/ptolemy/internal/approval"
	"github.com/luannn010/ptolemy/internal/command"
	"github.com/luannn010/ptolemy/internal/executor"
	"github.com/luannn010/ptolemy/internal/logging"
	"github.com/luannn010/ptolemy/internal/session"
	"github.com/luannn010/ptolemy/internal/terminal"
	"github.com/rs/zerolog/log"
)

type healthResponse struct {
	Status    string       `json:"status"`
	Service   string       `json:"service"`
	Timestamp string       `json:"timestamp"`
	Checks    healthChecks `json:"checks"`
}

type healthChecks struct {
	MCP     mcpHealthCheck     `json:"mcp"`
	Runtime runtimeHealthCheck `json:"runtime"`
}

type mcpHealthCheck struct {
	Enabled   bool   `json:"enabled"`
	Reachable bool   `json:"reachable"`
	LatencyMS int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
	Target    string `json:"target,omitempty"`
	Probe     string `json:"probe,omitempty"`
}

type runtimeHealthCheck struct {
	Context  string               `json:"context"`
	Strategy string               `json:"strategy"`
	Commands map[string]cmdHealth `json:"commands"`
}

type cmdHealth struct {
	Available bool   `json:"available"`
	Path      string `json:"path,omitempty"`
	Error     string `json:"error,omitempty"`
}

type HealthConfig struct {
	MCPBaseURL string
	Timeout    time.Duration
}

func NewRouter(
	sessionStore *session.Store,
	commandStore *command.Store,
	actionStore *action.Store,
	agentRunStore *agentloop.Store,
	agentRunService *agentloop.Service,
	logStore *logging.Store,
	approvalStore *approval.Store,
	runner *terminal.TmuxRunner,
	packStudioHandler *PackStudioHandler,
	healthCfg HealthConfig,
) http.Handler {
	r := chi.NewRouter()

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		checks := evaluateHealthChecks(r.Context(), r, healthCfg)
		status := "ok"
		if (checks.MCP.Enabled && !checks.MCP.Reachable) || hasMissingRuntimeCommand(checks.Runtime.Commands) {
			status = "degraded"
		}

		log.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Msg("health check called")

		writeJSON(w, http.StatusOK, healthResponse{
			Status:    status,
			Service:   "workerd",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Checks:    checks,
		})
	})

	exec := executor.NewExecutor(sessionStore, commandStore, actionStore, logStore, runner)
	executeHandler := NewExecuteHandler(exec)
	r.Post("/execute", executeHandler.Handle)

	sessionHandler := NewSessionHandler(sessionStore)
	r.Mount("/sessions", sessionHandler.Routes())

	commandHandler := NewCommandHandler(
		sessionStore,
		commandStore,
		actionStore,
		logStore,
		approvalStore,
		runner,
	)
	r.Mount("/sessions/{id}/commands", commandHandler.Routes())

	agentRunsHandler := NewAgentRunsHandler(agentRunStore, actionStore, agentRunService)
	r.Mount("/agent-runs", agentRunsHandler.Routes())

	fileHandler := NewFileHandler(sessionStore)

	r.Post("/file/read", fileHandler.Read)
	r.Post("/file/write", fileHandler.Write)
	r.Post("/file/list", fileHandler.List)
	r.Post("/file/search", fileHandler.Search)
	r.Post("/file/apply", fileHandler.Apply)
	r.Post("/file/replace_block", fileHandler.ReplaceBlock)
	r.Post("/file/insert_after", fileHandler.InsertAfter)

	navigatorHandler := NewNavigatorHandler(sessionStore)

	r.Post("/navigator/index", navigatorHandler.IndexWorkspace)
	r.Post("/navigator/context", navigatorHandler.ReadContext)
	r.Post("/navigator/session/start", navigatorHandler.StartTaskSession)
	r.Post("/navigator/session/note", navigatorHandler.AppendSessionNote)
	r.Post("/kb/build", navigatorHandler.IndexWorkspace)
	r.Post("/kb/read", navigatorHandler.ReadContext)
	r.Post("/kb/update", navigatorHandler.UpdateKnowledgeBase)
	r.Post("/kb/reset", navigatorHandler.ResetKnowledgeBase)

	gitHandler := NewGitHandler(sessionStore)

	r.Post("/git/status", gitHandler.Status)
	r.Post("/git/diff", gitHandler.Diff)
	r.Post("/git/log", gitHandler.Log)
	r.Post("/git/checkout", gitHandler.Checkout)
	r.Post("/git/branch", gitHandler.CreateBranch)
	r.Post("/git/commit", gitHandler.Commit)
	r.Post("/git/push", gitHandler.Push)
	r.Post("/git/generate-pr-body", gitHandler.GeneratePRBody)
	r.Post("/git/create-pr", gitHandler.CreatePR)

	worktreeHandler := NewWorktreeHandler(sessionStore)

	r.Post("/worktree/create", worktreeHandler.Create)
	r.Post("/worktree/list", worktreeHandler.List)
	r.Post("/worktree/remove", worktreeHandler.Remove)

	tasksHandler := NewTasksHandler()
	r.Post("/tasks/run-inbox", tasksHandler.RunInbox)

	if packStudioHandler != nil {
		r.Mount("/ui", packStudioHandler.Routes())
	}

	return r
}

func evaluateHealthChecks(ctx context.Context, r *http.Request, cfg HealthConfig) healthChecks {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 1500 * time.Millisecond
	}

	tool := strings.TrimSpace(r.URL.Query().Get("tool"))
	return healthChecks{
		MCP:     checkMCP(ctx, cfg.MCPBaseURL, timeout),
		Runtime: checkRuntimeCommands(tool),
	}
}

func checkMCP(ctx context.Context, baseURL string, timeout time.Duration) mcpHealthCheck {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return mcpHealthCheck{Enabled: false}
	}

	start := time.Now()
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, baseURL+"/health", nil)
	if err != nil {
		return mcpHealthCheck{Enabled: true, Reachable: false, Error: err.Error(), Target: baseURL, Probe: "/health"}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return mcpHealthCheck{Enabled: true, Reachable: false, Error: err.Error(), Target: baseURL, Probe: "/health"}
	}
	defer resp.Body.Close()

	check := mcpHealthCheck{
		Enabled:   true,
		Reachable: resp.StatusCode < 400,
		LatencyMS: time.Since(start).Milliseconds(),
		Target:    baseURL,
		Probe:     "/health",
	}
	if resp.StatusCode >= 400 {
		check.Error = resp.Status
	}
	return check
}

func checkRuntimeCommands(tool string) runtimeHealthCheck {
	required := commandsForTool(tool)
	result := runtimeHealthCheck{
		Context:  withDefault(tool, "generic"),
		Strategy: "static_allowlist_with_fallback",
		Commands: map[string]cmdHealth{},
	}
	for _, name := range required {
		path, err := resolveCommandPath(name)
		if err != nil {
			result.Commands[name] = cmdHealth{Available: false, Error: err.Error()}
		} else {
			result.Commands[name] = cmdHealth{Available: true, Path: path}
		}
	}
	return result
}

func resolveCommandPath(name string) (string, error) {
	candidates := []string{name}
	if name == "python" {
		candidates = append(candidates, "python3")
	}
	var lastErr error
	for _, candidate := range candidates {
		path, err := exec.LookPath(candidate)
		if err == nil {
			return path, nil
		}
		lastErr = err
	}
	return "", lastErr
}

func commandsForTool(tool string) []string {
	switch strings.TrimSpace(tool) {
	case "go", "ptolemy_kb_build", "ptolemy_kb_update":
		return []string{"go"}
	case "npm":
		return []string{"npm"}
	case "python":
		return []string{"python"}
	default:
		return []string{"go", "npm", "python"}
	}
}

func hasMissingRuntimeCommand(commands map[string]cmdHealth) bool {
	for _, cmd := range commands {
		if !cmd.Available {
			return true
		}
	}
	return false
}

func withDefault(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
