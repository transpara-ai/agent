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
//
// Holds a.mu across Score → Update → record to ensure the Previous value
// in the emitted TrustUpdatedContent is consistent — without this, concurrent
// calls could read stale Previous scores.
func (a *Agent) RecordVerifiedWork(ctx context.Context, trustModel trust.ITrustModel, evidence event.Event) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("trust: %w", err)
	}

	actor, err := a.graph.ActorStore().Get(a.runtime.ID())
	if err != nil {
		return fmt.Errorf("trust: get actor: %w", err)
	}

	// Hold a.mu across the entire Score → Update → record sequence.
	// Lock ordering: a.mu → trustModel's internal lock → graph.mu.
	a.mu.Lock()
	defer a.mu.Unlock()

	// Capture previous score before the update.
	previous, _ := trustModel.Score(ctx, actor)

	metrics, err := trustModel.Update(ctx, actor, evidence)
	if err != nil {
		return fmt.Errorf("trust: update: %w", err)
	}

	// Emit trust assessment event on the graph.
	ev, err := a.record(event.EventTypeTrustUpdated.Value(), event.TrustUpdatedContent{
		Actor:    a.runtime.ID(),
		Previous: previous.Overall(),
		Current:  metrics.Overall(),
		Domain:   types.MustDomainScope("hive"),
		Cause:    evidence.ID(),
	})
	if err != nil {
		return fmt.Errorf("trust: record event: %w", err)
	}
	a.lastEvent = ev.ID()

	return nil
}
