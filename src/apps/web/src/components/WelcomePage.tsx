import { useState, useCallback, useMemo, useRef, useEffect, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { Glasses } from 'lucide-react'
import type { LocaleStrings } from '../locales'
import { ChatInput, type Attachment, type ChatInputHandle } from './ChatInput'
import { ErrorCallout, type AppError } from './ErrorCallout'
import { NotificationBell } from './NotificationBell'
import { isDesktop } from '@arkloop/shared/desktop'
import { DebugTrigger, useTimeZone } from '@arkloop/shared'
import { buildDraftAttachmentRecords, restoreAttachmentFromDraftRecord } from '../draftAttachments'
import { createThread, uploadStagingAttachment, isApiError, type RunReasoningMode } from '../api'
import { useAgentClient } from '../agent-ui'
import {
  type InputDraftScope,
  writeActiveThreadIdToStorage,
  addSearchThreadId,
  SEARCH_PERSONA_KEY,
  DEFAULT_PERSONA_KEY,
  writeSelectedPersonaKeyToStorage,
  transferGlobalWorkFolderToThread,
  transferGlobalThinkingToThread,
  readSelectedReasoningMode,
  readInputDraftAttachments,
  readWorkFolder,
  readDeveloperShowDebugPanel,
  writeInputDraftAttachments,
} from '../storage'
import { useLocale } from '../contexts/LocaleContext'
import { buildMessageRequest } from '../messageContent'
import { useAuth } from '../contexts/auth'
import { useThreadList } from '../contexts/thread-list'
import {
  useAppModeUI,
  useNotificationsUI,
  useSearchUI,
  useSettingsUI,
  useSkillPromptUI,
} from '../contexts/app-ui'
import { useCredits } from '../contexts/credits'

type AgentEntry = {
  id: string
  personaKey: string
  name: string
  description: string
  color: string
  bg: string
  comingSoon: boolean
}

type ShowcaseTab = {
  key: string
  label: string
  agents: AgentEntry[]
}

const SHOWCASE_TABS: ShowcaseTab[] = [
  {
    key: 'industry',
    label: '产教融合',
    agents: [
      { id: 'pj', personaKey: 'industry-education-index', name: '双高产教融合评估', description: '评估双高院校产教融合建设水平', color: '#3B5BDB', bg: '#EDF0FF', comingSoon: false },
      { id: 'gw', personaKey: 'job-competency-model',      name: '岗位能力模型',     description: '构建岗位胜任力素质画像',     color: '#0CA678', bg: '#E6F8F1', comingSoon: true },
      { id: 'cy', personaKey: 'industry-college',          name: '产业学院',         description: '产业学院规划与运营辅助',     color: '#E8590C', bg: '#FFF1E6', comingSoon: true },
      { id: 'xc', personaKey: 'field-engineer',            name: '现场工程师',       description: '现场工程师培养方案设计',     color: '#7048E8', bg: '#F0EBFF', comingSoon: true },
    ],
  },
  {
    key: 'education',
    label: '教育',
    agents: [
      { id: 'xx', personaKey: '', name: '学习辅导', description: '先理解你的困惑，验证解法后从直觉出发逐步讲清楚', color: '#D6336C', bg: '#FFE9F0', comingSoon: false },
    ],
  },
]

/* color-mix 辅助 — 用于 SVG 渐变色 */
const cmix  = (c: string, r: number) => `color-mix(in oklab, ${c}, white ${r}%)`
const cdeep = (c: string, r: number) => `color-mix(in oklab, ${c}, black ${r}%)`

/* SVG 封面插图（Dribbble 风格，渐变 + 阴影） */
function CoverPj({ color, bg }: { color: string; bg: string }) {
  return (
    <svg viewBox="0 0 280 120" preserveAspectRatio="xMidYMid slice" style={{ width: '100%', height: '100%', display: 'block' }}>
      <defs>
        <linearGradient id="pj-bg" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor={bg} />
          <stop offset="1" stopColor={cmix(color, 88)} />
        </linearGradient>
        <filter id="pj-shadow" x="-20%" y="-20%" width="140%" height="140%">
          <feDropShadow dx="0" dy="2" stdDeviation="3" floodColor={color} floodOpacity="0.18"/>
        </filter>
      </defs>
      <rect width="280" height="120" fill="url(#pj-bg)"/>
      <circle cx="240" cy="20" r="55" fill={color} opacity="0.16"/>
      <circle cx="40"  cy="110" r="40" fill={color} opacity="0.12"/>
      <g transform="translate(60 22)" filter="url(#pj-shadow)">
        <rect width="160" height="74" rx="8" fill="#fff"/>
        <circle cx="14" cy="14" r="4" fill={color} opacity="0.35"/>
        <rect x="24" y="11" width="60" height="6" rx="3" fill={color} opacity="0.18"/>
        <rect x="130" y="9" width="22" height="10" rx="2" fill={color} opacity="0.18"/>
        <rect x="14" y="48" width="14" height="16" rx="2" fill={color} opacity="0.35"/>
        <rect x="32" y="38" width="14" height="26" rx="2" fill={color} opacity="0.55"/>
        <rect x="50" y="28" width="14" height="36" rx="2" fill={color}/>
        <rect x="68" y="44" width="14" height="20" rx="2" fill={color} opacity="0.4"/>
        <rect x="86" y="32" width="14" height="32" rx="2" fill={color} opacity="0.7"/>
        <path d="M110 56 L 124 44 L 140 50 L 152 32" stroke={color} strokeWidth="2" fill="none" strokeLinecap="round" strokeLinejoin="round"/>
        <circle cx="152" cy="32" r="2.5" fill={color}/>
      </g>
      <g transform="translate(228 78)">
        <circle r="14" fill="#fff" opacity="0.9"/>
        <path d="M-6 0 L-2 4 L6 -4" stroke={color} strokeWidth="2.2" fill="none" strokeLinecap="round" strokeLinejoin="round"/>
      </g>
      <circle cx="36" cy="32" r="3" fill={color}/>
      <circle cx="48" cy="22" r="1.5" fill={color} opacity="0.6"/>
    </svg>
  )
}

function CoverGw({ color, bg }: { color: string; bg: string }) {
  return (
    <svg viewBox="0 0 280 120" preserveAspectRatio="xMidYMid slice" style={{ width: '100%', height: '100%', display: 'block' }}>
      <defs>
        <linearGradient id="gw-bg" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor={bg} />
          <stop offset="1" stopColor={cmix(color, 86)} />
        </linearGradient>
        <radialGradient id="gw-core" cx="0.3" cy="0.3">
          <stop offset="0" stopColor={cmix(color, 30)}/>
          <stop offset="1" stopColor={color}/>
        </radialGradient>
      </defs>
      <rect width="280" height="120" fill="url(#gw-bg)"/>
      <circle cx="40"  cy="20"   r="48" fill={color} opacity="0.12"/>
      <circle cx="250" cy="110"  r="40" fill={color} opacity="0.1"/>
      <g transform="translate(140 60)">
        <ellipse rx="64" ry="34" fill="none" stroke={color} strokeOpacity="0.25" strokeWidth="1"/>
        <ellipse rx="46" ry="22" fill="none" stroke={color} strokeOpacity="0.35" strokeWidth="1" transform="rotate(-18)"/>
        <circle cx="-64" cy="0"   r="6" fill="#fff" stroke={color} strokeWidth="1.5"/>
        <circle cx="48"  cy="-18" r="7" fill={color} opacity="0.85"/>
        <circle cx="58"  cy="14"  r="5" fill="#fff" stroke={color} strokeWidth="1.5"/>
        <circle cx="-30" cy="-26" r="5" fill={color} opacity="0.55"/>
        <circle cx="-44" cy="20"  r="4" fill="#fff" stroke={color} strokeWidth="1.5"/>
        <circle r="18" fill="url(#gw-core)"/>
        <circle r="22" fill="none" stroke="#fff" strokeOpacity="0.5" strokeWidth="2"/>
        <circle cy="-4" r="4" fill="#fff"/>
        <path d="M-7 10 Q 0 2 7 10 Z" fill="#fff"/>
      </g>
      <circle cx="208" cy="30" r="2"   fill={color}/>
      <circle cx="72"  cy="92" r="2.5" fill={color} opacity="0.7"/>
    </svg>
  )
}

function CoverCy({ color, bg }: { color: string; bg: string }) {
  const light = cmix(color, 35)
  const dark  = cdeep(color, 15)
  return (
    <svg viewBox="0 0 280 120" preserveAspectRatio="xMidYMid slice" style={{ width: '100%', height: '100%', display: 'block' }}>
      <defs>
        <linearGradient id="cy-bg" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor={bg} />
          <stop offset="1" stopColor={cmix(color, 88)} />
        </linearGradient>
      </defs>
      <rect width="280" height="120" fill="url(#cy-bg)"/>
      <circle cx="240" cy="20"  r="46" fill={color} opacity="0.14"/>
      <circle cx="30"  cy="100" r="34" fill={color} opacity="0.1"/>
      <g transform="translate(110 28)">
        <ellipse cx="44" cy="82" rx="56" ry="6" fill={color} opacity="0.18"/>
        <path d="M 12 22 L 12 70 L 42 84 L 42 36 Z" fill={color}/>
        <path d="M 42 36 L 42 84 L 70 70 L 70 22 Z" fill={dark}/>
        <path d="M 12 22 L 42 8 L 70 22 L 42 36 Z" fill={light}/>
        {[0,1,2,3].map(i => <rect key={`wl${i}`} x="18" y={32+i*10} width="6" height="5" fill="#fff" opacity="0.55"/>)}
        {[0,1,2,3].map(i => <rect key={`wm${i}`} x="28" y={37+i*10} width="6" height="5" fill="#fff" opacity="0.55"/>)}
        {[0,1,2,3].map(i => <rect key={`wr${i}`} x="50" y={36+i*10} width="6" height="5" fill="#fff" opacity="0.4"/>)}
        {[0,1,2,3].map(i => <rect key={`wr2${i}`} x="60" y={31+i*10} width="6" height="5" fill="#fff" opacity="0.4"/>)}
        <g transform="translate(56 24)">
          <path d="M 0 24 L 0 58 L 28 70 L 28 38 Z" fill={light}/>
          <path d="M 28 38 L 28 70 L 50 60 L 50 28 Z" fill={color}/>
          <path d="M 0 24 L 28 10 L 50 28 L 28 38 Z" fill="#fff" opacity="0.92"/>
          <path d="M 12 50 L 12 60 L 18 62 L 18 52 Z" fill={dark} opacity="0.7"/>
          <rect x="34" y="42" width="6" height="5" fill="#fff" opacity="0.6"/>
          <rect x="42" y="38" width="6" height="5" fill="#fff" opacity="0.6"/>
        </g>
      </g>
      <circle cx="60" cy="34" r="3"   fill={color}/>
      <circle cx="76" cy="46" r="1.6" fill={color} opacity="0.6"/>
    </svg>
  )
}

function CoverXc({ color, bg }: { color: string; bg: string }) {
  const cx = 90, cy = 60, rO = 40, rI = 32, teeth = 12
  let d = ''
  for (let i = 0; i < teeth * 2; i++) {
    const a = (i / (teeth * 2)) * Math.PI * 2 - Math.PI / 2
    const rr = i % 2 === 0 ? rO : rI
    d += (i === 0 ? 'M' : 'L') + (cx + Math.cos(a) * rr).toFixed(1) + ' ' + (cy + Math.sin(a) * rr).toFixed(1) + ' '
  }
  d += 'Z'
  const smallTeeth = 8, sRo = 12, sRi = 9
  let sd = ''
  for (let i = 0; i < smallTeeth * 2; i++) {
    const a = (i / (smallTeeth * 2)) * Math.PI * 2 - Math.PI / 2
    const rr = i % 2 === 0 ? sRo : sRi
    sd += (i === 0 ? 'M' : 'L') + (Math.cos(a) * rr).toFixed(1) + ' ' + (Math.sin(a) * rr).toFixed(1) + ' '
  }
  sd += 'Z'
  return (
    <svg viewBox="0 0 280 120" preserveAspectRatio="xMidYMid slice" style={{ width: '100%', height: '100%', display: 'block' }}>
      <defs>
        <linearGradient id="xc-bg" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor={bg} />
          <stop offset="1" stopColor={cmix(color, 88)} />
        </linearGradient>
        <radialGradient id="xc-gear" cx="0.35" cy="0.35">
          <stop offset="0" stopColor={cmix(color, 25)}/>
          <stop offset="1" stopColor={color}/>
        </radialGradient>
        <pattern id="xc-grid" width="14" height="14" patternUnits="userSpaceOnUse">
          <path d="M 14 0 L 0 0 0 14" fill="none" stroke={color} strokeOpacity="0.18" strokeWidth="1"/>
        </pattern>
      </defs>
      <rect width="280" height="120" fill="url(#xc-bg)"/>
      <rect x="150" y="0" width="130" height="120" fill="url(#xc-grid)"/>
      <circle cx="245" cy="20" r="40" fill={color} opacity="0.12"/>
      <path d={d} fill="url(#xc-gear)"/>
      <circle cx={cx} cy={cy} r="12" fill={bg}/>
      <circle cx={cx} cy={cy} r="5"  fill={color}/>
      <g transform="translate(60 100)">
        <path d={sd} fill={color} opacity="0.55"/>
        <circle r="4" fill={bg}/>
      </g>
      <g stroke={color} strokeWidth="2" fill="none" strokeLinecap="round" strokeLinejoin="round">
        <path d="M154 32 H 188 V 18 H 232"/>
        <path d="M154 60 H 210"/>
        <path d="M154 88 H 188 V 102 H 244"/>
      </g>
      <g fill={color}>
        <circle cx="232" cy="18"  r="3.5"/>
        <circle cx="210" cy="60"  r="3.5"/>
        <circle cx="244" cy="102" r="3.5"/>
      </g>
      <circle cx="232" cy="18"  r="6" fill="none" stroke={color} strokeOpacity="0.4" strokeWidth="1"/>
      <circle cx="244" cy="102" r="6" fill="none" stroke={color} strokeOpacity="0.4" strokeWidth="1"/>
    </svg>
  )
}

function CoverXx({ color, bg }: { color: string; bg: string }) {
  const light = cmix(color, 40)
  const dark  = cdeep(color, 10)
  return (
    <svg viewBox="0 0 280 120" preserveAspectRatio="xMidYMid slice" style={{ width: '100%', height: '100%', display: 'block' }}>
      <defs>
        <linearGradient id="xx-bg" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor={bg} />
          <stop offset="1" stopColor={cmix(color, 88)} />
        </linearGradient>
        <filter id="xx-shadow" x="-20%" y="-20%" width="140%" height="140%">
          <feDropShadow dx="0" dy="3" stdDeviation="4" floodColor={color} floodOpacity="0.22"/>
        </filter>
      </defs>
      <rect width="280" height="120" fill="url(#xx-bg)"/>
      <circle cx="40"  cy="20"  r="44" fill={color} opacity="0.14"/>
      <circle cx="240" cy="100" r="36" fill={color} opacity="0.1"/>
      <g transform="translate(90 30) rotate(-6 50 30)" filter="url(#xx-shadow)">
        <path d="M 4 6 Q 50 -2 50 8 L 50 60 Q 50 50 4 58 Z"  fill="#fff"/>
        <path d="M 96 6 Q 50 -2 50 8 L 50 60 Q 50 50 96 58 Z" fill="#fff"/>
        <path d="M 50 8 L 50 60" stroke={color} strokeOpacity="0.25" strokeWidth="1"/>
        <g stroke={color} strokeOpacity="0.45" strokeWidth="1.4" strokeLinecap="round">
          <line x1="12" y1="20" x2="42" y2="14"/>
          <line x1="12" y1="28" x2="42" y2="22"/>
          <line x1="12" y1="36" x2="36" y2="30"/>
          <line x1="58" y1="14" x2="88" y2="20"/>
          <line x1="58" y1="22" x2="88" y2="28"/>
          <line x1="58" y1="30" x2="82" y2="36"/>
        </g>
        <path d="M 4 6 Q 28 2 50 6 L 50 14 Q 28 12 4 16 Z"  fill={light}/>
        <path d="M 96 6 Q 72 2 50 6 L 50 14 Q 72 12 96 16 Z" fill={color} opacity="0.7"/>
      </g>
      <g transform="translate(210 32) rotate(15)">
        <path d="M -16 0 L 0 -8 L 16 0 L 0 8 Z" fill={dark}/>
        <path d="M -10 4 L -10 12 Q 0 16 10 12 L 10 4" fill={color} opacity="0.85"/>
        <circle cx="14" cy="-2" r="1.5" fill={color}/>
        <path d="M 14 -2 Q 18 4 16 8" stroke={color} strokeWidth="1.2" fill="none"/>
      </g>
      <g fill={color}>
        <path d="M44 80 L46 85 L51 87 L46 89 L44 94 L42 89 L37 87 L42 85 Z" opacity="0.75"/>
        <circle cx="230" cy="80" r="2"/>
        <circle cx="60"  cy="40" r="1.6" opacity="0.7"/>
      </g>
    </svg>
  )
}

const COVER_MAP: Record<string, React.ComponentType<{ color: string; bg: string }>> = {
  pj: CoverPj,
  gw: CoverGw,
  cy: CoverCy,
  xc: CoverXc,
  xx: CoverXx,
}

function AgentShowcase({ onSelect }: { onSelect: (personaKey: string) => void }) {
  const [activeTab, setActiveTab] = useState(SHOWCASE_TABS[0].key)
  const currentTab = SHOWCASE_TABS.find((t) => t.key === activeTab) ?? SHOWCASE_TABS[0]
  const visible = currentTab.agents.slice(0, 3)
  const placeholders = Math.max(0, 3 - visible.length)

  return (
    <div className="w-full max-w-[675px] mt-8">
      {/* Tab 头部 */}
      <div className="flex items-center justify-between mb-[18px]">
        <div className="flex items-center gap-[22px]">
          {SHOWCASE_TABS.map((tab) => {
            const active = tab.key === activeTab
            return (
              <button
                key={tab.key}
                onClick={() => setActiveTab(tab.key)}
                className="relative pb-[7px] transition-colors"
                style={{
                  fontSize: 15,
                  fontWeight: active ? 600 : 400,
                  color: active ? 'var(--c-text-primary)' : 'var(--c-text-tertiary)',
                  background: 'transparent',
                  border: 'none',
                  cursor: 'pointer',
                }}
              >
                {tab.label}
                {active && (
                  <span
                    className="absolute left-1/2 -translate-x-1/2"
                    style={{ bottom: 0, width: 4, height: 4, borderRadius: '50%', background: 'var(--c-text-primary)', display: 'block' }}
                  />
                )}
              </button>
            )
          })}
        </div>
        <a
          href="#"
          className="flex items-center gap-1 text-[13px] transition-colors"
          style={{ color: 'var(--c-text-tertiary)', textDecoration: 'none' }}
        >
          全部 <span style={{ fontSize: 14 }}>→</span>
        </a>
      </div>

      {/* 卡片网格 */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 14 }}>
        {visible.map((agent) => {
          const Cover = COVER_MAP[agent.id]
          const coverColor = agent.comingSoon ? '#A8A6A0' : agent.color
          const coverBg   = agent.comingSoon ? '#EFEDE9' : agent.bg
          return (
            <button
              key={agent.id}
              disabled={agent.comingSoon}
              onClick={() => !agent.comingSoon && onSelect(agent.personaKey)}
              className="text-left transition-all"
              style={{
                background: 'var(--c-bg-input)',
                border: '1px solid var(--c-border)',
                borderRadius: 12,
                padding: 0,
                cursor: agent.comingSoon ? 'not-allowed' : 'pointer',
                opacity: agent.comingSoon ? 0.62 : 1,
                position: 'relative',
                overflow: 'hidden',
                fontFamily: 'inherit',
              }}
              onMouseEnter={(e) => {
                if (!agent.comingSoon) {
                  e.currentTarget.style.borderColor = 'var(--c-border-mid)'
                  e.currentTarget.style.boxShadow = '0 4px 12px rgba(20,20,20,0.05)'
                }
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.borderColor = 'var(--c-border)'
                e.currentTarget.style.boxShadow = 'none'
              }}
            >
              {/* 封面插图 */}
              <div style={{ height: 124, borderBottom: '1px solid var(--c-border-subtle)' }}>
                {Cover
                  ? <Cover color={coverColor} bg={coverBg} />
                  : <div style={{ height: '100%', background: coverBg }} />}
              </div>
              {/* 即将上线徽章 */}
              {agent.comingSoon && (
                <span style={{
                  position: 'absolute', top: 10, right: 10,
                  fontSize: 10.5, padding: '2px 8px', borderRadius: 999,
                  background: 'rgba(255,255,255,0.92)',
                  color: 'var(--c-text-secondary)',
                  backdropFilter: 'blur(4px)',
                  letterSpacing: '0.02em',
                }}>即将上线</span>
              )}
              {/* 文字区 */}
              <div style={{ padding: '14px 16px 16px' }}>
                <div style={{ fontSize: 14, fontWeight: 500, color: 'var(--c-text-primary)', marginBottom: 5 }}>
                  {agent.name}
                </div>
                <div style={{
                  fontSize: 12.5, color: 'var(--c-text-secondary)', lineHeight: 1.5,
                  display: '-webkit-box', WebkitBoxOrient: 'vertical', WebkitLineClamp: 2, overflow: 'hidden',
                } as React.CSSProperties}>
                  {agent.description}
                </div>
              </div>
            </button>
          )
        })}
        {/* 占位空卡片 */}
        {Array.from({ length: placeholders }).map((_, i) => (
          <div key={`ph-${i}`} style={{
            border: '1px dashed var(--c-border)',
            borderRadius: 12,
            background: 'transparent',
            minHeight: 200,
          }} />
        ))}
      </div>
    </div>
  )
}

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
  return cleaned.length > 40 ? `${cleaned.slice(0, 40)}…` : cleaned
}

