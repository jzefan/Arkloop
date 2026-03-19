import { describe, expect, it } from 'vitest'
import { buildTurns, type RunEventRaw } from './run-turns'

describe('buildTurns', () => {
  it('extracts telegram envelope input and assistant output', () => {
    const events: RunEventRaw[] = [
      {
        event_id: '1',
        run_id: 'r1',
        seq: 1,
        ts: '2026-03-19T10:19:42Z',
        type: 'llm.request',
        data: {
          llm_call_id: 'call-1',
          provider_kind: 'openai',
          api_mode: 'chat_completions',
          payload: {
            model: 'openai/gpt-4o-mini',
            input: '---
channel-identity-id: "u1"
display-name: "清风"
channel: "telegram"
conversation-type: "private"
time: "2026-03-19T10:19:42Z"
---
我上一句话说的什么',
          },
        },
      },
      {
        event_id: '2',
        run_id: 'r1',
        seq: 2,
        ts: '2026-03-19T10:19:43Z',
        type: 'message.delta',
        data: {
          role: 'assistant',
          content_delta: '你上一句是：',
        },
      },
      {
        event_id: '3',
        run_id: 'r1',
        seq: 3,
        ts: '2026-03-19T10:19:44Z',
        type: 'message.delta',
        data: {
          role: 'assistant',
          content_delta: '我上一句话说的什么',
        },
      },
      {
        event_id: '4',
        run_id: 'r1',
        seq: 4,
        ts: '2026-03-19T10:19:45Z',
        type: 'llm.turn.completed',
        data: {
          llm_call_id: 'call-1',
          usage: {
            input_tokens: 10,
            output_tokens: 8,
          },
        },
      },
    ]

    const turns = buildTurns(events)
    expect(turns).toHaveLength(1)
    expect(turns[0].userInput).toBe('我上一句话说的什么')
    expect(turns[0].inputMeta).toEqual({
      'channel-identity-id': 'u1',
      'display-name': '清风',
      channel: 'telegram',
      'conversation-type': 'private',
      time: '2026-03-19T10:19:42Z',
    })
    expect(turns[0].assistantText).toBe('你上一句是：我上一句话说的什么')
  })

  it('keeps prior request messages visible for channel runs', () => {
    const events: RunEventRaw[] = [
      {
        event_id: '1',
        run_id: 'r1',
        seq: 1,
        ts: '2026-03-19T10:44:37Z',
        type: 'llm.request',
        data: {
          llm_call_id: 'call-1',
          provider_kind: 'openai',
          api_mode: 'chat_completions',
          payload: {
            messages: [
              { role: 'system', content: '你是Arkloop' },
              {
                role: 'user',
                content: '---\nchannel: "telegram"\nconversation-type: "private"\ndisplay-name: "清风"\n---\n你是谁',
              },
              {
                role: 'assistant',
                content: '我是Arkloop，很高兴见到你！',
              },
              {
                role: 'user',
                content: '---\nchannel: "telegram"\nconversation-type: "private"\ndisplay-name: "清风"\n---\n我上一句话说的什么',
              },
            ],
          },
        },
      },
      {
        event_id: '2',
        run_id: 'r1',
        seq: 2,
        ts: '2026-03-19T10:44:39Z',
        type: 'message.delta',
        data: {
          role: 'assistant',
          content_delta: '你上一句说的是“你是谁”。',
        },
      },
    ]

    const turns = buildTurns(events)
    expect(turns).toHaveLength(1)
    expect(turns[0].requestMessages).toEqual([
      {
        role: 'user',
        text: '你是谁',
        meta: {
          channel: 'telegram',
          'conversation-type': 'private',
          'display-name': '清风',
        },
      },
      {
        role: 'assistant',
        text: '我是Arkloop，很高兴见到你！',
      },
      {
        role: 'user',
        text: '我上一句话说的什么',
        meta: {
          channel: 'telegram',
          'conversation-type': 'private',
          'display-name': '清风',
        },
      },
    ])
  })

  it('falls back to completed assistant text when no visible delta exists', () => {
    const events: RunEventRaw[] = [
      {
        event_id: '1',
        run_id: 'r1',
        seq: 1,
        ts: '2026-03-19T10:19:42Z',
        type: 'llm.request',
        data: {
          llm_call_id: 'call-1',
          provider_kind: 'openai',
          api_mode: 'responses',
          payload: {
            input: 'hello',
          },
        },
      },
      {
        event_id: '2',
        run_id: 'r1',
        seq: 2,
        ts: '2026-03-19T10:19:43Z',
        type: 'message.delta',
        data: {
          channel: 'thinking',
          content_delta: 'internal',
        },
      },
      {
        event_id: '3',
        run_id: 'r1',
        seq: 3,
        ts: '2026-03-19T10:19:44Z',
        type: 'llm.turn.completed',
        data: {
          llm_call_id: 'call-1',
          assistant_text: 'done',
        },
      },
      {
        event_id: '4',
        run_id: 'r1',
        seq: 4,
        ts: '2026-03-19T10:19:45Z',
        type: 'run.completed',
        data: {},
      },
    ]

    const turns = buildTurns(events)
    expect(turns).toHaveLength(1)
    expect(turns[0].assistantText).toBe('done')
  })
})
