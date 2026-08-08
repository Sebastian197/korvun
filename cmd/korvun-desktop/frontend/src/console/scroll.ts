// Scroll math for the conversation pane (operator-console spec
// FR-POLISH): autoscroll must respect a human who scrolled up to read.
// Lives outside the component file so the view exports only components
// (react-refresh/only-export-components).

/** True when el is scrolled close enough to its bottom that new content
 * should keep it pinned there. */
export function isNearBottom(el: {
  scrollHeight: number
  scrollTop: number
  clientHeight: number
}): boolean {
  return el.scrollHeight - el.scrollTop - el.clientHeight < 48
}
