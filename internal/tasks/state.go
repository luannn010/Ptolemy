package tasks

import "sync"

const (
	StatusInbox     = "inbox"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusBlocked   = "blocked"
)

type StateStore interface {
	Get(taskID string) (string, bool)
	Set(taskID string, status string)
	Completed(taskID string) bool
	Snapshot() map[string]string
}

type MemoryStateStore struct {
	mu   sync.RWMutex
	data map[string]string
}

func NewMemoryStateStore() *MemoryStateStore {
	return &MemoryStateStore{data: map[string]string{}}
}

func (s *MemoryStateStore) Get(taskID string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	status, ok := s.data[taskID]
	return status, ok
}

func (s *MemoryStateStore) Set(taskID string, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[taskID] = status
}

func (s *MemoryStateStore) Completed(taskID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data[taskID] == StatusCompleted
}

func (s *MemoryStateStore) Snapshot() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.data))
	for k, v := range s.data {
		out[k] = v
	}
	return out
}
