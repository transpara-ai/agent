package agent

import (
	"fmt"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
)

// EmitKnowledgeInsight records a distilled knowledge insight on the graph.
// Returns an error if the agent is retired or suspended.
func (a *Agent) EmitKnowledgeInsight(content event.KnowledgeInsightContent) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("knowledge insight: %w", err)
	}
	_, err := a.recordAndTrack(event.EventTypeKnowledgeInsightRecorded.Value(), content)
	if err != nil {
		return fmt.Errorf("knowledge insight: %w", err)
	}
	return nil
}

// EmitKnowledgeSupersession records the replacement of an outdated insight.
// Returns an error if the agent is retired or suspended.
func (a *Agent) EmitKnowledgeSupersession(content event.KnowledgeSupersessionContent) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("knowledge supersession: %w", err)
	}
	_, err := a.recordAndTrack(event.EventTypeKnowledgeInsightSuperseded.Value(), content)
	if err != nil {
		return fmt.Errorf("knowledge supersession: %w", err)
	}
	return nil
}

// EmitKnowledgeExpiration records the TTL-based expiration of an insight.
// Returns an error if the agent is retired or suspended.
func (a *Agent) EmitKnowledgeExpiration(content event.KnowledgeExpirationContent) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("knowledge expiration: %w", err)
	}
	_, err := a.recordAndTrack(event.EventTypeKnowledgeInsightExpired.Value(), content)
	if err != nil {
		return fmt.Errorf("knowledge expiration: %w", err)
	}
	return nil
}
