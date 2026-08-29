import { test, expect, type Page } from '@playwright/test'
import { resetToFixture } from './reset'

// Deflake tanda (2026-08-29, patient a): every spec starts from the
// pristine fixture — the shared live core persists applied configs, and
// order-dependent leftovers were the root cause of the edges failures.
test.beforeEach(async ({ page }) => {
  await resetToFixture(page)
})

import AxeBuilder from '@axe-core/playwright'

// SP4 RED (builder-canvas, e2e against the REAL binary — make build + serve,
// real embed, real CSP, real control API): the canvas as the builder's main
// face. RED today: after the token paste the binary still serves the 2b form
// editor, so `canvas-surface` never appears and every spec times out on it.
//
// ORIGIN and token match playwright.binary.config.ts's webServer command.
const TOKEN = 'e2e-canvas-admin'

// The fixture boots channel-less with one private brain (ollama local + groq
// cloud) — the exclusion case is on screen from the first paint (spec b).

async function openCanvas(page: Page) {
  await page.goto('/builder/')
  await page.getByLabel('admin bearer token').fill(TOKEN)
  await page.getByRole('button', { name: 'Load' }).click()
  await expect(page.getByTestId('canvas-surface')).toBeVisible({ timeout: 10_000 })
}

test('a. master flow: brain + model + channel from the palette → apply → the config carries it all', async ({
  page,
}) => {
  await openCanvas(page)

  // Create a brain: REAL palette drag (HTML5 DnD) onto the canvas.
  await page.dragAndDrop('[data-testid="palette:brain"]', '[data-testid="canvas-surface"]')
  const newBrain = page.getByTestId('brain:1')
  await expect(newBrain).toBeVisible()
  // Name it through the panel (Validate requires a name).
  await newBrain.click()
  await page.getByLabel('name', { exact: true }).fill('nuevo')

  // Drop a model ONTO the brain (NC-6) and give it a model_id.
  await page.dragAndDrop('[data-testid="palette:model"]', '[data-testid="brain:1"]')
  await expect(page.getByTestId('model:1.0')).toBeVisible()
  await page.getByTestId('model:1.0').click()
  await page.getByLabel('model_id').fill('llama3.2:1b')

  // Create a channel; make it a loopback webhook (the one no-network type):
  // token_env resolves (env set by the harness command), outbound_url is
  // required by Validate — the SP4-editable webhook block in action.
  await page.dragAndDrop('[data-testid="palette:channel"]', '[data-testid="canvas-surface"]')
  await page.getByTestId('channel:0').click()
  await page.getByLabel('type').selectOption('webhook')
  await expect(page.getByLabel('mode', { exact: true })).toHaveCount(0) // webhook takes no mode
  await page.getByLabel('token_env', { exact: true }).fill('KORVUN_E2E_HOOK')
  await page.getByLabel('outbound_url').fill('http://127.0.0.1:19999/reply')

  // Cable the route: real handle-to-handle pointer drag (SP0 precedent).
  const src = await page
    .locator(
      '[data-testid="channel:0"] ~ .react-flow__handle.source, [data-testid="channel:0"] .react-flow__handle.source',
    )
    .first()
    .boundingBox()
  const dst = await page
    .locator('[data-testid="brain:1"] .react-flow__handle.target')
    .first()
    .boundingBox()
  if (!src || !dst) throw new Error('handles not measurable')
  await page.mouse.move(src.x + src.width / 2, src.y + src.height / 2)
  await page.mouse.down()
  await page.mouse.move(dst.x + dst.width / 2, dst.y + dst.height / 2, { steps: 12 })
  await page.mouse.up()

  // Apply → the REAL reload machine against the REAL binary.
  await page.getByRole('button', { name: /aplicar/i }).click()
  await expect(page.getByTestId('reload-succeeded')).toBeVisible({ timeout: 30_000 })

  // The GET after: everything landed in the served config.
  const resp = await page.request.get('/api/config', {
    headers: { Authorization: `Bearer ${TOKEN}` },
  })
  expect(resp.ok()).toBe(true)
  const cfg = (await resp.json()) as {
    channels: Array<{ type: string; token_env: string; webhook?: { outbound_url?: string } }>
    brains: Array<{ name: string; models: Array<{ model_id: string }> }>
    routes: Array<{ channel: string; brain: string }>
  }
  expect(cfg.brains.map((b) => b.name)).toContain('nuevo')
  const nuevo = cfg.brains.find((b) => b.name === 'nuevo')
  expect(nuevo?.models.map((m) => m.model_id)).toContain('llama3.2:1b')
  expect(cfg.channels[0]?.type).toBe('webhook')
  expect(cfg.channels[0]?.webhook?.outbound_url).toBe('http://127.0.0.1:19999/reply')
  expect(cfg.routes).toContainEqual({ channel: 'webhook', brain: 'nuevo' })
})

test("b. the excluded composition edge paints a dashed stroke (SP3's declared visual debt)", async ({
  page,
}) => {
  await openCanvas(page)
  // asistente (private) × groq (cloud) → comp:0.1 carries edge-excluded; the
  // CSS must PAINT it: computed dashed stroke on the real SVG path.
  const path = page.locator('.react-flow__edge.edge-excluded .react-flow__edge-path').first()
  await expect(path).toBeVisible()
  const dash = await path.evaluate((el) => getComputedStyle(el).strokeDasharray)
  expect(dash).not.toBe('none')
  expect(dash.trim()).not.toBe('')
})

test("c. axe COMPLETE (color-contrast ON), dark and light (SP3's declared a11y debt)", async ({
  page,
}) => {
  await openCanvas(page)

  const dark = await new AxeBuilder({ page }).analyze()
  expect(dark.violations, JSON.stringify(dark.violations, null, 2)).toEqual([])

  await page.evaluate(() => {
    document.documentElement.dataset.theme = 'light'
  })
  const light = await new AxeBuilder({ page }).analyze()
  expect(light.violations, JSON.stringify(light.violations, null, 2)).toEqual([])
})
