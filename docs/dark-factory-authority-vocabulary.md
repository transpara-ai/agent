# Dark Factory Authority Vocabulary

Date: 2026-05-08

Source of truth: `transpara-ai/docs` `dark-factory/DF-SOP-0001-authority-gated-side-effects.md`.

Agent does not currently execute repository, deployment, policy, or secret side effects directly. If Agent code adds such a path, it must use the shared vocabulary below and emit or preserve an audit-readable authority request before execution.

## Authority Outcomes

```text
Autonomous
Notify
ApprovalRequired
Forbidden
```

## Protected Actions

```text
production.deploy
repo.create
repo.delete
repo.push.default_branch
repo.merge.main
repo.mutate.cross_repo
agent.spawn.persistent
agent.retire
agent.escalate_permissions
policy.change
secret.access
external_communication.company_voice
data.delete
self_modification.activate
billing.spend_above_threshold
license.change
```

## Local Alignment Notes

- Agent lifecycle and identity events are not replacements for protected action names.
- Persistent spawning must use `agent.spawn.persistent`.
- Retirement must use `agent.retire`.
- Permission escalation must use `agent.escalate_permissions`.
- Self-modification activation must use `self_modification.activate`; do not invent `self.modify`, `self_modification`, or similar aliases.
