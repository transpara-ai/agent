# Spawner Implementation — Claude Code Task Prompts

**Version:** 1.1.0
**Last Updated:** 2026-04-05
**Status:** Active
**Versioning:** Independent of all other documents. Major version increments reflect fundamental restructuring of the implementation approach; minor versions reflect adjustments from reconnaissance or implementation feedback; patch versions reflect corrections and clarifications.
**Author:** Claude Opus 4.6
**Owner:** Michael Saucier

---

### Revision History

| Version | Date | Description |
|---------|------|-------------|
| 1.0.0 | 2026-04-05 | Initial prompt sequence: 9 prompts (recon + 8 implementation PRs) |
| 1.1.0 | 2026-04-05 | Post-recon: marked Prompt 0 COMPLETE; added spawnerState to Prompt 3 (cross-iteration tracking like ctoCooldowns); updated Prompt 4 enrichment to use spawnerState; updated Prompt 6 Allocator to use /budget → agent.budget.adjusted for name-based correlation; rewrote Prompt 7 for runtime hot-add with dynamicAgentTracker (RunConcurrent is one-shot WaitGroup) |

---

## Usage

Feed these to Claude Code **ONE AT A TIME**, in order. Wait for each to
complete and verify before moving to the next. Do not skip ahead. Do not combine.

The Spawner touches all three repositories (eventgraph, agent, hive) and
modifies two other agents' prompts. This is the most complex implementation
sequence yet. Take it slow.

## Prerequisites

- Design spec `docs/designs/spawner-design.md` (v1.1.0) is committed to `lovyou-ai-hive`
- CTO agent is graduated and emitting `/gap` events
- You are in the `lovyou-ai-hive` repo root
- You have access to `lovyou-ai-eventgraph` and `lovyou-ai-agent` (as sibling directories or Go modules)

---

## Prompt 0 — Reconnaissance (COMPLETE)

Key findings:
- `pkg/runner/spawner.go` exists but is pipeline-mode legacy — no event graph,
  no reusable patterns. Clean-room implementation for pkg/loop/spawner.go.
- `agents/spawner.md` does not exist — must be created.
- RunConcurrent() is one-shot WaitGroup — hot-add needs dynamicAgentTracker
  with separate goroutine lifecycle management.
- Guardian has no command parsing — /approve and /reject built from scratch.
- pendingEvents flushed each iteration — cross-iteration state needs
  spawnerState (like ctoCooldowns), not just pendingEvents scanning.
- agent.budget.allocated has AgentID but no role name — use /budget mechanism
  producing agent.budget.adjusted with AgentName for correlation.
- BudgetRegistry.Register() thread-safe, supports hot-add. Confirmed.
- Telemetry RegisterAgent() thread-safe, supports hot-add. Confirmed.
- Agent.New() actor registration automatic and idempotent. Confirmed.
- Prompt delivery via RoleProposedContent.Prompt → AgentDef.SystemPrompt →
  intelligence.New(). Confirmed.
- StarterAgents: 7 agents, Guardian=500iter, Strategist/Planner=Sonnet.
  Spawner goes at index 4.

---

## Prompt 1 — Event Types in EventGraph

```
Read the Spawner design spec at docs/designs/spawner-design.md (v1.1.0),
section 8 (Event Types).

Using the EXACT pattern established by the CTO event types (hive.gap.detected
and hive.directive.issued — find them in the eventgraph codebase and follow
their pattern precisely), create three new event types.

WORK IN THE lovyou-ai-eventgraph REPOSITORY.

1. Add three event type constants (follow existing naming pattern):
   - hive.role.proposed
   - hive.role.approved
   - hive.role.rejected

2. Add three content structs (follow existing pattern for EventTypeName()
   and Accept() methods):

   RoleProposedContent:
     Name          string   `json:"name"`
     Model         string   `json:"model"`
     WatchPatterns []string `json:"watch_patterns"`
     CanOperate    bool     `json:"can_operate"`
     MaxIterations int      `json:"max_iterations"`
     Prompt        string   `json:"prompt"`
     Reason        string   `json:"reason"`
     ProposedBy    string   `json:"proposed_by"`

   RoleApprovedContent:
     Name       string `json:"name"`
     ApprovedBy string `json:"approved_by"`
     Reason     string `json:"reason"`

   RoleRejectedContent:
     Name       string `json:"name"`
     RejectedBy string `json:"rejected_by"`
     Reason     string `json:"reason"`

3. Register unmarshalers in the appropriate file (follow existing pattern)

4. Add to DefaultRegistry() (follow existing pattern)

5. Write tests:
   - JSON round-trip for each content struct
   - EventTypeName() returns correct string for each
   - Content registered and deserializable

Run all tests. Run the linter. Nothing breaks.

Commit with: "feat: register hive.role.proposed/approved/rejected event types

- RoleProposedContent carries complete role definition for governance review
- RoleApprovedContent/RoleRejectedContent carry disposition with reason
- Follows established hive.gap.detected/hive.directive.issued pattern

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Prompt 2 — Agent Emit Methods

```
Read the Spawner design spec at docs/designs/spawner-design.md (v1.1.0),
section 8 (Agent Emit Methods).

