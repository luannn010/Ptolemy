package memory

import (
	"bytes"
	"context"
	"testing"
)

// Argument shape is validated before the module opens, so these run DB-free.
func TestRunDemo_ArgValidation(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"no subcommand", nil},
		{"unknown subcommand", []string{"frob"}},
		{"ingest missing args", []string{"ingest", "only-id"}},
		{"ingest too many args", []string{"ingest", "id", "file", "extra"}},
		{"ask missing question", []string{"ask"}},
		{"ask too many args", []string{"ask", "q1", "q2"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if err := RunDemo(context.Background(), tc.args, &out, &errOut); err == nil {
				t.Fatalf("expected a validation error for args %v, got nil", tc.args)
			}
		})
	}
}
