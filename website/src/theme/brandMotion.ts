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
  innerWidth?: number
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
  let railX = 40
  let firstY = 0
  let docHeight = Number.POSITIVE_INFINITY

  // Geometry is written ONCE per measure (init + resize): the rail-and-elbows
  // path d, the journey mode, and the SVG viewBox — never inside a frame.
  const measure = () => {
    const journeyRect = journey.getBoundingClientRect?.()
    const baseTop = (journeyRect?.top ?? 0) + runtime.scrollY
    const baseLeft = journeyRect?.left ?? 0
    docHeight = journeyRect ? baseTop + journeyRect.height : Number.POSITIVE_INFINITY
    const mobile = (runtime.innerWidth ?? 1200) < 996
    railX = mobile ? 14 : 40
    journey.setAttribute('data-k-journey-mode', mobile ? 'mobile' : 'desktop')
    const points: Array<{ x: number; y: number }> = []
    centers = sections.map((section) => {
      const port = section.querySelector<HTMLElement>('[data-k-route-port]')
      if (!port) return 0
      const rect = port.getBoundingClientRect()
      const pageY = rect.top + rect.height / 2 + runtime.scrollY
      points.push({ x: rect.left + rect.width / 2 - baseLeft, y: pageY - baseTop })
      return pageY
    })
    span = Math.max(1, (centers[centers.length - 1] ?? 1) - (centers[0] ?? 0))
    firstY = (points[0]?.y ?? 0)
    const base = journey.querySelector?.('[data-k-route-path-base]')
    const active = journey.querySelector?.('[data-k-route-path-active]')
    if (base && active && points.length > 0 && journeyRect) {
      let d = ''
      let previousY = Math.max(0, (points[0]?.y ?? 0) - 60)
      for (const point of points) {
        d += `M${railX} ${previousY}V${point.y}H${point.x}`
        previousY = point.y
      }
      base.setAttribute('d', d)
      active.setAttribute('d', d)
      const svg = journey.querySelector?.('svg')
      svg?.setAttribute('viewBox', `0 0 ${Math.max(1, journeyRect.width)} ${Math.max(1, journeyRect.height)}`)
    }
  }
  measure()

  let frame: number | null = null
  const render = () => {
    frame = null
    // The document's bottom IS the journey's end: the last port can sit
    // below the half-viewport line when scrolling maxes out.
    const atBottom = runtime.scrollY + runtime.innerHeight >= docHeight - 2
    const pos = atBottom ? Number.POSITIVE_INFINITY : runtime.scrollY + runtime.innerHeight / 2
    const progress = atBottom
      ? 1
      : Math.min(1, Math.max(0, (pos - (centers[0] ?? 0)) / span))
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
      signal.style.setProperty(
        'transform',
        `translate(${railX}px, ${Math.round(firstY + progress * span)}px)`,
      )
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
    journey.removeAttribute('data-k-journey-mode')
    signal?.style.removeProperty('transform')
    sections.forEach((section) => section.removeAttribute('data-k-route-state'))
  }
}