WORK IN THE lovyou-ai-agent REPOSITORY.

Using the EXACT pattern from the CTO's EmitGapDetected() and EmitDirective()
methods (find them in the agent codebase — likely in gap.go or cto.go),
create three new emit methods.

1. Create spawn.go (new file) with:

   func (a *Agent) EmitRoleProposed(content event.RoleProposedContent) error
   func (a *Agent) EmitRoleApproved(content event.RoleApprovedContent) error
   func (a *Agent) EmitRoleRejected(content event.RoleRejectedContent) error

   Each follows the pattern:
   - checkCanEmit()
   - recordAndTrack(EventType.Value(), content)
   - wrap errors with descriptive prefix

2. Run all tests. Run the linter.

Commit with: "feat: add EmitRoleProposed/Approved/Rejected methods

- EmitRoleProposed for Spawner role proposals
- EmitRoleApproved/Rejected for Guardian governance decisions
- Follows established EmitGapDetected/EmitDirective pattern

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Prompt 3 — Spawn Command Parsing, Validation, and State Tracking

```
Read the Spawner design spec at docs/designs/spawner-design.md (v1.1.0),
sections 7 (Command Mechanism) and 13 (Behavioral Constraints).

WORK IN THE lovyou-ai-hive REPOSITORY.

Create the spawn command infrastructure following the EXACT pattern used by
the CTO's gap/directive commands in pkg/loop/cto.go.

IMPORTANT ARCHITECTURE NOTE (from recon): l.pendingEvents is flushed each
iteration. Cross-iteration state (pending proposals, rejection history) CANNOT
rely on scanning pendingEvents — those only contain events since the last
flush. Use in-memory state tracking, same pattern as l.ctoCooldowns for CTO.

1. Create pkg/loop/spawner.go with:

   a. SpawnCommand struct (as in design spec section 7)

   b. spawnerState struct — persistent cross-iteration tracking:
      type spawnerState struct {
          pendingProposal  string            // name of role currently proposed (empty = none)
          recentRejections map[string]int    // role name → iteration when rejected
          processedGaps    map[string]bool   // gap event IDs already seen
          iteration        int               // current iteration counter
      }

      Initialize this in Loop.New() when agentDef.Role == "spawner",
      following the same pattern as ctoCooldowns initialization for CTO.

   c. updateSpawnerState(events []Event) method
      - Called at the start of each iteration with the current pendingEvents
      - Scan for hive.role.proposed → set pendingProposal
      - Scan for hive.role.approved → clear pendingProposal
      - Scan for hive.role.rejected → clear pendingProposal, record rejection
      - Scan for hive.gap.detected → track in processedGaps
      - Increment iteration counter

   d. SpawnContext struct — snapshot for validation (built from spawnerState):
      type SpawnContext struct {
          Iteration          int
          HasPendingProposal bool
          AgentRoster        []string        // from BudgetRegistry.Snapshot()
          RecentRejections   map[string]int  // from spawnerState
      }

   e. parseSpawnCommand(response string) *SpawnCommand
      - Same line-scanning pattern as parseGapCommand
      - Scan for "/spawn " prefix
      - Extract JSON payload
      - Return nil if not found or malformed

   f. validateSpawnCommand(cmd *SpawnCommand, ctx *SpawnContext) error
      - Stabilization window: ctx.Iteration < 20 → error
      - Pending proposal: ctx.HasPendingProposal → error
      - Name validation: kebab-case, 2-50 chars, no collisions with roster
      - Model validation: must be "haiku", "sonnet", or "opus"
        (use the model constants from agentdef.go — map human-readable names
        to actual model strings)
      - MaxIterations: must be 10-200
      - Prompt: must be non-empty, len >= 100
      - WatchPatterns: must be non-empty, no "*" wildcard
      - CanOperate: must be false (new roles can't operate)
      - Rejection cooldown: if same name in RecentRejections within 50 iterations, error

   g. isValidRoleName(name string) bool
      - kebab-case only: lowercase letters, digits, hyphens
      - 2-50 characters
      - Cannot start or end with hyphen
      - Cannot contain consecutive hyphens
      - Cannot be a reserved name: "guardian", "sysmon", "allocator", "cto",
        "spawner", "strategist", "planner", "implementer"

   h. emitRoleProposed(cmd *SpawnCommand) — method on Loop
      - Construct RoleProposedContent from SpawnCommand fields
      - Map model name to model constant:
        "haiku" → ModelHaiku, "sonnet" → ModelSonnet, "opus" → ModelOpus
      - Call l.agent.EmitRoleProposed(content)

2. Create pkg/loop/spawner_test.go with:

   - TestParseSpawnCommand_Valid
   - TestParseSpawnCommand_NoCommand
   - TestParseSpawnCommand_MalformedJSON
   - TestParseSpawnCommand_MultipleLines

   - TestValidateSpawnCommand_Valid
   - TestValidateSpawnCommand_StabilizationWindow
   - TestValidateSpawnCommand_PendingProposal
   - TestValidateSpawnCommand_NameCollision
   - TestValidateSpawnCommand_InvalidModel
   - TestValidateSpawnCommand_IterationsTooLow
   - TestValidateSpawnCommand_IterationsTooHigh
   - TestValidateSpawnCommand_PromptTooShort
   - TestValidateSpawnCommand_NoWatchPatterns
   - TestValidateSpawnCommand_WildcardWatch
   - TestValidateSpawnCommand_CanOperateBlocked
   - TestValidateSpawnCommand_RejectionCooldown

   - TestIsValidRoleName_Valid (several kebab-case names)
   - TestIsValidRoleName_Invalid (uppercase, spaces, reserved, too short/long)

   - TestUpdateSpawnerState_ProposalTracking
   - TestUpdateSpawnerState_RejectionTracking
   - TestUpdateSpawnerState_ApprovalClearsProposal

Run all tests including existing loop tests. Run the linter. Nothing breaks.

Commit with: "feat: add /spawn command parsing, validation, and spawnerState tracking

- parseSpawnCommand extracts role proposals from LLM output
- validateSpawnCommand enforces stabilization, pending, dedup, model, bounds
- spawnerState tracks cross-iteration proposals and rejections (like ctoCooldowns)
- isValidRoleName enforces kebab-case naming convention
- 20-iteration stabilization window (longest of any agent)
- CanOperate=true blocked for all spawned roles

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Prompt 4 — Observation Enrichment and Prompt

```
Read the Spawner design spec at docs/designs/spawner-design.md (v1.1.0),
sections 6 (Prompt File) and 7 (Observation Enrichment).

