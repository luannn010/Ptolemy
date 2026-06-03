// syntheval drives the Phase 6b multi-session synthesis eval: for each scenario
// it injects the turns as atom rows, consolidates the (subject,project) via the
// live BRAIN LLM, recalls scoped to it, and scores the recalled synthesis
// summary against expect/reject keywords. Atoms are injected (not run through
// the per-turn extractor) so the gate is deterministic and makes one LLM call
// per scenario. Exposed as `ptolemy memory synth-eval`; uses the live BRAIN
// LLM, so it is NOT a `go test` target.
package memory

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/luannn010/ptolemy/internal/memory"
	"github.com/luannn010/ptolemy/internal/memory/eval"
)

// RunSynthEval is the `ptolemy memory synth-eval` subcommand. It returns
// ErrEvalFailed (mapped to exit 1 by the dispatcher) when one or more scenarios
// fail, distinct from setup errors (exit 2).
func RunSynthEval(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("memory synth-eval", flag.ContinueOnError)
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

	orch, conn, err := memory.NewModule(ctx, cfg)
	if err != nil {
		return fmt.Errorf("module: %w", err)
	}
	defer conn.Close(ctx)

	store := memory.NewPgStore(conn)
	cons := memory.NewConsolidator(
		conn, store,
		memory.NewOpenAIGenerator(cfg.LLMBaseURL, cfg.LLMModel, ""),
		cfg.Consolidate,
	).WithEmbedder(orch.Embedder)

	scenarios, err := eval.LoadSynthScenarios(*scenariosPath)
	if err != nil {
		return fmt.Errorf("scenarios: %w", err)
	}

	passed, failures, err := eval.RunSynthEval(ctx, store, orch.Embedder, cons, orch.Retriever, scenarios)
	if err != nil {
		return fmt.Errorf("synth eval: %w", err)
	}

	fmt.Fprintf(stdout, "synth-eval: passed %d/%d\n", passed, len(scenarios))
	for _, f := range failures {
		fmt.Fprintf(stdout, "  FAIL %s\n", f)
	}
	if passed != len(scenarios) {
		return ErrEvalFailed
	}
	return nil
}
