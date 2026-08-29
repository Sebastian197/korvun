// Brand-motion runtime (brand-motion spec, Tasks 2–3): the masthead runs
// only while visible and motion is allowed, and the scroll journey drives
// route state from cached geometry inside one coalesced animation frame.
// Additive to armStorytelling — never touches the reveal-once contract.

type BrandMotionRuntime = {
  IntersectionObserver?: typeof IntersectionObserver
  matchMedia: (query: string) => Pick<MediaQueryList, 'matches'>
}

type BrandMotionRoot = Pick<Document, 'querySelector'>

/** Arm the masthead visibility gate: `data-k-running` follows viewport
 * intersection; reduced motion never observes (the CSS static frame is the
 * complete experience); cleanup removes the attribute. */
export function armBrandMotion(
  root: BrandMotionRoot = document,
  runtime: BrandMotionRuntime = window,
): () => void {
  const masthead = root.querySelector<HTMLElement>('[data-k-masthead]')
  if (!masthead) return () => undefined
  if (!runtime.matchMedia('(prefers-reduced-motion: no-preference)').matches) {
    return () => masthead.removeAttribute('data-k-running')
  }
  if (typeof runtime.IntersectionObserver !== 'function') return () => undefined
  const observer = new runtime.IntersectionObserver(
    ([entry]) => {
      masthead.setAttribute('data-k-running', entry?.isIntersecting ? 'true' : 'false')
    },
    { threshold: 0.1 },
  )
  observer.observe(masthead)
  return () => {
    observer.disconnect()
    masthead.removeAttribute('data-k-running')
  }
}

// ---- Scroll routing journey (Task 3) --------------------------------------

type RouteState = 'pending' | 'active' | 'complete'

/** Pure state fan-out around the active index. */
export function routeStates(activeIndex: number, count: number): RouteState[] {
  return Array.from({ length: count }, (_, index) =>
    index < activeIndex ? 'complete' : index === activeIndex ? 'active' : 'pending',
  )
}

type JourneyRuntime = {
  matchMedia: (query: string) => Pick<MediaQueryList, 'matches'>
  scrollY: number
  innerHeight: number
  requestAnimationFrame: (job: () => void) => number
  cancelAnimationFrame: (handle: number) => void
  addEventListener: (name: string, fn: () => void, opts?: AddEventListenerOptions) => void
  removeEventListener: (name: string, fn: () => void) => void
  ResizeObserver?: typeof ResizeObserver
}

type JourneyRoot = Pick<Document, 'querySelector' | 'querySelectorAll'>

/** Arm the scroll journey: port geometry is measured at init and on resize
 * ONLY; the passive scroll handler schedules at most one animation frame;
 * the frame reads nothing but the scroll position and writes progress,
 * section states, and the signal transform. Reduced motion renders the
 * complete static circuit (data-k-static) with no listeners at all.
 * Cleanup removes every listener, observer, frame, attribute and property. */
export function armRoutingJourney(
  root: JourneyRoot = document,
  runtime: JourneyRuntime = window,
): () => void {
  const journey = root.querySelector<HTMLElement>('[data-k-journey]')
  if (!journey) return () => undefined
  const sections = Array.from(root.querySelectorAll<HTMLElement>('[data-k-section]'))
  if (sections.length === 0) return () => undefined

  if (!runtime.matchMedia('(prefers-reduced-motion: no-preference)').matches) {
    journey.setAttribute('data-k-static', 'true')
    return () => journey.removeAttribute('data-k-static')
  }

  const signal = root.querySelector<HTMLElement>('[data-k-route-signal]')
  let centers: number[] = []
  let span = 1

  const measure = () => {
    centers = sections.map((section) => {
      const port = section.querySelector<HTMLElement>('[data-k-route-port]')
      if (!port) return 0
      const rect = port.getBoundingClientRect()
      return rect.top + rect.height / 2 + runtime.scrollY
    })
    span = Math.max(1, (centers[centers.length - 1] ?? 1) - (centers[0] ?? 0))
  }
  measure()

  let frame: number | null = null
  const render = () => {
    frame = null
    const pos = runtime.scrollY + runtime.innerHeight / 2
    const progress = Math.min(1, Math.max(0, (pos - (centers[0] ?? 0)) / span))
    journey.style.setProperty('--k-route-progress', String(Math.round(progress * 1000) / 1000))
    let active = 0
    for (let index = 0; index < centers.length; index++) {
      if ((centers[index] ?? Number.POSITIVE_INFINITY) <= pos) active = index
    }
    const states = routeStates(active, sections.length)
    sections.forEach((section, index) => {
      section.setAttribute('data-k-route-state', states[index] ?? 'pending')
    })
    if (signal) {
      signal.style.setProperty('transform', `translateY(${Math.round(progress * span)}px)`)
    }
  }

  const onScroll = () => {
    if (frame !== null) return
    frame = runtime.requestAnimationFrame(render)
  }
  runtime.addEventListener('scroll', onScroll, { passive: true })

  let resizeObserver: ResizeObserver | null = null
  const onResize = () => {
    measure()
    onScroll()
  }
  if (typeof runtime.ResizeObserver === 'function') {
    resizeObserver = new runtime.ResizeObserver(onResize)
    resizeObserver.observe(journey)
  } else {
    runtime.addEventListener('resize', onResize)
  }

  render()

  return () => {
    if (frame !== null) runtime.cancelAnimationFrame(frame)
    frame = null
    runtime.removeEventListener('scroll', onScroll)
    if (resizeObserver) resizeObserver.disconnect()
    else runtime.removeEventListener('resize', onResize)
    journey.style.removeProperty('--k-route-progress')
    signal?.style.removeProperty('transform')
    sections.forEach((section) => section.removeAttribute('data-k-route-state'))
  }
}
