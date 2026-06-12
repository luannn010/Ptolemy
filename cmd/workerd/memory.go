package main

import (
	"context"
	"os"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/luannn010/ptolemy/internal/httpapi"
	"github.com/luannn010/ptolemy/internal/memory"
)

// Default subject/project scope when PTOLEMY_MEMORY_{SUBJECT,PROJECT} are unset.
// Mirrors cmd/ptolemy-mcp so HTTP and MCP callers land in the same memory scope.
const (
	defaultSubject = "default"
	defaultProject = "ptolemy"
)

// memoryScope resolves the subject/project used for /chat queries that omit them.
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

// buildRAGDeps wires the in-process memory module for the RAG HTTP listener.
// Returns ok=false (logging why) when memory is unconfigured or fails to start,
// so workerd serves its core duties without the RAG port. When ok=true, the
// caller must run cleanup() on shutdown — AFTER the RAG server has drained, so
// in-flight /chat requests keep their DB connection. The Answerer is serialized
// because NewModule's single *pgx.Conn is not safe under concurrent requests.
func buildRAGDeps(ctx context.Context) (deps httpapi.RAGDeps, cleanup func(), ok bool) {
	cfg, err := memory.LoadConfig()
	if err != nil {
		log.Warn().Err(err).Msg("rag http disabled: memory unconfigured")
		return httpapi.RAGDeps{}, nil, false
	}
	orch, conn, err := memory.NewModule(ctx, cfg)
	if err != nil {
		log.Warn().Err(err).Msg("rag http disabled: memory module failed to start")
		return httpapi.RAGDeps{}, nil, false
	}
	subject, project := memoryScope()
	deps = httpapi.RAGDeps{
		Answerer:       httpapi.NewSerialAnswerer(orch),
		DefaultSubject: subject,
		DefaultProject: project,
	}
	cleanup = func() { _ = conn.Close(context.Background()) }
	return deps, cleanup, true
}
