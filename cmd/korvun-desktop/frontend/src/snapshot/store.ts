// The snapshot store (FR-WIN-6): mirrors the read-only control API while the
// core runs and RETAINS the last answer when it stops — the stopped Home
// paints it dimmed and stale ("métricas atenuadas si hay último dato de la
// sesión"), never as live data.
import { useSyncExternalStore } from 'react'
import { getCoreState, subscribeCore } from '../status/store'

export interface ChannelSummary {
  type: string
  mode: string
  name: string
  dropped?: number
}

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

export interface Snapshot {
  channels: ChannelSummary[] | null
  brains: BrainSummary[] | null
  /** True until a successful poll, and again once the core stops. */
  stale: boolean
}

export const SNAPSHOT_INTERVAL_MS = 5000

let state: Snapshot = { channels: null, brains: null, stale: true }
const listeners = new Set<() => void>()

function set(next: Snapshot): void {
  state = next
  for (const l of listeners) l()
}

async function getJSON<T>(fetcher: typeof fetch, url: string): Promise<T | null> {
  const resp = await fetcher(url, { cache: 'no-store' })
  if (!resp.ok) return null
  return (await resp.json()) as T
}

/** One poll tick (exported for tests): both endpoints, or stale on failure. */
export async function pollSnapshotOnce(fetcher: typeof fetch = fetch): Promise<void> {
  try {
    const [channels, brains] = await Promise.all([
      getJSON<ChannelSummary[]>(fetcher, '/api/channels'),
      getJSON<BrainSummary[]>(fetcher, '/api/brains'),
    ])
    if (channels === null || brains === null) {
      if (!state.stale) set({ ...state, stale: true })
      return
    }
    set({ channels, brains, stale: false })
  } catch {
    if (!state.stale) set({ ...state, stale: true })
  }
}

let timer: ReturnType<typeof setInterval> | undefined
let started = false
let unsubscribeCore: (() => void) | null = null

function reconcile(): void {
  if (getCoreState() === 'running') {
    if (timer === undefined) {
      void pollSnapshotOnce()
      timer = setInterval(() => void pollSnapshotOnce(), SNAPSHOT_INTERVAL_MS)
    }
  } else {
    if (timer !== undefined) {
      clearInterval(timer)
      timer = undefined
    }
    if (!state.stale) set({ ...state, stale: true })
  }
}

/** Wire the snapshot to the core-state store (idempotent; called at boot). */
export function startSnapshot(): void {
  if (started) return
  started = true
  unsubscribeCore = subscribeCore(reconcile)
  reconcile()
}

export function getSnapshot(): Snapshot {
  return state
}

export function resetSnapshotForTests(): void {
  // Detach the core-store listener (same leak class as the feed store).
  unsubscribeCore?.()
  unsubscribeCore = null
  state = { channels: null, brains: null, stale: true }
  if (timer !== undefined) {
    clearInterval(timer)
    timer = undefined
  }
  started = false
}

function subscribe(l: () => void): () => void {
  listeners.add(l)
  return () => listeners.delete(l)
}

/** React hook over the store. */
export function useSnapshot(): Snapshot {
  return useSyncExternalStore(subscribe, () => state)
}
