package memory

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/luannn010/ptolemy/internal/memory/eval"
)

func TestFilterByType_KeepsMatching(t *testing.T) {
	qs := []eval.Question{
		{ID: "q1", QuestionType: eval.QuestionParaphrase},
		{ID: "q2", QuestionType: eval.QuestionExactToken},
		{ID: "q3", QuestionType: eval.QuestionParaphrase},
		{ID: "q4", QuestionType: eval.QuestionFreshVsStale},
	}
	got := filterByType(qs, eval.QuestionParaphrase)
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}
	if got[0].ID != "q1" || got[1].ID != "q3" {
		t.Fatalf("expected q1,q3 got %+v", got)
	}
}

func TestFilterByType_EmptyResult(t *testing.T) {
	qs := []eval.Question{{ID: "q1", QuestionType: eval.QuestionParaphrase}}
	got := filterByType(qs, eval.QuestionNegative)
	if len(got) != 0 {
		t.Fatalf("expected empty, got %+v", got)
	}
}

// fakeSweepRunner returns deterministic Summaries keyed by (weight, halflife).
// Used by all the sweep-mode unit tests below.
type fakeSweepRunner struct {
	ingests     int
	queries     int
	freshRecall map[string]float64 // key fmt.Sprintf("%v|%v", w, h)
	fullRecall  map[string]float64
}

func (f *fakeSweepRunner) Ingest() error { f.ingests++; return nil }
func (f *fakeSweepRunner) RunCell(w float64, h time.Duration) (sweepCell, error) {
	f.queries++
	key := fmt.Sprintf("%v|%v", w, h)
	return sweepCell{
		Weight:     w,
		HalfLife:   h,
		FreshFsR5:  f.freshRecall[key],
		FullR5:     f.fullRecall[key],
		FixtureVer: 1,
	}, nil
}

func TestSweepMode_RunsAllNineCells(t *testing.T) {
	f := &fakeSweepRunner{freshRecall: map[string]float64{}, fullRecall: map[string]float64{}}
	for _, w := range sweepWeights {
		for _, h := range sweepHalfLives {
			key := fmt.Sprintf("%v|%v", w, h)
			f.freshRecall[key] = 0.5
			f.fullRecall[key] = 0.7
		}
	}
	var buf bytes.Buffer
	runSweep(f, &buf)
	if f.ingests != 1 {
		t.Fatalf("expected 1 ingest, got %d", f.ingests)
	}
	if f.queries != 9 {
		t.Fatalf("expected 9 query batches, got %d", f.queries)
	}
}

func TestSweepMode_EmitsMarkdownTable(t *testing.T) {
	f := &fakeSweepRunner{freshRecall: map[string]float64{}, fullRecall: map[string]float64{}}
	var buf bytes.Buffer
	runSweep(f, &buf)
	out := buf.String()
	// Header columns in order
	for _, col := range []string{"weight", "halflife", "fs_recall", "full_recall", "Δ_fs", "Δ_full"} {
		if !strings.Contains(out, col) {
			t.Fatalf("expected column %q in table, got:\n%s", col, out)
		}
	}
	// Footer with fixture version
	if !strings.Contains(out, "fixture_version=1") {
		t.Fatalf("expected footer fixture_version=1, got:\n%s", out)
	}
}

// NOTE: TestSweepMode_DeclaresWinnerInteriorOnly from the spec is intentionally
// omitted. The 3x3 grid's only interior cell IS the baseline centroid (0.1, 30d),
// so the WINNER path is unreachable on this grid by construction. The
// classifySweep function still handles WINNER for future larger grids; the
// behavior is covered by TestSweepMode_FlagsEdgeOptimum (rejects edge) and
// TestSweepMode_NoWinner* (rejects no-improvement / regression). If the grid is
// extended in a follow-up sub-PR, add a dedicated WINNER test then.

func TestSweepMode_FlagsEdgeOptimum(t *testing.T) {
	// Best cell is at corner (0.2, 90d). Decision rule emits WARNING and
	// does NOT emit WINNER.
	f := &fakeSweepRunner{freshRecall: map[string]float64{}, fullRecall: map[string]float64{}}
	for _, w := range sweepWeights {
		for _, h := range sweepHalfLives {
			key := fmt.Sprintf("%v|%v", w, h)
			f.freshRecall[key] = 0.50
			f.fullRecall[key] = 0.70
		}
	}
	corner := fmt.Sprintf("%v|%v", 0.2, 90*24*time.Hour)
	f.freshRecall[corner] = 0.80 // +30pp at corner
	var buf bytes.Buffer
	runSweep(f, &buf)
	out := buf.String()
	if !strings.Contains(out, "WARNING") || !strings.Contains(out, "grid edge") {
		t.Fatalf("expected WARNING + 'grid edge' message, got:\n%s", out)
	}
	if strings.Contains(out, "WINNER:") {
		t.Fatalf("must not declare WINNER when best is on edge, got:\n%s", out)
	}
}