type GreetingParts = {
  hour: number
  month: number
  day: number
  weekday: number
  minute: number
}

type WelcomeGreetingTexts = LocaleStrings['welcomeGreeting']

function isSameDraftDomain(left: InputDraftScope | null, right: InputDraftScope): boolean {
  if (!left) return false
  return left.page === right.page
    && (left.threadId ?? null) === (right.threadId ?? null)
    && left.appMode === right.appMode
    && !!left.searchMode === !!right.searchMode
}

function getGreetingParts(now: Date, timeZone: string): GreetingParts {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone,
    hour: '2-digit',
    minute: '2-digit',
    month: 'numeric',
    day: 'numeric',
    weekday: 'short',
    hour12: false,
  }).formatToParts(now)
  const getPart = (type: string) => parts.find((part) => part.type === type)?.value ?? '0'
  const weekdayLabel = getPart('weekday')
  const weekdayMap: Record<string, number> = {
    Sun: 0,
    Mon: 1,
    Tue: 2,
    Wed: 3,
    Thu: 4,
    Fri: 5,
    Sat: 6,
  }
  return {
    hour: Number(getPart('hour')),
    minute: Number(getPart('minute')),
    month: Number(getPart('month')) - 1,
    day: Number(getPart('day')),
    weekday: weekdayMap[weekdayLabel] ?? 0,
  }
}

