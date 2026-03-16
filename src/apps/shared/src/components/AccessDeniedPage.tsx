import { ShieldCheck } from 'lucide-react'

type Props = {
  title: string
  description: string
  signOutLabel: string
  onSignOut: () => void
}

export function AccessDeniedPage({ title, description, signOutLabel, onSignOut }: Props) {
  return (
    <div className="flex h-screen flex-col items-center justify-center gap-3 bg-[var(--c-bg-page)]">
      <ShieldCheck size={32} className="text-[var(--c-text-muted)]" />
      <p className="text-sm font-medium text-[var(--c-text-secondary)]">{title}</p>
      <p className="text-xs text-[var(--c-text-muted)]">{description}</p>
      <button
        type="button"
        onClick={onSignOut}
        className="mt-2 text-xs text-[var(--c-text-muted)] underline hover:opacity-70"
      >
        {signOutLabel}
      </button>
    </div>
  )
}
