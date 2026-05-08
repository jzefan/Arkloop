export type PersonaInvocationCandidate = {
  persona_key: string
  selector_name: string
}

export type PersonaInvocation = {
  personaKey: string
  body: string
}

function normalizeInvocationName(value: string): string {
  return value.trim().toLowerCase()
}

export function getLeadingPersonaMentionQuery(input: string): string | null {
  const match = input.match(/^\s*@([^\s@]*)/u)
  if (!match) return null
  return match[1] ?? ''
}

export function filterPersonaMentionCandidates<T extends PersonaInvocationCandidate>(
  personas: T[],
  query: string,
): T[] {
  const normalizedQuery = normalizeInvocationName(query)
  if (!normalizedQuery) return personas
  return personas.filter((persona) => (
    normalizeInvocationName(persona.persona_key).includes(normalizedQuery) ||
    normalizeInvocationName(persona.selector_name).includes(normalizedQuery)
  ))
}

export function parseLeadingPersonaInvocation(
  input: string,
  personas: PersonaInvocationCandidate[],
): PersonaInvocation | null {
  const match = input.match(/^\s*@([^\s@]+)(?:\s+|$)([\s\S]*)$/u)
  if (!match) return null

  const requested = normalizeInvocationName(match[1] ?? '')
  if (!requested) return null

  const persona = personas.find((item) => (
    normalizeInvocationName(item.persona_key) === requested ||
    normalizeInvocationName(item.selector_name) === requested
  ))
  if (!persona) return null

  return {
    personaKey: persona.persona_key,
    body: match[2] ?? '',
  }
}
