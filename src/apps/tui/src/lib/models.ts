import type { ApiClient } from "../api/client"
import type { LlmProvider } from "../api/types"

export interface FlatModel {
  id: string
  provider: string
  label: string
  isDefault: boolean
  showInPicker: boolean
  supportsReasoning: boolean
  contextLength: number | null
}

export async function listFlatModels(client: ApiClient): Promise<FlatModel[]> {
  const providers = normalizeProviders(await client.listModels())
  const result: FlatModel[] = []

  for (const provider of providers) {
    for (const model of provider.models ?? []) {
      const id = (model.model ?? "").trim()
      if (!id) continue
      result.push({
        id,
        provider: provider.name,
        label: id,
        isDefault: model.is_default === true,
        showInPicker: model.show_in_picker !== false,
        supportsReasoning: supportsReasoning(model.advanced_json),
        contextLength: contextLength(model.advanced_json),
      })
    }
  }

  return result
}

export function defaultModel(models: FlatModel[]): FlatModel | null {
  return models.find((item) => item.isDefault && item.showInPicker)
    ?? models.find((item) => item.showInPicker)
    ?? models[0]
    ?? null
}

export function findModel(models: FlatModel[], query: string): FlatModel | null {
  const needle = query.trim().toLowerCase()
  if (!needle) return null
  return models.find((item) => item.id.toLowerCase() === needle)
    ?? models.find((item) => `${item.provider}:${item.id}`.toLowerCase() === needle)
    ?? models.find((item) => `${item.provider}/${item.id}`.toLowerCase() === needle)
    ?? null
}

function normalizeProviders(raw: unknown): LlmProvider[] {
  if (Array.isArray(raw)) return raw as LlmProvider[]
  const wrapped = raw as { data?: LlmProvider[] } | null
  return Array.isArray(wrapped?.data) ? wrapped.data : []
}

function supportsReasoning(advancedJSON?: Record<string, unknown> | null): boolean {
  if (!advancedJSON) return false
  const catalog = advancedJSON["available_catalog"]
  if (!catalog || typeof catalog !== "object" || Array.isArray(catalog)) return false
  return (catalog as Record<string, unknown>).reasoning === true
}

function contextLength(advancedJSON?: Record<string, unknown> | null): number | null {
  if (!advancedJSON) return null
  const catalog = advancedJSON["available_catalog"]
  if (!catalog || typeof catalog !== "object" || Array.isArray(catalog)) return null
  const raw = (catalog as Record<string, unknown>).context_length
  return typeof raw === "number" && raw > 0 ? raw : null
}
