// memory-eval ingests the seed corpus into the live memory store and then
// runs every question through the retriever, printing per-question hit/miss
// and a mean recall@k summary. Intended for `make eval-memory`.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	// .env autoload (matches cmd/memory-demo).
	_ "github.com/joho/godotenv/autoload"

	"github.com/luannn010/ptolemy/internal/memory"
	"github.com/luannn010/ptolemy/internal/memory/eval"
)

func main() {
	seedPath := flag.String("seed", "docs/memory/eval/seed.json", "path to seed JSON")
	skipIngest := flag.Bool("skip-ingest", false, "skip the corpus ingest step (use existing chunks)")
	flag.Parse()

	cfg, err := memory.LoadConfig()
	if err != nil {
		die("config: %v", err)
	}
	ctx := context.Background()
	orch, conn, err := memory.NewModule(ctx, cfg)
	if err != nil {
		die("module: %v", err)
	}
	defer conn.Close(ctx)

	seed, err := eval.LoadSeed(*seedPath)
	if err != nil {
		die("seed: %v", err)
	}

	if !*skipIngest {
		fmt.Println("--- ingesting corpus ---")
		for _, item := range seed.Corpus {
			data, err := os.ReadFile(item.Path)
			if err != nil {
				die("read %s: %v", item.Path, err)
			}
			if err := orch.Ingest(ctx, memory.RawDocument{
				ID:     item.ID,
				Source: item.Path,
				Text:   string(data),
			}); err != nil {
				die("ingest %s: %v", item.ID, err)
			}
			fmt.Printf("  ingested %s (%s)\n", item.ID, item.Path)
		}
	}

	fmt.Println("--- running eval ---")
	results, err := eval.RunRetrieval(ctx, orch.Retriever, seed)
	if err != nil {
		die("eval: %v", err)
	}

	for _, r := range results {
		mark := "MISS"
		if len(r.Hits) == len(r.Expected) {
			mark = "HIT "
		} else if len(r.Hits) > 0 {
			mark = "PART"
		}
		fmt.Printf("[%s] %s  hits=%v expected=%v\n",
			mark, r.Question.ID, r.Hits, r.Expected)
	}

	s := eval.Summarize(results)
	fmt.Printf("\nmean recall@%d = %.3f over %d questions\n", seed.K, s.MeanRecall, s.Total)
}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(2)
}
