package agent

import (
	"context"
	"fmt"

	egagent "github.com/lovyou-ai/eventgraph/go/pkg/agent"
	"github.com/lovyou-ai/eventgraph/go/pkg/decision"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
)

// Reason sends a prompt to the agent's LLM and returns the response.
// Drives the state machine: Idle → Processing → Idle.
// Emits agent.evaluated with the result.
func (a *Agent) Reason(ctx context.Context, prompt string) (string, error) {
	if err := a.transitionTo(egagent.StateProcessing); err != nil {
		return "", fmt.Errorf("reason: %w", err)
	}

	resp, err := a.runtime.Provider().Reason(ctx, prompt, nil)
	if err != nil {
		_ = a.transitionTo(egagent.StateIdle)
		return "", fmt.Errorf("reason: %w", err)
	}

	content := resp.Content()

	// Emit evaluated event (atomic record + causality update).
	_, _ = a.recordAndTrack(event.EventTypeAgentEvaluated.Value(), event.AgentEvaluatedContent{
		AgentID:    a.runtime.ID(),
		Subject:    "reason",
		Confidence: types.MustScore(1.0),
		Result:     truncate(content, 500),
	})

	if err := a.transitionTo(egagent.StateIdle); err != nil {
		return content, fmt.Errorf("reason: transition back: %w", err)
	}

	return content, nil
}

// Operate runs the agent's LLM with filesystem/tool access (agentic mode).
// Drives the state machine: Idle → Processing → Idle.
// Emits agent.acted with the result summary.
func (a *Agent) Operate(ctx context.Context, workDir, instruction string) (decision.OperateResult, error) {
	op, ok := a.runtime.Provider().(decision.IOperator)
	if !ok {
		return decision.OperateResult{}, fmt.Errorf("operate: provider does not support Operate")
	}

	if err := a.transitionTo(egagent.StateProcessing); err != nil {
		return decision.OperateResult{}, fmt.Errorf("operate: %w", err)
	}

	result, err := op.Operate(ctx, decision.OperateTask{
		WorkDir:     workDir,
		Instruction: instruction,
	})
	if err != nil {
		_ = a.transitionTo(egagent.StateIdle)
		return decision.OperateResult{}, fmt.Errorf("operate: %w", err)
	}

	// Emit acted event (atomic record + causality update).
	_, _ = a.recordAndTrack(event.EventTypeAgentActed.Value(), event.AgentActedContent{
		AgentID: a.runtime.ID(),
		Action:  "operate",
		Target:  workDir,
	})

	if err := a.transitionTo(egagent.StateIdle); err != nil {
		return result, fmt.Errorf("operate: transition back: %w", err)
	}

	return result, nil
}

// Observe queries the graph for recent events relevant to this agent.
// Returns a slice of recent events for context building.
func (a *Agent) Observe(ctx context.Context, limit int) ([]event.Event, error) {
	page, err := a.graph.Store().Recent(limit, types.None[types.Cursor]())
	if err != nil {
		return nil, fmt.Errorf("observe: %w", err)
	}
	return page.Items(), nil
}

// Memory returns this agent's own recent events for self-context.
func (a *Agent) Memory(limit int) ([]event.Event, error) {
	return a.runtime.Memory(limit)
}

// Evaluate produces a judgment about a subject.
// Drives the state machine: Idle → Processing → Idle.
func (a *Agent) Evaluate(ctx context.Context, subject, prompt string) (string, error) {
	if err := a.transitionTo(egagent.StateProcessing); err != nil {
		return "", fmt.Errorf("evaluate: %w", err)
	}

	ev, result, err := a.runtime.Evaluate(ctx, subject, prompt)
	if err != nil {
		_ = a.transitionTo(egagent.StateIdle)
		return "", fmt.Errorf("evaluate: %w", err)
	}

	a.mu.Lock()
	a.lastEvent = ev.ID()
	a.mu.Unlock()

	if err := a.transitionTo(egagent.StateIdle); err != nil {
		return result, fmt.Errorf("evaluate: transition back: %w", err)
	}

	return result, nil
}

// Communicate sends a message to another agent through the graph.
// The channel identifies the communication medium (e.g. "general", "alerts").
// Emits agent.communicated, observable by the target and all graph subscribers.
func (a *Agent) Communicate(ctx context.Context, targetID types.ActorID, channel string) error {
	_, err := a.recordAndTrack(event.EventTypeAgentCommunicated.Value(), event.AgentCommunicatedContent{
		AgentID:   a.runtime.ID(),
		Recipient: targetID,
		Channel:   channel,
	})
	if err != nil {
		return fmt.Errorf("communicate: %w", err)
	}
	return nil
}

// Learn records a lesson from experience.
func (a *Agent) Learn(ctx context.Context, lesson, source string) error {
	ev, err := a.runtime.Learn(ctx, lesson, source)
	if err != nil {
		return fmt.Errorf("learn: %w", err)
	}

	a.mu.Lock()
	a.lastEvent = ev.ID()
	a.mu.Unlock()

	return nil
}

// Escalate passes a problem upward in the hierarchy.
// Drives the state machine: Idle → Processing → Escalating → Idle.
// Always escalates to the human operator — role hierarchy is the
// application layer's concern.
func (a *Agent) Escalate(ctx context.Context, reason string) error {
	// Must be Processing before Escalating (FSM constraint).
	if err := a.transitionTo(egagent.StateProcessing); err != nil {
		return fmt.Errorf("escalate: %w", err)
	}
	if err := a.transitionTo(egagent.StateEscalating); err != nil {
		_ = a.transitionTo(egagent.StateIdle)
		return fmt.Errorf("escalate: %w", err)
	}

	targetID := types.MustActorID("human")

	ev, err := a.runtime.Escalate(ctx, targetID, reason)
	if err != nil {
		_ = a.transitionTo(egagent.StateIdle)
		return fmt.Errorf("escalate: %w", err)
	}

	a.mu.Lock()
	a.lastEvent = ev.ID()
	a.mu.Unlock()

	return a.transitionTo(egagent.StateIdle)
}

// Refuse declines to perform an action (soul-protected refusal).
// Drives the state machine: Idle → Processing → Refusing → Idle.
func (a *Agent) Refuse(ctx context.Context, action, reason string) error {
	// Must be Processing before Refusing (FSM constraint).
	if err := a.transitionTo(egagent.StateProcessing); err != nil {
		return fmt.Errorf("refuse: %w", err)
	}
	if err := a.transitionTo(egagent.StateRefusing); err != nil {
		_ = a.transitionTo(egagent.StateIdle)
		return fmt.Errorf("refuse: %w", err)
	}

	ev, err := a.runtime.Refuse(ctx, action, reason)
	if err != nil {
		_ = a.transitionTo(egagent.StateIdle)
		return fmt.Errorf("refuse: %w", err)
	}

	a.mu.Lock()
	a.lastEvent = ev.ID()
	a.mu.Unlock()

	return a.transitionTo(egagent.StateIdle)
}

// truncate shortens a string to maxLen, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
