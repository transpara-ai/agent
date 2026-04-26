package agent

import (
	"fmt"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
)

// EmitCodeReview emits a code.review.submitted event on the chain.
// Called by the loop when parsing a /review command from the Reviewer agent.
func (a *Agent) EmitCodeReview(content event.CodeReviewContent) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("code review: %w", err)
	}
	_, err := a.recordAndTrack(event.EventTypeCodeReviewSubmitted.Value(), content)
	if err != nil {
		return fmt.Errorf("code review: %w", err)
	}
	return nil
}
