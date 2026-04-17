package agent

import (
	"fmt"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
)

// EmitSpecIngested records spec document recognition and queueing.
// Returns an error if the agent is retired or suspended.
func (a *Agent) EmitSpecIngested(content event.SpecIngestedContent) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("spec ingested: %w", err)
	}
	_, err := a.recordAndTrack(event.EventTypeSpecIngested.Value(), content)
	if err != nil {
		return fmt.Errorf("spec ingested: %w", err)
	}
	return nil
}

// EmitSpecCompleted records successful closure of all spec-derived tasks.
// Returns an error if the agent is retired or suspended.
func (a *Agent) EmitSpecCompleted(content event.SpecCompletedContent) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("spec completed: %w", err)
	}
	_, err := a.recordAndTrack(event.EventTypeSpecCompleted.Value(), content)
	if err != nil {
		return fmt.Errorf("spec completed: %w", err)
	}
	return nil
}
