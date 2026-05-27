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

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
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
