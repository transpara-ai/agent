# AGENTS.md

## Purpose
Universal Go Agent abstraction for the hive. Preserve identity, lifecycle, trust, observability, and event causality invariants.

## Commands
- Build: `make build`
- Test: `make test`
- Vet: `make vet`
- Verify: `make verify`

## Rules
- Every externally meaningful state change must remain observable as a signed event.
- Preserve immutable ID semantics; names are for humans, IDs are for systems.
- Do not bypass lifecycle guards for retired or suspended agents.
- Keep lock ordering and causality tracking intact.
- Do not push to `upstream`; `origin` is the writable fork.

## Exit Criteria
- `make verify` passes, or the blocker is explicit.
- Tests cover lifecycle, causality, and error paths touched by the change.
- Public behavior changes are reflected in docs or repo guidance.
