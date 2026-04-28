package agent

import (
	"strings"
	"testing"
	"time"

	egagent "github.com/transpara-ai/eventgraph/go/pkg/agent"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
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
					SourceOpID: types.MustEventID("01900000-0000-7000-8000-000000000003"),
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
		{
			name:     "EmitRefineryIntakeReceived",
			wantType: "refinery.intake.received",
			emit: func(a *Agent) error {
				return a.EmitRefineryIntakeReceived(event.RefineryIntakeReceivedContent{IntakeRef: "site-node-1", SpaceID: "space-1", Title: "Spec", Actor: "Tester", ActorID: "u1", ArtifactCount: 1, ReceivedAt: now})
			},
		},
		{
			name:     "EmitRefineryArtifactAttached",
			wantType: "refinery.artifact.attached",
			emit: func(a *Agent) error {
				return a.EmitRefineryArtifactAttached(event.RefineryArtifactAttachedContent{IntakeRef: "site-node-1", ArtifactRef: "doc-1", Filename: "spec.md", Hash: "sha256:abc", AttachedAt: now})
			},
		},
		{
			name:     "EmitRefineryIntakeClassified",
			wantType: "refinery.intake.classified",
			emit: func(a *Agent) error {
				return a.EmitRefineryIntakeClassified(event.RefineryIntakeClassifiedContent{IntakeRef: "site-node-1", ClassifierKind: "spec", RecommendedState: "spec.draft", PersistedState: "intake.raw", ArtifactCount: 1, ClassifiedAt: now})
			},
		},
		{
			name:     "EmitRefineryStateTransitioned",
			wantType: "refinery.state.transitioned",
			emit: func(a *Agent) error {
				return a.EmitRefineryStateTransitioned(event.RefineryStateTransitionedContent{IntakeRef: "site-node-1", FromState: "", ToState: "intake.raw", Reason: "all submissions start as raw intake", TransitionedAt: now})
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
