package agent

import (
	"fmt"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
)

// EmitBudgetAllocated records a budget allocation event on the graph.
// Returns an error if the agent is retired or suspended.
func (a *Agent) EmitBudgetAllocated(maxTokens int, maxCostUSD float64) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("budget allocated: %w", err)
	}
	_, err := a.recordAndTrack(event.EventTypeAgentBudgetAllocated.Value(), event.AgentBudgetAllocatedContent{
		AgentID:    a.runtime.ID(),
		TokenLimit: maxTokens,
		CostLimit:  maxCostUSD,
	})
	if err != nil {
		return fmt.Errorf("budget allocated: %w", err)
	}
	return nil
}

// EmitBudgetAdjusted records a budget reallocation by the Allocator.
// Returns an error if the agent is retired or suspended.
func (a *Agent) EmitBudgetAdjusted(content event.AgentBudgetAdjustedContent) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("budget adjusted: %w", err)
	}
	_, err := a.recordAndTrack(event.EventTypeAgentBudgetAdjusted.Value(), content)
	if err != nil {
		return fmt.Errorf("budget adjusted: %w", err)
	}
	return nil
}

// EmitBudgetExhausted records that a budget limit has been reached.
// Returns an error if the agent is retired or suspended.
func (a *Agent) EmitBudgetExhausted(resource string) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("budget exhausted: %w", err)
	}
	_, err := a.recordAndTrack(event.EventTypeAgentBudgetExhausted.Value(), event.AgentBudgetExhaustedContent{
		AgentID:  a.runtime.ID(),
		Resource: resource,
	})
	if err != nil {
		return fmt.Errorf("budget exhausted: %w", err)
	}
	return nil
}
