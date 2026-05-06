import { useState, useCallback, useEffect, useMemo, useRef, memo, type ReactNode } from 'react'
import {
  Plus,
  Trash2,
  Download,
  X,
  Loader2,
  Zap,
  SlidersHorizontal,
} from 'lucide-react'
import {
  type LlmProvider,
  type LlmProviderModel,
  type AvailableModel,
  listLlmProviders,
  createLlmProvider,
  updateLlmProvider,
  deleteLlmProvider,
  createProviderModel,
  deleteProviderModel,
  patchProviderModel,
  listAvailableModels,
  testLlmProviderModel,
  isApiError,
} from '../../api'
import { routeAdvancedJsonFromAvailableCatalog } from '@arkloop/shared/llm/available-catalog-advanced-json'
import { ConfirmDialog } from '@arkloop/shared'
import { useLocale } from '../../contexts/LocaleContext'
import { ModelOptionsModal } from '../ModelOptionsModal'
import { AnimatedCheck } from '../AnimatedCheck'
import { SettingsButton, SettingsIconButton } from './_SettingsButton'
import { SettingsInput, SettingsSearchInput } from './_SettingsInput'
import { SettingsModalFrame } from './_SettingsModalFrame'
import { SettingsSelect } from './_SettingsSelect'
import { SettingsSwitch } from './_SettingsSwitch'

const VENDOR_PRESETS = [
  { key: 'openai_responses', provider: 'openai', openai_api_mode: 'responses' },
  { key: 'openai_chat_completions', provider: 'openai', openai_api_mode: 'chat_completions' },
  { key: 'anthropic_message', provider: 'anthropic', openai_api_mode: undefined },
  { key: 'gemini', provider: 'gemini', openai_api_mode: undefined },
] as const

type VendorPresetKey = (typeof VENDOR_PRESETS)[number]['key']

const OPENVIKING_BACKEND_ADVANCED_KEY = 'openviking_backend'

type OpenVikingBackendKey = 'openai' | 'azure' | 'volcengine' | 'openai_compatible'

function isManagedLocalProvider(provider: LlmProvider): boolean {
  return provider.source === 'local' || provider.read_only === true
}

function localAuthModeLabel(provider: LlmProvider, p: ReturnType<typeof useLocale>['t']['adminProviders']): string | null {
  if (provider.auth_mode === 'api_key') return p.apiKey
  if (provider.auth_mode === 'oauth') return p.authModeOAuth
  return null
}

function vendorLabel(
  key: string,
  p: { vendorOpenai: string; vendorOpenaiChat: string; vendorAnthropic: string; vendorGemini: string },
): string {
  const map: Record<string, string> = {
    openai_responses: p.vendorOpenai,
    openai_chat_completions: p.vendorOpenaiChat,
    anthropic_message: p.vendorAnthropic,
    gemini: p.vendorGemini,
  }
  return map[key] ?? key
}

function toVendorKey(provider: string, mode: string | null): VendorPresetKey {
  if (provider === 'anthropic') return 'anthropic_message'
  if (provider === 'gemini') return 'gemini'
  if (mode === 'chat_completions') return 'openai_chat_completions'
  return 'openai_responses'
}

function defaultOpenVikingBackendForVendor(provider: string): OpenVikingBackendKey {
  if (provider === 'anthropic' || provider === 'gemini') return 'openai_compatible'
  return 'openai'
}

function readOpenVikingBackend(provider: LlmProvider): OpenVikingBackendKey {
  const raw = provider.advanced_json?.[OPENVIKING_BACKEND_ADVANCED_KEY]
  if (raw === 'openai' || raw === 'azure' || raw === 'volcengine' || raw === 'openai_compatible') {
    return raw
  }
  if (raw === 'litellm') {
    return 'openai_compatible'
  }
  return defaultOpenVikingBackendForVendor(provider.provider)
}

function mergeProviderAdvancedJSON(
  current: Record<string, unknown> | null | undefined,
  backend: OpenVikingBackendKey,
): Record<string, unknown> {
  const next = { ...(current ?? {}) }
  next[OPENVIKING_BACKEND_ADVANCED_KEY] = backend
  return next
}

type ProviderActionError = {
  message: string
  code?: string
  traceId?: string
  details?: unknown
}

class AvailableModelsLoadError extends Error {
  readonly displayError: ProviderActionError

  constructor(displayError: ProviderActionError) {
    super(displayError.message)
    this.name = 'AvailableModelsLoadError'
    this.displayError = displayError
  }
}

function providerActionErrorFromUnknown(error: unknown, fallback: string): ProviderActionError {
  if (isApiError(error)) {
    return {
      message: error.message || fallback,
      code: error.code,
      traceId: error.traceId,
      details: error.details,
    }
  }
  if (error instanceof Error) {
    return { message: error.message || fallback }
  }
  return { message: fallback }
}

function formatProviderDetail(value: unknown): string {
  if (value == null) return String(value)
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') return String(value)
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}

function formatProviderActionError(error: ProviderActionError): string {
  const lines = [error.message]
  if (error.code) lines.push(`code: ${error.code}`)
  if (error.traceId) lines.push(`trace_id: ${error.traceId}`)
  if (error.details && typeof error.details === 'object') {
    for (const [key, value] of Object.entries(error.details)) {
      lines.push(`${key}: ${formatProviderDetail(value)}`)
    }
  }
  return lines.join('\n')
}

