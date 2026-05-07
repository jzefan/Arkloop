export type ZenMuxModel = {
  id: string
  slug?: string
  name?: string
  display_name?: string
  input_modalities?: string[]
  output_modalities?: string[]
}

type ZenMuxModelListPayload = {
  data?: ZenMuxModel[]
}

const ZENMUX_MODEL_LIST_URL = 'https://zenmux.ai/api/frontend/model/available/list?sort=newest'

const cachedModels = new Map<string, ZenMuxModel[]>()
const pendingModels = new Map<string, Promise<ZenMuxModel[]>>()

export async function listZenMuxModels(modality?: string): Promise<ZenMuxModel[]> {
  const key = modality?.trim() || 'all'
  const cached = cachedModels.get(key)
  if (cached) return cached
  let pending = pendingModels.get(key)
  if (!pending) {
    const url = modality
      ? `${ZENMUX_MODEL_LIST_URL}&output_modalities=${encodeURIComponent(modality)}`
      : ZENMUX_MODEL_LIST_URL
    pending = fetch(url, { headers: { Accept: 'application/json' } })
      .then((response) => {
        if (!response.ok) throw new Error(`ZenMux models ${response.status}`)
        return response.json() as Promise<ZenMuxModelListPayload>
      })
      .then((payload) => {
        const models = Array.isArray(payload.data) ? payload.data : []
        cachedModels.set(key, models)
        return models
      })
      .finally(() => {
        pendingModels.delete(key)
      })
    pendingModels.set(key, pending)
  }
  return pending
}

export function zenMuxModelId(model: ZenMuxModel): string {
  return (model.slug ?? model.id).trim()
}

export function zenMuxModelLabel(model: ZenMuxModel): string {
  return (model.name ?? model.display_name ?? model.slug ?? model.id).trim()
}

export function zenMuxModelSupports(model: ZenMuxModel, modality: string): boolean {
  return Array.isArray(model.output_modalities) && model.output_modalities.includes(modality)
}
