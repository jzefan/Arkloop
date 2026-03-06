import { PageHeader } from '../components/PageHeader'
import { useLocale } from '../contexts/LocaleContext'

export function SettingsPage() {
  const { t } = useLocale()
  return (
    <div className="flex h-full flex-col overflow-hidden">
      <PageHeader title={t.nav.settings} />
      <div className="flex flex-1 items-center justify-center">
        <p className="text-sm text-[var(--c-text-muted)]">--</p>
      </div>
    </div>
  )
}