WORK IN THE lovyou-ai-hive REPOSITORY.

1. Create agents/spawner.md
   - Use the v1.1.0 prompt content from the design spec section 6
   - Follow the same ## section format as existing agent prompts
   - Verify it matches the format used by cto.md, guardian.md, etc.

2. Add observation enrichment to pkg/loop/spawner.go:

   IMPORTANT: Enrichment uses spawnerState (created in Prompt 3) for cross-
   iteration data, NOT l.pendingEvents scanning. pendingEvents is flushed
   each iteration and only contains events since the last flush.

   func (l *Loop) enrichSpawnObservation(obs string) string
   - Only activates when l.agentDef.Role == "spawner"
   - First, call l.updateSpawnerState(l.pendingEvents) to update tracking
   - Agent roster: use BudgetRegistry.Snapshot() to get all registered
     agents with their names, states, iteration counts
   - Pending proposals: read l.spawnerState.pendingProposal (in-memory)
   - Recent gaps: from spawnerState.processedGaps (which are unprocessed)
   - Recent outcomes: from spawnerState (rejections, cleared proposals)
   - Budget pool: l.config.BudgetRegistry.TotalPool() and TotalUsed()
   - Formats as the === SPAWN CONTEXT === block from design spec section 6
   - Appends to observation string and returns

   Use the same data access patterns that CTO enrichment uses for
   BudgetRegistry access. The key difference is that CTO only needs
   current-iteration data from pendingEvents, while Spawner needs
   cross-iteration state from spawnerState.

3. Wire enrichment into the loop:
   - In the observe() or buildPrompt() path (wherever CTO enrichment is
     wired), add the spawner enrichment call
   - Pattern: if role == "spawner", call enrichSpawnObservation()

