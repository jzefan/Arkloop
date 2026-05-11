export const PRODUCT_BRAND_NAME = '智能体平台'
export const PRODUCT_DESKTOP_VERSION = '0.0.1'
export const PRODUCT_SOURCE_LABEL = '来自于Arkloop'
export const PRODUCT_SOURCE_URL = 'https://github.com/qqqqqf-q/ArkLoop'

const LEGACY_BRAND_NAMES = ['Arkloop']

export function replaceLegacyBrandText(text: string): string {
  let next = text
  for (const legacyName of LEGACY_BRAND_NAMES) {
    if (!legacyName) continue
    next = next.replaceAll(legacyName, PRODUCT_BRAND_NAME)
  }
  return next
}
