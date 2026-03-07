import { describe, expect, it } from 'vitest'
import {
  buildMessageCodeExecutionsFromRunEvents,
  buildMessageThinkingFromRunEvents,
  patchCodeExecutionList,
  selectFreshRunEvents,
  shouldReplayMessageCodeExecutions,
} from '../runEventProcessing'
import type { RunEvent } from '../sse'

function makeRunEvent(params: {
  runId: string
  seq: number
  type: string
  data?: unknown
}): RunEvent {
  return {
    event_id: `evt_${params.seq}`,
    run_id: params.runId,
    seq: params.seq,
    ts: '2024-01-01T00:00:00.000Z',
    type: params.type,
    data: params.data ?? {},
  }
}

describe('selectFreshRunEvents', () => {
  it('应忽略旧 run 的尾部事件，避免误触发断开', () => {
    const events = [makeRunEvent({ runId: 'run_1', seq: 1, type: 'run.completed' })]

    const result = selectFreshRunEvents({
      events,
      activeRunId: 'run_2',
      processedCount: 0,
    })

    expect(result.fresh).toEqual([])
    expect(result.nextProcessedCount).toBe(1)
  })

  it('应在 events 被清空后重置 processedCount', () => {
    const result = selectFreshRunEvents({
      events: [],
      activeRunId: 'run_1',
      processedCount: 10,
    })

    expect(result.fresh).toEqual([])
    expect(result.nextProcessedCount).toBe(0)
  })

  it('应只返回当前 run 的新事件，并推进游标到末尾', () => {
    const events = [
      makeRunEvent({ runId: 'run_1', seq: 1, type: 'run.started' }),
      makeRunEvent({
        runId: 'run_2',
        seq: 2,
        type: 'message.delta',
        data: { content_delta: 'hi', role: 'assistant' },
      }),
      makeRunEvent({ runId: 'run_2', seq: 3, type: 'run.completed' }),
    ]

    const result = selectFreshRunEvents({
      events,
      activeRunId: 'run_2',
      processedCount: 0,
    })

    expect(result.fresh.map((item) => item.seq)).toEqual([2, 3])
    expect(result.nextProcessedCount).toBe(3)
  })

  it('应从 processedCount 之后开始取新事件', () => {
    const events = [
      makeRunEvent({ runId: 'run_1', seq: 1, type: 'run.started' }),
      makeRunEvent({ runId: 'run_1', seq: 2, type: 'message.delta' }),
    ]

    const result = selectFreshRunEvents({
      events,
      activeRunId: 'run_1',
      processedCount: 1,
    })

    expect(result.fresh.map((item) => item.seq)).toEqual([2])
    expect(result.nextProcessedCount).toBe(2)
  })

  it('processedCount 超过 events.length 时重置为 0', () => {
    const events = [
      makeRunEvent({ runId: 'run_1', seq: 1, type: 'run.started' }),
      makeRunEvent({ runId: 'run_1', seq: 2, type: 'message.delta' }),
    ]

    const result = selectFreshRunEvents({
      events,
      activeRunId: 'run_1',
      processedCount: 999,
    })

    expect(result.fresh.map((item) => item.seq)).toEqual([1, 2])
    expect(result.nextProcessedCount).toBe(2)
  })
})

