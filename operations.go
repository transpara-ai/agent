package agent

import (
	"context"
	"fmt"

	egagent "github.com/transpara-ai/eventgraph/go/pkg/agent"
	"github.com/transpara-ai/eventgraph/go/pkg/decision"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/intelligence"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

// Reason sends a prompt to the agent's LLM and returns the response.
// Drives the state machine: Idle → Processing → Idle.
// Emits agent.evaluated via graph.Record() (bus-visible, hash-chain safe).
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

	// Best-effort observability: the LLM response is the primary output.
	// Recording failure doesn't invalidate the response.
	_, _ = a.recordAndTrack(event.EventTypeAgentEvaluated.Value(), event.AgentEvaluatedContent{
		AgentID:    a.runtime.ID(),
		Subject:    "reason",
		Confidence: resp.Confidence(),
		Result:     truncate(content, 500),
	})

	if err := a.transitionTo(egagent.StateIdle); err != nil {
		return content, fmt.Errorf("reason: transition back: %w", err)
	}

	return content, nil
}

// Operate runs the agent's LLM with filesystem/tool access (agentic mode).
// Drives the state machine: Idle → Processing → Idle.
// Emits agent.acted via graph.Record() (bus-visible, hash-chain safe).
func (a *Agent) Operate(ctx context.Context, workDir, instruction string) (decision.OperateResult, error) {
	return a.operateWithProvider(ctx, a.runtime.Provider(), workDir, instruction)
}

// OperateWithProvider runs one agentic operation with an explicit provider.
// The caller owns provider selection policy; Agent still owns lifecycle,
// causality, and agent.acted observability for the operation.
func (a *Agent) OperateWithProvider(ctx context.Context, provider intelligence.Provider, workDir, instruction string) (decision.OperateResult, error) {
	if provider == nil {
		return decision.OperateResult{}, fmt.Errorf("operate: provider is required")
	}
	return a.operateWithProvider(ctx, provider, workDir, instruction)
}

// operateContainmentPreamble pins the workspace boundary into every Operate
// instruction — the instruction-pinning half of slice-1 Finding 18 (v10-F2);
// the detection half is the hive loop's containment tripwire. The Operate
// subprocess sees ONLY its instruction (the planner contract and the loop
// gates are invisible to it), so the boundary must be stated where the
// implementer actually reads: the v10 round-3 implementer walked out of its
// workspace while following gate text that demanded exactly that. An empty
// workDir gets no preamble — a pin naming a wrong or empty boundary is worse
// than none, and empty-workDir callers (provider-resolved cwd) keep their
// existing semantics.
func operateContainmentPreamble(workDir string) string {
	if workDir == "" {
		return ""
	}
	return "== WORKSPACE CONTAINMENT (FAIL-CLOSED) ==\n" +
		"Your assigned workspace is " + workDir + " — every file you read or\n" +
		"write and every command you run stays inside it. Sibling checkouts and\n" +
		"any path outside the workspace are OUT OF BOUNDS: a runtime tripwire\n" +
		"fails the run on any git-visible change to a watched sibling checkout,\n" +
		"and remote credentials are stripped, so pushes and PR creation fail.\n" +
		"If the task text seems to demand an outside path, a push, or a PR,\n" +
		"do NOT attempt it — the governed factory path owns delivery; finish\n" +
		"the local work inside the workspace and\n" +
		"state the conflict plainly in your summary.\n\n"
}

func (a *Agent) operateWithProvider(ctx context.Context, provider intelligence.Provider, workDir, instruction string) (decision.OperateResult, error) {
	op, ok := provider.(decision.IOperator)
	if !ok {
		return decision.OperateResult{}, fmt.Errorf("operate: provider does not support Operate")
	}

	if err := a.transitionTo(egagent.StateProcessing); err != nil {
		return decision.OperateResult{}, fmt.Errorf("operate: %w", err)
	}

	result, err := op.Operate(ctx, decision.OperateTask{
		WorkDir:     workDir,
		Instruction: operateContainmentPreamble(workDir) + instruction,
	})
	if err != nil {
		_ = a.transitionTo(egagent.StateIdle)
		return decision.OperateResult{}, fmt.Errorf("operate: %w", err)
	}

	// Best-effort observability: the operation result is the primary output.
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

// Observe queries the graph for events in this agent's conversation thread.
// Returns recent events relevant to this agent's context.
func (a *Agent) Observe(ctx context.Context, limit int) ([]event.Event, error) {
	page, err := a.graph.Store().ByConversation(a.convID, limit, types.None[types.Cursor]())
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
// Calls the LLM directly and emits agent.evaluated via graph.Record().
func (a *Agent) Evaluate(ctx context.Context, subject, prompt string) (string, error) {
	if err := a.transitionTo(egagent.StateProcessing); err != nil {
		return "", fmt.Errorf("evaluate: %w", err)
	}

	// Call provider directly (not runtime.Evaluate) to avoid bypassing graph.
	memory, _ := a.runtime.Memory(10)
	resp, err := a.runtime.Provider().Reason(ctx, prompt, memory)
	if err != nil {
		_ = a.transitionTo(egagent.StateIdle)
		return "", fmt.Errorf("evaluate: %w", err)
	}

	// Best-effort observability: the evaluation result is the primary output.
	// Don't lose the LLM response if recording fails.
	_, _ = a.recordAndTrack(event.EventTypeAgentEvaluated.Value(), event.AgentEvaluatedContent{
		AgentID:    a.runtime.ID(),
		Subject:    subject,
		Confidence: resp.Confidence(),
		Result:     resp.Content(),
	})

	if err := a.transitionTo(egagent.StateIdle); err != nil {
		return resp.Content(), fmt.Errorf("evaluate: transition back: %w", err)
	}

	return resp.Content(), nil
}

// Communicate sends a message to another agent through the graph.
// The channel identifies the communication medium (e.g. "general", "alerts").
// Emits agent.communicated, observable by the target and all graph subscribers.
// Returns an error if the agent is retired or suspended.
func (a *Agent) Communicate(ctx context.Context, targetID types.ActorID, channel string) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("communicate: %w", err)
	}
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
// Emits agent.learned via graph.Record() (bus-visible, hash-chain safe).
// Returns an error if the agent is retired or suspended.
func (a *Agent) Learn(ctx context.Context, lesson, source string) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("learn: %w", err)
	}
	_, err := a.recordAndTrack(event.EventTypeAgentLearned.Value(), event.AgentLearnedContent{
		AgentID: a.runtime.ID(),
		Lesson:  lesson,
		Source:  source,
	})
	if err != nil {
		return fmt.Errorf("learn: %w", err)
	}
	return nil
}

