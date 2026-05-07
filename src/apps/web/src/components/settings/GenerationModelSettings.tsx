import { useEffect, useMemo, useRef, useState } from 'react'
import {
  deleteSpawnProfile,
  listLlmProviders,
  listSpawnProfiles,
  setSpawnProfile,
  type LlmProvider,
  type SpawnProfile,
} from '../../api'
import { useLocale } from '../../contexts/LocaleContext'
import { getAvailableCatalogFromAdvancedJson } from '@arkloop/shared/llm/available-catalog-advanced-json'
import { SettingsModelDropdown } from './SettingsModelDropdown'
import { listZenMuxModels, zenMuxModelId, zenMuxModelLabel, zenMuxModelSupports, type ZenMuxModel } from '../../lib/zenmuxModels'

type Props = {
  accessToken: string
}

type GenerationProfile = 'image' | 'video'

export function GenerationModelSettings({ accessToken }: Props) {
  const { t } = useLocale()
  const a = t.agentSettings
  const [profiles, setProfiles] = useState<SpawnProfile[]>([])
  const [providers, setProviders] = useState<LlmProvider[]>([])
  const [zenMuxModels, setZenMuxModels] = useState<ZenMuxModel[]>([])
  const [saving, setSaving] = useState<string | null>(null)
  const autoSetRef = useRef<Set<GenerationProfile>>(new Set())

  useEffect(() => {
    void Promise.all([
      listSpawnProfiles(accessToken).catch(() => [] as SpawnProfile[]),
      listLlmProviders(accessToken).catch(() => [] as LlmProvider[]),
    ]).then(([nextProfiles, nextProviders]) => {
      setProfiles(nextProfiles)
      setProviders(nextProviders)
    })
  }, [accessToken])

  useEffect(() => {
    let cancelled = false
    void Promise.all([listZenMuxModels('image'), listZenMuxModels('video')])
      .then(([imageModels, videoModels]) => {
        if (!cancelled) setZenMuxModels(dedupeZenMuxModels([...imageModels, ...videoModels]))
      })
      .catch(() => {
        if (!cancelled) setZenMuxModels([])
      })
    return () => {
      cancelled = true
    }
  }, [])

  const imageModelOptions = useMemo(
    () => buildGenerationModelOptions(providers, zenMuxModels, 'image'),
    [providers, zenMuxModels],
  )
  const videoModelOptions = useMemo(
    () => buildGenerationModelOptions(providers, zenMuxModels, 'video'),
    [providers, zenMuxModels],
  )
  const canAutoSetImage = useMemo(
    () => hasConfiguredGenerationProvider(providers, 'image'),
    [providers],
  )
  const canAutoSetVideo = useMemo(
    () => hasConfiguredGenerationProvider(providers, 'video'),
    [providers],
  )

  const handleChange = async (name: GenerationProfile, value: string) => {
    setSaving(name)
    try {
      if (value === '') {
        await deleteSpawnProfile(accessToken, name)
      } else {
        await setSpawnProfile(accessToken, name, value)
      }
      setProfiles(await listSpawnProfiles(accessToken))
    } finally {
      setSaving(null)
    }
  }

  const imageProfile = profiles.find((profile) => profile.profile === 'image')
  const videoProfile = profiles.find((profile) => profile.profile === 'video')
  const imageRawValue = imageProfile?.resolved_model ?? ''
  const videoRawValue = videoProfile?.resolved_model ?? ''
  const imageValue = optionExists(imageProfile?.resolved_model ?? '', imageModelOptions)
    ? imageProfile?.resolved_model ?? ''
    : ''
  const videoValue = optionExists(videoProfile?.resolved_model ?? '', videoModelOptions)
    ? videoProfile?.resolved_model ?? ''
    : ''

  useEffect(() => {
    const staleProfiles: GenerationProfile[] = []
    if (imageRawValue && imageModelOptions.length > 0 && !optionExists(imageRawValue, imageModelOptions)) {
      staleProfiles.push('image')
    }
    if (videoRawValue && videoModelOptions.length > 0 && !optionExists(videoRawValue, videoModelOptions)) {
      staleProfiles.push('video')
    }
    if (staleProfiles.length === 0) return
    let cancelled = false
    void Promise.all(staleProfiles.map((profile) => deleteSpawnProfile(accessToken, profile)))
      .then(() => listSpawnProfiles(accessToken))
      .then((nextProfiles) => {
        if (!cancelled) setProfiles(nextProfiles)
      })
      .catch(() => undefined)
    return () => {
      cancelled = true
    }
  }, [accessToken, imageRawValue, videoRawValue, imageModelOptions, videoModelOptions])

  useEffect(() => {
    const updates: Array<[GenerationProfile, string]> = []
    if (!imageRawValue && canAutoSetImage && imageModelOptions.length > 0 && !autoSetRef.current.has('image')) {
      updates.push(['image', imageModelOptions[0].value])
    }
    if (!videoRawValue && canAutoSetVideo && videoModelOptions.length > 0 && !autoSetRef.current.has('video')) {
      updates.push(['video', videoModelOptions[0].value])
    }
    if (updates.length === 0) return
    let cancelled = false
    void Promise.all(updates.map(([profile, model]) => setSpawnProfile(accessToken, profile, model)))
      .then(() => listSpawnProfiles(accessToken))
      .then((nextProfiles) => {
        if (!cancelled) {
          for (const [profile] of updates) autoSetRef.current.add(profile)
          setProfiles(nextProfiles)
        }
      })
      .catch(() => undefined)
    return () => {
      cancelled = true
    }
  }, [accessToken, canAutoSetImage, canAutoSetVideo, imageRawValue, videoRawValue, imageModelOptions, videoModelOptions])

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h3 className="text-sm font-medium text-[var(--c-text-heading)]">
          {a.generationModelTitle}
        </h3>
        <p className="mt-1 text-xs text-[var(--c-text-muted)]">
          {a.generationModelSubtitle}
        </p>
      </div>

      <GenerationModelRow
        label={a.imageGenerativeTitle}
        description={a.imageGenerativeDesc}
        value={imageValue}
        options={imageModelOptions}
        placeholder={a.imageGenerativeUnset}
        disabled={saving === 'image'}
        onChange={(value) => void handleChange('image', value)}
      />

      <GenerationModelRow
        label={a.videoGenerativeTitle}
        description={a.videoGenerativeDesc}
        value={videoValue}
        options={videoModelOptions}
        placeholder={a.videoGenerativeUnset}
        disabled={saving === 'video'}
        onChange={(value) => void handleChange('video', value)}
      />
    </div>
  )
}

