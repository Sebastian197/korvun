// The config-mutation pipe (SP6c): the wizard (add) and Canales (remove)
// both take the current core config, transform it purely, and push it
// through the SP4 proxy — GET /api/config → POST /api/config → poll
// /api/reload/{handle} to a terminal state. The bearer is injected by the
// proxy server-side, so nothing here carries a token; the config carries
// only env-var NAMES, never secret values. This is the exact pipe the SP5
// e2e (TestFirstRun_builderAddsFirstChannel) already proved; the UI is its
// first human consumer.

/** The core config, only the fields the chrome reads or transforms. Unknown
 * fields ride through untouched so a POST never drops config the chrome does
 * not model (agents, storage, observability, …). */
export interface ChannelConfig {
  type: string
  mode: string
  token_env: string
}

export interface RouteConfig {
  channel: string
  brain: string
}

export interface CoreConfig {
  channels: ChannelConfig[]
  brains: Array<{ name: string } & Record<string, unknown>>
  routes: RouteConfig[]
  [k: string]: unknown
}

/** Append a channel and wire it to the first brain (mirrors the SP5 pipe:
 * a channel with no route is inert). Pure — the input is never mutated. */
export function addChannel(cfg: CoreConfig, ch: ChannelConfig): CoreConfig {
  const firstBrain = cfg.brains[0]?.name ?? ''
  const routes =
    firstBrain === '' ? cfg.routes : [...cfg.routes, { channel: ch.type, brain: firstBrain }]
  return { ...cfg, channels: [...cfg.channels, ch], routes }
}

/** Drop a channel by type and any route that pointed at it (a dangling route
 * fails config.Validate). Pure. */
export function removeChannel(cfg: CoreConfig, type: string): CoreConfig {
  return {
    ...cfg,
    channels: cfg.channels.filter((c) => c.type !== type),
    routes: cfg.routes.filter((r) => r.channel !== type),
  }
}

export interface MutateResult {
  ok: boolean
  /** Terminal reload state, or 'post-failed' / 'timeout' when the pipe itself
   * failed before a reload state was observed. */
  state: string
  detail?: string
}

export interface MutateOptions {
  fetcher?: typeof fetch
  pollIntervalMs?: number
  maxPolls?: number
}

/** Build a MutateOptions with only the defined keys — the two callers (the
 * wizard's add, Canales' remove) both need this under
 * exactOptionalPropertyTypes. */
export function mutateOpts(fetcher?: typeof fetch, pollIntervalMs?: number): MutateOptions {
  const o: MutateOptions = {}
  if (fetcher !== undefined) o.fetcher = fetcher
  if (pollIntervalMs !== undefined) o.pollIntervalMs = pollIntervalMs
  return o
}

const TERMINAL = new Set(['succeeded', 'failed', 'rolled-back'])

function sleep(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms))
}

/** Run one config mutation end-to-end. Never throws — every failure is a
 * MutateResult the UI paints. */
export async function mutateConfig(
  transform: (cfg: CoreConfig) => CoreConfig,
  opts: MutateOptions = {},
): Promise<MutateResult> {
  const fetcher = opts.fetcher ?? fetch
  const pollIntervalMs = opts.pollIntervalMs ?? 400
  const maxPolls = opts.maxPolls ?? 60

  let current: CoreConfig
  try {
    const resp = await fetcher('/api/config', { cache: 'no-store' })
    if (!resp.ok)
      return { ok: false, state: 'get-failed', detail: `GET /api/config ${resp.status}` }
    current = (await resp.json()) as CoreConfig
  } catch (e) {
    return { ok: false, state: 'get-failed', detail: e instanceof Error ? e.message : String(e) }
  }

  const next = transform(current)

  let handle: string
  try {
    const resp = await fetcher('/api/config', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(next),
    })
    if (resp.status !== 202) {
      const body = await resp.text().catch(() => '')
      return { ok: false, state: 'post-failed', detail: `POST ${resp.status}: ${body}` }
    }
    handle = ((await resp.json()) as { handle: string }).handle
  } catch (e) {
    return { ok: false, state: 'post-failed', detail: e instanceof Error ? e.message : String(e) }
  }

  for (let i = 0; i < maxPolls; i++) {
    try {
      const resp = await fetcher(`/api/reload/${encodeURIComponent(handle)}`, { cache: 'no-store' })
      if (resp.ok) {
        const state = ((await resp.json()) as { state: string }).state
        if (TERMINAL.has(state)) return { ok: state === 'succeeded', state }
      } else if (resp.status === 404) {
        // A stable 404 is the handle being unknown/expired — not the
        // transient mid-cutover error F4-retry is for; short-circuit rather
        // than burn the whole poll budget (review finding).
        return { ok: false, state: 'unknown-handle', detail: 'reload handle not found' }
      }
    } catch {
      // Transient: the admin server is restarting mid-cutover; the handle
      // survives (F4). Keep polling.
    }
    if (pollIntervalMs > 0) await sleep(pollIntervalMs)
  }
  return { ok: false, state: 'timeout', detail: 'reload did not reach a terminal state' }
}

/** Fetch the current core config (Canales detail: the route lives here, not
 * in /api/channels). Null on any failure — the caller degrades. */
export async function fetchConfig(fetcher: typeof fetch = fetch): Promise<CoreConfig | null> {
  try {
    const resp = await fetcher('/api/config', { cache: 'no-store' })
    if (!resp.ok) return null
    return (await resp.json()) as CoreConfig
  } catch {
    return null
  }
}
