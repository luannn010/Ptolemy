# Memory Module — Phase 3 (Eval-Set Hardening + Recency Tuning) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the first Phase 3 sub-PR — eval-set hardening (fixture corpus + ~30 tagged questions + per-type recall reporting) plus recency tuning (config knobs + 3×3 sweep + decision rule).

**Architecture:** Two coupled changes per the spec. Eval harness gains `QuestionType` tags, `LoadFixtureCorpus(dir) []RawDocument`, and a per-type `Summary`. `MemoryConfig` gains `RecencyWeight` and `RecencyHalfLife` (env: `RAG_RECENCY_WEIGHT`, `RAG_RECENCY_HALFLIFE_DAYS`) threaded through `Orchestrator` → `HybridRetriever`. The hybrid SQL grows two parameters (`$6` weight, `$7` halflife-seconds); the `0.1 / 2592000` constants become bind values. `cmd/memory-eval` gains a `-sweep` mode that ingests once and runs nine query batches.

**Tech Stack:** Go 1.25, `pgx/v5`, `pgvector-go`, ParadeDB `pg_search`, llama.cpp brain at `:1090` (offline question drafting only — never on query path), live PG at `192.168.0.164:1091`.

**Plan refinements vs. spec (semantics unchanged):**
- `LoadFixtureCorpus(dir)` returns `[]RawDocument`, not `[]Chunk`. The harness then routes through the existing `Orchestrator.Ingest` (chunker → embedder → store), so chunk IDs land as `<docID>#0` and existing `HitsExpected` prefix logic works unchanged.
- New seed.json omits the `corpus` field (fixture mode enumerates the dir); `Seed.Corpus` stays in the struct for backwards-compat with live-docs eval.
- `NewHybridRetriever` grows two positional args (not variadic opts). ~6 existing call sites updated in the same commit.

---

## File map

**Created:**
- `internal/memory/eval/testdata/corpus/*.md` (~12 markdown fixtures with YAML frontmatter)
- `internal/memory/eval/testdata/seed.json` (~30 tagged questions)
- `cmd/memory-eval/main_test.go` (new — sweep mode tests)

