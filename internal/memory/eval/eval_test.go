package eval

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luannn010/ptolemy/internal/memory"
)

func TestLoadSeed_ParsesValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "seed.json")
	if err := os.WriteFile(path, []byte(`{
	  "k": 5,
	  "corpus": [{"id": "eval/doc1", "path": "AGENTS.md"}],
	  "questions": [
	    {"id": "q1", "text": "what is this", "expected_doc_ids": ["eval/doc1"]}
	  ]
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSeed(path)
	if err != nil {
		t.Fatalf("LoadSeed: %v", err)
	}
	if s.K != 5 {
		t.Fatalf("expected K=5, got %d", s.K)
	}
	if len(s.Corpus) != 1 || s.Corpus[0].ID != "eval/doc1" {
		t.Fatalf("corpus not parsed: %+v", s.Corpus)
	}
	if len(s.Questions) != 1 || s.Questions[0].Text != "what is this" {
		t.Fatalf("questions not parsed: %+v", s.Questions)
	}
}

func TestLoadSeed_MissingFileErrors(t *testing.T) {
	if _, err := LoadSeed("/no/such/file.json"); err == nil {
		t.Fatalf("expected error for missing file")
	}
}

func TestHitsExpected_PrefixMatchesChunkIDs(t *testing.T) {
	retrieved := []memory.RetrievedChunk{
		{Chunk: memory.Chunk{ID: "eval/doc1#0"}},
		{Chunk: memory.Chunk{ID: "eval/doc2#3"}},
		{Chunk: memory.Chunk{ID: "eval/doc3#1"}},
	}
	hits := HitsExpected(retrieved, []string{"eval/doc1", "eval/doc3", "eval/missing"})
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d: %v", len(hits), hits)
	}
	hitSet := map[string]bool{}
	for _, h := range hits {
		hitSet[h] = true
	}
	if !hitSet["eval/doc1"] || !hitSet["eval/doc3"] {
		t.Fatalf("expected eval/doc1 and eval/doc3 hits, got %v", hits)
	}
}

func TestHitsExpected_RequiresChunkSuffix(t *testing.T) {
	// A retrieved id MUST be docID + "#" + n. A bare docID would be wrong
	// (chunks always carry the suffix), and we don't want false positives
	// from substring matches like "doc1" in "doc10".
	retrieved := []memory.RetrievedChunk{
		{Chunk: memory.Chunk{ID: "eval/doc10#0"}},
	}
	if hits := HitsExpected(retrieved, []string{"eval/doc1"}); len(hits) != 0 {
		t.Fatalf("expected zero hits to avoid 'doc1' matching 'doc10', got %v", hits)
	}
}

func TestSummarize_AveragesRecall(t *testing.T) {
	results := []QuestionResult{
		{Hits: []string{"a"}, Expected: []string{"a"}},          // recall = 1.0
		{Hits: []string{}, Expected: []string{"a"}},             // recall = 0.0
		{Hits: []string{"a"}, Expected: []string{"a", "b"}},     // recall = 0.5
	}
	s := Summarize(results)
	want := (1.0 + 0.0 + 0.5) / 3.0
	if abs(s.MeanRecall-want) > 1e-9 {
		t.Fatalf("expected mean recall %v, got %v", want, s.MeanRecall)
	}
	if s.Total != 3 {
		t.Fatalf("expected Total=3, got %d", s.Total)
	}
}

func TestLoadSeed_DefaultsKToFive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "seed.json")
	// k omitted → defaults to 5 per LoadSeed semantics.
	if err := os.WriteFile(path, []byte(`{"corpus": [], "questions": []}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSeed(path)
	if err != nil {
		t.Fatalf("LoadSeed: %v", err)
	}
	if s.K != 5 {
		t.Fatalf("expected K to default to 5, got %d", s.K)
	}
}

func TestSummarize_EmptyResultsReturnsZero(t *testing.T) {
	s := Summarize(nil)
	if s.Total != 0 || s.MeanRecall != 0 {
		t.Fatalf("expected zero Summary on empty results, got %+v", s)
	}
}

func TestSummarize_FiltersEmptyExpected(t *testing.T) {
	// A question with no expected docs MUST NOT drag the mean recall down.
	// Mean is computed over questions with non-empty Expected only.
	results := []QuestionResult{
		{Hits: []string{"a"}, Expected: []string{"a"}}, // 1.0
		{Hits: nil, Expected: nil},                     // skipped
	}
	s := Summarize(results)
	if abs(s.MeanRecall-1.0) > 1e-9 {
		t.Fatalf("expected MeanRecall=1.0 over the single answered question, got %v", s.MeanRecall)
	}
	if s.Total != 2 {
		t.Fatalf("Total should still count all results, got %d", s.Total)
	}
}

func TestRunRetrieval_UsesFakeRetriever(t *testing.T) {
	// End-to-end run of the eval loop without a DB. Proves that RunRetrieval
	// (a) calls the retriever per question, (b) caps results at seed.K,
	// (c) populates Hits via HitsExpected, (d) does not call any Generator.
	seed := Seed{
		K: 2,
		Questions: []Question{
			{ID: "q1", Text: "alpha", ExpectedDocIDs: []string{"eval/doc1"}},
			{ID: "q2", Text: "beta", ExpectedDocIDs: []string{"eval/doc2"}},
		},
	}
	r := &fakeRetriever{
		responses: map[string][]memory.RetrievedChunk{
			"alpha": {
				{Chunk: memory.Chunk{ID: "eval/doc1#0"}},
				{Chunk: memory.Chunk{ID: "eval/doc1#1"}},
				{Chunk: memory.Chunk{ID: "eval/other#0"}},
			},
			"beta": {
				{Chunk: memory.Chunk{ID: "eval/other#0"}},
			},
		},
	}
	results, err := RunRetrieval(context.Background(), r, seed)
	if err != nil {
		t.Fatalf("RunRetrieval: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if len(results[0].Retrieved) != 2 {
		t.Fatalf("expected K=2 cap on q1 retrieved, got %d", len(results[0].Retrieved))
	}
	if len(results[0].Hits) != 1 || results[0].Hits[0] != "eval/doc1" {
		t.Fatalf("expected q1 to hit eval/doc1, got %v", results[0].Hits)
	}
	if len(results[1].Hits) != 0 {
		t.Fatalf("expected q2 to miss, got %v", results[1].Hits)
	}
}

func TestRunRetrieval_PropagatesRetrieverError(t *testing.T) {
	seed := Seed{
		K: 2,
		Questions: []Question{
			{ID: "q1", Text: "anything", ExpectedDocIDs: []string{"eval/doc1"}},
		},
	}
	r := &erroringRetriever{}
	if _, err := RunRetrieval(context.Background(), r, seed); err == nil {
		t.Fatalf("expected retriever error to propagate")
	}
}

type fakeRetriever struct {
	responses map[string][]memory.RetrievedChunk
}

func (f *fakeRetriever) Retrieve(_ context.Context, q memory.Query, _ int) ([]memory.RetrievedChunk, error) {
	return f.responses[q.Text], nil
}

type erroringRetriever struct{}

func (erroringRetriever) Retrieve(_ context.Context, _ memory.Query, _ int) ([]memory.RetrievedChunk, error) {
	return nil, fmt.Errorf("retriever boom")
}

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

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

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

func TestMeasureDedup_RoundTripsFixtureDir(t *testing.T) {
	dir := t.TempDir()
	// Two normalized-equal docs → 1 would-collapse; one distinct doc stays.
	for name, body := range map[string]string{
		"a.md": "same fact about the sweep",
		"b.md": "same   fact about the sweep", // normalizes equal to a.md
		"c.md": "different fact about billing",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	corpusSize, wouldCollapse, err := MeasureDedup(dir)
	if err != nil {
		t.Fatalf("MeasureDedup: %v", err)
	}
	if corpusSize != 3 {
		t.Fatalf("corpusSize = %d, want 3", corpusSize)
	}
	if wouldCollapse != 1 {
		t.Fatalf("wouldCollapse = %d, want 1", wouldCollapse)
	}
}

func TestMeasureDedup_InvalidDirReturnsError(t *testing.T) {
	_, _, err := MeasureDedup("/no/such/dir/at/all")
	if err == nil {
		t.Fatal("expected error for nonexistent fixture dir")
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

func TestMeasureDedup_CountsNormalizedEqualPairs(t *testing.T) {
	docs := []memory.RawDocument{
		{ID: "a", Text: "user prefers tabs"},
		{ID: "b", Text: "user   prefers tabs"}, // normalized-equal to a
		{ID: "c", Text: "deploy target is AWS"},
		{ID: "d", Text: "deploy target is GCP"}, // similar but NOT equal → not a collapse
	}
	n := memory.MeasureDedupCollapses(docs)
	if n != 1 {
		t.Fatalf("would-collapse count = %d, want 1", n)
	}
}

func TestMeasureDedupCollapses_EdgeCases(t *testing.T) {
	if got := memory.MeasureDedupCollapses(nil); got != 0 {
		t.Errorf("nil slice: got %d, want 0", got)
	}
	if got := memory.MeasureDedupCollapses([]memory.RawDocument{}); got != 0 {
		t.Errorf("empty slice: got %d, want 0", got)
	}
	all := []memory.RawDocument{
		{ID: "a", Text: "same fact"},
		{ID: "b", Text: "same   fact"},
		{ID: "c", Text: "same fact "},
	}
	if got := memory.MeasureDedupCollapses(all); got != 2 {
		t.Errorf("three normalized-equal docs: got %d, want 2 (one survivor, two collapses)", got)
	}
}