function buildGreeting(strings: WelcomeGreetingTexts, name: string | null, now: GreetingParts): string {
  const { hour, month, day, weekday, minute } = now

  const first = name ? name.split(/[\s_]+/)[0] : null

  // 节日优先
  if (month === 11 && day >= 24 && day <= 26) return strings.merryChristmas(first)
  if (month === 0 && day === 1) return strings.happyNewYear(first)
  if (month === 1 && day >= 9 && day <= 15) return strings.happyLunarNewYear(first)

  // 周一激励
  if (weekday === 1 && hour >= 8 && hour < 12) {
    return strings.mondayMorning(first)
  }

  // 周五
  if (weekday === 5 && hour >= 15) {
    return strings.fridayAfternoon(first)
  }

  // 深夜
  if (hour >= 0 && hour < 5) {
    return strings.lateNight(first)
  }

  // 时段问候池，每个时段多条随机，避免每次一样
  const pools: Record<string, string[]> = {
    morning: strings.morning(first),
    afternoon: strings.afternoon(first),
    evening: strings.evening(first),
    generic: strings.generic(first),
  }

  let pool: string[]
  if (hour >= 5 && hour < 12) pool = pools.morning
  else if (hour >= 12 && hour < 18) pool = pools.afternoon
  else if (hour >= 18 && hour < 24) pool = pools.evening
  else pool = pools.generic

  // 用分钟做伪随机 seed，同一分钟内刷新不跳
  const seed = minute + hour * 60
  return pool[seed % pool.length]
}



