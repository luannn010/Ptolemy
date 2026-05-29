package main

import (
	"context"
	"fmt"
	"os"

	"github.com/luannn010/ptolemy/internal/config"
	"github.com/luannn010/ptolemy/internal/mcp"
	"github.com/luannn010/ptolemy/internal/mcp/executortools"
	"github.com/luannn010/ptolemy/internal/mcp/memorytools"
	"github.com/luannn010/ptolemy/internal/mcp/sessiontools"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	client := mcp.NewWorkerClient(cfg.WorkerBaseURL)

	toolGroups := [][]mcp.Tool{
		sessiontools.Tools(),
		executortools.Tools(),
	}

	// Memory is wired in-process (not via workerd); skip cleanly if unconfigured.
	memDeps, memCleanup, memOK := buildMemoryDeps(context.Background())
	if memOK {
		defer memCleanup()
		toolGroups = append(toolGroups, memorytools.Tools())
	}

	server := mcp.NewServer(client, toolGroups...)

	server.RegisterHandler(sessiontools.Handle)
	server.RegisterHandler(executortools.Handle)
	if memOK {
		server.RegisterHandler(memorytools.NewHandler(memDeps))
	}

	server.Run(os.Stdin, os.Stdout)
}
