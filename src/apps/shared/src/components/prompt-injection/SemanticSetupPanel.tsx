import { useEffect, useRef, useState } from 'react'
import { Loader2 } from 'lucide-react'
import { useToast } from '../useToast'
import type { PromptInjectionTexts } from './types'
import { SETTING_KEYS } from './types'

export interface SemanticSetupPanelProps {
  accessToken: string
  bridgeAvailable: boolean
  onSaved: () => void
  texts: PromptInjectionTexts
  setSetting: (key: string, value: string, token: string) => Promise<unknown>
  bridgeInstall: (variant: string) => Promise<{ operation_id: string }>
  waitForInstallCompletion?: (operationId: string) => Promise<void>
  formatError: (err: unknown) => string
  onInstallStarted?: (operationId: string) => void
  defaultMode?: 'local' | 'api'
  initialApiEndpoint?: string
  initialApiModel?: string
  initialApiTimeoutMs?: string
}

export function SemanticSetupPanel({
  accessToken,
  bridgeAvailable,
  onSaved,
  texts,
  setSetting,
  bridgeInstall,
  waitForInstallCompletion,
  formatError,
  onInstallStarted,
  defaultMode = 'api',
  initialApiEndpoint = 'https://openrouter.ai/api/v1',
  initialApiModel = 'openai/gpt-oss-safeguard-20b',
  initialApiTimeoutMs = '4000',
}: SemanticSetupPanelProps) {
  const { addToast } = useToast()

  const [mode, setMode] = useState<'local' | 'api'>(defaultMode)
  const [variant, setVariant] = useState<'22m' | '86m'>('22m')
  const [endpoint, setEndpoint] = useState(initialApiEndpoint)
  const [apiKey, setApiKey] = useState('')
  const [model, setModel] = useState(initialApiModel)
  const [timeoutMs, setTimeoutMs] = useState(initialApiTimeoutMs)
  const [saving, setSaving] = useState(false)
  const [waitingForInstall, setWaitingForInstall] = useState(false)
  const [installError, setInstallError] = useState('')
  const mountedRef = useRef(true)
  const localActivationGenerationRef = useRef(0)

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      localActivationGenerationRef.current += 1
    }
  }, [])

  useEffect(() => {
    setMode(defaultMode)
    setEndpoint(initialApiEndpoint)
    setModel(initialApiModel)
    setTimeoutMs(initialApiTimeoutMs)
    setApiKey('')
    setInstallError('')
  }, [defaultMode, initialApiEndpoint, initialApiModel, initialApiTimeoutMs])

  const handleSaveApi = async () => {
    if (!endpoint.trim() || !apiKey.trim() || !model.trim() || !timeoutMs.trim()) return
    localActivationGenerationRef.current += 1
    setWaitingForInstall(false)
    setSaving(true)
    try {
      await setSetting(SETTING_KEYS.SEMANTIC_API_ENDPOINT, endpoint.trim(), accessToken)
      await setSetting(SETTING_KEYS.SEMANTIC_API_KEY, apiKey.trim(), accessToken)
      await setSetting(SETTING_KEYS.SEMANTIC_API_MODEL, model.trim(), accessToken)
      await setSetting(SETTING_KEYS.SEMANTIC_API_TIMEOUT_MS, timeoutMs.trim(), accessToken)
      await setSetting(SETTING_KEYS.SEMANTIC_PROVIDER, 'api', accessToken)
      addToast(texts.toastUpdated, 'success')
      onSaved()
    } catch (err) {
      addToast(formatError(err), 'error')
    } finally {
      setSaving(false)
    }
  }

  const handleInstallLocal = async () => {
    setSaving(true)
    setInstallError('')
    try {
      const { operation_id } = await bridgeInstall(variant)
      const generation = localActivationGenerationRef.current + 1
      localActivationGenerationRef.current = generation
      onInstallStarted?.(operation_id)
      addToast(`${texts.semanticInstallStarted} (${operation_id.slice(0, 8)})`, 'success')
      if (!waitForInstallCompletion) {
        await setSetting(SETTING_KEYS.SEMANTIC_PROVIDER, 'local', accessToken)
        addToast(texts.toastUpdated, 'success')
        onSaved()
        return
      }

      setWaitingForInstall(true)
      void waitForInstallCompletion(operation_id)
        .then(async () => {
          if (!mountedRef.current || localActivationGenerationRef.current !== generation) {
            return
          }
          await setSetting(SETTING_KEYS.SEMANTIC_PROVIDER, 'local', accessToken)
          if (!mountedRef.current || localActivationGenerationRef.current !== generation) {
            return
          }
          addToast(texts.toastUpdated, 'success')
          onSaved()
        })
        .catch((err) => {
          if (!mountedRef.current || localActivationGenerationRef.current !== generation) {
            return
          }
          const msg = formatError(err)
          setInstallError(msg)
          addToast(msg, 'error')
        })
        .finally(() => {
          if (!mountedRef.current || localActivationGenerationRef.current !== generation) {
            return
          }
          setWaitingForInstall(false)
        })
    } catch (err) {
      const msg = formatError(err)
      setInstallError(msg)
      addToast(msg, 'error')
    } finally {
      setSaving(false)
    }
  }

  const modeBtn = (value: 'local' | 'api', label: string) => (
    <button
      onClick={() => {
        localActivationGenerationRef.current += 1
        setWaitingForInstall(false)
        setMode(value)
        setInstallError('')
      }}
      disabled={saving || waitingForInstall}
      className={[
        'rounded-md px-3 py-1.5 text-xs font-medium transition-colors',
        mode === value
          ? 'bg-[var(--c-text-primary)] text-[var(--c-bg-card)]'
          : 'border border-[var(--c-border-mid)] bg-[var(--c-bg-card)] text-[var(--c-text-secondary)] hover:text-[var(--c-text-primary)]',
        (saving || waitingForInstall) ? 'cursor-not-allowed opacity-60' : '',
      ].join(' ')}
    >
      {label}
    </button>
  )

  return (
    <div className="mt-2 rounded-lg border border-[var(--c-border-console)] bg-[var(--c-bg-deep2)] p-4">
      <div className="mb-4 flex gap-2">
        {modeBtn('local', texts.semanticProviderLocal)}
        {modeBtn('api', texts.semanticProviderApi)}
      </div>

      {mode === 'local' && (
        <div className="flex flex-col gap-3">
          <p className="text-xs text-[var(--c-text-muted)]">{texts.semanticLocalDesc}</p>
          <div className="flex flex-col gap-1.5">
            <span className="text-xs font-medium text-[var(--c-text-secondary)]">{texts.semanticModelVariant}</span>
            <div className="flex gap-2">
              {(['22m', '86m'] as const).map(v => (
                <button
                  key={v}
                  onClick={() => setVariant(v)}
                  className={[
                    'rounded-md px-3 py-1.5 text-xs font-medium transition-colors',
                    variant === v
                      ? 'bg-[var(--c-text-primary)] text-[var(--c-bg-card)]'
                      : 'border border-[var(--c-border-mid)] bg-[var(--c-bg-card)] text-[var(--c-text-secondary)] hover:text-[var(--c-text-primary)]',
                  ].join(' ')}
                >
                  {v === '22m' ? texts.semanticModel22m : texts.semanticModel86m}
                </button>
              ))}
            </div>
          </div>
          {!bridgeAvailable && (
            <p className="text-xs text-[var(--c-status-warning-text)]">{texts.semanticBridgeRequired}</p>
          )}
          {installError && (
            <p className="text-xs text-[var(--c-status-error-text,red)]">{installError}</p>
          )}
          <button
            disabled={!bridgeAvailable || saving || waitingForInstall}
            onClick={() => void handleInstallLocal()}
            className={[
              'w-fit rounded-md border px-3 py-1.5 text-xs font-medium transition-colors',
              bridgeAvailable && !waitingForInstall
                ? 'border-[var(--c-status-success-text)] text-[var(--c-status-success-text)] hover:bg-[var(--c-status-success-bg)]'
                : 'border-[var(--c-border-console)] text-[var(--c-text-muted)] opacity-50 cursor-not-allowed',
            ].join(' ')}
          >
            {(saving || waitingForInstall) ? <Loader2 size={12} className="inline animate-spin" /> : texts.actionInstallModel}
          </button>
        </div>
      )}

      {mode === 'api' && (
        <div className="flex flex-col gap-3">
          <p className="text-xs text-[var(--c-text-muted)]">{texts.semanticApiDesc}</p>
          <p className="text-xs text-[var(--c-text-secondary)]">{texts.semanticApiPresetHint}</p>
          <div className="flex flex-col gap-1.5">
            <span className="text-xs font-medium text-[var(--c-text-secondary)]">{texts.semanticApiEndpointLabel}</span>
            <input
              type="url"
              value={endpoint}
              onChange={e => setEndpoint(e.target.value)}
              placeholder={texts.semanticApiEndpointHint}
              className="rounded-md border border-[var(--c-border-console)] bg-[var(--c-bg-card)] px-3 py-2 text-xs text-[var(--c-text-primary)] placeholder:text-[var(--c-text-muted)] focus:outline-none focus:ring-1 focus:ring-[var(--c-text-muted)]"
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <span className="text-xs font-medium text-[var(--c-text-secondary)]">{texts.semanticApiModelLabel}</span>
            <input
              type="text"
              value={model}
              onChange={e => setModel(e.target.value)}
              placeholder={texts.semanticApiModelHint}
              className="rounded-md border border-[var(--c-border-console)] bg-[var(--c-bg-card)] px-3 py-2 text-xs text-[var(--c-text-primary)] placeholder:text-[var(--c-text-muted)] focus:outline-none focus:ring-1 focus:ring-[var(--c-text-muted)]"
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <span className="text-xs font-medium text-[var(--c-text-secondary)]">{texts.semanticApiTimeoutLabel}</span>
            <input
              type="number"
              min="500"
              step="100"
              value={timeoutMs}
              onChange={e => setTimeoutMs(e.target.value)}
              placeholder={texts.semanticApiTimeoutHint}
              className="rounded-md border border-[var(--c-border-console)] bg-[var(--c-bg-card)] px-3 py-2 text-xs text-[var(--c-text-primary)] placeholder:text-[var(--c-text-muted)] focus:outline-none focus:ring-1 focus:ring-[var(--c-text-muted)]"
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <span className="text-xs font-medium text-[var(--c-text-secondary)]">{texts.semanticApiKeyLabel}</span>
            <input
              type="password"
              value={apiKey}
              onChange={e => setApiKey(e.target.value)}
              placeholder={texts.semanticApiKeyHint}
              className="rounded-md border border-[var(--c-border-console)] bg-[var(--c-bg-card)] px-3 py-2 text-xs text-[var(--c-text-primary)] placeholder:text-[var(--c-text-muted)] focus:outline-none focus:ring-1 focus:ring-[var(--c-text-muted)]"
            />
          </div>
          <button
            disabled={saving || waitingForInstall || !endpoint.trim() || !apiKey.trim() || !model.trim() || !timeoutMs.trim()}
            onClick={() => void handleSaveApi()}
            className={[
              'w-fit rounded-md border px-3 py-1.5 text-xs font-medium transition-colors',
              !waitingForInstall && endpoint.trim() && apiKey.trim() && model.trim() && timeoutMs.trim()
                ? 'border-[var(--c-status-success-text)] text-[var(--c-status-success-text)] hover:bg-[var(--c-status-success-bg)]'
                : 'border-[var(--c-border-console)] text-[var(--c-text-muted)] opacity-50 cursor-not-allowed',
            ].join(' ')}
          >
            {(saving || waitingForInstall) ? <Loader2 size={12} className="inline animate-spin" /> : texts.actionSave}
          </button>
        </div>
      )}
    </div>
  )
}
