package memory

import "sort"

// RrfFusion is Reciprocal Rank Fusion with the spec's constant C=60. It exists
// for Option B (app-side fusion of independent VectorRetriever + Bm25Retriever
// lists). HybridRetriever's SQL CTE in Phase 1 computes the same math inside
// Postgres; this implementation is the fallback for callers that want to
// inspect or swap individual arms.
type RrfFusion struct{ C int }

func (r RrfFusion) constant() int {
	if r.C <= 0 {
		return 60
	}
	return r.C
}

func (r RrfFusion) Fuse(lists [][]RetrievedChunk, k int) []RetrievedChunk {
	if len(lists) == 0 {
		return nil
	}
	c := r.constant()
	type acc struct {
		chunk RetrievedChunk
		score float64
	}
	by := make(map[string]*acc)
	order := []string{} // first-seen insertion order; used to break ties deterministically
	for _, list := range lists {
		for rank, rc := range list {
			contribution := 1.0 / float64(c+rank+1) // rank is 0-based; +1 makes it 1-based per spec
			if existing, ok := by[rc.ID]; ok {
				existing.score += contribution
			} else {
				cp := rc
				cp.Score = contribution
				by[rc.ID] = &acc{chunk: cp, score: contribution}
				order = append(order, rc.ID)
			}
		}
	}
	out := make([]RetrievedChunk, 0, len(by))
	for _, id := range order {
		a := by[id]
		a.chunk.Score = a.score
		out = append(out, a.chunk)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if k > 0 && k < len(out) {
		out = out[:k]
	}
	return out
}