function isAvailableModelsLoadError(error: unknown): error is AvailableModelsLoadError {
  return error instanceof AvailableModelsLoadError
}

function VendorDropdown({
  value,
  onChange,
  p,
  triggerClassName,
}: {
  value: VendorPresetKey
  onChange: (v: VendorPresetKey) => void
  p: ReturnType<typeof useLocale>['t']['adminProviders']
  triggerClassName?: string
}) {
  return (
    <SettingsSelect
      value={value}
      options={VENDOR_PRESETS.map((preset) => ({
        value: preset.key,
        label: vendorLabel(preset.key, p),
      }))}
      onChange={(next) => onChange(next as VendorPresetKey)}
      triggerClassName={triggerClassName}
    />
  )
}

type Props = { accessToken: string }

export function ProvidersSettings({ accessToken }: Props) {
  const { t } = useLocale()
  const p = t.adminProviders

  const [providers, setProviders] = useState<LlmProvider[]>([])
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [showAddProvider, setShowAddProvider] = useState(false)

  const firstLoadRef = useRef(true)

  const load = useCallback(async () => {
    try {
      const list = await listLlmProviders(accessToken)
      setProviders(list)
      if (firstLoadRef.current && list.length > 0) {
        setSelectedId(list[0].id)
        firstLoadRef.current = false
      } else {
        setSelectedId((prev) => list.find((pv) => pv.id === prev) ? prev : (list[0]?.id ?? null))
      }
    } catch {
      setError(p.loadFailed)
    } finally {
      setLoading(false)
    }
  }, [accessToken, p.loadFailed])

  useEffect(() => { void load() }, [load])

  const selected = providers.find((pv) => pv.id === selectedId) ?? null

  if (loading) {
    return (
      <div className="flex items-center justify-center py-16">
        <Loader2 size={18} className="animate-spin text-[var(--c-text-muted)]" />
      </div>
    )
  }

  return (
    <div className="-m-6 flex min-h-0 min-w-0 overflow-hidden" style={{ height: 'calc(100% + 48px)' }}>
      {/* Provider list */}
      <div className="flex w-[220px] shrink-0 flex-col overflow-hidden max-[1230px]:w-[180px] xl:w-[240px]" style={{ borderRight: '0.5px solid var(--c-border-subtle)' }}>
        <div className="flex-1 overflow-y-auto px-2 py-1">
          <div className="flex flex-col gap-[3px]">
            {providers.map((pv) => (
              <button
                key={pv.id}
                onClick={() => setSelectedId(pv.id)}
                className={[
                  'flex h-[38px] items-center gap-2 rounded-lg px-2.5 text-left text-[14px] font-medium transition-all duration-[120ms] active:scale-[0.96]',
                  selectedId === pv.id
                    ? 'rounded-[10px] bg-[var(--c-bg-deep)] text-[var(--c-text-heading)]'
                    : 'text-[var(--c-text-secondary)] hover:bg-[var(--c-bg-deep)] hover:text-[var(--c-text-heading)]',
                ].join(' ')}
              >
                <span className="min-w-0 flex-1 truncate">{pv.name}</span>
                {isManagedLocalProvider(pv) && (
                  <span className="shrink-0 rounded-md px-1.5 py-0.5 text-[10px] font-medium text-[var(--c-text-muted)]" style={{ background: 'var(--c-bg-sub)' }}>
                    {p.localProvider}
                  </span>
                )}
              </button>
            ))}
          </div>
        </div>
        <div className="border-t border-[var(--c-border-subtle)] px-3 py-3">
          <SettingsButton
            variant="secondary"
            onClick={() => setShowAddProvider(true)}
            className="w-full"
            icon={<Plus size={14} />}
          >
            {p.addProvider}
          </SettingsButton>
        </div>
        {error && <p className="px-2 pb-2 text-xs text-[var(--c-status-error-text)]">{error}</p>}
      </div>

      {/* Detail */}
      <div className="min-w-0 flex-1 overflow-y-auto p-4 max-[1230px]:p-3 sm:p-5">
        {selected ? (
          <ProviderDetail
            key={selected.id}
            provider={selected}
            accessToken={accessToken}
            onUpdated={load}
            onDeleted={load}
            p={p}
          />
        ) : (
          <div className="flex h-full flex-col items-center justify-center gap-3">
            <p className="text-sm text-[var(--c-text-muted)]">{p.noProviders}</p>
            <SettingsButton
              variant="primary"
              onClick={() => setShowAddProvider(true)}
              icon={<Plus size={14} />}
            >
              {p.addProvider}
            </SettingsButton>
          </div>
        )}
      </div>

      {showAddProvider && (
        <AddProviderModal
          accessToken={accessToken}
          p={p}
          onClose={() => setShowAddProvider(false)}
          onCreated={() => { setShowAddProvider(false); void load() }}
        />
      )}
    </div>
  )
}

// -- Add Provider Modal --

