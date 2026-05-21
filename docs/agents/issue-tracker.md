# Issue tracker

Issues for this repository live in **GitHub Issues** on `jzefan/arkloop`.

## How agents interact with the tracker

Use the `gh` CLI for all issue operations.

### Create
- `gh issue create --title "<title>" --body "<body>" [--label <label>] [--assignee @me]`
- Pass the body via a HEREDOC for multi-line markdown.
- When creating from a plan/PRD, include "## Summary" and "## Acceptance criteria" sections.

### Read / search
- `gh issue list --state open --label <label>` to filter by triage state.
- `gh issue view <number>` to read a single issue.
- `gh search issues "in:title <term> repo:jzefan/arkloop"` for cross-repo queries.

### Update
- `gh issue edit <number> --add-label <label> --remove-label <label>`
- `gh issue comment <number> --body "<text>"`
- `gh issue close <number>` / `gh issue reopen <number>`

## Conventions

- One issue per independently-shippable vertical slice (see `to-issues` skill).
- Use the triage labels in `docs/agents/triage-labels.md` to move issues through the state machine.
- Link related issues with `#<number>` in bodies/comments — GitHub auto-renders the cross-reference.
- For PRDs / large specs, attach via comment or link to a `docs/` markdown file in the repo rather than pasting into the issue body.
