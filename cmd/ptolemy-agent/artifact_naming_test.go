package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestArtifactPathUsesUTCDateTaskNameStepAndLabel(t *testing.T) {
	now := time.Date(2024, 4, 29, 1, 2, 3, 0, time.FixedZone("AEST", 10*60*60))

	got := artifactPath("My Task", 7, "command output", now)
	want := filepath.Join(artifactDir, "280424-my-task-step007-command-output.txt")

	if got != want {
		t.Fatalf("artifactPath() = %q, want %q", got, want)
	}
}

func TestSaveArtifactAvoidsOverwriteOnCollision(t *testing.T) {
	chdirTemp(t)

	now := time.Date(2024, 4, 29, 1, 2, 3, 0, time.UTC)
	basePath := artifactPath("My Task", 1, "command output", now)
	if err := os.MkdirAll(filepath.Dir(basePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(basePath, []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}

	got := saveArtifactAt(now, "My Task", 1, "command output", "new content")
	if got == basePath {
		t.Fatalf("saveArtifactAt() overwrote existing artifact: %q", got)
	}
	if !strings.HasPrefix(filepath.Base(got), "290424-my-task-step001-command-output-2.txt") {
		t.Fatalf("saveArtifactAt() = %q, want collision suffix", got)
	}

	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new content" {
		t.Fatalf("saved content = %q, want %q", string(data), "new content")
	}
}

func TestDeriveTaskName(t *testing.T) {
	got := deriveTaskName("docs/tasks/inbox/My Task.md", "# ignored")
	if got != "My Task" {
		t.Fatalf("deriveTaskName() = %q, want %q", got, "My Task")
	}

	got = deriveTaskName("", "# Artifact Naming Update\nUse a new file format.")
	if got != "Artifact Naming Update" {
		t.Fatalf("deriveTaskName() = %q, want %q", got, "Artifact Naming Update")
	}

	got = deriveTaskName("", " ")
	if got != "task" {
		t.Fatalf("deriveTaskName() = %q, want %q", got, "task")
	}
}

func chdirTemp(t *testing.T) {
	t.Helper()

	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := os.Chdir(previousWD); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})
}
