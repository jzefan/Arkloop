import { describe, expect, it } from 'vitest'
import {
  assistantTurnPlainText,
  buildAssistantTurnFromRunEvents,
} from '../assistantTurnSegments'
import type { RunEvent } from '../sse'

function ev(runId: string, seq: number, type: string, data?: unknown, errorClass?: string): RunEvent {
  return {
    event_id: `evt_${seq}`,
    run_id: runId,
    seq,
    ts: `2026-03-20T00:00:${String(seq).padStart(2, '0')}.000Z`,
    type,
    data: data ?? {},
    error_class: errorClass,
  }
}

describe('buildAssistantTurnFromRunEvents', () => {
  it('合并连续 assistant 文本为单一 text segment', () => {
    const turn = buildAssistantTurnFromRunEvents([
      ev('r1', 1, 'message.delta', { role: 'assistant', content_delta: 'a' }),
      ev('r1', 2, 'message.delta', { role: 'assistant', content_delta: 'b' }),
    ])
    expect(turn.segments).toEqual([{ type: 'text', content: 'ab' }])
  })

  it('忽略 thinking channel', () => {
    const turn = buildAssistantTurnFromRunEvents([
      ev('r1', 1, 'message.delta', { role: 'assistant', channel: 'thinking', content_delta: 'hidden' }),
      ev('r1', 2, 'message.delta', { role: 'assistant', content_delta: 'visible' }),
    ])
    expect(turn.segments).toEqual([{ type: 'text', content: 'visible' }])
  })

  it('tool 之间的正文拆成独立 text，且位于两个 cop 之间（规范 §6 结构）', () => {
    const events: RunEvent[] = [
      ev('r1', 1, 'message.delta', { role: 'assistant', content_delta: '我来帮你读取 skills...' }),
      ev('r1', 2, 'tool.call', {
        tool_name: 'search_tools',
        tool_call_id: 'c1',
        arguments: { q: 'x' },
      }),
      ev('r1', 3, 'tool.call', {
        tool_name: 'read_file',
        tool_call_id: 'c2',
        arguments: { path: '/a' },
      }),
      ev('r1', 4, 'tool.result', {
        tool_name: 'search_tools',
        tool_call_id: 'c1',
        result: { ok: true },
      }),
      ev('r1', 5, 'tool.result', {
        tool_name: 'read_file',
        tool_call_id: 'c2',
        result: null,
      }),
      ev('r1', 6, 'message.delta', { role: 'assistant', content_delta: '让我重新读取：' }),
      ev('r1', 7, 'tool.call', {
        tool_name: 'read_file',
        tool_call_id: 'c3',
        arguments: { path: '/b' },
      }),
      ev('r1', 8, 'tool.result', {
        tool_name: 'read_file',
        tool_call_id: 'c3',
        result: { content: 'x' },
      }),
    ]

    const turn = buildAssistantTurnFromRunEvents(events)
    expect(turn.segments).toEqual([
      { type: 'text', content: '我来帮你读取 skills...' },
      {
        type: 'cop',
        title: null,
        calls: [
          {
            toolCallId: 'c1',
            toolName: 'search_tools',
            arguments: { q: 'x' },
            result: { ok: true },
          },
          {
            toolCallId: 'c2',
            toolName: 'read_file',
            arguments: { path: '/a' },
            result: null,
          },
        ],
      },
      { type: 'text', content: '让我重新读取：' },
      {
        type: 'cop',
        title: null,
        calls: [
          {
            toolCallId: 'c3',
            toolName: 'read_file',
            arguments: { path: '/b' },
            result: { content: 'x' },
          },
        ],
      },
    ])
    expect(assistantTurnPlainText(turn)).toBe('我来帮你读取 skills...让我重新读取：')
  })

  it('工具之间仅空白 message.delta 不拆分 cop', () => {
    const turn = buildAssistantTurnFromRunEvents([
      ev('r1', 1, 'message.delta', { role: 'assistant', content_delta: '读 skills：' }),
      ev('r1', 2, 'tool.call', { tool_name: 'cat', tool_call_id: 't1', arguments: {} }),
      ev('r1', 3, 'tool.call', { tool_name: 'cat', tool_call_id: 't2', arguments: {} }),
      ev('r1', 4, 'tool.call', { tool_name: 'cat', tool_call_id: 't3', arguments: {} }),
      ev('r1', 5, 'tool.call', { tool_name: 'cat', tool_call_id: 't4', arguments: {} }),
      ev('r1', 6, 'message.delta', { role: 'assistant', content_delta: '\n' }),
      ev('r1', 7, 'tool.call', { tool_name: 'cat', tool_call_id: 't5', arguments: {} }),
    ])
    expect(turn.segments).toHaveLength(2)
    expect(turn.segments[1]?.type).toBe('cop')
    if (turn.segments[1]?.type !== 'cop') throw new Error('expected cop')
    expect(turn.segments[1].calls).toHaveLength(5)
  })

  it('timeline_title 仅设置 cop.title，不进入 calls', () => {
    const turn = buildAssistantTurnFromRunEvents([
      ev('r1', 1, 'tool.call', {
        tool_name: 'timeline_title',
        tool_call_id: 't1',
        arguments: { label: '读取 Skills' },
      }),
      ev('r1', 2, 'tool.call', {
        tool_name: 'search_tools',
        tool_call_id: 'c1',
        arguments: {},
      }),
    ])
    expect(turn.segments).toHaveLength(1)
    expect(turn.segments[0]).toEqual({
      type: 'cop',
      title: '读取 Skills',
      calls: [{ toolCallId: 'c1', toolName: 'search_tools', arguments: {}, result: undefined }],
    })
  })

  it('seq 乱序时按 seq+ts 排序后折叠', () => {
    const turn = buildAssistantTurnFromRunEvents([
      ev('r1', 2, 'message.delta', { role: 'assistant', content_delta: 'second' }),
      ev('r1', 1, 'message.delta', { role: 'assistant', content_delta: 'first' }),
    ])
    expect(turn.segments).toEqual([{ type: 'text', content: 'firstsecond' }])
  })

  it('cop 内找不到 call 时挂占位 tool 行', () => {
    const turn = buildAssistantTurnFromRunEvents([
      ev('r1', 1, 'tool.call', { tool_name: 'exec_command', tool_call_id: 'c1', arguments: { command: 'ls' } }),
      ev('r1', 2, 'tool.result', {
        tool_name: 'exec_command',
        tool_call_id: 'orphan',
        result: { out: 1 },
      }),
    ])
    const first = turn.segments[0]
    expect(first?.type).toBe('cop')
    if (first?.type !== 'cop') throw new Error('expected cop')
    expect(first.calls).toHaveLength(2)
    expect(first.calls[0]?.toolCallId).toBe('c1')
    expect(first.calls[1]).toMatchObject({
      toolCallId: 'orphan',
      toolName: 'exec_command',
      arguments: {},
      result: { out: 1 },
    })
  })

  it('空 timeline_title 不单独产出空 cop', () => {
    const turn = buildAssistantTurnFromRunEvents([
      ev('r1', 1, 'tool.call', {
        tool_name: 'timeline_title',
        tool_call_id: 't1',
        arguments: { label: '' },
      }),
      ev('r1', 2, 'message.delta', { role: 'assistant', content_delta: 'hi' }),
    ])
    expect(turn.segments).toEqual([{ type: 'text', content: 'hi' }])
  })
})
