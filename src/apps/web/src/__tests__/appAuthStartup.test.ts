import { describe, expect, it } from 'vitest'

import { shouldDelayLocalSession, shouldUseLocalSetupRoute } from '../appAuthStartup'

describe('app auth startup', () => {
  it('delays local session exchange until desktop runtime is ready', () => {
    expect(shouldDelayLocalSession(true, false, true)).toBe(true)
    expect(shouldDelayLocalSession(true, true, null)).toBe(true)
    expect(shouldDelayLocalSession(true, true, false)).toBe(true)
    expect(shouldDelayLocalSession(true, true, true)).toBe(false)
  })

  it('does not delay non-local auth startup', () => {
    expect(shouldDelayLocalSession(false, false, null)).toBe(false)
  })

  it('uses local setup route only when local mode has no session', () => {
    expect(shouldUseLocalSetupRoute(true, null)).toBe(true)
    expect(shouldUseLocalSetupRoute(true, '')).toBe(true)
    expect(shouldUseLocalSetupRoute(true, 'jwt-token')).toBe(false)
    expect(shouldUseLocalSetupRoute(false, null)).toBe(false)
  })
})
