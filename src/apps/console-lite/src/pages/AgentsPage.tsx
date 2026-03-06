import { useState, useCallback, useEffect, useMemo } from 'react'
import { useOutletContext } from 'react-router-dom'
import { Plus, Pencil, Trash2, ChevronDown, ChevronRight } from 'lucide-react'
import type { LiteOutletContext } from '../layouts/LiteLayout'
import { PageHeader } from '../components/PageHeader'
import { Modal } from '../components/Modal'
import { FormField } from '../components/FormField'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { useToast } from '../components/useToast'
import { useLocale } from '../contexts/LocaleContext'
import { isApiError } from '../api'
import {
  listPersonas,
  createPersona,
  patchPersona,
  listAgentConfigs,
  createAgentConfig,
  updateAgentConfig,
  deleteAgentConfig,
  listLlmCredentials,
  listToolProviders,
  type Persona,
  type AgentConfig,
  type LlmCredential,
  type ToolProviderItem,
} from '../api/agents'

// merged view
type AgentView = {
  persona: Persona
  config: AgentConfig
}

type FormState = {
  name: string
  model: string
  systemPrompt: string
  tools: string[]
  isDefault: boolean
  isActive: boolean
  temperature: number
  maxOutputTokens: string
  reasoningMode: string
}

function emptyForm(): FormState {
  return {
    name: '',
    model: '',
    systemPrompt: '',
    tools: [],
    isDefault: false,
    isActive: true,
    temperature: 0.7,
    maxOutputTokens: '',
    reasoningMode: 'disabled',
  }
}

function agentToForm(agent: AgentView): FormState {
  return {
    name: agent.persona.display_name,
    model: agent.config.model ?? agent.persona.preferred_credential ?? '',
    systemPrompt: agent.persona.prompt_md,
    tools: agent.persona.tool_allowlist,
    isDefault: agent.config.is_default,
    isActive: agent.persona.is_active,
    temperature: agent.config.temperature ?? 0.7,
    maxOutputTokens: agent.config.max_output_tokens != null ? String(agent.config.max_output_tokens) : '',
    reasoningMode: agent.config.reasoning_mode ?? 'disabled',
  }
}

function slugify(name: string): string {
  const slug = name
    .toLowerCase()
    .replace(/[^a-z0-9\u4e00-\u9fff]+/g, '-')
    .replace(/^-|-$/g, '')
  return slug || 'agent'
}

const INPUT_CLS =
  'w-full rounded-lg border border-[var(--c-border)] bg-[var(--c-bg-input)] px-3 py-1.5 text-sm text-[var(--c-text-primary)] outline-none transition-colors focus:border-[var(--c-border-focus)]'
const SELECT_CLS =
  'w-full rounded-lg border border-[var(--c-border)] bg-[var(--c-bg-input)] px-3 py-1.5 text-sm text-[var(--c-text-primary)] outline-none transition-colors focus:border-[var(--c-border-focus)]'