describe('buildMessageCodeExecutionsFromRunEvents', () => {
  it('应将 write_stdin 结果回填到原始 exec_command 记录', () => {
    const events = [
      makeRunEvent({
        runId: 'run_1',
        seq: 1,
        type: 'tool.call',
        data: { tool_name: 'exec_command', tool_call_id: 'call_exec', arguments: { command: 'uname -a' } },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 2,
        type: 'tool.result',
        data: {
          tool_name: 'exec_command',
          tool_call_id: 'call_exec',
          result: { session_id: 'sess_1', running: true, output: 'Linux ' },
        },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 3,
        type: 'tool.call',
        data: { tool_name: 'write_stdin', tool_call_id: 'call_write', arguments: { session_id: 'sess_1' } },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 4,
        type: 'tool.result',
        data: {
          tool_name: 'write_stdin',
          tool_call_id: 'call_write',
          result: { session_id: 'sess_1', running: false, output: '6.12.72', exit_code: 0 },
        },
      }),
    ]

    const executions = buildMessageCodeExecutionsFromRunEvents(events)
    expect(executions).toHaveLength(1)
    expect(executions[0]).toMatchObject({
      id: 'call_exec',
      language: 'shell',
      code: 'uname -a',
      sessionId: 'sess_1',
      output: 'Linux 6.12.72',
      exitCode: 0,
    })
  })

  it('应过滤常见终端控制序列', () => {
    const events = [
      makeRunEvent({
        runId: 'run_1',
        seq: 1,
        type: 'tool.call',
        data: { tool_name: 'exec_command', tool_call_id: 'call_exec', arguments: { command: 'uname -a' } },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 2,
        type: 'tool.result',
        data: {
          tool_name: 'exec_command',
          tool_call_id: 'call_exec',
          result: { session_id: 'sess_1', running: false, output: '\u001b[?2004hLinux\n\u001b[?2004l', exit_code: 0 },
        },
      }),
    ]

    const executions = buildMessageCodeExecutionsFromRunEvents(events)
    expect(executions).toHaveLength(1)
    expect(executions[0]?.output).toBe('Linux\n')
  })

  it('累计输出与全量输出混用时应避免重复拼接', () => {
    const events = [
      makeRunEvent({
        runId: 'run_1',
        seq: 1,
        type: 'tool.call',
        data: { tool_name: 'exec_command', tool_call_id: 'call_exec', arguments: { command: 'echo hi' } },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 2,
        type: 'tool.result',
        data: {
          tool_name: 'exec_command',
          tool_call_id: 'call_exec',
          result: { session_id: 'sess_1', running: true, output: 'hi' },
        },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 3,
        type: 'tool.result',
        data: {
          tool_name: 'write_stdin',
          tool_call_id: 'call_write',
          result: { session_id: 'sess_1', running: false, output: 'hi there', exit_code: 0 },
        },
      }),
    ]

    const executions = buildMessageCodeExecutionsFromRunEvents(events)
    expect(executions).toHaveLength(1)
    expect(executions[0]?.output).toBe('hi there')
  })

  it('缺少原始 exec_command 时，write_stdin 结果也不应丢失', () => {
    const events = [
      makeRunEvent({
        runId: 'run_1',
        seq: 1,
        type: 'tool.result',
        data: {
          tool_name: 'write_stdin',
          tool_call_id: 'call_write',
          result: { session_id: 'sess_orphan', running: false, output: 'done', exit_code: 0 },
        },
      }),
    ]

    const executions = buildMessageCodeExecutionsFromRunEvents(events)
    expect(executions).toHaveLength(1)
    expect(executions[0]).toMatchObject({
      id: 'call_write',
      language: 'shell',
      sessionId: 'sess_orphan',
      output: 'done',
      exitCode: 0,
    })
  })
})

describe('shouldReplayMessageCodeExecutions', () => {
  it('shell 记录缺少 sessionId 时应触发回放修复', () => {
    expect(shouldReplayMessageCodeExecutions([{
      id: 'call_exec',
      language: 'shell',
      code: 'uname -a',
      output: 'Linux',
    }])).toBe(true)
  })

  it('空数组哨兵不应重复触发回放', () => {
    expect(shouldReplayMessageCodeExecutions([])).toBe(false)
  })

  it('已有 sessionId 的 shell 记录不需要额外回放', () => {
    expect(shouldReplayMessageCodeExecutions([{
      id: 'call_exec',
      language: 'shell',
      code: 'uname -a',
      output: 'Linux',
      sessionId: 'sess_1',
    }])).toBe(false)
  })
})

describe('patchCodeExecutionList', () => {
  it('同一 shell session 下更新后续命令时，不应覆盖之前的命令', () => {
    const executions = [
      {
        id: 'call_exec_1',
        language: 'shell' as const,
        code: 'pwd',
        output: '/tmp',
        sessionId: 'sess_1',
      },
      {
        id: 'call_exec_2',
        language: 'shell' as const,
        code: 'ls',
        sessionId: 'sess_1',
      },
    ]

    const result = patchCodeExecutionList(executions, {
      id: 'call_exec_2',
      language: 'shell',
      code: 'ls',
      output: 'a.txt',
      exitCode: 0,
      sessionId: 'sess_1',
    })

    expect(result.next).toEqual([
      {
        id: 'call_exec_1',
        language: 'shell',
        code: 'pwd',
        output: '/tmp',
        sessionId: 'sess_1',
      },
      {
        id: 'call_exec_2',
        language: 'shell',
        code: 'ls',
        output: 'a.txt',
        exitCode: 0,
        sessionId: 'sess_1',
      },
    ])
  })
})

