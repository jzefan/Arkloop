export const effortLevels = ["none", "minimal", "low", "medium", "high", "max"] as const

export type EffortLevel = typeof effortLevels[number]

const effortLabels: Record<EffortLevel, string> = {
  none: "None",
  minimal: "Minimal",
  low: "Low",
  medium: "Medium",
  high: "High",
  max: "Max",
}

const effortSymbols: Record<EffortLevel, string> = {
  none: "",
  minimal: "◌",
  low: "○",
  medium: "◐",
  high: "●",
  max: "◉",
}

export function parseEffort(value: string): EffortLevel | null {
  const normalized = value.trim().toLowerCase()
  return effortLevels.find((item) => item === normalized) ?? null
}

export function formatEffort(value: EffortLevel): string {
  return effortLabels[value]
}

export function effortSymbol(value: EffortLevel): string {
  return effortSymbols[value]
}

export function cycleEffort(current: EffortLevel, direction: "left" | "right"): EffortLevel {
  const index = effortLevels.indexOf(current)
  const next = direction === "left" ? index - 1 : index + 1
  if (next < 0) return effortLevels[0]
  if (next >= effortLevels.length) return effortLevels[effortLevels.length - 1]
  return effortLevels[next]
}
