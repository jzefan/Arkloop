type Props = {
  label?: string
}

export function FullScreenLoading({ label }: Props) {
  return (
    <div className="flex h-screen items-center justify-center bg-[var(--c-bg-page)]">
      <span className="text-sm text-[var(--c-text-muted)]">{label}</span>
    </div>
  )
}
