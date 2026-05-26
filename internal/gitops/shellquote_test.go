package gitops

import (
	"strings"
	"testing"
)

func TestShellQuote_WrapsAndEscapesSingleQuotes(t *testing.T) {
	got := shellQuote("foo's bar")
	if !strings.HasPrefix(got, "'") || !strings.HasSuffix(got, "'") {
		t.Fatalf("shellQuote must wrap in single quotes: %q", got)
	}
	if !strings.Contains(got, `'\''`) {
		t.Fatalf("shellQuote must escape embedded single quote: %q", got)
	}
}

func TestShellQuotePath_QuotesPlainPath(t *testing.T) {
	got := shellQuotePath("/some/path with space")
	if !strings.Contains(got, "/some/path with space") {
		t.Fatalf("expected original path inside quotes: %q", got)
	}
}

func TestTruncateOutput(t *testing.T) {
	out, truncated := truncateOutput("short", 100)
	if truncated || out != "short" {
		t.Fatalf("short input must pass through unchanged: %q truncated=%v", out, truncated)
	}
	big := strings.Repeat("x", 200)
	out, truncated = truncateOutput(big, 50)
	if !truncated {
		t.Fatalf("expected truncated=true")
	}
	if !strings.HasSuffix(out, "[truncated]") {
		t.Fatalf("truncated marker missing: %q", out)
	}
}
