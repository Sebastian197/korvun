import { describe, it, expect } from 'vitest'
import { themes, contrast, AA_TEXT, UI_MIN, type Theme } from './tokens'

// The AA contrast floor as an EXECUTABLE gate (ADR-0030 §8 / §2, Phase 2a
// Principle 1). Every readable text tier must clear WCAG AA (>= 4.5:1) on both
// base and surface, in BOTH themes. This is the guard that would have caught the
// design-review finding that form labels rode a ~3:1 "faint" tier. A sub-AA token
// makes this test go red.

const textTiers: (keyof Theme)[] = ['text1', 'text2', 'text3', 'accentText']
const backgrounds: (keyof Theme)[] = ['base', 'surface']
const semantic: (keyof Theme)[] = ['received', 'sent', 'dropped', 'failed']

for (const [name, theme] of Object.entries(themes)) {
  describe(`theme "${name}" contrast floor`, () => {
    for (const tier of textTiers) {
      for (const bg of backgrounds) {
        it(`${tier} on ${bg} passes AA (>= ${AA_TEXT}:1)`, () => {
          const ratio = contrast(theme[tier], theme[bg])
          expect(
            ratio,
            `${name}.${tier} (${theme[tier]}) on ${name}.${bg} (${theme[bg]}) = ${ratio.toFixed(2)}:1`,
          ).toBeGreaterThanOrEqual(AA_TEXT)
        })
      }
    }
    for (const s of semantic) {
      it(`${s} status dot on surface clears the UI floor (>= ${UI_MIN}:1)`, () => {
        const ratio = contrast(theme[s], theme.surface)
        expect(ratio, `${name}.${s} on surface = ${ratio.toFixed(2)}:1`).toBeGreaterThanOrEqual(
          UI_MIN,
        )
      })
    }
  })
}
