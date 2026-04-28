package agent

import (
	"fmt"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
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

// EmitRefineryIntakeReceived records raw refinery intake receipt.
// Returns an error if the agent is retired or suspended.
func (a *Agent) EmitRefineryIntakeReceived(content event.RefineryIntakeReceivedContent) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("refinery intake received: %w", err)
	}
	_, err := a.recordAndTrack(event.EventTypeRefineryIntakeReceived.Value(), content)
	if err != nil {
		return fmt.Errorf("refinery intake received: %w", err)
	}
	return nil
}

// EmitRefineryArtifactAttached records a refinery artifact attachment.
// Returns an error if the agent is retired or suspended.
func (a *Agent) EmitRefineryArtifactAttached(content event.RefineryArtifactAttachedContent) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("refinery artifact attached: %w", err)
	}
	_, err := a.recordAndTrack(event.EventTypeRefineryArtifactAttached.Value(), content)
	if err != nil {
		return fmt.Errorf("refinery artifact attached: %w", err)
	}
	return nil
}

// EmitRefineryIntakeClassified records the classifier recommendation.
// Returns an error if the agent is retired or suspended.
func (a *Agent) EmitRefineryIntakeClassified(content event.RefineryIntakeClassifiedContent) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("refinery intake classified: %w", err)
	}
	_, err := a.recordAndTrack(event.EventTypeRefineryIntakeClassified.Value(), content)
	if err != nil {
		return fmt.Errorf("refinery intake classified: %w", err)
	}
	return nil
}

// EmitRefineryStateTransitioned records an actual refinery FSM transition.
// Returns an error if the agent is retired or suspended.
func (a *Agent) EmitRefineryStateTransitioned(content event.RefineryStateTransitionedContent) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("refinery state transitioned: %w", err)
	}
	_, err := a.recordAndTrack(event.EventTypeRefineryStateTransitioned.Value(), content)
	if err != nil {
		return fmt.Errorf("refinery state transitioned: %w", err)
	}
	return nil
}
