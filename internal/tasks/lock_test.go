package tasks

import "testing"

func TestConflictsOverlap(t *testing.T) {
	a := Task{AllowedFiles: []string{"a.go"}}
	b := Task{AllowedFiles: []string{"a.go"}}
	if !Conflicts(a, b) {
		t.Fatal("expected conflict")
	}
}

func TestConflictsDifferentFiles(t *testing.T) {
	a := Task{AllowedFiles: []string{"a.go"}}
	b := Task{AllowedFiles: []string{"b.go"}}
	if Conflicts(a, b) {
		t.Fatal("did not expect conflict")
	}
}

func TestConflictsDirectoryContainsFile(t *testing.T) {
	a := Task{AllowedFiles: []string{"internal/"}}
	b := Task{AllowedFiles: []string{"internal/tasks/validator.go"}}
	if !Conflicts(a, b) {
		t.Fatal("expected directory/file conflict")
	}
}

func TestConflictsNestedDirectories(t *testing.T) {
	a := Task{AllowedFiles: []string{"internal/"}}
	b := Task{AllowedFiles: []string{"internal/tasks/"}}
	if !Conflicts(a, b) {
		t.Fatal("expected nested directory conflict")
	}
}

func TestPickNonConflictingBatch(t *testing.T) {
	tasks := []Task{
		{ID: "a", AllowedFiles: []string{"x.go"}},
		{ID: "b", AllowedFiles: []string{"x.go"}},
		{ID: "c", AllowedFiles: []string{"y.go"}},
	}
	got := PickNonConflictingBatch(tasks, 0)
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "c" {
		t.Fatalf("unexpected batch: %+v", got)
	}
}

func TestPickNonConflictingBatchHonorsMax(t *testing.T) {
	tasks := []Task{
		{ID: "a", AllowedFiles: []string{"x.go"}},
		{ID: "c", AllowedFiles: []string{"y.go"}},
	}
	got := PickNonConflictingBatch(tasks, 1)
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("unexpected batch: %+v", got)
	}
}
