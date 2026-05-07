export function shouldDelayLocalSession(localMode: boolean, sidecarReady: boolean, onboardingDone: boolean | null): boolean {
  return localMode && (!sidecarReady || onboardingDone !== true)
}

export function shouldUseLocalSetupRoute(localMode: boolean, accessToken: string | null): boolean {
  return localMode && !accessToken
}
