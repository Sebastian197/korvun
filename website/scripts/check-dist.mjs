// Link + asset integrity over the built site (design spec AS-1, ADR-0040).
//
// The site is a GitHub *project* page served under /korvun/ — a root-absolute
// URL that does not start with the base 404s in production while working fine
// on a local dev server, which is exactly the regression this guard exists to
// catch. Stdlib Node only; run from website/ (the Makefile website-check
// target does) after `vitepress build`.
//
// Checks, over every .html in .vitepress/dist:
//   1. every root-absolute href/src/poster starts with BASE;
//   2. every internal link/asset resolves to a file in dist
//      (accepting p, p.html, p/index.html — VitePress default URL style);
//   3. the entry pages exist (index.html and es/index.html — the two locale
//      roots the i18n layering promises).

import { readdirSync, readFileSync, existsSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const BASE = '/korvun/'
const DIST = fileURLToPath(new URL('../.vitepress/dist/', import.meta.url))

if (!existsSync(DIST)) {
  console.error(`FAIL: ${DIST} does not exist — run the build first`)
  process.exit(1)
}

// Collect every file in dist as a set of posix-relative paths.
const files = new Set()
const walk = (dir) => {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name)
    if (entry.isDirectory()) walk(full)
    else files.add(path.relative(DIST, full).split(path.sep).join('/'))
  }
}
walk(DIST)

for (const entry of ['index.html', 'es/index.html']) {
  if (!files.has(entry)) {
    console.error(`FAIL: expected locale root page missing in dist: ${entry}`)
    process.exit(1)
  }
}

const htmlFiles = [...files].filter((f) => f.endsWith('.html'))
const SKIP = /^(https?:)?\/\/|^mailto:|^tel:|^data:|^javascript:|^#/
const ATTR = /(?:href|src|poster)=["']([^"']+)["']/g

const resolves = (target) => {
  const t = target.replace(/\/+$/, '/') // collapse duplicate trailing slashes
  const candidates =
    t === '' || t.endsWith('/')
      ? [`${t}index.html`]
      : [t, `${t}.html`, `${t}/index.html`]
  return candidates.some((c) => files.has(c))
}

const violations = []
let checked = 0
for (const file of htmlFiles) {
  const html = readFileSync(path.join(DIST, file), 'utf-8')
  for (const [, raw] of html.matchAll(ATTR)) {
    if (SKIP.test(raw)) continue
    const url = raw.split(/[?#]/)[0]
    if (url === '') continue // pure anchor/query
    checked++
    let target
    if (url.startsWith('/')) {
      if (!url.startsWith(BASE)) {
        violations.push(`${file}: root-absolute outside base: ${raw}`)
        continue
      }
      target = url.slice(BASE.length)
    } else {
      target = path.posix.normalize(
        path.posix.join(path.posix.dirname(file), url),
      )
    }
    if (!resolves(decodeURIComponent(target))) {
      violations.push(`${file}: broken internal link/asset: ${raw}`)
    }
  }
}

if (violations.length > 0) {
  console.error(`FAIL: ${violations.length} integrity violation(s):`)
  for (const v of violations) console.error(`  - ${v}`)
  process.exit(1)
}
console.log(
  `check-dist: ${htmlFiles.length} pages, ${checked} internal refs checked, all resolve under ${BASE} — OK`,
)
