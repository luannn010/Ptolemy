package main

import (
	"fmt"
	"os"

	"github.com/luannn010/ptolemy/internal/config"
	"github.com/luannn010/ptolemy/internal/mcp"
	"github.com/luannn010/ptolemy/internal/mcp/executortools"
	"github.com/luannn010/ptolemy/internal/mcp/sessiontools"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	client := mcp.NewWorkerClient(cfg.WorkerBaseURL)

	server := mcp.NewServer(
		client,
		sessiontools.Tools(),
		executortools.Tools(),
	)

	server.RegisterHandler(sessiontools.Handle)
	server.RegisterHandler(executortools.Handle)

	server.Run(os.Stdin, os.Stdout)
}
