package memory

import "testing"

func TestPassthroughFusion_ReturnsFirstList(t *testing.T) {
	in := []RetrievedChunk{
		{Chunk: Chunk{ID: "a"}, Score: 1},
		{Chunk: Chunk{ID: "b"}, Score: 0.5},
	}
	out := PassthroughFusion{}.Fuse([][]RetrievedChunk{in}, 1)
	if len(out) != 1 || out[0].ID != "a" {
		t.Fatalf("passthrough must keep order and cap k: %+v", out)
	}
}

func TestPassthroughFusion_KEqualToLenReturnsAll(t *testing.T) {
	in := []RetrievedChunk{
		{Chunk: Chunk{ID: "a"}, Score: 1},
		{Chunk: Chunk{ID: "b"}, Score: 0.5},
	}
	out := PassthroughFusion{}.Fuse([][]RetrievedChunk{in}, 2)
	if len(out) != 2 {
		t.Fatalf("k==len must return all: got %d", len(out))
	}
}

func TestPassthroughFusion_KZeroReturnsAll(t *testing.T) {
	in := []RetrievedChunk{{Chunk: Chunk{ID: "a"}}}
	out := PassthroughFusion{}.Fuse([][]RetrievedChunk{in}, 0)
	if len(out) != 1 {
		t.Fatalf("k<=0 must mean unlimited, got %d", len(out))
	}
}

func TestPassthroughFusion_EmptyListsReturnsNil(t *testing.T) {
	out := PassthroughFusion{}.Fuse(nil, 5)
	if out != nil {
		t.Fatalf("expected nil for empty input, got %+v", out)
	}
}
