package memory

import (
	"math"
	"testing"
)

func TestRrfFusion_ConstantIs60(t *testing.T) {
	// Parens around the composite literal are required: `if RrfFusion{}.…` is
	// ambiguous with the if-block opener and fails to parse otherwise.
	if (RrfFusion{}).constant() != 60 {
		t.Fatalf("RRF C must default to 60 per spec; got %d", (RrfFusion{}).constant())
	}
}

func TestRrfFusion_SingleListPreservesOrder(t *testing.T) {
	in := [][]RetrievedChunk{{
		{Chunk: Chunk{ID: "a"}, Score: 0.9},
		{Chunk: Chunk{ID: "b"}, Score: 0.8},
		{Chunk: Chunk{ID: "c"}, Score: 0.7},
	}}
	out := RrfFusion{}.Fuse(in, 3)
	if len(out) != 3 || out[0].ID != "a" || out[1].ID != "b" || out[2].ID != "c" {
		t.Fatalf("single-list fusion must preserve order, got %+v", ids(out))
	}
}

func TestRrfFusion_TwoListsCombineByRank(t *testing.T) {
	// "shared" appears in both lists; should rank above singletons because
	// rrf("shared") = 1/(60+1) + 1/(60+1) > 1/(60+1) from any one list alone.
	list1 := []RetrievedChunk{
		{Chunk: Chunk{ID: "shared"}},
		{Chunk: Chunk{ID: "only-vec"}},
	}
	list2 := []RetrievedChunk{
		{Chunk: Chunk{ID: "shared"}},
		{Chunk: Chunk{ID: "only-bm25"}},
	}
	out := RrfFusion{}.Fuse([][]RetrievedChunk{list1, list2}, 3)
	if len(out) != 3 || out[0].ID != "shared" {
		t.Fatalf("expected 'shared' to rank first, got %+v", ids(out))
	}
	if out[0].Score <= out[1].Score {
		t.Fatalf("expected shared.Score > singleton.Score, got %v vs %v", out[0].Score, out[1].Score)
	}
	want := 1.0/61 + 1.0/61
	if math.Abs(out[0].Score-want) > 1e-9 {
		t.Fatalf("expected shared.Score=%v, got %v", want, out[0].Score)
	}
}

func TestRrfFusion_HonoursK(t *testing.T) {
	in := [][]RetrievedChunk{{
		{Chunk: Chunk{ID: "a"}},
		{Chunk: Chunk{ID: "b"}},
		{Chunk: Chunk{ID: "c"}},
	}}
	out := RrfFusion{}.Fuse(in, 2)
	if len(out) != 2 {
		t.Fatalf("expected k=2 results, got %d", len(out))
	}
}

func TestRrfFusion_KZeroReturnsAll(t *testing.T) {
	in := [][]RetrievedChunk{{
		{Chunk: Chunk{ID: "a"}},
		{Chunk: Chunk{ID: "b"}},
	}}
	out := RrfFusion{}.Fuse(in, 0)
	if len(out) != 2 {
		t.Fatalf("k<=0 must mean unlimited, got %d", len(out))
	}
}

func TestRrfFusion_EmptyInputReturnsNil(t *testing.T) {
	if out := (RrfFusion{}).Fuse(nil, 5); out != nil {
		t.Fatalf("expected nil for empty input, got %+v", out)
	}
}

func ids(rs []RetrievedChunk) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.ID
	}
	return out
}
