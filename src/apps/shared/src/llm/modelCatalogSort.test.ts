import { describe, expect, it } from 'vitest'
import { sortAvailableModelsNewestFirst } from './modelCatalogSort'

describe('sortAvailableModelsNewestFirst', () => {
  it('sorts common imported model names from newest to oldest', () => {
    const sorted = sortAvailableModelsNewestFirst([
      { id: 'qwen/qwen2.5-72b-instruct', name: 'Qwen2.5 72B Instruct' },
      { id: 'qwen/qwen3-32b', name: 'Qwen3 32B' },
      { id: 'qwen/qwen3-235b-a22b-instruct-2504', name: 'Qwen3 235B A22B 2504' },
      { id: 'qwen/qwen3-235b-a22b-instruct-2507', name: 'Qwen3 235B A22B 2507' },
      { id: 'qwen/qwen-max', name: 'Qwen Max' },
    ]).map((model) => model.id)

    expect(sorted).toEqual([
      'qwen/qwen3-235b-a22b-instruct-2507',
      'qwen/qwen3-235b-a22b-instruct-2504',
      'qwen/qwen3-32b',
      'qwen/qwen2.5-72b-instruct',
      'qwen/qwen-max',
    ])
  })

  it('keeps chat models before utility model types when freshness is similar', () => {
    const sorted = sortAvailableModelsNewestFirst([
      { id: 'deepseek/deepseek-embedding-v1', name: 'DeepSeek Embedding V1', type: 'embedding' },
      { id: 'deepseek/deepseek-v3.1', name: 'DeepSeek V3.1', type: 'chat' },
      { id: 'anthropic/claude-opus-4-7', name: 'Claude Opus 4.7', type: 'chat' },
      { id: 'anthropic/claude-sonnet-4-6', name: 'Claude Sonnet 4.6', type: 'chat' },
    ]).map((model) => model.id)

    expect(sorted).toEqual([
      'anthropic/claude-opus-4-7',
      'anthropic/claude-sonnet-4-6',
      'deepseek/deepseek-v3.1',
      'deepseek/deepseek-embedding-v1',
    ])
  })
})
