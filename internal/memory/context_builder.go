package memory

import (
	"fmt"
	"math"
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

// MMRContextBuilder selects a diverse, relevant subset via Maximal Marginal
// Relevance, then packs it under a rune budget. Relevance is the retrieval score;
// pairwise similarity is token-set cosine over normalized content. This is the
// read-side relevance trim (do NOT push relevance trimming to capture).
type MMRContextBuilder struct {
	Lambda   float64 // ~0.7
	K        int     // final selection cap (0 = all candidates)
	MaxRunes int
}

func (b MMRContextBuilder) Build(q Query, chunks []RetrievedChunk) PromptContext {
	selected := selectMMR(chunks, b.Lambda, b.K)
	var body strings.Builder
	var ids []string
	used := 0
	for i, c := range selected {
		piece := fmt.Sprintf("\n[source:%s]\n%s\n", c.ID, c.Content)
		if i > 0 && b.MaxRunes > 0 && used+len([]rune(piece)) > b.MaxRunes {
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

func selectMMR(chunks []RetrievedChunk, lambda float64, k int) []RetrievedChunk {
	if len(chunks) == 0 {
		return nil
	}
	if k <= 0 || k > len(chunks) {
		k = len(chunks)
	}
	if lambda < 0 {
		lambda = 0.7 // reject nonsensical negatives only; lambda=0 (pure diversity) is valid
	}
	sets := make([]map[string]int, len(chunks))
	for i, c := range chunks {
		sets[i] = tokenCounts(c.Content)
	}
	chosen := make([]bool, len(chunks))
	var out []RetrievedChunk
	for len(out) < k {
		best, bestScore := -1, 0.0
		for i := range chunks {
			if chosen[i] {
				continue
			}
			maxSim := 0.0
			for j := range chunks {
				if !chosen[j] {
					continue
				}
				if s := cosineCounts(sets[i], sets[j]); s > maxSim {
					maxSim = s
				}
			}
			mmr := lambda*chunks[i].Score - (1-lambda)*maxSim
			if best == -1 || mmr > bestScore {
				best, bestScore = i, mmr
			}
		}
		if best == -1 {
			break
		}
		chosen[best] = true
		out = append(out, chunks[best])
	}
	return out
}

func tokenCounts(s string) map[string]int {
	m := map[string]int{}
	for _, t := range normalizeTokens(s) {
		m[t]++
	}
	return m
}

func cosineCounts(a, b map[string]int) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	var dot, na, nb float64
	for t, av := range a {
		na += float64(av * av)
		if bv, ok := b[t]; ok {
			dot += float64(av * bv)
		}
	}
	for _, bv := range b {
		nb += float64(bv * bv)
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
