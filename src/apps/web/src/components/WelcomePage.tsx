import { useCallback, useEffect, useRef, useState, type FormEvent, type KeyboardEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { FileText, Grid2X2, MoreHorizontal, Plus, Users } from 'lucide-react'
import { DebugTrigger } from '@arkloop/shared'
import { createThread, isApiError } from '../api'
import { useAgentClient } from '../agent-ui'
import { buildMessageRequest } from '../messageContent'
import { CJR_DEEPSEEK_FLASH_MODEL, writeActiveThreadIdToStorage, readDeveloperShowDebugPanel } from '../storage'
import { useLocale } from '../contexts/LocaleContext'
import { useAuth } from '../contexts/auth'
import { useThreadList } from '../contexts/thread-list'
import { useCredits } from '../contexts/credits'
import { useSkillPromptUI } from '../contexts/app-ui'
import { ErrorCallout, type AppError } from './ErrorCallout'

const CJR_ACTIONS = [
  { label: '研究', prompt: '研究', Icon: FileText },
  { label: '项目', prompt: '项目', Icon: Grid2X2 },
  { label: '案例', prompt: '案例', Icon: Users },
  { label: '更多', prompt: '更多', Icon: MoreHorizontal },
]

function normalizeError(error: unknown, fallback: string): AppError {
  if (isApiError(error)) {
    return { message: error.message, traceId: error.traceId, code: error.code }
  }
  if (error instanceof Error) {
    return { message: error.message }
  }
  return { message: fallback }
}

function deriveTitle(content: string, defaultTitle: string): string {
  const cleaned = content.trim().replace(/\s+/g, ' ')
  if (!cleaned) return defaultTitle
  return cleaned.length > 40 ? `${cleaned.slice(0, 40)}...` : cleaned
}

export function WelcomePage() {
  const { accessToken, logout: onLoggedOut, me } = useAuth()
  const agentClient = useAgentClient()
  const { addThread: onThreadCreated, isPrivateMode } = useThreadList()
  const { pendingSkillPrompt, consumeSkillPrompt } = useSkillPromptUI()
  const { refreshCredits } = useCredits()
  const { t } = useLocale()
  const navigate = useNavigate()
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const [draft, setDraft] = useState('')
  const [sending, setSending] = useState(false)
  const [error, setError] = useState<AppError | null>(null)
  const [showDebugPanel, setShowDebugPanel] = useState(() => readDeveloperShowDebugPanel())

  useEffect(() => {
    const handleChange = (e: Event) => {
      setShowDebugPanel((e as CustomEvent<boolean>).detail)
    }
    window.addEventListener('arkloop:developer_show_debug_panel', handleChange)
    return () => window.removeEventListener('arkloop:developer_show_debug_panel', handleChange)
  }, [])

  useEffect(() => {
    if (!pendingSkillPrompt) return
    setDraft(pendingSkillPrompt)
    consumeSkillPrompt()
    requestAnimationFrame(() => textareaRef.current?.focus())
  }, [pendingSkillPrompt, consumeSkillPrompt])

  const resizeTextarea = useCallback(() => {
    const textarea = textareaRef.current
    if (!textarea) return
    textarea.style.height = 'auto'
    textarea.style.height = `${Math.min(textarea.scrollHeight, 140)}px`
  }, [])

  useEffect(() => {
    resizeTextarea()
  }, [draft, resizeTextarea])

  const focusWithPrompt = useCallback((prompt: string) => {
    setDraft(prompt)
    requestAnimationFrame(() => textareaRef.current?.focus())
  }, [])

  const handleSubmit = async (e?: FormEvent<HTMLFormElement>) => {
    e?.preventDefault()
    const text = draft.trim()
    if (!text || sending) return

    setSending(true)
    setError(null)

    try {
      const thread = await createThread(accessToken, {
        title: deriveTitle(text, t.newChatTitle),
        is_private: isPrivateMode,
        mode: 'chat',
        collaboration_mode: 'default',
      })
      const userMessage = await agentClient.createMessage({
        threadId: thread.id,
        request: buildMessageRequest(text, []),
      })
      const run = await agentClient.createRun({
        threadId: thread.id,
        modelOverride: CJR_DEEPSEEK_FLASH_MODEL,
      })

      setDraft('')
      refreshCredits()
      writeActiveThreadIdToStorage(thread.id)
      onThreadCreated(thread)
      navigate(`/t/${thread.id}`, {
        state: {
          initialRunId: run.id,
          userEnterMessageId: userMessage.id,
          welcomeUserMessage: userMessage,
        },
      })
    } catch (err) {
      if (isApiError(err) && err.status === 401) {
        onLoggedOut()
        return
      }
      setError(normalizeError(err, t.requestFailed))
    } finally {
      setSending(false)
    }
  }

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key !== 'Enter' || e.shiftKey) return
    e.preventDefault()
    void handleSubmit()
  }

  const userInitial = (me?.username?.trim()?.charAt(0) || 'U').toUpperCase()

  return (
    <div className="cjr-main">
      <div className="cjr-top-bar">
        <div className="cjr-user" title={me?.username ?? undefined}>{userInitial}</div>
      </div>

      <div className="cjr-center-wrap">
        <div className="cjr-hero">
          <h1>AI赋能产教融合高质量发展</h1>
          <div className="cjr-hero-sub">标准引领 · 智能驱动 · 指数赋能</div>
        </div>

        <form className="cjr-input-hero" onSubmit={(e) => void handleSubmit(e)}>
          <div className="cjr-input-box">
            <textarea
              ref={textareaRef}
              rows={1}
              value={draft}
              placeholder="输入产教融合指令，或点击下方按钮..."
              onChange={(e) => setDraft(e.currentTarget.value)}
              onKeyDown={handleKeyDown}
              disabled={sending}
            />
            <div className="cjr-input-actions">
              <button className="cjr-input-icon-btn" type="button" title="附件" onClick={() => textareaRef.current?.focus()}>
                <Plus size={20} strokeWidth={2} />
              </button>
              <button className="cjr-input-send-btn" type="submit" disabled={!draft.trim() || sending} title="发送">
                ↑
              </button>
            </div>
          </div>
        </form>

        <div className="cjr-action-btns">
          {CJR_ACTIONS.map(({ label, prompt, Icon }) => (
            <button key={label} className="cjr-action-btn" type="button" onClick={() => focusWithPrompt(prompt)}>
              <Icon size={16} strokeWidth={2} />
              <span>{label}</span>
            </button>
          ))}
        </div>

        {error && <div className="cjr-error-wrap"><ErrorCallout error={error} /></div>}
      </div>

      {showDebugPanel && <DebugTrigger />}
    </div>
  )
}
