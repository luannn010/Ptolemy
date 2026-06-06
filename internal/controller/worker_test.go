package controller

import "testing"

func TestCanTransitionLegal(t *testing.T) {
	legal := []struct{ from, to State }{
		{StatePending, StateProvisioning},
		{StateProvisioning, StateRunning},
		{StateRunning, StateStage1Passed},
		{StateStage1Passed, StateIntegrating},
		{StateIntegrating, StateMerged},
		{StatePending, StateFailed},
		{StateProvisioning, StateFailed},
		{StateRunning, StateFailed},
		{StatePending, StateCancelled},
		{StateRunning, StateCancelled},
		{StateStage1Passed, StateCancelled},
	}
	for _, c := range legal {
		if !CanTransition(c.from, c.to) {
			t.Errorf("expected %s->%s legal", c.from, c.to)
		}
	}
}

func TestCanTransitionIllegal(t *testing.T) {
	illegal := []struct{ from, to State }{
		{StatePending, StateRunning},      // must provision first
		{StateProvisioning, StateMerged},  // skips stages
		{StateMerged, StateRunning},       // terminal
		{StateFailed, StateRunning},       // terminal
		{StateCancelled, StatePending},    // terminal
		{StateRunning, StatePending},      // no going back
		{StateStage1Passed, StateRunning}, // no going back
	}
	for _, c := range illegal {
		if CanTransition(c.from, c.to) {
			t.Errorf("expected %s->%s illegal", c.from, c.to)
		}
	}
}

func TestTerminalStatesHaveNoOutgoing(t *testing.T) {
	all := []State{
		StatePending, StateProvisioning, StateRunning, StateStage1Passed,
		StateIntegrating, StateMerged, StateFailed, StateCancelled,
	}
	for _, term := range []State{StateMerged, StateFailed, StateCancelled} {
		for _, to := range all {
			if CanTransition(term, to) {
				t.Errorf("terminal %s should not transition to %s", term, to)
			}
		}
	}
}
