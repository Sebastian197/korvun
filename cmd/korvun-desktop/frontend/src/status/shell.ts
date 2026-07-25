// The shell-status store (SP6b): every 2 s the chrome mirrors
// Desktop.Status() through the bindings — Running, ConfigPath, the effective
// AdminAddr and the admin-token env-var NAME. Outside the window (plain
// browser, bindings absent) the status is null and every consumer degrades
// honestly. Poll noise never blanks a previously good answer.
import { useSyncExternalStore } from 'react'
import { desktop, type ShellStatus } from '../lib/go'

export const SHELL_POLL_INTERVAL_MS = 2000

let current: ShellStatus | null = null
const listeners = new Set<() => void>()

function set(next: ShellStatus): void {
  const c = current
  if (
    c !== null &&
    c.Running === next.Running &&
    c.ConfigPath === next.ConfigPath &&
    c.AdminAddr === next.AdminAddr &&
    c.TokenEnv === next.TokenEnv
  ) {
    return
  }
  current = next
  for (const l of listeners) l()
}

/** One poll tick. Exported for tests; a rejection keeps the last value. */
export async function pollShellOnce(): Promise<void> {
  const d = desktop()
  if (!d) return
  try {
    set(await d.Status())
  } catch {
    // Binding noise (e.g. a timed-out read) — the next tick reconciles.
  }
}

let timer: ReturnType<typeof setInterval> | undefined

/** Start the polling loop (idempotent; a no-op without bindings). */
export function startShellPolling(): void {
  if (timer !== undefined) return
  void pollShellOnce()
  timer = setInterval(() => void pollShellOnce(), SHELL_POLL_INTERVAL_MS)
}

export function getShellStatus(): ShellStatus | null {
  return current
}

export function resetShellForTests(): void {
  current = null
  if (timer !== undefined) {
    clearInterval(timer)
    timer = undefined
  }
}

function subscribe(l: () => void): () => void {
  listeners.add(l)
  return () => listeners.delete(l)
}

/** React hook over the store. */
export function useShellStatus(): ShellStatus | null {
  return useSyncExternalStore(subscribe, () => current)
}
