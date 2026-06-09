package agent

import (
	"context"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/decision"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
)

type mockOperateProvider struct {
	mockProvider
	calls int
}

func (p *mockOperateProvider) Name() string  { return "mock-operator" }
func (p *mockOperateProvider) Model() string { return "mock-operator-model" }
func (p *mockOperateProvider) Operate(_ context.Context, task decision.OperateTask) (decision.OperateResult, error) {
	p.calls++
	return decision.OperateResult{
		Summary: "operated in " + task.WorkDir,
		Usage: decision.TokenUsage{
			InputTokens:  2,
			OutputTokens: 3,
			CostUSD:      0.04,
		},
	}, nil
}

func TestOperateWithProviderUsesExplicitProviderAndRecordsActed(t *testing.T) {
	a := newTestAgent(t, "operate_override")
	provider := &mockOperateProvider{}

	result, err := a.OperateWithProvider(context.Background(), provider, t.TempDir(), "touch the thing")
	if err != nil {
		t.Fatalf("OperateWithProvider: %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("override provider calls = %d, want 1", provider.calls)
	}
	if result.Usage.Total() != 5 {
		t.Fatalf("usage total = %d, want 5", result.Usage.Total())
	}

	events, err := a.runtime.EventsByType(event.EventTypeAgentActed.Value(), 10)
	if err != nil {
		t.Fatalf("EventsByType(%s): %v", event.EventTypeAgentActed.Value(), err)
	}
	if len(events) == 0 {
		t.Fatalf("OperateWithProvider did not record %s", event.EventTypeAgentActed.Value())
	}
}
