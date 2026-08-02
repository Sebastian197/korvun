// The locale-parity gate (SP4b — Chano's 2026-08-02 decision: the ES
// locale is a FULL MIRROR of EN, not a layer). Permanent: a future page
// added without its ES twin goes red HERE, forever. Stdlib Node only;
// run from website/ after the build.
//
// Three laws over the built dist:
//   1. BIJECTION — every non-es page has an es/ twin and vice versa
//      (404.html exempt: VitePress emits a single global one).
//   2. NO "(EN)" residue — the mirror is complete, so no ES page may
//      still mark a link as English-only.
//   3. TECHNICAL TRUTH — the fenced code blocks of each page pair are
//      byte-identical, in order: commands, paths, ports and env-var
//      names do not translate.

import { readdirSync, readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const DIST = fileURLToPath(new URL('../.vitepress/dist/', import.meta.url))

const pages = []
const walk = (dir) => {
  for (const e of readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, e.name)
    if (e.isDirectory()) walk(full)
    else if (e.name.endsWith('.html')) {
      pages.push(path.relative(DIST, full).split(path.sep).join('/'))
    }
  }
}
walk(DIST)

const en = new Set(
  pages.filter((p) => !p.startsWith('es/') && p !== '404.html'),
)
const es = new Set(
  pages.filter((p) => p.startsWith('es/')).map((p) => p.slice(3)),
)

const violations = []
for (const p of en) {
  if (!es.has(p)) violations.push(`missing ES twin: es/${p}`)
}
for (const p of es) {
  if (!en.has(p)) violations.push(`ES page without an EN twin: es/${p}`)
}

// Fenced code blocks as rendered text (pre > code, highlight spans
// stripped, entities decoded) — inline <code> in prose is NOT covered,
// only blocks.
const codeBlocks = (file) => {
  const html = readFileSync(path.join(DIST, file), 'utf-8')
  const blocks = []
  for (const m of html.matchAll(/<pre[^>]*><code[^>]*>([\s\S]*?)<\/code><\/pre>/g)) {
    blocks.push(
      m[1]
        .replace(/<[^>]+>/g, '')
        .replaceAll('&lt;', '<')
        .replaceAll('&gt;', '>')
        .replaceAll('&quot;', '"')
        .replaceAll('&#39;', "'")
        .replaceAll('&amp;', '&'),
    )
  }
  return blocks
}

for (const p of en) {
  if (!es.has(p)) continue
  const esFile = `es/${p}`
  // 2. "(EN)" residue.
  const esHtml = readFileSync(path.join(DIST, esFile), 'utf-8')
  if (esHtml.includes('(EN)')) {
    violations.push(`${esFile}: residual "(EN)" marker — the mirror is complete`)
  }
  // 3. code-block parity, ordered.
  const a = codeBlocks(p)
  const b = codeBlocks(esFile)
  if (a.length !== b.length) {
    violations.push(
      `${p} ↔ ${esFile}: code-block count differs (${a.length} vs ${b.length})`,
    )
    continue
  }
  for (let i = 0; i < a.length; i++) {
    if (a[i] !== b[i]) {
      violations.push(
        `${p} ↔ ${esFile}: code block #${i + 1} differs — technical truth must be byte-identical`,
      )
    }
  }
}

if (violations.length > 0) {
  console.error(`FAIL: ${violations.length} locale-parity violation(s):`)
  for (const v of violations) console.error(`  - ${v}`)
  process.exit(1)
}
console.log(
  `check-parity: ${en.size} page pairs — full ES mirror, zero "(EN)" residue, code blocks byte-identical — OK`,
)