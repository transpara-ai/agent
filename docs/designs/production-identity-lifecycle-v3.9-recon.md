# Production Identity Lifecycle v3.9 Recon

Issue: transpara-ai/agent#16, "Track production agent identity persistence and lifecycle model"

Program tracker: transpara-ai/docs#44

Status: design-only recon. Do not implement production identity persistence until EventGraph Stage 1 / Tier 0 identity, authority, lifecycle, trust, and execution receipt records are available.

## Sources Reviewed

- transpara-ai/agent#16, including the May 13 disposition comment.
- transpara-ai/agent PR #15 patch and metadata.
- Local agent hardening history: `a78c7f8 feat(agent): harden production identity signing (#17)`.
- `agent.go`, `transitions.go`, `retire.go`, `operations.go`, `trust.go`, `identity_test.go`, `testhelper_test.go`.
- `../eventgraph/go/pkg/agent/compositions.go`, `../eventgraph/go/pkg/event/agent_content.go`, `../eventgraph/go/pkg/actor/memory.go`, `../eventgraph/go/pkg/actor/pgactor/pgactor.go`.
- `../docs/dark-factory/v3.9/02-kernel-schema-and-state-v3.9.md`.
- `../docs/dark-factory/v3.9/03-authority-identity-and-sops-v3.9.md`.
- `../docs/dark-factory/v3.9/08-implementation-workflow-checklist-v3.9.md`.

## 1. Current Identity Behavior

`agent.New` currently creates an Ed25519 signer, registers the public key in EventGraph's actor store, creates an `intelligence.AgentRuntime`, and emits the boot sequence through `egagent.BootEvents`.

Production is the zero-value environment. The zero-value identity mode is `IdentityModeGenerated`, which generates a new Ed25519 private key with `crypto/rand` when `Config.SigningKey` is unset.

`Config.SigningKey` can supply generated or externally managed private key material. In production, `agent.New` rejects supplied key material when its public key matches the known public-name-derived fixture key for `sha256("agent:" + Name)`.

The emitted boot sequence currently includes `agent.identity.created`, soul/model/authority boot events, and `agent.state.changed` to Idle. Current EventGraph content records the agent ID, public key, and agent type for `agent.identity.created`; it is not the v3.9 `ActorIdentity` Tier 0 record.

Actor identity is effectively the EventGraph actor store registration derived from the public key. The actor ID is stable only when the same public key is supplied again and the actor store still has or can reconstruct that registration.

## 2. Current Production/Dev/Test Identity Modes

Production:

- `Config.Environment == ""` or `IdentityEnvironmentProduction`.
- `Config.IdentityMode == ""` or `IdentityModeGenerated`.
- Generates a fresh Ed25519 key by default.
- Allows supplied `Config.SigningKey` if it is the correct size and does not match the known deterministic public-name fixture derivation.
- Rejects `IdentityModeDeterministic`.

Development:

- `Config.Environment == IdentityEnvironmentDevelopment`.
- Allows `IdentityModeDeterministic` explicitly.
- Deterministic identity derives from `sha256("agent:" + Name)`.

Test:

- `Config.Environment == IdentityEnvironmentTest`.
- Allows `IdentityModeDeterministic` explicitly.
- Existing test helpers use deterministic identity to keep fixture actor IDs stable.

The guardrail from PR #15/#17 is correct and must remain: deterministic public-label-derived identity is dev/test-only.

## 3. Where Deterministic Identity Is Blocked Today

`signingKey(cfg)` blocks deterministic identity in two production paths:

- `IdentityModeDeterministic` returns `agent: deterministic identity is blocked in production`.
- `IdentityModeGenerated` with supplied `Config.SigningKey` rejects keys whose public key equals the known deterministic fixture derivation for the configured public name.

The block is covered by `TestProductionRejectsDeterministicIdentity`, `TestProductionRejectsSuppliedPublicNameDerivedSigningKey`, `TestProductionGeneratedIdentityDoesNotUsePublicNameSeed`, and development/test fixture allowance tests.

The block is local code only. There is not yet a canonical EventGraph `ActorIdentity` record with `identity_mode`, lifecycle status, authority linkage, or revocation/rotation history.