function AddProviderModal({ accessToken, p, onClose, onCreated }: {
  accessToken: string
  p: ReturnType<typeof useLocale>['t']['adminProviders']
  onClose: () => void
  onCreated: () => void
}) {
  const [name, setName] = useState('')
  const [preset, setPreset] = useState<VendorPresetKey>('openai_responses')
  const [apiKey, setApiKey] = useState('')
  const [baseUrl, setBaseUrl] = useState('')
  const [saving, setSaving] = useState(false)
  const [err, setErr] = useState('')

  const handleSave = async () => {
    if (!name.trim() || !apiKey.trim()) return
    setSaving(true)
    setErr('')
    try {
      const v = VENDOR_PRESETS.find((vv) => vv.key === preset)!
      await createLlmProvider(accessToken, {
        name: name.trim(),
        provider: v.provider,
        api_key: apiKey.trim(),
        base_url: baseUrl.trim() || undefined,
        openai_api_mode: v.openai_api_mode,
        advanced_json: mergeProviderAdvancedJSON({}, defaultOpenVikingBackendForVendor(v.provider)),
      })
      onCreated()
    } catch (e) {
      setErr(isApiError(e) ? e.message : p.saveFailed)
    } finally {
      setSaving(false)
    }
  }

  const fieldLabelCls = 'block text-[11px] font-medium text-[var(--c-placeholder)] mb-1 pl-[2px]'

  return (
    <SettingsModalFrame
      open
      title={p.addProvider}
      onClose={onClose}
      width={510}
      footer={(
        <>
          <SettingsButton size="modal" variant="secondary" onClick={onClose}>
            {p.cancel}
          </SettingsButton>
          <SettingsButton
            size="modal"
            variant="primary"
            onClick={() => void handleSave()}
            disabled={saving || !name.trim() || !apiKey.trim()}
            icon={saving ? <Loader2 size={14} className="animate-spin" /> : undefined}
          >
            {saving ? p.saving : p.save}
          </SettingsButton>
        </>
      )}
    >
        <div className="mt-6 grid grid-cols-2 gap-x-4 gap-y-4">
          <div>
            <label className={fieldLabelCls}>{p.providerName}</label>
            <SettingsInput
              variant="md"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="My Provider"
            />
          </div>
          <div>
            <label className={fieldLabelCls}>{p.vendor}</label>
            <VendorDropdown value={preset} onChange={setPreset} p={p} triggerClassName="h-[35px]" />
          </div>
          <div className="col-span-2">
            <label className={fieldLabelCls}>{p.apiKey}</label>
            <SettingsInput
              variant="md"
              type="password"
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              placeholder={p.apiKeyPlaceholder}
            />
          </div>
          <div className="col-span-2">
            <label className={fieldLabelCls}>{p.baseUrl}</label>
            <SettingsInput
              variant="md"
              value={baseUrl}
              onChange={(e) => setBaseUrl(e.target.value.slice(0, 500))}
              placeholder={p.baseUrlPlaceholder ?? 'https://api.example.com/v1'}
              maxLength={500}
            />
          </div>
        </div>

        {err && <p className="mt-3 text-xs text-[var(--c-status-error-text)]">{err}</p>}
    </SettingsModalFrame>
  )
}

// -- Provider Detail --

