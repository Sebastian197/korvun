// Chrome theme persistence (SP6 spec FR-WIN-5): localStorage-only state of
// the chrome, never config. 'system' follows prefers-color-scheme.
export type ThemeChoice = 'dark' | 'light' | 'system'

const KEY = 'korvun.chrome.theme'

export function storedTheme(): ThemeChoice {
  try {
    const v = localStorage.getItem(KEY)
    return v === 'light' || v === 'system' ? v : 'dark'
  } catch {
    return 'dark'
  }
}

function effective(choice: ThemeChoice): 'dark' | 'light' {
  if (choice !== 'system') return choice
  if (typeof window.matchMedia !== 'function') return 'dark'
  return window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark'
}

// While the choice is 'system' the chrome FOLLOWS the OS live (6a review
// rider d): one change listener on prefers-color-scheme, detached the moment
// an explicit choice takes over.
let systemMql: MediaQueryList | null = null
let systemListener: (() => void) | null = null

function detachSystemListener(): void {
  if (systemMql && systemListener) {
    systemMql.removeEventListener('change', systemListener)
  }
  systemMql = null
  systemListener = null
}

export function applyTheme(choice: ThemeChoice): void {
  try {
    localStorage.setItem(KEY, choice)
  } catch {
    // A restricted WebView storage must never blank the app; the theme
    // simply won't persist across launches.
  }
  detachSystemListener()
  if (choice === 'system' && typeof window.matchMedia === 'function') {
    systemMql = window.matchMedia('(prefers-color-scheme: light)')
    systemListener = () => {
      document.documentElement.dataset.theme = effective('system')
    }
    systemMql.addEventListener('change', systemListener)
  }
  document.documentElement.dataset.theme = effective(choice)
}
