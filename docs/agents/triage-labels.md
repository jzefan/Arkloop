# Triage labels

The `triage` skill moves issues through a 5-state machine. Each state maps to a GitHub label string below. **These strings must exist as labels in `jzefan/arkloop`** — create them once with `gh label create` if missing.

| Role | Label string | Meaning |
|------|--------------|---------|
| Needs evaluation | `needs-triage` | A maintainer must read and classify this issue. |
| Waiting on reporter | `needs-info` | Reproduction or details missing; reporter must respond. |
| AFK-ready | `ready-for-agent` | Fully specified; an autonomous agent can pick up with no human context. |
| Human-ready | `ready-for-human` | Spec is clear but requires human judgement/implementation. |
| Won't fix | `wontfix` | Declined; will not be actioned. |

## Bootstrap (run once per repo)

```bash
gh label create needs-triage     --color FBCA04 --description "Maintainer needs to evaluate"
gh label create needs-info       --color D4C5F9 --description "Waiting on reporter for more information"
gh label create ready-for-agent  --color 0E8A16 --description "Fully specified; AFK-ready for autonomous agent"
gh label create ready-for-human  --color 1D76DB --description "Ready for human implementation"
gh label create wontfix          --color FFFFFF --description "Will not be actioned"
```

## Transitions

- New issue → apply `needs-triage` on open.
- Triage decision: → `needs-info` (ask reporter), `ready-for-agent`, `ready-for-human`, or close with `wontfix`.
- Reporter responds to `needs-info` → re-apply `needs-triage` for re-evaluation.
- When work starts: remove the ready label; close the issue when the PR merges.

Only one of {needs-triage, needs-info, ready-for-agent, ready-for-human} should be applied at a time.
