# CTO Implementation — Claude Code Task Prompts

**Version:** 1.1.0
**Date:** 2026-04-04
**Status:** Active — Prompt 0 COMPLETE
**Versioning:** Independent of all other documents. Major version increments reflect fundamental restructuring of the implementation approach; minor versions reflect adjustments from reconnaissance or implementation feedback; patch versions reflect corrections and clarifications.
**Author:** Claude Opus 4.6
**Owner:** Michael Saucier

---

### Revision History

| Version | Date | Description |
|---------|------|-------------|
| 1.0.0 | 2026-04-04 | Initial prompt sequence: 7 prompts (recon + 6 implementation). Leverages established patterns from SysMon and Allocator — minimal recon needed. |
| 1.1.0 | 2026-04-04 | Post-recon (Prompt 0): marked COMPLETE with findings. Legacy agents/cto.md (117 lines, git hygiene focus, references "CEO/Matt") confirmed — will be replaced entirely. Site persona exists with same legacy content — also replaced. Corrected EmitGapDetected/EmitDirective to use `.Value()` wrapping per confirmed pattern. Added explicit REPLACE notes to Prompt 3. No structural changes to prompt sequence. |

---

## Usage

Feed these to Claude Code **ONE AT A TIME**, in order. Wait for each to
complete and verify before moving to the next. Do not skip ahead. Do not combine.

## Prerequisites

- Design spec `docs/designs/cto-design.md` (v1.1.0) is committed to `lovyou-ai-hive`
- SysMon, Allocator are graduated and running
- You have both repos: `lovyou-ai-hive` and `lovyou-ai-eventgraph`

---

## Prompt 0 — Targeted Reconnaissance (COMPLETE)

Key findings:

- `agents/cto.md`: EXISTS — 117-line legacy operational tech-lead prompt.
  Focused on git hygiene, uncommitted work alerts (WARNING >4h/>100 lines,
  CRITICAL >8h/>500 lines), dependency review, code quality. References
  "CEO/Matt". This is NOT the CTO we're building. Will be REPLACED entirely.
- Site persona: EXISTS — same 117-line content. Will be REPLACED.
- StarterAgents: 6 agents — guardian (Sonnet, 200), sysmon (Haiku, 150),
  allocator (Haiku, 150), strategist (Opus), planner (Opus), implementer
  (Opus, CanOperate=true, 100, 60min). CTO slot: index 3 (after allocator).
- `hive.gap.detected`: MISSING — must create in eventgraph.
- `hive.directive.issued`: MISSING — must create in eventgraph.
- Other `hive.*` registered: run.started, run.completed, agent.spawned,
  agent.stopped, progress. No collisions.
- BudgetRegistry access: `l.config.BudgetRegistry` → `.Snapshot()`,
  `.TotalPool()`, `.TotalUsed()`. Confirmed.
- EmitBudgetAdjusted pattern: `checkCanEmit()` →
  `recordAndTrack(EventType.Value(), content)` with error wrapping.
  Note `.Value()` wrapping — design spec v1.0.0 showed bare EventType.
  Corrected in v1.1.0 and in Prompt 1 below.
- TaskCreatedContent: Title, Description, CreatedBy, Priority, Workspace.
- TaskCompletedContent: TaskID, CompletedBy, Summary. Summary gives
  quality signal for gap detection.
- Human operator: Michael Saucier. "CEO/Matt" references are stale.

---

## Prompt 0.5 — Event Types in EventGraph (PR 0.5 COMPLETE)

Create the two new event types in eventgraph. This is infrastructure that must
exist before the hive code can emit gap and directive events.

```
Read the CTO design spec at docs/designs/cto-design.md (v1.1.0), section 8
(Event Types). Then look at how agent.budget.adjusted was created (the Allocator
added it). Follow that exact pattern.

In lovyou-ai-eventgraph:

1. Add event type constants:
   EventTypeGapDetected     = types.MustEventType("hive.gap.detected")
   EventTypeDirectiveIssued = types.MustEventType("hive.directive.issued")

   Place these where other hive.* type constants are defined.

2. Add content structs:

   type GapDetectedContent struct {
       Category    string `json:"category"`
       MissingRole string `json:"missing_role"`
       Evidence    string `json:"evidence"`
       Severity    string `json:"severity"`
   }

   type DirectiveIssuedContent struct {
       Target   string `json:"target"`
       Action   string `json:"action"`
       Reason   string `json:"reason"`
       Priority string `json:"priority"`
   }

   Each needs: EventTypeName() string method, Accept(EventContentVisitor)
   method (follow the exact pattern of AgentBudgetAdjustedContent or
   HealthReportContent).

3. Register unmarshalers in content_unmarshal.go.

4. Add to DefaultRegistry().

5. If there's a constructor pattern (like NewHealthReportContent), add
   constructors for both types.

Run all eventgraph tests. Run vet. Nothing breaks.

Commit to lovyou-ai-eventgraph with:
"feat: add hive.gap.detected and hive.directive.issued event types

- GapDetectedContent: category, missing_role, evidence, severity
- DirectiveIssuedContent: target, action, reason, priority
- Full unmarshaler registration and DefaultRegistry entries
- Enables CTO agent gap detection and directive issuance (Phase 2)

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Prompt 1 — Agent Emit Methods (PR 1)

```
In lovyou-ai-agent, add two new emit methods following the EXACT pattern of
EmitBudgetAdjusted (in budget.go:26-37):

    func (a *Agent) EmitBudgetAdjusted(content event.AgentBudgetAdjustedContent) error {
        if err := a.checkCanEmit(); err != nil {
            return fmt.Errorf("budget adjusted: %w", err)
        }
        _, err := a.recordAndTrack(event.EventTypeAgentBudgetAdjusted.Value(), content)
        if err != nil {
            return fmt.Errorf("budget adjusted: %w", err)
        }
        return nil
    }

