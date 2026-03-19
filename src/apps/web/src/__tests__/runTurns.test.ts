import { describe, expect, it } from 'vitest'
import { buildTurns, type RunEventRaw } from '@arkloop/shared/run-turns'

function makeEvent(params: {
  seq: number
  type: string
  data?: Record<string, unknown>
}): RunEventRaw {
  return {
    event_id: `evt_${params.seq}`,
    run_id: 'run_1',
    seq: params.seq,
    ts: '2026-03-18T00:00:00.000Z',
    type: params.type,
    data: params.data ?? {},
  }
}

describe('buildTurns', () => {
  it('应忽略 thinking channel，只保留正常 assistant 输出', () => {
    const turns = buildTurns([
      makeEvent({
        seq: 1,
        type: 'llm.request',
        data: {
          payload: {
            messages: [{ role: 'user', content: '帮我查一下' }],
          },
        },
      }),
      makeEvent({
        seq: 2,
        type: 'message.delta',
        data: { role: 'assistant', channel: 'thinking', content_delta: '先想一下' },
      }),
      makeEvent({
        seq: 3,
        type: 'message.delta',
        data: { role: 'assistant', content_delta: '我先检查一下现有工具。' },
      }),
      makeEvent({
        seq: 4,
        type: 'run.completed',
        data: {},
      }),
    ])

    expect(turns).toHaveLength(1)
    expect(turns[0]?.assistantText).toBe('我先检查一下现有工具。')
  })

  it('应从 telegram envelope 消息中提取正文并保留 assistant 历史', () => {
    const turns = buildTurns([
      makeEvent({
        seq: 1,
        type: 'llm.request',
        data: {
          llm_call_id: 'call_1',
          provider_kind: 'openai',
          api_mode: 'chat_completions',
          payload: {
            model: 'openai/gpt-4o-mini',
            messages: [
              { role: 'system', content: '你是Arkloop' },
              {
                role: 'user',
                content: '---\nchannel-identity-id: "551ab5d6-0239-46d7-bf91-2ef87c9454d0"\ndisplay-name: "清凤"\nchannel: "telegram"\nconversation-type: "private"\ntime: "2026-03-19T10:19:34Z"\n---\n你是谁',
              },
              {
                role: 'assistant',
                content: '我是Arkloop，很高兴见到你！我可以帮助你回答问题、提供信息或讨论各种话题。有任何想了解的，请随时问我。',
              },
              {
                role: 'user',
                content: '---\nchannel-identity-id: "551ab5d6-0239-46d7-bf91-2ef87c9454d0"\ndisplay-name: "清凤"\nchannel: "telegram"\nconversation-type: "private"\ntime: "2026-03-19T10:19:42Z"\n---\n我上一句话说的什么',
              },
              {
                role: 'assistant',
                content: '你上一句话是问：“你是谁”。如果你还有其他问题或者想聊的话题，请随时告诉我！',
              },
              {
                role: 'user',
                content: '---\nchannel-identity-id: "551ab5d6-0239-46d7-bf91-2ef87c9454d0"\ndisplay-name: "清凤"\nchannel: "telegram"\nconversation-type: "private"\ntime: "2026-03-19T10:44:37Z"\n---\n卡哇伊',
              },
            ],
          },
        },
      }),
      makeEvent({
        seq: 2,
        type: 'message.delta',
        data: { role: 'assistant', content_delta: '你这是在夸可爱呀。' },
      }),
      makeEvent({
        seq: 3,
        type: 'message.delta',
        data: { role: 'assistant', content_delta: '确实很卡哇伊。' },
      }),
      makeEvent({
        seq: 4,
        type: 'llm.turn.completed',
        data: {
          llm_call_id: 'call_1',
          usage: { input_tokens: 10, output_tokens: 8 },
        },
      }),
      makeEvent({
        seq: 5,
        type: 'run.completed',
        data: {},
      }),
    ])

    expect(turns).toHaveLength(1)
    expect(turns[0]?.userInput).toBe('卡哇伊')
    expect(turns[0]?.inputMeta?.channel).toBe('telegram')
    expect(turns[0]?.inputMeta?.['conversation-type']).toBe('private')
    expect(turns[0]?.requestMessages).toEqual([
      {
        role: 'user',
        text: '你是谁',
        meta: {
          'channel-identity-id': '551ab5d6-0239-46d7-bf91-2ef87c9454d0',
          'display-name': '清凤',
          channel: 'telegram',
          'conversation-type': 'private',
          time: '2026-03-19T10:19:34Z',
        },
      },
      {
        role: 'assistant',
        text: '我是Arkloop，很高兴见到你！我可以帮助你回答问题、提供信息或讨论各种话题。有任何想了解的，请随时问我。',
      },
      {
        role: 'user',
        text: '我上一句话说的什么',
        meta: {
          'channel-identity-id': '551ab5d6-0239-46d7-bf91-2ef87c9454d0',
          'display-name': '清凤',
          channel: 'telegram',
          'conversation-type': 'private',
          time: '2026-03-19T10:19:42Z',
        },
      },
      {
        role: 'assistant',
        text: '你上一句话是问：“你是谁”。如果你还有其他问题或者想聊的话题，请随时告诉我！',
      },
      {
        role: 'user',
        text: '卡哇伊',
        meta: {
          'channel-identity-id': '551ab5d6-0239-46d7-bf91-2ef87c9454d0',
          'display-name': '清凤',
          channel: 'telegram',
          'conversation-type': 'private',
          time: '2026-03-19T10:44:37Z',
        },
      },
    ])
    expect(turns[0]?.assistantText).toBe('你这是在夸可爱呀。确实很卡哇伊。')
  })

  it('应先按 seq 排序，再处理乱序返回的事件', () => {
    const turns = buildTurns([
      makeEvent({
        seq: 2,
        type: 'message.delta',
        data: { role: 'assistant', content_delta: '的' },
      }),
      makeEvent({
        seq: 3,
        type: 'message.delta',
        data: { role: 'assistant', content_delta: '确' },
      }),
      makeEvent({
        seq: 1,
        type: 'llm.request',
        data: {
          llm_call_id: 'call_1',
          provider_kind: 'openai',
          api_mode: 'chat_completions',
          payload: {
            messages: [
              { role: 'system', content: '你是Arkloop' },
              { role: 'user', content: '---\nchannel: "telegram"\nconversation-type: "private"\n---\n卡哇伊' },
            ],
          },
        },
      }),
      makeEvent({
        seq: 5,
        type: 'llm.turn.completed',
        data: {
          llm_call_id: 'call_1',
          usage: { input_tokens: 1, output_tokens: 2 },
        },
      }),
      makeEvent({
        seq: 6,
        type: 'run.completed',
        data: {},
      }),
      makeEvent({
        seq: 4,
        type: 'message.delta',
        data: { role: 'assistant', content_delta: '实' },
      }),
    ])

    expect(turns).toHaveLength(1)
    expect(turns[0]?.userInput).toBe('卡哇伊')
    expect(turns[0]?.assistantText).toBe('的确实')
  })
})
