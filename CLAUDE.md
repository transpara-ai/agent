# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
go build ./...   # Build
go test ./...    # Run tests
go vet ./...     # Static analysis
```

To run a single test:
```bash
go test -run TestName ./...
```

The `eventgraph` dependency is a local module — check `go.mod` for the replace directive pointing to its local path.

## Architecture

This is the `github.com/transpara-ai/agent` Go package — the **universal Agent abstraction** for the Transpara AI hive. Every agent in the hive (Mind, CTO, Guardian, SysMon, Allocator, etc.) is an instance of this type.

### Core Type

`Agent` (agent.go) wraps `intelligence.AgentRuntime` from `eventgraph` and adds:
- Deterministic Ed25519 signing key derived via `SHA256("agent:" + name)`
- FSM operational state (`Idle → Processing → Escalating/Refusing/Waiting → Retiring → Retired`, plus `Suspended`)
- Causality tracking via `lastEvent` — each event is caused by the previous event, not `store.Head()`
- Conversation threading (`convID`)

### Key Invariants

**OBSERVABLE:** Every FSM state change must emit a signed event on the chain. If emission fails, the state is rolled back. No hidden off-chain state.

**Causality chain:** `recordAndTrack()` updates `lastEvent` so each event causes the next. The first event uses `graph.Store().Head()` as cause to anchor the chain.

**Guard before emit:** `checkCanEmit()` blocks all event emission if the agent is retired or suspended. All public methods call this first.

**TOCTOU in Retire:** `resolveAndRetire()` holds `a.mu` across state observation + transition to prevent races.

**DIGNITY:** `Retire()` completes its shutdown ceremony without LLM calls (uses direct `recordAndTrack` for introspection) to avoid hanging if the provider is unresponsive.

### File Map

| File | Purpose |
|------|---------|
| `agent.go` | `New()`, `boot()`, `record()`, `recordAndTrack()`, accessors |
| `operations.go` | `Reason()`, `Operate()`, `Observe()`, `Evaluate()`, `Communicate()`, `Learn()`, `Escalate()`, `Refuse()`, `Introspect()`, `Act()`, `Research()` |
| `transitions.go` | `transitionTo()`, `transitionLocked()`, `Suspend()`, `Resume()` |
| `retire.go` | `Retire()`, `retireCeremony()`, `resolveAndRetire()` |
| `budget.go` | `EmitBudgetAllocated()`, `EmitBudgetAdjusted()`, `EmitBudgetExhausted()` |
| `trust.go` | `RecordVerifiedWork()` — locks `a.mu` across Score→Update→record |
| `health.go` | `EmitHealthReport()` |
| `cto.go` | `EmitGapDetected()`, `EmitDirective()` — CTO-specific chain events |

### Bootstrap Sequence

`New()` requires `cfg.Graph.Start()` to have been called first. Boot order emits: identity → soul → model → authority → state (Idle). This sequence is defined by `egagent.BootEvents()` from the eventgraph package.

### Mutex Discipline

- `a.mu` protects `state`, `lastEvent`, and all atomic emit sequences
- Lock ordering: `a.mu` → trustModel's lock → graph.mu
- `retireCeremony()` runs entirely under `a.mu`
- `transitionLocked()` is used when already holding the lock

### Event Routing

All events go through `graph.Record()` — never through `runtime.Emit()` directly (see commit history for why this matters for replay consistency).

### Design Documents

`docs/designs/` contains the CTO design spec (`cto-design-v1.1.0.md`) and implementation prompts. These are authoritative for CTO feature work but the code is truth when they conflict.
