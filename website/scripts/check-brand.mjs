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
  ['../../assets/brand/korvun-mark.svg', '../static/brand/korvun-mark.svg'],
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

// Task 5 — the README masthead GIF: one-shot (no infinite Netscape loop),
// within the approved dimensions and byte budget.
const gif = readFileSync(new URL('../../assets/brand/korvun-readme-masthead.gif', import.meta.url))
assert.equal(gif.subarray(0, 6).toString('ascii'), 'GIF89a')
assert.ok(gif.readUInt16LE(6) <= 1280)
assert.ok(gif.readUInt16LE(8) <= 520)
assert.ok(gif.byteLength <= 2.5 * 1024 * 1024)
const netscape = gif.indexOf(Buffer.from('NETSCAPE2.0', 'ascii'))
if (netscape >= 0) {
  assert.notDeepEqual([...gif.subarray(netscape + 11, netscape + 16)], [3, 1, 0, 0, 0])
}

console.log('check-brand: gif OK')
