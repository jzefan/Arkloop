import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { AutoResizeTextarea } from '@arkloop/shared'

describe('AutoResizeTextarea', () => {
  let container: HTMLDivElement

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
  })

  afterEach(() => {
    container.remove()
  })

  it('会为多行内容写入高度样式', async () => {
    const root = createRoot(container)
    await act(async () => {
      root.render(
        <AutoResizeTextarea
          value={'a\nb\nc'}
          onChange={() => {}}
          minRows={1}
          style={{ width: '240px', fontSize: '16px', lineHeight: '24px' }}
        />,
      )
    })

    const textarea = container.querySelector('textarea')
    expect(textarea).not.toBeNull()
    expect(textarea?.style.height).toMatch(/px$/)
    root.unmount()
  })
})
