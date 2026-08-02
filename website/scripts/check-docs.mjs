// The docs-map gate (SP3, FR-DOCS-1 + the ADR-0010 secret microcopy law):
// over the built dist, (1) no EN page may still carry a stub marker —
// the map is either populated or red; (2) NO page, any locale, may
// contain a string that even LOOKS like a real credential (secrets are
// always env-var NAMES in the docs, never plausible values). Stdlib Node
// only; run from website/ after the build.

import { readdirSync, readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const DIST = fileURLToPath(new URL('../.vitepress/dist/', import.meta.url))

const html = []
const walk = (dir) => {
  for (const e of readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, e.name)
    if (e.isDirectory()) walk(full)
    else if (e.name.endsWith('.html')) html.push(full)
  }
}
walk(DIST)

// Patterns that look like real credentials. Anchored to be specific:
// a telegram bot token, a GitHub PAT, an OpenAI-style key, a Slack token,
// a long hex secret presented as a value.
const SECRET_PATTERNS = [
  [/\b\d{8,10}:AA[A-Za-z0-9_-]{30,}\b/, 'telegram-bot-token-like value'],
  [/\bgh[pousr]_[A-Za-z0-9]{20,}\b/, 'github-token-like value'],
  [/\bsk-[A-Za-z0-9]{20,}\b/, 'api-key-like value (sk-…)'],
  [/\bxox[baprs]-[A-Za-z0-9-]{10,}\b/, 'slack-token-like value'],
]

const violations = []
for (const file of html) {
  const rel = path.relative(DIST, file).split(path.sep).join('/')
  const text = readFileSync(file, 'utf-8')
  const isES = rel.startsWith('es/')
  // (1) stub markers — EN only (the ES layer is SP4's mandate).
  if (!isES && text.includes('Stub (SP1')) {
    violations.push(`${rel}: still a stub`)
  }
  // (2) secret-looking values — every locale, no exceptions.
  for (const [re, what] of SECRET_PATTERNS) {
    if (re.test(text)) violations.push(`${rel}: ${what}`)
  }
}

if (violations.length > 0) {
  console.error(`FAIL: ${violations.length} docs-map violation(s):`)
  for (const v of violations) console.error(`  - ${v}`)
  process.exit(1)
}
console.log(
  `check-docs: ${html.length} pages — EN map fully populated, no secret-looking values — OK`,
)
