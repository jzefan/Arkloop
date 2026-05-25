import { buildUrl } from '@arkloop/shared'
import { apiFetch } from './client'

export type KBDocumentStatus =
  | 'queued'
  | 'parsing'
  | 'chunking'
  | 'embedding'
  | 'upserting'
  | 'ready'
  | 'failed'

export interface KnowledgeBase {
  id: string
  name: string
  workspace_ref: string
  description: string
  visibility: string
  integration_mode: string
  exam_scope_id?: string
  document_count?: number
  created_at: string
  updated_at: string
}

// KnowledgeScope is the curriculum hierarchy exposed by ArkLoop. Deployments
// may back it with a linked question-bank provider, but callers do not need to
// know where the data is stored.
export interface KnowledgeScope {
  id: string
  type: 'major' | 'direction' | 'topic'
  code: string
  display_name: string
  parent_id: string | null
}

export interface KBDocument {
  id: string
  original_filename: string
  mime_type: string
  size_bytes: number
  status: KBDocumentStatus
  error_message: string
  parse_meta?: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface SearchHit {
  document_ref: string
  ordinal: number
  heading_path: string[]
  chunk_type: string
  text: string
  score: number
  metadata?: Record<string, unknown>
}

export async function getDefaultKBWorkspace(accessToken: string): Promise<string> {
  const resp = await apiFetch<{ workspace_ref: string }>('/v1/knowledge-bases/default-workspace', { accessToken })
  return resp.workspace_ref
}

export async function listKnowledgeBases(accessToken: string, workspaceRef?: string): Promise<KnowledgeBase[]> {
  const query = workspaceRef ? `?workspace_ref=${encodeURIComponent(workspaceRef)}` : ''
  const resp = await apiFetch<{ items: KnowledgeBase[] }>(`/v1/knowledge-bases${query}`, { accessToken })
  return resp.items ?? []
}

export async function createKnowledgeBase(
  accessToken: string,
  body: {
    name: string
    workspace_ref?: string
    description?: string
    visibility?: string
    integration_mode?: string
    exam_scope_id?: string
  },
): Promise<KnowledgeBase> {
  return apiFetch<KnowledgeBase>('/v1/knowledge-bases', {
    method: 'POST',
    body: JSON.stringify(body),
    accessToken,
  })
}

export interface PlatformConfig {
  exam_integration_enabled: boolean
}

export async function getPlatformConfig(accessToken: string): Promise<PlatformConfig> {
  return apiFetch<PlatformConfig>('/v1/config', { accessToken })
}

export async function listKnowledgeScopes(accessToken: string): Promise<KnowledgeScope[]> {
  const resp = await apiFetch<{ items: KnowledgeScope[] }>('/v1/knowledge-bases/scopes', { accessToken })
  return resp.items ?? []
}

export async function deleteKnowledgeBase(accessToken: string, id: string): Promise<void> {
  await apiFetch<void>(`/v1/knowledge-bases/${id}`, {
    method: 'DELETE',
    accessToken,
  })
}

export async function getKnowledgeBase(accessToken: string, id: string): Promise<KnowledgeBase> {
  return apiFetch<KnowledgeBase>(`/v1/knowledge-bases/${id}`, { accessToken })
}

export async function listDocuments(accessToken: string, kbId: string): Promise<KBDocument[]> {
  const resp = await apiFetch<{ items: KBDocument[] }>(`/v1/knowledge-bases/${kbId}/documents`, { accessToken })
  return resp.items ?? []
}

export async function getDocument(accessToken: string, kbId: string, docId: string): Promise<KBDocument> {
  return apiFetch<KBDocument>(`/v1/knowledge-bases/${kbId}/documents/${docId}`, { accessToken })
}

export async function deleteDocument(accessToken: string, kbId: string, docId: string): Promise<void> {
  await apiFetch<void>(`/v1/knowledge-bases/${kbId}/documents/${docId}`, {
    method: 'DELETE',
    accessToken,
  })
}

export async function uploadDocument(
  accessToken: string,
  kbId: string,
  file: File,
): Promise<{ document_id: string; job_id: string }> {
  const fd = new FormData()
  fd.append('file', file)
  const resp = await fetch(buildUrl(`/v1/knowledge-bases/${kbId}/documents`), {
    method: 'POST',
    headers: {
      Accept: 'application/json',
      Authorization: `Bearer ${accessToken}`,
      'X-Client-App': 'console-lite',
    },
    credentials: 'include',
    body: fd,
  })
  if (!resp.ok) {
    const text = await resp.text()
    throw new Error(text || `upload failed (${resp.status})`)
  }
  return resp.json() as Promise<{ document_id: string; job_id: string }>
}

export async function searchKnowledgeBase(
  accessToken: string,
  kbId: string,
  query: string,
  k: number,
): Promise<SearchHit[]> {
  const params = new URLSearchParams({ q: query, k: String(k) })
  const resp = await apiFetch<{ hits: SearchHit[] }>(`/v1/knowledge-bases/${kbId}/search?${params.toString()}`, {
    accessToken,
  })
  return resp.hits ?? []
}
