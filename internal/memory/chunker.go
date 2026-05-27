package memory

import (
	"fmt"
)

type Chunker interface {
	Chunk(doc ParsedDocument) []Chunk
}

// FixedSizeChunker splits text into rune-based windows with overlap. Rune
// arithmetic avoids byte mid-codepoint splits on multi-byte text. The unit is
// runes (not tokens) because tokenization is model-specific and Phase 0 must
// not bind us to a particular tokenizer.
type FixedSizeChunker struct {
	MaxRunes int
	Overlap  int
}

func (c FixedSizeChunker) Chunk(doc ParsedDocument) []Chunk {
	runes := []rune(doc.Text)
	if c.MaxRunes <= 0 || len(runes) <= c.MaxRunes {
		return []Chunk{{
			ID:          fmt.Sprintf("%s#0", doc.ID),
			Content:     doc.Text,
			Metadata:    doc.Metadata,
			Source:      doc.Source,
			PublishedAt: doc.PublishedAt,
		}}
	}
	step := c.MaxRunes - c.Overlap
	if step <= 0 {
		step = c.MaxRunes
	}
	var out []Chunk
	for start, i := 0, 0; start < len(runes); start, i = start+step, i+1 {
		end := start + c.MaxRunes
		if end > len(runes) {
			end = len(runes)
		}
		out = append(out, Chunk{
			ID:          fmt.Sprintf("%s#%d", doc.ID, i),
			Content:     string(runes[start:end]),
			Metadata:    doc.Metadata,
			Source:      doc.Source,
			PublishedAt: doc.PublishedAt,
		})
		if end == len(runes) {
			break
		}
	}
	return out
}
