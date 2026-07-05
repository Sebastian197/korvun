// Same-origin control-API client (ADR-0030 §4). The bearer token is held only in
// React state by the caller and passed in per request as `Authorization: Bearer`
// — never a cookie (ADR-0028 CSRF-by-construction), never persisted by default.

export interface ModelSummary {
  provider: string
  model_id: string
}
export interface BrainSummary {
  name: string
  sensitivity: string
  policy: string
  dispatch: string
  models: ModelSummary[]
}
export interface ChannelSummary {
  type: string
  mode: string
  name: string
  dropped?: number
}

/** The raw config document is operator-shaped JSON; the read-only cut renders it
 *  verbatim, so an opaque record is the honest type here. */
export type RawConfig = Record<string, unknown>

async function getJSON<T>(path: string, token?: string): Promise<T> {
  const headers: HeadersInit = {}
  if (token) headers['Authorization'] = `Bearer ${token}`
  const res = await fetch(path, { headers })
  if (!res.ok) {
    throw new Error(`${path}: ${res.status}`)
  }
  return (await res.json()) as T
}

export const getBrains = () => getJSON<BrainSummary[]>('/api/brains')
export const getChannels = () => getJSON<ChannelSummary[]>('/api/channels')
export const getConfig = (token: string) => getJSON<RawConfig>('/api/config', token)
