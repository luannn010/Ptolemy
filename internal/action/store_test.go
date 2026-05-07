package action

import (
	"context"
	"strings"
	"testing"

	storepkg "github.com/luannn010/ptolemy/internal/store"
)

func TestMergeMetadataPreservesExistingKeys(t *testing.T) {
	raw := MergeMetadata(`{"batch_index":1}`, ActionMetadata{
		Title:         "Inspect parser",
		Purpose:       "Confirm required task pack fields.",
		ReasoningStep: "Check parser expectations",
		Target:        "internal/tasks/parser.go",
	})

	if !strings.Contains(raw, `"batch_index":1`) {
		t.Fatalf("expected merged metadata to preserve existing keys, got %s", raw)
	}
	if !strings.Contains(raw, `"title":"Inspect parser"`) {
		t.Fatalf("expected merged metadata title, got %s", raw)
	}
}

func TestFormatDisplayUsesTitleAndFallback(t *testing.T) {
	detailed := FormatDisplay("Used Ptolemy", "read_file", ActionMetadata{
		Title:         "Inspect task parser schema",
		Purpose:       "Confirm required task pack fields before generating the pack.",
		ReasoningStep: "Check parser expectations",
		Target:        "internal/tasks/parser.go",
	})

	if !strings.Contains(detailed, "Used Ptolemy — Inspect task parser schema") {
		t.Fatalf("expected descriptive title, got %q", detailed)
	}
	if !strings.Contains(detailed, "Action: file.read") {
		t.Fatalf("expected normalized action type, got %q", detailed)
	}

	fallback := FormatDisplay("Used Ptolemy", "", ActionMetadata{})
	if fallback != "Used Ptolemy" {
		t.Fatalf("expected fallback display, got %q", fallback)
	}
}

func TestStoreListBySessionHydratesMetadataFields(t *testing.T) {
	baseStore, err := storepkg.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = baseStore.Close()
	})

	if err := storepkg.RunMigrations(context.Background(), baseStore.SQLDB()); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	actionStore := NewStore(baseStore.SQLDB())
	_, err = actionStore.Create(context.Background(), Action{
		SessionID:     "session-1",
		Type:          "command.exec",
		Input:         "go test ./...",
		Status:        "pending",
		Metadata:      `{"batch_index":1}`,
		Title:         "Run Go test suite",
		Purpose:       "Confirm the metadata changes did not break executor behavior.",
		ReasoningStep: "Run tests",
		Target:        "go test ./...",
	})
	if err != nil {
		t.Fatalf("create action: %v", err)
	}

	actions, err := actionStore.ListBySession(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("list actions: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("expected one action, got %d", len(actions))
	}

	got := actions[0]
	if got.Title != "Run Go test suite" {
		t.Fatalf("expected title to round-trip, got %q", got.Title)
	}
	if !strings.Contains(got.Metadata, `"batch_index":1`) {
		t.Fatalf("expected metadata to preserve existing keys, got %s", got.Metadata)
	}
}