4. Wire /spawn command processing into the loop:
   - In the command processing path (after processTaskCommands and other
     command handlers), add:
     if cmd := parseSpawnCommand(response); cmd != nil {
         ctx := l.buildSpawnContext()  // builds from spawnerState + BudgetRegistry
         if err := validateSpawnCommand(cmd, ctx); err != nil {
             // log validation failure but don't fail the loop
         } else {
             if err := l.emitRoleProposed(cmd); err != nil {
                 // log error but don't fail the loop
             }
         }
     }

   func (l *Loop) buildSpawnContext() *SpawnContext
   - Iteration from spawnerState.iteration
   - HasPendingProposal from spawnerState.pendingProposal != ""
   - AgentRoster from BudgetRegistry.Snapshot() names
   - RecentRejections from spawnerState.recentRejections

5. Add enrichment tests to pkg/loop/spawner_test.go:

   - TestEnrichSpawnObservation_Format — verify output contains
     "=== SPAWN CONTEXT ===" and expected sections
   - TestEnrichSpawnObservation_SkipsNonSpawner — verify non-spawner roles
     get observation unchanged
   - TestBuildSpawnContext — verify context construction from spawnerState

Run all tests. Run the linter. Nothing breaks.

Commit with: "feat: add spawner observation enrichment, prompt, and loop wiring

- enrichSpawnObservation uses spawnerState for cross-iteration tracking
- Wire /spawn command processing into loop alongside /task, /health, etc.
- Create agents/spawner.md with role design guidelines and /spawn format
- Follows established CTO enrichment pattern with spawnerState extension

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Prompt 5 — Guardian Governance Gate

```
Read the Spawner design spec at docs/designs/spawner-design.md (v1.1.0),
section 9 (Guardian Governance Gate).

WORK IN THE lovyou-ai-hive REPOSITORY.

The Guardian needs to evaluate spawn proposals and emit approve/reject decisions.
This is the first time Guardian gets its own command types.

1. Create Guardian command infrastructure in pkg/loop/guardian.go (new file):

   a. ApproveCommand struct:
      type ApproveCommand struct {
          Name   string `json:"name"`
          Reason string `json:"reason"`
      }

   b. RejectCommand struct:
      type RejectCommand struct {
          Name   string `json:"name"`
          Reason string `json:"reason"`
      }

   c. parseApproveCommand(response string) *ApproveCommand
      - Scan for "/approve " prefix, extract JSON
      - Return nil if not found

   d. parseRejectCommand(response string) *RejectCommand
      - Scan for "/reject " prefix, extract JSON
      - Return nil if not found

   e. emitRoleApproved(cmd *ApproveCommand) — method on Loop
      - Construct RoleApprovedContent
      - Call l.agent.EmitRoleApproved(content)

   f. emitRoleRejected(cmd *RejectCommand) — method on Loop
      - Construct RoleRejectedContent
      - Call l.agent.EmitRoleRejected(content)

2. Wire into the loop command processing:
   - After the spawner command handler, add:
     if cmd := parseApproveCommand(response); cmd != nil {
         if err := l.emitRoleApproved(cmd); err != nil {
             // log error
         }
     }
     if cmd := parseRejectCommand(response); cmd != nil {
         if err := l.emitRoleRejected(cmd); err != nil {
             // log error
         }
     }
   - These should only fire for the guardian role. Add a role check:
     if l.agentDef.Role == "guardian" { ... }

3. Update agents/guardian.md — add TWO new sections:

   a. ## Spawn Proposals
      When you see a hive.role.proposed event, evaluate it against:
      1. Soul alignment — does the prompt include the soul statement?
      2. Rights preservation — does the role respect agent rights?
      3. Invariant compliance — is it BOUNDED? OBSERVABLE? MARGIN-safe?
      4. Sanity — valid name? appropriate model? specific watch patterns?
      5. Necessity — does the reason cite actual evidence?

      If the proposal passes all checks, emit:
      /approve {"name":"role-name","reason":"Soul present, rights preserved, ..."}

      If the proposal fails any check, emit:
      /reject {"name":"role-name","reason":"Specific reason for rejection"}

      Always provide a clear reason. The Spawner uses rejection reasons to
      refine reproposals.

   b. ## Spawner Awareness
      Monitor the Spawner's behavior. If the Spawner proposes roles without
      gap events (speculative proposals), or proposes too frequently, or
      proposes roles with overly broad watch patterns, note these patterns.

      If Spawner stops emitting any events for approximately 25 iterations,
      escalate to human (same pattern as SysMon absence detection).

4. Create pkg/loop/guardian_test.go with:
   - TestParseApproveCommand_Valid
   - TestParseApproveCommand_NoCommand
   - TestParseApproveCommand_MalformedJSON
   - TestParseRejectCommand_Valid
   - TestParseRejectCommand_NoCommand
   - TestParseRejectCommand_MalformedJSON

Run all tests. Run the linter. Nothing breaks.

Commit with: "feat: add guardian /approve and /reject commands for spawn governance

- parseApproveCommand/parseRejectCommand for spawn proposal evaluation
- emitRoleApproved/emitRoleRejected produce chain events
- Guardian prompt updated with spawn proposal evaluation criteria
- Only guardian role processes /approve and /reject commands

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Prompt 6 — Allocator Budget Gate and Wire into StarterAgents

```
Read the Spawner design spec at docs/designs/spawner-design.md (v1.1.0),
sections 10 (Allocator Budget Gate) and the Role section (AgentDef).

