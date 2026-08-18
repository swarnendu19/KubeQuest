package domain

import "testing"

func TestSessionStateMachine(t *testing.T) {
	s := Session{State: StateCreating}
	for _, next := range []SessionState{StateProvisioning, StateReady, StateRunning, StateCompleted, StateDestroyed} {
		if err := s.Transition(next); err != nil {
			t.Fatalf("transition to %s: %v", next, err)
		}
	}
}

func TestSessionRejectsInvalidTransition(t *testing.T) {
	s := Session{State: StateCreating}
	if err := s.Transition(StateRunning); err == nil {
		t.Fatal("expected invalid transition to fail")
	}
}
