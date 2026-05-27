package memory

import (
	"fmt"
	"strings"
)

type ContextBuilder interface {
	Build(q Query, chunks []RetrievedChunk) PromptContext
}

// BudgetContextBuilder packs chunks into a rune-budgeted prompt and tracks the
// source ids so the Generator can produce citations the caller can verify.
type BudgetContextBuilder struct {
	MaxRunes int
}

func (b BudgetContextBuilder) Build(q Query, chunks []RetrievedChunk) PromptContext {
	var body strings.Builder
	var ids []string
	used := 0
	for i, c := range chunks {
		piece := fmt.Sprintf("\n[source:%s]\n%s\n", c.ID, c.Content)
		if i > 0 && used+len([]rune(piece)) > b.MaxRunes && b.MaxRunes > 0 {
			break
		}
		body.WriteString(piece)
		ids = append(ids, c.ID)
		used += len([]rune(piece))
	}
	return PromptContext{
		System:    "You are a careful assistant. Answer using only the provided sources and cite them by id in square brackets like [source:id].",
		User:      fmt.Sprintf("Sources:\n%s\n\nQuestion: %s", body.String(), q.Text),
		SourceIDs: ids,
	}
}
