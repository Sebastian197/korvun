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
  return window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark'
}

export function applyTheme(choice: ThemeChoice): void {
  try {
    localStorage.setItem(KEY, choice)
  } catch {
    // A restricted WebView storage must never blank the app; the theme
    // simply won't persist across launches.
  }
  document.documentElement.dataset.theme = effective(choice)
}
