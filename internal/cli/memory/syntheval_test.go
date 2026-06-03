package memory

import (
	"bytes"
	"context"
	"testing"
)

// A malformed flag must return a parse error before any DB work, so this runs
// without a live Postgres. (ContinueOnError makes Parse return rather than
// exit the process.)
func TestRunSynthEval_BadFlagReturnsError(t *testing.T) {
	var out, errOut bytes.Buffer
	err := RunSynthEval(context.Background(), []string{"-nonexistent-flag"}, &out, &errOut)
	if err == nil {
		t.Fatal("expected a parse error for an unknown flag, got nil")
	}
}
