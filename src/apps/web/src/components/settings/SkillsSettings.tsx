import React, { useRef, useState } from 'react'
import { Github, Loader2, Upload } from 'lucide-react'
import {
  importSkillFromGitHub,
  importSkillFromUpload,
  installSkill,
  isApiError,
  listDefaultSkills,
  replaceDefaultSkills,
  type SkillPackageResponse,
  type SkillReference,
} from '../../api'
import { SkillsSettingsContent } from '../SkillsSettingsContent'
import { primaryButtonSmCls, secondaryButtonBorderStyle, secondaryButtonSmCls } from '../buttonStyles'
import { markdownSkillToZip } from '../../lib/skillArchive'

type Props = {
  accessToken: string
  onTrySkill?: (prompt: string) => void
}

class SkillsSettingsErrorBoundary extends React.Component<
  { children: React.ReactNode; fallback: React.ReactNode },
  { hasError: boolean; errorText: string }
> {
  state = { hasError: false, errorText: '' }

  static getDerivedStateFromError(error: unknown) {
    return { hasError: true, errorText: error instanceof Error ? error.message : String(error) }
  }

  render() {
    if (this.state.hasError) {
      return this.props.fallback
    }
    return this.props.children
  }
}

function dedupeSkillRefs(items: SkillReference[]): SkillReference[] {
  const seen = new Set<string>()
  return items.filter((item) => {
    const key = `${item.skill_key}@${item.version}`
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
}

async function enableImportedSkill(accessToken: string, skill: SkillPackageResponse) {
  await installSkill(accessToken, { skill_key: skill.skill_key, version: skill.version })
  const defaults = await listDefaultSkills(accessToken).catch(() => [])
  await replaceDefaultSkills(
    accessToken,
    dedupeSkillRefs([
      ...defaults.map((item) => ({ skill_key: item.skill_key, version: item.version })),
      { skill_key: skill.skill_key, version: skill.version },
    ]),
  )
}

function SkillsSettingsFallback({ accessToken }: { accessToken: string }) {
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [file, setFile] = useState<File | null>(null)
  const [githubUrl, setGithubUrl] = useState('')
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')

  const handleUpload = async () => {
    if (!file || busy) return
    setBusy(true)
    setMessage('')
    setError('')
    try {
      const uploadFile = file.name.toLowerCase().endsWith('.md') ? await markdownSkillToZip(file) : file
      const skill = await importSkillFromUpload(accessToken, { file: uploadFile, install_after_import: false })
      await enableImportedSkill(accessToken, skill)
      setFile(null)
      setMessage(`已导入并启用 Skill：${skill.display_name || skill.skill_key}`)
    } catch (err) {
      setError(isApiError(err) ? err.message : '导入失败')
    } finally {
      setBusy(false)
    }
  }

  const handleGitHubImport = async () => {
    const url = githubUrl.trim()
    if (!url || busy) return
    setBusy(true)
    setMessage('')
    setError('')
    try {
      const response = await importSkillFromGitHub(accessToken, { repository_url: url })
      await enableImportedSkill(accessToken, response.skill)
      setGithubUrl('')
      setMessage(`已导入并启用 Skill：${response.skill.display_name || response.skill.skill_key}`)
    } catch (err) {
      setError(isApiError(err) ? err.message : '导入失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <div
        className="rounded-xl px-4 py-5"
        style={{ border: '0.5px solid var(--c-border-subtle)', background: 'var(--c-bg-menu)' }}
      >
        <h3 className="text-sm font-medium text-[var(--c-text-heading)]">暂无可用 Skill</h3>
        <p className="mt-1 text-xs text-[var(--c-text-muted)]">
          可以上传 Markdown / ZIP Skill，或从 GitHub 导入。导入后会自动启用，之后在对话框输入 ～ 可呼出并用于本轮对话。
        </p>
      </div>

      <div
        className="rounded-xl p-4"
        style={{ border: '0.5px solid var(--c-border-subtle)', background: 'var(--c-bg-menu)' }}
      >
        <div className="flex items-center gap-2">
          <input
            ref={fileInputRef}
            type="file"
            accept=".md,.zip,.skill"
            className="hidden"
            onChange={(event) => setFile(event.target.files?.[0] ?? null)}
          />
          <button
            type="button"
            className={secondaryButtonSmCls}
            style={secondaryButtonBorderStyle}
            onClick={() => fileInputRef.current?.click()}
          >
            <Upload size={14} />
            选择 Skill 文件
          </button>
          <span className="min-w-0 flex-1 truncate text-xs text-[var(--c-text-muted)]">
            {file?.name ?? '支持 .md、.zip、.skill'}
          </span>
          <button
            type="button"
            className={primaryButtonSmCls}
            disabled={!file || busy}
            onClick={() => void handleUpload()}
            style={{ background: 'var(--c-btn-bg)', color: 'var(--c-btn-text)' }}
          >
            {busy ? <Loader2 size={14} className="animate-spin" /> : <Upload size={14} />}
            导入
          </button>
        </div>
      </div>

      <div
        className="rounded-xl p-4"
        style={{ border: '0.5px solid var(--c-border-subtle)', background: 'var(--c-bg-menu)' }}
      >
        <div className="flex items-center gap-2">
          <Github size={14} className="text-[var(--c-text-muted)]" />
          <input
            value={githubUrl}
            onChange={(event) => setGithubUrl(event.target.value)}
            placeholder="https://github.com/org/repo/tree/main/skills/demo"
            className="h-9 min-w-0 flex-1 rounded-lg px-3 text-sm text-[var(--c-text-heading)] outline-none placeholder:text-[var(--c-text-tertiary)]"
            style={{ border: '0.5px solid var(--c-border-subtle)', background: 'var(--c-bg-page)' }}
          />
          <button
            type="button"
            className={primaryButtonSmCls}
            disabled={!githubUrl.trim() || busy}
            onClick={() => void handleGitHubImport()}
            style={{ background: 'var(--c-btn-bg)', color: 'var(--c-btn-text)' }}
          >
            {busy ? <Loader2 size={14} className="animate-spin" /> : <Github size={14} />}
            导入
          </button>
        </div>
      </div>

      {message && <p className="text-xs text-[var(--c-status-ok-text)]">{message}。重新打开设置后可看到列表。</p>}
      {error && <p className="text-xs text-[var(--c-status-error-text)]">{error}</p>}
    </div>
  )
}

export function SkillsSettings({ accessToken, onTrySkill }: Props) {
  return (
    <SkillsSettingsErrorBoundary fallback={<SkillsSettingsFallback accessToken={accessToken} />}>
      <SkillsSettingsContent accessToken={accessToken} onTrySkill={onTrySkill} />
    </SkillsSettingsErrorBoundary>
  )
}
