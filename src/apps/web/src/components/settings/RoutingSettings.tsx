import { useEffect, useMemo, useState, type ReactNode } from 'react'
import {
  listSpawnProfiles,
  listLlmProviders,
  setSpawnProfile,
  deleteSpawnProfile,
  resolveOpenVikingConfig,
  testLlmProviderModel,
} from '../../api'
import type { SpawnProfile, LlmProvider } from '../../api'
import { useLocale } from '../../contexts/LocaleContext'
import { getDesktopMode, getDesktopApi, isDesktop, isLocalMode } from '@arkloop/shared/desktop'
import { getAvailableCatalogFromAdvancedJson } from '@arkloop/shared/llm/available-catalog-advanced-json'
import { SettingsModelDropdown } from './SettingsModelDropdown'
import { SettingsButton, SettingsIconButton } from './_SettingsButton'
import { bridgeClient, checkBridgeAvailable } from '../../api-bridge'
import { AnimatedCheck } from '../AnimatedCheck'
import { Loader2, X, Zap } from 'lucide-react'

type Props = {
  accessToken: string
}

const PROFILE_NAMES = ['explore', 'task', 'strong'] as const

function RoutingSection({
  title,
  description,
  children,
}: {
  title: string
  description?: string
  children: ReactNode
}) {
  return (
    <section className="flex flex-col gap-2.5">
      <div>
        <h3 className="text-sm font-semibold text-[var(--c-text-heading)]">{title}</h3>
        {description && (
          <p className="mt-1 text-xs leading-5 text-[var(--c-text-muted)]">{description}</p>
        )}
      </div>
      {children}
    </section>
  )
}

function RoutingCard({ children }: { children: ReactNode }) {
  return (
    <div className="overflow-hidden rounded-xl border border-[var(--c-border-subtle)] bg-[var(--c-bg-menu)]">
      {children}
    </div>
  )
}

function RoutingRow({
  title,
  description,
  control,
}: {
  title: string
  description?: ReactNode
  control: ReactNode
}) {
  return (
    <div className="grid gap-3 px-5 py-4 first:border-t-0 sm:grid-cols-[minmax(0,1fr)_minmax(220px,320px)] sm:items-center sm:gap-6 [&+&]:border-t [&+&]:border-[var(--c-border-subtle)]">
      <div className="min-w-0">
        <div className="text-sm font-medium text-[var(--c-text-primary)]">{title}</div>
        {description && (
          <div className="mt-1 text-xs leading-5 text-[var(--c-text-tertiary)]">{description}</div>
        )}
      </div>
      <div className="min-w-0 sm:justify-self-end">{control}</div>
    </div>
  )
}

