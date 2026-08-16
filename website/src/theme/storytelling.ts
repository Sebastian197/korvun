type StorytellingRuntime = {
  IntersectionObserver?: typeof IntersectionObserver
  matchMedia: (query: string) => MediaQueryList
}

const STAGGER_MS = 70
const MAX_STAGGER_MS = 420

export function armStorytelling(
  root: Document = document,
  runtime: StorytellingRuntime = window,
): () => void {
  // k-motion on <html> is a diagnostic marker only. Docusaurus rewrites the
  // <html> class attribute after hydration, so nothing may depend on it —
  // the reveal CSS gates on .k-reveal, which this controller alone assigns.
  root.documentElement.classList.remove('k-motion')

  if (!root.querySelector('[data-k-section="hero"]')) return () => undefined
  if (typeof runtime.IntersectionObserver !== 'function') return () => undefined
  if (!runtime.matchMedia('(prefers-reduced-motion: no-preference)').matches) {
    return () => undefined
  }

  const targets: HTMLElement[] = []
  const observer = new runtime.IntersectionObserver(
    (entries, currentObserver) => {
      for (const entry of entries) {
        if (!entry.isIntersecting) continue
        entry.target.classList.add('k-in')
        currentObserver.unobserve(entry.target)
      }
    },
    { threshold: 0.15, rootMargin: '0px 0px -8% 0px' },
  )

  root.documentElement.classList.add('k-motion')
  root.querySelectorAll<HTMLElement>('[data-k-section]').forEach((section) => {
    if (section.getAttribute('data-k-section') === 'hero') return

    section.querySelectorAll<HTMLElement>('[data-motion]').forEach((target, index) => {
      target.style.setProperty(
        '--k-motion-delay',
        `${Math.min(index * STAGGER_MS, MAX_STAGGER_MS)}ms`,
      )
      target.classList.add('k-reveal')
      targets.push(target)
      observer.observe(target)
    })
  })

  return () => {
    observer.disconnect()
    root.documentElement.classList.remove('k-motion')
    targets.forEach((target) => {
      target.classList.remove('k-reveal', 'k-in')
      target.style.removeProperty('--k-motion-delay')
    })
  }
}
