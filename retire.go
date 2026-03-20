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
// Resolves any mid-operation state (Escalating, Refusing) back to Idle
// before beginning the retirement sequence.
//
// Transitions: current state → [Idle →] Retiring → Retired.
func (a *Agent) Retire(ctx context.Context, reason string) error {
	// Resolve mid-operation states that can't transition directly to Retiring.
	// Escalating → Idle, Refusing → Idle are valid FSM transitions.
	state := a.State()
	switch state {
	case egagent.StateEscalating, egagent.StateRefusing:
		if err := a.transitionTo(egagent.StateIdle); err != nil {
			return fmt.Errorf("retire: resolve %s: %w", state, err)
		}
	case egagent.StateWaiting:
		// Waiting → Processing → Idle → Retiring would be cleanest,
		// but Waiting can also go to Idle directly.
		if err := a.transitionTo(egagent.StateIdle); err != nil {
			return fmt.Errorf("retire: resolve %s: %w", state, err)
		}
	case egagent.StateRetired:
		return fmt.Errorf("retire: agent already retired")
	}

	if err := a.transitionTo(egagent.StateRetiring); err != nil {
		return fmt.Errorf("retire: %w", err)
	}

	// Introspect: final self-observation.
	introEv, _, err := a.runtime.Introspect(ctx, "Final introspection before retirement: "+reason)
	if err == nil {
		a.mu.Lock()
		a.lastEvent = introEv.ID()
		a.mu.Unlock()
	}

	// Communicate: farewell on the "lifecycle" channel.
	_, _ = a.recordAndTrack(event.EventTypeAgentCommunicated.Value(), event.AgentCommunicatedContent{
		AgentID:   a.runtime.ID(),
		Recipient: a.runtime.ID(), // farewell to all — self-addressed
		Channel:   "lifecycle",
	})

	// Memory: archive — record the retirement in the agent's memory.
	_, _ = a.recordAndTrack(event.EventTypeAgentMemoryUpdated.Value(), event.AgentMemoryUpdatedContent{
		AgentID: a.runtime.ID(),
		Key:     "retirement",
		Action:  "archive",
	})

	// Lifespan: end.
	_, _ = a.recordAndTrack(event.EventTypeAgentLifespanEnded.Value(), event.AgentLifespanEndedContent{
		AgentID: a.runtime.ID(),
		Reason:  reason,
	})

	// Transition to Retired (terminal state).
	if err := a.transitionTo(egagent.StateRetired); err != nil {
		return fmt.Errorf("retire: final transition: %w", err)
	}

	// Update actor store: memorial.
	a.mu.Lock()
	lastID := a.lastEvent
	a.mu.Unlock()
	if !lastID.IsZero() {
		_, _ = a.graph.ActorStore().Memorial(a.runtime.ID(), lastID)
	}

	return nil
}
