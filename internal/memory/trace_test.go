package memory

import "testing"

func TestSnippet_ShortStringUnchanged(t *testing.T) {
	if got := snippet("hello world", 120); got != "hello world" {
		t.Fatalf("short string should be unchanged, got %q", got)
	}
}

func TestSnippet_TruncatesByRunes(t *testing.T) {
	got := snippet("abcdefghij", 4)
	if got != "abcd…" {
		t.Fatalf("expected rune-truncated with ellipsis, got %q", got)
	}
}

func TestSnippet_SingleLinesNewlines(t *testing.T) {
	if got := snippet("line one\nline two\r\nthree", 120); got != "line one line two three" {
		t.Fatalf("newlines/CRs should collapse to single spaces, got %q", got)
	}
}

func TestSnippet_CountsRunesNotBytes(t *testing.T) {
	// 5 multibyte runes, limit 5 -> unchanged (no truncation).
	if got := snippet("héllo", 5); got != "héllo" {
		t.Fatalf("rune count (not byte count) must govern, got %q", got)
	}
}