Create cto.go (new file):

func (a *Agent) EmitGapDetected(content event.GapDetectedContent) error {
    if err := a.checkCanEmit(); err != nil {
        return fmt.Errorf("gap detected: %w", err)
    }
    _, err := a.recordAndTrack(event.EventTypeGapDetected.Value(), content)
    if err != nil {
        return fmt.Errorf("gap detected: %w", err)
    }
    return nil
}

func (a *Agent) EmitDirective(content event.DirectiveIssuedContent) error {
    if err := a.checkCanEmit(); err != nil {
        return fmt.Errorf("directive: %w", err)
    }
    _, err := a.recordAndTrack(event.EventTypeDirectiveIssued.Value(), content)
    if err != nil {
        return fmt.Errorf("directive: %w", err)
    }
    return nil
}

IMPORTANT: Note the .Value() call on the EventType constant. This is the
established pattern from EmitBudgetAdjusted. Do NOT pass the bare
event.EventTypeGapDetected — it must be event.EventTypeGapDetected.Value().

Verify the import path for the new event types (they were created in Prompt 0.5).
Run tests. Run vet.

Commit to lovyou-ai-agent with:
"feat: add EmitGapDetected and EmitDirective methods for CTO agent

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Prompt 2 — CTO Glue Code (PR 2)

This is the main implementation PR in lovyou-ai-hive.

