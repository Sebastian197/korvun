// SP0 canvas spike (ADR-0039): NC-5 — the React Flow canvas must stay fully
// interactive INSIDE the desktop Builder iframe (drag, pointer capture, focus).
// Spike-only spec: the spike page is /builder/?spike=flow (query-gated, never
// linked in the production UI). Evidence for the SP0 GO/NO-GO; remove or
// promote when the real canvas lands.
import { expect, test, type Page } from '@playwright/test'
import { installBindings } from './bindings'
import { BASE, SHOT, settleFonts } from './util'

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

test('NC-5: drag, pointer-capture y focus del canvas dentro del iframe del escritorio', async ({
  page,
}) => {
  await installBindings(page)
  await page.goto('/')
  await ensureRunning(page)
  await page.getByRole('button', { name: 'Builder' }).click()
  const iframe = page.getByTitle('Builder')
  await expect(iframe).toHaveAttribute('src', '/builder/')

  // Point the SAME same-origin iframe at the spike page (query-gated).
  await iframe.evaluate((el) => {
    ;(el as HTMLIFrameElement).src = '/builder/?spike=flow'
  })
  const frame = page.frameLocator('iframe[title="Builder"]')
  await expect(frame.getByTestId('flow-spike')).toBeVisible({ timeout: 15000 })
  const nodeA = frame.locator('.react-flow__node[data-id="a"]')
  const nodeB = frame.locator('.react-flow__node[data-id="b"]')
  await expect(nodeA).toBeVisible()
  await expect(nodeB).toBeVisible()
  await page.waitForTimeout(300) // fitView settle

  const center = (box: { x: number; y: number; width: number; height: number }) => ({
    x: box.x + box.width / 2,
    y: box.y + box.height / 2,
  })
  const dragBetween = async (from: { x: number; y: number }, to: { x: number; y: number }) => {
    await page.mouse.move(from.x, from.y)
    await page.mouse.down()
    await page.mouse.move(to.x, to.y, { steps: 12 })
    await page.mouse.up()
    await page.waitForTimeout(150)
  }

  // Drag node b INSIDE the iframe — pointer capture must survive the frame
  // boundary (boundingBox of frame elements is page-viewport-relative).
  const before = await nodeB.boundingBox()
  if (!before) throw new Error('node b has no bounding box')
  const start = center(before)
  await dragBetween(start, { x: start.x + 90, y: start.y + 60 })
  const after = await nodeB.boundingBox()
  if (!after) throw new Error('node b lost its bounding box')
  expect(Math.abs(after.x - before.x)).toBeGreaterThan(40)
  expect(Math.abs(after.y - before.y)).toBeGreaterThan(20)

  // Create the b -> a connection dragging handle-to-handle inside the iframe.
  await expect(frame.locator('.react-flow__edge')).toHaveCount(1)
  const src = await frame
    .locator('.react-flow__node[data-id="b"] .react-flow__handle.source')
    .boundingBox()
  const tgt = await frame
    .locator('.react-flow__node[data-id="a"] .react-flow__handle.target')
    .boundingBox()
  if (!src || !tgt) throw new Error('handles have no bounding box')
  await dragBetween(center(src), center(tgt))
  await expect(frame.locator('.react-flow__edge')).toHaveCount(2)

  // Focus: after clicking a node, the iframe document must own the focus and
  // the node wrapper must be the active element.
  await nodeA.click()
  const focusState = await frame.locator('body').evaluate(() => ({
    hasFocus: document.hasFocus(),
    activeClass: document.activeElement?.className ?? 'none',
  }))
  expect(focusState.hasFocus).toBe(true)
  expect(focusState.activeClass).toContain('react-flow__node')

  await settleFonts(page)
  await page.screenshot({ path: SHOT('sp0-canvas-iframe.png'), animations: 'disabled' })
})