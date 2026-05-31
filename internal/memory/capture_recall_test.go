package memory

import (
	"context"
	"strings"
	"testing"
	"time"
)

// cannedExtractor is a deterministic ChatClient: it returns ONE factual entry
// echoing the user message, so the capture→store→recall path is exercised
// without the BRAIN LLM. (It implements ChatClient.Complete.)
type cannedExtractor struct{}

func (cannedExtractor) Complete(ctx context.Context, system, user string, opts CompleteOptions) (string, error) {
	parts := strings.SplitN(user, "\n\nASSISTANT:\n", 2)
	content := user
	if len(parts) == 2 {
		content = strings.TrimPrefix(parts[1], "ASSISTANT:\n")
	}
	return `{"atoms":[{"content":"` + jsonEscape(content) + `","perspective":"factual","fact_subject":"","fact_predicate":""}]}`, nil
}

func jsonEscape(s string) string {
	out := make([]rune, 0, len(s))
	for _, c := range s {
		switch c {
		case '"', '\\':
			out = append(out, '\\', c)
		case '\n':
			out = append(out, '\\', 'n')
		default:
			out = append(out, c)
		}
	}
	return string(out)
}

func TestCaptureRecall_MultiSession(t *testing.T) {
	conn := freshDB(t)
	store := NewPgStore(conn)
	emb := fakeEmbedder{vecs: [][]float32{{1, 0, 0, 0}}}
	hook := NewCaptureHook(NewExtractor(cannedExtractor{}), emb, store, 8)

	// Note: turns must clear the gate's 16-rune minimum. (The dangling-pronoun
	// filter runs on extracted *content*; here the canned extractor echoes the
	// whole assembled turn string starting "USER:", which never trips it.)
	exA := Exchange{
		UserText:      "synthesis summaries are stored as chunks rows tagged kind synthesis",
		AssistantText: "synthesis summaries are chunks rows distinguished by a metadata marker",
		SubjectID:     "userA", SessionID: "s1", ProjectID: "ptolemy",
	}
	if err := hook.processTurn(context.Background(), exA); err != nil {
		t.Fatalf("processTurn A: %v", err)
	}
	exB := Exchange{
		UserText:      "the billing service uses stripe webhooks for invoice state",
		AssistantText: "stripe webhooks drive invoice state in the billing service",
		SubjectID:     "userB", SessionID: "s2", ProjectID: "other",
	}
	if err := hook.processTurn(context.Background(), exB); err != nil {
		t.Fatalf("processTurn B: %v", err)
	}

	r := NewHybridRetriever(conn, emb, 0.1, 30*24*time.Hour)
	a := "userA"
	got, err := r.Retrieve(context.Background(), Query{Text: "synthesis summaries chunks", K: 10, SubjectID: &a}, 20)
	if err != nil {
		t.Fatal(err)
	}
	var recalledA, leakedB bool
	for _, c := range got {
		if c.SubjectID != nil && *c.SubjectID == "userA" {
			recalledA = true
		}
		if c.SubjectID != nil && *c.SubjectID == "userB" {
			leakedB = true
		}
	}
	if !recalledA {
		t.Fatalf("subject A's captured entry was not recalled; got %v", ids(got))
	}
	if leakedB {
		t.Fatalf("subject B's entry leaked into subject A's recall; got %v", ids(got))
	}
}
