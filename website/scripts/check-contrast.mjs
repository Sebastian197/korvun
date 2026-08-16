import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const CSS = fileURLToPath(new URL('../src/css/custom.css', import.meta.url))
const css = readFileSync(CSS, 'utf8')

const token = (name) => {
  const match = css.match(new RegExp(`${name}\\s*:\\s*(#[0-9a-fA-F]{6})`))
  return match?.[1]
}

const luminance = (hex) => {
  const linear = (channel) => {
    const value = channel / 255
    return value <= 0.04045
      ? value / 12.92
      : ((value + 0.055) / 1.055) ** 2.4
  }
  const value = Number.parseInt(hex.slice(1), 16)
  return (
    0.2126 * linear((value >> 16) & 0xff) +
    0.7152 * linear((value >> 8) & 0xff) +
    0.0722 * linear(value & 0xff)
  )
}

const contrast = (foreground, background) => {
  const values = [luminance(foreground), luminance(background)].sort(
    (a, b) => b - a,
  )
  return (values[0] + 0.05) / (values[1] + 0.05)
}

const background = token('--ifm-background-color')
const primary = token('--ifm-color-primary')
const primaryDark = token('--ifm-color-primary-dark')
const teal = token('--korvun-teal')
const pairs = [
  ['primary text', primary, background, 4.5],
  ['teal text', teal, background, 4.5],
  ['white button text', '#FFFFFF', primaryDark, 4.5],
]

const failures = []
for (const [label, foreground, surface, minimum] of pairs) {
  if (!foreground || !surface) {
    failures.push(`${label}: missing color token`)
    continue
  }
  const ratio = contrast(foreground, surface)
  console.log(`${label}: ${ratio.toFixed(2)}:1`)
  if (ratio < minimum) failures.push(`${label}: ${ratio.toFixed(2)}:1 < ${minimum}:1`)
}

if (failures.length > 0) {
  console.error(`FAIL: ${failures.length} contrast violation(s):`)
  for (const failure of failures) console.error(`  - ${failure}`)
  process.exit(1)
}

console.log('check-contrast: audited UI token pairs meet WCAG AA — OK')
