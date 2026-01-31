/**
 * SSE 客户端 - 基于 fetch stream 实现
 * 支持 after_seq 断线续传
 */

export type SSEEvent = {
  id?: string
  event?: string
  data: string
}

export type RunEvent = {
  event_id: string
  run_id: string
  seq: number
  ts: string
  type: string
  data: unknown
  tool_name?: string
  error_class?: string
}

export type SSEClientState = 'idle' | 'connecting' | 'connected' | 'reconnecting' | 'closed' | 'error'

export type SSEClientOptions = {
  url: string
  accessToken: string
  afterSeq?: number
  follow?: boolean
  onEvent: (event: RunEvent) => void
  onStateChange?: (state: SSEClientState) => void
  onError?: (error: Error) => void
  maxRetries?: number
  retryDelayMs?: number
}

/**
 * 解析 SSE 文本流，提取事件
 * 处理 data 行、空行分隔；忽略 comment 心跳
 */
export function parseSSEChunk(buffer: string): { events: SSEEvent[]; remaining: string } {
  const events: SSEEvent[] = []
  const lines = buffer.split('\n')

  let currentEvent: Partial<SSEEvent> = {}
  let dataLines: string[] = []
  let lastCompleteIndex = -1

  // split 后最后一项永远是“未完成的行”（即使为空），避免误判空行分隔
  for (let i = 0; i < lines.length - 1; i++) {
    let line = lines[i]
    if (line.endsWith('\r')) {
      line = line.slice(0, -1)
    }

    // 空行表示事件结束
    if (line === '') {
      if (dataLines.length > 0) {
        currentEvent.data = dataLines.join('\n')
        events.push(currentEvent as SSEEvent)
      }
      currentEvent = {}
      dataLines = []
      lastCompleteIndex = i
      continue
    }

    // 忽略注释行（心跳）
    if (line.startsWith(':')) {
      continue
    }

    // 解析字段
    const colonIndex = line.indexOf(':')
    if (colonIndex === -1) continue

    const field = line.slice(0, colonIndex)
    // SSE 规范：冒号后有空格则跳过
    const value = line.slice(colonIndex + 1).replace(/^ /, '')

    switch (field) {
      case 'id':
        currentEvent.id = value
        break
      case 'event':
        currentEvent.event = value
        break
      case 'data':
        dataLines.push(value)
        break
    }
  }

  // 返回未完成的部分
  const remaining = lastCompleteIndex === -1
    ? buffer
    : lines.slice(lastCompleteIndex + 1).join('\n')

  return { events, remaining }
}

/**
 * SSE 客户端类
 * 管理连接生命周期、自动重连、游标续传
 */
export class SSEClient {
  private options: Required<Omit<SSEClientOptions, 'onStateChange' | 'onError'>> &
    Pick<SSEClientOptions, 'onStateChange' | 'onError'>
  private state: SSEClientState = 'idle'
  private abortController: AbortController | null = null
  private lastSeq: number
  private retryCount = 0
  private closed = false

  constructor(options: SSEClientOptions) {
    this.options = {
      afterSeq: 0,
      follow: true,
      maxRetries: 5,
      retryDelayMs: 1000,
      ...options,
    }
    this.lastSeq = this.options.afterSeq
  }

  private setState(state: SSEClientState) {
    if (this.state === state) return
    this.state = state
    this.options.onStateChange?.(state)
  }

  getState(): SSEClientState {
    return this.state
  }

  getLastSeq(): number {
    return this.lastSeq
  }

  async connect(): Promise<void> {
    if (this.closed) return

    this.setState('connecting')
    this.abortController = new AbortController()

    try {
      await this.doConnect()
    } catch (err) {
      if (this.closed) return

      const error = err instanceof Error ? err : new Error(String(err))

      // 非中止错误才重试
      if (error.name !== 'AbortError') {
        this.options.onError?.(error)
        await this.scheduleRetry()
      }
    }
  }

  private async doConnect(): Promise<void> {
    const { url, accessToken, follow } = this.options

    // 构建带游标的 URL
    const urlObj = new URL(url, window.location.origin)
    urlObj.searchParams.set('after_seq', String(this.lastSeq))
    urlObj.searchParams.set('follow', follow ? 'true' : 'false')

    const response = await fetch(urlObj.toString(), {
      method: 'GET',
      headers: {
        'Accept': 'text/event-stream',
        'Authorization': `Bearer ${accessToken}`,
      },
      signal: this.abortController?.signal,
    })

    if (!response.ok) {
      throw new Error(`SSE 连接失败: HTTP ${response.status}`)
    }

    if (!response.body) {
      throw new Error('SSE 响应无 body')
    }

    this.setState('connected')
    this.retryCount = 0

    await this.readStream(response.body)
  }

  private async readStream(body: ReadableStream<Uint8Array>): Promise<void> {
    const reader = body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''

    try {
      while (true) {
        const { value, done } = await reader.read()

        if (done) {
          // 流正常结束
          this.setState('closed')
          break
        }

        buffer += decoder.decode(value, { stream: true })
        const { events, remaining } = parseSSEChunk(buffer)
        buffer = remaining

        for (const sseEvent of events) {
          this.processEvent(sseEvent)
        }
      }
    } finally {
      reader.releaseLock()
    }
  }

  private processEvent(sseEvent: SSEEvent): void {
    if (!sseEvent.data) return

    try {
      const runEvent = JSON.parse(sseEvent.data) as RunEvent

      // 更新游标
      if (typeof runEvent.seq === 'number') {
        this.lastSeq = runEvent.seq
      }

      this.options.onEvent(runEvent)
    } catch {
      // JSON 解析失败，忽略
    }
  }

  private async scheduleRetry(): Promise<void> {
    if (this.closed) return

    if (this.retryCount >= this.options.maxRetries) {
      this.setState('error')
      this.options.onError?.(new Error(`重连失败，已达最大重试次数 ${this.options.maxRetries}`))
      return
    }

    this.retryCount++
    this.setState('reconnecting')

    // 指数退避
    const delay = this.options.retryDelayMs * Math.pow(2, this.retryCount - 1)
    await new Promise(resolve => setTimeout(resolve, delay))

    if (!this.closed) {
      await this.connect()
    }
  }

  close(): void {
    this.closed = true
    this.abortController?.abort()
    this.setState('closed')
  }

  /**
   * 手动重连（用于网络恢复后）
   */
  async reconnect(): Promise<void> {
    if (this.state === 'connected' || this.state === 'connecting') return

    this.closed = false
    this.retryCount = 0
    await this.connect()
  }
}

/**
 * 创建 SSE 客户端的工厂函数
 */
export function createSSEClient(options: SSEClientOptions): SSEClient {
  return new SSEClient(options)
}
