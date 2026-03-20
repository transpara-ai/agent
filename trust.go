package agent

import (
	"context"
	"fmt"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/trust"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
)

// RecordVerifiedWork updates trust after successful work.
// Called by the pipeline after successful reviews, test passes, etc.
// The evidence event is the specific event that demonstrates the verified work.
func (a *Agent) RecordVerifiedWork(ctx context.Context, trustModel trust.ITrustModel, evidence event.Event) error {
	actor, err := a.graph.ActorStore().Get(a.runtime.ID())
	if err != nil {
		return fmt.Errorf("trust: get actor: %w", err)
	}

	// Capture previous score before the update.
	previous, _ := trustModel.Score(ctx, actor)

	metrics, err := trustModel.Update(ctx, actor, evidence)
	if err != nil {
		return fmt.Errorf("trust: update: %w", err)
	}

	// Emit trust assessment event on the graph.
	_, err = a.recordAndTrack(event.EventTypeTrustUpdated.Value(), event.TrustUpdatedContent{
		Actor:    a.runtime.ID(),
		Previous: previous.Overall(),
		Current:  metrics.Overall(),
		Domain:   types.MustDomainScope("hive"),
		Cause:    evidence.ID(),
	})
	if err != nil {
		return fmt.Errorf("trust: record event: %w", err)
	}

	return nil
}
