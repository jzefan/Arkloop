import { useCallback, useEffect, useMemo, useState } from 'react'
import { Loader2 } from 'lucide-react'
import {
  type ChannelIdentityResponse,
  type ChannelResponse,
  type LlmProvider,
  type Persona,
  listChannelPersonas,
  listChannels,
  listLlmProviders,
  listMyChannelIdentities,
} from '../../api'
import { useLocale } from '../../contexts/LocaleContext'
import { SettingsSectionHeader } from './_SettingsSectionHeader'
import { DesktopDiscordSettingsPanel } from './DesktopDiscordSettingsPanel'
import { DesktopTelegramSettingsPanel } from './DesktopTelegramSettingsPanel'
import { TabBar } from '@arkloop/shared/components/prompt-injection'

type Props = {
  accessToken: string
}

type IntegrationTab = 'telegram' | 'discord'

export function DesktopChannelsSettings({ accessToken }: Props) {
  const { t } = useLocale()
  const ct = t.channels
  const [activeTab, setActiveTab] = useState<IntegrationTab>('telegram')
  const [loading, setLoading] = useState(true)
  const [channels, setChannels] = useState<ChannelResponse[]>([])
  const [personas, setPersonas] = useState<Persona[]>([])
  const [identities, setIdentities] = useState<ChannelIdentityResponse[]>([])
  const [providers, setProviders] = useState<LlmProvider[]>([])

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [allChannels, linkedIdentities, allPersonas] = await Promise.all([
        listChannels(accessToken),
        listMyChannelIdentities(accessToken).catch(() => [] as ChannelIdentityResponse[]),
        listChannelPersonas(accessToken).catch(() => [] as Persona[]),
      ])
      setChannels(allChannels)
      setIdentities(linkedIdentities)
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
  const telegramIdentities = useMemo(
    () => identities.filter((identity) => identity.channel_type === 'telegram'),
    [identities],
  )
  const discordIdentities = useMemo(
    () => identities.filter((identity) => identity.channel_type === 'discord'),
    [identities],
  )

  const tabItems: { key: IntegrationTab; label: string }[] = [
    { key: 'telegram', label: ct.telegram },
    { key: 'discord', label: ct.discord },
  ]

  return (
    <div className="flex flex-col gap-6">
      <SettingsSectionHeader title={ct.title} description={ct.subtitle} />
      <TabBar tabs={tabItems} active={activeTab} onChange={setActiveTab} />

      {loading ? (
        <div className="flex items-center justify-center py-20 text-[var(--c-text-muted)]">
          <Loader2 size={20} className="animate-spin" />
        </div>
      ) : activeTab === 'telegram' ? (
        <DesktopTelegramSettingsPanel
          accessToken={accessToken}
          channel={telegramChannel}
          personas={personas}
          identities={telegramIdentities}
          providers={providers}
          reload={load}
        />
      ) : (
        <DesktopDiscordSettingsPanel
          accessToken={accessToken}
          channel={discordChannel}
          personas={personas}
          identities={discordIdentities}
          providers={providers}
          reload={load}
        />
      )}
    </div>
  )
}
