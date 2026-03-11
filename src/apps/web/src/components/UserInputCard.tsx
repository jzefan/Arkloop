import { useCallback, useEffect, useMemo, useState } from 'react'
import { ChevronLeft, ChevronRight, ArrowRight } from 'lucide-react'
import { useLocale } from '../contexts/LocaleContext'
import type {
  UserInputAnswer,
  UserInputRequest,
  UserInputResponse,
} from '../userInputTypes'

interface Props {
  request: UserInputRequest
  onSubmit: (response: UserInputResponse) => void
  onDismiss: () => void
  disabled?: boolean
}

export default function UserInputCard({ request, onSubmit, onDismiss, disabled }: Props) {
  const { t } = useLocale()
  const isMulti = request.questions.length > 1
  const [activeIdx, setActiveIdx] = useState(0)
  const [answers, setAnswers] = useState<Record<string, UserInputAnswer>>({})
  const [otherTexts, setOtherTexts] = useState<Record<string, string>>({})
  const [submitting, setSubmitting] = useState(false)
  const [cardHovered, setCardHovered] = useState(false)

  useEffect(() => {
    const initial: Record<string, UserInputAnswer> = {}
    for (const q of request.questions) {
      const rec = q.options.find((o) => o.recommended)
      if (rec) initial[q.id] = { type: 'option', value: rec.value }
    }
    setAnswers(initial)
    setActiveIdx(0)
  }, [request.questions])

  const currentQ = request.questions[activeIdx]
  const isLastQuestion = activeIdx === request.questions.length - 1

  const selectedCount = useMemo(() => {
    return request.questions.filter((q) => {
      const a = answers[q.id]
      return a && (a.type === 'option' || (a.type === 'other' && a.value.trim()))
    }).length
  }, [answers, request.questions])

  const allAnswered = useMemo(() => {
    if (submitting || disabled) return false
    for (const q of request.questions) {
      const a = answers[q.id]
      if (!a || (a.type === 'other' && !a.value.trim())) return false
    }
    return true
  }, [answers, request.questions, submitting, disabled])

  const doSubmit = useCallback((latestAnswers: Record<string, UserInputAnswer>) => {
    setSubmitting(true)
    onSubmit({ type: 'user_input_response', request_id: request.request_id, answers: latestAnswers })
  }, [onSubmit, request.request_id])

  // Single click: select + immediately advance or submit
  const handleOptionClick = useCallback((questionId: string, value: string) => {
    const updated: Record<string, UserInputAnswer> = { ...answers, [questionId]: { type: 'option', value } }
    setAnswers(updated)
    if (isMulti && !isLastQuestion) {
      setActiveIdx((i) => i + 1)
      return
    }
    let ready = true
    for (const q of request.questions) {
      const a = updated[q.id]
      if (!a || (a.type === 'other' && !a.value.trim())) { ready = false; break }
    }
    if (ready) doSubmit(updated)
  }, [answers, isMulti, isLastQuestion, request.questions, doSubmit])

  const handleSubmit = useCallback(() => {
    if (!allAnswered) return
    doSubmit(answers)
  }, [allAnswered, doSubmit, answers])

  const handleDismiss = useCallback(() => {
    if (submitting || disabled) return
    onDismiss()
  }, [submitting, disabled, onDismiss])

  const handleSelectOther = useCallback(() => {
    setAnswers((prev) => ({
      ...prev,
      [currentQ.id]: { type: 'other', value: otherTexts[currentQ.id] ?? '' },
    }))
  }, [currentQ?.id, otherTexts])

  const handleUpdateOther = useCallback((text: string) => {
    setOtherTexts((prev) => ({ ...prev, [currentQ.id]: text }))
    setAnswers((prev) => {
      if (prev[currentQ.id]?.type === 'other') {
        return { ...prev, [currentQ.id]: { type: 'other', value: text } }
      }
      return prev
    })
  }, [currentQ?.id])

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') { e.preventDefault(); handleDismiss() }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [handleDismiss])

  const arrowActive = allAnswered && !submitting

  return (
    <div
      className="flex flex-col w-full"
      style={{
        background: 'var(--c-bg-input)',
        borderWidth: '0.5px',
        borderStyle: 'solid',
        borderColor: cardHovered ? 'var(--c-input-border-color-hover)' : 'var(--c-input-border-color)',
        borderRadius: '24px',
        boxShadow: cardHovered ? 'var(--c-input-shadow-hover)' : 'var(--c-input-shadow)',
        transition: 'border-color 0.2s ease, box-shadow 0.2s ease',
        padding: '24px 28px 20px',
      }}
      onMouseEnter={() => setCardHovered(true)}
      onMouseLeave={() => setCardHovered(false)}
    >
      {/* Header */}
      <div className="flex items-start justify-between gap-4 mb-5">
        <div className="flex flex-col gap-1">
          {currentQ.header && (
            <span
              className="text-xs font-medium"
              style={{ color: 'var(--c-text-tertiary)', letterSpacing: '0.05em', textTransform: 'uppercase' }}
            >
              {currentQ.header}
            </span>
          )}
          <h2 className="text-xl font-semibold leading-snug m-0" style={{ color: 'var(--c-text-heading)' }}>
            {currentQ.question}
          </h2>
        </div>
        {isMulti && (
          <div className="flex items-center gap-0.5 flex-shrink-0 mt-1">
            <button
              type="button"
              onClick={() => activeIdx > 0 && setActiveIdx((i) => i - 1)}
              disabled={activeIdx === 0}
              className="flex h-6 w-6 items-center justify-center rounded-md border-none bg-transparent cursor-pointer disabled:opacity-25 transition-[background-color] duration-[60ms] hover:bg-[var(--c-bg-deep)]"
              style={{ color: 'var(--c-text-secondary)' }}
            >
              <ChevronLeft size={14} />
            </button>
            <span className="text-xs tabular-nums px-1" style={{ color: 'var(--c-text-tertiary)' }}>
              {activeIdx + 1} of {request.questions.length}
            </span>
            <button
              type="button"
              onClick={() => !isLastQuestion && setActiveIdx((i) => i + 1)}
              disabled={isLastQuestion}
              className="flex h-6 w-6 items-center justify-center rounded-md border-none bg-transparent cursor-pointer disabled:opacity-25 transition-[background-color] duration-[60ms] hover:bg-[var(--c-bg-deep)]"
              style={{ color: 'var(--c-text-secondary)' }}
            >
              <ChevronRight size={14} />
            </button>
          </div>
        )}
      </div>

      {/* Options */}
      <div className="flex flex-col">
        {currentQ.options.map((opt, idx) => {
          const selected = answers[currentQ.id]?.type === 'option' && answers[currentQ.id]?.value === opt.value
          const isLast = idx === currentQ.options.length - 1 && !currentQ.allow_other
          return (
            <OptionRow
              key={opt.value}
              index={idx}
              option={opt}
              selected={selected}
              disabled={submitting || !!disabled}
              isLast={isLast}
              onClick={() => handleOptionClick(currentQ.id, opt.value)}
              t={t}
            />
          )
        })}
        {currentQ.allow_other && (
          <OtherRow
            selected={answers[currentQ.id]?.type === 'other'}
            text={otherTexts[currentQ.id] ?? ''}
            disabled={submitting || !!disabled}
            onSelect={handleSelectOther}
            onUpdateText={handleUpdateOther}
            t={t}
          />
        )}
      </div>

      {/* Footer */}
      <div
        className="flex items-center justify-between mt-5 pt-4"
        style={{ borderTop: '0.5px solid var(--c-border-subtle)' }}
      >
        <span className="text-sm tabular-nums" style={{ color: 'var(--c-text-tertiary)' }}>
          {selectedCount} {t.userInput.selected}
        </span>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={handleDismiss}
            disabled={submitting || !!disabled}
            className="rounded-lg px-3 py-1.5 text-sm border-none bg-transparent cursor-pointer transition-[background-color] duration-[60ms] disabled:opacity-40 hover:bg-[var(--c-bg-deep)]"
            style={{ color: 'var(--c-text-secondary)' }}
          >
            {t.userInput.dismiss}
          </button>
          <button
            type="button"
            onClick={handleSubmit}
            disabled={!arrowActive}
            aria-label={t.userInput.submit}
            data-testid="user-input-submit"
            className="flex h-9 w-9 items-center justify-center rounded-xl border-none cursor-pointer disabled:opacity-30 transition-[background-color,color] duration-[60ms]"
            style={{
              background: arrowActive ? 'var(--c-text-primary)' : 'var(--c-bg-deep)',
              color: arrowActive ? 'var(--c-bg-page)' : 'var(--c-text-muted)',
            }}
          >
            <ArrowRight size={16} />
          </button>
        </div>
      </div>
    </div>
  )
}

