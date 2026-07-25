// The incident store (FR-WIN-6 `incidencia`, FR-WIN-4's honest triggers):
//  - reap-shaped: the core left 'running' with NO UI-initiated Stop. The
//    exit reason is log-only today, so the chrome says "stopped
//    unexpectedly" and NEVER invents a cause.
//  - feed-shaped: a message_dropped / handle_failed frame, carrying the
//    frame's REAL channel.
// A clean Start clears it (AS-6): any transition into 'running' resets.
import { useSyncExternalStore } from 'react'
import type { FeedFrame } from '../feed/frame'
import type { CoreState } from '../status/store'

export type Incident =
  | { kind: 'reap'; at: string }
  | {
      kind: 'feed'
      frameType: 'message_dropped' | 'handle_failed'
      channel: string
      at: string
    }

let current: Incident | null = null
let userStopPending = false
const listeners = new Set<() => void>()

function set(next: Incident | null): void {
  if (next === current) return
  current = next
  for (const l of listeners) l()
}

/** Home's Detener calls this right before DISPATCHING Stop — the flip that
 * follows is expected. Only a dispatched Stop may mark intent (a no-op click
 * must never arm the flag — review finding). */
export function markUserStop(): void {
  userStopPending = true
}

/** A Stop that never landed (binding rejection) must not leave the flag
 * armed — the next real unexpected exit would be swallowed as intended. */
export function clearUserStop(): void {
  userStopPending = false
}

/** Core-state transition sink (the status store drives it). */
export function notifyCoreTransition(prev: CoreState, next: CoreState): void {
  if (next === 'running') {
    userStopPending = false
    set(null)
    return
  }
  // Only the DEFINITIVE stop reading raises the reap incident: 'unknown'
  // (transport blip) and 'unreachable' (mid-cutover) are transient states a
  // poll later self-heals — a false red banner would be an invented cause
  // (review finding).
  if (prev !== 'running' || next !== 'stopped') return
  if (userStopPending) {
    userStopPending = false
    return
  }
  set({ kind: 'reap', at: new Date().toISOString() })
}

/** Failure-frame sink (the feed store drives it). */
export function notifyFailureFrame(frame: FeedFrame): void {
  if (frame.type !== 'message_dropped' && frame.type !== 'handle_failed') return
  set({
    kind: 'feed',
    frameType: frame.type,
    channel: frame.channel ?? '',
    at: frame.timestamp,
  })
}

export function getIncident(): Incident | null {
  return current
}

export function resetIncidentForTests(): void {
  current = null
  userStopPending = false
}

function subscribe(l: () => void): () => void {
  listeners.add(l)
  return () => listeners.delete(l)
}

/** React hook over the store. */
export function useIncident(): Incident | null {
  return useSyncExternalStore(subscribe, () => current)
}
