import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { MarkdownRenderer } from '../components/MarkdownRenderer'
import type { WebSource } from '../storage'

function renderMarkdown(content: string, options?: { webSources?: WebSource[]; disableMath?: boolean }): string {
  return renderToStaticMarkup(
    <MarkdownRenderer
      content={content}
      webSources={options?.webSources}
      disableMath={options?.disableMath}
    />,
  )
}

describe('MarkdownRenderer', () => {
  it('应解析大小写混合的 Web: 引用并关联到来源', () => {
    const html = renderMarkdown('参考 Web:1。', {
      webSources: [{ title: 'Example', url: 'https://example.com' }],
    })

    expect(html).toContain('example')
    expect(html).not.toContain('Web:1')
  })

  it('应把连续来源引用聚合为同一个 badge', () => {
    const html = renderMarkdown('来源 web:1, Web:2。', {
      webSources: [
        { title: 'Example', url: 'https://example.com' },
        { title: 'GitHub', url: 'https://github.com' },
      ],
    })

    expect(html).toContain('+1')
  })

  it('不应替换代码片段中的 web: 引用文本', () => {
    const html = renderMarkdown('命令 `web:1` 保持原样。', {
      webSources: [{ title: 'Example', url: 'https://example.com' }],
    })

    expect(html).toContain('<code')
    expect(html).toContain('web:1')
  })

  it('应显示代码块语言标签', () => {
    const pythonHtml = renderMarkdown('```python\nprint("ok")\n```')
    const bashHtml = renderMarkdown('```bash\necho ok\n```')
    const latexCodeHtml = renderMarkdown('```latex\n\\\\frac{a}{b}\n```')
    const textHtml = renderMarkdown('```\nplain text\n```')

    expect(pythonHtml).toContain('>python<')
    expect(bashHtml).toContain('>bash<')
    expect(latexCodeHtml).toContain('>latex<')
    expect(textHtml).toContain('>text<')
  })

  it('应在数学模式开启时渲染 KaTeX，关闭时保持原文', () => {
    const mathEnabled = renderMarkdown('行内公式 $a^2+b^2$')
    const mathDisabled = renderMarkdown('行内公式 $a^2+b^2$', { disableMath: true })
    const rawLatex = renderMarkdown('\\alpha + \\beta')

    expect(mathEnabled).toContain('class="katex"')
    expect(mathDisabled).not.toContain('class="katex"')
    expect(rawLatex).not.toContain('class="katex"')
  })
})
