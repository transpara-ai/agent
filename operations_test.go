package agent

import (
	"context"
	"strings"
	"testing"

	egagent "github.com/transpara-ai/eventgraph/go/pkg/agent"
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
	defaultProvider := a.Provider()
	provider := &mockOperateProvider{}

	if got := a.State(); got != egagent.StateIdle {
		t.Fatalf("initial state = %s, want %s", got, egagent.StateIdle)
	}
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
	if got := a.State(); got != egagent.StateIdle {
		t.Fatalf("state after OperateWithProvider = %s, want %s", got, egagent.StateIdle)
	}
	if got := a.Provider(); got != defaultProvider {
		t.Fatalf("default provider changed after OperateWithProvider")
	}

	events, err := a.runtime.EventsByType(event.EventTypeAgentActed.Value(), 10)
	if err != nil {
		t.Fatalf("EventsByType(%s): %v", event.EventTypeAgentActed.Value(), err)
	}
	if len(events) == 0 {
		t.Fatalf("OperateWithProvider did not record %s", event.EventTypeAgentActed.Value())
	}
}

func TestOperateWithProviderRejectsNilProvider(t *testing.T) {
	a := newTestAgent(t, "operate_nil_provider")

	_, err := a.OperateWithProvider(context.Background(), nil, t.TempDir(), "touch the thing")
	if err == nil {
		t.Fatal("OperateWithProvider nil provider error = nil, want error")
	}
	if !strings.Contains(err.Error(), "provider is required") {
		t.Fatalf("error = %q, want provider required", err.Error())
	}
	if got := a.State(); got != egagent.StateIdle {
		t.Fatalf("state after nil provider = %s, want %s", got, egagent.StateIdle)
	}
}

func TestOperateWithProviderRejectsNonOperatorBeforeStateChange(t *testing.T) {
	a := newTestAgent(t, "operate_non_operator")

	_, err := a.OperateWithProvider(context.Background(), &mockProvider{}, t.TempDir(), "touch the thing")
	if err == nil {
		t.Fatal("OperateWithProvider non-operator error = nil, want error")
	}
	if !strings.Contains(err.Error(), "provider does not support Operate") {
		t.Fatalf("error = %q, want non-operator error", err.Error())
	}
	if got := a.State(); got != egagent.StateIdle {
		t.Fatalf("state after non-operator = %s, want %s", got, egagent.StateIdle)
	}
	events, eventErr := a.runtime.EventsByType(event.EventTypeAgentActed.Value(), 10)
	if eventErr != nil {
		t.Fatalf("EventsByType(%s): %v", event.EventTypeAgentActed.Value(), eventErr)
	}
	if len(events) != 0 {
		t.Fatalf("non-operator path recorded %d %s events, want 0", len(events), event.EventTypeAgentActed.Value())
	}
}

func TestOperateWithProviderRejectsTerminalStatesWithoutCallingProvider(t *testing.T) {
	tests := []struct {
		name  string
		state egagent.OperationalState
	}{
		{name: "retired", state: egagent.StateRetired},
		{name: "suspended", state: egagent.StateSuspended},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newTestAgent(t, "operate_"+tt.name)
			provider := &mockOperateProvider{}
			a.mu.Lock()
			a.state = tt.state
			a.mu.Unlock()

			_, err := a.OperateWithProvider(context.Background(), provider, t.TempDir(), "touch the thing")
			if err == nil {
				t.Fatalf("OperateWithProvider in %s error = nil, want error", tt.state)
			}
			if !strings.Contains(err.Error(), "invalid transition") {
				t.Fatalf("error = %q, want invalid transition", err.Error())
			}
			if provider.calls != 0 {
				t.Fatalf("provider calls = %d, want 0", provider.calls)
			}
			if got := a.State(); got != tt.state {
				t.Fatalf("state after rejected operate = %s, want %s", got, tt.state)
			}
		})
	}
}

// capturingOperateProvider records the exact OperateTask handed to the
// provider so tests can assert on the final composed instruction.
type capturingOperateProvider struct {
	mockOperateProvider
	lastTask decision.OperateTask
}

func (p *capturingOperateProvider) Operate(ctx context.Context, task decision.OperateTask) (decision.OperateResult, error) {
	p.lastTask = task
	return p.mockOperateProvider.Operate(ctx, task)
}

// TestOperatePinsWorkspaceContainmentPreamble guards the instruction-pinning
// half of slice-1 Finding 18 (v10-F2): prompt-layer guidance stating the
// boundary at the one surface the Operate subprocess actually reads — the
// v10 round-3 implementer walked out of its workspace while following gate
// text that demanded it. Enforcement is NOT this package's claim: the hive
// tripwire (hive#152) and provider credential stripping (eventgraph#50)
// apply only when those layers are in the stack.
func TestOperatePinsWorkspaceContainmentPreamble(t *testing.T) {
	a := newTestAgent(t, "operate_pinning")
	provider := &capturingOperateProvider{}
	ws := t.TempDir()

	if _, err := a.OperateWithProvider(context.Background(), provider, ws, "write the catalog file"); err != nil {
		t.Fatalf("OperateWithProvider: %v", err)
	}

	got := provider.lastTask
	if got.WorkDir != ws {
		t.Fatalf("WorkDir = %q, want %q", got.WorkDir, ws)
	}
	if !strings.HasPrefix(got.Instruction, "== WORKSPACE CONTAINMENT POLICY ==") {
		t.Errorf("instruction does not begin with the containment preamble:\n%s", got.Instruction)
	}
	for _, phrase := range []string{
		"Your assigned workspace is " + ws,
		"OUT OF BOUNDS",
		"Do not rely on\nruntime checks or credential failures as permission to try outside work.",
		"do NOT attempt it",
		"state the conflict plainly in your summary",
	} {
		if !strings.Contains(got.Instruction, phrase) {
			t.Errorf("instruction missing containment clause %q", phrase)
		}
	}
	if !strings.HasSuffix(got.Instruction, "write the catalog file") {
		t.Errorf("original instruction must follow the preamble verbatim; got tail %q",
			got.Instruction[max(0, len(got.Instruction)-60):])
	}
}

// An empty workDir gets NO preamble: a pin naming a wrong/empty boundary is
// worse than none, and empty-workDir semantics (provider-resolved cwd) are
// preserved unchanged for callers outside the hive Operate path.
func TestOperateEmptyWorkDirGetsNoFalsePin(t *testing.T) {
	a := newTestAgent(t, "operate_no_false_pin")
	provider := &capturingOperateProvider{}

	if _, err := a.OperateWithProvider(context.Background(), provider, "", "inspect runtime state"); err != nil {
		t.Fatalf("OperateWithProvider: %v", err)
	}
	if got := provider.lastTask.Instruction; got != "inspect runtime state" {
		t.Errorf("empty-workDir instruction was modified: %q", got)
	}
}
