export type SortableAvailableModel = {
  id: string
  name?: string | null
  type?: string | null
}

type Freshness = {
  version: number[]
  date: number
}

const NATURAL_COLLATOR = new Intl.Collator(undefined, {
  numeric: true,
  sensitivity: 'base',
})

const FAMILY_VERSION_PATTERN =
  /(?:^|[-_/ .])(?:qwen|qwq|deepseek|doubao|gpt|claude|gemini|kimi|glm|yi|llama|mistral|mixtral|command|ernie|baichuan|moonshot|hunyuan|minimax|step|internlm|phi|nova|sonar|reka|jamba|spark)[-_. ]?v?(\d+(?:\.\d+){0,2})(?=$|[-_/ .])/gi
const NAMED_VERSION_PATTERN =
  /(?:^|[-_/ .])(?:opus|sonnet|haiku|flash|pro|turbo|mini)[-_. ](\d+(?:[-_.]\d+){0,2})(?=$|[-_/ .])/gi
const EXPLICIT_VERSION_PATTERN = /(?:^|[-_/ .])v(\d+(?:\.\d+){0,2})(?=$|[-_/ .])/gi

function displayName(model: SortableAvailableModel): string {
  return model.name?.trim() || model.id
}

function compareNumbersDesc(a: number, b: number): number {
  return b - a
}

function compareVersionsDesc(a: number[], b: number[]): number {
  const aKnown = a.length > 0
  const bKnown = b.length > 0
  if (aKnown !== bKnown) return aKnown ? -1 : 1

  const length = Math.max(a.length, b.length)
  for (let index = 0; index < length; index++) {
    const diff = (b[index] ?? 0) - (a[index] ?? 0)
    if (diff !== 0) return diff
  }
  return 0
}

function compareVersionsAsc(a: number[], b: number[]): number {
  const length = Math.max(a.length, b.length)
  for (let index = 0; index < length; index++) {
    const diff = (a[index] ?? 0) - (b[index] ?? 0)
    if (diff !== 0) return diff
  }
  return 0
}

function parseVersion(value: string): number[] {
  return value
    .replace(/[-_]/g, '.')
    .split('.')
    .map((part) => Number.parseInt(part, 10))
    .filter((part) => Number.isFinite(part))
}

function collectVersions(text: string): number[][] {
  const versions: number[][] = []
  for (const pattern of [FAMILY_VERSION_PATTERN, NAMED_VERSION_PATTERN, EXPLICIT_VERSION_PATTERN]) {
    pattern.lastIndex = 0
    let match: RegExpExecArray | null
    while ((match = pattern.exec(text)) !== null) {
      const version = parseVersion(match[1] ?? '')
      if (version.length > 0) versions.push(version)
    }
  }
  return versions
}

function newestVersion(text: string): number[] {
  return collectVersions(text).sort(compareVersionsAsc).at(-1) ?? []
}

function toDateScore(year: number, month: number, day = 1): number {
  if (month < 1 || month > 12 || day < 1 || day > 31) return 0
  return year * 10000 + month * 100 + day
}

function newestDate(text: string): number {
  const scores: number[] = []

  for (const match of text.matchAll(/(?:^|[^0-9])(20\d{2})[-_.]?([01]\d)[-_.]?([0-3]\d)(?:[^0-9]|$)/g)) {
    scores.push(toDateScore(Number(match[1]), Number(match[2]), Number(match[3])))
  }
  for (const match of text.matchAll(/(?:^|[^0-9])(20\d{2})[-_.]?([01]\d)(?:[^0-9]|$)/g)) {
    scores.push(toDateScore(Number(match[1]), Number(match[2])))
  }
  for (const match of text.matchAll(/(?:^|[^0-9])([2-3]\d)([01]\d)([0-3]\d)(?:[^0-9]|$)/g)) {
    scores.push(toDateScore(2000 + Number(match[1]), Number(match[2]), Number(match[3])))
  }
  for (const match of text.matchAll(/(?:^|[^0-9])([2-3]\d)([01]\d)(?:[^0-9]|$)/g)) {
    scores.push(toDateScore(2000 + Number(match[1]), Number(match[2])))
  }

  return Math.max(0, ...scores)
}

function typeRank(model: SortableAvailableModel): number {
  const type = model.type?.toLowerCase()
  if (!type || type === 'chat') return 0
  if (type === 'reasoning') return 1
  if (type === 'image' || type === 'audio') return 2
  if (type === 'embedding') return 3
  return 4
}

function readFreshness(model: SortableAvailableModel): Freshness {
  const text = `${model.id} ${model.name ?? ''}`.toLowerCase()
  return {
    version: newestVersion(text),
    date: newestDate(text),
  }
}

export function compareAvailableModelsNewestFirst<T extends SortableAvailableModel>(a: T, b: T): number {
  const aFreshness = readFreshness(a)
  const bFreshness = readFreshness(b)

  return (
    compareVersionsDesc(aFreshness.version, bFreshness.version) ||
    compareNumbersDesc(aFreshness.date, bFreshness.date) ||
    typeRank(a) - typeRank(b) ||
    NATURAL_COLLATOR.compare(displayName(a), displayName(b)) ||
    NATURAL_COLLATOR.compare(a.id, b.id)
  )
}

export function sortAvailableModelsNewestFirst<T extends SortableAvailableModel>(models: readonly T[]): T[] {
  return [...models].sort(compareAvailableModelsNewestFirst)
}
