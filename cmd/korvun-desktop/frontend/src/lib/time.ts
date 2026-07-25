// The chrome's one Spanish time-ago formatter (header badge + card
// captions) — two copies would drift (review finding).

/** "hace N s" / "hace N min" since the given epoch ms; '' for null. */
export function agoSeconds(fromMs: number | null): string {
  if (fromMs === null) return ''
  const s = Math.max(0, Math.round((Date.now() - fromMs) / 1000))
  if (s < 60) return `hace ${s} s`
  return `hace ${Math.floor(s / 60)} min`
}

/** Wall-clock hour, 24h es-ES (6b review rider a: the WHOLE app speaks
 * 24h — banner and Actividad had drifted apart); '—' when unparseable. */
export function hourES(ts: string): string {
  const d = new Date(ts)
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleTimeString('es-ES', { hour12: false })
}
