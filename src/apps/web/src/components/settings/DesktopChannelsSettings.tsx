import { useCallback, useEffect, useMemo, useState } from 'react'
import { Loader2 } from 'lucide-react'
import {
  type ChannelResponse,
  type LlmProvider,
  type Persona,
  listChannelPersonas,
  listChannels,
  listLlmProviders,
} from '../../api'
import { useLocale } from '../../contexts/LocaleContext'
import { DesktopDiscordSettingsPanel } from './DesktopDiscordSettingsPanel'
import { DesktopQQSettingsPanel } from './DesktopQQSettingsPanel'
import { DesktopTelegramSettingsPanel } from './DesktopTelegramSettingsPanel'

type Props = {
  accessToken: string
}

type IntegrationTab = 'telegram' | 'discord' | 'qq'

function TelegramIcon({ size = 15 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <path d="M11.944 0A12 12 0 0 0 0 12a12 12 0 0 0 12 12 12 12 0 0 0 12-12A12 12 0 0 0 12 0a12 12 0 0 0-.056 0zm4.962 7.224c.1-.002.321.023.465.14a.506.506 0 0 1 .171.325c.016.093.036.306.02.472-.18 1.898-.962 6.502-1.36 8.627-.168.9-.499 1.201-.82 1.23-.696.065-1.225-.46-1.9-.902-1.056-.693-1.653-1.124-2.678-1.8-1.185-.78-.417-1.21.258-1.91.177-.184 3.247-2.977 3.307-3.23.007-.032.014-.15-.056-.212s-.174-.041-.249-.024c-.106.024-1.793 1.14-5.061 3.345-.48.33-.913.49-1.302.48-.428-.008-1.252-.241-1.865-.44-.752-.245-1.349-.374-1.297-.789.027-.216.325-.437.893-.663 3.498-1.524 5.83-2.529 6.998-3.014 3.332-1.386 4.025-1.627 4.476-1.635z" />
    </svg>
  )
}

function DiscordIcon({ size = 15 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <path d="M20.317 4.3698a19.7913 19.7913 0 00-4.8851-1.5152.0741.0741 0 00-.0785.0371c-.211.3753-.4447.8648-.6083 1.2495-1.8447-.2762-3.68-.2762-5.4868 0-.1636-.3933-.4058-.8742-.6177-1.2495a.077.077 0 00-.0785-.037 19.7363 19.7363 0 00-4.8852 1.515.0699.0699 0 00-.0321.0277C.5334 9.0458-.319 13.5799.0992 18.0578a.0824.0824 0 00.0312.0561c2.0528 1.5076 4.0413 2.4228 5.9929 3.0294a.0777.0777 0 00.0842-.0276c.4616-.6304.8731-1.2952 1.226-1.9942a.076.076 0 00-.0416-.1057c-.6528-.2476-1.2743-.5495-1.8722-.8923a.077.077 0 01-.0076-.1277c.1258-.0943.2517-.1923.3718-.2914a.0743.0743 0 01.0776-.0105c3.9278 1.7933 8.18 1.7933 12.0614 0a.0739.0739 0 01.0785.0095c.1202.099.246.1981.3728.2924a.077.077 0 01-.0066.1276 12.2986 12.2986 0 01-1.873.8914.0766.0766 0 00-.0407.1067c.3604.698.7719 1.3628 1.225 1.9932a.076.076 0 00.0842.0286c1.961-.6067 3.9495-1.5219 6.0023-3.0294a.077.077 0 00.0313-.0552c.5004-5.177-.8382-9.6739-3.5485-13.6604a.061.061 0 00-.0312-.0286zM8.02 15.3312c-1.1825 0-2.1569-1.0857-2.1569-2.419 0-1.3332.9555-2.4189 2.157-2.4189 1.2108 0 2.1757 1.0952 2.1568 2.419 0 1.3332-.9555 2.4189-2.1569 2.4189zm7.9748 0c-1.1825 0-2.1569-1.0857-2.1569-2.419 0-1.3332.9554-2.4189 2.1569-2.4189 1.2108 0 2.1757 1.0952 2.1568 2.419 0 1.3332-.946 2.4189-2.1568 2.4189Z" />
    </svg>
  )
}

