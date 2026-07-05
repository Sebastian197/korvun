// The reload state machine (ADR-0030 §5) — the face of Phase 2a's reload-and-rebuild.
// Pure transitions (reloadReducer) + a dependency-injected poll loop (pollReload), so
// every property is Vitest-testable without a network or a DOM.

// Server-reported states, VERBATIM from supervisor.State (ADR-0027 §F4). The UI never
// invents a state: an unrecognized string is surfaced as `unknown`, not mapped to a
// valid one by accident.
export const SERVER_STATES = [
  'pending',
  'cutover-in-progress',
  'succeeded',
  'rolled-back',
  'failed',
] as const
export type ServerState = (typeof SERVER_STATES)[number]

/** Map a raw server string to a known state, or null if unrecognized (never invent). */
export function mapServerState(s: string): ServerState | null {
  return (SERVER_STATES as readonly string[]).includes(s) ? (s as ServerState) : null
}

export type ReloadStatus =
  | { phase: 'idle' }
  | { phase: 'polling'; handle: string; server: ServerState }
  | { phase: 'succeeded'; handle: string }
  | { phase: 'rolledBack'; handle: string }
  | { phase: 'failed'; handle: string }
  | { phase: 'unknown'; handle: string } // total-timeout OR an unrecognized server string

export type ReloadEvent =
  | { kind: 'started'; handle: string }
  | { kind: 'serverState'; state: string }
  | { kind: 'netError' } // transient (ECONNREFUSED during the cutover restart)
  | { kind: 'timeout' }
  | { kind: 'reset' }

export function reloadReducer(s: ReloadStatus, e: ReloadEvent): ReloadStatus {
  switch (e.kind) {
    case 'reset':
      return { phase: 'idle' }
    case 'started':
      return { phase: 'polling', handle: e.handle, server: 'pending' }
    case 'netError':
      // CRITICAL: the admin server restarts DURING cutover-in-progress, so the poll
      // gets connection-refused. The handle survives in the supervisor (F4), so this
      // is EXPECTED — keep polling. NEVER map a transient net error to `failed`.
      return s
    case 'timeout':
      return s.phase === 'polling' ? { phase: 'unknown', handle: s.handle } : s
    case 'serverState': {
      if (s.phase !== 'polling') return s
      const mapped = mapServerState(e.state)
      if (mapped === null) return { phase: 'unknown', handle: s.handle } // do NOT invent
      switch (mapped) {
        case 'pending':
        case 'cutover-in-progress':
          return { phase: 'polling', handle: s.handle, server: mapped }
        case 'succeeded':
          return { phase: 'succeeded', handle: s.handle }
        case 'rolled-back':
          return { phase: 'rolledBack', handle: s.handle }
        case 'failed':
          return { phase: 'failed', handle: s.handle }
      }
    }
  }
}

export function isTerminal(s: ReloadStatus): boolean {
  return s.phase === 'succeeded' || s.phase === 'rolledBack' || s.phase === 'failed' || s.phase === 'unknown'
}

// Poll timing — NAMED in one place (ADR-0030 §5). Gentle exponential backoff, capped,
// with a generous total budget after which the status is `unknown` ("refresh").
export const POLL = {
  initialMs: 500,
  maxMs: 3000,
  factor: 1.6,
  totalMs: 120_000,
} as const

/** Backoff interval for the nth poll attempt (0-based), capped at POLL.maxMs. */
export function nextInterval(attempt: number): number {
  return Math.min(POLL.maxMs, Math.round(POLL.initialMs * POLL.factor ** attempt))
}

export interface PollDeps {
  getStatus: (handle: string, token: string) => Promise<string>
  sleep: (ms: number) => Promise<void>
  now: () => number
}

/** Drive the reload machine to a terminal state, calling `onState` on every change.
 *  A getStatus rejection is a TRANSIENT net error → retry (never failed); the total
 *  budget bounds it into `unknown`. */
export async function pollReload(
  handle: string,
  token: string,
  deps: PollDeps,
  onState: (s: ReloadStatus) => void,
): Promise<ReloadStatus> {
  const start = deps.now()
  let status: ReloadStatus = reloadReducer({ phase: 'idle' }, { kind: 'started', handle })
  onState(status)
  let attempt = 0
  while (!isTerminal(status)) {
    await deps.sleep(nextInterval(attempt))
    if (deps.now() - start >= POLL.totalMs) {
      status = reloadReducer(status, { kind: 'timeout' })
      onState(status)
      break
    }
    try {
      const state = await deps.getStatus(handle, token)
      status = reloadReducer(status, { kind: 'serverState', state })
    } catch {
      status = reloadReducer(status, { kind: 'netError' })
    }
    onState(status)
    attempt++
  }
  return status
}
