import { ConfirmDialog as SharedConfirmDialog } from '@arkloop/shared'
import type { ComponentProps } from 'react'
import { useLocale } from '../contexts/LocaleContext'

type Props = Omit<ComponentProps<typeof SharedConfirmDialog>, 'cancelLabel'>

export function ConfirmDialog({ title, confirmLabel, ...rest }: Props) {
  const { t } = useLocale()
  return (
    <SharedConfirmDialog
      {...rest}
      title={title ?? t.common.confirm}
      confirmLabel={confirmLabel ?? t.common.confirm}
      cancelLabel={t.common.cancel}
    />
  )
}
