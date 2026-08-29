import { test, expect, type Page } from '@playwright/test'

// v0.9.2 (B8, bug-bash 2026-08-23): cables could not be disconnected by any
// gesture — controlled edges without onEdgesChange dropped every selection
// change, so onEdgesDelete was unreachable from the UI. This spec drives the
// REAL gesture end to end against the real binary: cable a route with a
// pointer drag, click the painted edge path, see the selection, press
// Backspace, and watch the cable leave the canvas.

const TOKEN = 'e2e-canvas-admin'

async function openCanvas(page: Page) {
  await page.goto('/builder/')
  await page.getByLabel('admin bearer token').fill(TOKEN)
  await page.getByRole('button', { name: 'Load' }).click()
  await expect(page.getByTestId('canvas-surface')).toBeVisible({ timeout: 10_000 })
}

// Cable channel:0 → brain:0 with a real pointer drag (SP0 precedent), then
// select the painted route path with a real click. Shared by both deletion
// gestures (Backspace and the B8-bis ✕ button).
async function cableAndSelectRoute(page: Page) {
  // A channel to cable from (the fixture boots channel-less).
  await page.dragAndDrop('[data-testid="palette:channel"]', '[data-testid="canvas-surface"]')
  await expect(page.getByTestId('channel:0')).toBeVisible()

  const src = await page
    .locator('[data-testid="channel:0"] ~ .react-flow__handle.source, [data-testid="channel:0"] .react-flow__handle.source')
    .first()
    .boundingBox()
  const dst = await page
    .locator('[data-testid="brain:0"] .react-flow__handle.target')
    .first()
    .boundingBox()
  if (!src || !dst) throw new Error('handles not measurable')
  await page.mouse.move(src.x + src.width / 2, src.y + src.height / 2)
  await page.mouse.down()
  await page.mouse.move(dst.x + dst.width / 2, dst.y + dst.height / 2, { steps: 12 })
  await page.mouse.up()

  const route = page.locator('.react-flow__edge[data-id^="route:"]').first()
  await expect(route).toBeVisible()

  // Click exactly ON the painted path (an edge's bounding-box center can sit
  // off a curved path, so aim at the path's midpoint in screen space).
  const pt = await route.locator('.react-flow__edge-path').first().evaluate((p) => {
    const path = p as SVGPathElement
    const mid = path.getPointAtLength(path.getTotalLength() / 2)
    const ctm = path.getScreenCTM()
    if (!ctm) throw new Error('no CTM')
    const sp = new DOMPoint(mid.x, mid.y).matrixTransform(ctm)
    return { x: sp.x, y: sp.y }
  })
  await page.mouse.click(pt.x, pt.y)

  // The selection is real AND visible (B8's second half: the stroke used to
  // swallow the selected state).
  await expect(page.locator('.react-flow__edge.selected[data-id^="route:"]')).toHaveCount(1)
}

// Leave the fixture pristine for the next spec (the channel is still a
// pending edit): discard everything.
async function discardAll(page: Page) {
  await page.getByRole('button', { name: /descartar/i }).click()
  await expect(page.getByText(/no changes/i)).toBeVisible()
}

test('a cabled route is selectable on click and deleted with Backspace', async ({ page }) => {
  await openCanvas(page)
  await cableAndSelectRoute(page)

  await page.keyboard.press('Backspace')
  await expect(page.locator('.react-flow__edge[data-id^="route:"]')).toHaveCount(0)

  await discardAll(page)
})

test('B8-bis: the selected cable offers the ✕ and a mouse click deletes it', async ({ page }) => {
  await openCanvas(page)
  await cableAndSelectRoute(page)

  // The sealed option (a): the ✕ floats at the midpoint of the SELECTED
  // cable only, and clicking it is the mouse twin of Backspace.
  const del = page.getByRole('button', { name: 'Eliminar conexión' })
  await expect(del).toBeVisible()
  await del.click()
  await expect(page.locator('.react-flow__edge[data-id^="route:"]')).toHaveCount(0)
  await expect(del).toHaveCount(0)

  await discardAll(page)
})
