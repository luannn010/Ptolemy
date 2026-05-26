package policy

import "sync"

type Approvals struct {
	mu      sync.Mutex
	parked  map[string]bool
	allowed map[string]bool
}

func NewApprovals() *Approvals {
	return &Approvals{
		parked:  map[string]bool{},
		allowed: map[string]bool{},
	}
}

func (a *Approvals) Park(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.parked[id] = true
}

func (a *Approvals) Approve(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.parked[id] {
		return false
	}
	a.allowed[id] = true
	return true
}

func (a *Approvals) ConsumeApproved(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.allowed[id] {
		return false
	}
	delete(a.allowed, id)
	delete(a.parked, id)
	return true
}
