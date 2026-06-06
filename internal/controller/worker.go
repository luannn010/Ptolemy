package controller

// State is a worker lifecycle state.
type State string

const (
	StatePending      State = "pending"
	StateProvisioning State = "provisioning"
	StateRunning      State = "running"
	StateStage1Passed State = "stage1_passed"
	StateIntegrating  State = "integrating" // legal but unused this slice
	StateMerged       State = "merged"      // legal but unused this slice
	StateFailed       State = "failed"
	StateCancelled    State = "cancelled"
)

// WorkSpec describes a unit of work assigned to a worker.
type WorkSpec struct {
	Name   string // worker / worktree name
	Branch string // optional; worktree.Manager defaults when empty
}

// Worker is the controller's record for one sandboxed unit of work.
type Worker struct {
	ID       string
	Spec     WorkSpec
	Worktree string // filesystem path, set after provisioning
	Branch   string
	State    State
	Detail   string // last outcome or error detail

	sessionID string // unexported: policy session that spawned this worker
}

// transitions maps each state to the set of states it may legally move to.
// Merged, Failed, Cancelled are terminal (absent here).
var transitions = map[State][]State{
	StatePending:      {StateProvisioning, StateFailed, StateCancelled},
	StateProvisioning: {StateRunning, StateFailed, StateCancelled},
	StateRunning:      {StateStage1Passed, StateFailed, StateCancelled},
	StateStage1Passed: {StateIntegrating, StateFailed, StateCancelled},
	StateIntegrating:  {StateMerged, StateFailed, StateCancelled},
}

// CanTransition reports whether from->to is a legal lifecycle transition.
func CanTransition(from, to State) bool {
	for _, allowed := range transitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}
