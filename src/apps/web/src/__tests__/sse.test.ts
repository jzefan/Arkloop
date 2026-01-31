import { describe, it, expect } from 'vitest'
import { parseSSEChunk } from '../sse'

describe('parseSSEChunk', () => {
  it('解析单个完整事件', () => {
    const input = 'id: 1\nevent: message\ndata: {"seq":1}\n\n'
    const { events, remaining } = parseSSEChunk(input)

    expect(events).toHaveLength(1)
    expect(events[0]).toEqual({
      id: '1',
      event: 'message',
      data: '{"seq":1}',
    })
    expect(remaining).toBe('')
  })

  it('解析多个事件', () => {
    const input = 'data: {"seq":1}\n\ndata: {"seq":2}\n\n'
    const { events, remaining } = parseSSEChunk(input)

    expect(events).toHaveLength(2)
    expect(events[0].data).toBe('{"seq":1}')
    expect(events[1].data).toBe('{"seq":2}')
    expect(remaining).toBe('')
  })

  it('忽略注释行（心跳）', () => {
    const input = ': ping\ndata: {"seq":1}\n\n'
    const { events, remaining } = parseSSEChunk(input)

    expect(events).toHaveLength(1)
    expect(events[0].data).toBe('{"seq":1}')
    expect(remaining).toBe('')
  })

  it('处理多行 data', () => {
    const input = 'data: line1\ndata: line2\n\n'
    const { events } = parseSSEChunk(input)

    expect(events).toHaveLength(1)
    expect(events[0].data).toBe('line1\nline2')
  })

  it('保留不完整的事件到 remaining', () => {
    const input = 'data: {"seq":1}\n\ndata: {"seq":2'
    const { events, remaining } = parseSSEChunk(input)

    expect(events).toHaveLength(1)
    expect(events[0].data).toBe('{"seq":1}')
    expect(remaining).toBe('data: {"seq":2')
  })

  it('处理空输入', () => {
    const { events, remaining } = parseSSEChunk('')

    expect(events).toHaveLength(0)
    expect(remaining).toBe('')
  })

  it('处理只有注释的输入', () => {
    const input = ': heartbeat\n: ping\n'
    const { events } = parseSSEChunk(input)

    expect(events).toHaveLength(0)
  })

  it('处理 data 后有空格的情况', () => {
    // SSE 规范：冒号后的第一个空格应被忽略
    const input = 'data: {"key": "value"}\n\n'
    const { events } = parseSSEChunk(input)

    expect(events[0].data).toBe('{"key": "value"}')
  })

  it('处理连续空行', () => {
    const input = 'data: first\n\n\ndata: second\n\n'
    const { events } = parseSSEChunk(input)

    expect(events).toHaveLength(2)
    expect(events[0].data).toBe('first')
    expect(events[1].data).toBe('second')
  })

  it('处理真实的 run event 格式', () => {
    const eventData = JSON.stringify({
      event_id: '550e8400-e29b-41d4-a716-446655440000',
      run_id: '550e8400-e29b-41d4-a716-446655440001',
      seq: 1,
      ts: '2024-01-01T00:00:00.000Z',
      type: 'run.started',
      data: {},
    })

    const input = `id: 1\nevent: run.started\ndata: ${eventData}\n\n`
    const { events } = parseSSEChunk(input)

    expect(events).toHaveLength(1)
    expect(events[0].id).toBe('1')
    expect(events[0].event).toBe('run.started')

    const parsed = JSON.parse(events[0].data)
    expect(parsed.seq).toBe(1)
    expect(parsed.type).toBe('run.started')
  })

  it('处理分块接收的场景', () => {
    // 模拟网络分块
    const chunk1 = 'data: {"seq":'
    const chunk2 = '1}\n\ndata: {"seq":2}\n\n'

    // 第一块
    const result1 = parseSSEChunk(chunk1)
    expect(result1.events).toHaveLength(0)
    expect(result1.remaining).toBe('data: {"seq":')

    // 第二块（拼接 remaining）
    const result2 = parseSSEChunk(result1.remaining + chunk2)
    expect(result2.events).toHaveLength(2)
    expect(result2.events[0].data).toBe('{"seq":1}')
    expect(result2.events[1].data).toBe('{"seq":2}')
  })

  it('兼容 CRLF 行尾与空行分隔', () => {
    const input = 'data: {"seq":1}\r\n\r\n'
    const { events, remaining } = parseSSEChunk(input)

    expect(events).toHaveLength(1)
    expect(events[0].data).toBe('{"seq":1}')
    expect(remaining).toBe('')
  })

  it('注释行不应影响 remaining（避免跨分块丢事件）', () => {
    const chunk1 = 'data: {"seq":1}\n: ping\n'
    const result1 = parseSSEChunk(chunk1)
    expect(result1.events).toHaveLength(0)
    expect(result1.remaining).toBe(chunk1)

    const chunk2 = '\n'
    const result2 = parseSSEChunk(result1.remaining + chunk2)
    expect(result2.events).toHaveLength(1)
    expect(result2.events[0].data).toBe('{"seq":1}')
  })
})
