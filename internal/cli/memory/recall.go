package memory

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/luannn010/ptolemy/internal/memory"
)

// setupLogging routes zerolog to stderr (stdout stays clean for the recalled
// context, which hooks pipe into the model). Default level is Info so the
// pipeline narrates every stage to the terminal; verbose drops to Debug;
// quiet silences everything but errors (use in hooks so injected context
// isn't polluted).
func setupLogging(verbose, quiet bool) {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	switch {
	case quiet:
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	case verbose:
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}

// recaller is the slice of *memory.Orchestrator the recall command needs, kept
// as an interface so the logic is unit-testable without a live DB.
type recaller interface {
	Answer(ctx context.Context, q memory.Query) (memory.Answer, error)
	Recall(ctx context.Context, q memory.Query) (memory.RecallResult, error)
}

type recallOpts struct {
	Query    string
	Subject  string
	Project  string
	K        int
	Generate bool // true → LLM-synthesized prose answer; false (default) → fast retrieval-only context
}

// dbConn is the subset of *pgx.Conn the CLI commands need for cleanup.
type dbConn interface {
	Close(context.Context) error
}

// RunRecall is the `ptolemy memory recall` (and `ptolemy-memory recall`)
// subcommand. It opens the memory module, runs a recall, and prints the
// recalled context plus sources to stdout.
func RunRecall(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("recall", flag.ContinueOnError)
	fs.SetOutput(stderr)
	query := fs.String("query", "", "what to recall (empty = recall this project's context)")
	subject := fs.String("subject", "", "owner scope")
	project := fs.String("project", "", "project scope")
	k := fs.Int("k", 0, "max results (0 = config default)")
	generate := fs.Bool("generate", false, "synthesize a prose answer via the LLM (slower); default returns retrieval-only context")
	verbose := fs.Bool("verbose", false, "extra debug detail")
	quiet := fs.Bool("quiet", false, "errors only (use in hooks; keeps piped context clean)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	setupLogging(*verbose, *quiet)

	orch, conn, err := openModule(ctx)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	sub, prj := scope(*subject, *project)
	return runRecall(ctx, orch, recallOpts{Query: *query, Subject: sub, Project: prj, K: *k, Generate: *generate}, stdout)
}

// runRecall builds the query (defaulting an empty query to a project-context
// recall) and prints the recalled context plus sources to out. By default it
// uses the fast retrieval-only path (no LLM); Generate opts into a synthesized
// prose answer.
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

	var body string
	var sources []string
	if opts.Generate {
		ans, err := r.Answer(ctx, q)
		if err != nil {
			return err
		}
		body, sources = ans.Text, ans.Citations
	} else {
		res, err := r.Recall(ctx, q)
		if err != nil {
			return err
		}
		body, sources = res.Context, res.SourceIDs
	}

	fmt.Fprintln(out, body)
	if len(sources) > 0 {
		fmt.Fprintf(out, "\nsources: %s\n", strings.Join(sources, ", "))
	}
	return nil
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

func openModule(ctx context.Context) (*memory.Orchestrator, dbConn, error) {
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