```
Read the CTO design spec at docs/designs/cto-design.md (v1.1.0), sections 7
(/gap and /directive Command Mechanisms), 9 (Observation Enrichment), and 11
(Configuration).

Read pkg/loop/health.go and pkg/loop/budget.go to see the established patterns.

Create pkg/loop/cto.go:

1. GapCommand struct and DirectiveCommand struct (from design spec section 7)

2. parseGapCommand(response string) *GapCommand
   - Scan for line starting with "/gap "
   - Extract JSON after prefix
   - Return nil if not found or malformed
   - Follow parseHealthCommand() pattern exactly

3. parseDirectiveCommand(response string) *DirectiveCommand
   - Same pattern for "/directive " prefix

4. CTO configuration:
   type CTOConfig struct {
       StabilizationWindow int
       GapCooldown         int
       DirectiveCooldown   int
       ValidCategories     []string
   }
   func DefaultCTOConfig() CTOConfig — from design spec section 11
   func LoadCTOConfig() CTOConfig — from CTO_* env vars with defaults

5. Validation with cooldown tracking:
   type CTOCooldowns struct {
       gapByCategory    map[string]int  // category → last emission iteration
       directiveByTarget map[string]int // target → last emission iteration
       emittedGaps      map[string]bool // missing_role → already emitted
   }
   func NewCTOCooldowns() *CTOCooldowns

   validateGapCommand(cmd *GapCommand, iteration int, cooldowns *CTOCooldowns, cfg CTOConfig) error
   - Check stabilization window (first 15 iterations blocked)
   - Check category cooldown (15 iterations between same category)
   - Check dedup (missing_role already emitted)
   - Check valid category (quality, operations, security, knowledge, governance)

   validateDirectiveCommand(cmd *DirectiveCommand, iteration int, cooldowns *CTOCooldowns, cfg CTOConfig) error
   - Check stabilization window (first 15 iterations blocked)
   - Check target cooldown (5 iterations between same target)

6. Emission:
   emitGap(cmd *GapCommand) error — on Loop
   - Construct event.GapDetectedContent
   - Call l.agent.EmitGapDetected(content)

   emitDirective(cmd *DirectiveCommand) error — on Loop
   - Construct event.DirectiveIssuedContent
   - Call l.agent.EmitDirective(content)

7. enrichCTOObservation(obs string) string — on Loop
   - Only activates for role == "cto"
   - Builds === LEADERSHIP BRIEFING === block from:
     a. Task flow: count pending events by work.task.* subtypes
     b. Health: find most recent health.report in pending events, extract severity
     c. Budget: l.config.BudgetRegistry.Snapshot() for agent states and
        iteration usage. Also l.config.BudgetRegistry.TotalPool() and
        .TotalUsed() for pool-level summary.
     d. Previous gaps: count hive.gap.detected in pending events
     e. Previous directives: count hive.directive.issued in pending events
   - Format matches design spec section 6 (=== LEADERSHIP BRIEFING === block)

8. Wire into Loop:

   a. Add CTOCooldowns field to Loop struct (initialized in New() if role == "cto")
   b. Add CTOConfig field to Loop struct (loaded via LoadCTOConfig() if role == "cto")
   c. In observe() path: call enrichCTOObservation()
   d. After health command processing and budget command processing,
      add gap and directive processing:
      if cmd := parseGapCommand(response); cmd != nil {
          if err := l.validateAndEmitGap(cmd, iteration); err != nil {
              fmt.Printf("warning: /gap rejected: %v\n", err)
          }
      }
      if cmd := parseDirectiveCommand(response); cmd != nil {
          if err := l.validateAndEmitDirective(cmd, iteration); err != nil {
              fmt.Printf("warning: /directive rejected: %v\n", err)
          }
      }

Run all tests including existing loop tests. Run vet. Nothing breaks.

Commit with:
"feat: add /gap and /directive command parsing, validation, and emission for CTO

- parseGapCommand/parseDirectiveCommand extract CTO commands from LLM output
- Stabilization window (15 iter), gap cooldown (15 iter), directive cooldown (5 iter)
- Gap dedup prevents same missing_role from being emitted twice
- enrichCTOObservation provides leadership briefing with task/health/budget data
- Follows established /health and /budget command patterns

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Prompt 3 — Agent Prompt and Wire into StarterAgents (PR 3)

```
Read the CTO design spec at docs/designs/cto-design.md (v1.1.0), section 6
(Prompt File) for the prompt content. Note section 6 explicitly states the
legacy agents/cto.md (117-line operational tech-lead prompt with git hygiene
alerts and "CEO/Matt" references) is REPLACED entirely.

1. REPLACE agents/cto.md with the v1.1.0 content from the design spec.
   This is NOT an update — the existing file defines a different agent
   (code quality babysitter). Replace the entire file content.
   Match the ## section format of guardian.md, sysmon.md, allocator.md.

2. In pkg/hive/agentdef.go, add CTO to StarterAgents() at index 3
   (after allocator, before strategist):

   {
       Name:  "cto",
       Role:  "cto",
       Model: ModelOpus,
       SystemPrompt: mission(`== ROLE: CTO ==
   You are the CTO — the civilization's technical leader.

   You make architecture decisions, identify structural gaps in the role
   taxonomy, and issue directives to guide work agents.

   Each iteration you receive a leadership briefing with task flow, health,
   budget, and gap data. Assess patterns. Look for:
   - Tasks that stall or fail repeatedly
   - Failure categories no current agent handles
   - Work patterns that indicate missing roles

   When you identify a genuine structural gap, emit:
   /gap {"category":"<cat>","missing_role":"<n>","evidence":"<what>","severity":"low|medium|high|critical"}

   Categories: quality, operations, security, knowledge, governance

   When work agents need course correction, emit:
   /directive {"target":"<agent-or-all>","action":"<what>","reason":"<why>","priority":"low|medium|high"}

   First 15 iterations are observe-only. Build your mental model.
   Minimum 15 iterations between /gap in same category.
   Minimum 5 iterations between /directive to same target.

   You NEVER write code, manage budgets, or halt agents.
   You think about structure, not individual tasks.
   Ground every decision in observable events, not speculation.

   Escalate existential concerns to Michael via /signal ESCALATE.
   `),
       WatchPatterns: []string{
           "work.task.*",
           "hive.*",
           "health.report",
           "agent.budget.adjusted",
           "agent.state.*",
           "agent.escalated",
       },
       CanOperate:    false,
       MaxIterations: 50,
   },

3. Update tests: agent count (6 → 7), expected roles list includes "cto",
   boot order verification (guardian, sysmon, allocator, cto, strategist,
   planner, implementer).

4. The CTO site persona already exists in lovyou-ai-site/graph/personas/cto.md
   but with legacy content (same 117-line tech-lead prompt). REPLACE its
   content with the governance-focused persona from design spec section 10.
   Ensure the YAML frontmatter has:
     category: governance
     model: opus
     active: true

Run all tests. Run vet.