export function WelcomePage() {
  const { accessToken, logout: onLoggedOut, me } = useAuth()
  const agentClient = useAgentClient()
  const { timeZone } = useTimeZone()
  const { addThread: onThreadCreated, isPrivateMode, togglePrivateMode: onTogglePrivateMode } = useThreadList()
  const { isSearchMode, enterSearchMode: onEnterSearchMode, exitSearchMode: onExitSearchMode } = useSearchUI()
  const { openNotifications: onOpenNotifications, notificationVersion } = useNotificationsUI()
  const { openSettings: onOpenSettings } = useSettingsUI()
  const { appMode } = useAppModeUI()
  const { pendingSkillPrompt, consumeSkillPrompt } = useSkillPromptUI()
  const { refreshCredits } = useCredits()
  const [showDebugPanel, setShowDebugPanel] = useState(() => readDeveloperShowDebugPanel())
  const chatInputRef = useRef<ChatInputHandle>(null)

  // 每次进入首页时，重置已选智能体为默认（不预选双高等特殊智能体）
  useEffect(() => {
    writeSelectedPersonaKeyToStorage(DEFAULT_PERSONA_KEY)
    chatInputRef.current?.selectPersona(DEFAULT_PERSONA_KEY)
  }, [])
  const [attachments, setAttachments] = useState<Attachment[]>([])
  const [initialPlanMode, setInitialPlanMode] = useState(false)
  const [initialLearningModeEnabled, setInitialLearningModeEnabled] = useState(false)
  const attachmentsRef = useRef<Attachment[]>([])
  const skipAttachmentDraftPersistRef = useRef(false)
  const prevAttachmentDraftScopeRef = useRef<InputDraftScope | null>(null)
  const [sending, setSending] = useState(false)
  const [error, setError] = useState<AppError | null>(null)
  const navigate = useNavigate()
  const { t } = useLocale()

  const greeting = useMemo(
    () => buildGreeting(t.welcomeGreeting, me?.username ?? null, getGreetingParts(new Date(), timeZone)),
    [me?.username, t.welcomeGreeting, timeZone],
  )
  const draftScope = useMemo<InputDraftScope>(() => ({
    ownerKey: me?.id,
    page: 'welcome',
    appMode: appMode === 'work' ? 'work' : 'chat',
    searchMode: isSearchMode,
  }), [appMode, isSearchMode, me?.id])
  const draftScopeKey = useMemo(() => JSON.stringify(draftScope), [draftScope])

  useEffect(() => {
    const handleChange = (e: Event) => {
      setShowDebugPanel((e as CustomEvent<boolean>).detail)
    }
    window.addEventListener('arkloop:developer_show_debug_panel', handleChange)
    return () => window.removeEventListener('arkloop:developer_show_debug_panel', handleChange)
  }, [])

  useEffect(() => {
    if (pendingSkillPrompt) {
      chatInputRef.current?.setValue(pendingSkillPrompt)
      consumeSkillPrompt()
    }
  }, [pendingSkillPrompt, consumeSkillPrompt])

  const [typedGreeting, setTypedGreeting] = useState('')
  useEffect(() => {
    setTypedGreeting('')
    if (isPrivateMode) return
    let i = 0
    const id = setInterval(() => {
      i++
      if (i > greeting.length) { clearInterval(id); return }
      setTypedGreeting(greeting.slice(0, i))
    }, 45)
    return () => clearInterval(id)
  }, [greeting, isPrivateMode])

  const [typedIncognito, setTypedIncognito] = useState('')
  useEffect(() => {
    setTypedIncognito('')
    if (!isPrivateMode) return
    let i = 0
    const text = t.youAreIncognito
    const id = setInterval(() => {
      i++
      if (i > text.length) { clearInterval(id); return }
      setTypedIncognito(text.slice(0, i))
    }, 55)
    return () => clearInterval(id)
  }, [isPrivateMode, t.youAreIncognito])

  const revokeDraftAttachment = useCallback((attachment: Attachment) => {
    if (attachment.preview_url) URL.revokeObjectURL(attachment.preview_url)
  }, [])

  useEffect(() => {
    const prevScope = prevAttachmentDraftScopeRef.current
    const storedAttachments = readInputDraftAttachments(draftScope)
    const shouldMigrateCurrent =
      isSameDraftDomain(prevScope, draftScope)
      && prevScope?.ownerKey !== draftScope.ownerKey
      && storedAttachments.length === 0
      && attachmentsRef.current.length > 0
    const nextAttachments = shouldMigrateCurrent
      ? buildDraftAttachmentRecords(attachmentsRef.current)
      : storedAttachments
    if (shouldMigrateCurrent) {
      writeInputDraftAttachments(draftScope, nextAttachments)
    }
    prevAttachmentDraftScopeRef.current = draftScope
    skipAttachmentDraftPersistRef.current = true
    setAttachments((prev) => {
      prev.forEach((attachment) => revokeDraftAttachment(attachment))
      return nextAttachments.map(restoreAttachmentFromDraftRecord)
    })
  }, [draftScope, draftScopeKey, revokeDraftAttachment])

  useEffect(() => {
    if (skipAttachmentDraftPersistRef.current) {
      skipAttachmentDraftPersistRef.current = false
      return
    }
    writeInputDraftAttachments(draftScope, buildDraftAttachmentRecords(attachments))
  }, [attachments, draftScope, draftScopeKey])

  useEffect(() => {
    attachmentsRef.current = attachments
  }, [attachments])

  useEffect(() => {
    return () => {
      attachmentsRef.current.forEach((attachment) => revokeDraftAttachment(attachment))
    }
  }, [revokeDraftAttachment])

  const handleAttachFiles = useCallback((files: File[]) => {
    const newAttachments = files.map((file) => ({
      id: `${file.name}-${file.size}-${file.lastModified}`,
      file,
      name: file.name,
      size: file.size,
      mime_type: file.type || 'application/octet-stream',
      preview_url: file.type.startsWith('image/') ? URL.createObjectURL(file) : undefined,
      status: 'uploading' as const,
    }))
    if (newAttachments.length === 0) return
    setAttachments((prev) => {
      const existingIDs = new Set(prev.map((item) => item.id))
      const deduped = newAttachments.filter((item) => !existingIDs.has(item.id))
      return [...prev, ...deduped]
    })
    for (const att of newAttachments) {
      uploadStagingAttachment(accessToken, att.file)
        .then((uploaded) => {
          setAttachments((prev) =>
            prev.map((a) => a.id === att.id ? { ...a, status: 'ready' as const, uploaded } : a),
          )
        })
        .catch(() => {
          setAttachments((prev) =>
            prev.map((a) => a.id === att.id ? { ...a, status: 'error' as const } : a),
          )
        })
    }
  }, [accessToken])

  const handlePasteContent = useCallback((text: string) => {
    const ts = Math.floor(Date.now() / 1000)
    const filename = `pasted-${ts}.txt`
    const blob = new Blob([text], { type: 'text/plain' })
    const file = new File([blob], filename, { type: 'text/plain', lastModified: Date.now() })
    const lineCount = text.split('\n').length
    const att: Attachment = {
      id: `${filename}-${file.size}-${Date.now()}`,
      file,
      name: filename,
      size: file.size,
      mime_type: 'text/plain',
      status: 'uploading',
      pasted: { text, lineCount },
    }
    setAttachments((prev) => [...prev, att])
    uploadStagingAttachment(accessToken, file)
      .then((uploaded) => {
        setAttachments((prev) =>
          prev.map((a) => a.id === att.id ? { ...a, status: 'ready' as const, uploaded } : a),
        )
      })
      .catch(() => {
        setAttachments((prev) =>
          prev.map((a) => a.id === att.id ? { ...a, status: 'error' as const } : a),
        )
      })
  }, [accessToken])

  const handleRemoveAttachment = useCallback((id: string) => {
    setAttachments((prev) => {
      const target = prev.find((item) => item.id === id)
      if (target) revokeDraftAttachment(target)
      return prev.filter((item) => item.id !== id)
    })
  }, [revokeDraftAttachment])

  const handleAsrError = useCallback((err: unknown) => {
    if (isApiError(err) && err.status === 401) {
      onLoggedOut()
      return
    }
    setError(normalizeError(err, t.requestFailed))
  }, [onLoggedOut, t.requestFailed])

  const handleTogglePlanMode = useCallback(async (_currentMode: boolean) => {
    if (appMode !== 'work') return
    setInitialPlanMode((prev) => !prev)
  }, [appMode])

  const handleToggleLearningMode = useCallback(async (_currentMode: boolean) => {
    setInitialLearningModeEnabled((prev) => !prev)
  }, [])

  const handleSubmit = async (e: FormEvent<HTMLFormElement>, personaKey: string, modelOverride?: string) => {
    e.preventDefault()
    const text = (chatInputRef.current?.getValue() ?? '').trim()
    if ((!text && attachments.length === 0) || sending) return

    setSending(true)
    setError(null)

    try {
      const title = deriveTitle(text, t.newChatTitle)
      const thread = await createThread(accessToken, {
        title,
        is_private: isPrivateMode,
        mode: appMode === 'work' ? 'work' : 'chat',
        collaboration_mode: appMode === 'work' && initialPlanMode ? 'plan' : 'default',
        learning_mode_enabled: initialLearningModeEnabled,
      })
      const uploaded = await Promise.all(
        attachments.map(async (attachment) => {
          if (attachment.uploaded) return attachment.uploaded
          if (!attachment.file) throw new Error('attachment file missing')
          return await uploadStagingAttachment(accessToken, attachment.file)
        }),
      )
      const userMessage = await agentClient.createMessage({
        threadId: thread.id,
        request: buildMessageRequest(text, uploaded),
      })
      const run = await agentClient.createRun({
        threadId: thread.id,
        personaId: personaKey,
        modelOverride,
        workDir: readWorkFolder() ?? undefined,
        reasoningMode: readSelectedReasoningMode() !== 'off' ? readSelectedReasoningMode() as RunReasoningMode : undefined,
      })

      if (personaKey === SEARCH_PERSONA_KEY) addSearchThreadId(thread.id)
      attachments.forEach((attachment) => revokeDraftAttachment(attachment))
      chatInputRef.current?.clear()
      setAttachments([])
      refreshCredits()
      writeActiveThreadIdToStorage(thread.id)
      if (appMode === 'work') transferGlobalWorkFolderToThread(thread.id)
      transferGlobalThinkingToThread(thread.id)
      onThreadCreated(thread)
      navigate(`/t/${thread.id}`, {
        state: {
          initialRunId: run.id,
          isSearch: personaKey === SEARCH_PERSONA_KEY,
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

  return (
    <div className="flex h-full flex-col">
      {/* 顶部 header */}
      <div className="relative z-10 flex min-h-[51px] items-center justify-end gap-2 px-[15px] py-[15px]">
        {!isDesktop() && (
          <NotificationBell accessToken={accessToken} onClick={onOpenNotifications} refreshKey={notificationVersion} title={t.notificationsTitle} />
        )}
        {!isDesktop() && (
          <button
            onClick={onTogglePrivateMode}
            title={isPrivateMode ? t.disableIncognito : t.enableIncognito}
            className={[
              'flex h-8 w-8 items-center justify-center rounded-lg transition-colors',
              isPrivateMode
                ? 'bg-[var(--c-bg-deep)] text-[var(--c-text-primary)]'
                : 'text-[var(--c-text-secondary)] hover:bg-[var(--c-bg-deep)] hover:text-[var(--c-text-primary)]',
            ].join(' ')}
          >
            <Glasses size={18} />
          </button>
        )}
      </div>

      {/* 居中内容 — paddingTop 带过渡动画，模式切换时平滑移动 */}
      <div
        className="flex flex-1 flex-col items-center px-5 pb-8 overflow-y-auto"
        style={{
          paddingTop: appMode === 'work' ? '32vh' : '14vh',
          transition: 'padding-top 0.38s cubic-bezier(0.16, 1, 0.3, 1)',
        }}
      >
        {/* 标题：三层绝对定位交叉淡出 */}
        <div className="mb-[40px]" style={{ position: 'relative', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          {/* 常规问候 / 无痕文本 */}
          <h2
            className="relative whitespace-nowrap text-[40px] font-normal tracking-[-0.5px] text-[var(--c-text-heading)]"
            style={{
              opacity: (isSearchMode || appMode === 'work') ? 0 : 1,
              transform: (isSearchMode || appMode === 'work') ? 'translateY(-6px)' : 'translateY(0)',
              transition: 'opacity 0.22s ease, transform 0.24s ease',
              pointerEvents: (isSearchMode || appMode === 'work') ? 'none' : 'auto',
            }}
          >
            <span className="invisible select-none" aria-hidden="true">
              {isPrivateMode ? t.youAreIncognito : greeting}
            </span>
            <span className="absolute inset-0">
              {isPrivateMode ? typedIncognito : typedGreeting}
            </span>
          </h2>
          {/* Search for everything */}
          <h2
            className="absolute text-[40px] font-normal tracking-[-0.5px] text-[var(--c-text-heading)]"
            style={{
              opacity: isSearchMode ? 1 : 0,
              transform: isSearchMode ? 'translateY(0)' : 'translateY(6px)',
              transition: 'opacity 0.2s ease, transform 0.22s ease',
              pointerEvents: isSearchMode ? 'auto' : 'none',
              whiteSpace: 'nowrap',
            }}
          >
            Search for everything
          </h2>
          {/* Work 模式欢迎语 */}
          <h2
            className="absolute text-[40px] font-normal tracking-[-0.5px] text-[var(--c-text-heading)]"
            style={{
              opacity: appMode === 'work' && !isSearchMode ? 1 : 0,
              transform: appMode === 'work' && !isSearchMode ? 'translateY(0)' : 'translateY(6px)',
              transition: 'opacity 0.22s ease, transform 0.24s ease',
              pointerEvents: appMode === 'work' && !isSearchMode ? 'auto' : 'none',
              whiteSpace: 'nowrap',
            }}
          >
            {t.workGreeting}
          </h2>
        </div>

        <div className="w-full max-w-[675px]">
          <ChatInput
            key={`welcome:${appMode}:${isSearchMode ? 'search' : 'default'}`}
            ref={chatInputRef}
            onSubmit={handleSubmit}
            placeholder={isSearchMode ? '今天有什么想搜索的吗？' : t.chatPlaceholder}
            disabled={sending}
            isStreaming={false}
            variant="welcome"
            searchMode={isSearchMode}
            attachments={attachments}
            onAttachFiles={handleAttachFiles}
            onPasteContent={handlePasteContent}
            onRemoveAttachment={handleRemoveAttachment}
            accessToken={accessToken}
            onAsrError={handleAsrError}
            onPersonaChange={(personaKey) => {
              if (personaKey === SEARCH_PERSONA_KEY && !isSearchMode) onEnterSearchMode()
              else if (personaKey !== SEARCH_PERSONA_KEY && isSearchMode) onExitSearchMode()
            }}
            onOpenSettings={onOpenSettings}
            appMode={appMode}
            draftOwnerKey={me?.id}
            planMode={appMode === 'work' && initialPlanMode}
            onTogglePlanMode={handleTogglePlanMode}
            learningModeEnabled={initialLearningModeEnabled}
            onToggleLearningMode={handleToggleLearningMode}
          />
          {/* incognito note: 平滑展开/收起 */}
          <div
            style={{
              display: 'grid',
              gridTemplateRows: isPrivateMode ? '1fr' : '0fr',
              opacity: isPrivateMode ? 1 : 0,
              transition: 'grid-template-rows 0.2s ease, opacity 0.15s ease',
              overflow: 'hidden',
            }}
          >
            <div style={{ minHeight: 0 }}>
              <p className="mt-2 text-center text-xs" style={{ color: 'var(--c-text-muted)' }}>
                {t.incognitoThreadNote}
              </p>
            </div>
          </div>
          {error && <ErrorCallout error={error} />}
        </div>

        {/* 智能体展示区：仅在普通聊天模式下显示 */}
        {!isSearchMode && appMode !== 'work' && (
          <AgentShowcase
            onSelect={(personaKey) => {
              if (personaKey) {
                chatInputRef.current?.selectPersona(personaKey)
              }
              chatInputRef.current?.focus()
            }}
          />
        )}
      </div>
      {showDebugPanel && <DebugTrigger />}
    </div>
  )
}
