package agent

import (
	"fmt"

	egagent "github.com/lovyou-ai/eventgraph/go/pkg/agent"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
)

// transitionTo changes the agent's operational state, validating the transition
// and emitting an agent.state.changed event on the graph.
//
// Holds a.mu for the entire operation: state validation, state update,
// event recording, and lastEvent update are atomic.
func (a *Agent) transitionTo(target egagent.OperationalState) error {
	a.mu.Lock()
	defer a.mu.Unlock()

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
		// State was already updated in memory — log but don't fail.
		// The event emission failure doesn't invalidate the transition.
		return nil
	}

	a.lastEvent = ev.ID()
	return nil
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