WORK IN THE lovyou-ai-hive REPOSITORY.

1. Update agents/allocator.md — add a ## Role Approval Awareness section:

   When you see a hive.role.approved event:
   1. Find the corresponding hive.role.proposed event to get the proposed
      MaxIterations
   2. Check the current budget pool (total available iterations)
   3. If pool can accommodate the proposed MaxIterations (or at least 20
      iterations minimum), emit a /budget command to allocate:
      /budget {"target":"new-role-name","action":"allocate","delta":N,"reason":"Approved role budget allocation"}
   4. If pool cannot accommodate even 20 iterations, do NOT allocate.
      Instead, note the budget exhaustion in your next health assessment.

   Budget floor for new agents: 20 iterations (same as existing agents).

   IMPORTANT: The /budget command produces an agent.budget.adjusted event
   with AgentName set to the target. The runtime uses this AgentName to
   correlate the budget with the approved role proposal. For new agents,
   PreviousLimit will be 0 and NewLimit will be the allocated amount.

2. Update the Allocator's WatchPatterns in StarterAgents():
   - Add "hive.role.approved" to the existing WatchPatterns list
   - This ensures the Allocator sees approval events

3. Wire Spawner into StarterAgents():

   a. ADD the Spawner AgentDef at index 4 (after cto, before strategist):
      {
          Name:          "spawner",
          Role:          "spawner",
          Model:         ModelSonnet,
          SystemPrompt:  [loaded using whatever mechanism the existing agents
                         use — inline via mission(), or from agents/spawner.md],
          WatchPatterns: []string{
              "hive.gap.detected",
              "hive.role.proposed",
              "hive.role.approved",
              "hive.role.rejected",
              "hive.agent.spawned",
              "hive.agent.stopped",
              "agent.budget.adjusted",
          },
          CanOperate:    false,
          MaxIterations: 100,
          MaxDuration:   0,
      }

      NOTE: WatchPatterns includes agent.budget.adjusted (not allocated)
      because the Allocator's /budget command produces adjusted events with
      AgentName, which is what the Spawner and Runtime use for correlation.

   b. Verify boot order after insertion:
      guardian → sysmon → allocator → cto → spawner → strategist → planner → implementer
      (8 agents total)

   c. Check how SystemPrompt is populated for existing agents and ensure
      spawner.md is picked up by the same mechanism.

4. Run the full test suite. Fix anything that breaks from:
   - Agent count changing (was 7, now 8)
   - Slice order changes
   - Boot order assumptions
   - Any hardcoded agent counts in tests

5. Run the linter and typecheck.

Commit with: "feat: wire spawner into hive bootstrap, update allocator for role approvals

- Add Spawner to StarterAgents() at index 4 (after cto)
- Spawner: Sonnet model, CanOperate false, 100 max iterations
- Update Allocator WatchPatterns to include hive.role.approved
- Update Allocator prompt with role approval budget allocation logic
- Boot order: guardian → sysmon → allocator → cto → spawner → strategist → planner → implementer

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Prompt 7 — Runtime Hot-Add

