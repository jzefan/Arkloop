import { PageHeader } from '../components/PageHeader'
import { useLocale } from '../contexts/LocaleContext'

export function MemoryPage() {
  const { t } = useLocale()
  return (
    <div className="flex h-full flex-col overflow-hidden">
      <PageHeader title={t.nav.memory} />
      <div className="flex flex-1 items-center justify-center">
        <p className="text-sm text-[var(--c-text-muted)]">--</p>
      </div>
    </div>
  )
}
