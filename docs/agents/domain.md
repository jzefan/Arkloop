# Domain documentation

This repository uses a **single-context** layout:

- `CONTEXT.md` at the repo root — the canonical glossary of domain language (Account, Workspace, Profile, Skill, Persona, Thread, Run, etc.) and the high-level architecture.
- `docs/adr/` at the repo root — Architecture Decision Records, numbered sequentially (`0001-*.md`, `0002-*.md`, …) using the standard "Context / Decision / Consequences" format.

## How agents should use these

- **Before suggesting refactors or architectural changes** (`improve-codebase-architecture`, `grill-with-docs`): read `CONTEXT.md` to anchor terminology and check `docs/adr/` for prior decisions on the area being changed.
- **When diagnosing bugs** (`diagnose`, `systematic-debugging`): use `CONTEXT.md` to map symptoms onto the right domain boundary (e.g. is this a Worker vs. API responsibility?).
- **When writing tests** (`tdd`): use `CONTEXT.md` vocabulary in test names and descriptions so they stay aligned with how the team talks about the system.
- **When making a decision that changes architecture or breaks a previously-documented convention**: add a new ADR under `docs/adr/` rather than silently editing.

## Bootstrap status

- `CONTEXT.md` — **not yet created**. Bootstrap with the domain model summary from `AGENTS.md` / `CLAUDE.md` (Account / Workspace / Profile / Skill model) plus the service responsibility table.
- `docs/adr/` — directory created, empty. Add the first ADR when the next architectural decision is made.

These can be filled in incrementally — agents reading this file should note when they discover information worth promoting into `CONTEXT.md` and offer to do so.