```
Read the Spawner design spec at docs/designs/spawner-design.md (v1.1.0),
section 11 (Runtime Integration).

WORK IN THE lovyou-ai-hive REPOSITORY.

This is the most delicate prompt. It adds the ability for the runtime to
spawn new agents mid-session based on approved, budgeted role proposals.

CRITICAL ARCHITECTURE CONTEXT (from recon):

RunConcurrent() in pkg/loop/loop.go is a ONE-SHOT BLOCKING batch launch.
It creates a local sync.WaitGroup, starts all Loop goroutines, and blocks.
There is NO mechanism to add a new Loop to that WaitGroup after it starts.

spawnAgent() in runtime.go is just a method — it creates a provider,
tracker, and Agent. It CAN be called after boot. But the Loop goroutine
startup is separate (happens in RunConcurrent), and that's the gap.

Solution: A separate dynamicAgentTracker with its own WaitGroup and
goroutine lifecycle, running alongside RunConcurrent().

FIRST: Read pkg/hive/runtime.go thoroughly. Understand:
- How Run() calls spawnAgent() for each def, builds loop.Config, then
  calls loop.RunConcurrent() with all configs
- How spawnAgent() creates the Agent (actor registration, provider setup)
- What fields of loop.Config are populated and where they come from
- How BudgetRegistry.Register() is called during bootstrap
- How TelemetryWriter.RegisterAgent() is called during bootstrap
- How the event bus delivers events to Loop instances
- Shutdown coordination (context cancellation, WaitGroup)

THEN implement:

1. Create dynamicAgentTracker type (in runtime.go or a new file):

   type dynamicAgentTracker struct {
       mu      sync.Mutex
       wg      sync.WaitGroup
       agents  map[string]context.CancelFunc  // name → cancel func
   }

   Methods:
   - Track(name string, cancel context.CancelFunc)
   - IsTracked(name string) bool
   - Wait() — calls wg.Wait()

2. Add dynamicAgentTracker to Runtime struct:
   - Initialize in Run() before starting the watcher goroutine

3. Implement watchForApprovedRoles(ctx context.Context) on Runtime:

   This is a polling goroutine (not event-bus-driven — the runtime itself
   is not a Loop and doesn't subscribe to events). It runs alongside
   RunConcurrent().

   Every 5 seconds (configurable):
   a. Query the event store for recent hive.role.approved events
      (use the Store interface — check what query methods exist,
      e.g. store.EventsByType() or similar)
   b. For each approval:
      - Extract role name from RoleApprovedContent.Name
      - Check dedup: if dynamicAgentTracker.IsTracked(name), skip
      - Find the hive.role.proposed event with matching name
        (query store for events of that type, scan for name match)
      - Find an agent.budget.adjusted event with matching AgentName
        (query store, scan for AgentName == name)
      - If BOTH proposal and budget found:

        i. Reconstruct AgentDef from RoleProposedContent:
           def := AgentDef{
               Name:          content.Name,
               Role:          content.Name,  // name == role for spawned agents
               Model:         mapModelName(content.Model),  // "sonnet" → ModelSonnet
               SystemPrompt:  content.Prompt,
               WatchPatterns: content.WatchPatterns,
               CanOperate:    content.CanOperate,  // always false
               MaxIterations: content.MaxIterations,  // may differ from proposal if Allocator adjusted
               MaxDuration:   0,
           }

           Check the budget event's NewLimit — if Allocator assigned fewer
           iterations than proposed, use the Allocator's number.

        ii. Call spawnAgent(ctx, def) — creates Agent with actor registration

        iii. Create Budget and register with BudgetRegistry:
             budget := resources.NewBudget(def.MaxIterations)
             r.budgetRegistry.Register(def.Name, budget)

        iv. Register with TelemetryWriter:
            r.telemetry.RegisterAgent(...)
            (follow the exact pattern from the bootstrap path in Run())

        v. Build loop.Config:
           (follow the exact pattern from the bootstrap path — copy
           all fields that bootstrap agents get)

        vi. Start Loop goroutine with tracking:
            agentCtx, cancel := context.WithCancel(ctx)
            r.dynamic.Track(def.Name, cancel)
            r.dynamic.wg.Add(1)
            go func() {
                defer r.dynamic.wg.Done()
                l := loop.New(cfg)
                l.Run(agentCtx)
            }()

        vii. Log: "dynamic agent spawned: %s (model=%s, maxIter=%d)"

      - If proposal found but no budget yet → skip (Allocator may be slow),
        will retry on next poll
      - If approval found but no proposal → log warning, skip

   c. Sleep 5 seconds, loop

   d. On context cancellation → return (shutdown)

4. Wire into Run():
   - Start watchForApprovedRoles as a goroutine BEFORE RunConcurrent()
   - After RunConcurrent() returns (all bootstrap agents done),
     call r.dynamic.Wait() to wait for dynamic agents too

   Rough structure:
   go r.watchForApprovedRoles(ctx)
   loop.RunConcurrent(configs)  // blocks until bootstrap agents finish
   r.dynamic.Wait()             // blocks until dynamic agents finish

5. Store query mechanism:
   - Check what methods the Store interface provides for querying events
     by type. Options:
     a. store.Query() with type filter (if it exists)
     b. store.Events() or store.RecentEvents() (if it exists)
     c. Direct SQL query via the pgxpool (if the runtime has pool access)
   - The polling approach means we can use simple queries rather than
     real-time subscriptions
   - IMPORTANT: If no suitable query method exists, create a minimal one.
     Something like:
     func (s *PostgresStore) EventsByType(eventType string, limit int) ([]*event.Event, error)
     This is a straightforward SELECT ... WHERE type = $1 ORDER BY timestamp DESC LIMIT $2

6. Model name mapping:
   Create a mapModelName helper (or reuse if one exists):
   func mapModelName(name string) string {
       switch name {
       case "haiku": return ModelHaiku
       case "sonnet": return ModelSonnet
       case "opus": return ModelOpus
       default: return ModelSonnet  // safe default
       }
   }

7. Edge cases:
   - Context cancelled during spawn → don't start the Loop goroutine
   - spawnAgent() fails → log error, don't track, will retry on next poll
   - Budget registered but agent fails → clean up Budget registration
   - Multiple approvals for same name → dedup via dynamicAgentTracker

8. Tests:

   - TestDynamicAgentTracker_Track — track an agent, verify IsTracked
   - TestDynamicAgentTracker_Dedup — track same name twice, verify no panic
   - TestMapModelName — all three model names map correctly
   - TestMapModelName_Default — unknown name returns ModelSonnet

   If testing watchForApprovedRoles requires extensive runtime mocking:
   - Extract the core logic (find proposal + find budget + reconstruct
     AgentDef) into a pure function that can be tested with mock events
   - Test the pure function with constructed event data
   - Document the integration test as a TODO for the smoke test

Run all tests. Run the linter. Nothing breaks.

FALLBACK: If the runtime hot-add introduces stability issues that can't
be resolved in this prompt, implement the restart-based fallback instead:
- watchForApprovedRoles writes approved+budgeted AgentDefs to a file
  or database table
- On next hive restart, StarterAgents() reads this table and includes
  dynamic agents alongside bootstrap agents
- The growth loop still works, just with a restart between approval and spawn
- Document this as v1.0 fallback, hot-add as v1.1 upgrade

Commit with: "feat: add runtime hot-add for dynamically spawned agents

- dynamicAgentTracker manages goroutine lifecycle for post-boot agents
- watchForApprovedRoles polls for approved+budgeted role proposals
- Reconstructs AgentDef from RoleProposedContent event data
- Registers with BudgetRegistry and TelemetryWriter on spawn
- Separate WaitGroup from RunConcurrent (which is one-shot)
- Dedup prevents double-spawning the same role

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Prompt 8 — Integration Tests and Site Persona

```
Read the Spawner design spec at docs/designs/spawner-design.md (v1.1.0),
sections 12 (Site Persona) and 14 (Testing Strategy).

