package agent

import (
	"context"
	"fmt"

	egagent "github.com/lovyou-ai/eventgraph/go/pkg/agent"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
)

// Retire gracefully shuts down the agent.
// Follows the Retire composition: Introspect → Communicate (farewell) →
// Memory (archive) → Lifespan (end).
//
// All events are emitted via graph.Record() for bus visibility.
// Resolves any mid-operation state (Escalating, Refusing, Waiting) back
// to Idle before beginning the retirement sequence — atomically under
// a.mu to prevent TOCTOU races.
//
// The retirement ceremony (introspect through lifespan-end) and the final
// transition to StateRetired are held under a.mu as one atomic block.
// This prevents concurrent non-FSM methods from interleaving events
// into the retirement sequence.
//
// Introspection records the reason directly (no LLM call) to prevent
// indefinite hangs in the terminal StateRetiring.
//
// Transitions: current state → [Idle →] Retiring → Retired.
func (a *Agent) Retire(ctx context.Context, reason string) error {
	if err := a.resolveAndRetire(); err != nil {
		return fmt.Errorf("retire: %w", err)
	}

	if err := a.retireCeremony(reason); err != nil {
		return fmt.Errorf("retire: %w", err)
	}

	// Update actor store: memorial (outside a.mu — ActorStore has its own lock).
	a.mu.Lock()
	lastID := a.lastEvent
	a.mu.Unlock()
	if !lastID.IsZero() {
		_, _ = a.graph.ActorStore().Memorial(a.runtime.ID(), lastID)
	}

	return nil
}

// retireCeremony emits the retirement event sequence and transitions to
// StateRetired, all under a.mu. This prevents concurrent methods from
// interleaving events into the ceremony.
//
// Uses record() + manual lastEvent updates (not recordAndTrack) because
// a.mu is already held. Uses transitionLocked() for the final transition.
func (a *Agent) retireCeremony(reason string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Introspect: record the retirement reason directly. No LLM call —
	// an unresponsive provider would block the agent in StateRetiring
	// indefinitely, violating the DIGNITY invariant (graceful shutdown).
	if ev, err := a.record(event.EventTypeAgentIntrospected.Value(), event.AgentIntrospectedContent{
		AgentID:     a.runtime.ID(),
		Observation: "Retiring: " + reason,
	}); err == nil {
		a.lastEvent = ev.ID()
	}

	// Communicate: farewell on the "lifecycle" channel.
	if ev, err := a.record(event.EventTypeAgentCommunicated.Value(), event.AgentCommunicatedContent{
		AgentID:   a.runtime.ID(),
		Recipient: a.runtime.ID(),
		Channel:   "lifecycle",
	}); err == nil {
		a.lastEvent = ev.ID()
	}

	// Memory: archive — record the retirement in the agent's memory.
	if ev, err := a.record(event.EventTypeAgentMemoryUpdated.Value(), event.AgentMemoryUpdatedContent{
		AgentID: a.runtime.ID(),
		Key:     "retirement",
		Action:  "archive",
	}); err == nil {
		a.lastEvent = ev.ID()
	}

	// Lifespan: end.
	if ev, err := a.record(event.EventTypeAgentLifespanEnded.Value(), event.AgentLifespanEndedContent{
		AgentID: a.runtime.ID(),
		Reason:  reason,
	}); err == nil {
		a.lastEvent = ev.ID()
	}

	// Transition to Retired (terminal state).
	return a.transitionLocked(egagent.StateRetired)
}

// resolveAndRetire atomically resolves mid-operation states and transitions
// to StateRetiring. Holds a.mu for the entire sequence to prevent TOCTOU
// races between state observation and transition.
func (a *Agent) resolveAndRetire() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.state == egagent.StateRetired {
		return fmt.Errorf("agent already retired")
	}

	// Resolve mid-operation states that can't transition directly to Retiring.
	switch a.state {
	case egagent.StateEscalating, egagent.StateRefusing, egagent.StateWaiting:
		if err := a.transitionLocked(egagent.StateIdle); err != nil {
			return fmt.Errorf("resolve %s: %w", a.state, err)
		}
	}

	return a.transitionLocked(egagent.StateRetiring)
}