Commit with:
"feat: wire CTO into hive bootstrap as fourth agent after allocator

- Replace legacy agents/cto.md (git hygiene focus) with gap-detection CTO
- Boot order: guardian → sysmon → allocator → cto → strategist → planner → implementer
- CTO: Opus model, CanOperate false, 50 max iterations
- Watches: work.task.*, hive.*, health.report, agent.budget.adjusted, agent.state.*, agent.escalated
- Replace legacy site persona with governance-category CTO

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Prompt 4 — Guardian and Work Agent Updates (PR 4)

```
Read the CTO design spec section 12 (Integration Points).

1. Edit agents/guardian.md — add ## CTO Awareness section:
   - hive.gap.detected events are architectural observations, not violations
   - hive.directive.issued events are guidance, not commands
   - Guardian should NOT treat gap events as integrity issues
   - Guardian should flag ONLY if a directive appears to violate soul or invariants
   - Place logically near existing agent-awareness sections (e.g., ## SysMon
     Awareness, ## Allocator Awareness)

2. If agents/strategist.md or inline strategist prompt exists:
   - Add awareness that hive.directive.issued events from CTO are strategic
     guidance to consider when creating tasks
   - Not commands to blindly follow — Strategist applies its own judgment

Run vet.

Commit with:
"feat: add CTO awareness to guardian and work agent prompts

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Prompt 5 — Tests (PR 5)

```
Read the CTO design spec section 13 (Testing Strategy).

Create pkg/loop/cto_test.go:

Unit tests:
- TestParseGapCommand_Valid — well-formed /gap line → correct GapCommand struct
- TestParseGapCommand_NoCommand — response without /gap → nil
- TestParseGapCommand_MalformedJSON — /gap with bad JSON → nil
- TestParseGapCommand_MultipleLines — /gap buried in other LLM output → found
- TestParseDirectiveCommand_Valid — well-formed /directive → correct struct
- TestParseDirectiveCommand_NoCommand — no /directive → nil
- TestValidateGapCommand_StabilizationBlocks — iteration < 15 → error
- TestValidateGapCommand_CooldownBlocks — same category within 15 iter → error
- TestValidateGapCommand_DedupBlocks — same missing_role → error
- TestValidateGapCommand_InvalidCategory — unknown category → error
- TestValidateGapCommand_Valid — after stabilization, valid category → nil error
- TestValidateDirectiveCommand_StabilizationBlocks — iteration < 15 → error
- TestValidateDirectiveCommand_CooldownBlocks — same target within 5 iter → error
- TestValidateDirectiveCommand_Valid — after stabilization, new target → nil error

Create pkg/loop/cto_integration_test.go:

Tier 1 (deterministic, no LLM):
- TestGapCommandToEvent — construct GapCommand, call emitGap, verify
  hive.gap.detected event in in-memory store with correct GapDetectedContent
- TestDirectiveCommandToEvent — same for /directive → hive.directive.issued
- TestCTOObservationEnrichmentFormat — set up Loop with role "cto", call
  enrichCTOObservation, verify output contains "=== LEADERSHIP BRIEFING ===" block
  with TASK FLOW, HEALTH, BUDGET, GAPS, DIRECTIVES sections
- TestCTOObservationEnrichmentSkipsNonCTO — role "implementer" → observation
  returned unchanged
- TestGapCommandInLoop — mock provider returns /gap response, verify event in
  store after loop iteration (follow pattern from health_integration_test.go
  and budget_integration_test.go if they exist)

Follow the test infrastructure from existing pkg/loop/ test files.

Run all tests. Everything must pass.

Commit with:
"test: add CTO gap detection and directive framework tests

- Unit: parse, validate (stabilization, cooldown, dedup, categories)
- Integration: command-to-event, enrichment format, loop-level gap emission

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Post-Implementation Verification

```
Rebuild the hive binary and restart the service:

cd ~/transpara-ai/repos/lovyou-ai-hive
go build -o /home/transpara/bin/hive ./cmd/hive
systemctl --user restart lovyou-hive.service
sleep 10
systemctl --user status lovyou-hive.service
journalctl --user -u lovyou-hive.service --no-pager -n 40

Verify:
1. CTO boots as the fourth agent (after guardian, sysmon, allocator)
2. CTO uses Opus model
3. CTO's observation includes === LEADERSHIP BRIEFING === block
4. CTO does NOT emit /gap or /directive in first 15 iterations (stabilization)
5. After stabilization, CTO assesses and may emit gap/directive events
6. Guardian sees CTO events without treating them as violations
7. Telemetry dashboard shows CTO as an active agent
8. Total agent count is now 7
9. No references to "CEO/Matt" anywhere in agent prompts or personas

Report back. If everything checks out, CTO is graduated.
Phase 2 is complete. Spawner is unblocked.
```