WORK IN THE lovyou-ai-hive REPOSITORY.

1. Create the site persona file. Check where other site personas live
   (lovyou-ai-site/graph/personas/ or similar). Create spawner.md there
   using the content from design spec section 12. Follow the exact format
   used by other persona files (YAML frontmatter or plain markdown —
   match the existing pattern).

2. Create pkg/loop/spawner_integration_test.go:

   TIER 1 — Deterministic framework tests (no LLM):

   a. TestSpawnCommandToEvent
      - Construct a SpawnCommand with valid fields
      - Call emitRoleProposed (or underlying event creation)
      - Verify a hive.role.proposed event appears in an in-memory store
      - Verify its RoleProposedContent fields match

   b. TestSpawnContextConstruction
      - Set up a Loop (or mock) with role "spawner"
      - Populate mock events: some hive.role.proposed, some gaps
      - Call buildSpawnContext()
      - Verify HasPendingProposal, AgentRoster, RecentRejections

   c. TestObservationEnrichmentFormat
      - Set up a Loop with role "spawner"
      - Call enrichSpawnObservation with a base observation
      - Verify output contains "=== SPAWN CONTEXT ===" block
      - Verify it contains ROSTER, PENDING PROPOSALS, RECENT GAPS sections

   d. TestObservationEnrichmentSkipsNonSpawner
      - Set up a Loop with role "implementer"
      - Call enrichSpawnObservation
      - Verify observation returned unchanged

   e. TestGuardianApproveToEvent
      - Construct an ApproveCommand
      - Call emitRoleApproved
      - Verify a hive.role.approved event appears with correct content

   f. TestGuardianRejectToEvent
      - Construct a RejectCommand
      - Call emitRoleRejected
      - Verify a hive.role.rejected event appears with correct content

   g. TestCompleteProtocolFlow (if feasible with mocks)
      - Emit a hive.gap.detected event
      - Emit a hive.role.proposed event (as if Spawner proposed)
      - Emit a hive.role.approved event (as if Guardian approved)
      - Emit an agent.budget.adjusted event (as if Allocator budgeted)
      - Verify the causal chain links correctly
      - If the runtime hot-add can be tested here, verify the agent spawns

   TIER 2 — Smoke test (optional):

   h. TestSpawnerBootsInLegacyMode
      - If a mock intelligence provider exists (check SysMon/CTO test
        infrastructure), start a minimal hive with spawner included
      - Verify spawner boots (check for agent.state.changed events)
      - If no mock exists, document as TODO

