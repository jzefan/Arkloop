# 本地模式登录与 Setup 分流修复 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix local desktop auth routing so first-time setup stays on `/setup`, while expired or missing local sessions route users to the normal login page instead of the setup page.

**Architecture:** Keep the change entirely in the web app’s auth startup routing. Remove the current `/login` → `HeadlessSetupPage` dynamic switch based on `!accessToken`, preserve `/setup` as the explicit setup entry, and update startup tests so local setup is no longer inferred from missing session state.

**Tech Stack:** React, React Router, TypeScript, Vitest, existing `AuthPage`, `HeadlessSetupPage`, and local desktop auth helpers.

---

## Files And Responsibilities

- Modify `src/apps/web/src/appAuthStartup.ts`: narrow or remove the setup-route predicate so missing `accessToken` no longer implies local setup.
- Modify `src/apps/web/src/App.tsx`: ensure `/login` renders `AuthPage` when unauthenticated, and `/setup` remains the explicit `HeadlessSetupPage` route.
- Modify `src/apps/web/src/__tests__/appAuthStartup.test.ts`: update startup predicate tests to reflect the new setup/login split.
- Create `src/apps/web/src/__tests__/appLocalRouting.test.tsx`: verify local-mode unauthenticated `/login` renders `AuthPage` and `/setup` renders `HeadlessSetupPage`.

---

### Task 1: Update startup predicate semantics

**Files:**
- Modify: `src/apps/web/src/appAuthStartup.ts`
- Modify: `src/apps/web/src/__tests__/appAuthStartup.test.ts`
- Test: `src/apps/web/src/__tests__/appAuthStartup.test.ts`

- [ ] **Step 1: Write the failing startup predicate test**

Replace the third test in `src/apps/web/src/__tests__/appAuthStartup.test.ts` with:

