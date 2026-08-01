// SP4 RED (builder-canvas): the CANVAS as the builder's main face INSIDE the
// desktop iframe (SP6 harness — real core via the bearer-injecting proxy, so
// any pasted token is overwritten with the real one, internal/shell/proxy.go).
// Real palette drag (HTML5 DnD via Frame.dragAndDrop) and a real handle
// connection must work in the WebView — the NC-5 duty extended to the canvas.
// RED today: /builder/ still serves the 2b form editor after the paste, so
// canvas-surface never appears in the frame.
import { expect, test, type Page } from '@playwright/test'
import { installBindings } from './bindings'
import { BASE } from './util'

test.describe.configure({ mode: 'serial' })

async function ensureRunning(page: Page): Promise<void> {
  await page.request.post(`${BASE}/__test/bindings/Start`, { data: [] }).catch(() => undefined)
  await expect(page.getByTestId('healthz-badge')).toContainText('OK', { timeout: 15000 })
}

test.beforeEach(async ({ request }) => {
  await request.post(`${BASE}/__test/model`, { data: { mode: 'ok' } })
  await request.post(`${BASE}/__test/channel`, { data: { send: 'ok' } })
  await request.post(`${BASE}/__test/reset-config`, { data: [] })
})

// This file sorts alphabetically BEFORE chrome.spec.ts, whose first tests
// contract the INITIAL stopped core (-start=false). The suite is serial over
// ONE shared core, so hand the stopped state back on the way out.
test.afterAll(async ({ request }) => {
  await request.post(`${BASE}/__test/core-exit`, { data: [] }).catch(() => undefined)
})

test('el lienzo es la cara del builder en el iframe: drag de paleta y conexión reales', async ({
  page,
}) => {
  await installBindings(page)
  await page.goto('/')
  await ensureRunning(page)
  await page.getByRole('button', { name: 'Builder' }).click()
  const frameLoc = page.frameLocator('iframe[title="Builder"]')

  // The token gate inside the iframe: ANY value works — the SP4 proxy
  // overwrites Authorization with the real bearer (proxy.go:77).
  await frameLoc.getByLabel('admin bearer token').fill('x')
  await frameLoc.getByRole('button', { name: 'Load' }).click()
  await expect(frameLoc.getByTestId('canvas-surface')).toBeVisible({ timeout: 15000 })

  // Real palette drag INSIDE the WebView (HTML5 DnD across the frame).
  const frame = page.frames().find((f) => f.url().includes('/builder'))
  if (!frame) throw new Error('builder frame not found')
  const brainsBefore = await frameLoc.locator('[data-kind="brain"]').count()
  await frame.dragAndDrop('[data-testid="palette:brain"]', '[data-testid="canvas-surface"]')
  await expect(frameLoc.locator('[data-kind="brain"]')).toHaveCount(brainsBefore + 1)

  // Real handle-to-handle connection (pointer capture across the frame
  // boundary — the SP0 NC-5 duty, now on the production canvas). The harness
  // core boots with telegram→asistente already routed; cable the NEW brain.
  const newBrainId = `brain:${brainsBefore}` // stable index scheme (SP2)
  const src = await frameLoc
    .locator('[data-testid="channel:0"] .react-flow__handle.source')
    .first()
    .boundingBox()
  const dst = await frameLoc
    .locator(`[data-testid="${newBrainId}"] .react-flow__handle.target`)
    .first()
    .boundingBox()
  if (!src || !dst) throw new Error('handles not measurable inside the frame')
  const edgesBefore = await frameLoc.locator('.react-flow__edge').count()
  await page.mouse.move(src.x + src.width / 2, src.y + src.height / 2)
  await page.mouse.down()
  await page.mouse.move(dst.x + dst.width / 2, dst.y + dst.height / 2, { steps: 12 })
  await page.mouse.up()
  await expect(frameLoc.locator('.react-flow__edge')).toHaveCount(edgesBefore + 1)
})
