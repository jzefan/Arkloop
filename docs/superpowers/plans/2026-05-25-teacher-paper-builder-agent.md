# Teacher Paper Builder Agent Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first usable teacher-facing "组卷" agent path: KB-first Q&A/source transparency, question draft context, explicit teacher-confirmed saves, and paper composition backed by the Account-level "组卷题库".

**Architecture:** Keep knowledge-base construction in `console-lite`; change `book-tutor-agent` into a teacher-facing paper-builder persona. Implement provider-neutral `kb_*` worker tools directly against the worker DB connection for standalone KBs and the existing exam client for linked KBs where possible. Generated local questions and papers are saved into an automatically ensured Account/workspace KB named "组卷题库"; drafting tools never persist.

**Tech Stack:** Go 1.26 worker/API services, existing `pgxpool` repositories, worker built-in tools, persona YAML/Markdown, existing `show_widget` tool, existing `kb_search` embedding path.

---

### Task 1: Rebrand And Constrain The Persona

**Files:**
- Modify: `src/personas/book-tutor-agent/persona.yaml`
- Modify: `src/personas/book-tutor-agent/prompt.md`

- [ ] **Step 1: Update persona metadata**

Set the title and selector to "智能组卷", and describe the agent as a teacher-facing paper builder that uses ready knowledge bases. Keep `id: book-tutor-agent` to avoid breaking stored persona references.

- [ ] **Step 2: Rewrite the prompt**

The prompt must state:
- this is "智能组卷", not "备课助手";
- teachers do not build/upload KBs here;
- Q&A searches KB first and says "未在当前课程资料中找到相关内容，以下为通用回答。" before general fallback;
- `kb_draft_questions` prepares drafts only;
- `kb_save_questions` requires explicit teacher confirmation;
- `kb_compose_paper` requires explicit confirmation to save;
- generated questions save to "组卷题库".

- [ ] **Step 3: Verify persona files parse**

Run: `go test ./internal/personas -count=1` from `src/services/api`.
Expected: PASS.

### Task 2: Register Provider-Neutral RAG Tools

**Files:**
- Modify: `src/services/worker/internal/tools/builtin/builtin.go`
- Modify: `src/services/worker/internal/tools/builtin/kb/rag_spec.go`

- [ ] **Step 1: Add the RAG tool specs to catalogs**

Add `kb.ListKnowledgePointsAgentSpec`, `kb.DraftQuestionsAgentSpec`, `kb.SaveQuestionsAgentSpec`, and `kb.ComposePaperAgentSpec` to `AgentSpecs()`. Add the matching LLM specs to `LlmSpecs()`.

- [ ] **Step 2: Add confirmation fields to save schemas**

Require `confirmed: true` on `kb_save_questions`. Add optional `confirmed` to `kb_compose_paper`; when false or omitted it returns a preview without saving.

- [ ] **Step 3: Wire executor map**

Point the four new tool names at the same KB executor instance as `kb_search`.

- [ ] **Step 4: Verify catalog build**

Run: `go test ./internal/tools/builtin -count=1` from `src/services/worker`.
Expected: PASS.

### Task 3: Implement Worker-Side RAG Executor

**Files:**
- Create: `src/services/worker/internal/tools/builtin/kb/rag_executor.go`
- Modify: `src/services/worker/internal/tools/builtin/kb/executor.go`
- Modify: `src/services/worker/internal/tools/builtin/kb/executor_test.go`

- [ ] **Step 1: Write failing tests**

Add tests that verify:
- `kb_save_questions` without `confirmed: true` returns `tool.confirmation_required`;
- `kb_draft_questions` returns `action=draft_questions`, retrieval hits, references, and does not save;
- `kb_compose_paper` without `confirmed` returns preview markdown and no paper id;
- `kb_compose_paper` with `confirmed: true` returns a paper id.

- [ ] **Step 2: Add narrow DB collaborators**

Extend `Executor` with optional `pool *pgxpool.Pool`. Keep existing fake-friendly collaborators for search tests. In `NewToolExecutor(pool)`, set the pool on the executor.

- [ ] **Step 3: Implement KB descriptor and group bank helpers**

Add helpers:
- `loadKBDescriptor(ctx, kbID)` returning `account_id`, `workspace_ref`, `integration_mode`, `exam_scope_id`;
- `ensurePaperBankKB(ctx, accountID, workspaceRef, userID)` that finds or creates a standalone KB named "组卷题库" for that Account/workspace;
- `isActorKBMember` using existing access checker before every tool.

- [ ] **Step 4: Implement `kb_list_knowledge_points`**

For standalone KBs, read `kb_knowledge_points` by `kb_id`. For linked KBs in this first slice, return `exam_linked_not_supported` if no exam client is wired into this executor.

- [ ] **Step 5: Implement `kb_draft_questions`**

Validate access, run existing `kb_search` logic with `retrieval_query`, load reference questions from "组卷题库" for the same knowledge point, and return structured generation instructions. Do not persist.

- [ ] **Step 6: Implement `kb_save_questions`**

Require `confirmed: true`. Ensure "组卷题库", validate each draft, insert into `kb_questions` with `quality_flag='accepted'`, `created_by=user_id`, and source snippets/options JSON. Return partial success.

- [ ] **Step 7: Implement `kb_compose_paper`**

Ensure "组卷题库", query candidate questions by `knowledge_point_ids`, type, and difficulty. If insufficient, return `shortage_warnings`. Otherwise select deterministically by seed, produce markdown. Save to `kb_papers` only when `confirmed: true`.

- [ ] **Step 8: Verify worker tests**

Run: `go test ./internal/tools/builtin/kb -count=1` from `src/services/worker`.
Expected: PASS.

### Task 4: App Wiring Safety

**Files:**
- Modify: `src/services/api/internal/app/app.go`

- [ ] **Step 1: Register existing localstore dependencies during API boot**

Construct `KBKnowledgePointsRepository`, `KBQuestionsRepository`, and `KBPapersRepository` beside the existing KB repos and call `localstore.Register(...)`. This keeps future API handlers and shared `questionstore.For` usable.

- [ ] **Step 2: Verify API app build**

Run: `go test ./internal/app ./internal/questionstore/localstore -count=1` from `src/services/api`.
Expected: PASS.

### Task 5: Final Verification

**Files:**
- All touched files.

- [ ] **Step 1: Run focused Go tests**

Run from `src/services/worker`: `go test ./internal/tools/builtin ./internal/tools/builtin/kb -count=1`.

Run from `src/services/api`: `go test ./internal/app ./internal/personas ./internal/questionstore/localstore -count=1`.

- [ ] **Step 2: Inspect dirty diff**

Run: `git status --short` and `git diff --stat`.

- [ ] **Step 3: Commit implementation**

Stage only implementation files for this feature and commit with:

```bash
git commit -m "feat: add teacher paper builder agent path"
```
