package agent

import (
	"fmt"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
)

// EmitGapDetected records a detected gap in the hive's composition.
// Returns an error if the agent is retired or suspended.
func (a *Agent) EmitGapDetected(content event.GapDetectedContent) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("gap detected: %w", err)
	}
	_, err := a.recordAndTrack(event.EventTypeGapDetected.Value(), content)
	if err != nil {
		return fmt.Errorf("gap detected: %w", err)
	}
	return nil
}

// EmitDirective records a directive issued by the CTO agent.
// Returns an error if the agent is retired or suspended.
func (a *Agent) EmitDirective(content event.DirectiveIssuedContent) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("directive: %w", err)
	}
	_, err := a.recordAndTrack(event.EventTypeDirectiveIssued.Value(), content)
	if err != nil {
		return fmt.Errorf("directive: %w", err)
	}
	return nil
}
