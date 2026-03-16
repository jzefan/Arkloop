import { Loader2 } from 'lucide-react'

type Props = {
  size?: number
}

export function PageLoading({ size = 20 }: Props) {
  return (
    <div className="flex flex-1 items-center justify-center py-16">
      <Loader2 size={size} className="animate-spin text-[var(--c-text-muted)]" />
    </div>
  )
}
