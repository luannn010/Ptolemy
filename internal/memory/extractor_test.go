package memory

import (
	"context"
	"testing"
)

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