function GenerationModelRow({
  label,
  description,
  value,
  options,
  placeholder,
  disabled,
  onChange,
}: {
  label: string
  description: string
  value: string
  options: { value: string; label: string }[]
  placeholder: string
  disabled: boolean
  onChange: (value: string) => void
}) {
  return (
    <div
      className="flex items-center justify-between gap-4 rounded-xl px-5 py-4"
      style={{ border: '0.5px solid var(--c-border-subtle)', background: 'var(--c-bg-menu)' }}
    >
      <div className="min-w-0 shrink-0">
        <span className="text-sm font-medium text-[var(--c-text-primary)]">
          {label}
        </span>
        <p className="mt-0.5 text-xs text-[var(--c-text-muted)]">
          {description}
        </p>
      </div>
      <div className="min-w-0 flex-1" style={{ maxWidth: 320 }}>
        <SettingsModelDropdown
          value={value}
          options={options}
          placeholder={placeholder}
          disabled={disabled}
          onChange={onChange}
        />
      </div>
    </div>
  )
}

function buildGenerationModelOptions(
  providers: LlmProvider[],
  zenMuxModels: ZenMuxModel[],
  modality: 'image' | 'video',
) {
  const taggedOptions = providers
    .filter((provider) => !isZenMuxProvider(provider) || isZenMuxVertexProvider(provider))
    .flatMap((provider) =>
      provider.models
        .filter((model) => modelSupportsOutputModality(model, modality))
        .map((model) => ({
          value: `${provider.name}^${model.model}`,
          label: `${provider.name} / ${model.model}`,
        })),
    )
  const zenMuxProvider = providers.find(isZenMuxVertexProvider) ?? providers.find(isZenMuxProvider)
  const zenMuxProviderName = zenMuxProvider?.name ?? 'ZenMux'
  const zenMuxOptions = zenMuxModels
    .filter((model) => zenMuxModelSupports(model, modality))
    .map((model) => ({
      value: `${zenMuxProviderName}^${zenMuxModelId(model)}`,
      label: `ZenMux / ${zenMuxModelLabel(model)}`,
    }))
  return dedupeModelOptions([...zenMuxOptions, ...taggedOptions])
}

function modelSupportsOutputModality(model: LlmProvider['models'][number], modality: string): boolean {
  const catalog = getAvailableCatalogFromAdvancedJson(model.advanced_json)
  const outputModalities = Array.isArray(catalog?.output_modalities) ? catalog.output_modalities : []
  return outputModalities.includes(modality)
}

function isZenMuxProvider(provider: LlmProvider): boolean {
  const baseUrl = provider.base_url?.toLowerCase() ?? ''
  const name = provider.name.toLowerCase()
  return baseUrl.includes('zenmux.ai') || name.includes('zenmux')
}

function isZenMuxVertexProvider(provider: LlmProvider): boolean {
  const baseUrl = provider.base_url?.toLowerCase() ?? ''
  return provider.provider === 'gemini' && baseUrl.includes('zenmux.ai/api/vertex-ai')
}

function hasConfiguredGenerationProvider(providers: LlmProvider[], modality: 'image' | 'video'): boolean {
  return providers.some((provider) => {
    if (isZenMuxProvider(provider)) return true
    return provider.models.some((model) => modelSupportsOutputModality(model, modality))
  })
}

function optionExists(value: string, options: { value: string; label: string }[]): boolean {
  if (!value) return true
  return options.some((option) => option.value === value)
}

function dedupeModelOptions(options: { value: string; label: string }[]) {
  const seen = new Set<string>()
  return options.filter((option) => {
    if (seen.has(option.value)) return false
    seen.add(option.value)
    return true
  })
}

function dedupeZenMuxModels(models: ZenMuxModel[]) {
  const seen = new Set<string>()
  return models.filter((model) => {
    const id = zenMuxModelId(model)
    if (!id || seen.has(id)) return false
    seen.add(id)
    return true
  })
}