describe('buildMessageThinkingFromRunEvents', () => {
  it('应提取顶层 thinking 文本', () => {
    const events = [
      makeRunEvent({
        runId: 'run_1',
        seq: 1,
        type: 'message.delta',
        data: { role: 'assistant', channel: 'thinking', content_delta: 'A' },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 2,
        type: 'message.delta',
        data: { role: 'assistant', channel: 'thinking', content_delta: 'B' },
      }),
    ]

    const snapshot = buildMessageThinkingFromRunEvents(events)
    expect(snapshot).not.toBeNull()
    expect(snapshot?.thinkingText).toBe('AB')
    expect(snapshot?.segments).toEqual([])
  })

  it('应提取 segment 文本并过滤 hidden 段', () => {
    const events = [
      makeRunEvent({
        runId: 'run_1',
        seq: 1,
        type: 'run.segment.start',
        data: { segment_id: 'seg_1', kind: 'planning_round', display: { mode: 'collapsed', label: 'Plan' } },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 2,
        type: 'message.delta',
        data: { role: 'assistant', content_delta: 'P1' },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 3,
        type: 'run.segment.end',
        data: { segment_id: 'seg_1' },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 4,
        type: 'run.segment.start',
        data: { segment_id: 'seg_2', kind: 'planning_round', display: { mode: 'hidden', label: 'Hidden' } },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 5,
        type: 'message.delta',
        data: { role: 'assistant', content_delta: 'H1' },
      }),
    ]

    const snapshot = buildMessageThinkingFromRunEvents(events)
    expect(snapshot).not.toBeNull()
    expect(snapshot?.segments).toHaveLength(1)
    expect(snapshot?.segments[0]).toMatchObject({
      segmentId: 'seg_1',
      label: 'Plan',
      content: 'P1',
    })
  })

  it('没有 thinking 内容时应返回 null', () => {
    const events = [
      makeRunEvent({
        runId: 'run_1',
        seq: 1,
        type: 'message.delta',
        data: { role: 'assistant', content_delta: 'Final answer' },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 2,
        type: 'run.completed',
      }),
    ]

    const snapshot = buildMessageThinkingFromRunEvents(events)
    expect(snapshot).toBeNull()
  })

  it('segment.start 缺少 segment_id 时跳过', () => {
    const events = [
      makeRunEvent({
        runId: 'run_1',
        seq: 1,
        type: 'run.segment.start',
        data: { kind: 'planning_round', display: { mode: 'collapsed', label: 'NoID' } },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 2,
        type: 'message.delta',
        data: { role: 'assistant', content_delta: 'orphaned delta' },
      }),
    ]

    const snapshot = buildMessageThinkingFromRunEvents(events)
    expect(snapshot).toBeNull()
  })

  it('非 assistant role 的 message.delta 被过滤', () => {
    const events = [
      makeRunEvent({
        runId: 'run_1',
        seq: 1,
        type: 'run.segment.start',
        data: { segment_id: 'seg_1', kind: 'planning_round', display: { mode: 'collapsed', label: 'Plan' } },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 2,
        type: 'message.delta',
        data: { role: 'user', content_delta: 'user message' },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 3,
        type: 'message.delta',
        data: { role: 'assistant', content_delta: 'valid' },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 4,
        type: 'run.segment.end',
        data: { segment_id: 'seg_1' },
      }),
    ]

    const snapshot = buildMessageThinkingFromRunEvents(events)
    expect(snapshot).not.toBeNull()
    expect(snapshot?.segments[0].content).toBe('valid')
  })

  it('空 content_delta 被过滤', () => {
    const events = [
      makeRunEvent({
        runId: 'run_1',
        seq: 1,
        type: 'message.delta',
        data: { role: 'assistant', channel: 'thinking', content_delta: '' },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 2,
        type: 'message.delta',
        data: { role: 'assistant', channel: 'thinking', content_delta: 'real' },
      }),
    ]

    const snapshot = buildMessageThinkingFromRunEvents(events)
    expect(snapshot).not.toBeNull()
    expect(snapshot?.thinkingText).toBe('real')
  })

  it('segment.end 的 segment_id 不匹配当前活跃 segment 时不关闭', () => {
    const events = [
      makeRunEvent({
        runId: 'run_1',
        seq: 1,
        type: 'run.segment.start',
        data: { segment_id: 'seg_1', kind: 'planning_round', display: { mode: 'collapsed', label: 'Plan' } },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 2,
        type: 'run.segment.end',
        data: { segment_id: 'seg_wrong' },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 3,
        type: 'message.delta',
        data: { role: 'assistant', content_delta: 'still in seg_1' },
      }),
    ]

    const snapshot = buildMessageThinkingFromRunEvents(events)
    expect(snapshot).not.toBeNull()
    expect(snapshot?.segments).toHaveLength(1)
    expect(snapshot?.segments[0].content).toBe('still in seg_1')
  })

  it('content_delta 非 string 类型被过滤', () => {
    const events = [
      makeRunEvent({
        runId: 'run_1',
        seq: 1,
        type: 'message.delta',
        data: { role: 'assistant', channel: 'thinking', content_delta: 123 },
      }),
    ]

    const snapshot = buildMessageThinkingFromRunEvents(events)
    expect(snapshot).toBeNull()
  })
})
