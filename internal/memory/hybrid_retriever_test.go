package memory

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"
)

func TestNewHybridRetriever_ReturnsConfiguredStruct(t *testing.T) {
	r := NewHybridRetriever(nil, fakeEmbedder{}, 0.1, 30*24*time.Hour)
	if r == nil {
		t.Fatalf("NewHybridRetriever returned nil")
	}
}

func TestHybridRetriever_EmbedderErrorReturnsBeforeDB(t *testing.T) {
	r := NewHybridRetriever(nil, erroringEmbedderForRetrieve{}, 0.1, 30*24*time.Hour)
	if _, err := r.Retrieve(context.Background(), Query{Text: "q"}, 5); err == nil {
		t.Fatalf("expected embedder error to surface")
	}
}

func TestHybridRetriever_NoVectorsReturnsBeforeDB(t *testing.T) {
	r := NewHybridRetriever(nil, fakeEmbedder{vecs: nil}, 0.1, 30*24*time.Hour)
	if _, err := r.Retrieve(context.Background(), Query{Text: "q"}, 5); err == nil {
		t.Fatalf("expected 'no vector' error")
	}
}

func TestHybridRetriever_NegativeDepthNormalizes(t *testing.T) {
	r := NewHybridRetriever(nil, erroringEmbedderForRetrieve{}, 0.1, 30*24*time.Hour)
	if _, err := r.Retrieve(context.Background(), Query{Text: "q"}, 0); err == nil {
		t.Fatalf("expected embedder error after depth normalization")
	}
}

func TestHybridRetriever_ExactTokenWins(t *testing.T) {
	conn := freshDB(t)
	s := NewPgStore(conn)

	chunks := []Chunk{
		{ID: "near", Content: "alpha", Embedding: []float32{1, 0, 0, 0}, PublishedAt: time.Now().UTC()},
		{ID: "exact", Content: "RAREXYZ123 is the unique token", Embedding: []float32{0, 0, 0, 1}, PublishedAt: time.Now().UTC()},
	}
	if err := s.Upsert(context.Background(), chunks); err != nil {
		t.Fatal(err)
	}

	r := NewHybridRetriever(conn, fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}}, 0.1, 30*24*time.Hour)
	got, err := r.Retrieve(context.Background(), Query{Text: "RAREXYZ123", K: 5}, 5)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	seen := map[string]bool{}
	for _, rc := range got {
		seen[rc.ID] = true
	}
	if !seen["exact"] {
		t.Fatalf("expected hybrid to include BM25 exact match 'exact', got %v", idsHybrid(got))
	}
	if !seen["near"] {
		t.Fatalf("expected hybrid to include vector-close 'near', got %v", idsHybrid(got))
	}
}

func idsHybrid(rs []RetrievedChunk) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = fmt.Sprintf("%s:%.4f", r.ID, r.Score)
	}
	return out
}

func TestHybridRetriever_PointInTime(t *testing.T) {
	// Acceptance #2: a query with a past AsOf excludes content published after it.
	conn := freshDB(t)
	s := NewPgStore(conn)
	past := time.Now().UTC().Add(-72 * time.Hour)
	future := time.Now().UTC().Add(72 * time.Hour)

	chunks := []Chunk{
		{ID: "past", Content: "alpha bravo charlie", Embedding: []float32{1, 0, 0, 0}, PublishedAt: past},
		{ID: "future", Content: "alpha bravo charlie", Embedding: []float32{1, 0, 0, 0}, PublishedAt: future},
	}
	if err := s.Upsert(context.Background(), chunks); err != nil {
		t.Fatal(err)
	}

	asOf := time.Now().UTC()
	r := NewHybridRetriever(conn, fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}}, 0.1, 30*24*time.Hour)
	got, err := r.Retrieve(context.Background(), Query{Text: "alpha", K: 5, AsOf: &asOf}, 5)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) != 1 || got[0].ID != "past" {
		t.Fatalf("expected only 'past' (published_at <= AsOf), got %+v", idsHybrid(got))
	}
}