function ProviderDetail({ provider, accessToken, onUpdated, onDeleted, p }: {
  provider: LlmProvider
  accessToken: string
  onUpdated: () => void
  onDeleted: () => void
  p: ReturnType<typeof useLocale>['t']['adminProviders']
}) {
  const [formPreset, setFormPreset] = useState<VendorPresetKey>(toVendorKey(provider.provider, provider.openai_api_mode))
  const [formName, setFormName] = useState(provider.name)
  const [formApiKey, setFormApiKey] = useState('')
  const [formBaseUrl, setFormBaseUrl] = useState(provider.base_url ?? '')
  const [saving, setSaving] = useState(false)
  const [err, setErr] = useState('')
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const readOnly = isManagedLocalProvider(provider)

  useEffect(() => {
    setFormPreset(toVendorKey(provider.provider, provider.openai_api_mode))
    setFormName(provider.name)
    setFormApiKey('')
    setFormBaseUrl(provider.base_url ?? '')
    setErr('')
    setConfirmDelete(false)
  }, [provider.base_url, provider.id, provider.name, provider.openai_api_mode, provider.provider])

  if (readOnly) {
    const authModeLabel = localAuthModeLabel(provider, p)
    return (
      <div className="mx-auto min-w-0 max-w-2xl space-y-6">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--c-border-subtle)] pb-4">
          <h3 className="text-base font-semibold text-[var(--c-text-primary)]">{provider.name}</h3>
          <div className="flex flex-wrap items-center gap-1.5">
            <span className="rounded-md px-2 py-1 text-xs font-medium text-[var(--c-text-muted)]" style={{ background: 'var(--c-bg-sub)' }}>
              {p.localProvider}
            </span>
            <span className="rounded-md px-2 py-1 text-xs font-medium text-[var(--c-text-muted)]" style={{ background: 'var(--c-bg-sub)' }}>
              {p.readOnlyProvider}
            </span>
            {authModeLabel && (
              <span className="rounded-md px-2 py-1 text-xs font-medium text-[var(--c-text-muted)]" style={{ background: 'var(--c-bg-sub)' }}>
                {authModeLabel}
              </span>
            )}
          </div>
        </div>
        <ModelsSection provider={provider} accessToken={accessToken} onChanged={onUpdated} p={p} readOnly />
      </div>
    )
  }

  const handleSave = async () => {
    setSaving(true)
    setErr('')
    try {
      const selected = VENDOR_PRESETS.find((v) => v.key === formPreset)
      await updateLlmProvider(accessToken, provider.id, {
        name: formName.trim() || undefined,
        api_key: formApiKey.trim() || undefined,
        base_url: formBaseUrl.trim() || null,
        provider: selected?.provider,
        openai_api_mode: selected?.openai_api_mode ?? null,
        advanced_json: mergeProviderAdvancedJSON(provider.advanced_json, readOpenVikingBackend(provider)),
      })
      setFormApiKey('')
      onUpdated()
    } catch (e) {
      setErr(isApiError(e) ? e.message : p.saveFailed)
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async () => {
    setDeleting(true)
    try {
      await deleteLlmProvider(accessToken, provider.id)
      onDeleted()
    } catch (e) {
      setErr(isApiError(e) ? e.message : p.saveFailed)
      setDeleting(false)
      setConfirmDelete(false)
    }
  }

  return (
    <div className="mx-auto min-w-0 max-w-2xl space-y-6">
      <h3 className="text-base font-semibold text-[var(--c-text-primary)]">{provider.name}</h3>

      <div className="space-y-4">
        <LabelField label={p.vendor}>
          <VendorDropdown value={formPreset} onChange={setFormPreset} p={p} />
        </LabelField>
        <LabelField label={p.providerName}>
          <SettingsInput value={formName} onChange={(e) => setFormName(e.target.value)} />
        </LabelField>
        <LabelField label={p.apiKey}>
          <SettingsInput
            type="password"
            value={formApiKey}
            onChange={(e) => setFormApiKey(e.target.value)}
            placeholder={provider.key_prefix ? `${provider.key_prefix}${'*'.repeat(40)}` : p.apiKeyPlaceholder}
          />
          {provider.key_prefix && <p className="mt-1 text-xs text-[var(--c-text-muted)]">{provider.key_prefix}{'*'.repeat(8)}</p>}
        </LabelField>
        <LabelField label={p.baseUrl}>
          <SettingsInput
            value={formBaseUrl}
            onChange={(e) => setFormBaseUrl(e.target.value.slice(0, 500))}
            placeholder={p.baseUrlPlaceholder ?? 'https://api.example.com/v1'}
            maxLength={500}
          />
        </LabelField>
      </div>

      {err && <p className="text-xs text-[var(--c-status-error-text)]">{err}</p>}

      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--c-border-subtle)] pb-4">
        {confirmDelete ? (
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-xs text-[var(--c-text-tertiary)]">{p.deleteProviderConfirm}</span>
            <SettingsButton
              variant="danger"
              onClick={() => void handleDelete()}
              disabled={deleting}
              icon={deleting ? <Loader2 size={14} className="animate-spin" /> : undefined}
            >
              {p.deleteProvider}
            </SettingsButton>
            <SettingsButton variant="secondary" onClick={() => setConfirmDelete(false)}>
              {p.cancel}
            </SettingsButton>
          </div>
        ) : (
          <SettingsIconButton label={p.deleteProvider} danger onClick={() => setConfirmDelete(true)}>
            <Trash2 size={12} />
          </SettingsIconButton>
        )}
        <SettingsButton
          variant="primary"
          onClick={() => void handleSave()}
          disabled={saving || !formName.trim()}
          icon={saving ? <Loader2 size={14} className="animate-spin" /> : undefined}
        >
          {saving ? p.saving : p.save}
        </SettingsButton>
      </div>

      <ModelsSection provider={provider} accessToken={accessToken} onChanged={onUpdated} p={p} />
    </div>
  )
}

// -- Models Section (same pattern as ModelConfigContent) --

