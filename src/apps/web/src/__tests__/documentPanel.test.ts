import { describe, expect, it } from 'vitest'

import { canPreviewDocumentAsText } from '../components/DocumentPanel'

describe('canPreviewDocumentAsText', () => {
  it('服务端回退成 octet-stream 时，仍应信任 artifact 的文本类型', () => {
    expect(canPreviewDocumentAsText('application/octet-stream', 'text/markdown', 'shell_execute_demo.md')).toBe(true)
  })

  it('应识别 application/markdown 为可预览文本', () => {
    expect(canPreviewDocumentAsText('application/markdown', '', 'shell_execute_demo.md')).toBe(true)
  })

  it('通用二进制类型下，应按 .md 扩展名回退为文本预览', () => {
    expect(canPreviewDocumentAsText('application/octet-stream', '', 'shell_execute_demo.md')).toBe(true)
  })

  it('真正的二进制文件不应误判为文本预览', () => {
    expect(canPreviewDocumentAsText('application/pdf', 'application/pdf', 'report.pdf')).toBe(false)
  })
})
