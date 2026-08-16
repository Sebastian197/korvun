import assert from 'node:assert/strict'
import test from 'node:test'

import { patchWebpackBarSource } from './webpackbar-compat.mjs'

const vulnerableSource = `
class OtherReporter {
  constructor(options) {
    this.options = options;
  }
}
class WebpackBarPlugin extends Webpack.ProgressPlugin {
  constructor(options) {
    super({ activeModules: true });
    __publicField(this, "options");
    __publicField(this, "reporters");
    this.options = Object.assign({}, DEFAULTS, options);
    const reporters = [
      ...this.options.reporters || [],
      this.options.reporter
    ];
  }
  get state() {
    return globalStates[this.options.name];
  }
}

export { WebpackBarPlugin as default };
`

test('keeps webpackbar settings separate from Webpack ProgressPlugin options', () => {
  const patched = patchWebpackBarSource(vulnerableSource)

  assert.match(patched, /__publicField\(this, "webpackBarOptions"\)/)
  assert.match(patched, /this\.webpackBarOptions = Object\.assign/)
  assert.match(patched, /this\.webpackBarOptions\.reporters/)
  assert.match(patched, /globalStates\[this\.webpackBarOptions\.name\]/)
  assert.match(patched, /class OtherReporter[\s\S]*this\.options = options/)
  assert.doesNotMatch(patched, /WebpackBarPlugin[\s\S]*this\.options/)
})

test('does not alter an already compatible webpackbar build', () => {
  const patched = patchWebpackBarSource(vulnerableSource)

  assert.equal(patchWebpackBarSource(patched), patched)
})

test('rejects an unexpected webpackbar build instead of changing it blindly', () => {
  assert.throws(
    () => patchWebpackBarSource('export default class SomethingElse {}'),
    /WebpackBarPlugin class was not found/,
  )
})