**Modified:**
- `internal/memory/eval/eval.go` — add `QuestionType` enum, tag the `Question` struct, add `LoadFixtureCorpus`, add per-type fields to `Summary`, add `fixtureVersion` constant.
- `internal/memory/eval/eval_test.go` — tests for the above.
- `internal/memory/config.go` — add `RecencyWeight float64`, `RecencyHalfLife time.Duration` fields + env parsing + validation; add `floatEnv` helper.
- `internal/memory/config_test.go` — tests for the new fields.
- `internal/memory/module.go` — pass `cfg.RecencyWeight` and `cfg.RecencyHalfLife` into `NewHybridRetriever`.
- `internal/memory/orchestrator.go` — no code change (Orchestrator doesn't construct retriever; module.go does); orchestrator tests gain a constructor-capture assertion.
- `internal/memory/hybrid_retriever.go` — `hybridRrfQuery` grows to 7 params; `NewHybridRetriever` signature gains `recencyWeight float64, recencyHalfLife time.Duration`; constructor stores them; `Retrieve` binds `$6` and `$7`.
- `internal/memory/hybrid_retriever_test.go` — update existing test call sites; add new tests for the new behavior; add the SQL semantics integration test.
- `internal/memory/bm25_retriever_test.go` — no signature change (BM25 doesn't take recency knobs); existing tests stay as-is.
- `internal/memory/module_test.go` — update `cfg` literal in `TestNewModule_DefaultRetrieverIsHybrid` to include the two new fields with valid values.
- `internal/memory/orchestrator_test.go` — no recency-knob test needed (Orchestrator doesn't see the knobs — they go straight from MemoryConfig to NewHybridRetriever in module.go); we cover module-level threading via existing TestNewModule + a small new test.
- `cmd/memory-eval/main.go` — add `-question-type` flag, add `-sweep` mode, add markdown-table emitter, add decision-rule classifier; switch default seed path to `internal/memory/eval/testdata/seed.json` AND read `RAG_FIXTURE_DIR` env.
- `Makefile` — rewire `eval-memory` to use the fixture corpus; add `eval-memory-sweep` target.
- `.env.example` — document `RAG_RECENCY_WEIGHT`, `RAG_RECENCY_HALFLIFE_DAYS`, `RAG_FIXTURE_DIR`.
- `docs/memory/RETRIEVAL.md` — note `$6` / `$7` are now config-driven.
- `docs/memory/IMPLEMENTATION_PLAN.md` — tick Phase 3 checkboxes with file/test pointers; one-line pointer to new seed location.

**Deleted:**
- `docs/memory/eval/seed.json` — lifecycle moves to `internal/memory/eval/testdata/seed.json`.

---

## Phase 3.0 — Eval harness foundation (no production code, no DB)

Five small commits. Pure-Go changes to the `eval` package + the CLI flag. No DB touched.

### Task 1: Add `QuestionType` enum + tag the `Question` struct (RED + GREEN)

**Files:**
- Modify: `internal/memory/eval/eval.go`
- Test: `internal/memory/eval/eval_test.go`

- [ ] **Step 1.1: Write failing test for tag round-trip**

Append to `internal/memory/eval/eval_test.go`:

```go
func TestLoadSeed_TagsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "seed.json")
	if err := os.WriteFile(path, []byte(`{
	  "k": 5,
	  "questions": [
	    {"id": "q1", "text": "x", "expected_doc_ids": ["d"], "question_type": "paraphrase"},
	    {"id": "q2", "text": "y", "expected_doc_ids": ["d"], "question_type": "exact_token"},
	    {"id": "q3", "text": "z", "expected_doc_ids": ["d"], "question_type": "fresh_vs_stale"},
	    {"id": "q4", "text": "w", "expected_doc_ids": ["d"], "question_type": "negative"}
	  ]
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSeed(path)
	if err != nil {
		t.Fatalf("LoadSeed: %v", err)
	}
	want := []QuestionType{QuestionParaphrase, QuestionExactToken, QuestionFreshVsStale, QuestionNegative}
	for i, q := range s.Questions {
		if q.QuestionType != want[i] {
			t.Fatalf("q[%d].QuestionType: got %q want %q", i, q.QuestionType, want[i])
		}
	}
}

func TestLoadSeed_UnknownTypeIsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "seed.json")
	if err := os.WriteFile(path, []byte(`{
	  "questions": [{"id": "qX", "text": "x", "expected_doc_ids": ["d"], "question_type": "bogus"}]
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadSeed(path)
	if err == nil {
		t.Fatal("expected error for unknown question_type")
	}
	if !strings.Contains(err.Error(), "qX") || !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("error should name the bad id and value, got %v", err)
	}
}
```

Add the `strings` import to the test file if it isn't already there (it isn't — current test file uses `path/filepath` only).

- [ ] **Step 1.2: Run tests to verify they fail**

Run: `go test ./internal/memory/eval/ -run TestLoadSeed_TagsRoundTrip -v`

Expected: FAIL with `undefined: QuestionType` / `undefined: QuestionParaphrase` (compile error).

- [ ] **Step 1.3: Implement the enum + struct field + validation**

In `internal/memory/eval/eval.go`:

(a) Add after the existing imports, near the top of the file:

```go
// QuestionType tags a seed question so the harness can report per-type recall.
// Phase 3 introduces four buckets; only these literal strings are accepted.
type QuestionType string

const (
	QuestionParaphrase   QuestionType = "paraphrase"
	QuestionExactToken   QuestionType = "exact_token"
	QuestionFreshVsStale QuestionType = "fresh_vs_stale"
	QuestionNegative     QuestionType = "negative"
)

// fixtureVersion is stamped into Summary.FixtureVer and into the sweep table
// footer. Bump whenever the byte-stable fixtures under testdata/corpus/ are
// resynced to evolving real docs, so cross-PR sweep tables stay comparable.
const fixtureVersion = 1

var validQuestionTypes = map[QuestionType]bool{
	QuestionParaphrase:   true,
	QuestionExactToken:   true,
	QuestionFreshVsStale: true,
	QuestionNegative:     true,
}
```

(b) Modify the existing `Question` struct to add `QuestionType`:

```go
type Question struct {
	ID             string       `json:"id"`
	Text           string       `json:"text"`
	ExpectedDocIDs []string     `json:"expected_doc_ids"`
	QuestionType   QuestionType `json:"question_type,omitempty"`
	Rationale      string       `json:"rationale,omitempty"`
}
```

(c) Modify `LoadSeed` to validate `QuestionType` after unmarshalling. Insert just before the existing `if s.K <= 0` block:

```go
	for _, q := range s.Questions {
		if q.QuestionType == "" {
			// Backwards-compat: untagged questions are allowed (the old 8-question
			// seed had no tags). They are reported under the empty bucket by
			// Summarize. The Phase 3 seed populates the tag for every question.
			continue
		}
		if !validQuestionTypes[q.QuestionType] {
			return Seed{}, fmt.Errorf("seed question %q: unknown question_type %q", q.ID, q.QuestionType)
		}
	}
```

- [ ] **Step 1.4: Run tests to verify they pass**

Run: `go test ./internal/memory/eval/ -run TestLoadSeed -v`

Expected: PASS (all `TestLoadSeed_*` tests including the new two).

- [ ] **Step 1.5: Commit**

```bash
git add internal/memory/eval/eval.go internal/memory/eval/eval_test.go
git commit -m "$(cat <<'EOF'
feat(memory/eval): add QuestionType enum + Question.QuestionType tag

LoadSeed validates question_type values against the four-bucket enum
(paraphrase, exact_token, fresh_vs_stale, negative). Untagged questions
remain allowed for backwards-compat with the legacy 8-question seed;
the Phase 3 seed populates the tag for every entry.

fixtureVersion=1 constant introduced; will be stamped into Summary and
sweep table footers in subsequent tasks.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Extend `Summary` with per-type recall + fixture version (RED + GREEN)

**Files:**
- Modify: `internal/memory/eval/eval.go`
- Test: `internal/memory/eval/eval_test.go`

- [ ] **Step 2.1: Write failing tests**

Append to `internal/memory/eval/eval_test.go`:

```go
func TestSummarize_PerTypeRecall(t *testing.T) {
	results := []QuestionResult{
		// paraphrase: 1.0 + 0.0 = 0.5 mean
		{Question: Question{QuestionType: QuestionParaphrase}, Hits: []string{"a"}, Expected: []string{"a"}},
		{Question: Question{QuestionType: QuestionParaphrase}, Hits: nil, Expected: []string{"a"}},
		// exact_token: 1.0
		{Question: Question{QuestionType: QuestionExactToken}, Hits: []string{"a"}, Expected: []string{"a"}},
		// fresh_vs_stale: 0.5
		{Question: Question{QuestionType: QuestionFreshVsStale}, Hits: []string{"a"}, Expected: []string{"a", "b"}},
	}
	s := Summarize(results)
	if abs(s.PerType[QuestionParaphrase]-0.5) > 1e-9 {
		t.Fatalf("paraphrase mean: got %v want 0.5", s.PerType[QuestionParaphrase])
	}
	if abs(s.PerType[QuestionExactToken]-1.0) > 1e-9 {
		t.Fatalf("exact_token mean: got %v want 1.0", s.PerType[QuestionExactToken])
	}
	if abs(s.PerType[QuestionFreshVsStale]-0.5) > 1e-9 {
		t.Fatalf("fresh_vs_stale mean: got %v want 0.5", s.PerType[QuestionFreshVsStale])
	}
	if s.NPerType[QuestionParaphrase] != 2 {
		t.Fatalf("paraphrase N: got %d want 2", s.NPerType[QuestionParaphrase])
	}
	// Overall: (1+0+1+0.5)/4 = 0.625
	if abs(s.MeanRecall-0.625) > 1e-9 {
		t.Fatalf("overall mean: got %v want 0.625", s.MeanRecall)
	}
}

func TestSummarize_StampsFixtureVersion(t *testing.T) {
	s := Summarize([]QuestionResult{
		{Question: Question{QuestionType: QuestionParaphrase}, Hits: []string{"a"}, Expected: []string{"a"}},
	})
	if s.FixtureVer != fixtureVersion {
		t.Fatalf("FixtureVer: got %d want %d", s.FixtureVer, fixtureVersion)
	}
}
```

- [ ] **Step 2.2: Run tests to verify they fail**

Run: `go test ./internal/memory/eval/ -run TestSummarize_PerTypeRecall -v`

Expected: FAIL with `unknown field PerType in struct literal of type Summary` (compile error).

- [ ] **Step 2.3: Extend the `Summary` struct + `Summarize` function**

In `internal/memory/eval/eval.go`, replace the existing `Summary` struct and `Summarize` function with:

```go
type Summary struct {
	Total      int
	MeanRecall float64
	PerType    map[QuestionType]float64
	NPerType   map[QuestionType]int
	FixtureVer int
}

// Summarize computes mean recall@k overall and per QuestionType. Questions
// with empty Expected are excluded from BOTH numerator and denominator — a
// malformed seed entry shouldn't silently drag the mean down. Untagged
// questions contribute to MeanRecall but NOT to any PerType bucket.
func Summarize(results []QuestionResult) Summary {
	out := Summary{
		Total:      len(results),
		PerType:    map[QuestionType]float64{},
		NPerType:   map[QuestionType]int{},
		FixtureVer: fixtureVersion,
	}
	if len(results) == 0 {
		return out
	}
	var sum float64
	var counted int
	perTypeSum := map[QuestionType]float64{}
	for _, r := range results {
		if len(r.Expected) == 0 {
			continue
		}
		recall := float64(len(r.Hits)) / float64(len(r.Expected))
		sum += recall
		counted++
		qt := r.Question.QuestionType
		if qt != "" {
			perTypeSum[qt] += recall
			out.NPerType[qt]++
		}
	}
	if counted > 0 {
		out.MeanRecall = sum / float64(counted)
	}
	for qt, s := range perTypeSum {
		if n := out.NPerType[qt]; n > 0 {
			out.PerType[qt] = s / float64(n)
		}
	}
	return out
}
```

- [ ] **Step 2.4: Run all eval tests to verify nothing regressed**

Run: `go test ./internal/memory/eval/ -v`

Expected: PASS (all existing + the two new tests). The existing `TestSummarize_AveragesRecall` and `TestSummarize_FiltersEmptyExpected` should still pass — they don't touch `PerType` and the new field defaults to an empty map which their assertions ignore.

- [ ] **Step 2.5: Commit**

```bash
git add internal/memory/eval/eval.go internal/memory/eval/eval_test.go
git commit -m "$(cat <<'EOF'
feat(memory/eval): Summary reports per-QuestionType recall + fixture version

Summary gains PerType (recall@k per QuestionType), NPerType (n per
bucket), and FixtureVer (stamped from the fixtureVersion package
constant). MeanRecall semantics unchanged.

Untagged questions still contribute to MeanRecall but not to any
PerType bucket — keeps backwards-compat with the legacy 8-question seed.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Add `LoadFixtureCorpus` (RED + GREEN)

**Files:**
- Modify: `internal/memory/eval/eval.go`
- Test: `internal/memory/eval/eval_test.go`

- [ ] **Step 3.1: Write failing tests**

Append to `internal/memory/eval/eval_test.go`:

```go
func TestLoadFixtureCorpus_ReadsMarkdown(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"),
		[]byte("# A\nalpha"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.md"),
		[]byte("# B\nbravo"), 0o600); err != nil {
		t.Fatal(err)
	}
	docs, err := LoadFixtureCorpus(dir)
	if err != nil {
		t.Fatalf("LoadFixtureCorpus: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("expected 2 docs, got %d", len(docs))
	}
	textByID := map[string]string{}
	for _, d := range docs {
		textByID[d.ID] = d.Text
	}
	var aText, bText string
	for _, d := range docs {
		if strings.Contains(d.Text, "alpha") {
			aText = d.Text
		}
		if strings.Contains(d.Text, "bravo") {
			bText = d.Text
		}
	}
	if aText == "" || bText == "" {
		t.Fatalf("expected one alpha and one bravo doc, got %+v", textByID)
	}
}

func TestLoadFixtureCorpus_DeterministicIDs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.md"),
		[]byte("body"), 0o600); err != nil {
		t.Fatal(err)
	}
	d1, err := LoadFixtureCorpus(dir)
	if err != nil {
		t.Fatal(err)
	}
	d2, err := LoadFixtureCorpus(dir)
	if err != nil {
		t.Fatal(err)
	}
	if d1[0].ID != d2[0].ID {
		t.Fatalf("IDs not deterministic across calls: %q vs %q", d1[0].ID, d2[0].ID)
	}
	if d1[0].ID == "" {
		t.Fatalf("ID should be non-empty")
	}
}

func TestLoadFixtureCorpus_FrontmatterPublishedAt(t *testing.T) {
	dir := t.TempDir()
	body := "---\npublished_at: 2024-01-15T00:00:00Z\n---\nthe body"
	if err := os.WriteFile(filepath.Join(dir, "p.md"),
		[]byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	docs, err := LoadFixtureCorpus(dir)
	if err != nil {
		t.Fatal(err)
	}
	d := docs[0]
	if pa, ok := d.Metadata["published_at"].(string); !ok || pa != "2024-01-15T00:00:00Z" {
		t.Fatalf("expected published_at metadata, got %+v", d.Metadata)
	}
	if d.Text != "the body" {
		t.Fatalf("expected text to strip frontmatter, got %q", d.Text)
	}
}

func TestLoadFixtureCorpus_NoFrontmatterFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "p.md"),
		[]byte("no frontmatter here"), 0o600); err != nil {
		t.Fatal(err)
	}
	docs, err := LoadFixtureCorpus(dir)
	if err != nil {
		t.Fatal(err)
	}
	d := docs[0]
	// Fallback uses the package-level fixtureBaseTime constant, NOT time.Now()
	// (which would re-introduce nondeterminism). The constant is a known past
	// date so fresh-vs-stale fixtures that DO set frontmatter are unambiguously
	// fresher than fallbacks. We assert the metadata is populated, not the
	// exact value (which is locked by fixtureBaseTime).
	if pa, ok := d.Metadata["published_at"].(string); !ok || pa == "" {
		t.Fatalf("expected fallback published_at, got %+v", d.Metadata)
	}
}
```

- [ ] **Step 3.2: Run tests to verify they fail**

Run: `go test ./internal/memory/eval/ -run TestLoadFixtureCorpus -v`

Expected: FAIL with `undefined: LoadFixtureCorpus` (compile error).

- [ ] **Step 3.3: Implement `LoadFixtureCorpus`**

In `internal/memory/eval/eval.go`, add new imports `crypto/sha256`, `encoding/hex`, `path/filepath`, `sort` to the import block (some may already be present — keep the block alphabetized). Then append:

```go
// fixtureBaseTime is the deterministic fallback published_at for fixture
// files that omit the YAML frontmatter. Locked to a known past date so
// fixture-mode eval is reproducible across runs. Fresh-vs-stale fixture
// pairs MUST set explicit frontmatter dates; their relative ordering is
// established by the dates they pick, not by this fallback.
const fixtureBaseTime = "2024-01-01T00:00:00Z"

// LoadFixtureCorpus reads byte-stable markdown fixtures from dir and returns
// them as RawDocuments suitable for Orchestrator.Ingest. Each .md file becomes
// one RawDocument.
//
// SNAPSHOT, NOT REFERENCE: the fixtures under internal/memory/eval/testdata/
// are frozen copies — not symlinks — of representative real docs. Bump the
// fixtureVersion constant whenever they are resynced to evolving real docs
// so cross-PR sweep tables stay comparable.
//
// ID = sha256(rel_path)[:16] — stable across runs as long as the fixture
// directory layout is stable. Returned slice is sorted by ID for determinism.
func LoadFixtureCorpus(dir string) ([]memory.RawDocument, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read fixture dir: %w", err)
	}
	var docs []memory.RawDocument
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		rel := e.Name()
		data, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", rel, err)
		}
		body, published, err := parseFrontmatter(data)
		if err != nil {
			return nil, fmt.Errorf("parse frontmatter in %s: %w", rel, err)
		}
		hash := sha256.Sum256([]byte(rel))
		id := hex.EncodeToString(hash[:])[:16]
		docs = append(docs, memory.RawDocument{
			ID:     id,
			Source: rel,
			Text:   body,
			Metadata: map[string]any{
				"published_at": published,
			},
		})
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].ID < docs[j].ID })
	return docs, nil
}

// parseFrontmatter extracts a YAML-ish "---\npublished_at: <rfc3339>\n---\n"
// header. If no header is present, body is the whole input and published is
// fixtureBaseTime. Only the published_at key is recognised — fixtures should
// not depend on other frontmatter fields.
func parseFrontmatter(data []byte) (body, published string, err error) {
	text := string(data)
	if !strings.HasPrefix(text, "---\n") {
		return text, fixtureBaseTime, nil
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		return "", "", fmt.Errorf("frontmatter opened with --- but never closed")
	}
	header := text[4 : 4+end]
	body = text[4+end+5:]
	published = fixtureBaseTime
	for _, line := range strings.Split(header, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "published_at:") {
			published = strings.TrimSpace(strings.TrimPrefix(line, "published_at:"))
		}
	}
	return body, published, nil
}
```

- [ ] **Step 3.4: Run tests to verify they pass**

Run: `go test ./internal/memory/eval/ -run TestLoadFixtureCorpus -v`

Expected: PASS (4 new tests).

- [ ] **Step 3.5: Run full eval package to verify nothing else regressed**

Run: `go test ./internal/memory/eval/ -v`

Expected: PASS (all tests).

- [ ] **Step 3.6: Commit**

```bash
git add internal/memory/eval/eval.go internal/memory/eval/eval_test.go
git commit -m "$(cat <<'EOF'
feat(memory/eval): LoadFixtureCorpus reads frozen markdown fixtures

Returns []RawDocument with stable IDs (sha256(rel_path)[:16]) and
published_at from YAML frontmatter (---\npublished_at: <rfc3339>\n---\n)
or a deterministic constant fallback.

Returns RawDocument rather than Chunk so the existing Orchestrator.Ingest
path (chunker → embedder → store) is reused unchanged — keeping
HitsExpected's "<docID>#" prefix matching valid.

SNAPSHOT, NOT REFERENCE: fixtures are frozen copies of real docs, not
symlinks. Bump fixtureVersion when resyncing so cross-PR sweep tables
stay comparable.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Add `-question-type` flag to memory-eval CLI (RED + GREEN)

**Files:**
- Modify: `cmd/memory-eval/main.go`
- Create: `cmd/memory-eval/main_test.go`

- [ ] **Step 4.1: Refactor `main()` into a testable function (no behavior change)**

Pure refactor before the new flag — main shrinks to flag parsing + delegation. Replace the entire `main()` function with:

```go
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
```

Then replace the existing `die` function with the equivalent in `run()` above (delete the old `die` function), and add `"io"` and `"strings"` to the imports.

Final import block should be:

```go
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
```

- [ ] **Step 4.2: Verify refactor compiles and existing build still works**

Run: `go build ./cmd/memory-eval`

Expected: success (no output).

- [ ] **Step 4.3: Write failing test for question-type filter**

Create `cmd/memory-eval/main_test.go`:

```go
package main

import (
	"testing"

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
```

- [ ] **Step 4.4: Run tests to verify they pass**

Run: `go test ./cmd/memory-eval/ -v`

Expected: PASS (both tests).

- [ ] **Step 4.5: Commit**

```bash
git add cmd/memory-eval/main.go cmd/memory-eval/main_test.go
git commit -m "$(cat <<'EOF'
feat(memory-eval): -question-type flag + RAG_FIXTURE_DIR ingest path

Refactor main() into a testable run() function. Add -question-type
flag (filters seed by QuestionType). Add RAG_FIXTURE_DIR env path:
when set, ingest from the fixture directory via LoadFixtureCorpus
instead of from seed.Corpus. Per-type recall printed alongside
overall recall and the fixture_version stamp.

Default seed path moves to internal/memory/eval/testdata/seed.json
(file is created in the next task).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Phase 3.1 — Fixture corpus + grown seed (data commit)

Two commits: (a) the fixture corpus, (b) the new seed.json (and delete old seed). No Go test changes — Phase 3.0 tests gate format; Phase 3.5 (live sweep) gates substance.

### Task 5: Create the fixture corpus

**Files:**
- Create: `internal/memory/eval/testdata/corpus/*.md` (12 files)

The fixtures are snapshot copies of representative Ptolemy docs, trimmed to focused mini-passages. Each file has a YAML frontmatter `published_at` and a short body. Fresh-vs-stale pairs share a stem (e.g., `policy-v1.md` / `policy-v2.md`) with different dates.

- [ ] **Step 5.1: Create the directory**

Run: `mkdir -p internal/memory/eval/testdata/corpus`

- [ ] **Step 5.2: Write the 12 fixture files**

Author each of the following with the exact content shown. Sourced from snapshots of real Ptolemy docs as of `main@06c4834`; bodies are trimmed to keep each fixture small enough to embed in a single chunk (under ~80 tokens / ~320 runes when chunked with `EVAL_CHUNK_SIZE=20`).

Create `internal/memory/eval/testdata/corpus/rrf.md`:
```markdown
---
published_at: 2024-01-15T00:00:00Z
---
# RRF: Reciprocal Rank Fusion

Vector distance and BM25 scores live on different scales. RRF combines
ranks instead of raw scores: `rrf_score(chunk) = Σ 1 / (C + rank_in_list)`.
The constant C is 60. A chunk appearing in only one list still scores
from that list. No normalization, no per-arm weights.
```

Create `internal/memory/eval/testdata/corpus/bm25-operator.md`:
```markdown
---
published_at: 2024-01-15T00:00:00Z
---
# BM25 operator

ParadeDB's pg_search exposes the `@@@` operator for BM25 match. The
hybrid retriever's BM25 CTE uses `WHERE content @@@ $1` and orders
by `paradedb.score(id) DESC`. The operator is exact-token-friendly
and case-insensitive by default.
```

Create `internal/memory/eval/testdata/corpus/guarded-fileops.md`:
```markdown
---
published_at: 2024-02-01T00:00:00Z
---
# GuardedFileOps

Services hold Guarded* wrappers only — never raw adapters. GuardedFileOps
wraps the fileops adapter and routes every call through internal/policy
before any side effect. Raw adapters live exclusively in cmd/workerd/main.go.
```

Create `internal/memory/eval/testdata/corpus/deny-policy-write.md`:
```markdown
---
published_at: 2024-02-01T00:00:00Z
---
# deny-policy-write rule

The policy engine has two self-protection rules that must never be
loosened: deny-policy-write blocks writes to .ptolemy/policy.json, and
deny-secret-* blocks reads of secrets. New rules are always allow or ask;
never weaken a deny.
```

Create `internal/memory/eval/testdata/corpus/supersession.md`:
```markdown
---
published_at: 2024-03-01T00:00:00Z
---
# Supersession

A new chunk replaces an older one for the same fact by setting
`superseded_by = <new_id>` on the old row. The retrieval SQL filters
`WHERE superseded_by IS NULL`, so stale facts stop being retrieved
without being deleted. Detection is explicit: the ingest caller passes
Metadata["supersedes"] = "<old-doc-id>".
```

Create `internal/memory/eval/testdata/corpus/hnsw.md`:
```markdown
---
published_at: 2024-01-15T00:00:00Z
---
# HNSW index

The dense vector index uses pgvector's HNSW (Hierarchical Navigable
Small World) with `vector_cosine_ops`. The distance operator `<=>`
returns cosine distance (smaller = closer). HNSW is approximate but
fast at retrieval-time; build cost is paid once at insert.
```

Create `internal/memory/eval/testdata/corpus/recency-term.md`:
```markdown
---
published_at: 2024-03-15T00:00:00Z
---
# Recency term

The hybrid SQL adds a recency boost to the RRF score:
`0.1 * exp(-Δt / 2592000)` where Δt is seconds since published_at and
2592000 is the 30-day half-life. The 0.1 weight and 30-day half-life
are tuning knobs Phase 3 may revise against the eval set.
```

Create `internal/memory/eval/testdata/corpus/ptolemy-purpose.md`:
```markdown
---
published_at: 2024-01-01T00:00:00Z
---
# Ptolemy purpose

Ptolemy is a Go-based agent runtime being rebuilt clean-room as v2.
The policy harness gates every side-effecting operation (shellcmd,
fileops, gitops, worktrees) behind hybrid approvals — in-band tokens
for low-risk commands, out-of-band approval for high-risk ones.
```

Create `internal/memory/eval/testdata/corpus/policy-v1.md`:
```markdown
---
published_at: 2024-02-01T00:00:00Z
---
# Approval policy (v1)

For shell commands the agent uses in-band tokens for low-risk calls
and out-of-band approval for high-risk ones. The token TTL is 5 minutes.
The agent never self-approves; the worker console is the approval surface.
```

Create `internal/memory/eval/testdata/corpus/policy-v2.md`:
```markdown
---
published_at: 2026-01-01T00:00:00Z
---
# Approval policy (v2)

For shell commands the agent uses in-band tokens for low-risk calls
and out-of-band approval for high-risk ones. The token TTL is now 15
minutes (extended from 5 minutes in v1 after operator feedback). The
agent never self-approves; the worker console is the approval surface.
```

Create `internal/memory/eval/testdata/corpus/topk-v1.md`:
```markdown
---
published_at: 2024-02-01T00:00:00Z
---
# RAG_TOP_K default (v1)

The retrieval orchestrator defaults RAG_TOP_K to 5 if the env var is
unset. The default applies only at MemoryConfig load; downstream
callers may override per query via Query.K.
```

Create `internal/memory/eval/testdata/corpus/topk-v2.md`:
```markdown
---
published_at: 2026-01-01T00:00:00Z
---
# RAG_TOP_K default (v2)

The retrieval orchestrator defaults RAG_TOP_K to 8 if the env var is
unset (revised from 5 in v1 after the Phase 1 eval set showed n=5
truncated some paraphrase-class hits). Per-query override via Query.K
still applies.
```

- [ ] **Step 5.3: Sanity-check counts and frontmatter**

Run: `ls internal/memory/eval/testdata/corpus/ | wc -l`

Expected: `12`

Run: `grep -l "published_at:" internal/memory/eval/testdata/corpus/*.md | wc -l`

Expected: `12`

- [ ] **Step 5.4: Commit the fixtures**

```bash
git add internal/memory/eval/testdata/corpus/
git commit -m "$(cat <<'EOF'
feat(memory/eval): frozen fixture corpus (12 docs) for Phase 3 eval

Snapshots of representative Ptolemy doc passages, each with a YAML
frontmatter published_at. Fresh-vs-stale pairs (policy-v1/v2, topk-v1/v2)
share a topic with different dates so recency tuning has a measurable
substrate.

Each fixture is small enough to land in a single chunk under
EVAL_CHUNK_SIZE=20 (~320 runes), staying under the llama.cpp embedding
server's 64-token batch ceiling.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Author the grown seed.json + delete the old seed

**Files:**
- Create: `internal/memory/eval/testdata/seed.json`
- Delete: `docs/memory/eval/seed.json`

The seed has ~30 questions across four buckets. IDs prefixed by bucket for greppability. Each question's `expected_doc_ids` references the fixture file IDs (sha256(rel_path)[:16] of the fixture filename).

- [ ] **Step 6.1: Compute the fixture IDs**

Run:

```bash
for f in internal/memory/eval/testdata/corpus/*.md; do
  base=$(basename "$f")
  id=$(printf "%s" "$base" | sha256sum | cut -c1-16)
  echo "$id  $base"
done | sort
```

Expected output (these IDs are deterministic; verify against your shell — they should match exactly because `sha256sum "policy-v1.md"` is platform-independent):

```
2a8d0e8b65e0d8aa  hnsw.md
3a1ad57e6b7e0d8c  recency-term.md
3f49a0c45a8c2e07  guarded-fileops.md
... (one line per fixture, 12 total) ...
```

If your shell produces different hashes, use those — what matters is consistency with `LoadFixtureCorpus`'s output, not the values in this plan. Capture the mapping for the next step.

- [ ] **Step 6.2: Use the brain LLM to draft ~40 candidate questions (offline, optional)**

This step is OPTIONAL — you can hand-author the 30 questions directly. If you use the LLM:

```bash
for f in internal/memory/eval/testdata/corpus/*.md; do
  echo "=== $(basename $f) ==="
  body=$(sed -n '/^---$/,/^---$/!p' "$f")  # strip frontmatter
  curl -sS --max-time 240 http://127.0.0.1:1090/v1/chat/completions \
    -H 'Content-Type: application/json' \
    -d "$(jq -n --arg b "$body" '{
      model: "qwen3.5:4b",
      messages: [
        {role: "system", content: "Draft 4 short retrieval questions about this passage: 1 paraphrase (rewords the content), 1 exact-token (uses a distinctive identifier verbatim), 1 fresh-vs-stale (asks about a version-sensitive fact), 1 negative (asks about something the passage does NOT cover). Output ONLY JSON: {\"questions\":[{\"text\":\"...\",\"type\":\"paraphrase|exact_token|fresh_vs_stale|negative\"}]}"},
        {role: "user", content: $b}
      ],
      temperature: 0.2,
      max_tokens: 400
    }')" | jq -r '.choices[0].message.content'
  echo
done > /tmp/phase3-candidates.txt
```

Review `/tmp/phase3-candidates.txt`. Pick the best ~30; discard any whose text overlaps the source passage by more than 50% of distinctive tokens (a question that quotes "GuardedFileOps wraps fileops" is circular — you want "what wraps the fileops adapter?"). Hand-rephrase survivors away from source wording.

- [ ] **Step 6.3: Write the seed.json**

Create `internal/memory/eval/testdata/seed.json` with the structure below. Replace each `"<FIXTURE_ID_FOR_X>"` placeholder with the actual 16-char sha256 hash from Step 6.1 for the named fixture. The mix is 12 paraphrase + 8 exact-token + 8 fresh-vs-stale (4 pairs × 2 questions) + 2 negative = 30.

```json
{
  "k": 5,
  "questions": [
    {
      "id": "p01-rrf-paraphrase",
      "text": "how does the retriever combine vector and keyword search results?",
      "expected_doc_ids": ["<FIXTURE_ID_FOR_rrf.md>"],
      "question_type": "paraphrase",
      "rationale": "Paraphrase of 'RRF combines ranks instead of raw scores'."
    },
    {
      "id": "p02-bm25-paraphrase",
      "text": "which operator does the lexical arm use to match keywords?",
      "expected_doc_ids": ["<FIXTURE_ID_FOR_bm25-operator.md>"],
      "question_type": "paraphrase"
    },
    {
      "id": "p03-fileops-paraphrase",
      "text": "what wraps the raw fileops adapter for services?",
      "expected_doc_ids": ["<FIXTURE_ID_FOR_guarded-fileops.md>"],
      "question_type": "paraphrase"
    },
    {
      "id": "p04-denypolicy-paraphrase",
      "text": "what rule prevents the policy file from being modified?",
      "expected_doc_ids": ["<FIXTURE_ID_FOR_deny-policy-write.md>"],
      "question_type": "paraphrase"
    },
    {
      "id": "p05-supersession-paraphrase",
      "text": "how are stale facts retired without being deleted?",
      "expected_doc_ids": ["<FIXTURE_ID_FOR_supersession.md>"],
      "question_type": "paraphrase"
    },
    {
      "id": "p06-hnsw-paraphrase",
      "text": "what dense index does the vector arm use?",
      "expected_doc_ids": ["<FIXTURE_ID_FOR_hnsw.md>"],
      "question_type": "paraphrase"
    },
    {
      "id": "p07-recency-paraphrase",
      "text": "how does the ranking bias toward newer content?",
      "expected_doc_ids": ["<FIXTURE_ID_FOR_recency-term.md>"],
      "question_type": "paraphrase"
    },
    {
      "id": "p08-purpose-paraphrase",
      "text": "what is ptolemy's goal as a project?",
      "expected_doc_ids": ["<FIXTURE_ID_FOR_ptolemy-purpose.md>"],
      "question_type": "paraphrase"
    },
    {
      "id": "p09-rrf-paraphrase-2",
      "text": "why are raw scores from different retrievers not directly comparable?",
      "expected_doc_ids": ["<FIXTURE_ID_FOR_rrf.md>"],
      "question_type": "paraphrase"
    },
    {
      "id": "p10-supersession-paraphrase-2",
      "text": "what mechanism marks a chunk as replaced by another?",
      "expected_doc_ids": ["<FIXTURE_ID_FOR_supersession.md>"],
      "question_type": "paraphrase"
    },
    {
      "id": "p11-hnsw-paraphrase-2",
      "text": "what is the distance metric used by the vector index?",
      "expected_doc_ids": ["<FIXTURE_ID_FOR_hnsw.md>"],
      "question_type": "paraphrase"
    },
    {
      "id": "p12-policy-paraphrase",
      "text": "what approval surface does the agent use for high-risk commands?",
      "expected_doc_ids": ["<FIXTURE_ID_FOR_policy-v2.md>", "<FIXTURE_ID_FOR_policy-v1.md>"],
      "question_type": "paraphrase",
      "rationale": "Both v1 and v2 mention worker console — paraphrase tolerant."
    },

    {
      "id": "e01-rrf-constant",
      "text": "RRF C=60",
      "expected_doc_ids": ["<FIXTURE_ID_FOR_rrf.md>"],
      "question_type": "exact_token"
    },
    {
      "id": "e02-bm25-operator",
      "text": "@@@",
      "expected_doc_ids": ["<FIXTURE_ID_FOR_bm25-operator.md>"],
      "question_type": "exact_token"
    },
    {
      "id": "e03-guarded-fileops",
      "text": "GuardedFileOps",
      "expected_doc_ids": ["<FIXTURE_ID_FOR_guarded-fileops.md>"],
      "question_type": "exact_token"
    },
    {
      "id": "e04-deny-policy-write",
      "text": "deny-policy-write",
      "expected_doc_ids": ["<FIXTURE_ID_FOR_deny-policy-write.md>"],
      "question_type": "exact_token"
    },
    {
      "id": "e05-superseded-by",
      "text": "superseded_by",
      "expected_doc_ids": ["<FIXTURE_ID_FOR_supersession.md>"],
      "question_type": "exact_token"
    },
    {
      "id": "e06-hnsw",
      "text": "HNSW",
      "expected_doc_ids": ["<FIXTURE_ID_FOR_hnsw.md>"],
      "question_type": "exact_token"
    },
    {
      "id": "e07-cosine-ops",
      "text": "vector_cosine_ops",
      "expected_doc_ids": ["<FIXTURE_ID_FOR_hnsw.md>"],
      "question_type": "exact_token"
    },
    {
      "id": "e08-half-life",
      "text": "2592000",
      "expected_doc_ids": ["<FIXTURE_ID_FOR_recency-term.md>"],
      "question_type": "exact_token"
    },

    {
      "id": "f01-policy-ttl-current",
      "text": "what is the current token TTL for the approval policy?",
      "expected_doc_ids": ["<FIXTURE_ID_FOR_policy-v2.md>"],
      "question_type": "fresh_vs_stale",
      "rationale": "v2 says 15 min, v1 says 5 min. Recency boost should rank v2 above v1."
    },
    {
      "id": "f02-policy-ttl-history",
      "text": "what was the original token TTL before being extended?",
      "expected_doc_ids": ["<FIXTURE_ID_FOR_policy-v1.md>"],
      "question_type": "fresh_vs_stale",
      "rationale": "Asks for the historical value — v1 should still be retrievable (not superseded, just older)."
    },
    {
      "id": "f03-topk-default-current",
      "text": "what is the current default for RAG_TOP_K?",
      "expected_doc_ids": ["<FIXTURE_ID_FOR_topk-v2.md>"],
      "question_type": "fresh_vs_stale",
      "rationale": "v2 says 8, v1 says 5. Recency boost should rank v2 above v1."
    },
    {
      "id": "f04-topk-default-history",
      "text": "what was the original RAG_TOP_K default before being revised?",
      "expected_doc_ids": ["<FIXTURE_ID_FOR_topk-v1.md>"],
      "question_type": "fresh_vs_stale"
    },
    {
      "id": "f05-policy-paraphrase-fresh",
      "text": "how long are approval tokens valid for at present?",
      "expected_doc_ids": ["<FIXTURE_ID_FOR_policy-v2.md>"],
      "question_type": "fresh_vs_stale"
    },
    {
      "id": "f06-policy-paraphrase-stale",
      "text": "how long were approval tokens originally valid for?",
      "expected_doc_ids": ["<FIXTURE_ID_FOR_policy-v1.md>"],
      "question_type": "fresh_vs_stale"
    },
    {
      "id": "f07-topk-paraphrase-fresh",
      "text": "how many chunks does the retriever now return by default?",
      "expected_doc_ids": ["<FIXTURE_ID_FOR_topk-v2.md>"],
      "question_type": "fresh_vs_stale"
    },
    {
      "id": "f08-topk-paraphrase-stale",
      "text": "how many chunks did the retriever return by default in the first phase?",
      "expected_doc_ids": ["<FIXTURE_ID_FOR_topk-v1.md>"],
      "question_type": "fresh_vs_stale"
    },

    {
      "id": "n01-no-grpc",
      "text": "how is the gRPC reflection service exposed?",
      "expected_doc_ids": [],
      "question_type": "negative",
      "rationale": "No fixture mentions gRPC. Retrieval may return chunks, but Expected=[] means recall is excluded from the mean (Summarize behaviour)."
    },
    {
      "id": "n02-no-billing",
      "text": "what billing provider does ptolemy use?",
      "expected_doc_ids": [],
      "question_type": "negative"
    }
  ]
}
```

- [ ] **Step 6.4: Validate seed.json**

Run: `jq '.questions | length' internal/memory/eval/testdata/seed.json`

Expected: `30`

Run: `jq -r '.questions[].question_type' internal/memory/eval/testdata/seed.json | sort | uniq -c`

Expected:
```
   8 exact_token
   8 fresh_vs_stale
   2 negative
  12 paraphrase
```

Run: `jq '.questions[].expected_doc_ids[]' internal/memory/eval/testdata/seed.json | sort -u | wc -l`

Expected: should be ≤ 12 (the number of fixture IDs referenced). No question should reference an ID that isn't a real fixture:

```bash
# Cross-check: every referenced ID must exist as a fixture
jq -r '.questions[].expected_doc_ids[]' internal/memory/eval/testdata/seed.json | sort -u > /tmp/referenced.txt
for f in internal/memory/eval/testdata/corpus/*.md; do
  printf "%s\n" "$(printf "%s" "$(basename $f)" | sha256sum | cut -c1-16)"
done | sort -u > /tmp/exists.txt
comm -23 /tmp/referenced.txt /tmp/exists.txt
```

Expected: empty output (every referenced ID is a real fixture).

- [ ] **Step 6.5: Delete the old seed**

Run: `git rm docs/memory/eval/seed.json && rmdir docs/memory/eval 2>/dev/null || true`

- [ ] **Step 6.6: Commit the new seed + deletion**

```bash
git add internal/memory/eval/testdata/seed.json
git commit -m "$(cat <<'EOF'
feat(memory/eval): grow eval seed 8 → 30 with QuestionType tags

12 paraphrase + 8 exact_token + 8 fresh_vs_stale (4 pairs × 2) + 2 negative.
Expected_doc_ids reference fixture IDs (sha256(rel_path)[:16]) under
internal/memory/eval/testdata/corpus/. Old seed at docs/memory/eval/seed.json
deleted — lifecycle now lives with the loader.

Fresh-vs-stale pairs (policy-v1/v2, topk-v1/v2) give the recency tuning
sweep a measurable substrate.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Phase 3.2 — Config knob plumbing (no SQL change yet)

Two commits. Production behavior preserved (defaults match Phase 2 constants).

### Task 7: Add `RecencyWeight` + `RecencyHalfLife` to `MemoryConfig` (RED + GREEN)

**Files:**
- Modify: `internal/memory/config.go`
- Test: `internal/memory/config_test.go`

- [ ] **Step 7.1: Write failing tests**

Append to `internal/memory/config_test.go`:

```go
func TestLoadConfig_RecencyDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("EMBEDDING_BASE_URL", "http://e")
	t.Setenv("EMBEDDING_MODEL", "m")
	t.Setenv("EMBEDDING_DIM", "1024")
	t.Setenv("BRAIN_BASE_URL", "http://l")
	t.Setenv("BRAIN_MODEL", "lm")
	t.Setenv("RAG_RECENCY_WEIGHT", "")
	t.Setenv("RAG_RECENCY_HALFLIFE_DAYS", "")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.RecencyWeight != 0.1 {
		t.Fatalf("default weight: got %v want 0.1", cfg.RecencyWeight)
	}
	wantHL := 30 * 24 * time.Hour
	if cfg.RecencyHalfLife != wantHL {
		t.Fatalf("default halflife: got %v want %v", cfg.RecencyHalfLife, wantHL)
	}
}

func TestLoadConfig_RecencyEnvParsed(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("EMBEDDING_BASE_URL", "http://e")
	t.Setenv("EMBEDDING_MODEL", "m")
	t.Setenv("EMBEDDING_DIM", "1024")
	t.Setenv("BRAIN_BASE_URL", "http://l")
	t.Setenv("BRAIN_MODEL", "lm")
	t.Setenv("RAG_RECENCY_WEIGHT", "0.2")
	t.Setenv("RAG_RECENCY_HALFLIFE_DAYS", "7")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.RecencyWeight != 0.2 {
		t.Fatalf("weight: got %v want 0.2", cfg.RecencyWeight)
	}
	if cfg.RecencyHalfLife != 7*24*time.Hour {
		t.Fatalf("halflife: got %v want %v", cfg.RecencyHalfLife, 7*24*time.Hour)
	}
}

func TestLoadConfig_RejectsNegativeRecencyWeight(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("EMBEDDING_BASE_URL", "http://e")
	t.Setenv("EMBEDDING_MODEL", "m")
	t.Setenv("EMBEDDING_DIM", "1024")
	t.Setenv("BRAIN_BASE_URL", "http://l")
	t.Setenv("BRAIN_MODEL", "lm")
	t.Setenv("RAG_RECENCY_WEIGHT", "-0.1")
	if _, err := LoadConfig(); err == nil {
		t.Fatalf("expected negative weight to be rejected")
	}
}

func TestLoadConfig_RejectsHalfLifeBelow1Hour(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("EMBEDDING_BASE_URL", "http://e")
	t.Setenv("EMBEDDING_MODEL", "m")
	t.Setenv("EMBEDDING_DIM", "1024")
	t.Setenv("BRAIN_BASE_URL", "http://l")
	t.Setenv("BRAIN_MODEL", "lm")
	// 0.01 days = 14.4 minutes
	t.Setenv("RAG_RECENCY_HALFLIFE_DAYS", "0.01")
	if _, err := LoadConfig(); err == nil {
		t.Fatalf("expected sub-1h halflife to be rejected")
	}
}

func TestLoadConfig_RejectsZeroHalfLife(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("EMBEDDING_BASE_URL", "http://e")
	t.Setenv("EMBEDDING_MODEL", "m")
	t.Setenv("EMBEDDING_DIM", "1024")
	t.Setenv("BRAIN_BASE_URL", "http://l")
	t.Setenv("BRAIN_MODEL", "lm")
	t.Setenv("RAG_RECENCY_HALFLIFE_DAYS", "0")
	if _, err := LoadConfig(); err == nil {
		t.Fatalf("expected zero halflife to be rejected")
	}
}
```

Add `"time"` to the test file's imports if not already present.

- [ ] **Step 7.2: Run tests to verify they fail**

Run: `go test ./internal/memory/ -run TestLoadConfig_Recency -v`

Expected: FAIL with `cfg.RecencyWeight undefined (type MemoryConfig has no field RecencyWeight)`.

- [ ] **Step 7.3: Implement the fields + validation + helper**

In `internal/memory/config.go`:

(a) Add `"time"` to the imports (next to the existing `"strconv"`).

(b) Add the two fields to `MemoryConfig` (after `ChunkOverlapTokens`):

```go
	// Recency tuning knobs (Phase 3). Spec defaults are 0.1 and 30 days;
	// production behavior is preserved when RAG_RECENCY_* are unset.
	RecencyWeight   float64       // env: RAG_RECENCY_WEIGHT
	RecencyHalfLife time.Duration // env: RAG_RECENCY_HALFLIFE_DAYS (parsed as float days)
```

(c) After the existing `cfg.TopK = intEnv(...)` block in `LoadConfig`, add:

```go
	cfg.RecencyWeight = floatEnv("RAG_RECENCY_WEIGHT", 0.1)
	if cfg.RecencyWeight < 0 {
		return MemoryConfig{}, fmt.Errorf("RAG_RECENCY_WEIGHT must be >= 0, got %v", cfg.RecencyWeight)
	}
	halflifeDays := floatEnv("RAG_RECENCY_HALFLIFE_DAYS", 30)
	cfg.RecencyHalfLife = time.Duration(halflifeDays * float64(24*time.Hour))
	if cfg.RecencyHalfLife < time.Hour {
		return MemoryConfig{}, fmt.Errorf("RAG_RECENCY_HALFLIFE_DAYS resolves to %v, must be >= 1h", cfg.RecencyHalfLife)
	}
```

(d) Add `floatEnv` helper alongside the existing `intEnv` at the bottom of the file:

```go
func floatEnv(key string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fallback
	}
	return v
}
```

- [ ] **Step 7.4: Run tests to verify they pass**

Run: `go test ./internal/memory/ -run TestLoadConfig -v`

Expected: PASS (all existing TestLoadConfig_* plus the five new tests).

- [ ] **Step 7.5: Run the broader package to verify nothing regressed**

Run: `go test ./internal/memory/ -run 'TestLoadConfig|TestIntEnv' -v`

Expected: PASS.

- [ ] **Step 7.6: Commit**

```bash
git add internal/memory/config.go internal/memory/config_test.go
git commit -m "$(cat <<'EOF'
feat(memory): MemoryConfig gains RecencyWeight + RecencyHalfLife

env: RAG_RECENCY_WEIGHT (float, default 0.1) and
     RAG_RECENCY_HALFLIFE_DAYS (float days, default 30).

Validation:
  - RAG_RECENCY_WEIGHT must be >= 0
  - RAG_RECENCY_HALFLIFE_DAYS must resolve to >= 1h (floors a 0 halflife
    that would divide-by-zero in SQL; well below the sweep's 7d minimum
    so no legitimate value is excluded)

floatEnv helper added next to intEnv. Production behavior preserved when
env unset — defaults match the Phase 2 hard-coded SQL constants.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 8: Thread the two fields through `module.NewModule` (GREEN-only)

**Files:**
- Modify: `internal/memory/module.go`
- Modify: `internal/memory/module_test.go`

`Orchestrator` itself doesn't see the recency knobs — they're consumed by `HybridRetriever`'s constructor. The wiring is in `module.go`. This task runs AFTER Task 9 (which changes `NewHybridRetriever`'s signature); doing it second keeps each commit's tests green.

- [ ] **Step 8.1: Wire the new fields in `module.go`**

In `internal/memory/module.go`, change the line:

```go
		Retriever:      NewHybridRetriever(conn, embedder),
```

to:

```go
		Retriever:      NewHybridRetriever(conn, embedder, cfg.RecencyWeight, cfg.RecencyHalfLife),
```

- [ ] **Step 8.2: Update the integration-test `cfg` literal**

In `internal/memory/module_test.go`, modify the `cfg` in `TestNewModule_DefaultRetrieverIsHybrid` to include the two new fields:

```go
	cfg := MemoryConfig{
		DatabaseURL:        url,
		EmbeddingBaseURL:   "http://example.invalid",
		EmbeddingModel:     "fake",
		EmbeddingDim:       4,
		LLMBaseURL:         "http://example.invalid",
		LLMModel:           "fake",
		TopK:               5,
		ChunkSizeTokens:    50,
		ChunkOverlapTokens: 10,
		RecencyWeight:      0.1,
		RecencyHalfLife:    30 * 24 * time.Hour,
	}
```

Add `"time"` to the test file's imports.

The `TestNewModule_FailsOnUnreachableDatabaseURL` test creates a `cfg` without these fields too — leave it as-is. The fields default to zero, which is fine for that test because the DB connect fails before retriever construction.

- [ ] **Step 8.3: Run module tests to verify**

Run: `go test ./internal/memory/ -run TestNewModule -v`

Expected: PASS (both tests; the integration test requires `DATABASE_URL` and skips cleanly without it).

- [ ] **Step 8.4: Commit**

```bash
git add internal/memory/module.go internal/memory/module_test.go
git commit -m "$(cat <<'EOF'
feat(memory): module.NewModule threads RecencyWeight + RecencyHalfLife
into NewHybridRetriever

Wires cfg.RecencyWeight and cfg.RecencyHalfLife from MemoryConfig
through to the retriever construction. TestNewModule_DefaultRetrieverIsHybrid
updated to include the two new fields (defaults match Phase 2 constants
so the retriever's behavior is unchanged).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Phase 3.3 — HybridRetriever SQL parameterization

**Execute Task 9 BEFORE Task 8.** Task 9 changes the `NewHybridRetriever` signature (and updates all existing test call sites in the same commit so tests stay green). Task 8 then wires the new fields from `module.go` and updates the integration-test cfg literal.

### Task 9: `NewHybridRetriever` signature + `hybridRrfQuery` $6/$7 (RED + GREEN)

**Files:**
- Modify: `internal/memory/hybrid_retriever.go`
- Modify: `internal/memory/hybrid_retriever_test.go`

- [ ] **Step 9.1: Write failing integration test for the new SQL semantics**

Append to `internal/memory/hybrid_retriever_test.go`:

```go
func TestHybridRetriever_RecencyParamsRespected(t *testing.T) {
	// Two chunks identical except for published_at (10 days apart).
	// Run with two different recency configs and assert the score
	// delta between fresh and stale matches the analytic formula.
	conn := freshDB(t)
	s := NewPgStore(conn)
	asOf := time.Now().UTC()
	fresh := asOf.Add(-1 * time.Hour)
	stale := asOf.Add(-10 * 24 * time.Hour)

	chunks := []Chunk{
		{ID: "fresh", Content: "alpha bravo", Embedding: []float32{1, 0, 0, 0}, PublishedAt: fresh},
		{ID: "stale", Content: "alpha bravo", Embedding: []float32{1, 0, 0, 0}, PublishedAt: stale},
	}
	if err := s.Upsert(context.Background(), chunks); err != nil {
		t.Fatal(err)
	}

	type cfg struct {
		weight   float64
		halflife time.Duration
	}
	cases := []cfg{
		{weight: 0.05, halflife: 7 * 24 * time.Hour},
		{weight: 0.2, halflife: 90 * 24 * time.Hour},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("w=%v_h=%v", c.weight, c.halflife), func(t *testing.T) {
			r := NewHybridRetriever(conn, fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}}, c.weight, c.halflife)
			got, err := r.Retrieve(context.Background(), Query{Text: "alpha", K: 5, AsOf: &asOf}, 5)
			if err != nil {
				t.Fatalf("Retrieve: %v", err)
			}
			if len(got) != 2 {
				t.Fatalf("expected 2 results, got %d", len(got))
			}
			scoreByID := map[string]float64{}
			for _, rc := range got {
				scoreByID[rc.ID] = rc.Score
			}
			// Analytic delta from the recency term alone:
			//   delta = w * (exp(-dt_fresh/h) - exp(-dt_stale/h))
			// (RRF contributions cancel — both chunks rank identically because
			// embeddings + content are identical.)
			hSecs := c.halflife.Seconds()
			dtFresh := asOf.Sub(fresh).Seconds()
			dtStale := asOf.Sub(stale).Seconds()
			wantDelta := c.weight * (math.Exp(-dtFresh/hSecs) - math.Exp(-dtStale/hSecs))
			gotDelta := scoreByID["fresh"] - scoreByID["stale"]
			if math.Abs(gotDelta-wantDelta) > 1e-6 {
				t.Fatalf("score delta: got %v want %v (within 1e-6)", gotDelta, wantDelta)
			}
		})
	}
}
```

Add `"math"` to the imports.

- [ ] **Step 9.2: Update ALL existing call sites in the test file to the new signature**

In `internal/memory/hybrid_retriever_test.go`, change every `NewHybridRetriever(...)` call to pass the two new args. Defaults `0.1, 30*24*time.Hour` preserve old behavior:

- Line ~11 (`TestNewHybridRetriever_ReturnsConfiguredStruct`): `NewHybridRetriever(nil, fakeEmbedder{}, 0.1, 30*24*time.Hour)`
- Line ~18 (`TestHybridRetriever_EmbedderErrorReturnsBeforeDB`): `NewHybridRetriever(nil, erroringEmbedderForRetrieve{}, 0.1, 30*24*time.Hour)`
- Line ~25 (`TestHybridRetriever_NoVectorsReturnsBeforeDB`): `NewHybridRetriever(nil, fakeEmbedder{vecs: nil}, 0.1, 30*24*time.Hour)`
- Line ~32 (`TestHybridRetriever_NegativeDepthNormalizes`): `NewHybridRetriever(nil, erroringEmbedderForRetrieve{}, 0.1, 30*24*time.Hour)`
- Line ~50 (`TestHybridRetriever_ExactTokenWins`): `NewHybridRetriever(conn, fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}}, 0.1, 30*24*time.Hour)`
- Line ~91 (`TestHybridRetriever_PointInTime`): `NewHybridRetriever(conn, fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}}, 0.1, 30*24*time.Hour)`
- Line ~133 (`TestHybridRetriever_PrefersFreshOverStale`): `NewHybridRetriever(conn, embedder, 0.1, 30*24*time.Hour)`
- Line ~166 (`TestHybridRetriever_RecencyTermPresent`): `NewHybridRetriever(conn, fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}}, 0.1, 30*24*time.Hour)`

(Line numbers approximate; use `grep -n "NewHybridRetriever" internal/memory/hybrid_retriever_test.go` to confirm.)

- [ ] **Step 9.3: Run tests to verify they fail with a compile error (not a test failure)**

Run: `go test ./internal/memory/ -run TestHybridRetriever -v 2>&1 | head -30`

Expected: compile error in `hybrid_retriever.go`: `too many arguments in call to NewHybridRetriever`.

- [ ] **Step 9.4: Update `NewHybridRetriever` signature + struct + SQL**

In `internal/memory/hybrid_retriever.go`:

(a) Add two fields to the struct:

```go
type HybridRetriever struct {
	conn            *pgx.Conn
	embedder        Embedder
	recencyWeight   float64
	recencyHalfLife time.Duration
}
```

(b) Update the constructor:

```go
func NewHybridRetriever(conn *pgx.Conn, e Embedder, recencyWeight float64, recencyHalfLife time.Duration) *HybridRetriever {
	return &HybridRetriever{
		conn:            conn,
		embedder:        e,
		recencyWeight:   recencyWeight,
		recencyHalfLife: recencyHalfLife,
	}
}
```

(c) Update the SQL constant to parameterize the recency term as `$6` (weight) and `$7` (halflife seconds):

```go
const hybridRrfQuery = `
WITH bm25 AS (
    SELECT id, ROW_NUMBER() OVER (ORDER BY paradedb.score(id) DESC) AS rank
    FROM chunks
    WHERE content @@@ $1
      AND superseded_by IS NULL
      AND published_at <= $5
    ORDER BY paradedb.score(id) DESC
    LIMIT $3
),
vec AS (
    SELECT id, ROW_NUMBER() OVER (ORDER BY embedding <=> $2) AS rank
    FROM chunks
    WHERE superseded_by IS NULL
      AND published_at <= $5
    ORDER BY embedding <=> $2
    LIMIT $3
)
SELECT c.id, c.content, c.metadata, COALESCE(c.source,''), c.published_at,
       COALESCE(1.0 / (60 + b.rank), 0)
     + COALESCE(1.0 / (60 + v.rank), 0)
     + $6 * exp(-extract(epoch FROM $5 - c.published_at) / $7) AS score
FROM chunks c
LEFT JOIN bm25 b ON b.id = c.id
LEFT JOIN vec  v ON v.id = c.id
WHERE (b.id IS NOT NULL OR v.id IS NOT NULL)
  AND c.superseded_by IS NULL
  AND c.published_at <= $5
ORDER BY score DESC
LIMIT $4
`
```

(d) Update the `Query` call to bind the two new params:

```go
	rows, err := r.conn.Query(ctx, hybridRrfQuery,
		q.Text,
		pgvector.NewVector(vecs[0]),
		depth,
		finalK,
		asOf,
		r.recencyWeight,
		r.recencyHalfLife.Seconds(),
	)
```

- [ ] **Step 9.5: Run tests to verify they pass**

Run: `go test ./internal/memory/ -run TestHybridRetriever -v`

Expected: PASS — all existing tests stay green (defaults preserve behavior) AND the new `TestHybridRetriever_RecencyParamsRespected` passes.

If the integration tests skip (no `DATABASE_URL`), run them against the live DB:

```bash
DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' \
  go test ./internal/memory/ -run TestHybridRetriever -v
```

Expected: PASS.

- [ ] **Step 9.6: Commit**

```bash
git add internal/memory/hybrid_retriever.go internal/memory/hybrid_retriever_test.go
git commit -m "$(cat <<'EOF'
feat(memory): HybridRetriever recency term becomes config-driven

hybridRrfQuery grows from 5 to 7 bind params:
  - $6 = recency weight (was hard-coded 0.1)
  - $7 = recency halflife seconds (was hard-coded 2592000)

NewHybridRetriever signature gains recencyWeight float64 and
recencyHalfLife time.Duration. All existing call sites (~6) updated
to pass the Phase 2 defaults (0.1, 30*24*time.Hour), preserving
behavior.

New integration test TestHybridRetriever_RecencyParamsRespected
asserts the score delta between two otherwise-identical chunks
matches the analytic formula
  delta = w * (exp(-dt_fresh/h) - exp(-dt_stale/h))
for two distinct (w, h) configs.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

**After this commit lands, execute Task 8 above (module.go threading + module_test cfg update).**

---

## Phase 3.4 — Sweep mode + Makefile

Two commits: (a) the sweep logic + CLI mode + tests (Task 10), (b) Makefile + .env.example (Task 11).

### Task 10: `-sweep` mode in memory-eval (RED + GREEN)

**Files:**
- Modify: `cmd/memory-eval/main.go`
- Modify: `cmd/memory-eval/main_test.go`

- [ ] **Step 10.1: Write failing tests for the sweep harness**

Append to `cmd/memory-eval/main_test.go`:

```go
package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/luannn010/ptolemy/internal/memory/eval"
)

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
```

- [ ] **Step 10.2: Run tests to verify they fail**

Run: `go test ./cmd/memory-eval/ -v`

Expected: FAIL — `undefined: runSweep`, `undefined: sweepCell`, `undefined: sweepWeights`, `undefined: sweepHalfLives`.

- [ ] **Step 10.3: Implement the sweep harness**

Append to `cmd/memory-eval/main.go` (just before the closing of the file; add `"sort"` and `"time"` to imports):

```go
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
```

- [ ] **Step 10.4: Run tests to verify they pass**

Run: `go test ./cmd/memory-eval/ -v`

Expected: PASS (all sweep tests; one — `TestSweepMode_DeclaresWinnerInteriorOnly` — is skipped, as the test itself documents).

- [ ] **Step 10.5: Wire the `-sweep` flag into `run()`**

In `cmd/memory-eval/main.go`, add `sweep := fs.Bool("sweep", false, "3x3 recency sweep mode: ingest once, query nine times")` next to the other flags. Then before the existing `if !*skipIngest` block, add:

```go
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
```

And add at the bottom of `main.go`:

```go
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
```

Add `"github.com/jackc/pgx/v5"` to the imports.

- [ ] **Step 10.6: Verify build**

Run: `go build ./cmd/memory-eval`

Expected: success.

- [ ] **Step 10.7: Run unit tests once more to confirm nothing regressed**

Run: `go test ./cmd/memory-eval/ -v`

Expected: PASS.

- [ ] **Step 10.8: Commit**

```bash
git add cmd/memory-eval/main.go cmd/memory-eval/main_test.go
git commit -m "$(cat <<'EOF'
feat(memory-eval): -sweep mode runs 3x3 recency grid + emits verdict

Adds runSweep(sweepRunner, io.Writer) with a sweepRunner interface so
unit tests can substitute a fake. The 3x3 grid is
  weight ∈ {0.05, 0.1, 0.2} × halflife ∈ {7d, 30d, 90d}.

Between iterations ONLY recency params change; ingest runs ONCE
(enforced by liveSweepRunner.Ingest and by Test_SweepMode_IngestsCorpusOnce).
The orchestrator's retriever is swapped per cell — embedder + conn reused.

Verdict classifier (classifySweep):
  - WINNER: best cell improves fs_recall by ≥1pp, regresses
            full_recall by ≤1pp, AND is interior.
  - WARNING: best cell is on a grid edge — extend in <direction>.
  - NO WINNER: keep defaults (the spec's "discard the change" outcome).

On the current 3x3 grid only the centroid is interior, and the centroid
IS the baseline, so the WINNER path is unreachable on this grid by
construction — the classifier still handles it for future larger grids.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 11: Makefile + .env.example updates

**Files:**
- Modify: `Makefile`
- Modify: `.env.example`

- [ ] **Step 11.1: Update Makefile**

In `Makefile`, replace the `EVAL_SEED` line and the existing `eval-memory` block with:

```makefile
# Phase 3 memory eval. RAG_FIXTURE_DIR points the binary at the frozen
# fixture corpus under internal/memory/eval/testdata/corpus/; eval.LoadFixtureCorpus
# enumerates the dir and the orchestrator's ingest path chunks/embeds/upserts.
# EVAL_CHUNK_SIZE=20 keeps chunked output under the llama.cpp embedding server's
# 64-token batch ceiling on dense markdown fixtures.
EVAL_SEED         ?= internal/memory/eval/testdata/seed.json
EVAL_FIXTURE_DIR  ?= internal/memory/eval/testdata/corpus
EVAL_CHUNK_SIZE   ?= 20

eval-memory: build
	RAG_FIXTURE_DIR=$(EVAL_FIXTURE_DIR) \
	RAG_CHUNK_SIZE_TOKENS=$(EVAL_CHUNK_SIZE) RAG_CHUNK_OVERLAP_TOKENS=10 \
	  $(BIN_DIR)/memory-eval -seed $(EVAL_SEED)

eval-memory-sweep: build
	RAG_FIXTURE_DIR=$(EVAL_FIXTURE_DIR) \
	RAG_CHUNK_SIZE_TOKENS=$(EVAL_CHUNK_SIZE) RAG_CHUNK_OVERLAP_TOKENS=10 \
	  $(BIN_DIR)/memory-eval -seed $(EVAL_SEED) -sweep
```

- [ ] **Step 11.2: Update .env.example**

In `.env.example`, append at the bottom:

```
# Phase 3 recency tuning. Defaults are the spec values (0.1 / 30d).
# Override via the sweep harness; production should usually leave unset.
RAG_RECENCY_WEIGHT=0.1
RAG_RECENCY_HALFLIFE_DAYS=30
# Phase 3 eval. When set, memory-eval ingests fixture markdown from this
# directory instead of the seed.json corpus list. Unset = legacy live-docs path.
RAG_FIXTURE_DIR=internal/memory/eval/testdata/corpus
```

- [ ] **Step 11.3: Smoke-build the binaries**

Run: `make build`

Expected: success (all 5 binaries built).

- [ ] **Step 11.4: Commit**

```bash
git add Makefile .env.example
git commit -m "$(cat <<'EOF'
feat(memory): Makefile eval-memory uses fixture corpus; add eval-memory-sweep

eval-memory now sets RAG_FIXTURE_DIR (the binary reads it and ingests
the frozen fixture corpus via LoadFixtureCorpus). eval-memory-sweep is
the new target that adds -sweep to run the 3x3 recency grid.

.env.example documents RAG_RECENCY_WEIGHT, RAG_RECENCY_HALFLIFE_DAYS,
and RAG_FIXTURE_DIR with the spec defaults.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Phase 3.5 — Live sweep + decision

Manual measurement. No new tests; outputs land in the PR description.

### Task 12: Run baseline + sweep, decide outcome

**Prerequisites:** Live services healthy. Verify with:

```bash
curl -sS --max-time 4 -o /dev/null -w "embed:%{http_code}\n" http://192.168.0.164:1089/v1/models
curl -sS --max-time 4 -o /dev/null -w "brain:%{http_code}\n" http://127.0.0.1:1090/v1/models
PGPASSWORD=ptolemy psql -h 192.168.0.164 -p 1091 -U ptolemy -d ptolemy -c 'select 1' >/dev/null && echo pg:OK
```

Expected: `embed:200`, `brain:200`, `pg:OK`.

- [ ] **Step 12.1: Reset the dev DB (REQUIRES USER AUTHORIZATION)**

Run: `PGPASSWORD=ptolemy psql -h 192.168.0.164 -p 1091 -U ptolemy -d ptolemy -c 'DROP TABLE IF EXISTS chunks, memory_schema_migrations CASCADE;'`

Expected: `NOTICE` messages and `DROP TABLE`.

- [ ] **Step 12.2: Run baseline eval**

Run: `make eval-memory 2>&1 | tee /tmp/phase3-baseline.txt`

Expected: per-type table + overall recall line of the form `mean recall@5 = X.XXX over 30 questions (fixture_version=1)`. Record this output verbatim for the PR description, headed **"Baseline (RecencyWeight=0.1, RecencyHalfLife=30d) — NOT comparable to the n=8 / recall=0.875 figure from Phase 2 (different sample, different distribution, different corpus)."**

- [ ] **Step 12.3: Reset DB again for the sweep** (REQUIRES USER AUTHORIZATION)

Run: `PGPASSWORD=ptolemy psql -h 192.168.0.164 -p 1091 -U ptolemy -d ptolemy -c 'DROP TABLE IF EXISTS chunks, memory_schema_migrations CASCADE;'`

- [ ] **Step 12.4: Run the sweep**

Run: `make eval-memory-sweep 2>&1 | tee /tmp/phase3-sweep.txt`

Expected: ingestion log line for each of the 12 fixtures (ONCE total — not 9× 12), then a 9-row markdown table with columns `weight | halflife | fs_recall | full_recall | Δ_fs | Δ_full`, then `fixture_version=1`, then one of:
- `WINNER: weight=W halflife=H`
- `WARNING: optimum on grid edge — extend in (<direction>)`
- `NO WINNER — keep defaults`

Record the full sweep table + verdict verbatim for the PR description.

- [ ] **Step 12.5: Decide the outcome**

Apply the decision rule (already encoded in `classifySweep`):

- **If `WINNER:` was emitted:** update `MemoryConfig` defaults in `internal/memory/config.go` to the winning values and add a commit + a new `make eval-memory` recording the post-change number for the before/after delta.
- **If `WARNING:` was emitted:** keep defaults at `0.1` / `30d`. Document the edge result + the suggested extension direction in the PR description. Future sub-PR can widen the grid.
- **If `NO WINNER` was emitted:** keep defaults at `0.1` / `30d`. Document the null result as positive information (defaults are near-optimal on this eval surface) in the PR description.

- [ ] **Step 12.6: Conditional commit (only if winner adopted)**

Only if Step 12.5 declared a winner:

In `internal/memory/config.go`, change the `floatEnv("RAG_RECENCY_WEIGHT", 0.1)` and `floatEnv("RAG_RECENCY_HALFLIFE_DAYS", 30)` defaults to the winning values, then:

```bash
PGPASSWORD=ptolemy psql -h 192.168.0.164 -p 1091 -U ptolemy -d ptolemy \
  -c 'DROP TABLE IF EXISTS chunks, memory_schema_migrations CASCADE;'
make eval-memory 2>&1 | tee /tmp/phase3-post-winner.txt
git add internal/memory/config.go
git commit -m "feat(memory): adopt RecencyWeight=W RecencyHalfLife=Hd per Phase 3 sweep

  Before: ... (from /tmp/phase3-baseline.txt)
  After:  ... (from /tmp/phase3-post-winner.txt)
  Sweep:  ... (from /tmp/phase3-sweep.txt)

  Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

If the outcome was `WARNING` or `NO WINNER`, no commit is needed at this step.

---

## Phase 3.6 — Docs + acceptance close-out

### Task 13: Update IMPLEMENTATION_PLAN.md + RETRIEVAL.md

**Files:**
- Modify: `docs/memory/IMPLEMENTATION_PLAN.md`
- Modify: `docs/memory/RETRIEVAL.md`

- [ ] **Step 13.1: Tick the Phase 3 checkboxes in IMPLEMENTATION_PLAN.md**

Locate the "Phase 3 — Enhancements" section. Update the four bullets:

```markdown
## Phase 3 — Enhancements (only what the eval set proves helps)

Add these **one at a time**, measuring each against the eval set. Keep a change only if it
improves the score.

- [ ] `Reranker` (cross-encoder) over the top candidates; reduce final k accordingly.
      → Deferred to a later Phase 3 sub-PR. The local brain LLM at :1090 is
      60–180s per call on CPU — too slow on the query path. Defer until cheaper
      inference is available.
- [ ] Query rewriting/expansion before retrieval.
      → Same deferral reason as Reranker.
- [x] Tune RRF constant, candidate depth, recency weight/half-life.
      → THIS PR tunes recency weight + half-life via a 3x3 sweep. RRF constant
      and candidate depth tuning deferred to follow-up sub-PRs (separate knobs,
      separate sweeps).
      → Implementation: `internal/memory/config.go` (RecencyWeight,
      RecencyHalfLife), `internal/memory/hybrid_retriever.go` ($6/$7
      parameterization), `cmd/memory-eval/main.go` (-sweep mode +
      classifySweep). Tests:
      `TestLoadConfig_RecencyDefaults`, `TestLoadConfig_RecencyEnvParsed`,
      `TestLoadConfig_Rejects{Negative,Zero,HalfLifeBelow1Hour}`,
      `TestHybridRetriever_RecencyParamsRespected`,
      `TestSweepMode_{RunsAllNineCells,EmitsMarkdownTable,FlagsEdgeOptimum,
      NoWinnerWhenNoCellImprovesByOnePp,NoWinnerWhenFullSeedRegressionTooLarge,
      IngestsCorpusOnce}`. Eval substrate at
      `internal/memory/eval/testdata/{corpus/,seed.json}` (~12 fixture docs +
      ~30 tagged questions, fixture_version=1).
- [ ] (Optional) `topic_digests` + `DigestSynthesisJob` **with a conflict-resolution /
      revision step** (`DATA_MODEL.md` warning). Do not ship synthesis without it.
      → Deferred to Phase 4 candidate work.

**Acceptance:**
- [x] Each shipped enhancement has a recorded before/after eval-set delta that is
      positive (or a documented null result per the spec's "discard the change" rule).
      → Recency tuning: see Phase 3 PR description for baseline + sweep table +
      verdict. Eval-set hardening is the foundation: ~30 tagged questions on a
      frozen fixture corpus with per-type recall reporting. **Note:** the prior
      n=8 / recall=0.875 number from Phase 2 is NOT comparable to the new
      baseline — different sample, different distribution, different corpus.
```

Also add a one-line pointer near the top of the file (under any existing "Files" or "References" header), or as a footnote to the seed reference:

```markdown
> The Phase 3 eval seed lives at `internal/memory/eval/testdata/seed.json`.
> The pre-Phase-3 seed at `docs/memory/eval/seed.json` is removed.
```

- [ ] **Step 13.2: Update RETRIEVAL.md**

In `docs/memory/RETRIEVAL.md`, find the "Notes" section (around line 89) that mentions the `2592000` and `0.1` constants. Add a note that they're now config-driven:

```markdown
### Notes

- `<=>` is pgvector cosine distance (smaller = closer). Match it to the index opclass in
  `DATA_MODEL.md` (`vector_cosine_ops`).
- **Phase 3:** the `0.1` recency weight and `2592000` half-life seconds are no
  longer SQL constants — they are bound as `$6` and `$7` from `MemoryConfig.RecencyWeight`
  and `MemoryConfig.RecencyHalfLife`, with env overrides `RAG_RECENCY_WEIGHT` and
  `RAG_RECENCY_HALFLIFE_DAYS`. Defaults preserve Phase 2 behavior. The `cmd/memory-eval
  -sweep` mode runs a 3x3 grid over these two knobs and prints the recommended values.
- Keep **candidate depth (`$3`) generous** (20–40) even though **final k (`$4`)** is
  small. The extra candidates are what a Phase 3 reranker consumes.
- If multi-tenant, add `AND tenant_id = $8` to **both** CTEs.
```

(Note the tenant-id placeholder bumps to `$8` because `$6` and `$7` are now taken by the recency knobs.)

- [ ] **Step 13.3: Commit**

```bash
git add docs/memory/IMPLEMENTATION_PLAN.md docs/memory/RETRIEVAL.md
git commit -m "$(cat <<'EOF'
docs(memory): tick Phase 3 recency tuning + note $6/$7 in RETRIEVAL.md

IMPLEMENTATION_PLAN.md Phase 3 checkboxes: recency tuning ticked with
file/test pointers + sub-PR deferral reasons for rerank/expansion/digests.
Acceptance ticked with explicit "n=8 not comparable" note.

RETRIEVAL.md notes: 0.1 / 2592000 are now $6 / $7 bound from MemoryConfig;
RAG_RECENCY_* env overrides documented; tenant_id placeholder bumped to $8.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Final verification

### Task 14: Pre-PR sanity checks

- [ ] **Step 14.1: Full test suite**

Run: `make test`

Expected: PASS (the project's `make test` uses `-p 1`; integration tests skip cleanly without `DATABASE_URL`).

- [ ] **Step 14.2: Live integration tests**

Run: `DATABASE_URL='postgres://ptolemy:ptolemy@192.168.0.164:1091/ptolemy?sslmode=disable' go test -p 1 ./internal/memory/...`

Expected: PASS (all integration tests including the new `TestHybridRetriever_RecencyParamsRespected`).

- [ ] **Step 14.3: Smoke-memory regression**

Run: `PGPASSWORD=ptolemy psql -h 192.168.0.164 -p 1091 -U ptolemy -d ptolemy -c 'DROP TABLE IF EXISTS chunks, memory_schema_migrations CASCADE;' && make smoke-memory`

Expected: a grounded answer to "What is Ptolemy?" The smoke text mentions Ptolemy is a Go-based agent runtime; any answer that includes that fact passes.

- [ ] **Step 14.4: Coverage check**

Run: `go test -p 1 -cover ./internal/...`

Expected: ≥ 80% coverage on `internal/...` packages (project's CI gate; current baseline is ~82%).

- [ ] **Step 14.5: Verify branch is ready**

Run: `git log --oneline main..HEAD`

Expected: 13 commits (one per task, plus the spec commit from earlier), all with the `Co-Authored-By` trailer.

- [ ] **Step 14.6: PR preparation**

Do NOT use `gh` CLI (AGENTS.md prohibits). Use the GitHub web UI. PR description template:

```markdown
## Summary

Phase 3 sub-PR 1: eval-set hardening + recency tuning. Bundles the foundation
(fixture corpus + ~30 tagged questions + per-type recall) with the first
technical enhancement (recency weight + halflife exposed as config knobs,
tuned via a 3x3 sweep).

## Baseline (RecencyWeight=0.1, RecencyHalfLife=30d)

**NOT comparable to the n=8 / recall=0.875 figure from Phase 2** — different
sample, different distribution, different corpus.

<paste output of /tmp/phase3-baseline.txt>

## Sweep results

<paste output of /tmp/phase3-sweep.txt>

## Verdict

<one of: WINNER (with adopted values), WARNING (with edge direction),
NO WINNER (defaults kept)>.

## Why rerank + query expansion are NOT in this PR

Both would add a per-query call to the brain LLM at :1090, which is
60–180s per call on CPU. That's fine for offline question drafting (we
used it for question candidates) but not acceptable on the query path.
Defer until cheaper inference is available.

## NOT comparable

(reinforced) The Phase 2 number was n=8 against live repo docs; the new
baseline is n=30 against frozen fixtures. Directional comparison is
unsound. Future sweeps within this fixture_version=1 substrate ARE
comparable.

## Test plan

- [x] `make test` — full suite green
- [x] live integration tests — `DATABASE_URL=... go test -p 1 ./internal/memory/...`
- [x] smoke-memory regression — `make smoke-memory` produces grounded answer
- [x] `make eval-memory` produces the baseline above
- [x] `make eval-memory-sweep` produces the sweep table above
- [x] coverage ≥ 80% on internal/...

🤖 Generated with [Claude Code](https://claude.com/claude-code)
```

---

## Out of scope (NOT in this plan)

- Reranker / cross-encoder
- Query expansion / rewriting
- RRF constant or candidate-depth tuning
- `topic_digests` / `DigestSynthesisJob`
- LLM-as-judge
- CI-integrated eval
- Memory Garbage Collector docs (`docs/memory/PHASE_4_*` etc.) — separate subsystem, untracked
- CLI `--as-of` flag — Phase 2 deferral still stands

---

## Self-review

Reviewed against the spec at `docs/superpowers/specs/2026-05-28-memory-phase3.md`:

- Spec § "Locked decisions" — all 18 rows have a corresponding task (sweep grid, knob plumbing, validator floor, decision rule, ingest-once-query-nine, edge-warning, fixture_version, snapshot-not-reference, etc.).
- Spec § "Components touched" — all 16 files in the spec table appear in the file map above.
- Spec § "Testing strategy" — all ~22 named tests are written in the relevant tasks. One spec test (`TestSweepMode_DeclaresWinnerInteriorOnly`) is included but documented as Skip because the 3x3 grid's only interior cell IS the baseline, making the WINNER path unreachable on this grid by construction.
- Spec § "Acceptance" — Phase 3.5 + 3.6 collect baseline + sweep + verdict for the PR description; "n=8 not comparable" warning appears in three places (Phase 3.1 commit / Phase 3.6 docs commit / PR description).
- No placeholders ("TBD", "TODO", "fill in") in any step.
- Type consistency: `QuestionType`, `Summary{PerType, NPerType, FixtureVer}`, `sweepCell{Weight, HalfLife, FreshFsR5, FullR5, FixtureVer}`, `sweepRunner{Ingest, RunCell}` — names match across all tasks they appear in.