export function RoutingSettings({ accessToken }: Props) {
  const { t } = useLocale()
  const a = t.agentSettings
  const ds = t.desktopSettings
  const [profiles, setProfiles] = useState<SpawnProfile[]>([])
  const [providers, setProviders] = useState<LlmProvider[]>([])
  const [saving, setSaving] = useState<string | null>(null)
  const [testingToolModel, setTestingToolModel] = useState(false)
  const [toolModelTestResult, setToolModelTestResult] = useState<{ success: boolean; latency?: number; error?: string } | null>(null)
  const [showTestError, setShowTestError] = useState(false)
  const nonSaaSUi = getDesktopMode() !== null || isDesktop() || isLocalMode()
  const subAgentPlaceholder = isLocalMode()
    ? a.spawnProfileFollowCurrentChat
    : a.spawnProfilePlatformDefault

  useEffect(() => {
    listSpawnProfiles(accessToken).then(setProfiles).catch(() => {})
    listLlmProviders(accessToken).then(setProviders).catch(() => {})
  }, [accessToken])

  const modelOptions = providers
    .flatMap(p => p.models.filter(m => m.show_in_picker).map(m => ({
      value: `${p.name}^${m.model}`,
      label: `${p.name} / ${m.model}`,
    })))
  const imageModelOptions = providers
    .flatMap(p => p.models
      .filter((m) => {
        const catalog = getAvailableCatalogFromAdvancedJson(m.advanced_json)
        const outputModalities = Array.isArray(catalog?.output_modalities) ? catalog.output_modalities : []
        return outputModalities.includes('image')
      })
      .map(m => ({
        value: `${p.name}^${m.model}`,
        label: `${p.name} / ${m.model}`,
      })))

  const handleChange = async (name: string, value: string) => {
    setSaving(name)
    if (name === 'tool') setToolModelTestResult(null)
    try {
      if (value === '') {
        await deleteSpawnProfile(accessToken, name)
      } else {
        await setSpawnProfile(accessToken, name, value)
      }
      const updated = await listSpawnProfiles(accessToken)
      setProfiles(updated)
      if (name === 'tool') void syncToolModelToOpenViking(value)
    } finally {
      setSaving(null)
    }
  }

  const buildOpenVikingConfigureParams = (
    rootApiKey: string | undefined,
    vlm: NonNullable<Awaited<ReturnType<typeof resolveOpenVikingConfig>>['vlm']>,
    embedding: NonNullable<Awaited<ReturnType<typeof resolveOpenVikingConfig>>['embedding']>,
  ): Record<string, unknown> => ({
    embedding_provider: embedding.provider,
    embedding_model: embedding.model,
    embedding_api_key: embedding.api_key,
    embedding_api_base: embedding.api_base,
    embedding_extra_headers: embedding.extra_headers ?? {},
    embedding_dimension: String(embedding.dimension),
    vlm_provider: vlm.provider,
    vlm_model: vlm.model,
    vlm_api_key: vlm.api_key,
    vlm_api_base: vlm.api_base,
    vlm_extra_headers: vlm.extra_headers ?? {},
    root_api_key: rootApiKey ?? null,
  })

  const syncToolModelToOpenViking = async (value: string) => {
    const desktopApi = getDesktopApi()
    if (!desktopApi?.config) return

    const currentConfig = await desktopApi.config.get()
    if (currentConfig.memory.provider !== 'openviking') return

    const currentOV = currentConfig.memory.openviking ?? {}
    const providerName = value.split('^', 1)[0] ?? ''
    const modelName = value.includes('^') ? value.split('^').slice(1).join('^') : ''
    const matchedProvider = providers.find((provider) => provider.name === providerName)

    const nextOV = {
      ...currentOV,
      vlmSelector: value || undefined,
      vlmModel: modelName || undefined,
      vlmProvider: matchedProvider?.provider ?? currentOV.vlmProvider,
      vlmApiKey: undefined,
      vlmApiBase: matchedProvider?.base_url ?? currentOV.vlmApiBase,
    }

    if (
      value === ''
      || !currentOV.embeddingSelector
      || !(await checkBridgeAvailable().catch(() => false))
    ) {
      await desktopApi.config.set({
        ...currentConfig,
        memory: {
          ...currentConfig.memory,
          openviking: nextOV,
        },
      })
      return
    }

    try {
      const resolved = await resolveOpenVikingConfig(accessToken, {
        vlm_selector: value,
        embedding_selector: currentOV.embeddingSelector,
        embedding_dimension_hint: currentOV.embeddingDimension,
      })
      if (!resolved.vlm || !resolved.embedding) return

      const params = buildOpenVikingConfigureParams(currentOV.rootApiKey, resolved.vlm, resolved.embedding)
      const { operation_id } = await bridgeClient.performAction('openviking', 'configure', params)
      await new Promise<void>((resolve, reject) => {
        let done = false
        const stop = bridgeClient.streamOperation(operation_id, () => {}, (result) => {
          if (done) return
          done = true
          stop()
          if (result.status === 'completed') resolve()
          else reject(new Error(result.error ?? 'configure failed'))
        })
      })

      const syncedOV = {
        ...nextOV,
        vlmSelector: resolved.vlm.selector,
        vlmProvider: resolved.vlm.provider,
        vlmModel: resolved.vlm.model,
        vlmApiKey: undefined,
        vlmApiBase: resolved.vlm.api_base,
        embeddingSelector: resolved.embedding.selector,
        embeddingProvider: resolved.embedding.provider,
        embeddingModel: resolved.embedding.model,
        embeddingApiKey: undefined,
        embeddingApiBase: resolved.embedding.api_base,
        embeddingDimension: resolved.embedding.dimension,
      }
      await desktopApi.config.set({
        ...currentConfig,
        memory: {
          ...currentConfig.memory,
          openviking: syncedOV,
        },
      })
    } catch {
      // 工具模型保存不应被 OpenViking 同步失败阻断。
    }
  }

  const profileMeta: Record<string, { label: string; desc: string }> = {
    explore: { label: a.spawnProfileExplore, desc: a.spawnProfileExploreDesc },
    task:    { label: a.spawnProfileTask,    desc: a.spawnProfileTaskDesc    },
    strong:  { label: a.spawnProfileStrong,  desc: a.spawnProfileStrongDesc  },
  }
  const imageProfile = profiles.find((p) => p.profile === 'image')
  const imageModelValue = imageProfile?.resolved_model ?? ''
  const toolProfile = profiles.find((p) => p.profile === 'tool')
  const toolModelValue = toolProfile?.has_override ? toolProfile.resolved_model : ''
  const toolModelPlaceholder = (() => {
    const autoModel = toolProfile?.auto_model
    if (autoModel) {
      const parts = autoModel.split('^')
      const displayName = parts.length === 2 ? `${parts[0]} / ${parts[1]}` : autoModel
      return `${displayName} (${ds.toolModelAutoSuffix})`
    }
    return nonSaaSUi ? ds.toolModelSameAsChat : ds.toolModelPlatformDefault
  })()
  const effectiveToolModelValue = toolModelValue || toolProfile?.auto_model || ''
  const toolModelSelection = useMemo(() => {
    if (!effectiveToolModelValue.includes('^')) return null
    const [providerName, ...rest] = effectiveToolModelValue.split('^')
    const modelName = rest.join('^')
    if (!providerName || !modelName) return null
    const provider = providers.find((item) => item.name === providerName)
    const model = provider?.models.find((item) => item.model === modelName)
    if (!provider || !model) return null
    return { provider, model }
  }, [effectiveToolModelValue, providers])

  const handleTestToolModel = async () => {
    if (!toolModelSelection) return
    setTestingToolModel(true)
    try {
      const result = await testLlmProviderModel(accessToken, toolModelSelection.provider.id, toolModelSelection.model.id)
      setToolModelTestResult({ success: result.success, latency: result.latency_ms ?? undefined, error: result.error ?? undefined })
    } catch (e) {
      const message = e instanceof Error ? e.message : 'Unknown error'
      setToolModelTestResult({ success: false, error: message })
    } finally {
      setTestingToolModel(false)
    }
  }

  return (
    <div className="mx-auto flex w-full max-w-[1040px] flex-col gap-7 pb-8">
      <RoutingSection title={a.spawnProfileTitle} description={a.spawnProfileSubtitle}>
        <RoutingCard>
          {PROFILE_NAMES.map(name => {
            const profile = profiles.find(p => p.profile === name)
            const currentValue = profile?.has_override ? profile.resolved_model : ''
            const meta = profileMeta[name]
            return (
              <RoutingRow
                key={name}
                title={meta.label}
                description={meta.desc}
                control={(
                  <SettingsModelDropdown
                    value={currentValue}
                    options={modelOptions}
                    placeholder={subAgentPlaceholder}
                    disabled={saving === name}
                    onChange={v => handleChange(name, v)}
                  />
                )}
              />
            )
          })}
        </RoutingCard>
      </RoutingSection>

      <RoutingSection title={ds.toolModel} description={ds.toolModelDesc}>
        <RoutingCard>
          <RoutingRow
            title={ds.toolModel}
            description={!toolProfile?.has_override && toolProfile?.auto_model ? ds.toolModelAutoHint : undefined}
            control={(
              <div className="flex w-full items-center gap-2">
                <div className="min-w-0 flex-1">
                  <SettingsModelDropdown
                    value={toolModelValue}
                    options={modelOptions}
                    placeholder={toolModelPlaceholder}
                    disabled={saving === 'tool'}
                    onChange={v => handleChange('tool', v)}
                  />
                </div>
                <SettingsIconButton
                  label={ds.toolModel}
                  onClick={() => {
                    if (toolModelTestResult?.success) { setToolModelTestResult(null); return }
                    void handleTestToolModel()
                  }}
                  disabled={testingToolModel || (!toolModelSelection && !toolModelTestResult)}
                  className="h-9 w-9"
                >
                  {testingToolModel
                    ? <Loader2 size={14} className="animate-spin" />
                    : toolModelTestResult
                      ? toolModelTestResult.success
                        ? <AnimatedCheck size={14} color="var(--c-status-success-text)" />
                        : <X size={14} className="text-[var(--c-status-error-text)]" />
                      : <Zap size={14} strokeWidth={1.5} />}
                </SettingsIconButton>
                {toolModelTestResult && !toolModelTestResult.success && !testingToolModel && (
                  <div className="relative">
                    <SettingsButton
                      variant="danger"
                      onClick={() => setShowTestError((v) => !v)}
                      className="h-9 shrink-0 text-xs"
                    >
                      Error
                    </SettingsButton>
                    {showTestError && (
                      <>
                        <div className="fixed inset-0 z-40" onClick={() => setShowTestError(false)} />
                        <div
                          className="dropdown-menu absolute right-0 top-[calc(100%+6px)] z-50 max-w-[320px] min-w-[200px]"
                          style={{
                            border: '0.5px solid var(--c-border-subtle)',
                            borderRadius: '10px',
                            padding: '12px',
                            background: 'var(--c-bg-menu)',
                            boxShadow: 'var(--c-dropdown-shadow)',
                            maxHeight: '160px',
                            overflowY: 'auto',
                          }}
                        >
                          <pre className="whitespace-pre-wrap break-all text-xs text-[var(--c-text-secondary)]">{toolModelTestResult?.error ?? ''}</pre>
                        </div>
                      </>
                    )}
                  </div>
                )}
              </div>
            )}
          />
        </RoutingCard>
      </RoutingSection>

      <RoutingSection title={a.imageGenerativeTitle} description={a.imageGenerativeDesc}>
        <RoutingCard>
          <RoutingRow
            title={a.imageGenerativeTitle}
            description={a.imageGenerativeDesc}
            control={(
              <SettingsModelDropdown
                value={imageModelValue}
                options={imageModelOptions}
                placeholder={a.imageGenerativeUnset}
                disabled={saving === 'image'}
                onChange={v => handleChange('image', v)}
              />
            )}
          />
        </RoutingCard>
      </RoutingSection>
    </div>
  )
}
