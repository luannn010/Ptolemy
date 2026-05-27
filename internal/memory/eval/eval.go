// Package eval provides the pure logic for the memory module's retrieval
// eval harness: seed loading, hit detection by document-id prefix, and
// recall@k aggregation. The cmd/memory-eval CLI is a thin wrapper.
package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/luannn010/ptolemy/internal/memory"
)

// Seed mirrors the on-disk eval JSON.
type Seed struct {
	K         int          `json:"k"`
	Corpus    []CorpusItem `json:"corpus"`
	Questions []Question   `json:"questions"`
}

// CorpusItem points at a repo-local document the harness should ingest.
// ID becomes the doc id passed to Orchestrator.Ingest; the chunker then
// suffixes "#0", "#1", ... so a question's expected_doc_ids = []string{ID}
// matches any chunk derived from it.
type CorpusItem struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

type Question struct {
	ID             string   `json:"id"`
	Text           string   `json:"text"`
	ExpectedDocIDs []string `json:"expected_doc_ids"`
	Rationale      string   `json:"rationale,omitempty"`
}

type QuestionResult struct {
	Question  Question
	Retrieved []memory.RetrievedChunk
	Hits      []string // expected doc ids that the retrieved list covered
	Expected  []string // copy of question.ExpectedDocIDs for Summarize
}

type Summary struct {
	Total      int
	MeanRecall float64
}

func LoadSeed(path string) (Seed, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Seed{}, fmt.Errorf("read seed: %w", err)
	}
	var s Seed
	if err := json.Unmarshal(data, &s); err != nil {
		return Seed{}, fmt.Errorf("parse seed: %w", err)
	}
	if s.K <= 0 {
		s.K = 5
	}
	return s, nil
}

// HitsExpected returns the subset of expectedDocIDs that have at least one
// retrieved chunk whose id starts with "<expected>#". The "#" guard prevents
// "doc1" from matching "doc10".
func HitsExpected(retrieved []memory.RetrievedChunk, expectedDocIDs []string) []string {
	var hits []string
	for _, want := range expectedDocIDs {
		for _, rc := range retrieved {
			if strings.HasPrefix(rc.ID, want+"#") {
				hits = append(hits, want)
				break
			}
		}
	}
	return hits
}

// RunRetrieval executes the retriever for each question and returns per-question
// results. It deliberately does NOT call the Generator — recall@k is purely a
// retrieval-quality measure and LLM calls would slow the harness 10–100x.
func RunRetrieval(ctx context.Context, r memory.Retriever, s Seed) ([]QuestionResult, error) {
	depth := s.K * 4 // generous candidate depth; reranker fodder for Phase 3
	if depth < 20 {
		depth = 20
	}
	results := make([]QuestionResult, 0, len(s.Questions))
	for _, q := range s.Questions {
		got, err := r.Retrieve(ctx, memory.Query{Text: q.Text, K: s.K}, depth)
		if err != nil {
			return nil, fmt.Errorf("retrieve %s: %w", q.ID, err)
		}
		topK := got
		if len(topK) > s.K {
			topK = topK[:s.K]
		}
		results = append(results, QuestionResult{
			Question:  q,
			Retrieved: topK,
			Hits:      HitsExpected(topK, q.ExpectedDocIDs),
			Expected:  q.ExpectedDocIDs,
		})
	}
	return results, nil
}

// Summarize computes mean recall@k over results. Questions with empty
// Expected are excluded from BOTH numerator and denominator — a malformed
// seed entry shouldn't silently drag the mean down. Total still counts all
// results so the caller can see how many entries were evaluated.
func Summarize(results []QuestionResult) Summary {
	if len(results) == 0 {
		return Summary{}
	}
	var sum float64
	var counted int
	for _, r := range results {
		if len(r.Expected) == 0 {
			continue
		}
		sum += float64(len(r.Hits)) / float64(len(r.Expected))
		counted++
	}
	if counted == 0 {
		return Summary{Total: len(results), MeanRecall: 0}
	}
	return Summary{
		Total:      len(results),
		MeanRecall: sum / float64(counted),
	}
}
