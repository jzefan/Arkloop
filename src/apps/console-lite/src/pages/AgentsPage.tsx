import { useState, useCallback, useEffect, useMemo } from 'react'
import { useOutletContext } from 'react-router-dom'
import { Plus, Trash2, ChevronLeft, Check } from 'lucide-react'
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
  listToolCatalog,
  type Persona,
  type AgentConfig,
  type LlmCredential,
  type ToolCatalogGroup,
} from '../api/agents'

// -- types --

type AgentView = {
  persona: Persona | null
  config: AgentConfig
}

type DetailTab = 'overview' | 'persona' | 'tools'

type DetailForm = {
  name: string
  model: string
  isDefault: boolean
  isActive: boolean
  temperature: number
  maxOutputTokens: string
  reasoningMode: string
  systemPrompt: string
  tools: string[]
}

function agentToForm(agent: AgentView): DetailForm {
  return {
    name: agent.persona?.display_name || agent.config.name,
    model: agent.config.model || agent.persona?.preferred_credential || '',
    isDefault: agent.config.is_default,
    isActive: agent.persona?.is_active ?? true,
    temperature: agent.config.temperature ?? 0.7,
    maxOutputTokens: agent.config.max_output_tokens != null
      ? String(agent.config.max_output_tokens) : '',
    reasoningMode: agent.config.reasoning_mode ?? 'disabled',
    systemPrompt: agent.persona?.prompt_md || agent.config.system_prompt_override || '',
    tools: agent.persona?.tool_allowlist ?? agent.config.tool_allowlist ?? [],
  }
}

function slugify(name: string): string {
  const s = name.toLowerCase().replace(/[^a-z0-9\u4e00-\u9fff]+/g, '-').replace(/^-|-$/g, '')
  return s || 'agent'
}

function agentName(a: AgentView): string {
  return a.persona?.display_name || a.config.name
}

// build persona.yaml-style JSON from current state
function buildPersonaConfig(persona: Persona | null, form: DetailForm): string {
  if (!persona) return '{}'
  return JSON.stringify({
    id: persona.persona_key,
    version: persona.version,
    title: form.name,
    description: persona.description ?? '',
    tool_allowlist: form.tools,
    budgets: persona.budgets,
    is_active: form.isActive,
    executor_type: persona.executor_type,
    agent_config: form.model,
  }, null, 2)
}

// -- custom checkbox --

function CheckboxField({ checked, onChange, label }: {
  checked: boolean
  onChange: (v: boolean) => void
  label: string
}) {
  return (
    <label className="flex cursor-pointer select-none items-center gap-2.5 text-sm text-[var(--c-text-secondary)]">
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        className="sr-only"
      />
      <span
        className={[
          'flex h-[16px] w-[16px] shrink-0 items-center justify-center rounded-[4px] border transition-colors',
          checked
            ? 'border-[var(--c-accent)] bg-[var(--c-accent)]'
            : 'border-[var(--c-border)] bg-[var(--c-bg-input)]',
        ].join(' ')}
      >
        {checked && <Check size={11} className="text-white" strokeWidth={3} />}
      </span>
      {label}
    </label>
  )
}

// -- styles --

const INPUT_CLS =
  'w-full rounded-lg border border-[var(--c-border)] bg-[var(--c-bg-input)] px-3 py-1.5 text-sm text-[var(--c-text-primary)] outline-none transition-colors focus:border-[var(--c-border-focus)]'
const SELECT_CLS =
  'w-full rounded-lg border border-[var(--c-border)] bg-[var(--c-bg-input)] px-3 py-1.5 text-sm text-[var(--c-text-primary)] outline-none transition-colors focus:border-[var(--c-border-focus)]'
const MONO_CLS =
  'w-full rounded-lg border border-[var(--c-border)] bg-[var(--c-bg-input)] px-3 py-2 font-mono text-xs leading-relaxed text-[var(--c-text-primary)] outline-none transition-colors focus:border-[var(--c-border-focus)]'

