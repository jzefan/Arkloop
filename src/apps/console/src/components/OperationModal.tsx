import { OperationModal as SharedOperationModal } from '@arkloop/shared'
import type { ComponentProps } from 'react'
import { bridgeClient } from '../api/bridge'

type Props = Omit<ComponentProps<typeof SharedOperationModal>, 'client'>

export function OperationModal(props: Props) {
  return <SharedOperationModal {...props} client={bridgeClient} />
}
