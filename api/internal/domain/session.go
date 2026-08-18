package domain

import (
	"fmt"
	"time"
)

type SessionState string

const (
	StateCreating     SessionState = "creating"
	StateProvisioning SessionState = "provisioning"
	StateReady        SessionState = "ready"
	StateRunning      SessionState = "running"
	StateResetting    SessionState = "resetting"
	StateCompleted    SessionState = "completed"
	StateExpired      SessionState = "expired"
	StateDestroyed    SessionState = "destroyed"
)

type Scenario struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Difficulty   string   `json:"difficulty"`
	DurationMin  int      `json:"durationMin"`
	Technologies []string `json:"technologies"`
}

type Session struct {
	ID         string       `json:"id"`
	UserID     string       `json:"userId"`
	ScenarioID string       `json:"scenarioId"`
	State      SessionState `json:"state"`
	RuntimeID  string       `json:"runtimeId,omitempty"`
	CreatedAt  time.Time    `json:"createdAt"`
	ExpiresAt  time.Time    `json:"expiresAt"`
}

var transitions = map[SessionState]map[SessionState]bool{
	StateCreating:     {StateProvisioning: true, StateDestroyed: true},
	StateProvisioning: {StateReady: true, StateDestroyed: true},
	StateReady:        {StateRunning: true, StateDestroyed: true},
	StateRunning:      {StateCompleted: true, StateResetting: true, StateExpired: true, StateDestroyed: true},
	StateResetting:    {StateProvisioning: true, StateDestroyed: true},
	StateCompleted:    {StateDestroyed: true},
	StateExpired:      {StateDestroyed: true},
}

func (s *Session) Transition(next SessionState) error {
	if !transitions[s.State][next] {
		return fmt.Errorf("invalid session transition: %s -> %s", s.State, next)
	}
	s.State = next
	return nil
}
