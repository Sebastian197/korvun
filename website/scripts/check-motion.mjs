import { readdirSync, readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const SRC = fileURLToPath(new URL('../src/', import.meta.url))
const cssFiles = []
const walk = (directory) => {
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    const absolute = path.join(directory, entry.name)
    if (entry.isDirectory()) walk(absolute)
    else if (entry.name.endsWith('.css')) cssFiles.push(absolute)
  }
}
walk(SRC)

const allowed = new Set(['transform', 'opacity', 'filter', 'none'])
const violations = []

for (const file of cssFiles) {
  const css = readFileSync(file, 'utf8')
  const relative = path.relative(SRC, file)

  for (const match of css.matchAll(/@keyframes\s+([\w-]+)[^{]*\{([\s\S]*?)\n\}/g)) {
    for (const declaration of match[2].matchAll(/([a-z-]+)\s*:/g)) {
      if (!allowed.has(declaration[1])) {
        violations.push(`${relative} @keyframes ${match[1]} animates ${declaration[1]}`)
      }
    }
  }

  for (const match of css.matchAll(/transition(?:-property)?\s*:\s*([^;]+);/g)) {
    for (const segment of match[1].replace(/\([^)]*\)/g, '()').split(',')) {
      const property = segment.trim().split(/\s+/)[0]
      if (!allowed.has(property)) {
        violations.push(`${relative} transition animates ${property}`)
      }
    }
  }
}

if (violations.length > 0) {
  console.error(`FAIL: ${violations.length} motion-property violation(s):`)
  for (const violation of violations) console.error(`  - ${violation}`)
  process.exit(1)
}

console.log('check-motion: every authored animation uses transform/opacity/filter — OK')
