package memory

import "testing"

func TestNonEmptyValidator(t *testing.T) {
	v := NonEmptyValidator{}
	if err := v.Validate(Atom{Content: "ok", Perspective: "factual"}, TurnContext{}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := v.Validate(Atom{Content: " ", Perspective: "factual"}, TurnContext{}); err == nil {
		t.Fatal("expected empty content rejection")
	}
}

func TestLengthBoundsValidator(t *testing.T) {
	v := LengthBoundsValidator{MaxContent: 5, MaxFact: 3}
	if err := v.Validate(Atom{Content: "short", Perspective: "factual", FactSubject: "abc", FactPredicate: "xy"}, TurnContext{}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := v.Validate(Atom{Content: "too long", Perspective: "factual"}, TurnContext{}); err == nil {
		t.Fatal("expected length rejection")
	}
}

func TestPredicateTaxonomyValidator(t *testing.T) {
	v := PredicateTaxonomyValidator{}
	if err := v.Validate(Atom{FactPredicate: "archives"}, TurnContext{}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := v.Validate(Atom{FactPredicate: "not_allowed"}, TurnContext{}); err == nil {
		t.Fatal("expected predicate rejection")
	}
}

func TestEvidenceInSourceValidator(t *testing.T) {
	v := EvidenceInSourceValidator{}
	src := TurnContext{UserText: "GC sweep archives stale rows", AssistantText: ""}
	if err := v.Validate(Atom{Content: "GC sweep archives stale rows"}, src); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := v.Validate(Atom{Content: "totally unrelated fact"}, src); err == nil {
		t.Fatal("expected evidence rejection")
	}
}

func TestValidatorChain_AllPassStores(t *testing.T) {
	c := NewDefaultValidatorChain()
	src := TurnContext{UserText: "GC sweep archives stale rows", AssistantText: ""}
	out := c.Filter([]Atom{{Content: "GC sweep archives stale rows", Perspective: "factual", FactPredicate: "archives"}}, src)
	if len(out) != 1 {
		t.Fatalf("expected 1 kept atom, got %d", len(out))
	}
}

func TestValidatorChain_AnyRejectDrops(t *testing.T) {
	c := NewDefaultValidatorChain()
	src := TurnContext{UserText: "GC sweep archives stale rows", AssistantText: ""}
	out := c.Filter([]Atom{
		{Content: "GC sweep archives stale rows", Perspective: "factual", FactPredicate: "archives"},
		{Content: "not in source", Perspective: "factual", FactPredicate: "archives"},
	}, src)
	if len(out) != 1 {
		t.Fatalf("expected 1 kept atom, got %d", len(out))
	}
}

func TestValidatorChain_LogsRejectionReason(t *testing.T) {
	c := NewDefaultValidatorChain()
	var gotValidator, gotReason string
	c.onReject = func(validator, reason string, atom Atom, source TurnContext) {
		gotValidator = validator
		gotReason = reason
	}
	src := TurnContext{UserText: "GC sweep archives stale rows", AssistantText: ""}
	_ = c.Filter([]Atom{{Content: "not in source", Perspective: "factual", FactPredicate: "archives"}}, src)
	if gotValidator == "" || gotReason == "" {
		t.Fatalf("expected rejection callback details, got validator=%q reason=%q", gotValidator, gotReason)
	}
}