function QQIcon({ size = 15 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <path d="M21.395 15.035a40 40 0 0 0-.803-2.264l-1.079-2.695c.001-.032.014-.562.014-.836C19.526 4.632 17.351 0 12 0S4.474 4.632 4.474 9.241c0 .274.013.804.014.836l-1.08 2.695a39 39 0 0 0-.802 2.264c-1.021 3.283-.69 4.643-.438 4.673.54.065 2.103-2.472 2.103-2.472 0 1.469.756 3.387 2.394 4.771-.612.188-1.363.479-1.845.835-.434.32-.379.646-.301.778.343.578 5.883.369 7.482.189 1.6.18 7.14.389 7.483-.189.078-.132.132-.458-.301-.778-.483-.356-1.233-.646-1.846-.836 1.637-1.384 2.393-3.302 2.393-4.771 0 0 1.563 2.537 2.103 2.472.251-.03.581-1.39-.438-4.673" />
    </svg>
  )
}

const PLATFORM_ICONS: Record<IntegrationTab, React.ReactNode> = {
  telegram: <TelegramIcon />,
  discord: <DiscordIcon />,
  qq: <QQIcon />,
}

export function DesktopChannelsSettings({ accessToken }: Props) {
  const { t } = useLocale()
  const ct = t.channels
  const [activeTab, setActiveTab] = useState<IntegrationTab>('telegram')
  const [loading, setLoading] = useState(true)
  const [channels, setChannels] = useState<ChannelResponse[]>([])
  const [personas, setPersonas] = useState<Persona[]>([])
  const [providers, setProviders] = useState<LlmProvider[]>([])

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [allChannels, allPersonas] = await Promise.all([
        listChannels(accessToken),
        listChannelPersonas(accessToken).catch(() => [] as Persona[]),
      ])
      setChannels(allChannels)
      setPersonas(allPersonas)
    } finally {
      setLoading(false)
    }
  }, [accessToken])

  useEffect(() => {
    void load()
  }, [load])

  useEffect(() => {
    listLlmProviders(accessToken).then(setProviders).catch(() => {})
  }, [accessToken])

  const telegramChannel = useMemo(
    () => channels.find((channel) => channel.channel_type === 'telegram') ?? null,
    [channels],
  )
  const discordChannel = useMemo(
    () => channels.find((channel) => channel.channel_type === 'discord') ?? null,
    [channels],
  )
  const qqChannel = useMemo(
    () => channels.find((channel) => channel.channel_type === 'qq') ?? null,
    [channels],
  )

  const tabItems: { key: IntegrationTab; label: string; channel: ChannelResponse | null }[] = [
    { key: 'telegram', label: ct.telegram, channel: telegramChannel },
    { key: 'discord', label: ct.discord, channel: discordChannel },
    { key: 'qq', label: ct.qq, channel: qqChannel },
  ]

  return (
    <div className="-m-6 flex min-h-0 min-w-0 overflow-hidden" style={{ height: 'calc(100% + 48px)' }}>
      {/* Platform list */}
      <div
        className="flex w-[200px] shrink-0 flex-col overflow-y-auto py-2"
        style={{ borderRight: '0.5px solid var(--c-border-subtle)' }}
      >
        <div className="flex flex-col gap-[3px] px-2">
          {tabItems.map(({ key, label, channel }) => (
            <button
              key={key}
              onClick={() => setActiveTab(key)}
              className={[
                'flex h-[38px] items-center gap-2.5 truncate rounded-lg px-2.5 text-left text-[13px] font-medium transition-all duration-[120ms] active:scale-[0.97]',
                activeTab === key
                  ? 'bg-[var(--c-bg-deep)] text-[var(--c-text-heading)]'
                  : 'text-[var(--c-text-secondary)] hover:bg-[var(--c-bg-deep)]',
              ].join(' ')}
            >
              <span className="shrink-0 text-[var(--c-text-muted)]">{PLATFORM_ICONS[key]}</span>
              <span className="min-w-0 flex-1 truncate">{label}</span>
              {channel?.is_active && (
                <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-[var(--c-status-success-text)]" />
              )}
            </button>
          ))}
        </div>
      </div>

      {/* Detail panel */}
      <div className="min-w-0 flex-1 overflow-y-auto p-6">
        {loading ? (
          <div className="flex items-center justify-center py-20 text-[var(--c-text-muted)]">
            <Loader2 size={20} className="animate-spin" />
          </div>
        ) : activeTab === 'telegram' ? (
          <DesktopTelegramSettingsPanel
            accessToken={accessToken}
            channel={telegramChannel}
            personas={personas}
            providers={providers}
            reload={load}
          />
        ) : activeTab === 'discord' ? (
          <DesktopDiscordSettingsPanel
            accessToken={accessToken}
            channel={discordChannel}
            personas={personas}
            providers={providers}
            reload={load}
          />
        ) : (
          <DesktopQQSettingsPanel
            accessToken={accessToken}
            channel={qqChannel}
            personas={personas}
            providers={providers}
            reload={load}
          />
        )}
      </div>
    </div>
  )
}
