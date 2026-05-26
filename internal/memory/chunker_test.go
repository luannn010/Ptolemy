package memory

import (
	"strings"
	"testing"
	"time"
)

func TestFixedSizeChunker_SplitsAndOverlaps(t *testing.T) {
	doc := ParsedDocument{
		ID:          "doc1",
		Text:        strings.Repeat("abcdefghij", 12), // 120 runes
		PublishedAt: time.Now(),
	}
	c := FixedSizeChunker{MaxRunes: 50, Overlap: 10}
	chunks := c.Chunk(doc)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for _, ch := range chunks {
		if len([]rune(ch.Content)) == 0 || len([]rune(ch.Content)) > 50 {
			t.Fatalf("chunk size out of bounds: %d", len([]rune(ch.Content)))
		}
		if ch.ID == "" || !strings.HasPrefix(ch.ID, "doc1#") {
			t.Fatalf("unexpected chunk id %q", ch.ID)
		}
	}
}

func TestFixedSizeChunker_ShortDocSingleChunk(t *testing.T) {
	c := FixedSizeChunker{MaxRunes: 100, Overlap: 10}
	chunks := c.Chunk(ParsedDocument{ID: "d", Text: "tiny", PublishedAt: time.Now()})
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Content != "tiny" {
		t.Fatalf("content: %q", chunks[0].Content)
	}
}

func TestFixedSizeChunker_RejectsBadConfig(t *testing.T) {
	c := FixedSizeChunker{MaxRunes: 0, Overlap: 0}
	chunks := c.Chunk(ParsedDocument{ID: "d", Text: "any", PublishedAt: time.Now()})
	if len(chunks) != 1 || chunks[0].Content != "any" {
		t.Fatalf("zero MaxRunes should yield a single passthrough chunk, got %+v", chunks)
	}
}

func TestFixedSizeChunker_PreservesMetadataAndSource(t *testing.T) {
	doc := ParsedDocument{
		ID:          "doc-m",
		Text:        strings.Repeat("x", 200),
		Source:      "test-src",
		Metadata:    map[string]any{"k": "v"},
		PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	c := FixedSizeChunker{MaxRunes: 80, Overlap: 10}
	chunks := c.Chunk(doc)
	if len(chunks) < 2 {
		t.Fatalf("expected splitting")
	}
	for _, ch := range chunks {
		if ch.Source != "test-src" {
			t.Fatalf("source not preserved: %+v", ch)
		}
		if ch.Metadata["k"] != "v" {
			t.Fatalf("metadata not preserved: %+v", ch.Metadata)
		}
		if !ch.PublishedAt.Equal(doc.PublishedAt) {
			t.Fatalf("published_at not preserved")
		}
	}
}

func TestFixedSizeChunker_HandlesMultibyteRunes(t *testing.T) {
	doc := ParsedDocument{
		ID:          "doc-mb",
		Text:        strings.Repeat("世界你好", 30), // 120 runes, 360 bytes
		PublishedAt: time.Now(),
	}
	c := FixedSizeChunker{MaxRunes: 40, Overlap: 5}
	chunks := c.Chunk(doc)
	for _, ch := range chunks {
		// Reconstructing as runes ensures no broken codepoints.
		got := []rune(ch.Content)
		if len(got) == 0 || len(got) > 40 {
			t.Fatalf("rune count out of bounds: %d", len(got))
		}
	}
}
