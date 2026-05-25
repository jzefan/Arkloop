# Knowledge Scope Provider Proxy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make ArkLoop expose curriculum scopes through `GET /v1/knowledge-bases/scopes`, backed internally by a server-minted exam OIDC token for the current ArkLoop user.

**Architecture:** Add a narrow exam token source interface to `kbapi`, route ArkLoop frontends through `handleKnowledgeBaseScopes`, and keep `handleExamScopes` as a compatibility alias. The API mints `openid exam:read` server-side, calls the configured provider, and never returns exam tokens to the frontend.

**Tech Stack:** Go 1.26 workspace, `net/http`, existing `oauthapi` OIDC signer, focused `go test` package verification.

---

### Task 1: Server-side Exam Token for Scopes Proxy

**Files:**
- Modify: `src/services/api/internal/http/kbapi/handler_kb.go`
- Modify: `src/services/api/internal/http/kbapi/deps.go`
- Modify: `src/services/api/internal/http/kbapi/handler_kb_test.go`
- Modify: `src/services/api/internal/http/kbapi/register.go`
- Create: `src/services/api/internal/http/kbapi/exam_token_source.go`
- Create: `src/services/api/internal/http/kbapi/exam_scopes_lister.go`
- Modify: `src/services/api/internal/app/app.go`
- Modify: `src/apps/console-lite/src/api/knowledge-bases.ts`
- Modify: `src/apps/console-lite/src/components/CreateKBModal.tsx`
- Modify: `src/apps/console-lite/src/pages/KnowledgeBasesPage.tsx`

- [ ] **Step 1: Write the failing test**

Add a test where `handleKnowledgeBaseScopes` receives an authenticated ArkLoop actor and the fake lister must receive `exam-token`. The fake token source records `userID` and scopes.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/http/kbapi -run TestHandleKnowledgeBaseScopes_MintsExamTokenInternally -count=1`

Expected: FAIL because the ArkLoop domain handler does not exist yet.

- [ ] **Step 3: Implement minimal code**

Add `examTokenSource`, route `/v1/knowledge-bases/scopes` to `handleKnowledgeBaseScopes`, keep `/v1/exam/scopes` as an alias, add an `examstore` adapter for `GET /api/exam-scopes`, and wire production with the existing OIDC signing service.

- [ ] **Step 4: Run focused tests**

Run: `go test ./internal/http/kbapi -run 'TestHandleExamScopes|TestCreateKB' -count=1`

Expected: PASS.

- [ ] **Step 5: Run package tests**

Run: `go test ./internal/http/kbapi -count=1`

Expected: PASS.