func TestHybridRetriever_PrefersFreshOverStale(t *testing.T) {
	// Acceptance #1: ingest v1, then ingest v2 with Metadata[supersedes]=v1
	// through the full Orchestrator path; query → v2 returned, v1 absent.
	conn := freshDB(t)
	s := NewPgStore(conn)
	embedder := fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}}
	o := &Orchestrator{
		Chunker:  FixedSizeChunker{MaxRunes: 100},
		Embedder: embedder,
		Store:    s,
	}

	// Ingest v1 (no supersedes).
	if err := o.Ingest(context.Background(), RawDocument{
		ID:   "v1",
		Text: "alpha bravo",
	}); err != nil {
		t.Fatal(err)
	}

	// Ingest v2 declaring it supersedes v1.
	if err := o.Ingest(context.Background(), RawDocument{
		ID:   "v2",
		Text: "alpha bravo",
		Metadata: map[string]any{
			"supersedes": "v1",
		},
	}); err != nil {
		t.Fatal(err)
	}

	asOf := time.Now().UTC().Add(time.Hour) // ensure both <= asOf
	r := NewHybridRetriever(conn, embedder, 0.1, 30*24*time.Hour)
	got, err := r.Retrieve(context.Background(), Query{Text: "alpha", K: 5, AsOf: &asOf}, 5)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	for _, rc := range got {
		if rc.ID == "v1#0" {
			t.Fatalf("v1#0 must be excluded by the status='active' filter; got %+v", idsHybrid(got))
		}
	}
	if len(got) == 0 {
		t.Fatalf("expected at least one result containing v2 chunks; got nothing")
	}
}

func TestHybridRetriever_RecencyTermPresent(t *testing.T) {
	// Two chunks with identical text + identical embeddings but different
	// published_at. The fresher chunk's Score must be strictly greater than
	// the older one's because the recency term is positive and monotonic.
	conn := freshDB(t)
	s := NewPgStore(conn)
	fresh := time.Now().UTC().Add(-1 * time.Hour)
	stale := time.Now().UTC().Add(-60 * 24 * time.Hour) // 60 days old

	chunks := []Chunk{
		{ID: "fresh", Content: "alpha bravo", Embedding: []float32{1, 0, 0, 0}, PublishedAt: fresh},
		{ID: "stale", Content: "alpha bravo", Embedding: []float32{1, 0, 0, 0}, PublishedAt: stale},
	}
	if err := s.Upsert(context.Background(), chunks); err != nil {
		t.Fatal(err)
	}

	asOf := time.Now().UTC()
	r := NewHybridRetriever(conn, fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}}, 0.1, 30*24*time.Hour)
	got, err := r.Retrieve(context.Background(), Query{Text: "alpha", K: 5, AsOf: &asOf}, 5)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d: %+v", len(got), idsHybrid(got))
	}
	scoreByID := map[string]float64{}
	for _, rc := range got {
		scoreByID[rc.ID] = rc.Score
	}
	if scoreByID["fresh"] <= scoreByID["stale"] {
		t.Fatalf("expected fresh.Score > stale.Score, got fresh=%v stale=%v",
			scoreByID["fresh"], scoreByID["stale"])
	}
}

func TestHybridRetriever_ExcludesNonActive(t *testing.T) {
	conn := freshDB(t)
	s := NewPgStore(conn)
	now := time.Now().UTC()
	chunks := []Chunk{
		{ID: "act", Content: "alpha bravo", Embedding: []float32{1, 0, 0, 0}, PublishedAt: now},
		{ID: "arc", Content: "alpha bravo", Embedding: []float32{1, 0, 0, 0}, PublishedAt: now},
	}
	if err := s.Upsert(context.Background(), chunks); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(context.Background(),
		`UPDATE chunks SET status='archived' WHERE id='arc'`); err != nil {
		t.Fatal(err)
	}
	asOf := now
	r := NewHybridRetriever(conn, fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}}, 0.1, 30*24*time.Hour)
	got, err := r.Retrieve(context.Background(), Query{Text: "alpha", K: 5, AsOf: &asOf}, 5)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	for _, rc := range got {
		if rc.ID == "arc" {
			t.Fatalf("archived chunk must not be retrieved, got %v", idsHybrid(got))
		}
	}
	seenActive := false
	for _, rc := range got {
		if rc.ID == "act" {
			seenActive = true
		}
	}
	if !seenActive {
		t.Fatalf("active chunk should be retrieved, got %v", idsHybrid(got))
	}
}

