// The live-feed store (FR-WIN-4, ADR-0024 metadata-only): one EventSource on
// /api/events through the SP4 proxy, opened while the core runs, closed when
// it stops, retried while running. Everything derived here is WINDOW-SCOPED
// ("desde que se abrió la ventana") and lossy under backpressure — the
// consumers label it that way, never as an all-time total.
import { useSyncExternalStore } from 'react'
import { notifyFailureFrame } from '../incident/store'
import { getCoreState, subscribeCore } from '../status/store'
import { parseFrame, type FeedFrame } from './frame'

export const MAX_FRAMES = 250
const MAX_REPLY_TIMES = 1000
const RETRY_MS = 3000
/** The sparkline's trailing window (design: 14 bars = 14 minutes). */
export const SPARK_MINUTES = 14

export interface FeedCounters {
  received: number
  replied: number
  dropped: number
  failed: number
}

export interface FeedState {
  /** Newest first, capped at MAX_FRAMES. */
  frames: FeedFrame[]
  counters: FeedCounters
  /** Channel names seen on the wire, in first-seen order. */
  channels: string[]
  /** True while the SSE stream is open. */
  live: boolean
  /** Epoch ms of the last reply_sent frame (the "último hace Xs" caption). */
  lastReplyAt: number | null
}

let state: FeedState = emptyState()
let replyTimes: number[] = []
let nextSeq = 1
const listeners = new Set<() => void>()

function emptyState(): FeedState {
  return {
    frames: [],
    counters: { received: 0, replied: 0, dropped: 0, failed: 0 },
    channels: [],
    live: false,
    lastReplyAt: null,
  }
}

function emit(): void {
  for (const l of listeners) l()
}

/** Ingest one SSE data payload (exported for tests). */
export function ingestFrame(data: string): void {
  const f = parseFrame(data)
  if (f === null) return
  f.seq = nextSeq++ // stable identity for React keys (a prepend must not remount the list)
  const counters = { ...state.counters }
  switch (f.type) {
    case 'message_received':
      counters.received++
      break
    case 'reply_sent':
      counters.replied++
      break
    case 'message_dropped':
      counters.dropped++
      break
    case 'handle_failed':
      counters.failed++
      break
    default:
      return // an unknown frame type is not painted, not counted
  }
  let lastReplyAt = state.lastReplyAt
  if (f.type === 'reply_sent') {
    const t = Date.parse(f.timestamp)
    if (!Number.isNaN(t)) {
      lastReplyAt = t
      replyTimes.push(t)
      // Hysteresis: prune only at 2x the cap so steady state is amortized
      // O(1), not an O(cap) copy per reply (review finding).
      if (replyTimes.length > 2 * MAX_REPLY_TIMES) replyTimes = replyTimes.slice(-MAX_REPLY_TIMES)
    }
  }
  const channels =
    f.channel !== undefined && f.channel !== '' && !state.channels.includes(f.channel)
      ? [...state.channels, f.channel]
      : state.channels
  state = {
    frames: [f, ...state.frames].slice(0, MAX_FRAMES),
    counters,
    channels,
    live: state.live,
    lastReplyAt,
  }
  notifyFailureFrame(f)
  emit()
}

/** Replies per minute over the trailing `minutes`, oldest → newest. */
export function minuteSeries(nowMs: number, minutes: number): number[] {
  const out = new Array<number>(minutes).fill(0)
  const curMinute = Math.floor(nowMs / 60_000)
  for (const t of replyTimes) {
    const idx = minutes - 1 - (curMinute - Math.floor(t / 60_000))
    if (idx >= 0 && idx < minutes) out[idx] = (out[idx] ?? 0) + 1
  }
  return out
}

// ---- EventSource lifecycle (injectable for tests / absent in jsdom) ----

type ESFactory = (url: string) => EventSource

let es: EventSource | null = null
let retryTimer: ReturnType<typeof setTimeout> | undefined
let factory: ESFactory | null = null
let started = false
let unsubscribeCore: (() => void) | null = null

function setLive(live: boolean): void {
  if (state.live === live) return
  state = { ...state, live }
  emit()
}

function open(): void {
  if (es !== null || factory === null) return
  const s = factory('/api/events')
  es = s
  s.onopen = () => setLive(true)
  s.onmessage = (ev) => ingestFrame(String(ev.data))
  s.onerror = () => {
    // The stream died (core stopping, proxy cycling). Close and retry while
    // the core still reads as running; a stopped core reconnects on the
    // next running transition instead.
    s.close()
    if (es === s) es = null
    setLive(false)
    if (getCoreState() === 'running' && retryTimer === undefined) {
      retryTimer = setTimeout(() => {
        retryTimer = undefined
        if (getCoreState() === 'running') open()
      }, RETRY_MS)
    }
  }
}

function close(): void {
  if (retryTimer !== undefined) {
    clearTimeout(retryTimer)
    retryTimer = undefined
  }
  if (es !== null) {
    es.close()
    es = null
  }
  setLive(false)
}

function reconcile(): void {
  if (getCoreState() === 'running') open()
  else close()
}

/** Wire the feed to the core-state store (idempotent; called at boot). */
export function startFeed(f?: ESFactory): void {
  if (started) return
  started = true
  factory = f ?? (typeof EventSource !== 'undefined' ? (url: string) => new EventSource(url) : null)
  unsubscribeCore = subscribeCore(reconcile)
  reconcile()
}

export function getFeed(): FeedState {
  return state
}

export function resetFeedForTests(): void {
  close()
  // Detach the core-store listener too — otherwise every re-start stacks a
  // stale reconciler on the shared core store (review finding).
  unsubscribeCore?.()
  unsubscribeCore = null
  state = emptyState()
  replyTimes = []
  nextSeq = 1
  factory = null
  started = false
}

function subscribe(l: () => void): () => void {
  listeners.add(l)
  return () => listeners.delete(l)
}

/** React hook over the store. */
export function useFeed(): FeedState {
  return useSyncExternalStore(subscribe, () => state)
}
