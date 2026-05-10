package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/luannn010/ptolemy/internal/config"
	"github.com/luannn010/ptolemy/internal/mcp"
	"github.com/luannn010/ptolemy/internal/mcp/executortools"
	"github.com/luannn010/ptolemy/internal/mcp/filetools"
	"github.com/luannn010/ptolemy/internal/mcp/gittools"
	"github.com/luannn010/ptolemy/internal/mcp/navigatortools"
	"github.com/luannn010/ptolemy/internal/mcp/sessiontools"
	"github.com/luannn010/ptolemy/internal/mcp/worktreetools"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	workerURL := firstNonEmpty(os.Getenv("PTOLEMY_WORKER_URL"), cfg.WorkerBaseURL)
	client := mcp.NewWorkerClient(workerURL)

	server := mcp.NewServer(
		client,
		sessiontools.Tools(),
		executortools.Tools(),
		filetools.Tools(),
		navigatortools.Tools(),
		gittools.Tools(),
		worktreetools.Tools(),
	)

	server.RegisterHandler(sessiontools.Handle)
	server.RegisterHandler(executortools.Handle)
	server.RegisterHandler(filetools.Handle)
	server.RegisterHandler(navigatortools.Handle)
	server.RegisterHandler(gittools.Handle)
	server.RegisterHandler(worktreetools.Handle)

	server.Run(os.Stdin, os.Stdout)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