function ModelsSection({ provider, accessToken, onChanged, p, readOnly = false }: {
  provider: LlmProvider
  accessToken: string
  onChanged: () => void
  p: ReturnType<typeof useLocale>['t']['adminProviders']
  readOnly?: boolean
}) {
  const { t } = useLocale()
  const [available, setAvailable] = useState<AvailableModel[] | null>(null)
  const [loadingAvailable, setLoadingAvailable] = useState(false)
  const [availableError, setAvailableError] = useState<ProviderActionError | null>(null)
  const [importing, setImporting] = useState(false)
  const [deletingAll, setDeletingAll] = useState(false)
  const [creatingModel, setCreatingModel] = useState(false)
  const [actionError, setActionError] = useState<ProviderActionError | null>(null)
  const [search, setSearch] = useState('')
  const [editingModel, setEditingModel] = useState<LlmProviderModel | null>(null)
  const [hasLoadedAvailable, setHasLoadedAvailable] = useState(false)
  const [showDeleteAllConfirm, setShowDeleteAllConfirm] = useState(false)

  useEffect(() => {
    setAvailable(null)
    setHasLoadedAvailable(false)
    setSearch('')
    setEditingModel(null)
    setCreatingModel(false)
    setActionError(null)
    setAvailableError(null)
    setShowDeleteAllConfirm(false)
  }, [provider.id])

  const loadAvailable = useCallback(async () => {
    setLoadingAvailable(true)
    setAvailableError(null)
    try {
      const res = await listAvailableModels(accessToken, provider.id)
      setAvailable(res.models)
      setHasLoadedAvailable(true)
    } catch (e) {
      setAvailableError(providerActionErrorFromUnknown(e, t.models.availableFetchFailed))
    } finally {
      setLoadingAvailable(false)
    }
  }, [accessToken, provider.id, t.models.availableFetchFailed])

  const ensureAvailableLoaded = useCallback(async (): Promise<AvailableModel[]> => {
    if (available !== null) return available
    setLoadingAvailable(true)
    setAvailableError(null)
    try {
      const res = await listAvailableModels(accessToken, provider.id)
      setAvailable(res.models)
      setHasLoadedAvailable(true)
      return res.models
    } catch (e) {
      const displayError = providerActionErrorFromUnknown(e, t.models.availableFetchFailed)
      setAvailableError(displayError)
      throw new AvailableModelsLoadError(displayError)
    } finally {
      setLoadingAvailable(false)
    }
  }, [accessToken, available, provider.id, t.models.availableFetchFailed])

  const handleImportAll = async () => {
    setImporting(true)
    setActionError(null)
    try {
      const source = await ensureAvailableLoaded()
      const unconfigured = source.filter((am) => !am.configured)
      const byId = new Map<string, AvailableModel>()
      for (const am of unconfigured) {
        if (!byId.has(am.id)) byId.set(am.id, am)
      }
      const toImport = [...byId.values()]
      const embeddingIds = new Set(toImport.filter((am) => am.type === 'embedding').map((am) => am.id))
      const created: LlmProviderModel[] = []
      for (const am of toImport) {
        const isEmb = am.type === 'embedding'
        try {
          const pm = await createProviderModel(accessToken, provider.id, {
            model: am.id,
            show_in_picker: false,
            tags: isEmb ? ['embedding'] : undefined,
            advanced_json: routeAdvancedJsonFromAvailableCatalog(am),
          })
          created.push(pm)
        } catch (e) {
          if (isApiError(e) && e.code === 'llm_provider_models.model_conflict') continue
          throw e
        }
      }
      const toEnable = created.filter((pm) => pm.model.toLowerCase().includes('gpt-4o-mini') && !embeddingIds.has(pm.model))
      if (toEnable.length > 0) {
        try {
          await patchProviderModel(accessToken, provider.id, toEnable[0].id, { show_in_picker: true, is_default: true })
          await Promise.all(toEnable.slice(1).map((pm) => patchProviderModel(accessToken, provider.id, pm.id, { show_in_picker: true })))
        } catch { /* default-setting is best-effort */ }
      }
      onChanged()
      await loadAvailable()
    } catch (e) {
      if (isAvailableModelsLoadError(e)) return
      setActionError(providerActionErrorFromUnknown(e, p.saveFailed))
    } finally {
      setImporting(false)
    }
  }

  const handleDeleteModel = useCallback(async (modelId: string) => {
    try {
      await deleteProviderModel(accessToken, provider.id, modelId)
      onChanged()
    } catch (e) {
      setActionError(providerActionErrorFromUnknown(e, p.saveFailed))
    }
  }, [accessToken, provider.id, onChanged, p.saveFailed])

  const handleDeleteAll = async () => {
    setDeletingAll(true)
    setActionError(null)
    let failed = 0
    let firstError: ProviderActionError | null = null
    for (const pm of provider.models) {
      try {
        await deleteProviderModel(accessToken, provider.id, pm.id)
      } catch (e) {
        failed++
        if (!firstError) firstError = providerActionErrorFromUnknown(e, p.saveFailed)
      }
    }
    setDeletingAll(false)
    if (failed > 0) setActionError(firstError ?? { message: p.saveFailed })
    onChanged()
    setAvailable(null)
    setHasLoadedAvailable(false)
  }

  const handleTogglePicker = useCallback(async (modelId: string, current: boolean) => {
    try {
      await patchProviderModel(accessToken, provider.id, modelId, { show_in_picker: !current })
      onChanged()
    } catch (e) {
      setActionError(providerActionErrorFromUnknown(e, p.saveFailed))
    }
  }, [accessToken, provider.id, onChanged, p.saveFailed])

  const handleSaveModelOptions = useCallback(async (payload: {
    advancedJSON: Record<string, unknown> | null
    tags: string[]
  }) => {
    if (!editingModel) return
    try {
      await patchProviderModel(accessToken, provider.id, editingModel.id, {
        advanced_json: payload.advancedJSON,
        tags: payload.tags,
      })
      setEditingModel(null)
      onChanged()
    } catch (e) {
      throw new Error(isApiError(e) ? e.message : p.saveFailed)
    }
  }, [accessToken, editingModel, onChanged, p.saveFailed, provider.id])

  const unconfiguredCount = available?.filter((am) => !am.configured).length ?? 0
  const importDisabled = importing || loadingAvailable || (hasLoadedAvailable && unconfiguredCount === 0)
  const deleteAllDisabled = deletingAll || provider.models.length === 0
  const sectionError = availableError ?? actionError
  const importButtonLabel =
    unconfiguredCount > 0 && !importing && !loadingAvailable
      ? `${p.importAll ?? 'Import all'} (${unconfiguredCount})`
      : (loadingAvailable || importing ? (p.importing ?? '...') : '')
  const filteredModels = search.trim()
    ? provider.models.filter((pm) => pm.model.toLowerCase().includes(search.trim().toLowerCase()))
    : provider.models

  const INITIAL_BATCH = 30
  const BATCH_SIZE = 100

  const [visibleCount, setVisibleCount] = useState(INITIAL_BATCH)

  // filteredModels 变化时重置
  useEffect(() => {
    setVisibleCount(INITIAL_BATCH)
  }, [filteredModels.length, search])

  // 逐帧追加
  useEffect(() => {
    if (visibleCount >= filteredModels.length) return
    const id = requestAnimationFrame(() => {
      setVisibleCount((prev) => Math.min(prev + BATCH_SIZE, filteredModels.length))
    })
    return () => cancelAnimationFrame(id)
  }, [visibleCount, filteredModels.length])

  const visibleModels = filteredModels.slice(0, visibleCount)

  return (
    <div>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h4 className="text-sm font-medium text-[var(--c-text-primary)]">{p.modelsSection}</h4>
        {!readOnly && (
          <div className="flex flex-wrap items-center gap-2">
            <SettingsIconButton
              label={p.deleteAll ?? 'Delete all'}
              danger
              onClick={() => setShowDeleteAllConfirm(true)}
              disabled={deleteAllDisabled}
            >
              {deletingAll ? <Loader2 size={12} className="animate-spin" /> : <Trash2 size={12} />}
            </SettingsIconButton>
            <SettingsButton
              variant="secondary"
              onClick={() => void handleImportAll()}
              disabled={importDisabled}
              className={importButtonLabel || availableError ? undefined : 'w-[32px] px-0'}
              icon={loadingAvailable || importing
                ? <Loader2 size={12} className="animate-spin" />
                : (
                    <>
                      {availableError && <X size={12} className="text-[var(--c-status-error-text)]" />}
                      <Download
                        size={12}
                        className={availableError ? 'text-[var(--c-status-error-text)]' : undefined}
                      />
                    </>
                  )}
            >
              {importButtonLabel}
            </SettingsButton>
            {sectionError && <ErrorDetailsButton error={sectionError} />}
            <ModelTestButton
              accessToken={accessToken}
              provider={provider}
              label={p.testModel ?? 'Test'}
              searchPlaceholder={p.searchProviders}
            />
            <SettingsButton variant="primary" onClick={() => setCreatingModel(true)}>
              {p.addModel}
            </SettingsButton>
          </div>
        )}
      </div>

      {!readOnly && hasLoadedAvailable && !loadingAvailable && !availableError && available !== null && available.length === 0 && (
        <p className="mt-2 text-xs text-[var(--c-text-muted)]">{t.models.noModelsAvailable}</p>
      )}

      {provider.models.length > 0 && (
        <div className="mt-3">
          <SettingsSearchInput value={search} onChange={(e) => setSearch(e.target.value)} placeholder={p.searchProviders} />
        </div>
      )}

      <div className="mt-2 space-y-1 overflow-y-auto" style={{ maxHeight: '320px' }}>
        {provider.models.length === 0 ? (
          <p className="py-8 text-center text-sm text-[var(--c-text-muted)]">--</p>
        ) : filteredModels.length === 0 ? (
          <p className="py-4 text-center text-sm text-[var(--c-text-muted)]">--</p>
        ) : (
          visibleModels.map((pm) => (
            <ModelRow
              key={pm.id}
              pm={pm}
              onToggle={handleTogglePicker}
              onEdit={setEditingModel}
              onDelete={handleDeleteModel}
              readOnly={readOnly}
            />
          ))
        )}
      </div>

      {!readOnly && editingModel !== null && (
      <ModelOptionsModal
        open
        model={editingModel}
        availableModels={available}
        labels={{
          modelOptionsTitle: p.modelOptionsTitle ?? 'Model Options',
          modelOptionsFor: p.modelOptionsFor ?? 'Configure options for',
          modelCapabilities: p.modelCapabilities ?? 'Model Capabilities',
          modelType: p.modelType ?? 'Model Type',
          modelTypeChat: p.modelTypeChat ?? 'Chat',
          modelTypeEmbedding: p.modelTypeEmbedding ?? 'Embedding',
          modelTypeImage: p.modelTypeImage ?? 'Image',
          modelTypeAudio: p.modelTypeAudio ?? 'Audio',
          modelTypeModeration: p.modelTypeModeration ?? 'Moderation',
          modelTypeOther: p.modelTypeOther ?? 'Other',
          toolCalling: p.toolCalling ?? 'Tool Calling',
          reasoning: p.reasoning ?? 'Reasoning',
          defaultTemperature: p.defaultTemperature ?? 'Default Temperature',
          vision: p.vision ?? 'Vision',
          imageOutput: p.imageOutput ?? 'Image Output',
          embedding: p.embedding ?? 'Embedding',
          contextWindow: p.contextWindow ?? 'Context Window',
          maxOutputTokens: p.maxOutputTokens ?? 'Max Output Tokens',
          providerOptionsJson: p.providerOptionsJson ?? 'Provider Options (JSON)',
          providerOptionsHint: p.providerOptionsHint ?? 'Only provider-specific fields belong here. Model capability fields are managed above.',
          save: p.save,
          cancel: p.cancel,
          reset: p.reset ?? 'Reset',
          invalidJson: p.invalidJson ?? 'Provider options must be a JSON object',
          invalidNumber: p.invalidNumber ?? 'Context window, max output tokens, and temperature must be valid numbers',
          visionBridgeHint: t.models.visionBridgeHint,
          addModelTitle: t.models.addModelTitle ?? 'Add Model',
          modelNameLabel: t.models.modelName ?? 'Model name',
          modelNamePlaceholder: t.models.modelNamePlaceholder ?? 'e.g. gpt-4o',
        }}
        onClose={() => setEditingModel(null)}
        onSave={handleSaveModelOptions}
      />
      )}

      {!readOnly && creatingModel && (
      <ModelOptionsModal
        open
        mode="create"
        model={null}
        availableModels={available}
        labels={{
          modelOptionsTitle: p.modelOptionsTitle ?? 'Model Options',
          modelOptionsFor: p.modelOptionsFor ?? 'Configure options for',
          modelCapabilities: p.modelCapabilities ?? 'Model Capabilities',
          modelType: p.modelType ?? 'Model Type',
          modelTypeChat: p.modelTypeChat ?? 'Chat',
          modelTypeEmbedding: p.modelTypeEmbedding ?? 'Embedding',
          modelTypeImage: p.modelTypeImage ?? 'Image',
          modelTypeAudio: p.modelTypeAudio ?? 'Audio',
          modelTypeModeration: p.modelTypeModeration ?? 'Moderation',
          modelTypeOther: p.modelTypeOther ?? 'Other',
          toolCalling: p.toolCalling ?? 'Tool Calling',
          reasoning: p.reasoning ?? 'Reasoning',
          defaultTemperature: p.defaultTemperature ?? 'Default Temperature',
          vision: p.vision ?? 'Vision',
          imageOutput: p.imageOutput ?? 'Image Output',
          embedding: p.embedding ?? 'Embedding',
          contextWindow: p.contextWindow ?? 'Context Window',
          maxOutputTokens: p.maxOutputTokens ?? 'Max Output Tokens',
          providerOptionsJson: p.providerOptionsJson ?? 'Provider Options (JSON)',
          providerOptionsHint: p.providerOptionsHint ?? 'Only provider-specific fields belong here. Model capability fields are managed above.',
          save: p.save,
          cancel: p.cancel,
          reset: p.reset ?? 'Reset',
          invalidJson: p.invalidJson ?? 'Provider options must be a JSON object',
          invalidNumber: p.invalidNumber ?? 'Context window, max output tokens, and temperature must be valid numbers',
          visionBridgeHint: t.models.visionBridgeHint,
          addModelTitle: t.models.addModelTitle ?? 'Add Model',
          modelNameLabel: t.models.modelName ?? 'Model name',
          modelNamePlaceholder: t.models.modelNamePlaceholder ?? 'e.g. gpt-4o',
        }}
        onClose={() => setCreatingModel(false)}
        onSave={async () => {}}
        onCreate={async (payload) => {
          try {
            await createProviderModel(accessToken, provider.id, {
              model: payload.model,
              show_in_picker: false,
              tags: payload.tags.length > 0 ? payload.tags : undefined,
              advanced_json: payload.advancedJSON ?? undefined,
            })
            setCreatingModel(false)
            onChanged()
          } catch (e) {
            throw new Error(isApiError(e) ? e.message : p.saveFailed)
          }
        }}
      />
      )}

      {!readOnly && (
        <ConfirmDialog
          open={showDeleteAllConfirm}
          onClose={() => setShowDeleteAllConfirm(false)}
          onConfirm={() => {
            setShowDeleteAllConfirm(false)
            void handleDeleteAll()
          }}
          title={p.deleteAllConfirmTitle ?? 'Delete all models'}
          message={p.deleteAllConfirmDesc ?? 'This will remove every model under this provider. Continue?'}
          confirmLabel={p.deleteAll ?? 'Delete all'}
          loading={deletingAll}
        />
      )}
    </div>
  )
}

