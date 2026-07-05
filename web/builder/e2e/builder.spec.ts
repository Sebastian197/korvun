import { test, expect, type Page, type Route } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'

const ORIGIN = 'http://localhost:4173'

const BASE_CONFIG = {
  channels: [{ type: 'telegram', mode: 'polling', token_env: 'KORVUN_TG' }],
  brains: [
    {
      name: 'support',
      sensitivity: 'private',
      policy: { kind: 'priority' },
      dispatch: 'fanout',
      models: [{ provider: 'ollama', model_id: 'llama3.2:1b', locality: 'local' }],
    },
  ],
  routes: [{ channel: 'telegram', brain: 'support' }],
  admin: { token_env: 'KORVUN_ADMIN_TOKEN' },
}
const BRAINS = [
  { name: 'support', sensitivity: 'private', policy: 'priority', dispatch: 'fanout', models: [] },
]
const CHANNELS = [{ type: 'telegram', mode: 'polling', name: 'telegram' }]

function json(route: Route, status: number, body: unknown) {
  return route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })
}

// Record any request to a non-same-origin origin — the network no-CDN gate (ADR-0029 §5).
function trackExternal(page: Page): string[] {
  const external: string[] = []
  page.on('request', (req) => {
    const u = req.url()
    if (!u.startsWith(ORIGIN) && !u.startsWith('data:') && !u.startsWith('blob:')) external.push(u)
  })
  return external
}

async function readOnly(page: Page) {
  await page.route('**/api/brains', (r) => json(r, 200, BRAINS))
  await page.route('**/api/channels', (r) => json(r, 200, CHANNELS))
}

async function loadWithToken(page: Page) {
  await page.goto('/builder/')
  await page.getByLabel('admin bearer token').fill('secret')
  await page.getByRole('button', { name: 'Load' }).click()
  await expect(page.getByLabel('name', { exact: true })).toBeVisible()
}

test('reload survives ECONNREFUSED during cutover, reaches succeeded, makes no external requests', async ({
  page,
}) => {
  const external = trackExternal(page)
  await readOnly(page)

  let cfg: unknown = BASE_CONFIG
  await page.route('**/api/config', (r) =>
    r.request().method() === 'POST' ? json(r, 202, { handle: 'r1' }) : json(r, 200, cfg),
  )

  let poll = 0
  await page.route('**/api/reload/r1', (r) => {
    poll++
    if (poll === 1) return json(r, 200, { state: 'cutover-in-progress' })
    if (poll <= 3) return r.abort('connectionrefused') // the admin server is restarting (F4)
    cfg = { ...BASE_CONFIG, brains: [{ ...BASE_CONFIG.brains[0], name: 'support-v2' }] }
    return json(r, 200, { state: 'succeeded' })
  })

  await loadWithToken(page)
  await page.getByLabel('name', { exact: true }).fill('support-v2')
  await page.getByRole('button', { name: 'Save and reload' }).click()

  await expect(page.getByTestId('reload-inflight')).toContainText('cutover-in-progress')
  await expect(page.getByTestId('reload-succeeded')).toBeVisible({ timeout: 30_000 })
  expect(external, `unexpected external requests: ${external.join(', ')}`).toEqual([])
})

test('axe-core: the editor forms have no accessibility violations', async ({ page }) => {
  await readOnly(page)
  await page.route('**/api/config', (r) => json(r, 200, BASE_CONFIG))
  await loadWithToken(page)
  const results = await new AxeBuilder({ page }).include('.editor').analyze()
  expect(results.violations, JSON.stringify(results.violations, null, 2)).toEqual([])
})

test('409 self-lock renders its distinct treatment', async ({ page }) => {
  await readOnly(page)
  await page.route('**/api/config', (r) =>
    r.request().method() === 'POST'
      ? json(r, 409, { error_code: 'config_would_self_lock', message: 'x' })
      : json(r, 200, BASE_CONFIG),
  )
  await loadWithToken(page)
  await page.getByLabel('name', { exact: true }).fill('x')
  await page.getByRole('button', { name: 'Save and reload' }).click()
  await expect(page.getByTestId('save-selflock')).toBeVisible()
  await expect(page.getByTestId('save-reload-in-progress')).toHaveCount(0)
})

test('401 clears the token and returns to the paste screen', async ({ page }) => {
  await readOnly(page)
  await page.route('**/api/config', (r) =>
    r.request().method() === 'POST' ? json(r, 401, { error: 'unauthorized' }) : json(r, 200, BASE_CONFIG),
  )
  await loadWithToken(page)
  await page.getByLabel('name', { exact: true }).fill('x')
  await page.getByRole('button', { name: 'Save and reload' }).click()
  await expect(page.getByLabel('admin bearer token')).toBeVisible()
})

test('the no-same-origin guard BITES: a simulated external request is detected (CSP-blocked)', async ({
  page,
}) => {
  const external = trackExternal(page)
  await readOnly(page)
  await page.goto('/builder/')
  await page.evaluate(() => {
    void fetch('https://cdn.example.com/smuggled.js').catch(() => undefined)
  })
  await expect.poll(() => external.some((u) => u.includes('cdn.example.com'))).toBe(true)
})
