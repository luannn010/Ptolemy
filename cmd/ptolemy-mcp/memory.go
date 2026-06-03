package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/luannn010/ptolemy/internal/mcp/memorytools"
	"github.com/luannn010/ptolemy/internal/memory"
)

// Default subject/project scope when PTOLEMY_MEMORY_{SUBJECT,PROJECT} are unset.
const (
	defaultSubject = "default"
	defaultProject = "ptolemy"
)

// memoryScope resolves the subject/project used for recall/capture/consolidate
// when a tool call omits them.
func memoryScope() (subject, project string) {
	subject = strings.TrimSpace(os.Getenv("PTOLEMY_MEMORY_SUBJECT"))
	if subject == "" {
		subject = defaultSubject
	}
	project = strings.TrimSpace(os.Getenv("PTOLEMY_MEMORY_PROJECT"))
	if project == "" {
		project = defaultProject
	}
	return subject, project
}

// buildMemoryDeps wires the in-process memory dependencies for the MCP tools.
// It returns ok=false (and logs to stderr) when memory is unconfigured or fails
// to start, so the MCP server still serves its other tool groups. When ok=true,
// the caller must defer cleanup() to close the DB connection on shutdown.
func buildMemoryDeps(ctx context.Context) (deps memorytools.Deps, cleanup func(), ok bool) {
	cfg, err := memory.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "memory disabled: %v\n", err)
		return memorytools.Deps{}, nil, false
	}
	orch, conn, err := memory.NewModule(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "memory disabled: %v\n", err)
		return memorytools.Deps{}, nil, false
	}
	hook := memory.NewCaptureHookFromConfig(cfg, orch.Store, orch.Embedder)
	cons := memory.NewConsolidator(conn, orch.Store,
		memory.NewOpenAIGenerator(cfg.LLMBaseURL, cfg.LLMModel, ""), cfg.Consolidate).
		WithEmbedder(orch.Embedder)

	subject, project := memoryScope()
	deps = memorytools.Deps{
		Recall:      orch,
		Capture:     hook,
		Consolidate: cons,
		Subject:     subject,
		Project:     project,
	}
	cleanup = func() { _ = conn.Close(context.Background()) }
	return deps, cleanup, true
}
