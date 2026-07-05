// Same-origin control-API client (ADR-0030 §4). The bearer token is held only in
// React state by the caller and passed in per request as `Authorization: Bearer`
// — never a cookie (ADR-0028 CSRF-by-construction), never persisted by default.

import type { Config } from './config/schema'

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

/** The reload handle returned by a successful POST /api/config (202). */
export interface ReloadHandle {
  handle: string
}

/** An HTTP error carrying the status so callers can branch (401 re-auth, the two
 *  409 codes, 400 validation — the full mapping is 2b.2c). */
export class HttpError extends Error {
  readonly status: number
  readonly body: string
  constructor(status: number, body: string) {
    super(`HTTP ${status}`)
    this.name = 'HttpError'
    this.status = status
    this.body = body
  }
}

async function req(path: string, init?: RequestInit): Promise<Response> {
  const res = await fetch(path, init)
  if (!res.ok) {
    throw new HttpError(res.status, await res.text().catch(() => ''))
  }
  return res
}

function auth(token: string): HeadersInit {
  return { Authorization: `Bearer ${token}` }
}

export const getBrains = async (): Promise<BrainSummary[]> =>
  (await req('/api/brains')).json()
export const getChannels = async (): Promise<ChannelSummary[]> =>
  (await req('/api/channels')).json()
export const getConfig = async (token: string): Promise<Config> =>
  (await req('/api/config', { headers: auth(token) })).json()

/** POST the full working-copy config. Returns the reload handle (202). */
export const postConfig = async (token: string, config: Config): Promise<ReloadHandle> =>
  (
    await req('/api/config', {
      method: 'POST',
      headers: { ...auth(token), 'Content-Type': 'application/json' },
      body: JSON.stringify(config),
    })
  ).json()

/** Poll a reload handle's state (ADR-0030 §5). Returns the raw `state` string, or
 *  REJECTS on a transient network error — which the reload poll treats as a retry
 *  (the admin server is restarting mid-cutover; the handle survives, F4). */
export const getReloadStatus = async (handle: string, token: string): Promise<string> => {
  const r = await req(`/api/reload/${encodeURIComponent(handle)}`, { headers: auth(token) })
  const body = (await r.json()) as { state: string }
  return body.state
}
