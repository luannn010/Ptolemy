package memory

import (
	"context"
	"strings"
	"testing"
)

func TestExtractorPromptV2_Wiring(t *testing.T) {
	if ExtractorVersion != "extract_v2" {
		t.Fatalf("ExtractorVersion = %q, want extract_v2", ExtractorVersion)
	}
	if strings.TrimSpace(extractPromptTemplate) == "" {
		t.Fatal("embedded extract prompt is empty")
	}
	// v2 must advertise the predicate vocabulary so the model emits taxonomy values.
	for _, tok := range []string{"uses", "runs_on", "listens_on", "stores_in", "configured_as"} {
		if !strings.Contains(extractPromptTemplate, tok) {
			t.Fatalf("extract prompt missing predicate vocabulary token %q", tok)
		}
	}
	// v2 must instruct substring-safe content (copy from source, do not paraphrase).
	if !strings.Contains(strings.ToLower(extractPromptTemplate), "copy") {
		t.Fatal("extract prompt should instruct copying content from the source turn")
	}
}

type fakeChat struct{ resp string }

func (f fakeChat) Complete(ctx context.Context, system, user string, opts CompleteOptions) (string, error) {
	return f.resp, nil
}

func TestExtractor_ParsesAndKeepsGrounded(t *testing.T) {
	ex := Exchange{
		UserText:      "how does the GC decide what to archive?",
		AssistantText: "the sweep archives project rows whose decay score falls below the threshold",
	}
	json := `{"atoms":[{"content":"The GC sweep archives project rows below the decay threshold","perspective":"factual","fact_subject":"GC sweep","fact_predicate":"archives"}]}`
	e := NewExtractor(fakeChat{resp: json})
	got, err := e.Extract(context.Background(), ex)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Perspective != "factual" {
		t.Fatalf("want 1 factual entry, got %+v", got)
	}
}

func TestExtractor_KeepsAtomsForValidatorStage(t *testing.T) {
	ex := Exchange{UserText: "what does the sweep do?", AssistantText: "it archives stale rows"}
	json := `{"atoms":[{"content":"It archives stale rows","perspective":"factual","fact_subject":"","fact_predicate":""}]}`
	e := NewExtractor(fakeChat{resp: json})
	got, _ := e.Extract(context.Background(), ex)
	if len(got) != 1 {
		t.Fatalf("extractor should pass semantic checks to validator stage, got %+v", got)
	}
}

func TestExtractor_KeepsUngroundedForValidatorStage(t *testing.T) {
	ex := Exchange{UserText: "how does the GC archive?", AssistantText: "the sweep archives stale rows"}
	json := `{"atoms":[{"content":"Quarterly revenue grew forty percent in Brazil","perspective":"factual","fact_subject":"","fact_predicate":""}]}`
	e := NewExtractor(fakeChat{resp: json})
	got, _ := e.Extract(context.Background(), ex)
	if len(got) != 1 {
		t.Fatalf("extractor should pass semantic checks to validator stage, got %+v", got)
	}
}

func TestExtractor_GroundingAllowsCorefResolved(t *testing.T) {
	ex := Exchange{UserText: "what does it do?", AssistantText: "the sweep implemented archiving of stale project rows"}
	json := `{"atoms":[{"content":"The GC sweep implements archiving of stale project rows","perspective":"factual","fact_subject":"GC sweep","fact_predicate":"implements"}]}`
	e := NewExtractor(fakeChat{resp: json})
	got, _ := e.Extract(context.Background(), ex)
	if len(got) != 1 {
		t.Fatalf("coref-resolved grounded entry wrongly rejected, got %+v", got)
	}
}

func TestExtractor_HandlesEmptyAndFenced(t *testing.T) {
	e := NewExtractor(fakeChat{resp: `{"atoms":[]}`})
	got, err := e.Extract(context.Background(), Exchange{UserText: "hi", AssistantText: "hello"})
	if err != nil || len(got) != 0 {
		t.Fatalf("want empty, got %+v err=%v", got, err)
	}
}

func TestExtractor_ParseError(t *testing.T) {
	e := NewExtractor(fakeChat{resp: `{"atoms":[`})
	_, err := e.Extract(context.Background(), Exchange{UserText: "u", AssistantText: "a"})
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestExtractor_NormalizesUnknownPerspective(t *testing.T) {
	e := NewExtractor(fakeChat{resp: `{"atoms":[{"content":"x","perspective":"weird","fact_subject":"","fact_predicate":""}]}`})
	got, err := e.Extract(context.Background(), Exchange{UserText: "x", AssistantText: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Perspective != "relational" {
		t.Fatalf("expected fallback perspective relational, got %+v", got)
	}
}
