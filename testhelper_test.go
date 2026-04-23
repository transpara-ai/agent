package agent

import (
	"context"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/actor"
	"github.com/transpara-ai/eventgraph/go/pkg/decision"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/graph"
	"github.com/transpara-ai/eventgraph/go/pkg/intelligence"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

type mockProvider struct{}

func (mockProvider) Name() string  { return "mock" }
func (mockProvider) Model() string { return "mock-model" }
func (mockProvider) Reason(_ context.Context, _ string, _ []event.Event) (decision.Response, error) {
	confidence, _ := types.NewScore(0.8)
	return decision.NewResponse("ok", confidence, decision.TokenUsage{InputTokens: 1, OutputTokens: 1}), nil
}

var _ intelligence.Provider = (*mockProvider)(nil)

// newTestAgent builds an in-memory Agent suitable for emitter tests.
// Returns the agent in its post-boot Idle state.
func newTestAgent(t *testing.T, name string) *Agent {
	t.Helper()
	s := store.NewInMemoryStore()
	as := actor.NewInMemoryActorStore()
	g := graph.New(s, as)
	if err := g.Start(); err != nil {
		t.Fatalf("graph.Start: %v", err)
	}
	t.Cleanup(func() { _ = g.Close() })

	a, err := New(context.Background(), Config{
		Role:     "test",
		Name:     name,
		Graph:    g,
		Provider: &mockProvider{},
		Model:    "mock-model",
		CostTier: "standard",
	})
	if err != nil {
		t.Fatalf("agent.New: %v", err)
	}
	return a
}
