package agent

import (
	"strings"
	"testing"

	egagent "github.com/transpara-ai/eventgraph/go/pkg/agent"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

type explicitCauseEmitCase struct {
	name     string
	wantType string
	emit     func(*Agent, types.EventID) error
}

func explicitCauseEmitCases() []explicitCauseEmitCase {
	return []explicitCauseEmitCase{
		{
			name:     "role proposed",
			wantType: event.EventTypeRoleProposed.Value(),
			emit: func(a *Agent, cause types.EventID) error {
				return a.EmitRoleProposedCausedBy(event.RoleProposedContent{Name: "dependency-owner"}, cause)
			},
		},
		{
			name:     "role approved",
			wantType: event.EventTypeRoleApproved.Value(),
			emit: func(a *Agent, cause types.EventID) error {
				return a.EmitRoleApprovedCausedBy(event.RoleApprovedContent{Name: "dependency-owner"}, cause)
			},
		},
		{
			name:     "role rejected",
			wantType: event.EventTypeRoleRejected.Value(),
			emit: func(a *Agent, cause types.EventID) error {
				return a.EmitRoleRejectedCausedBy(event.RoleRejectedContent{Name: "dependency-owner"}, cause)
			},
		},
		{
			name:     "budget adjusted",
			wantType: event.EventTypeAgentBudgetAdjusted.Value(),
			emit: func(a *Agent, cause types.EventID) error {
				return a.EmitBudgetAdjustedCausedBy(event.AgentBudgetAdjustedContent{AgentName: "dependency-owner", Action: "set", NewBudget: 40}, cause)
			},
		},
	}
}

func TestExplicitCauseEmittersPersistExactlyOneCauseAndTrackLastEvent(t *testing.T) {
	for _, tc := range explicitCauseEmitCases() {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestAgent(t, strings.ReplaceAll(tc.name, " ", "-")+"-exact")
			cause := a.LastEvent()
			if cause.IsZero() {
				t.Fatal("boot cause is zero")
			}
			if err := tc.emit(a, cause); err != nil {
				t.Fatalf("emit: %v", err)
			}
			last := a.LastEvent()
			if last == cause || last.IsZero() {
				t.Fatalf("last event was not advanced: %s", last.Value())
			}
			ev, err := a.Graph().Store().Get(last)
			if err != nil {
				t.Fatalf("get emitted event: %v", err)
			}
			if ev.Type().Value() != tc.wantType {
				t.Fatalf("event type = %q, want %q", ev.Type().Value(), tc.wantType)
			}
			causes := ev.Causes()
			if len(causes) != 1 || causes[0] != cause {
				t.Fatalf("causes = %v, want exactly [%s]", causes, cause.Value())
			}
		})
	}
}

func TestExplicitCauseEmittersRejectZeroWithoutEmission(t *testing.T) {
	for _, tc := range explicitCauseEmitCases() {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestAgent(t, strings.ReplaceAll(tc.name, " ", "-")+"-zero")
			before := a.LastEvent()
			err := tc.emit(a, types.EventID{})
			if err == nil || !strings.Contains(err.Error(), "explicit cause is zero") {
				t.Fatalf("error = %v, want explicit-cause rejection", err)
			}
			if a.LastEvent() != before {
				t.Fatalf("last event changed on rejected emission: %s -> %s", before.Value(), a.LastEvent().Value())
			}
		})
	}
}

func TestExplicitCauseEmittersPreserveLifecycleGuards(t *testing.T) {
	for _, state := range []egagent.OperationalState{egagent.StateRetired, egagent.StateSuspended} {
		for _, tc := range explicitCauseEmitCases() {
			t.Run(state.String()+"/"+tc.name, func(t *testing.T) {
				a := newTestAgent(t, state.String()+"-guard")
				cause := a.LastEvent()
				a.mu.Lock()
				a.state = state
				a.mu.Unlock()
				if err := tc.emit(a, cause); err == nil {
					t.Fatal("expected lifecycle error")
				}
				if a.LastEvent() != cause {
					t.Fatal("last event changed after lifecycle rejection")
				}
			})
		}
	}
}
