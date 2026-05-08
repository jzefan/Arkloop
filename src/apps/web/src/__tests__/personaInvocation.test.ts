import { describe, expect, it } from 'vitest'

import {
  filterPersonaMentionCandidates,
  getLeadingPersonaMentionQuery,
  parseLeadingPersonaInvocation,
} from '../personaInvocation'

const personas = [
  { persona_key: 'normal', selector_name: 'Normal' },
  { persona_key: 'extended-search', selector_name: 'Search' },
  { persona_key: 'work', selector_name: 'Work' },
  { persona_key: 'industry-education-index', selector_name: '双高产教融合评估' },
]

describe('parseLeadingPersonaInvocation', () => {
  it('matches a selector name at the beginning and strips the invocation', () => {
    expect(parseLeadingPersonaInvocation('@Search 查询今天的 AI 新闻', personas)).toEqual({
      personaKey: 'extended-search',
      body: '查询今天的 AI 新闻',
    })
  })

  it('matches a persona key case-insensitively', () => {
    expect(parseLeadingPersonaInvocation('@WORK 修复构建错误', personas)).toEqual({
      personaKey: 'work',
      body: '修复构建错误',
    })
  })

  it('does not match mentions outside the beginning of the message', () => {
    expect(parseLeadingPersonaInvocation('请让 @Search 查一下', personas)).toBeNull()
  })

  it('does not consume unknown invocations', () => {
    expect(parseLeadingPersonaInvocation('@Unknown 保持原样', personas)).toBeNull()
  })
})

describe('getLeadingPersonaMentionQuery', () => {
  it('returns an empty query when the draft starts with @', () => {
    expect(getLeadingPersonaMentionQuery('@')).toBe('')
  })

  it('returns the text after a leading @ before whitespace', () => {
    expect(getLeadingPersonaMentionQuery('@双高 评估苏州职业技术大学')).toBe('双高')
  })

  it('ignores @ outside the beginning of the draft', () => {
    expect(getLeadingPersonaMentionQuery('请使用 @双高')).toBeNull()
  })
})

describe('filterPersonaMentionCandidates', () => {
  it('returns all personas for an empty query', () => {
    expect(filterPersonaMentionCandidates(personas, '').map((persona) => persona.persona_key)).toEqual([
      'normal',
      'extended-search',
      'work',
      'industry-education-index',
    ])
  })

  it('filters by selector name or persona key', () => {
    expect(filterPersonaMentionCandidates(personas, '双高')).toEqual([
      { persona_key: 'industry-education-index', selector_name: '双高产教融合评估' },
    ])
    expect(filterPersonaMentionCandidates(personas, 'extended')).toEqual([
      { persona_key: 'extended-search', selector_name: 'Search' },
    ])
  })
})
