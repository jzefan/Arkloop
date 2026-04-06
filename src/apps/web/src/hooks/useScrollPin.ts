import { useCallback, useEffect, useRef, useState } from 'react'
import type { AssistantTurnUi } from '../assistantTurnSegments'

interface UseScrollPinOptions {
  messagesLoading?: boolean
  messages?: readonly unknown[]
  liveAssistantTurn?: AssistantTurnUi | null
  liveRunUiVisible?: boolean
  topLevelCodeExecutionsLength?: number
}

export interface ScrollPinResult {
  isAtBottom: boolean
  bottomRef: React.RefObject<HTMLDivElement | null>
  scrollContainerRef: React.RefObject<HTMLDivElement | null>
  lastUserMsgRef: React.RefObject<HTMLDivElement | null>
  inputAreaRef: React.RefObject<HTMLDivElement | null>
  copCodeExecScrollRef: React.RefObject<HTMLDivElement | null>
  forceInstantBottomScrollRef: React.MutableRefObject<boolean>
  wasLoadingRef: React.MutableRefObject<boolean>
  documentPanelScrollFrameRef: React.MutableRefObject<number | null>
  isAtBottomRef: React.MutableRefObject<boolean>
  handleScrollContainerScroll: () => void
  scrollToBottom: () => void
  syncBottomState: (el: HTMLDivElement) => void
  stabilizeDocumentPanelScroll: (trigger?: HTMLElement | null) => void
}

export function useScrollPin(options: UseScrollPinOptions = {}): ScrollPinResult {
  const {
    messagesLoading = false,
    messages = [],
    liveAssistantTurn = null,
    liveRunUiVisible = false,
    topLevelCodeExecutionsLength = 0,
  } = options
  const bottomRef = useRef<HTMLDivElement>(null)
  const scrollContainerRef = useRef<HTMLDivElement>(null)
  const lastUserMsgRef = useRef<HTMLDivElement>(null)
  const inputAreaRef = useRef<HTMLDivElement>(null)
  const copCodeExecScrollRef = useRef<HTMLDivElement>(null)
  const forceInstantBottomScrollRef = useRef(false)
  const wasLoadingRef = useRef(false)
  const documentPanelScrollFrameRef = useRef<number | null>(null)
  const isAtBottomRef = useRef(true)
  const [isAtBottom, setIsAtBottom] = useState(true)

  const syncBottomState = useCallback((el: HTMLDivElement) => {
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight <= 80
    isAtBottomRef.current = atBottom
    setIsAtBottom(atBottom)
  }, [])

  const handleScrollContainerScroll = useCallback(() => {
    const el = scrollContainerRef.current
    if (!el) return
    syncBottomState(el)
  }, [syncBottomState])

  const stabilizeDocumentPanelScroll = useCallback((trigger?: HTMLElement | null) => {
    const container = scrollContainerRef.current
    if (!container) return

    if (documentPanelScrollFrameRef.current !== null) {
      cancelAnimationFrame(documentPanelScrollFrameRef.current)
      documentPanelScrollFrameRef.current = null
    }

    const anchor = trigger && container.contains(trigger) ? trigger : null
    const anchorTop = anchor
      ? anchor.getBoundingClientRect().top - container.getBoundingClientRect().top
      : null
    const distanceFromBottom = container.scrollHeight - container.scrollTop - container.clientHeight
    const startedAt = performance.now()

    const step = () => {
      const currentContainer = scrollContainerRef.current
      if (!currentContainer) return

      if (anchor && anchorTop !== null && anchor.isConnected && currentContainer.contains(anchor)) {
        const nextTop = anchor.getBoundingClientRect().top - currentContainer.getBoundingClientRect().top
        currentContainer.scrollTop += nextTop - anchorTop
      } else {
        currentContainer.scrollTop = Math.max(0, currentContainer.scrollHeight - currentContainer.clientHeight - distanceFromBottom)
      }

      syncBottomState(currentContainer)

      if (performance.now() - startedAt < 360) {
        documentPanelScrollFrameRef.current = requestAnimationFrame(step)
        return
      }

      documentPanelScrollFrameRef.current = null
    }

    documentPanelScrollFrameRef.current = requestAnimationFrame(step)
  }, [syncBottomState])

  const scrollToBottom = useCallback(() => {
    requestAnimationFrame(() => {
      requestAnimationFrame(() => {
        const target = lastUserMsgRef.current
        if (target) {
          target.scrollIntoView({ block: 'start', behavior: 'smooth' })
        } else {
          bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
        }
        isAtBottomRef.current = true
        setIsAtBottom(true)
      })
    })
  }, [])

  useEffect(() => {
    if (messagesLoading) {
      wasLoadingRef.current = true
      return
    }
    if (!wasLoadingRef.current) return
    wasLoadingRef.current = false
    lastUserMsgRef.current?.scrollIntoView({ behavior: 'instant', block: 'start' })
  }, [messagesLoading])

  useEffect(() => {
    if (!isAtBottomRef.current) return
    const forceInstant = forceInstantBottomScrollRef.current
    const liveHandoffPaint =
      liveAssistantTurn != null && liveAssistantTurn.segments.length > 0
    const behavior: ScrollBehavior = forceInstant || liveRunUiVisible || liveHandoffPaint ? 'instant' : 'smooth'
    const container = scrollContainerRef.current
    const bottom = bottomRef.current
    if (container && bottom) {
      const bottomTop = bottom.offsetTop
      const viewBottom = container.scrollTop + container.clientHeight
      if (bottomTop > viewBottom) {
        const targetScroll = bottomTop - container.clientHeight
        if (behavior === 'instant') {
          container.scrollTop = targetScroll
          bottom.scrollIntoView({ behavior: 'instant' })
        } else {
          container.scrollTo({ top: targetScroll, behavior })
        }
      }
    } else {
      bottomRef.current?.scrollIntoView({ behavior })
    }
    if (forceInstant) forceInstantBottomScrollRef.current = false
  }, [messages, liveAssistantTurn, liveRunUiVisible])

  useEffect(() => {
    const el = copCodeExecScrollRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [topLevelCodeExecutionsLength, liveAssistantTurn])

  // input area resize observer
  useEffect(() => {
    const el = inputAreaRef.current
    if (!el) return
    if (typeof ResizeObserver === 'undefined') {
      document.documentElement.style.setProperty('--chat-input-area-height', `${el.getBoundingClientRect().height}px`)
      return
    }
    const ro = new ResizeObserver(([entry]) => {
      document.documentElement.style.setProperty('--chat-input-area-height', `${entry.contentRect.height}px`)
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  // cleanup animation frames on unmount
  useEffect(() => {
    return () => {
      if (documentPanelScrollFrameRef.current !== null) {
        cancelAnimationFrame(documentPanelScrollFrameRef.current)
      }
    }
  }, [])

  return {
    isAtBottom,
    bottomRef,
    scrollContainerRef,
    lastUserMsgRef,
    inputAreaRef,
    copCodeExecScrollRef,
    forceInstantBottomScrollRef,
    wasLoadingRef,
    documentPanelScrollFrameRef,
    isAtBottomRef,
    handleScrollContainerScroll,
    scrollToBottom,
    syncBottomState,
    stabilizeDocumentPanelScroll,
  }
}
