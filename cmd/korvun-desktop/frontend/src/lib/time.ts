// The chrome's one Spanish time-ago formatter (header badge + card
// captions) — two copies would drift (review finding).

/** "hace N s" / "hace N min" since the given epoch ms; '' for null. */
export function agoSeconds(fromMs: number | null): string {
  if (fromMs === null) return ''
  const s = Math.max(0, Math.round((Date.now() - fromMs) / 1000))
  if (s < 60) return `hace ${s} s`
  return `hace ${Math.floor(s / 60)} min`
}