```ts
it('does not infer local setup from missing session state', () => {
  expect(shouldUseLocalSetupRoute(true, null)).toBe(false)
  expect(shouldUseLocalSetupRoute(true, '')).toBe(false)
  expect(shouldUseLocalSetupRoute(true, 'jwt-token')).toBe(false)
  expect(shouldUseLocalSetupRoute(false, null)).toBe(false)
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run:

```bash
cd "/Users/jzefan/work/proj/ArkLoop/src/apps/web" && pnpm test -- --run src/__tests__/appAuthStartup.test.ts
```

Expected: FAIL because the current implementation still returns `true` for local mode with no session.

- [ ] **Step 3: Implement the minimal predicate change**

Change `src/apps/web/src/appAuthStartup.ts` from:

```ts
export function shouldUseLocalSetupRoute(localMode: boolean, accessToken: string | null): boolean {
  return localMode && !accessToken
}
```

to:

```ts
export function shouldUseLocalSetupRoute(_localMode: boolean, _accessToken: string | null): boolean {
  return false
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run:

```bash
cd "/Users/jzefan/work/proj/ArkLoop/src/apps/web" && pnpm test -- --run src/__tests__/appAuthStartup.test.ts
```

Expected: PASS.

- [ ] **Step 5: Commit the predicate change**

Run:

```bash
git -C "/Users/jzefan/work/proj/ArkLoop" add src/apps/web/src/appAuthStartup.ts src/apps/web/src/__tests__/appAuthStartup.test.ts
git -C "/Users/jzefan/work/proj/ArkLoop" commit -m "fix: stop inferring local setup from missing session"
```

---

### Task 2: Make `/login` always render the normal auth page

**Files:**
- Modify: `src/apps/web/src/App.tsx`
- Test: `src/apps/web/src/__tests__/appLocalRouting.test.tsx`

- [ ] **Step 1: Write the failing route test for `/login`**

Create `src/apps/web/src/__tests__/appLocalRouting.test.tsx` with this test first:

```tsx
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

vi.mock('@arkloop/shared/desktop', () => ({
  isLocalMode: () => true,
  isDesktop: () => false,
  getDesktopApi: () => null,
  getDesktopAccessToken: () => null,
}))

vi.mock('../components/AuthPage', () => ({
  AuthPage: () => <div data-testid="auth-page">auth-page</div>,
}))

vi.mock('../components/HeadlessSetupPage', () => ({
  HeadlessSetupPage: () => <div data-testid="headless-setup-page">headless-setup-page</div>,
}))

vi.mock('../contexts/LocaleContext', () => ({
  useLocale: () => ({ t: { loading: 'loading' } }),
}))

vi.mock('@arkloop/shared', async () => {
  const actual = await vi.importActual<typeof import('@arkloop/shared')>('@arkloop/shared')
  return {
    ...actual,
    useToast: () => ({ addToast: vi.fn() }),
    LoadingPage: ({ label }: { label: string }) => <div>{label}</div>,
  }
})

vi.mock('../api', () => ({
  createLocalSession: vi.fn(),
  isApiError: () => false,
  setUnauthenticatedHandler: vi.fn(),
  setAccessTokenHandler: vi.fn(),
  setSessionExpiredHandler: vi.fn(),
  restoreAccessSession: vi.fn(),
}))

vi.mock('@arkloop/shared/api', () => ({
  setClientApp: vi.fn(),
}))

import App from '../App'

describe('local auth routing', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('renders AuthPage on /login when local mode has no access token', async () => {
    window.history.replaceState({}, '', '/login')
    render(
      <MemoryRouter initialEntries={['/login']}>
        <App />
      </MemoryRouter>,
    )

    expect(await screen.findByTestId('auth-page')).toBeInTheDocument()
    expect(screen.queryByTestId('headless-setup-page')).not.toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run:

```bash
cd "/Users/jzefan/work/proj/ArkLoop/src/apps/web" && pnpm test -- --run src/__tests__/appLocalRouting.test.tsx
```

Expected: FAIL because `/login` currently still renders `HeadlessSetupPage` in local mode without an access token.

- [ ] **Step 3: Implement the route split in `App.tsx`**

Change `src/apps/web/src/App.tsx` from:

```tsx
const useLocalSetupRoute = shouldUseLocalSetupRoute(isLocalMode(), accessToken)
```

and:

```tsx
<Route path="/login" element={useLocalSetupRoute ? <HeadlessSetupPage onLoggedIn={handleLoggedIn} /> : <AuthPage onLoggedIn={handleLoggedIn} />} />
```

to:

```tsx
<Route path="/login" element={<AuthPage onLoggedIn={handleLoggedIn} />} />
```

and remove the now-unused `useLocalSetupRoute` local variable and its import if they become unused.

- [ ] **Step 4: Run the route test to verify it passes**

Run:

```bash
cd "/Users/jzefan/work/proj/ArkLoop/src/apps/web" && pnpm test -- --run src/__tests__/appLocalRouting.test.tsx
```

Expected: PASS.

- [ ] **Step 5: Commit the `/login` route fix**

Run:

```bash
git -C "/Users/jzefan/work/proj/ArkLoop" add src/apps/web/src/App.tsx src/apps/web/src/__tests__/appLocalRouting.test.tsx
git -C "/Users/jzefan/work/proj/ArkLoop" commit -m "fix: send local login to auth page"
```

---

### Task 3: Prove `/setup` still renders the setup page

**Files:**
- Modify: `src/apps/web/src/__tests__/appLocalRouting.test.tsx`
- Test: `src/apps/web/src/__tests__/appLocalRouting.test.tsx`

- [ ] **Step 1: Add the failing `/setup` route test**

Append this test to `src/apps/web/src/__tests__/appLocalRouting.test.tsx`:

```tsx
it('renders HeadlessSetupPage on /setup', async () => {
  window.history.replaceState({}, '', '/setup')
  render(
    <MemoryRouter initialEntries={['/setup']}>
      <App />
    </MemoryRouter>,
  )

  expect(await screen.findByTestId('headless-setup-page')).toBeInTheDocument()
  expect(screen.queryByTestId('auth-page')).not.toBeInTheDocument()
})
```

- [ ] **Step 2: Run the route test file**

Run:

```bash
cd "/Users/jzefan/work/proj/ArkLoop/src/apps/web" && pnpm test -- --run src/__tests__/appLocalRouting.test.tsx
```

Expected: PASS if `/setup` still routes correctly; if it fails, fix only the explicit `/setup` route wiring.

- [ ] **Step 3: If needed, repair only the explicit setup route**

Ensure `src/apps/web/src/App.tsx` still contains:

```tsx
<Route path="/setup" element={<HeadlessSetupPage onLoggedIn={handleLoggedIn} />} />
```

Do not change `HeadlessSetupPage` behavior.

- [ ] **Step 4: Re-run the route test file**

Run:

```bash
cd "/Users/jzefan/work/proj/ArkLoop/src/apps/web" && pnpm test -- --run src/__tests__/appLocalRouting.test.tsx
```

Expected: PASS with both route tests green.

- [ ] **Step 5: Commit the explicit setup coverage**

Run:

```bash
git -C "/Users/jzefan/work/proj/ArkLoop" add src/apps/web/src/__tests__/appLocalRouting.test.tsx
if ! git -C "/Users/jzefan/work/proj/ArkLoop" diff --cached --quiet; then
  git -C "/Users/jzefan/work/proj/ArkLoop" commit -m "test: cover local setup routing"
fi
```

---

### Task 4: Run focused verification for local auth routing

**Files:**
- Modify: `src/apps/web/src/appAuthStartup.ts`
- Modify: `src/apps/web/src/App.tsx`
- Modify: `src/apps/web/src/__tests__/appAuthStartup.test.ts`
- Modify/Create: `src/apps/web/src/__tests__/appLocalRouting.test.tsx`

- [ ] **Step 1: Run both auth routing test files together**

Run:

```bash
cd "/Users/jzefan/work/proj/ArkLoop/src/apps/web" && pnpm test -- --run src/__tests__/appAuthStartup.test.ts src/__tests__/appLocalRouting.test.tsx
```

Expected: PASS.

- [ ] **Step 2: Run the app web test suite once if the focused tests pass**

Run:

```bash
cd "/Users/jzefan/work/proj/ArkLoop/src/apps/web" && pnpm test -- --run
```

Expected: PASS. If the suite is too large or already known flaky, at minimum capture the exact failing tests before proceeding.

- [ ] **Step 3: Inspect the final diff**

Run:

```bash
git -C "/Users/jzefan/work/proj/ArkLoop" diff -- src/apps/web/src/appAuthStartup.ts src/apps/web/src/App.tsx src/apps/web/src/__tests__/appAuthStartup.test.ts src/apps/web/src/__tests__/appLocalRouting.test.tsx
```

Confirm the diff is limited to:

- removing the `!accessToken => setup` inference
- making `/login` render `AuthPage`
- preserving `/setup` as the explicit setup route
- the new or updated local auth routing tests

- [ ] **Step 4: Commit the final verified patch**

Run:

```bash
git -C "/Users/jzefan/work/proj/ArkLoop" add src/apps/web/src/appAuthStartup.ts src/apps/web/src/App.tsx src/apps/web/src/__tests__/appAuthStartup.test.ts src/apps/web/src/__tests__/appLocalRouting.test.tsx
git -C "/Users/jzefan/work/proj/ArkLoop" commit -m "fix: separate local setup from login"
```

---

## Self-Review

- **Spec coverage:** Task 1 removes the old startup inference from missing session state. Task 2 makes `/login` always use `AuthPage`. Task 3 proves `/setup` still uses `HeadlessSetupPage`. Task 4 verifies the focused auth-routing behavior and checks the final diff stays within scope.
- **Placeholder scan:** No TODO/TBD markers remain. Every task includes exact file paths, code blocks, test commands, and expected outcomes.
- **Type consistency:** The plan consistently treats `/setup` as explicit setup, `/login` as login, and `shouldUseLocalSetupRoute` as no longer depending on `!accessToken` semantics.