export function AgentsPage() {
  const { accessToken } = useOutletContext<LiteOutletContext>()
  const { addToast } = useToast()
  const { t } = useLocale()
  const ta = t.agents

  // data
  const [agents, setAgents] = useState<AgentView[]>([])
  const [credentials, setCredentials] = useState<LlmCredential[]>([])
  const [catalogGroups, setCatalogGroups] = useState<ToolCatalogGroup[]>([])
  const [loading, setLoading] = useState(false)

  // detail view
  const [selected, setSelected] = useState<AgentView | null>(null)
  const [tab, setTab] = useState<DetailTab>('overview')
  const [form, setForm] = useState<DetailForm | null>(null)
  const [saving, setSaving] = useState(false)

  // create modal
  const [createOpen, setCreateOpen] = useState(false)
  const [createName, setCreateName] = useState('')
  const [createModel, setCreateModel] = useState('')
  const [creating, setCreating] = useState(false)

  // delete
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleting, setDeleting] = useState(false)

  // -- load --

  const load = useCallback(async (): Promise<AgentView[]> => {
    setLoading(true)
    try {
      const [personas, configs, creds, catalogResp] = await Promise.all([
        listPersonas(accessToken),
        listAgentConfigs(accessToken),
        listLlmCredentials(accessToken),
        listToolCatalog(accessToken),
      ])

      const personaMap = new Map(personas.map((p) => [p.id, p]))
      const joined: AgentView[] = configs.map((cfg) => ({
        persona: cfg.persona_id ? personaMap.get(cfg.persona_id) ?? null : null,
        config: cfg,
      }))

      setAgents(joined)
      setCredentials(creds)
      setCatalogGroups(catalogResp.groups)
      return joined
    } catch (err) {
      addToast(isApiError(err) ? err.message : t.requestFailed, 'error')
      return []
    } finally {
      setLoading(false)
    }
  }, [accessToken, addToast, t.requestFailed])

  useEffect(() => { void load() }, [load])

  // -- navigation --

  const selectAgent = useCallback((agent: AgentView) => {
    setSelected(agent)
    setForm(agentToForm(agent))
    setTab('overview')
  }, [])

  const goBack = useCallback(() => {
    setSelected(null)
    setForm(null)
  }, [])

  const allCatalogToolNames = useMemo(
    () => catalogGroups.flatMap((g) => g.tools.map((t) => t.name)),
    [catalogGroups],
  )

  // -- create --

  const handleCreate = useCallback(async () => {
    if (!createName.trim() || !createModel.trim()) return
    setCreating(true)
    try {
      const defaultTools = allCatalogToolNames

      const persona = await createPersona({
        persona_key: `${slugify(createName)}-${Date.now()}`,
        version: '1.0',
        display_name: createName.trim(),
        prompt_md: createName.trim(),
        preferred_credential: createModel.trim(),
        executor_type: 'agent.simple',
        tool_allowlist: defaultTools,
      }, accessToken)

      const config = await createAgentConfig({
        scope: 'platform',
        name: createName.trim(),
        model: createModel.trim(),
        persona_id: persona.id,
        tool_policy: 'allowlist',
        tool_allowlist: defaultTools,
        prompt_cache_control: 'none',
        reasoning_mode: 'disabled',
        content_filter_level: '',
      }, accessToken)

      setCreateOpen(false)
      setCreateName('')
      setCreateModel('')
      void load()
      selectAgent({ persona, config })
    } catch (err) {
      addToast(isApiError(err) ? err.message : t.requestFailed, 'error')
    } finally {
      setCreating(false)
    }
  }, [createName, createModel, accessToken, addToast, t.requestFailed, load, selectAgent, allCatalogToolNames])

  // -- save --

  const handleSave = useCallback(async () => {
    if (!selected || !form || !form.name.trim()) return
    setSaving(true)
    try {
      await updateAgentConfig(selected.config.id, {
        name: form.name.trim(),
        model: form.model.trim() || undefined,
        is_default: form.isDefault,
        temperature: form.temperature,
        max_output_tokens: form.maxOutputTokens ? Number(form.maxOutputTokens) : undefined,
        reasoning_mode: form.reasoningMode,
        tool_policy: form.tools.length > 0 ? 'allowlist' : 'none',
        tool_allowlist: form.tools,
        system_prompt_override: form.systemPrompt.trim(),
      }, accessToken)

      if (selected.persona) {
        await patchPersona(selected.persona.id, {
          display_name: form.name.trim(),
          prompt_md: form.systemPrompt.trim() || undefined,
          tool_allowlist: form.tools,
          is_active: form.isActive,
          preferred_credential: form.model.trim() || undefined,
        }, accessToken)
      }

      const fresh = await load()
      const updated = fresh.find((a) => a.config.id === selected.config.id)
      if (updated) {
        setSelected(updated)
        setForm(agentToForm(updated))
      }
    } catch (err) {
      addToast(isApiError(err) ? err.message : t.requestFailed, 'error')
    } finally {
      setSaving(false)
    }
  }, [selected, form, accessToken, addToast, t.requestFailed, load])

  // -- delete --

  const handleDelete = useCallback(async () => {
    if (!selected) return
    setDeleting(true)
    try {
      await deleteAgentConfig(selected.config.id, accessToken)
      if (selected.persona) {
        await patchPersona(selected.persona.id, { is_active: false }, accessToken)
      }
      setDeleteOpen(false)
      goBack()
      void load()
    } catch (err) {
      addToast(isApiError(err) ? err.message : t.requestFailed, 'error')
    } finally {
      setDeleting(false)
    }
  }, [selected, accessToken, addToast, t.requestFailed, goBack, load])

  // -- tool toggle --

  const toggleTool = useCallback((key: string) => {
    setForm((prev) =>
      prev
        ? {
            ...prev,
            tools: prev.tools.includes(key)
              ? prev.tools.filter((k) => k !== key)
              : [...prev.tools, key],
          }
        : prev,
    )
  }, [])

  // -- derived --

  const sortedAgents = useMemo(
    () =>
      [...agents].sort((a, b) => {
        if (a.config.is_default !== b.config.is_default) return a.config.is_default ? -1 : 1
        return agentName(a).localeCompare(agentName(b))
      }),
    [agents],
  )

  const personaConfigJson = useMemo(
    () => (selected && form) ? buildPersonaConfig(selected.persona, form) : '',
    [selected, form],
  )

  // ============================================================
  //  DETAIL VIEW
  // ============================================================

  if (selected && form) {
    const tabs: { key: DetailTab; label: string }[] = [
      { key: 'overview', label: ta.overview },
      { key: 'persona', label: ta.persona },
      { key: 'tools', label: ta.tools },
    ]

    return (
      <div className="flex h-full flex-col overflow-hidden">
        <PageHeader
          title={
            <div className="flex items-center gap-2">
              <button
                onClick={goBack}
                className="flex items-center text-[var(--c-text-tertiary)] transition-colors hover:text-[var(--c-text-secondary)]"
              >
                <ChevronLeft size={16} />
              </button>
              <span>{agentName(selected)}</span>
              {selected.config.is_default && (
                <span className="rounded bg-[var(--c-bg-tag)] px-1.5 py-0.5 text-[10px] font-medium text-[var(--c-text-muted)]">
                  {t.common.default}
                </span>
              )}
              {selected.persona?.is_active && (
                <span className="rounded bg-emerald-500/10 px-1.5 py-0.5 text-[10px] font-medium text-emerald-500">
                  {ta.active}
                </span>
              )}
            </div>
          }
          actions={
            <div className="flex items-center gap-2">
              <button
                onClick={() => setDeleteOpen(true)}
                className="flex items-center gap-1 rounded-lg px-2.5 py-1.5 text-xs text-[var(--c-text-tertiary)] transition-colors hover:bg-[var(--c-bg-sub)] hover:text-red-500"
              >
                <Trash2 size={13} />
                {t.common.delete}
              </button>
              <button
                onClick={handleSave}
                disabled={saving || !form.name.trim()}
                className="rounded-lg bg-[var(--c-accent)] px-3.5 py-1.5 text-xs font-medium text-white transition-colors hover:opacity-90 disabled:opacity-50"
              >
                {saving ? '...' : t.common.save}
              </button>
            </div>
          }
        />

        <div className="flex flex-1 overflow-hidden">
          {/* inner sidebar */}
          <nav className="w-[160px] shrink-0 overflow-y-auto border-r border-[var(--c-border-console)] p-2">
            <div className="flex flex-col gap-[3px]">
              {tabs.map((item) => (
                <button
                  key={item.key}
                  onClick={() => setTab(item.key)}
                  className={[
                    'w-full rounded-[5px] px-3 py-[7px] text-left text-sm font-medium transition-colors',
                    tab === item.key
                      ? 'bg-[var(--c-bg-sub)] text-[var(--c-text-primary)]'
                      : 'text-[var(--c-text-tertiary)] hover:bg-[var(--c-bg-sub)] hover:text-[var(--c-text-secondary)]',
                  ].join(' ')}
                >
                  {item.label}
                </button>
              ))}
            </div>
          </nav>

          {/* content -- left-aligned */}
          <div className="flex-1 overflow-auto p-6">
            <div className="flex max-w-[640px] flex-col gap-5">

              {/* -- Overview tab -- */}
              {tab === 'overview' && (
                <>
                  <FormField label={`${ta.name} *`}>
                    <input
                      className={INPUT_CLS}
                      value={form.name}
                      onChange={(e) => setForm((f) => f && { ...f, name: e.target.value })}
                    />
                  </FormField>

                  <FormField label={ta.model}>
                    <select
                      className={SELECT_CLS}
                      value={form.model}
                      onChange={(e) => setForm((f) => f && { ...f, model: e.target.value })}
                    >
                      <option value="" />
                      {credentials.map((c) => (
                        <option key={c.id} value={c.name}>{c.name}</option>
                      ))}
                    </select>
                  </FormField>

                  <div className="flex flex-col gap-3">
                    <CheckboxField
                      checked={form.isDefault}
                      onChange={(v) => setForm((f) => f && { ...f, isDefault: v })}
                      label={ta.setDefault}
                    />
                    {selected.persona && (
                      <CheckboxField
                        checked={form.isActive}
                        onChange={(v) => setForm((f) => f && { ...f, isActive: v })}
                        label={ta.active}
                      />
                    )}
                  </div>

                  <FormField label={ta.temperature}>
                    <div className="flex items-center gap-3">
                      <input
                        type="range"
                        min={0}
                        max={2}
                        step={0.1}
                        value={form.temperature}
                        onChange={(e) => setForm((f) => f && { ...f, temperature: Number(e.target.value) })}
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
                      onChange={(e) => setForm((f) => f && { ...f, maxOutputTokens: e.target.value })}
                    />
                  </FormField>

                  <FormField label={ta.reasoningMode}>
                    <select
                      className={SELECT_CLS}
                      value={form.reasoningMode}
                      onChange={(e) => setForm((f) => f && { ...f, reasoningMode: e.target.value })}
                    >
                      <option value="disabled">{ta.reasoningDisabled}</option>
                      <option value="enabled">{ta.reasoningEnabled}</option>
                    </select>
                  </FormField>
                </>
              )}

              {/* -- Persona tab -- */}
              {tab === 'persona' && (
                <>
                  <FormField label="prompt.md">
                    <textarea
                      className={`${MONO_CLS} min-h-[240px] resize-y`}
                      rows={10}
                      value={form.systemPrompt}
                      onChange={(e) => setForm((f) => f && { ...f, systemPrompt: e.target.value })}
                    />
                  </FormField>

                  <FormField label="persona.yaml">
                    <textarea
                      className={`${MONO_CLS} min-h-[180px] resize-y`}
                      rows={8}
                      value={personaConfigJson}
                      readOnly
                    />
                  </FormField>
                </>
              )}

              {/* -- Tools tab -- */}
              {tab === 'tools' && (
                <>
                  {catalogGroups.length > 0 ? (
                    <div className="flex flex-col gap-4">
                      {catalogGroups.map((group) => (
                        <div key={group.group} className="flex flex-col gap-2">
                          <span className="text-xs font-medium uppercase tracking-wide text-[var(--c-text-muted)]">
                            {group.group}
                          </span>
                          {group.tools.map((tool) => (
                            <CheckboxField
                              key={tool.name}
                              checked={form.tools.includes(tool.name)}
                              onChange={() => toggleTool(tool.name)}
                              label={tool.name}
                            />
                          ))}
                        </div>
                      ))}
                    </div>
                  ) : (
                    <p className="text-sm text-[var(--c-text-muted)]">--</p>
                  )}
                </>
              )}
            </div>
          </div>
        </div>

        <ConfirmDialog
          open={deleteOpen}
          onClose={() => setDeleteOpen(false)}
          onConfirm={handleDelete}
          message={ta.deleteConfirm}
          confirmLabel={t.common.delete}
          loading={deleting}
        />
      </div>
    )
  }

  // ============================================================
  //  LIST VIEW
  // ============================================================

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <PageHeader
        title={ta.title}
        actions={
          <button
            onClick={() => { setCreateOpen(true); setCreateName(''); setCreateModel('') }}
            className="flex items-center gap-1.5 rounded-lg bg-[var(--c-bg-tag)] px-3 py-1.5 text-xs font-medium text-[var(--c-text-secondary)] transition-colors hover:bg-[var(--c-bg-sub)]"
          >
            <Plus size={13} />
            {ta.newAgent}
          </button>
        }
      />

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
              <button
                key={agent.config.id}
                onClick={() => selectAgent(agent)}
                className="flex flex-col gap-3 rounded-xl border border-[var(--c-border)] bg-[var(--c-bg-sub)] px-5 py-4 text-left transition-colors hover:border-[var(--c-border-focus)]"
              >
                <div className="flex items-start justify-between gap-2">
                  <h3 className="text-sm font-medium text-[var(--c-text-primary)]">
                    {agentName(agent)}
                  </h3>
                  <div className="flex shrink-0 items-center gap-1.5">
                    {agent.config.is_default && (
                      <span className="rounded bg-[var(--c-bg-tag)] px-1.5 py-0.5 text-[10px] font-medium text-[var(--c-text-muted)]">
                        {t.common.default}
                      </span>
                    )}
                    {agent.persona?.is_active && (
                      <span className="rounded bg-emerald-500/10 px-1.5 py-0.5 text-[10px] font-medium text-emerald-500">
                        {ta.active}
                      </span>
                    )}
                  </div>
                </div>
                <div className="text-xs text-[var(--c-text-muted)]">
                  {ta.model}: {agent.config.model || '-'}
                </div>
              </button>
            ))}
          </div>
        )}
      </div>

      {/* create modal */}
      <Modal open={createOpen} onClose={() => setCreateOpen(false)} title={ta.newAgent} width="420px">
        <div className="flex flex-col gap-4">
          <FormField label={`${ta.name} *`}>
            <input
              className={INPUT_CLS}
              value={createName}
              onChange={(e) => setCreateName(e.target.value)}
              autoFocus
            />
          </FormField>
          <FormField label={`${ta.model} *`}>
            <select
              className={SELECT_CLS}
              value={createModel}
              onChange={(e) => setCreateModel(e.target.value)}
            >
              <option value="" />
              {credentials.map((c) => (
                <option key={c.id} value={c.name}>{c.name}</option>
              ))}
            </select>
          </FormField>
          <div className="flex justify-end gap-2 pt-2">
            <button
              onClick={() => setCreateOpen(false)}
              className="rounded-lg border border-[var(--c-border)] px-3.5 py-1.5 text-sm text-[var(--c-text-secondary)] transition-colors hover:bg-[var(--c-bg-sub)]"
            >
              {t.common.cancel}
            </button>
            <button
              onClick={handleCreate}
              disabled={creating || !createName.trim() || !createModel.trim()}
              className="rounded-lg bg-[var(--c-accent)] px-3.5 py-1.5 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-50"
            >
              {creating ? '...' : t.common.save}
            </button>
          </div>
        </div>
      </Modal>
    </div>
  )
}
