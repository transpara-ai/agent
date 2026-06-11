package agent

import (
	"testing"

	egagent "github.com/transpara-ai/eventgraph/go/pkg/agent"
)

// TestResetIfStuckProcessing pins the allowlisted stranded-state recovery
// (hive v13-F1 fix set, codex rounds 1+2): the ONLY state a recovery reset
// may leave is Processing — the shape left behind when a failed operation's
// cleanup transition (Processing → Idle) could not record its state event
// and rolled back (OBSERVABLE invariant). Authority states are untouchable:
// an unconditional reset silently revived Guardian-suspended agents (r1),
// while no reset at all turned a store hiccup into a terminal loop death
// (r2). The gate must be atomic — check and reset under one lock.
func TestResetIfStuckProcessing(t *testing.T) {
	t.Run("resets from Processing", func(t *testing.T) {
		a := newTestAgent(t, "reset-stuck-processing")
		if err := a.transitionTo(egagent.StateProcessing); err != nil {
			t.Fatal(err)
		}
		if !a.ResetIfStuckProcessing() {
			t.Fatal("want reset=true from Processing (the stranded shape)")
		}
		if got := a.State(); got != egagent.StateIdle {
			t.Fatalf("state = %s, want Idle after recovery", got)
		}
	})

	t.Run("no-op from Idle", func(t *testing.T) {
		a := newTestAgent(t, "reset-noop-idle")
		if a.ResetIfStuckProcessing() {
			t.Fatal("want reset=false from Idle — nothing is stranded")
		}
		if got := a.State(); got != egagent.StateIdle {
			t.Fatalf("state = %s, want Idle unchanged", got)
		}
	})

	t.Run("never touches Suspended", func(t *testing.T) {
		a := newTestAgent(t, "reset-keeps-suspended")
		if err := a.Suspend(); err != nil {
			t.Fatal(err)
		}
		if a.ResetIfStuckProcessing() {
			t.Fatal("want reset=false from Suspended — authority is not a stranded state")
		}
		if got := a.State(); got != egagent.StateSuspended {
			t.Fatalf("state = %s, want Suspended (a recovery reset must never revive a Guardian suspension)", got)
		}
	})
}
