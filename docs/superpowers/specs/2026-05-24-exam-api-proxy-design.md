# Knowledge Scope Provider Proxy Design

## Goal

ArkLoop users should operate only inside ArkLoop. When ArkLoop needs curriculum or question-bank data, it may use exam internally, but users and ArkLoop frontends should not need to know where that data comes from.

## Scope

This slice adds `GET /v1/knowledge-bases/scopes` as the ArkLoop-facing API for selectable curriculum scopes. It also fixes the existing compatibility path `GET /v1/exam/scopes` so neither route forwards ArkLoop's first-party access token to exam. Instead, the API server derives the current actor from the ArkLoop request, mints an exam OIDC token server-side, and forwards that token to exam.

Future provider-backed routes should use ArkLoop domain names first, with exam-specific naming kept behind API internals.

## Architecture

- `kbapi` owns `/v1/knowledge-bases/scopes` and keeps `/v1/exam/scopes` only for compatibility.
- `handlerCtx` gets an `examTokenSource` collaborator with a narrow `IssueExamToken(ctx, userID, scopes)` method.
- `handleKnowledgeBaseScopes` requires an authenticated actor, requests `openid exam:read`, calls the existing `examScopesLister`, and never exposes the OIDC token in the response.
- ArkLoop console-lite calls `/v1/knowledge-bases/scopes` and labels this as "课程范围" / "题库来源"; it does not mention exam.
- Production wiring can use the existing OIDC service directly instead of calling `/internal/oauth/issue`, avoiding a loopback HTTP dependency inside the API process.

## Error Handling

- Missing token source or upstream lister returns `404 exam.not_configured`.
- Token mint failure returns `502 exam.token_issue_failed`.
- Exam upstream failure returns `502 exam.upstream_failed`.

## Testing

Add a focused `kbapi` unit test that injects an actor, verifies the lister receives the minted exam token, and verifies the token source receives the current user id with `openid exam:read`.