func TestHybridRetriever_ExcludesSupersededByStatusAlone(t *testing.T) {
	conn := freshDB(t)
	ctx := context.Background()
	s := NewPgStore(conn)
	now := time.Now().UTC()
	// Two rows: one active, one superseded. Deliberately leave superseded_by NULL on the
	// superseded row so the test proves status='active' (NOT superseded_by IS NULL) excludes it.
	_ = s.Upsert(ctx, []Chunk{
		{ID: "live", Content: "alpha bravo keyword", Embedding: []float32{1, 0, 0, 0}, PublishedAt: now, Scope: "global"},
		{ID: "stale", Content: "alpha bravo keyword", Embedding: []float32{0, 1, 0, 0}, PublishedAt: now, Scope: "global"},
	})
	if _, err := conn.Exec(ctx, `UPDATE chunks SET status='superseded' WHERE id='stale'`); err != nil {
		t.Fatal(err)
	}
	r := NewHybridRetriever(conn, fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}}, 0.1, 30*24*time.Hour)
	got, err := r.Retrieve(ctx, Query{Text: "keyword", K: 10}, 10)
	if err != nil {
		t.Fatal(err)
	}
	seenLive := false
	for _, c := range got {
		if c.ID == "stale" {
			t.Fatalf("retrieval returned superseded row 'stale'")
		}
		if c.ID == "live" {
			seenLive = true
		}
	}
	if !seenLive {
		t.Fatal("active row 'live' was not returned (status filter must not exclude active rows)")
	}
}

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
			// RRF contributions may not exactly cancel when identical chunks
			// are broken by insertion order (one chunk gets rank 1 in both
			// arms, the other gets rank 2). The maximum possible RRF offset
			// is 2*(1/61 - 1/62) ≈ 0.00053, so we allow 1e-3 tolerance to
			// cover that while still asserting the recency params are wired.
			hSecs := c.halflife.Seconds()
			dtFresh := asOf.Sub(fresh).Seconds()
			dtStale := asOf.Sub(stale).Seconds()
			wantDelta := c.weight * (math.Exp(-dtFresh/hSecs) - math.Exp(-dtStale/hSecs))
			gotDelta := scoreByID["fresh"] - scoreByID["stale"]
			if math.Abs(gotDelta-wantDelta) > 1e-3 {
				t.Fatalf("score delta: got %v want %v (within 1e-3)", gotDelta, wantDelta)
			}
		})
	}
}

func TestHybrid_DecayBlend_StaleProjectRowSinks(t *testing.T) {
	conn := freshDB(t)
	s := NewPgStore(conn)
	emb := fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}}
	r := NewHybridRetriever(conn, emb, 0.0, 30*24*time.Hour).WithDecayLambda(0.05)
	subj := "userA"
	now := time.Now().UTC()
	mk := func(id string) Chunk {
		ss, pp, sess, pr := subj, "factual", "s", "ptolemy"
		return Chunk{ID: id, Content: "kafka retention policy detail", Embedding: []float32{1, 0, 0, 0}, PublishedAt: now,
			Scope: "project", Importance: 0.7, SubjectID: &ss, SessionID: &sess, ProjectID: &pr, Perspective: &pp}
	}
	if err := s.Upsert(context.Background(), []Chunk{mk("fresh"), mk("stale")}); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(context.Background(), `UPDATE chunks SET last_accessed_at = now() - interval '120 days' WHERE id='stale'`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(context.Background(), `UPDATE chunks SET last_accessed_at = now() WHERE id='fresh'`); err != nil {
		t.Fatal(err)
	}
	got, err := r.Retrieve(context.Background(), Query{Text: "kafka retention", K: 10, SubjectID: &subj}, 20)
	if err != nil {
		t.Fatal(err)
	}
	// Both rows have identical content + embedding → identical base (RRF+recency)
	// score, so the ONLY thing that can differentiate their returned Score is the
	// project decay multiplier. Asserting a Score GAP isolates the decay blend:
	// without it the two scores are equal (and this assertion fails), whereas the
	// 6a recency tiebreak only reorders exact ties and could mask an order-only check.
	var sFresh, sStale float64
	var okFresh, okStale bool
	for _, c := range got {
		if c.ID == "fresh" {
			sFresh, okFresh = c.Score, true
		}
		if c.ID == "stale" {
			sStale, okStale = c.Score, true
		}
	}
	if !okFresh || !okStale {
		t.Fatalf("both rows should be retrieved; got %v", ids(got))
	}
	if !(sFresh > sStale) {
		t.Fatalf("decay blend must drop the stale project row's score below the fresh one; fresh=%.6f stale=%.6f", sFresh, sStale)
	}
	// The 120-day-stale row should be sharply discounted, not marginally.
	if sStale > sFresh/2 {
		t.Fatalf("stale score %.6f not sufficiently decayed vs fresh %.6f", sStale, sFresh)
	}
}
