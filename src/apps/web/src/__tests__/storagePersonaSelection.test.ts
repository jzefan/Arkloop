import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import {
  DEFAULT_PERSONA_KEY,
  SEARCH_PERSONA_KEY,
  readSelectedPersonaKeyFromStorage,
  writeSelectedPersonaKeyToStorage,
} from '../storage'

const nextKey = 'arkloop:web:selected_persona_key'

function createMemoryStorage(): Storage {
  const store = new Map<string, string>()
  return {
    get length() {
      return store.size
    },
    clear() {
      store.clear()
    },
    getItem(key: string) {
      return store.has(key) ? store.get(key)! : null
    },
    key(index: number) {
      return Array.from(store.keys())[index] ?? null
    },
    removeItem(key: string) {
      store.delete(key)
    },
    setItem(key: string, value: string) {
      store.set(key, value)
    },
  }
}

describe('selected persona storage', () => {
  const originalLocalStorage = globalThis.localStorage

  beforeEach(() => {
    const storage = createMemoryStorage()
    Object.defineProperty(globalThis, 'localStorage', { value: storage, configurable: true })
    Object.defineProperty(window, 'localStorage', { value: storage, configurable: true })
  })

  afterEach(() => {
    localStorage.clear()
    Object.defineProperty(globalThis, 'localStorage', { value: originalLocalStorage, configurable: true })
    Object.defineProperty(window, 'localStorage', { value: originalLocalStorage, configurable: true })
  })

  it('没有 persona_key 时返回默认值', () => {
    expect(readSelectedPersonaKeyFromStorage()).toBe(DEFAULT_PERSONA_KEY)
  })

  it('写入 persona_key', () => {
    writeSelectedPersonaKeyToStorage(SEARCH_PERSONA_KEY)

    expect(localStorage.getItem(nextKey)).toBe(SEARCH_PERSONA_KEY)
  })
})
