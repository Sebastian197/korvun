import { readFile, writeFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'
import { createRequire } from 'node:module'

import { patchWebpackBarSource } from './webpackbar-compat.mjs'

const require = createRequire(import.meta.url)
const commonJsEntry = require.resolve('webpackbar')
const packageRoot = dirname(dirname(commonJsEntry))
const packageJson = JSON.parse(await readFile(join(packageRoot, 'package.json'), 'utf8'))

if (packageJson.version !== '6.0.1') {
  throw new Error(`Unsupported webpackbar version: ${packageJson.version}`)
}

for (const filename of ['index.cjs', 'index.mjs']) {
  const path = join(packageRoot, 'dist', filename)
  const source = await readFile(path, 'utf8')
  const patched = patchWebpackBarSource(source)

  if (patched !== source) {
    await writeFile(path, patched)
  }
}

console.log('webpackbar 6.0.1 compatibility patch applied')
