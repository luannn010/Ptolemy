package memory

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/luannn010/ptolemy/internal/memory"
)

// RunDemo is the `ptolemy memory demo` subcommand: a small interactive harness
// over the memory module for ad-hoc ingest/ask (used by `make smoke-memory`).
// args are the words after "demo", e.g. ["ingest", "<doc-id>", "<file>"] or
// ["ask", "<question>"]. Argument shape is validated before the module is
// opened so usage errors don't require a live DB.
func RunDemo(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) < 1 {
		demoUsage(stderr)
		return fmt.Errorf("demo: missing subcommand")
	}
	switch args[0] {
	case "ingest":
		if len(args) != 3 {
			demoUsage(stderr)
			return fmt.Errorf("demo ingest: want <doc-id> <path-to-file>, got %d args", len(args)-1)
		}
	case "ask":
		if len(args) != 2 {
			demoUsage(stderr)
			return fmt.Errorf("demo ask: want \"<question>\", got %d args", len(args)-1)
		}
	default:
		demoUsage(stderr)
		return fmt.Errorf("demo: unknown subcommand %q", args[0])
	}

	orch, conn, err := openModule(ctx)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	switch args[0] {
	case "ingest":
		docID, path := args[1], args[2]
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		if err := orch.Ingest(ctx, memory.RawDocument{ID: docID, Source: path, Text: string(data)}); err != nil {
			return fmt.Errorf("ingest: %w", err)
		}
		fmt.Fprintf(stdout, "ingested %s from %s\n", docID, path)
	case "ask":
		ans, err := orch.Answer(ctx, memory.Query{Text: args[1]})
		if err != nil {
			return fmt.Errorf("answer: %w", err)
		}
		fmt.Fprintln(stdout, ans.Text)
		if len(ans.Citations) > 0 {
			fmt.Fprintf(stdout, "\nsources: %v\n", ans.Citations)
		}
	}
	return nil
}

func demoUsage(w io.Writer) {
	fmt.Fprintln(w, "usage:")
	fmt.Fprintln(w, "  ptolemy memory demo ingest <doc-id> <path-to-file>")
	fmt.Fprintln(w, "  ptolemy memory demo ask    \"<question>\"")
}
