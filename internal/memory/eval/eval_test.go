package eval

import (
	"os"
	"path/filepath"
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

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
