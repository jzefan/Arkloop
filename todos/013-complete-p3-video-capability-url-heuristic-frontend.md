---
name: hasConfiguredGenerationProvider for video uses Vertex AI URL substring heuristic
description: UI checks for "zenmux.ai/api/vertex-ai" URL string to determine video capability — will silently exclude future video providers not on Vertex AI
type: pending
priority: p3
issue_id: "013"
tags: [code-review, maintainability, architecture]
dependencies: []
---

## Problem Statement

`hasConfiguredGenerationProvider` for the `'video'` profile returns `true` only when the provider passes `isZenMuxVertexProvider` — a function checking for `provider === 'gemini'` AND a URL containing `zenmux.ai/api/vertex-ai`. This couples a frontend UI decision to a backend routing implementation detail. Adding a second video backend (e.g., Replicate, Runway) will not automatically enable the auto-set logic in the UI.

## Findings

```typescript
// GenerationModelSettings.tsx:259-263
case 'video':
  return providers.some(p => isZenMuxVertexProvider(p) && zenMuxModelSupports(m, 'video'))
```
File: `src/apps/web/src/components/settings/GenerationModelSettings.tsx:259-263`

The provider model already has `output_modalities` used elsewhere (`modelSupportsOutputModality` at line 242). A modality-based check would be more robust.

## Proposed Solution

Replace the URL heuristic with the existing modality check:

```typescript
case 'video':
  return providers.some(p => modelSupportsOutputModality(m, 'video'))
```

Or add an explicit `supports_video_generation` capability flag populated by the backend provider catalog.

**Effort:** Small — contingent on the provider model having a reliable `video` modality signal.

## Acceptance Criteria

- [ ] `hasConfiguredGenerationProvider('video')` does not reference URL substrings
- [ ] A non-Vertex video provider, if added to the backend, is correctly detected in the UI

## Work Log

- 2026-05-06: Identified by maintainability review agent (MAINT-008).
