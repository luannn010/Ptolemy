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
	"sort"
	"strings"
	"time"

	// .env autoload (matches cmd/memory-demo).
	_ "github.com/joho/godotenv/autoload"

	"github.com/jackc/pgx/v5"
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
	sweep := fs.Bool("sweep", false, "3x3 recency sweep mode: ingest once, query nine times")
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

	if *sweep {
		runner := &liveSweepRunner{
			ctx:  ctx,
			orch: orch,
			seed: seed,
			conn: conn,
			ingestFn: func() error {
				return ingestFixturesOrCorpus(ctx, orch, seed, stdout)
			},
		}
		runSweep(runner, stdout)
		return nil
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

// --- Phase 3 sweep mode --------------------------------------------------

// sweepWeights and sweepHalfLives define the 3x3 grid. The defaults (0.1
// weight, 30d halflife) are the centroid, so the centroid IS the baseline.
var sweepWeights = []float64{0.05, 0.1, 0.2}
var sweepHalfLives = []time.Duration{
	7 * 24 * time.Hour,
	30 * 24 * time.Hour,
	90 * 24 * time.Hour,
}

// Decision-rule thresholds, expressed as proportions (1pp = 0.01).
const (
	improveFloor      = 0.01 // fs_recall must improve by at least this
	regressionCeiling = 0.01 // full_recall regression cap (cell >= baseline - this)
)

type sweepCell struct {
	Weight     float64
	HalfLife   time.Duration
	FreshFsR5  float64
	FullR5     float64
	FixtureVer int
}

// sweepRunner abstracts the work so tests can substitute a fake.
type sweepRunner interface {
	Ingest() error
	RunCell(weight float64, halflife time.Duration) (sweepCell, error)
}

// runSweep executes the 3x3 grid (ingest once, query nine times),
// emits a markdown table, and prints a verdict per the decision rule.
// Between iterations, ONLY recency params change; everything else
// (ingested corpus, embedder, brain LLM, RRF constant) is held fixed
// to isolate the recency effect.
func runSweep(r sweepRunner, out io.Writer) {
	if err := r.Ingest(); err != nil {
		fmt.Fprintf(out, "INCONCLUSIVE — ingest error: %v\n", err)
		return
	}
	cells := make([]sweepCell, 0, len(sweepWeights)*len(sweepHalfLives))
	for _, w := range sweepWeights {
		for _, h := range sweepHalfLives {
			c, err := r.RunCell(w, h)
			if err != nil {
				fmt.Fprintf(out, "| %v | %v | cell error: %v |\n", w, h, err)
				continue
			}
			cells = append(cells, c)
		}
	}
	// Locate baseline cell (centroid = 0.1, 30d).
	var baseline sweepCell
	for _, c := range cells {
		if c.Weight == 0.1 && c.HalfLife == 30*24*time.Hour {
			baseline = c
			break
		}
	}
	// Print markdown table.
	fmt.Fprintln(out, "| weight | halflife | fs_recall | full_recall | Δ_fs | Δ_full |")
	fmt.Fprintln(out, "|--------|----------|-----------|-------------|------|--------|")
	for _, c := range cells {
		dfs := c.FreshFsR5 - baseline.FreshFsR5
		dfull := c.FullR5 - baseline.FullR5
		fmt.Fprintf(out, "| %v | %v | %.3f | %.3f | %+.3f | %+.3f |\n",
			c.Weight, c.HalfLife, c.FreshFsR5, c.FullR5, dfs, dfull)
	}
	if len(cells) > 0 {
		fmt.Fprintf(out, "\nfixture_version=%d\n", cells[0].FixtureVer)
	}
	// Verdict.
	verdict := classifySweep(cells, baseline)
	fmt.Fprintln(out, verdict)
}

// classifySweep returns one of:
//   - "WINNER: weight=W halflife=H" (cell is interior, improves fs >= 1pp,
//     regresses full <= 1pp)
//   - "WARNING: optimum on grid edge — extend in (<direction>)" (best cell
//     by fs is on an edge of the grid)
//   - "NO WINNER — keep defaults"
//
// On a 3x3 grid, the only interior cell is the centroid; with the centroid
// also being the baseline, the WINNER path is unreachable on the current
// grid. The function still classifies correctly so future larger grids work.
func classifySweep(cells []sweepCell, baseline sweepCell) string {
	if len(cells) == 0 {
		return "NO WINNER — empty sweep"
	}
	// Find best fs cell.
	sorted := make([]sweepCell, len(cells))
	copy(sorted, cells)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].FreshFsR5 > sorted[j].FreshFsR5 })
	best := sorted[0]
	// Check it improves over baseline by the floor.
	if best.FreshFsR5-baseline.FreshFsR5 < improveFloor {
		return "NO WINNER — keep defaults"
	}
	// Check full-seed regression cap.
	if baseline.FullR5-best.FullR5 > regressionCeiling {
		return "NO WINNER — keep defaults"
	}
	// Check interior.
	if isOnEdge(best.Weight, best.HalfLife) {
		dir := edgeDirection(best.Weight, best.HalfLife)
		return fmt.Sprintf("WARNING: optimum on grid edge — extend in (%s)", dir)
	}
	return fmt.Sprintf("WINNER: weight=%v halflife=%v", best.Weight, best.HalfLife)
}

func isOnEdge(w float64, h time.Duration) bool {
	wMin, wMax := sweepWeights[0], sweepWeights[len(sweepWeights)-1]
	hMin, hMax := sweepHalfLives[0], sweepHalfLives[len(sweepHalfLives)-1]
	return w == wMin || w == wMax || h == hMin || h == hMax
}

func edgeDirection(w float64, h time.Duration) string {
	wMin, wMax := sweepWeights[0], sweepWeights[len(sweepWeights)-1]
	hMin, hMax := sweepHalfLives[0], sweepHalfLives[len(sweepHalfLives)-1]
	dirs := []string{}
	switch w {
	case wMin:
		dirs = append(dirs, "weight↓")
	case wMax:
		dirs = append(dirs, "weight↑")
	}
	switch h {
	case hMin:
		dirs = append(dirs, "halflife↓")
	case hMax:
		dirs = append(dirs, "halflife↑")
	}
	return strings.Join(dirs, ", ")
}

// liveSweepRunner adapts the production retriever to the sweepRunner
// interface. It rebuilds HybridRetriever per cell with the new
// (weight, halflife) — the underlying *pgx.Conn is reused, and the
// previously-ingested chunks remain in the DB for all 9 query batches.
type liveSweepRunner struct {
	ctx      context.Context
	orch     *memory.Orchestrator
	seed     eval.Seed
	conn     *pgx.Conn
	ingestFn func() error
	ingested bool
}

func (l *liveSweepRunner) Ingest() error {
	if l.ingested {
		return nil
	}
	l.ingested = true
	return l.ingestFn()
}

func (l *liveSweepRunner) RunCell(weight float64, halflife time.Duration) (sweepCell, error) {
	// Swap the orchestrator's retriever for one bound to this cell's recency
	// knobs. Embedder is reused (it's stateless).
	l.orch.Retriever = memory.NewHybridRetriever(l.conn, l.orch.Embedder, weight, halflife)
	results, err := eval.RunRetrieval(l.ctx, l.orch.Retriever, l.seed)
	if err != nil {
		return sweepCell{}, err
	}
	s := eval.Summarize(results)
	return sweepCell{
		Weight:     weight,
		HalfLife:   halflife,
		FreshFsR5:  s.PerType[eval.QuestionFreshVsStale],
		FullR5:     s.MeanRecall,
		FixtureVer: s.FixtureVer,
	}, nil
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
