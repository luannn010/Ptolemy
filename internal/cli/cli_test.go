package cli

import (
	"bytes"
	"context"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var stdout bytes.Buffer

	err := Run(context.Background(), Config{
		Args:    []string{"--version"},
		Version: "test-version",
		Stdout:  &stdout,
		Workdir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if stdout.String() != "test-version\n" {
		t.Fatalf("stdout = %q, want test-version", stdout.String())
	}
}

func TestRunWorkspace(t *testing.T) {
	var stdout bytes.Buffer
	dir := t.TempDir()

	err := Run(context.Background(), Config{
		Args:    []string{"--workspace"},
		Stdout:  &stdout,
		Workdir: dir,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if stdout.String() == "" {
		t.Fatal("expected workspace output")
	}
}
