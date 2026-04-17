package agent

import (
	"strings"
	"testing"
	"time"

	egagent "github.com/lovyou-ai/eventgraph/go/pkg/agent"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
)

// siteEmitCase is one emitter under test. emit invokes the method on the
// agent with a representative content value; wantType is the expected event
// type name emitted on the happy path.
type siteEmitCase struct {
	name     string
	wantType string
	emit     func(a *Agent) error
}

func siteEmitCases() []siteEmitCase {
	ref := event.ExternalRef{System: "site", ID: "op_test1"}
	now := time.Unix(1700000000, 0).UTC()

	return []siteEmitCase{
		{
			name:     "EmitSiteOpReceived",
			wantType: "site.op.received",
			emit: func(a *Agent) error {
				return a.EmitSiteOpReceived(event.SiteOpReceivedContent{
					ExternalRef:   ref,
					SpaceID:       "space_1",
					Actor:         "alice",
					ActorID:       "u_alice",
					ActorKind:     "user",
					OpKind:        "created",
					PayloadHash:   "sha256:deadbeef",
					ReceivedAt:    now,
					SiteCreatedAt: now,
				})
			},
		},
		{
			name:     "EmitSiteOpTranslated",
			wantType: "site.op.translated",
			emit: func(a *Agent) error {
				return a.EmitSiteOpTranslated(event.SiteOpTranslatedContent{
					ExternalRef:  ref,
					BusEventID:   "evt_translated_1",
					TranslatedAt: now,
				})
			},
		},
		{
			name:     "EmitSiteOpRejected",
			wantType: "site.op.rejected",
			emit: func(a *Agent) error {
				return a.EmitSiteOpRejected(event.SiteOpRejectedContent{
					ExternalRef: ref,
					Reason:      "schema mismatch",
					RejectedAt:  now,
				})
			},
		},
		{
			name:     "EmitSiteOpMirrored",
			wantType: "site.op.mirrored",
			emit: func(a *Agent) error {
				return a.EmitSiteOpMirrored(event.SiteOpMirroredContent{
					ExternalRef:   ref,
					MirrorEventID: "evt_mirror_1",
					HTTPStatus:    200,
					MirroredAt:    now,
				})
			},
		},
	}
}

func TestSiteOpEmittersHappyPath(t *testing.T) {
	for _, tc := range siteEmitCases() {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestAgent(t, tc.name+"_happy")
			if err := tc.emit(a); err != nil {
				t.Fatalf("%s: unexpected error: %v", tc.name, err)
			}
			// LastEvent must point at the event just emitted.
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

func TestSiteOpEmittersRetired(t *testing.T) {
	for _, tc := range siteEmitCases() {
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

func TestSiteOpEmittersSuspended(t *testing.T) {
	for _, tc := range siteEmitCases() {
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
