package tasks

import "testing"

func TestMemoryStateStore_SetGet(t *testing.T) {
	s := NewMemoryStateStore()
	s.Set("t1", StatusRunning)
	got, ok := s.Get("t1")
	if !ok {
		t.Fatal("expected key to exist")
	}
	if got != StatusRunning {
		t.Fatalf("unexpected status: %s", got)
	}
}

func TestMemoryStateStore_Completed(t *testing.T) {
	s := NewMemoryStateStore()
	s.Set("a", StatusCompleted)
	s.Set("b", StatusFailed)
	if !s.Completed("a") {
		t.Fatal("expected completed true for completed status")
	}
	if s.Completed("b") {
		t.Fatal("expected completed false for failed status")
	}
	if s.Completed("missing") {
		t.Fatal("expected completed false for missing status")
	}
}

func TestMemoryStateStore_SnapshotCopy(t *testing.T) {
	s := NewMemoryStateStore()
	s.Set("t1", StatusInbox)
	snap := s.Snapshot()
	snap["t1"] = StatusCompleted
	got, _ := s.Get("t1")
	if got != StatusInbox {
		t.Fatalf("snapshot mutation should not change store, got %s", got)
	}
}