## 4. Current Restart Behavior

A process restart with no supplied signing key creates a new Ed25519 key and therefore a new public-key-derived actor identity. That is safe from public-name derivation but not persistent production identity.

A process restart with the same supplied signing key can regain the same actor ID if the actor store recognizes the public key or can register it consistently. This is key continuity, not a complete production lifecycle model.

Current restart behavior does not:

- load production key material from a governed key reference;
- verify an `ActorIdentity` status before constructing the runtime;
- distinguish intentional new identity creation from accidental restart drift;
- emit v3.9 `LifecycleTransition` records for restart/resume;
- check revocation, retirement, or suspension in canonical Tier 0 records;
- link persistent startup to `AuthorityRequest`, `AuthorityDecision`, and `ExecutionReceipt`.

## 5. Missing Persistence Model

The missing model is not just key storage. Agent needs an identity resolver and lifecycle helper that operates against EventGraph Tier 0 records after those schemas land.

Required missing pieces:

- a production identity reference in config, likely a stable actor identity id or public key reference, not a private key literal;
- an interface boundary for externally managed signing material that never writes private keys to EventGraph, logs, docs, PRs, audit reports, MemPalace, or LLM Wiki;
- lookup of the canonical `ActorIdentity` by configured reference;
- status validation before runtime construction: no revoked, retired, memorial, closed, or unauthorized trial-to-active promotion;
- authority gating for persistent creation, rotation, retirement, and revocation;
- idempotent EventGraph writes for identity registration, lifecycle transitions, trust changes, and protected lifecycle receipts;
- explicit semantics for intentional new production identity versus restart continuity;
- tests that prove production restart identity continuity and failure on accidental identity drift.

## 6. Required EventGraph Records

Agent should wait for EventGraph Stage 1 to provide the canonical record contracts. Do not define replacement schemas in this repo.

`ActorIdentity`:

- Canonical production identity record for human, agent, service, and runtime actors.
- Must carry actor id, actor type, public key reference, identity mode, and lifecycle status per v3.9.
- Agent should consume it to validate whether production startup is creation, restart, trial, active, suspended, revoked, retired, or memorial.

`LifecycleTransition`:

- Canonical lifecycle edge for actor state movement.
- Agent should emit helper-level transitions only through the EventGraph contract once available.
- Must preserve v3.9 allowed transitions such as proposed to trial, trial to active, active to suspended, active to retiring, retiring to retired, active to revoked, and retired to memorial.

`TrustRecord`:

- Canonical trust-change record for actor trust.
- Current `trust.updated` event is not enough for v3.9 production identity lifecycle.
- `RecordVerifiedWork` should eventually map trust updates to `TrustRecord` without breaking lock ordering or causality.

`AuthorityRequest`:

- Required before persistent agent creation and protected lifecycle/key actions.
- Relevant protected actions include `agent.spawn.persistent`, `agent.retire`, `agent.revoke`, `agent.key.rotate`, `agent.escalate_permissions`, and `secret.access`.

`AuthorityDecision`:

- Canonical approval/denial record for protected identity and lifecycle actions.
- Agent must not treat proposal artifacts, advisory policy output, or local config as authority approval.

`ExecutionReceipt`:

- Required evidence that an approved protected action was attempted and whether it succeeded, failed, was blocked, or was skipped.
- Key rotation, revocation, persistent spawn, and protected retirement should write receipts after authority decision validation.

## 7. Proposed Implementation Plan After EventGraph Stage 1 Lands

1. Add a production identity resolver abstraction in Agent that depends on EventGraph Tier 0 APIs and an external signing provider interface. Keep private key handling outside EventGraph and outside logs.

2. Extend `Config` with production-safe identity references, not schema copies. Candidate inputs should reference canonical EventGraph identity/public key records and externally managed key handles. Preserve current `SigningKey` support for tests and controlled embedding, but avoid making private key literals the production path.

3. Split startup into explicit modes:

- fixture startup: deterministic, dev/test only;
- ephemeral production startup: generated key, no persistence guarantee, allowed only where v3.9 policy permits;
- persistent production startup: resolve existing `ActorIdentity`, validate status and authority, bind signer, register actor store view, emit lifecycle/receipt records idempotently;
- persistent production creation: require `agent.spawn.persistent` approval before active identity creation.

4. Add status guards before runtime construction. Revoked, retired, memorial, and closed identities must fail closed. Suspended identities must not resume unless v3.9 policy and authority allow the transition.

5. Map current boot identity events to Tier 0 records. Continue preserving existing event causality with `lastEvent`, but make `ActorIdentity` and `LifecycleTransition` the source of production lifecycle truth.

6. Implement key rotation only after EventGraph exposes the record and authority path. Rotation should require `agent.key.rotate`, write new public key reference metadata, preserve old-key auditability, reject deterministic fixture keys in production, and produce `ExecutionReceipt`.

7. Implement revocation and protected retirement against `agent.revoke` and `agent.retire`. Revocation must block future emissions and startup. Retirement must preserve the current observable retirement ceremony while also writing canonical lifecycle records.

8. Update `RecordVerifiedWork` integration to write or emit `TrustRecord` when trust changes, while preserving the current lock ordering: `a.mu` before trust model lock before graph lock.

9. Add migration/backward compatibility behavior for existing `agent.identity.created` events. They can remain compatibility events, but production v3.9 behavior must rely on `ActorIdentity` and `LifecycleTransition`.

10. Run full verification and keep `make verify` as the exit gate.

## 8. Required Tests

Production persistence:

- production persistent startup with the same identity reference returns the same actor ID across restart;
- production startup without required persistent identity reference fails when policy requires persistence;
- accidental generated-key drift is detectable and does not masquerade as restart continuity.

Guardrails:

- deterministic identity remains blocked in production;
- supplied public-name-derived key remains blocked in production;
- fixture identity remains available only for explicit development/test modes.

Authority:

- persistent identity creation requires `agent.spawn.persistent`;
- trial to active requires approval and cannot be inferred from a proposal artifact;
- expired approval fails;
- key rotation requires `agent.key.rotate`;
- retirement and revocation require their protected action decisions where v3.9 policy requires them;
- authority request, decision, and execution receipt link correctly.

Lifecycle:

- revoked, retired, memorial, and closed identities cannot emit or restart as active;
- suspended identities cannot bypass resume authority;
- lifecycle transition writes are idempotent;
- invalid v3.9 lifecycle transitions fail closed;
- current retired/suspended emission guards still hold.

Trust and causality:

- trust changes write `TrustRecord` and keep previous/current values consistent;
- lifecycle, identity, trust, and receipt writes preserve causal ordering through `lastEvent`;
- failed record writes roll back in-memory lifecycle state where applicable.

Rotation and revocation:

- rotation preserves actor identity semantics defined by EventGraph and records old/new public key refs without exposing private keys;
- revoked keys cannot sign accepted future production events;
- rotation/revocation produce execution receipts with evidence refs.

## 9. Risks

- Implementing before EventGraph Tier 0 lands would force Agent to invent schema or lifecycle semantics and risk conflict with v3.9.
- Treating generated production keys as persistent would create accidental new identities after restart.
- Storing or logging private key material would violate the Production Agent Identity SOP.
- Allowing local config or proposal artifacts to substitute for authority records would weaken protected-action governance.
- Adding persistence without revocation checks would let revoked agents restart and emit.
- Changing current lock ordering could reintroduce trust or causality races.
- Mapping old `agent.identity.created` events directly to v3.9 truth could obscure the difference between compatibility events and canonical `ActorIdentity`.

## 10. Explicit Non-Goals

- Do not implement key storage in this recon.
- Do not invent EventGraph schema or local replacement records in Agent.
- Do not add runtime execution behavior.
- Do not weaken the deterministic identity dev/test-only rule.
- Do not make Agent the owner of direct execution authority.
- Do not implement RuntimeBroker, GovernancePolicyEngine, Site approval inbox, or Work DAG behavior here.
- Do not write private keys to EventGraph, logs, docs, PRs, audit reports, MemPalace, or LLM Wiki.
- Do not treat PR #15/#17 hardening as complete production identity persistence.
