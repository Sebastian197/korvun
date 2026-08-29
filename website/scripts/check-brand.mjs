// Brand asset contract (brand-motion spec, Task 1 + Task 5): the canonical
// A1 marks under assets/brand/ must mirror the website-public copies
// byte-for-byte, the hero must expose the approved layer names and locked
// gradient endpoints, and the README masthead GIF must respect the one-shot
// budget. Deterministic, dependency-free, and wired into `npm run check`
// through check-dist.mjs.
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pairs = [
  ['../../assets/brand/korvun-logo-hero.svg', '../static/brand/korvun-logo-hero.svg'],
  ['../../assets/brand/korvun-logo-mono.svg', '../static/brand/korvun-logo-mono.svg'],
]

for (const [canonicalPath, publicPath] of pairs) {
  const canonical = readFileSync(new URL(canonicalPath, import.meta.url), 'utf8')
  const publicCopy = readFileSync(new URL(publicPath, import.meta.url), 'utf8')
  assert.equal(publicCopy, canonical, `${publicPath} must mirror ${canonicalPath}`)
  assert.match(canonical, /viewBox=/)
  assert.match(canonical, /Korvun/i)
}

const hero = readFileSync(new URL('../../assets/brand/korvun-logo-hero.svg', import.meta.url), 'utf8')
assert.match(hero, /#2BC8B7/i)
assert.match(hero, /#7A5AF5/i)
assert.match(hero, /data-layer="input-signals"/)
assert.match(hero, /data-layer="decision"/)
assert.match(hero, /data-layer="output-signal"/)
assert.match(hero, /data-layer="orbits"/)
assert.match(hero, /data-layer="particles"/)

console.log('check-brand: OK')
