// Client module for the legacy GitHub Pages deployment (ADR-0040 site).
// When the site is served from sebastian197.github.io it forwards the
// visitor to the canonical https://korvun.dev, preserving the path (minus
// the `/korvun` project prefix), the query string, and the fragment.
// On every other host — korvun.dev itself, korvun.pages.dev previews,
// localhost dev/e2e — it does nothing.

const LEGACY_HOST = 'sebastian197.github.io'
const LEGACY_PREFIX = '/korvun'
const CANONICAL_ORIGIN = 'https://korvun.dev'

export function legacyRedirectTarget(href: string): string | null {
  let url: URL
  try {
    url = new URL(href)
  } catch {
    return null
  }
  if (url.hostname !== LEGACY_HOST) {
    return null
  }
  let path = url.pathname
  if (path === LEGACY_PREFIX) {
    path = '/'
  } else if (path.startsWith(`${LEGACY_PREFIX}/`)) {
    path = path.slice(LEGACY_PREFIX.length)
  }
  return `${CANONICAL_ORIGIN}${path}${url.search}${url.hash}`
}

// Side effect on load in the browser only; client modules are also pulled
// into the SSR bundle, where `window` does not exist.
if (typeof window !== 'undefined') {
  const target = legacyRedirectTarget(window.location.href)
  if (target !== null) {
    // `replace` keeps the legacy URL out of the history stack so the back
    // button does not bounce the visitor between the two hosts.
    window.location.replace(target)
  }
}
