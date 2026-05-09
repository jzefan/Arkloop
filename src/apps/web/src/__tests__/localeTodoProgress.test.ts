import { describe, expect, it } from 'vitest'
import { zh } from '../locales/zh'

describe('zh todo progress copy', () => {
  it('uses Chinese labels for progress cards', () => {
    expect(zh.todoListTitle).toBe('任务进度')
    expect(zh.todoListProgress(2, 4)).toBe('已完成 2 / 4')
    expect(zh.todoChangeCompleted(2, 4)).toBe('已完成 2 / 4')
    expect(zh.todoChangeStarted(2, 4)).toBe('开始第 2 / 4 项')
    expect(zh.todoChangeCancelled(2, 4)).toBe('已取消第 2 / 4 项')
    expect(zh.todoChangeUpdated(2, 4)).toBe('已更新第 2 / 4 项')
  })
})
