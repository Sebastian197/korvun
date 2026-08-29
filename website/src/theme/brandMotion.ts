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
