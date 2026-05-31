package memory

import "strings"

// aliasMap expands common terms whose synonyms BM25 treats as unrelated tokens.
// Keys are matched case-insensitively as whole words; values are appended to the
// lexical query. Keep small and in-repo (SPEC-GC §6); measure recall delta before
// trusting (ships behind RAG_ALIAS_EXPANSION, default off).
var aliasMap = map[string][]string{
	"gc":       {"garbage collector"},
	"rag":      {"retrieval augmented generation"},
	"bm25":     {"lexical search"},
	"pgvector": {"vector search"},
	"oob":      {"out of band"},
}

func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

// expandAliases appends known synonyms for any alias term present in the query,
// preserving the original text. Whole-word, case-insensitive match on the term.
func expandAliases(text string) string {
	lowerTokens := map[string]bool{}
	for _, t := range strings.Fields(strings.ToLower(text)) {
		lowerTokens[strings.Trim(t, ".,!?;:()")] = true
	}
	var add []string
	seen := map[string]bool{}
	for term, syns := range aliasMap {
		if lowerTokens[term] {
			for _, syn := range syns {
				if !seen[syn] && !containsFold(text, syn) {
					add = append(add, syn)
					seen[syn] = true
				}
			}
		}
	}
	if len(add) == 0 {
		return text
	}
	return text + " " + strings.Join(add, " ")
}
