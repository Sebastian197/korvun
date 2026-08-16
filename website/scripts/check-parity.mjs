import { existsSync, readdirSync, readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const ROOT = fileURLToPath(new URL('../', import.meta.url))
const EN = path.join(ROOT, 'docs')
const ES = path.join(ROOT, 'i18n/es/docusaurus-plugin-content-docs/current')

const markdownFiles = (root) => {
  if (!existsSync(root)) return []
  const result = []
  const walk = (directory) => {
    for (const entry of readdirSync(directory, { withFileTypes: true })) {
      const absolute = path.join(directory, entry.name)
      if (entry.isDirectory()) walk(absolute)
      else if (/\.mdx?$/.test(entry.name)) {
        result.push(path.relative(root, absolute).split(path.sep).join('/'))
      }
    }
  }
  walk(root)
  return result.sort()
}

const en = markdownFiles(EN)
const es = markdownFiles(ES)
const enSet = new Set(en)
const esSet = new Set(es)
const violations = []

if (en.length === 0) violations.push('English Docusaurus docs tree is missing or empty')
if (es.length === 0) violations.push('Spanish Docusaurus docs tree is missing or empty')

for (const file of en) {
  if (!esSet.has(file)) violations.push(`missing ES twin: ${file}`)
}
for (const file of es) {
  if (!enSet.has(file)) violations.push(`ES page without an EN twin: ${file}`)
}

const fencedBlocks = (source) =>
  [...source.matchAll(/```[^\n]*\n([\s\S]*?)```/g)].map((match) => match[1])

for (const file of en) {
  if (!esSet.has(file)) continue
  const enSource = readFileSync(path.join(EN, file), 'utf8')
  const esSource = readFileSync(path.join(ES, file), 'utf8')

  if (esSource.includes('(EN)')) {
    violations.push(`${file}: residual "(EN)" marker in Spanish mirror`)
  }

  const enBlocks = fencedBlocks(enSource)
  const esBlocks = fencedBlocks(esSource)
  if (enBlocks.length !== esBlocks.length) {
    violations.push(
      `${file}: code-block count differs (${enBlocks.length} vs ${esBlocks.length})`,
    )
    continue
  }
  enBlocks.forEach((block, index) => {
    if (block !== esBlocks[index]) {
      violations.push(`${file}: code block #${index + 1} differs between locales`)
    }
  })
}

if (violations.length > 0) {
  console.error(`FAIL: ${violations.length} locale-parity violation(s):`)
  for (const violation of violations) console.error(`  - ${violation}`)
  process.exit(1)
}

console.log(
  `check-parity: ${en.length} source page pairs — full ES mirror and byte-identical technical blocks — OK`,
)
