// memory-eval ingests the seed corpus into the live memory store and then
// runs every question through the retriever, printing per-question hit/miss
// and a mean recall@k summary. Intended for `make eval-memory`.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	// .env autoload (matches cmd/memory-demo).
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
	fs := flag.NewFlagSet("memory-eval", flag.ContinueOnError)
	fs.SetOutput(stderr)
	seedPath := fs.String("seed", "internal/memory/eval/testdata/seed.json", "path to seed JSON")
	skipIngest := fs.Bool("skip-ingest", false, "skip the corpus ingest step (use existing chunks)")
	questionType := fs.String("question-type", "", "filter to a single QuestionType (paraphrase|exact_token|fresh_vs_stale|negative); empty = all")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := memory.LoadConfig()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	ctx := context.Background()
	orch, conn, err := memory.NewModule(ctx, cfg)
	if err != nil {
		return fmt.Errorf("module: %w", err)
	}
	defer conn.Close(ctx)

	seed, err := eval.LoadSeed(*seedPath)
	if err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	if *questionType != "" {
		seed.Questions = filterByType(seed.Questions, eval.QuestionType(*questionType))
	}

	if !*skipIngest {
		if err := ingestFixturesOrCorpus(ctx, orch, seed, stdout); err != nil {
			return err
		}
	}

	fmt.Fprintln(stdout, "--- running eval ---")
	results, err := eval.RunRetrieval(ctx, orch.Retriever, seed)
	if err != nil {
		return fmt.Errorf("eval: %w", err)
	}

	for _, r := range results {
		mark := "MISS"
		if len(r.Hits) == len(r.Expected) {
			mark = "HIT "
		} else if len(r.Hits) > 0 {
			mark = "PART"
		}
		fmt.Fprintf(stdout, "[%s] %s  hits=%v expected=%v\n",
			mark, r.Question.ID, r.Hits, r.Expected)
	}

	s := eval.Summarize(results)
	fmt.Fprintf(stdout, "\nmean recall@%d = %.3f over %d questions (fixture_version=%d)\n",
		seed.K, s.MeanRecall, s.Total, s.FixtureVer)
	for _, qt := range []eval.QuestionType{eval.QuestionParaphrase, eval.QuestionExactToken, eval.QuestionFreshVsStale, eval.QuestionNegative} {
		if n := s.NPerType[qt]; n > 0 {
			fmt.Fprintf(stdout, "  %-16s recall@%d = %.3f over %d\n", qt, seed.K, s.PerType[qt], n)
		}
	}
	return nil
}

func filterByType(qs []eval.Question, t eval.QuestionType) []eval.Question {
	out := make([]eval.Question, 0, len(qs))
	for _, q := range qs {
		if q.QuestionType == t {
			out = append(out, q)
		}
	}
	return out
}

func ingestFixturesOrCorpus(ctx context.Context, orch *memory.Orchestrator, seed eval.Seed, stdout io.Writer) error {
	if dir := strings.TrimSpace(os.Getenv("RAG_FIXTURE_DIR")); dir != "" {
		fmt.Fprintf(stdout, "--- ingesting fixtures from %s ---\n", dir)
		docs, err := eval.LoadFixtureCorpus(dir)
		if err != nil {
			return fmt.Errorf("fixtures: %w", err)
		}
		for _, d := range docs {
			if err := orch.Ingest(ctx, d); err != nil {
				return fmt.Errorf("ingest %s: %w", d.Source, err)
			}
			fmt.Fprintf(stdout, "  ingested %s (id=%s)\n", d.Source, d.ID)
		}
		return nil
	}
	fmt.Fprintln(stdout, "--- ingesting seed corpus ---")
	for _, item := range seed.Corpus {
		data, err := os.ReadFile(item.Path)
		if err != nil {
			return fmt.Errorf("read %s: %w", item.Path, err)
		}
		if err := orch.Ingest(ctx, memory.RawDocument{
			ID:     item.ID,
			Source: item.Path,
			Text:   string(data),
		}); err != nil {
			return fmt.Errorf("ingest %s: %w", item.ID, err)
		}
		fmt.Fprintf(stdout, "  ingested %s (%s)\n", item.ID, item.Path)
	}
	return nil
}
