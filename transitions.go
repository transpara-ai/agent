package agent

import (
	"fmt"

	egagent "github.com/transpara-ai/eventgraph/go/pkg/agent"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
)

// transitionTo changes the agent's operational state, validating the transition
// and emitting an agent.state.changed event on the graph.
//
// Acquires a.mu for the entire operation: state validation, state update,
// event recording, and lastEvent update are atomic.
//
// Returns an error if the transition is invalid or if the state change
// event cannot be emitted (OBSERVABLE invariant — state changes must be
// recorded on the graph).
func (a *Agent) transitionTo(target egagent.OperationalState) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.transitionLocked(target)
}

// transitionLocked performs a state transition while a.mu is already held.
// All callers must hold a.mu. Use transitionTo() when the lock is not held.
func (a *Agent) transitionLocked(target egagent.OperationalState) error {
	prev := a.state
	next, err := a.state.TransitionTo(target)
	if err != nil {
		return fmt.Errorf("invalid transition %s → %s: %w", prev, target, err)
	}
	a.state = next

	// Emit state change event.
	ev, err := a.record(event.EventTypeAgentStateChanged.Value(), event.AgentStateChangedContent{
		AgentID:  a.runtime.ID(),
		Previous: prev.String(),
		Current:  next.String(),
	})
	if err != nil {
		// Roll back the state — an unrecorded transition violates OBSERVABLE.
		a.state = prev
		return fmt.Errorf("transition %s → %s: record event: %w", prev, next, err)
	}

	a.lastEvent = ev.ID()
	return nil
}

// ResetToIdle forces the agent back to Idle state without validation.
// Used for recovery when the agent is stuck in Processing due to failed
// event recording during a transition back from Processing.
// Does NOT emit a state change event — this is a recovery path, not a
// normal transition.
func (a *Agent) ResetToIdle() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state = egagent.StateIdle
}

// Suspend puts the agent into suspended state (e.g., Guardian HALT).
// Can only transition from Idle or Processing.
func (a *Agent) Suspend() error {
	return a.transitionTo(egagent.StateSuspended)
}

// Resume brings the agent out of suspended state back to Idle.
func (a *Agent) Resume() error {
	return a.transitionTo(egagent.StateIdle)
}