const ModelRow = memo(function ModelRow({ pm, onToggle, onEdit, onDelete, readOnly = false }: {
  pm: LlmProviderModel
  onToggle: (id: string, current: boolean) => void
  onEdit: (pm: LlmProviderModel) => void
  onDelete: (id: string) => void
  readOnly?: boolean
}) {
  return (
    <div
      className="group flex flex-wrap items-center justify-between gap-2 rounded-lg border border-[var(--c-border-subtle)] px-4 py-2.5"
      style={{ contentVisibility: 'auto', containIntrinsicBlockSize: '52px' }}
    >
      <div className="min-w-0 flex-1 flex items-center gap-1.5">
        <p className="truncate text-sm font-medium text-[var(--c-text-primary)]">{pm.model}</p>
        {pm.tags.includes('embedding') && (
          <span className="shrink-0 rounded-md px-2 py-0.5 text-xs font-medium" style={{ background: 'var(--c-bg-sub)', color: 'var(--c-text-muted)' }}>emb</span>
        )}
      </div>
      <div className="flex w-full shrink-0 items-center justify-end gap-1.5 sm:w-auto">
        {readOnly ? (
          <SettingsSwitch checked={pm.show_in_picker} onChange={() => onToggle(pm.id, pm.show_in_picker)} />
        ) : (
          <>
            <SettingsSwitch checked={pm.show_in_picker} onChange={() => onToggle(pm.id, pm.show_in_picker)} />
            <SettingsIconButton
              label="Edit model"
              onClick={() => onEdit(pm)}
            >
              <SlidersHorizontal size={14} />
            </SettingsIconButton>
            <SettingsIconButton
              label="Delete model"
              danger
              onClick={() => onDelete(pm.id)}
            >
              <Trash2 size={14} />
            </SettingsIconButton>
          </>
        )}
      </div>
    </div>
  )
})

