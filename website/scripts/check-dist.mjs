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

await import('./check-brand.mjs')

const BASE = process.env.SITE_BASE_URL ?? '/korvun/'
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

const requiredPages = [
  'index.html',
  'guide/what-is-korvun/index.html',
  'guide/install/index.html',
  'guide/quickstart/index.html',
  'guide/builder/index.html',
  'channels/telegram/index.html',
  'channels/discord/index.html',
  'channels/webhook/index.html',
  'reference/configuration/index.html',
  'releases/index.html',
]

for (const entry of [
  ...requiredPages,
  ...requiredPages.map((entry) => `es/${entry}`),
]) {
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
    if (
      file.endsWith('404.html') &&
      (target === '404/' || target === 'es/404/')
    ) {
      continue
    }
    if (!resolves(decodeURIComponent(target))) {
      violations.push(`${file}: broken internal link/asset: ${raw}`)
    }
  }
}

// The two locale roots are a product surface, not only a collection of
// resolvable files. Assert the selected landing story on the rendered HTML so
// this contract still runs where a browser server cannot bind a local port.
const landingContracts = [
  {
    file: 'index.html',
    headings: [
      'One binary. Your models. Your rules.',
      'Install with proof, not promises.',
      'Eight pillars. One binary.',
      'Sensitive data never leaves the machine.',
      'A cloud model, governed tools, one config.',
      'Run it today.',
    ],
  },
  {
    file: 'es/index.html',
    headings: [
      'Un binario. Tus modelos. Tus reglas.',
      'Instala con pruebas, no promesas.',
      'Ocho pilares. Un binario.',
      'Los datos sensibles no salen de tu equipo.',
      'Un modelo de nube, herramientas gobernadas, una sola config.',
      'Ponlo en marcha hoy.',
    ],
  },
]

const renderedText = (html) =>
  html
    .replace(/<script[\s\S]*?<\/script>/g, ' ')
    .replace(/<style[\s\S]*?<\/style>/g, ' ')
    .replace(/<[^>]+>/g, ' ')
    .replace(/&nbsp;|&#160;/g, ' ')
    .replace(/&amp;/g, '&')
    .replace(/&#39;|&#x27;/g, "'")
    .replace(/&quot;/g, '"')
    .replace(/\s+/g, ' ')
    .trim()

for (const contract of landingContracts) {
  const html = readFileSync(path.join(DIST, contract.file), 'utf-8')
  const text = renderedText(html)
  let cursor = -1
  for (const heading of contract.headings) {
    const next = text.indexOf(heading, cursor + 1)
    if (next < 0) {
      violations.push(`${contract.file}: missing landing heading: ${heading}`)
      continue
    }
    if (next < cursor) {
      violations.push(`${contract.file}: landing heading out of order: ${heading}`)
    }
    cursor = next
  }

  for (const fact of ['v0.9.0', 'Apache-2.0', 'cosign', 'SBOM']) {
    if (!text.includes(fact)) {
      violations.push(`${contract.file}: missing verified release fact: ${fact}`)
    }
  }
  for (const claim of [
    'MIT',
    'install.sh',
    'brew install',
    'apt-get install',
    'scoop install',
  ]) {
    if (text.includes(claim)) {
      violations.push(`${contract.file}: unverified landing claim: ${claim}`)
    }
  }
  if ((html.match(/data-k-section=/g) ?? []).length !== 6) {
    violations.push(`${contract.file}: expected 6 landing story sections`)
  }
  const landingHtml = html.match(/<main\b[\s\S]*?<\/main>/)?.[0] ?? ''
  if (/href=["']#["']/.test(landingHtml)) {
    violations.push(`${contract.file}: placeholder landing link: href="#"`)
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
