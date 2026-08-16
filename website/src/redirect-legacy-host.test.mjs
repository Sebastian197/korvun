import assert from 'node:assert/strict'
import test from 'node:test'

// The module must be importable in Node (no `window`): the client module's
// side effect only runs in a browser, so a plain import must never throw.
const module = await import('./redirect-legacy-host.ts').catch(() => null)

test('redirect-legacy-host module loads outside a browser', () => {
  assert.ok(module, 'importing redirect-legacy-host.ts must not throw in Node')
})

test('legacyRedirectTarget maps legacy GitHub Pages URLs onto korvun.dev', () => {
  const { legacyRedirectTarget } = module
  const cases = [
    // [input href, expected target]
    ['https://sebastian197.github.io/korvun/', 'https://korvun.dev/'],
    ['https://sebastian197.github.io/korvun', 'https://korvun.dev/'],
    ['https://sebastian197.github.io/korvun/es/', 'https://korvun.dev/es/'],
    [
      'https://sebastian197.github.io/korvun/guide/quickstart/?ref=old#install',
      'https://korvun.dev/guide/quickstart/?ref=old#install',
    ],
    [
      'https://sebastian197.github.io/korvun/es/reference/configuration/?a=1&b=2',
      'https://korvun.dev/es/reference/configuration/?a=1&b=2',
    ],
    // `/korvun` is stripped only as a whole path segment.
    [
      'https://sebastian197.github.io/korvun-other/',
      'https://korvun.dev/korvun-other/',
    ],
  ]
  for (const [href, expected] of cases) {
    assert.equal(legacyRedirectTarget(href), expected, `href: ${href}`)
  }
})

test('legacyRedirectTarget is inert on every non-legacy host', () => {
  const { legacyRedirectTarget } = module
  const inert = [
    'https://korvun.dev/',
    'https://korvun.dev/es/',
    'https://www.korvun.dev/guide/quickstart/',
    'https://korvun.pages.dev/',
    'http://localhost:3000/korvun/',
    'http://127.0.0.1:3000/',
    'https://example.github.io/korvun/', // another owner's Pages site
  ]
  for (const href of inert) {
    assert.equal(legacyRedirectTarget(href), null, `href: ${href}`)
  }
})

test('legacyRedirectTarget never throws on malformed input', () => {
  const { legacyRedirectTarget } = module
  assert.equal(legacyRedirectTarget('not a url'), null)
  assert.equal(legacyRedirectTarget(''), null)
})
