package tasks

import "testing"

func TestFindAllowedFileConflictsNoOverlap(t *testing.T) {
	conflicts := FindAllowedFileConflicts([]Task{
		{ID: "a", AllowedFiles: []string{"a.go"}},
		{ID: "b", AllowedFiles: []string{"b.go"}},
	})
	if len(conflicts) != 0 {
		t.Fatalf("expected no conflicts, got %+v", conflicts)
	}
}

func TestFindAllowedFileConflictsSharedPath(t *testing.T) {
	conflicts := FindAllowedFileConflicts([]Task{
		{ID: "a", AllowedFiles: []string{"x.go"}},
		{ID: "b", AllowedFiles: []string{"x.go"}},
	})
	if len(conflicts) != 1 || conflicts[0].File != "x.go" {
		t.Fatalf("unexpected conflicts: %+v", conflicts)
	}
}

func TestFindAllowedFileConflictsGroupsMultipleTasks(t *testing.T) {
	conflicts := FindAllowedFileConflicts([]Task{
		{ID: "c", AllowedFiles: []string{"x.go"}},
		{ID: "a", AllowedFiles: []string{"x.go"}},
		{ID: "b", AllowedFiles: []string{"x.go"}},
	})
	if len(conflicts) != 1 {
		t.Fatalf("unexpected conflicts: %+v", conflicts)
	}
	want := []string{"a", "b", "c"}
	for i, id := range want {
		if conflicts[0].TaskIDs[i] != id {
			t.Fatalf("unexpected task ids: %+v", conflicts[0].TaskIDs)
		}
	}
}

func TestFindAllowedFileConflictsCleansPaths(t *testing.T) {
	conflicts := FindAllowedFileConflicts([]Task{
		{ID: "a", AllowedFiles: []string{"./internal/x.go"}},
		{ID: "b", AllowedFiles: []string{"internal/x.go"}},
	})
	if len(conflicts) != 1 || conflicts[0].File != "internal/x.go" {
		t.Fatalf("unexpected conflicts: %+v", conflicts)
	}
}

func TestFindAllowedFileConflictsDirectoryOverlapsFile(t *testing.T) {
	conflicts := FindAllowedFileConflicts([]Task{
		{ID: "a", AllowedFiles: []string{"internal/"}},
		{ID: "b", AllowedFiles: []string{"internal/x.go"}},
	})
	if len(conflicts) != 2 {
		t.Fatalf("unexpected conflicts: %+v", conflicts)
	}
}

func TestCanRunTogetherFalseWhenConflictExists(t *testing.T) {
	if CanRunTogether([]Task{
		{ID: "a", AllowedFiles: []string{"x.go"}},
		{ID: "b", AllowedFiles: []string{"x.go"}},
	}) {
		t.Fatal("expected tasks not to run together")
	}
}
