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

/** How long an unanswered poll may stay unanswered before the header stops
 * calling itself live.
 *
 * `pollOnce` opens with `if (polling) return current`, so a wedged proxy makes
 * every later tick a no-op: the state and `lastOkAt` both freeze and the badge
 * would go on printing «en vivo» over a measurement that stopped moving. Two
 * missed ticks is the smallest window that cannot be a slow-but-healthy
 * answer. */
export const POLL_STALE_AFTER_MS = POLL_INTERVAL_MS * 2

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
  armStaleWatchdog()
  try {
    const startedAt = Date.now()
    lastAskedAt = startedAt
    const resp = await fetcher('/healthz', { cache: 'no-store' })
    if (resp.ok) {
      const changed = current !== 'running'
      lastOkAt = Date.now()
      // The round trip, MEASURED. Nothing timed this before, so «hace X s» was
      // the only number the header had — and polling every 2 s it could only
      // ever read 0, 1 or 2. True, and carrying no information.
      lastRoundTripMs = lastOkAt - startedAt
      set('running')
      if (!changed) {
        // lastOkAt moved though the state did not — one notify so the
        // header's "hace Xs" ticks (set() already notified on a change;
        // never both — review finding).
        for (const l of listeners) l()
      }
      return current
    }
    if (resp.status === 503) {
      const body = (await resp.json().catch(() => null)) as {
        error?: string
      } | null
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
    disarmStaleWatchdog()
  }
  return current
}

let staleTimer: ReturnType<typeof setTimeout> | undefined

/** Wakes the subscribers once the in-flight poll has been unanswered longer
 * than POLL_STALE_AFTER_MS.
 *
 * Without it `isPollStale()` is a fact nobody is ever told. `pollOnce` opens
 * with `if (polling) return current`, so a wedged proxy makes every later tick
 * a no-op: no state changes, no notify fires, and a component that read
 * "healthy" at mount goes on painting it forever. The header measured exactly
 * that during the internal pass — «en vivo» unchanged a full minute past the
 * window, over a request that never answered.
 *
 * So staleness gets its own wake-up. One timer, re-armed per poll and cleared
 * when the poll settles, which is the only moment the answer can change
 * without anything else notifying. */
function armStaleWatchdog(): void {
  disarmStaleWatchdog()
  staleTimer = setTimeout(() => {
    staleTimer = undefined
    // Re-checked rather than assumed: a poll that settled in the meantime
    // already notified, and waking the tree twice for one fact is noise.
    if (polling) for (const l of listeners) l()
  }, POLL_STALE_AFTER_MS + 1)
}

function disarmStaleWatchdog(): void {
  if (staleTimer !== undefined) {
    clearTimeout(staleTimer)
    staleTimer = undefined
  }
}

let lastRoundTripMs: number | null = null
let lastAskedAt: number | null = null

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

/** Milliseconds the last healthy answer took, or null before the first. */
export function getLastRoundTripMs(): number | null {
  return lastRoundTripMs
}

/** True when a poll was asked and nothing has answered for longer than
 * POLL_STALE_AFTER_MS. The header must not call itself live over this. */
export function isPollStale(now: number = Date.now()): boolean {
  if (!polling || lastAskedAt === null) return false
  return now - lastAskedAt > POLL_STALE_AFTER_MS
}

/** Drops every module-level fact back to boot. Tests only: this store is a
 * singleton, so without it one test's answer leaks into the next. */
export function resetCoreForTests(): void {
  current = 'unknown'
  lastOkAt = null
  lastRoundTripMs = null
  lastAskedAt = null
  polling = false
  disarmStaleWatchdog()
}

/** Epoch ms of the last healthy /healthz answer (null before the first). */
export function getLastOkAt(): number | null {
  return lastOkAt
}

/** Subscribe a non-React consumer; returns the unsubscribe. */
export function subscribeCore(l: () => void): () => void {
  return subscribe(l)
}

/** React hook over the staleness of the in-flight poll.
 *
 * It subscribes, which is the whole point: `isPollStale()` called during render
 * gives a component the answer at THAT moment and nothing brings it back when
 * the answer changes under a wedged poll. */
export function usePollStale(): boolean {
  return useSyncExternalStore(subscribe, () => isPollStale())
}

/** React hook over the store. */
export function useCoreState(): CoreState {
  return useSyncExternalStore(subscribe, () => current)
}

/** React hook over the last healthy poll timestamp. */
export function useLastRoundTripMs(): number | null {
  return useSyncExternalStore(subscribe, () => lastRoundTripMs)
}

export function useLastOkAt(): number | null {
  return useSyncExternalStore(subscribe, () => lastOkAt)
}