// --- OptionRow ---

interface OptionRowProps {
  index: number
  option: { value: string; label: string; description?: string; recommended?: boolean }
  selected: boolean
  disabled: boolean
  isLast: boolean
  onClick: () => void
  t: ReturnType<typeof useLocale>['t']
}

function OptionRow({ index, option, selected, disabled, isLast, onClick, t }: OptionRowProps) {
  const [rowHovered, setRowHovered] = useState(false)
  const [showTooltip, setShowTooltip] = useState(false)

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={() => !disabled && onClick()}
      onKeyDown={(e) => {
        if (disabled) return
        if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onClick() }
      }}
      onMouseEnter={() => setRowHovered(true)}
      onMouseLeave={() => setRowHovered(false)}
      className="flex items-center gap-3 py-3.5 cursor-pointer"
      style={{
        background: rowHovered && !disabled ? 'var(--c-bg-deep)' : 'transparent',
        borderBottom: isLast ? 'none' : '0.5px solid var(--c-border-subtle)',
        opacity: disabled ? 0.5 : 1,
        transition: 'background 60ms ease',
        marginLeft: '-4px',
        marginRight: '-4px',
        paddingLeft: '4px',
        paddingRight: '4px',
      }}
    >
      <div
        className="flex-shrink-0 flex items-center justify-center rounded-md text-sm font-semibold"
        style={{
          width: '28px',
          height: '28px',
          background: selected ? 'var(--c-text-primary)' : 'var(--c-bg-deep)',
          color: selected ? 'var(--c-bg-page)' : 'var(--c-text-secondary)',
          transition: 'background 60ms ease, color 60ms ease',
        }}
      >
        {index + 1}
      </div>
      <span className="flex-1 text-base" style={{ color: 'var(--c-text-primary)' }}>
        {option.label}
      </span>
      {option.recommended && (
        <span
          className="flex-shrink-0 rounded-full px-2 py-0.5 text-[10px] font-medium"
          style={{ background: 'var(--c-bg-deep)', color: 'var(--c-text-tertiary)' }}
        >
          {t.userInput.recommended}
        </span>
      )}
      {option.description && (
        <span
          className="relative flex-shrink-0"
          onMouseEnter={() => setShowTooltip(true)}
          onMouseLeave={() => setShowTooltip(false)}
        >
          <span
            className="inline-flex h-5 w-5 items-center justify-center rounded-full text-[10px] cursor-help"
            style={{ border: '0.5px solid var(--c-border-subtle)', color: 'var(--c-text-muted)' }}
          >
            i
          </span>
          {showTooltip && (
            <div
              className="absolute bottom-full right-0 z-10 mb-1 max-w-[200px] rounded-xl px-2.5 py-1.5 text-xs"
              style={{
                background: 'var(--c-bg-menu)',
                border: '0.5px solid var(--c-border-subtle)',
                color: 'var(--c-text-secondary)',
                boxShadow: '0 2px 8px rgba(0,0,0,0.15)',
              }}
            >
              {option.description}
            </div>
          )}
        </span>
      )}
    </div>
  )
}

