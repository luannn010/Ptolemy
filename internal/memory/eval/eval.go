// Package eval provides the pure logic for the memory module's retrieval
// eval harness: seed loading, hit detection by document-id prefix, and
// recall@k aggregation. The cmd/memory-eval CLI is a thin wrapper.
package eval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/luannn010/ptolemy/internal/memory"
)

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
	ID             string       `json:"id"`
	Text           string       `json:"text"`
	ExpectedDocIDs []string     `json:"expected_doc_ids"`
	QuestionType   QuestionType `json:"question_type,omitempty"`
	Rationale      string       `json:"rationale,omitempty"`
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
	PerType    map[QuestionType]float64
	NPerType   map[QuestionType]int
	FixtureVer int
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

// MeasureDedup loads the fixture corpus and reports how many docs the dedup pass
// would collapse (normalized-content equality), plus the corpus size. DB-free.
func MeasureDedup(fixtureDir string) (corpusSize, wouldCollapse int, err error) {
	docs, err := LoadFixtureCorpus(fixtureDir)
	if err != nil {
		return 0, 0, err
	}
	return len(docs), memory.MeasureDedupCollapses(docs), nil
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
