// The Status polling store (SP6 spec FR-WIN-6): every 2 s the chrome asks
// /healthz THROUGH the SP4 proxy and reduces the answer to one honest state.
// The 503 two-body contract is the degradation the stopped/incident chrome
// paints; a transport error means even the shell proxy is gone (harness torn
// down) and reads as 'unknown'. No Wails events in v1 — polling reconciles
// everything, including a core that died on its own.
import { useSyncExternalStore } from 'react'
import { notifyCoreTransition } from '../incident/store'

export type CoreState = 'running' | 'stopped' | 'unreachable' | 'unknown'

export const POLL_INTERVAL_MS = 2000

let current: CoreState = 'unknown'
/** Epoch ms of the last healthy /healthz answer (the header's "hace Xs"). */
let lastOkAt: number | null = null
const listeners = new Set<() => void>()

function set(next: CoreState): void {
  if (next === current) return
  const prev = current
  current = next
  notifyCoreTransition(prev, next)
  for (const l of listeners) l()
}

let polling = false

/** One poll tick: classify /healthz through the proxy. Exported for tests.
 * Overlap-guarded: a tick that finds the previous one still in flight
 * (wedged proxy) skips, so a stale response can never overwrite a newer
 * state. Both 503 bodies are matched EXACTLY; anything else is unknown. */
export async function pollOnce(fetcher: typeof fetch = fetch): Promise<CoreState> {
  if (polling) return current
  polling = true
  try {
    const resp = await fetcher('/healthz', { cache: 'no-store' })
    if (resp.ok) {
      lastOkAt = Date.now()
      set('running')
      // lastOkAt moved even when the state did not — the header caption
      // ("hace Xs") re-renders on every healthy tick.
      for (const l of listeners) l()
      return current
    }
    if (resp.status === 503) {
      const body = (await resp.json().catch(() => null)) as { error?: string } | null
      if (body?.error === 'core stopped') set('stopped')
      else if (body?.error === 'core unreachable') set('unreachable')
      else set('unknown')
      return current
    }
    set('unknown')
  } catch {
    set('unknown')
  } finally {
    polling = false
  }
  return current
}

let timer: ReturnType<typeof setInterval> | undefined

/** Start the 2 s polling loop (idempotent). */
export function startPolling(): void {
  if (timer !== undefined) return
  void pollOnce()
  timer = setInterval(() => void pollOnce(), POLL_INTERVAL_MS)
}

export function stopPolling(): void {
  if (timer !== undefined) {
    clearInterval(timer)
    timer = undefined
  }
}

function subscribe(l: () => void): () => void {
  listeners.add(l)
  return () => listeners.delete(l)
}

/** Current state, for non-React consumers (feed/snapshot lifecycles). */
export function getCoreState(): CoreState {
  return current
}

/** Epoch ms of the last healthy /healthz answer (null before the first). */
export function getLastOkAt(): number | null {
  return lastOkAt
}

/** Subscribe a non-React consumer; returns the unsubscribe. */
export function subscribeCore(l: () => void): () => void {
  return subscribe(l)
}

/** React hook over the store. */
export function useCoreState(): CoreState {
  return useSyncExternalStore(subscribe, () => current)
}

/** React hook over the last healthy poll timestamp. */
export function useLastOkAt(): number | null {
  return useSyncExternalStore(subscribe, () => lastOkAt)
}
