// memory-synth-eval drives the Phase 6b multi-session synthesis eval: for each
// scenario it captures every turn synchronously through the live extractor,
// consolidates the (subject,project), recalls scoped to it, and scores the
// recalled synthesis summary against expect/reject keywords. Intended for
// `make eval-synth`. Uses the live BRAIN LLM, so it is NOT a `go test` target.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	// .env autoload (matches cmd/memory-eval).
	_ "github.com/joho/godotenv/autoload"

	"github.com/luannn010/ptolemy/internal/memory"
	"github.com/luannn010/ptolemy/internal/memory/eval"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("memory-synth-eval", flag.ContinueOnError)
	fs.SetOutput(stderr)
	scenariosPath := fs.String("scenarios", "internal/memory/eval/testdata/synth_scenarios.json", "path to synthesis scenarios JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := memory.LoadConfig()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if evalURL := strings.TrimSpace(os.Getenv("MEMORY_EVAL_DATABASE_URL")); evalURL != "" {
		cfg.DatabaseURL = evalURL // eval runs against a dedicated DB so unit-test freshDB (dim=4) and eval (dim=768) stop clobbering each other
	}
	// Small seed scenarios must consolidate even though they hold far fewer atoms
	// than the production MinAtoms default; force it to 1 for the eval.
	cfg.Consolidate.MinAtoms = 1

	ctx := context.Background()
	orch, conn, err := memory.NewModule(ctx, cfg)
	if err != nil {
		return fmt.Errorf("module: %w", err)
	}
	defer conn.Close(ctx)

	store := memory.NewPgStore(conn)
	hook := memory.NewCaptureHookFromConfig(cfg, store, orch.Embedder)
	cons := memory.NewConsolidator(
		conn, store,
		memory.NewOpenAIGenerator(cfg.LLMBaseURL, cfg.LLMModel, ""),
		cfg.Consolidate,
	).WithEmbedder(orch.Embedder)

	scenarios, err := eval.LoadSynthScenarios(*scenariosPath)
	if err != nil {
		return fmt.Errorf("scenarios: %w", err)
	}

	passed, failures, err := eval.RunSynthEval(ctx, hook, cons, orch.Retriever, scenarios)
	if err != nil {
		return fmt.Errorf("synth eval: %w", err)
	}

	fmt.Fprintf(stdout, "synth-eval: passed %d/%d\n", passed, len(scenarios))
	for _, f := range failures {
		fmt.Fprintf(stdout, "  FAIL %s\n", f)
	}
	if passed != len(scenarios) {
		os.Exit(1)
	}
	return nil
}
