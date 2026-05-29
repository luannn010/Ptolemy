// Command ptolemy-memory is a thin CLI over internal/memory for use by Claude
// Code / Codex hooks (SessionStart recall, Stop capture). It shares the same
// memory.NewModule wiring as the MCP server; hooks are shell commands, not MCP
// callers, so they need a binary entry point.
//
// Usage:
//
//	ptolemy-memory recall  [--query Q] [--subject S] [--project P] [--k N]
//	ptolemy-memory capture --user TEXT --assistant TEXT [--subject S] [--project P] [--session S]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	// autoload reads .env at startup so memory.LoadConfig() picks up
	// DATABASE_URL, EMBEDDING_*, BRAIN_* without the caller (e.g. a hook
	// shell) having to export them.
	_ "github.com/joho/godotenv/autoload"

	"github.com/luannn010/ptolemy/internal/memory"
)

// recaller / capturer are the slices of *memory.Orchestrator and
// *memory.PerTurnCaptureHook the CLI needs, kept as interfaces so the
// command logic is unit-testable without a live DB.
type recaller interface {
	Answer(ctx context.Context, q memory.Query) (memory.Answer, error)
}

type capturer interface {
	Capture(ctx context.Context, ex memory.Exchange) error
}

type recallOpts struct {
	Query   string
	Subject string
	Project string
	K       int
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: ptolemy-memory <recall|capture> [flags]")
		os.Exit(2)
	}
	ctx := context.Background()
	switch os.Args[1] {
	case "recall":
		os.Exit(cmdRecall(ctx, os.Args[2:]))
	case "capture":
		os.Exit(cmdCapture(ctx, os.Args[2:]))
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q (want recall|capture)\n", os.Args[1])
		os.Exit(2)
	}
}

func cmdRecall(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("recall", flag.ExitOnError)
	query := fs.String("query", "", "what to recall (empty = recall this project's context)")
	subject := fs.String("subject", "", "owner scope")
	project := fs.String("project", "", "project scope")
	k := fs.Int("k", 0, "max results (0 = config default)")
	_ = fs.Parse(args)

	orch, conn, err := openModule(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	defer conn.Close(ctx)

	sub, prj := scope(*subject, *project)
	if err := runRecall(ctx, orch, recallOpts{Query: *query, Subject: sub, Project: prj, K: *k}, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "recall: %v\n", err)
		return 1
	}
	return 0
}

func cmdCapture(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("capture", flag.ExitOnError)
	user := fs.String("user", "", "user message text")
	assistant := fs.String("assistant", "", "assistant reply text")
	subject := fs.String("subject", "", "owner scope")
	project := fs.String("project", "", "project scope")
	session := fs.String("session", "", "session id")
	_ = fs.Parse(args)

	hook, conn, err := openCapture(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	defer conn.Close(ctx)

	sub, prj := scope(*subject, *project)
	ex := memory.Exchange{
		UserText: *user, AssistantText: *assistant,
		SubjectID: sub, ProjectID: prj, SessionID: *session,
	}
	if err := runCapture(ctx, hook, ex); err != nil {
		fmt.Fprintf(os.Stderr, "capture: %v\n", err)
		return 1
	}
	return 0
}

// runRecall builds the query (defaulting an empty --query to a project-context
// recall) and prints the synthesis summary plus citations to out.
func runRecall(ctx context.Context, r recaller, opts recallOpts, out io.Writer) error {
	text := strings.TrimSpace(opts.Query)
	if text == "" {
		text = "Summarize the durable context, decisions, and facts for this project."
	}
	q := memory.Query{Text: text, K: opts.K}
	if opts.Subject != "" {
		q.SubjectID = &opts.Subject
	}
	if opts.Project != "" {
		q.ProjectID = &opts.Project
	}
	ans, err := r.Answer(ctx, q)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, ans.Text)
	if len(ans.Citations) > 0 {
		fmt.Fprintf(out, "\nsources: %s\n", strings.Join(ans.Citations, ", "))
	}
	return nil
}

// runCapture validates the exchange has text and captures it synchronously.
func runCapture(ctx context.Context, c capturer, ex memory.Exchange) error {
	if strings.TrimSpace(ex.UserText) == "" && strings.TrimSpace(ex.AssistantText) == "" {
		return errors.New("provide at least one of --user or --assistant")
	}
	return c.Capture(ctx, ex)
}

// scope applies the PTOLEMY_MEMORY_{SUBJECT,PROJECT} env defaults when a flag
// is empty, with a final hard default so capture/recall always have a scope.
func scope(subject, project string) (string, string) {
	if strings.TrimSpace(subject) == "" {
		subject = strings.TrimSpace(os.Getenv("PTOLEMY_MEMORY_SUBJECT"))
	}
	if subject == "" {
		subject = "default"
	}
	if strings.TrimSpace(project) == "" {
		project = strings.TrimSpace(os.Getenv("PTOLEMY_MEMORY_PROJECT"))
	}
	if project == "" {
		project = "ptolemy"
	}
	return subject, project
}

func openModule(ctx context.Context) (*memory.Orchestrator, interface {
	Close(context.Context) error
}, error) {
	cfg, err := memory.LoadConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("config: %w", err)
	}
	orch, conn, err := memory.NewModule(ctx, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("module: %w", err)
	}
	return orch, conn, nil
}

func openCapture(ctx context.Context) (*memory.PerTurnCaptureHook, interface {
	Close(context.Context) error
}, error) {
	cfg, err := memory.LoadConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("config: %w", err)
	}
	orch, conn, err := memory.NewModule(ctx, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("module: %w", err)
	}
	hook := memory.NewCaptureHookFromConfig(cfg, orch.Store, orch.Embedder)
	return hook, conn, nil
}