function LabelField({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <label className="mb-1 block text-xs font-medium text-[var(--c-text-tertiary)]">{label}</label>
      {children}
    </div>
  )
}

function ErrorDetailsButton({ error }: { error: ProviderActionError }) {
  const [open, setOpen] = useState(false)

  return (
    <div className="relative">
      <SettingsButton
        variant="danger"
        onClick={() => setOpen((v) => !v)}
        style={{ color: 'var(--c-status-error-text)' }}
      >
        Error
      </SettingsButton>
      {open && (
        <>
          <div className="fixed inset-0 z-40" onClick={() => setOpen(false)} />
          <div
            className="dropdown-menu absolute right-0 top-[calc(100%+6px)] z-50 max-w-[360px] min-w-[240px]"
            style={{
              border: '0.5px solid var(--c-border-subtle)',
              borderRadius: '10px',
              padding: '12px',
              background: 'var(--c-bg-menu)',
              boxShadow: 'var(--c-dropdown-shadow)',
              maxHeight: '180px',
              overflowY: 'auto',
            }}
          >
            <pre className="whitespace-pre-wrap break-all text-xs text-[var(--c-text-secondary)]">{formatProviderActionError(error)}</pre>
          </div>
        </>
      )}
    </div>
  )
}

function ModelTestButton({ accessToken, provider, label, searchPlaceholder }: {
  accessToken: string
  provider: LlmProvider
  label: string
  searchPlaceholder: string
}) {
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const [testing, setTesting] = useState<string | null>(null)
  const [result, setResult] = useState<{ modelId: string; success: boolean; latency?: number; error?: ProviderActionError } | null>(null)

  const pickerModels = useMemo(
    () => provider.models.filter((m) => m.show_in_picker),
    [provider.models],
  )

  const filtered = useMemo(() => {
    if (!open) return []
    const q = search.trim().toLowerCase()
    return q ? pickerModels.filter((m) => m.model.toLowerCase().includes(q)) : pickerModels
  }, [open, search, pickerModels])

  const handleTest = async (model: LlmProviderModel) => {
    setTesting(model.id)
    setOpen(false)
    try {
      const res = await testLlmProviderModel(accessToken, provider.id, model.id)
      setResult({
        modelId: model.id,
        success: res.success,
        latency: res.latency_ms ?? undefined,
        error: res.error ? { message: res.error } : undefined,
      })
    } catch (e) {
      setResult({ modelId: model.id, success: false, error: providerActionErrorFromUnknown(e, 'Unknown error') })
    } finally {
      setTesting(null)
    }
  }

  return (
    <div className="relative flex items-center gap-2">
      <SettingsButton
        variant="secondary"
        onClick={() => {
          if (result?.success && !testing) { setResult(null); return }
          setOpen((prev) => { if (!prev) setSearch(''); return !prev })
        }}
        disabled={testing !== null || pickerModels.length === 0}
        icon={testing
          ? <Loader2 size={12} className="animate-spin" />
          : result
            ? result.success
              ? <AnimatedCheck size={12} color="var(--c-status-success-text)" />
              : <X size={12} className="text-[var(--c-status-error-text)]" />
            : <Zap size={12} strokeWidth={1.5} />}
      >
        {label}
      </SettingsButton>
      {result && !result.success && !testing && (
        <ErrorDetailsButton error={result.error ?? { message: 'Unknown error' }} />
      )}
      {open && (
        <>
          <div className="fixed inset-0 z-10" onClick={() => setOpen(false)} />
          <div
            className="absolute right-0 top-[calc(100%+6px)] z-20 min-w-[220px] overflow-hidden dropdown-menu"
            style={{
              border: '0.5px solid var(--c-border-subtle)',
              borderRadius: '10px',
              padding: '4px',
              background: 'var(--c-bg-menu)',
              boxShadow: 'var(--c-dropdown-shadow)',
            }}
          >
            <div style={{ padding: '4px 4px 2px' }}>
              <SettingsSearchInput
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder={searchPlaceholder}
              />
            </div>
            <div className="max-h-[280px] overflow-y-auto py-1">
              {filtered.length === 0 ? (
                <p className="px-3 py-2 text-sm text-[var(--c-text-muted)]">--</p>
              ) : filtered.map((model) => (
                <button
                  key={model.id}
                  type="button"
                  onClick={() => void handleTest(model)}
                  className="flex w-full items-center justify-between gap-3 rounded-lg px-3 py-2 text-left text-sm transition-colors hover:bg-[var(--c-bg-deep)]"
                  style={{
                    color: result?.modelId === model.id ? 'var(--c-text-heading)' : 'var(--c-text-secondary)',
                    fontWeight: result?.modelId === model.id ? 600 : 400,
                  }}
                >
                  <span className="truncate">{model.model}</span>
                  {result?.modelId === model.id && result.success && <AnimatedCheck size={12} color="var(--c-status-success-text)" />}
                </button>
              ))}
            </div>
          </div>
        </>
      )}
    </div>
  )
}