func TestSweepMode_NoWinnerWhenNoCellImprovesByOnePp(t *testing.T) {
	// All cells within ±0.5pp of baseline (0.1, 30d).
	f := &fakeSweepRunner{freshRecall: map[string]float64{}, fullRecall: map[string]float64{}}
	for _, w := range sweepWeights {
		for _, h := range sweepHalfLives {
			key := fmt.Sprintf("%v|%v", w, h)
			f.freshRecall[key] = 0.502
			f.fullRecall[key] = 0.70
		}
	}
	// Baseline cell: 0.500
	baseKey := fmt.Sprintf("%v|%v", 0.1, 30*24*time.Hour)
	f.freshRecall[baseKey] = 0.500
	var buf bytes.Buffer
	runSweep(f, &buf)
	out := buf.String()
	if !strings.Contains(out, "NO WINNER") {
		t.Fatalf("expected 'NO WINNER' verdict, got:\n%s", out)
	}
}

func TestSweepMode_NoWinnerWhenFullSeedRegressionTooLarge(t *testing.T) {
	// One cell improves fs by 5pp but regresses full by 2pp → still NO WINNER
	// because the full-regression cap is 1pp. classifySweep picks the cell
	// with highest fs (the corner) and rejects it on the full check.
	f := &fakeSweepRunner{freshRecall: map[string]float64{}, fullRecall: map[string]float64{}}
	for _, w := range sweepWeights {
		for _, h := range sweepHalfLives {
			key := fmt.Sprintf("%v|%v", w, h)
			f.freshRecall[key] = 0.50 // includes baseline (0.1, 30d)
			f.fullRecall[key] = 0.70
		}
	}
	// Corner cell improves fs by 5pp but regresses full by 2pp.
	corner := fmt.Sprintf("%v|%v", 0.2, 7*24*time.Hour)
	f.freshRecall[corner] = 0.55
	f.fullRecall[corner] = 0.68

	var buf bytes.Buffer
	runSweep(f, &buf)
	out := buf.String()
	if !strings.Contains(out, "NO WINNER") {
		t.Fatalf("expected NO WINNER (full-regression > 1pp), got:\n%s", out)
	}
	if strings.Contains(out, "WINNER:") {
		t.Fatalf("must not declare WINNER, got:\n%s", out)
	}
}

func TestSweepMode_IngestsCorpusOnce(t *testing.T) {
	// Same assertion as TestSweepMode_RunsAllNineCells but isolating the
	// "ingest happens exactly once" property as a separate regression guard.
	f := &fakeSweepRunner{freshRecall: map[string]float64{}, fullRecall: map[string]float64{}}
	var buf bytes.Buffer
	runSweep(f, &buf)
	if f.ingests != 1 {
		t.Fatalf("ingest must run exactly once across the sweep, got %d", f.ingests)
	}
}

// TestRunEval_BadFlagReturnsError exercises the flag-parse guard in RunEval
// (the exported function). Passing an unknown flag triggers fs.Parse before any
// DB is opened.
func TestRunEval_BadFlagReturnsError(t *testing.T) {
	var out, errBuf bytes.Buffer
	err := RunEval(context.Background(), []string{"--no-such-flag"}, &out, &errBuf)
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

// TestRunEval_DedupRequiresFixtureDir exercises the dedup-only path in RunEval.
// With --dedup set but RAG_FIXTURE_DIR unset, RunEval returns an error before
// touching the DB.
func TestRunEval_DedupRequiresFixtureDir(t *testing.T) {
	t.Setenv("RAG_FIXTURE_DIR", "")
	var out, errBuf bytes.Buffer
	err := RunEval(context.Background(), []string{"--dedup"}, &out, &errBuf)
	if err == nil {
		t.Fatal("expected error when --dedup is set without RAG_FIXTURE_DIR")
	}
	if !strings.Contains(err.Error(), "RAG_FIXTURE_DIR") {
		t.Fatalf("expected RAG_FIXTURE_DIR in error, got: %v", err)
	}
}

// TestRunEval_DedupWithDir exercises the full --dedup output path, pointing at a
// temp dir with one markdown fixture. No DB or LLM required.
func TestRunEval_DedupWithDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("fact about ptolemy"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RAG_FIXTURE_DIR", dir)
	var out bytes.Buffer
	if err := RunEval(context.Background(), []string{"--dedup"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "dedup-redundancy") {
		t.Fatalf("expected dedup-redundancy in output, got: %q", out.String())
	}
}
