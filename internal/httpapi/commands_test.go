package httpapi

import "testing"

func TestFirstNonEmptyTrimsWhitespace(t *testing.T) {
	got := firstNonEmpty("   ", "\t target \n", "fallback")
	if got != "target" {
		t.Fatalf("expected trimmed non-empty value, got %q", got)
	}
}