export function AgentsPage() {
  const { accessToken } = useOutletContext<LiteOutletContext>()
  const { addToast } = useToast()
  const { t } = useLocale()
  const ta = t.agents

  const [agents, setAgents] = useState<AgentView[]>([])
  const [credentials, setCredentials] = useState<LlmCredential[]>([])
  const [activeTools, setActiveTools] = useState<ToolProviderItem[]>([])
  const [loading, setLoading] = useState(false)

  // modal state
  const [modalOpen, setModalOpen] = useState(false)
  const [editingAgent, setEditingAgent] = useState<AgentView | null>(null)
  const [form, setForm] = useState<FormState>(emptyForm)
  const [saving, setSaving] = useState(false)
  const [advancedOpen, setAdvancedOpen] = useState(false)

  // delete state
  const [deleteTarget, setDeleteTarget] = useState<AgentView | null>(null)
  const [deleting, setDeleting] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [personas, configs, creds, toolsResp] = await Promise.all([
        listPersonas(accessToken),
        listAgentConfigs(accessToken),
        listLlmCredentials(accessToken),
        listToolProviders(accessToken),
      ])

      // join: config.persona_id -> persona.id
      const personaMap = new Map(personas.map((p) => [p.id, p]))
      const joined: AgentView[] = []
      for (const cfg of configs) {
        if (!cfg.persona_id) continue
        const persona = personaMap.get(cfg.persona_id)
        if (persona) joined.push({ persona, config: cfg })
      }
      setAgents(joined)
      setCredentials(creds)

      const tools: ToolProviderItem[] = []
      for (const g of toolsResp.groups) {
        for (const p of g.providers) {
          if (p.is_active) tools.push(p)
        }
      }
      setActiveTools(tools)
    } catch (err) {
      addToast(isApiError(err) ? err.message : t.requestFailed, 'error')
    } finally {
      setLoading(false)
    }
  }, [accessToken, addToast, t.requestFailed])

  useEffect(() => {
    void load()
  }, [load])

  // -- modal handlers --

  const openCreate = useCallback(() => {
    setEditingAgent(null)
    setForm(emptyForm())
    setAdvancedOpen(false)
    setModalOpen(true)
  }, [])

  const openEdit = useCallback((agent: AgentView) => {
    setEditingAgent(agent)
    setForm(agentToForm(agent))
    setAdvancedOpen(false)
    setModalOpen(true)
  }, [])

  const closeModal = useCallback(() => {
    setModalOpen(false)
    setEditingAgent(null)
  }, [])

  const handleSave = useCallback(async () => {
    if (!form.name.trim() || !form.model.trim() || !form.systemPrompt.trim()) return
    setSaving(true)
    try {
      if (editingAgent) {
        // update persona
        await patchPersona(editingAgent.persona.id, {
          display_name: form.name.trim(),
          prompt_md: form.systemPrompt.trim(),
          tool_allowlist: form.tools,
          is_active: form.isActive,
          preferred_credential: form.model.trim(),
        }, accessToken)

        // update agent config
        await updateAgentConfig(editingAgent.config.id, {
          name: form.name.trim(),
          system_prompt_override: form.systemPrompt.trim(),
          model: form.model.trim(),
          temperature: form.temperature,
          max_output_tokens: form.maxOutputTokens ? Number(form.maxOutputTokens) : undefined,
          tool_policy: form.tools.length > 0 ? 'allowlist' : 'none',
          tool_allowlist: form.tools,
          is_default: form.isDefault,
          reasoning_mode: form.reasoningMode,
        }, accessToken)
      } else {
        // create persona
        const personaKey = `${slugify(form.name)}-${Date.now()}`
        const persona = await createPersona({
          persona_key: personaKey,
          version: '1.0',
          display_name: form.name.trim(),
          prompt_md: form.systemPrompt.trim(),
          tool_allowlist: form.tools,
          preferred_credential: form.model.trim(),
          executor_type: 'agent.simple',
        }, accessToken)

        // create agent config
        await createAgentConfig({
          scope: 'platform',
          name: form.name.trim(),
          system_prompt_override: form.systemPrompt.trim(),
          model: form.model.trim(),
          temperature: form.temperature,
          max_output_tokens: form.maxOutputTokens ? Number(form.maxOutputTokens) : undefined,
          tool_policy: form.tools.length > 0 ? 'allowlist' : 'none',
          tool_allowlist: form.tools,
          persona_id: persona.id,
          is_default: form.isDefault,
          prompt_cache_control: 'none',
          reasoning_mode: form.reasoningMode,
          content_filter_level: '',
        }, accessToken)
      }
      closeModal()
      await load()
    } catch (err) {
      addToast(isApiError(err) ? err.message : t.requestFailed, 'error')
    } finally {
      setSaving(false)
    }
  }, [form, editingAgent, accessToken, addToast, t.requestFailed, closeModal, load])

  // -- delete handlers --

  const handleDelete = useCallback(async () => {
    if (!deleteTarget) return
    setDeleting(true)
    try {
      await deleteAgentConfig(deleteTarget.config.id, accessToken)
      await patchPersona(deleteTarget.persona.id, { is_active: false }, accessToken)
      setDeleteTarget(null)
      await load()
    } catch (err) {
      addToast(isApiError(err) ? err.message : t.requestFailed, 'error')
    } finally {
      setDeleting(false)
    }
  }, [deleteTarget, accessToken, addToast, t.requestFailed, load])

  // -- tool toggle --
  const toggleTool = useCallback((toolKey: string) => {
    setForm((prev) => ({
      ...prev,
      tools: prev.tools.includes(toolKey)
        ? prev.tools.filter((k) => k !== toolKey)
        : [...prev.tools, toolKey],
    }))
  }, [])

  // sorted: default first, then by name
  const sortedAgents = useMemo(
    () =>
      [...agents].sort((a, b) => {
        if (a.config.is_default !== b.config.is_default) return a.config.is_default ? -1 : 1
        return a.persona.display_name.localeCompare(b.persona.display_name)
      }),
    [agents],
  )

  const actions = (
    <button
      onClick={openCreate}
      className="flex items-center gap-1.5 rounded-lg bg-[var(--c-bg-tag)] px-3 py-1.5 text-xs font-medium text-[var(--c-text-secondary)] transition-colors hover:bg-[var(--c-bg-sub)]"
    >
      <Plus size={13} />
      {ta.newAgent}
    </button>
  )

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <PageHeader title={ta.title} actions={actions} />

      <div className="flex flex-1 flex-col gap-4 overflow-auto p-4">
        {loading && agents.length === 0 ? (
          <div className="flex flex-1 items-center justify-center">
            <span className="text-sm text-[var(--c-text-muted)]">{t.common.loading}</span>
          </div>
        ) : agents.length === 0 ? (
          <div className="flex flex-1 items-center justify-center">
            <span className="text-sm text-[var(--c-text-muted)]">{ta.noAgents}</span>
          </div>
        ) : (
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {sortedAgents.map((agent) => (
              <div
                key={agent.config.id}
                className="flex flex-col gap-3 rounded-xl border border-[var(--c-border)] bg-[var(--c-bg-sub)] px-5 py-4"
              >
                <div className="flex items-start justify-between gap-2">
                  <h3 className="text-sm font-medium text-[var(--c-text-primary)]">
                    {agent.persona.display_name}
                  </h3>
                  <div className="flex shrink-0 items-center gap-1.5">
                    {agent.config.is_default && (
                      <span className="rounded bg-[var(--c-bg-tag)] px-1.5 py-0.5 text-[10px] font-medium text-[var(--c-text-muted)]">
                        {t.common.default}
                      </span>
                    )}
                    {agent.persona.is_active && (
                      <span className="rounded bg-emerald-500/10 px-1.5 py-0.5 text-[10px] font-medium text-emerald-500">
                        {ta.active}
                      </span>
                    )}
                  </div>
                </div>

                <div className="text-xs text-[var(--c-text-muted)]">
                  {ta.model}: {agent.config.model || '-'}
                </div>

                <div className="flex items-center justify-end gap-2 pt-1">
                  <button
                    onClick={() => openEdit(agent)}
                    className="flex items-center gap-1 rounded-md px-2 py-1 text-xs text-[var(--c-text-tertiary)] transition-colors hover:bg-[var(--c-bg-page)] hover:text-[var(--c-text-secondary)]"
                  >
                    <Pencil size={12} />
                    {t.common.edit}
                  </button>
                  <button
                    onClick={() => setDeleteTarget(agent)}
                    className="flex items-center gap-1 rounded-md px-2 py-1 text-xs text-[var(--c-text-tertiary)] transition-colors hover:bg-[var(--c-bg-page)] hover:text-red-500"
                  >
                    <Trash2 size={12} />
                    {t.common.delete}
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Create / Edit Modal */}
      <Modal
        open={modalOpen}
        onClose={closeModal}
        title={editingAgent ? ta.editAgent : ta.newAgent}
        width="540px"
      >
        <div className="flex flex-col gap-4">
          <FormField label={`${ta.name} *`}>
            <input
              className={INPUT_CLS}
              value={form.name}
              onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
            />
          </FormField>

          <FormField label={`${ta.model} *`}>
            <select
              className={SELECT_CLS}
              value={form.model}
              onChange={(e) => setForm((f) => ({ ...f, model: e.target.value }))}
            >
              <option value="" />
              {credentials.map((c) => (
                <option key={c.id} value={c.name}>{c.name}</option>
              ))}
            </select>
          </FormField>

          <FormField label={`${ta.systemPrompt} *`}>
            <textarea
              className={`${INPUT_CLS} min-h-[100px] resize-y`}
              rows={4}
              value={form.systemPrompt}
              onChange={(e) => setForm((f) => ({ ...f, systemPrompt: e.target.value }))}
            />
          </FormField>

          {activeTools.length > 0 && (
            <FormField label={ta.tools}>
              <div className="flex flex-wrap gap-x-4 gap-y-2">
                {activeTools.map((tool) => {
                  const key = `${tool.group_name}.${tool.provider_name}`
                  return (
                    <label key={key} className="flex items-center gap-1.5 text-sm text-[var(--c-text-secondary)]">
                      <input
                        type="checkbox"
                        checked={form.tools.includes(key)}
                        onChange={() => toggleTool(key)}
                        className="accent-[var(--c-accent)]"
                      />
                      {tool.provider_name}
                    </label>
                  )
                })}
              </div>
            </FormField>
          )}

          <label className="flex items-center gap-2 text-sm text-[var(--c-text-secondary)]">
            <input
              type="checkbox"
              checked={form.isDefault}
              onChange={(e) => setForm((f) => ({ ...f, isDefault: e.target.checked }))}
              className="accent-[var(--c-accent)]"
            />
            {ta.setDefault}
          </label>

          {/* Advanced section */}
          <button
            type="button"
            onClick={() => setAdvancedOpen((v) => !v)}
            className="flex items-center gap-1 text-xs font-medium text-[var(--c-text-tertiary)] transition-colors hover:text-[var(--c-text-secondary)]"
          >
            {advancedOpen ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
            {ta.advanced}
          </button>

          {advancedOpen && (
            <div className="flex flex-col gap-4 border-t border-dashed border-[var(--c-border)] pt-4">
              <FormField label={ta.temperature}>
                <div className="flex items-center gap-3">
                  <input
                    type="range"
                    min={0}
                    max={2}
                    step={0.1}
                    value={form.temperature}
                    onChange={(e) => setForm((f) => ({ ...f, temperature: Number(e.target.value) }))}
                    className="flex-1"
                  />
                  <span className="w-8 text-right text-xs tabular-nums text-[var(--c-text-muted)]">
                    {form.temperature.toFixed(1)}
                  </span>
                </div>
              </FormField>

              <FormField label={ta.maxOutputTokens}>
                <input
                  type="number"
                  className={INPUT_CLS}
                  value={form.maxOutputTokens}
                  onChange={(e) => setForm((f) => ({ ...f, maxOutputTokens: e.target.value }))}
                  placeholder=""
                />
              </FormField>

              <FormField label={ta.reasoningMode}>
                <select
                  className={SELECT_CLS}
                  value={form.reasoningMode}
                  onChange={(e) => setForm((f) => ({ ...f, reasoningMode: e.target.value }))}
                >
                  <option value="disabled">{ta.reasoningDisabled}</option>
                  <option value="enabled">{ta.reasoningEnabled}</option>
                </select>
              </FormField>
            </div>
          )}

          {/* Footer */}
          <div className="flex justify-end gap-2 pt-2">
            <button
              onClick={closeModal}
              className="rounded-lg border border-[var(--c-border)] px-3.5 py-1.5 text-sm text-[var(--c-text-secondary)] transition-colors hover:bg-[var(--c-bg-sub)]"
            >
              {t.common.cancel}
            </button>
            <button
              onClick={handleSave}
              disabled={saving || !form.name.trim() || !form.model.trim() || !form.systemPrompt.trim()}
              className="rounded-lg bg-[var(--c-accent)] px-3.5 py-1.5 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-50"
            >
              {saving ? '...' : t.common.save}
            </button>
          </div>
        </div>
      </Modal>

      {/* Delete Confirmation */}
      <ConfirmDialog
        open={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        onConfirm={handleDelete}
        message={ta.deleteConfirm}
        confirmLabel={t.common.delete}
        loading={deleting}
      />
    </div>
  )
}
