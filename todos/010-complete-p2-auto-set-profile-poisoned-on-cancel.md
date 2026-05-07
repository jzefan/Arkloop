---
name: Auto-set video/image model profile poisoned if API call is cancelled before response
description: autoSetRef marks profile as "auto-set" before the API call completes — if cancelled, profile is never set but autoSetRef prevents retry for the lifetime of the component mount
type: pending
priority: p2
issue_id: "010"
tags: [code-review, correctness]
dependencies: []
---

## Problem Statement

In `GenerationModelSettings.tsx`, `autoSetRef.current.add(profile)` is called synchronously before `await setSpawnProfile(...)`. If the effect's cleanup fires (dependency change, re-render) while the API call is in-flight, the profile is permanently marked as auto-set even though the API call never completed. The user ends up with no default model configured and no mechanism to retry within the same component mount.

## Findings

```typescript
// GenerationModelSettings.tsx:117-137
for (const [profile, value] of updates) {
  autoSetRef.current.add(profile)   // ← marks as done BEFORE await
  await setSpawnProfile(accessToken, profile, value)
  if (cancelled) { setProfiles(...); return }
  ...
}
```

- `autoSetRef.current.add(profile)` at line 127 runs synchronously before `await`
- If `cancelled` becomes `true` (effect cleanup called) during the `await`, the profile name is already in the Set
- `autoSetRef` is a `useRef` — it persists for the component's lifetime and is NOT reset on re-render
- Subsequent renders skip the auto-set block because `autoSetRef.current.has(profile)` returns true
- Impact: limited to the current mount (navigation away and back creates a new `Set`), but on a long-lived settings page this is a real failure mode

## Proposed Solutions

### Option A — Mark as done only after successful API response (Recommended)

```typescript
const succeeded = await setSpawnProfile(accessToken, profile, value)
if (!cancelled && succeeded) {
  autoSetRef.current.add(profile)
}
```

- **Pros:** Correct. Simple rearrangement.
- **Cons:** May cause duplicate API calls on rapid re-render; add a `pending` guard if needed.
- **Effort:** Small
- **Risk:** Low

### Option B — Add a pending Set alongside the done Set

Track which profiles have an in-flight request to prevent duplicate calls, and only promote to the "done" Set on success.

```typescript
const pendingAutoSet = useRef(new Set<GenerationProfile>())
// check both sets before starting; add to done only on success
```

- **Pros:** Prevents duplicate in-flight calls during rapid re-renders.
- **Cons:** More complex.
- **Effort:** Small-Medium
- **Risk:** Low

## Recommended Action

Option A. The rapid re-render duplicate-call concern is low probability given the effect dependencies.

## Technical Details

- File: `src/apps/web/src/components/settings/GenerationModelSettings.tsx:117-137`

## Acceptance Criteria

- [ ] `autoSetRef` is only updated after a confirmed successful API response
- [ ] If the effect is cancelled mid-call, the profile is not marked as auto-set
- [ ] Test: auto-set with a cancelled effect does not permanently suppress the next attempt

## Work Log

- 2026-05-06: Identified by correctness review agent (F2).
