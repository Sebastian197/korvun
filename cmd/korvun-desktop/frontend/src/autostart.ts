// "Iniciar con la aplicación" (FR-WIN-5): chrome-local state in localStorage
// (korvun.chrome.autostart) meaning auto-START-THE-CORE when the window
// opens. An OS login item is a v1 exclusion (ADR-0035 Consequences §8) and
// stays out.
import { desktop } from './lib/go'

const KEY = 'korvun.chrome.autostart'

export function isAutostartEnabled(): boolean {
  try {
    return localStorage.getItem(KEY) === 'true'
  } catch {
    return false
  }
}

export function setAutostart(enabled: boolean): void {
  try {
    localStorage.setItem(KEY, enabled ? 'true' : 'false')
  } catch {
    // A restricted WebView storage must never blank the app.
  }
}

/** Boot hook: start the core when enabled and bound. Failures (e.g. already
 * running) are swallowed — the status poll reconciles the truth. */
export async function runAutostart(): Promise<void> {
  if (!isAutostartEnabled()) return
  const d = desktop()
  if (!d) return
  try {
    await d.Start()
  } catch {
    // Reconciled by polling.
  }
}