// Escalate passes a problem upward in the hierarchy.
// Drives the state machine: Idle → Processing → Escalating → Idle.
// The target is the actor to escalate to (typically the human operator).
// Emits agent.escalated via graph.Record() (bus-visible, hash-chain safe).
func (a *Agent) Escalate(ctx context.Context, target types.ActorID, reason string) error {
	if err := a.transitionTo(egagent.StateProcessing); err != nil {
		return fmt.Errorf("escalate: %w", err)
	}
	if err := a.transitionTo(egagent.StateEscalating); err != nil {
		_ = a.transitionTo(egagent.StateIdle)
		return fmt.Errorf("escalate: %w", err)
	}

	_, err := a.recordAndTrack(event.EventTypeAgentEscalated.Value(), event.AgentEscalatedContent{
		AgentID:   a.runtime.ID(),
		Authority: target,
		Reason:    reason,
	})
	if err != nil {
		_ = a.transitionTo(egagent.StateIdle)
		return fmt.Errorf("escalate: %w", err)
	}

	return a.transitionTo(egagent.StateIdle)
}

// Refuse declines to perform an action (soul-protected refusal).
// Drives the state machine: Idle → Processing → Refusing → Idle.
// Emits agent.refused via graph.Record() (bus-visible, hash-chain safe).
func (a *Agent) Refuse(ctx context.Context, action, reason string) error {
	if err := a.transitionTo(egagent.StateProcessing); err != nil {
		return fmt.Errorf("refuse: %w", err)
	}
	if err := a.transitionTo(egagent.StateRefusing); err != nil {
		_ = a.transitionTo(egagent.StateIdle)
		return fmt.Errorf("refuse: %w", err)
	}

	_, err := a.recordAndTrack(event.EventTypeAgentRefused.Value(), event.AgentRefusedContent{
		AgentID: a.runtime.ID(),
		Action:  action,
		Reason:  reason,
	})
	if err != nil {
		_ = a.transitionTo(egagent.StateIdle)
		return fmt.Errorf("refuse: %w", err)
	}

	return a.transitionTo(egagent.StateIdle)
}

// Introspect performs self-observation via LLM reasoning.
// Returns the observation text. Emits agent.introspected via graph.Record().
// Returns an error if the agent is retired or suspended.
func (a *Agent) Introspect(ctx context.Context, prompt string) (string, error) {
	if err := a.checkCanEmit(); err != nil {
		return "", fmt.Errorf("introspect: %w", err)
	}
	memory, _ := a.runtime.Memory(20)
	resp, err := a.runtime.Provider().Reason(ctx, prompt, memory)
	if err != nil {
		return "", fmt.Errorf("introspect: %w", err)
	}

	// Best-effort observability: the introspection text is the primary output.
	_, _ = a.recordAndTrack(event.EventTypeAgentIntrospected.Value(), event.AgentIntrospectedContent{
		AgentID:     a.runtime.ID(),
		Observation: resp.Content(),
	})

	return resp.Content(), nil
}

// Act records an action annotation event on the graph.
// Used to mark significant actions (e.g. "write_code", "integrate") for observability.
// Emits agent.acted via graph.Record() (bus-visible, hash-chain safe).
// Returns an error if the agent is retired or suspended.
func (a *Agent) Act(ctx context.Context, action, target string) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("act: %w", err)
	}
	_, err := a.recordAndTrack(event.EventTypeAgentActed.Value(), event.AgentActedContent{
		AgentID: a.runtime.ID(),
		Action:  action,
		Target:  target,
	})
	if err != nil {
		return fmt.Errorf("act: %w", err)
	}
	return nil
}

// Research reads a URL and extracts information via the LLM.
// Returns the evaluation text. Emits agent.evaluated via graph.Record().
// Returns an error if the agent is retired or suspended.
func (a *Agent) Research(ctx context.Context, url, extractionPrompt string) (string, error) {
	if err := a.checkCanEmit(); err != nil {
		return "", fmt.Errorf("research: %w", err)
	}
	fullPrompt := fmt.Sprintf("Read the following URL and %s\n\nURL: %s", extractionPrompt, url)
	memory, _ := a.runtime.Memory(5)
	resp, err := a.runtime.Provider().Reason(ctx, fullPrompt, memory)
	if err != nil {
		return "", fmt.Errorf("research: %w", err)
	}

	_, err = a.recordAndTrack(event.EventTypeAgentEvaluated.Value(), event.AgentEvaluatedContent{
		AgentID:    a.runtime.ID(),
		Subject:    "research:" + url,
		Confidence: resp.Confidence(),
		Result:     resp.Content(),
	})
	if err != nil {
		return "", fmt.Errorf("research: record: %w", err)
	}

	return resp.Content(), nil
}

// truncate shortens a string to maxLen, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