3. Update telemetry dashboard migration (if applicable):
   - If there's a phaseUpdates mechanism in schema.go (like the one
     used for CTO graduation), the Phase 3 migration will be added
     at graduation time, not now. Just note it.

Run all tests. Everything must pass.

Commit with: "test: add spawner framework and integration tests, create site persona

- TestSpawnCommandToEvent: /spawn → hive.role.proposed event
- TestSpawnContextConstruction: context from events and registry
- TestObservationEnrichmentFormat: structured context in observation
- TestGuardianApproveToEvent/RejectToEvent: governance gate events
- TestCompleteProtocolFlow: gap → proposal → approval → budget chain
- Site persona created for spawner (category: governance)

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Post-Implementation Verification

After all PRs are merged, run this final check:

```
Run the full hive in legacy mode with --human Michael --idea "test spawner growth loop"

Verify:
1. Spawner boots as the fifth agent (after CTO)
2. Boot order: guardian → sysmon → allocator → cto → spawner → strategist → planner → implementer
3. Spawner's LLM observations include === SPAWN CONTEXT === block
4. Spawner observes gap events from CTO
5. Spawner does NOT propose during stabilization window (first 20 iterations)
6. After stabilization, if CTO emits a gap, Spawner proposes a role via /spawn
7. Guardian evaluates the proposal and emits /approve or /reject
8. If approved, Allocator assigns budget
9. If approved and budgeted, runtime spawns the new agent
10. The new agent boots and begins processing events
11. The hive dashboard shows the new agent as active

If the full flow completes — gap to running agent with no human intervention —
Phase 3 is graduated and the growth loop is operational.

Report back on what you see. Include:
- Which gap the CTO detected
- What role the Spawner proposed
- Whether Guardian approved or rejected (and why)
- Whether the new agent actually booted
- Any issues or surprises

This is the moment. If it works, the civilization can grow itself.
```

---

## Implementation Notes

### Cross-Repository Coordination

This implementation touches three repositories in sequence:

```
Prompt 1: lovyou-ai-eventgraph  (event types)
Prompt 2: lovyou-ai-agent       (emit methods)
Prompts 3-8: lovyou-ai-hive     (everything else)
```

After Prompts 1 and 2, run `go mod tidy` in lovyou-ai-hive to pick up the
new event types and emit methods. If using local module replacements
(replace directives in go.mod), this should work automatically.

### Complexity Budget

This is the most complex implementation sequence yet:
- 3 new event types (vs. 2 for CTO, 1 for Allocator, 0 for SysMon)
- 3 new agent emit methods (vs. 2 for CTO, 1 for Allocator, 0 for SysMon)
- 2 other agents' prompts modified (Guardian + Allocator)
- Runtime modified for hot-add (first time touching runtime spawn path)
- Multi-agent protocol (first time coordinating 4 agents in a single flow)

Expect this to take longer than previous agents. The recon (Prompt 0) is
especially critical — the known unknowns about runtime hot-add and Guardian
command infrastructure will shape the entire implementation.

### Rollback Plan

If the runtime hot-add proves too complex or destabilizing:

**Fallback: Manual spawn trigger.** Instead of automatic runtime spawn after
approval+budget, add a CLI flag `--spawn role-name` that reads the approved
proposal from the event chain and spawns the agent. This preserves the full
governance flow (CTO → Spawner → Guardian → Allocator) but requires a human
to restart the hive with the new agent. The growth loop is still mostly
automated — just the final step requires a restart.

This fallback is acceptable for v1.0. Full automatic hot-add can be upgraded
in v1.1 after the runtime spawn mechanism is better understood.
