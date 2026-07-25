// One channel-glyph table for the whole chrome (Home panel + Activity rows)
// — a new channel type is added HERE, nowhere else.
const GLYPH: Record<string, string> = { telegram: 'TG', discord: 'DC' }

/** Two-letter tile glyph for a channel type (design's TG/DC tiles). */
export function channelGlyph(type: string | undefined): string {
  if (type === undefined || type === '') return '??'
  return GLYPH[type] ?? type.slice(0, 2).toUpperCase()
}
