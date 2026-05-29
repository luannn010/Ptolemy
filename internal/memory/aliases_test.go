package memory

import "testing"

func TestExpandAliases_AddsKnownSynonyms(t *testing.T) {
	out := expandAliases("how does the GC work")
	if out == "how does the GC work" {
		t.Fatalf("expected expansion, got unchanged %q", out)
	}
	if !containsFold(out, "garbage collector") || !containsFold(out, "GC") {
		t.Fatalf("expansion must keep original + add alias, got %q", out)
	}
}

func TestExpandAliases_NoMatchUnchanged(t *testing.T) {
	in := "completely unrelated query text"
	if out := expandAliases(in); out != in {
		t.Fatalf("no-alias query must be unchanged, got %q", out)
	}
}