// --- OtherRow ---

interface OtherRowProps {
  selected: boolean
  text: string
  disabled: boolean
  onSelect: () => void
  onUpdateText: (text: string) => void
  t: ReturnType<typeof useLocale>['t']
}

function OtherRow({ selected, text, disabled, onSelect, onUpdateText, t }: OtherRowProps) {
  const [rowHovered, setRowHovered] = useState(false)

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={() => !disabled && onSelect()}
      onKeyDown={(e) => {
        if (disabled) return
        if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onSelect() }
      }}
      onMouseEnter={() => setRowHovered(true)}
      onMouseLeave={() => setRowHovered(false)}
      className="flex items-center gap-3 py-3.5 cursor-pointer"
      style={{
        background: rowHovered && !disabled ? 'var(--c-bg-deep)' : 'transparent',
        opacity: disabled ? 0.5 : 1,
        transition: 'background 60ms ease',
        marginLeft: '-4px',
        marginRight: '-4px',
        paddingLeft: '4px',
        paddingRight: '4px',
      }}
    >
      <div
        className="flex-shrink-0 flex items-center justify-center rounded-md text-sm font-semibold"
        style={{
          width: '28px',
          height: '28px',
          background: selected ? 'var(--c-text-primary)' : 'var(--c-bg-deep)',
          color: selected ? 'var(--c-bg-page)' : 'var(--c-text-tertiary)',
          transition: 'background 60ms ease, color 60ms ease',
        }}
      >
        *
      </div>
      <input
        type="text"
        value={text}
        onChange={(e) => { onUpdateText(e.target.value); onSelect() }}
        onClick={(e) => { e.stopPropagation(); onSelect() }}
        disabled={disabled}
        placeholder={t.userInput.otherPlaceholder}
        className="flex-1 bg-transparent border-none outline-none text-base"
        style={{ color: 'var(--c-text-primary)', caretColor: 'var(--c-text-primary)' }}
      />
    </div>
  )
}
