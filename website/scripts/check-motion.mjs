// The motion-property gate (SP2b, the "transform/opacity ONLY" law from
// the design spec FR-MOT-1 and ROAD-TO-BETA): every @keyframes block and
// every transition declared in the site's own CSS may animate transform
// and opacity, nothing else — anything else is compositor-hostile and
// goes red HERE, not in a code review. Stdlib Node only; run from
// website/.

import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const CSS = fileURLToPath(
  new URL('../.vitepress/theme/custom.css', import.meta.url),
)
const css = readFileSync(CSS, 'utf-8')
const ALLOWED = new Set(['transform', 'opacity', 'none'])
const offences = []

// @keyframes blocks: any declared property must be transform/opacity.
// (Blocks close with a "\n}" at column 0 — the file's Prettier format.)
for (const m of css.matchAll(/@keyframes\s+([\w-]+)[^{]*\{([\s\S]*?)\n\}/g)) {
  const [, name, body] = m
  for (const d of body.matchAll(/([a-z-]+)\s*:/g)) {
    if (!ALLOWED.has(d[1])) {
      offences.push(`@keyframes ${name}: animates "${d[1]}"`)
    }
  }
}

// transition / transition-property: every comma-separated segment's first
// token is the animated property. Parenthesized groups are emptied FIRST —
// a cubic-bezier(a, b, c, d) easing carries commas that would otherwise
// split the segment mid-function (this gate bit its own author on day one).
for (const m of css.matchAll(/transition(?:-property)?\s*:\s*([^;]+);/g)) {
  for (const part of m[1].replace(/\([^)]*\)/g, '()').split(',')) {
    const prop = part.trim().split(/\s+/)[0]
    if (!ALLOWED.has(prop)) {
      offences.push(`transition animates "${prop}" ("${part.trim()}")`)
    }
  }
}

if (offences.length > 0) {
  console.error(`FAIL: ${offences.length} motion-property offence(s):`)
  for (const o of offences) console.error(`  - ${o}`)
  process.exit(1)
}
console.log('check-motion: every animated property is transform/opacity — OK')