// Scroll-storytelling arming (SP2b).
//
// THE LAW (spec AS-4b/4c): this module is the ONLY thing that ever enables
// hiding. Both `html.k-motion` and the per-element `.k-reveal` class are
// added HERE, and only when IntersectionObserver exists AND the user
// welcomes motion. No JS, an old browser, or prefers-reduced-motion ⇒
// neither class exists ⇒ the stylesheet never hides anything: the page is
// complete from the first pixel and the choreography is pure enhancement.

const REVEAL_TARGETS = ['.VPFeatures .item', '.k-privacy', '.k-clip']

export function armStorytelling(): void {
  if (typeof window === 'undefined') return
  if (!('IntersectionObserver' in window)) return
  if (!window.matchMedia('(prefers-reduced-motion: no-preference)').matches) {
    return
  }
  const home = document.querySelector('.VPHome')
  if (!home) {
    // Not the landing: keep the document unarmed so nothing on docs pages
    // can ever match a hiding rule.
    document.documentElement.classList.remove('k-motion')
    return
  }

  const io = new IntersectionObserver(
    (entries, observer) => {
      for (const entry of entries) {
        if (entry.isIntersecting) {
          entry.target.classList.add('k-in')
          observer.unobserve(entry.target) // reveal ONCE — never re-hide
        }
      }
    },
    { threshold: 0.15, rootMargin: '0px 0px -8% 0px' },
  )

  document.documentElement.classList.add('k-motion')
  for (const sel of REVEAL_TARGETS) {
    // Query from document, not from .VPHome: the landing's markdown body
    // (the privacy scene, the clip) renders in a SIBLING vp-doc container,
    // not inside VPHome — scoping to home silently skipped it, leaving its
    // choreographed children hidden forever (the harness caught it).
    document.querySelectorAll(sel).forEach((el, i) => {
      if (el.classList.contains('k-in')) return
      // Short stagger within a group (the pillars), capped — subtle, not
      // a parade.
      ;(el as HTMLElement).style.setProperty(
        '--k-d',
        `${Math.min(i * 70, 420)}ms`,
      )
      el.classList.add('k-reveal')
      io.observe(el)
    })
  }
}