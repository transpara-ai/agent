package agent

import (
	"strings"
	"testing"
	"time"

	egagent "github.com/transpara-ai/eventgraph/go/pkg/agent"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
)

type specEmitCase struct {
	name     string
	wantType string
	emit     func(a *Agent) error
}

func specEmitCases() []specEmitCase {
	now := time.Unix(1700000000, 0).UTC()
	return []specEmitCase{
		{
			name:     "EmitSpecIngested",
			wantType: "hive.spec.ingested",
			emit: func(a *Agent) error {
				return a.EmitSpecIngested(event.SpecIngestedContent{
					SpecRef:    "spec_test_1",
					SourceOpID: "evt_source_1",
					IngestedAt: now,
				})
			},
		},
		{
			name:     "EmitSpecCompleted",
			wantType: "hive.spec.completed",
			emit: func(a *Agent) error {
				return a.EmitSpecCompleted(event.SpecCompletedContent{
					SpecRef:     "spec_test_1",
					Outcome:     "success",
					CompletedAt: now,
				})
			},
		},
	}
}

func TestSpecEmittersHappyPath(t *testing.T) {
	for _, tc := range specEmitCases() {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestAgent(t, tc.name+"_happy")
			if err := tc.emit(a); err != nil {
				t.Fatalf("%s: unexpected error: %v", tc.name, err)
			}
			last := a.LastEvent()
			if last.IsZero() {
				t.Fatalf("%s: LastEvent is zero after emit", tc.name)
			}
			ev, err := a.Graph().Store().Get(last)
			if err != nil {
				t.Fatalf("%s: Get(%s): %v", tc.name, last.Value(), err)
			}
			if got := ev.Type().Value(); got != tc.wantType {
				t.Fatalf("%s: emitted event type = %q, want %q", tc.name, got, tc.wantType)
			}
		})
	}
}

func TestSpecEmittersRetired(t *testing.T) {
	for _, tc := range specEmitCases() {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestAgent(t, tc.name+"_retired")
			a.mu.Lock()
			a.state = egagent.StateRetired
			a.mu.Unlock()

			err := tc.emit(a)
			if err == nil {
				t.Fatalf("%s: expected error on retired agent, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), "agent is retired") {
				t.Fatalf("%s: error = %q, want it to contain %q", tc.name, err.Error(), "agent is retired")
			}
		})
	}
}

func TestSpecEmittersSuspended(t *testing.T) {
	for _, tc := range specEmitCases() {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestAgent(t, tc.name+"_suspended")
			a.mu.Lock()
			a.state = egagent.StateSuspended
			a.mu.Unlock()

			err := tc.emit(a)
			if err == nil {
				t.Fatalf("%s: expected error on suspended agent, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), "agent is suspended") {
				t.Fatalf("%s: error = %q, want it to contain %q", tc.name, err.Error(), "agent is suspended")
			}
		})
	}
}
