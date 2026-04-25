package main

import (
	"os"

	"github.com/luannn010/ptolemy/internal/mcp"
	"github.com/luannn010/ptolemy/internal/mcp/executortools"
	"github.com/luannn010/ptolemy/internal/mcp/filetools"
	"github.com/luannn010/ptolemy/internal/mcp/gittools"
	"github.com/luannn010/ptolemy/internal/mcp/sessiontools"
	"github.com/luannn010/ptolemy/internal/mcp/worktreetools"
)

func main() {
	workerURL := os.Getenv("PTOLEMY_WORKER_URL")
	if workerURL == "" {
		workerURL = "http://localhost:8080"
	}

	client := mcp.NewWorkerClient(workerURL)

	server := mcp.NewServer(
		client,
		sessiontools.Tools(),
		executortools.Tools(),
		filetools.Tools(),
		gittools.Tools(),
		worktreetools.Tools(),
	)

	server.RegisterHandler(sessiontools.Handle)
	server.RegisterHandler(executortools.Handle)
	server.RegisterHandler(filetools.Handle)
	server.RegisterHandler(gittools.Handle)
	server.RegisterHandler(worktreetools.Handle)

	server.Run(os.Stdin, os.Stdout)
}
