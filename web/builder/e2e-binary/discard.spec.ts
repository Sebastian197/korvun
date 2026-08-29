import { test, expect, type Page } from '@playwright/test'
import { resetToFixture } from './reset'

// Deflake tanda (2026-08-29, patient a): every spec starts from the
// pristine fixture — the shared live core persists applied configs, and
// order-dependent leftovers were the root cause of the edges failures.
test.beforeEach(async ({ page }) => {
  await resetToFixture(page)
})

// SP6 RED (builder-canvas, 2a): Descartar against the REAL binary — compose
// changes, discard, the UI returns to the applied state AND the served config
// never changed. RED today: there is no Descartar on the canvas.
const TOKEN = 'e2e-canvas-admin'

async function openCanvas(page: Page) {
  await page.goto('/builder/')
  await page.getByLabel('admin bearer token').fill(TOKEN)
  await page.getByRole('button', { name: 'Load' }).click()
  await expect(page.getByTestId('canvas-surface')).toBeVisible({ timeout: 10_000 })
}

// Relative counting (the e2e-binary suite shares ONE server whose config the
// other specs mutate, so absolute node indices are not stable across the run).
test('compose changes → Descartar → the UI reverts and the GET is unchanged', async ({ page }) => {
  await openCanvas(page)

  const before = await (
    await page.request.get('/api/config', { headers: { Authorization: `Bearer ${TOKEN}` } })
  ).text()
  const brains = page.locator('[data-kind="brain"]')
  const n = await brains.count()

  // Compose real changes (a new brain node), NOT applied.
  await page.dragAndDrop('[data-testid="palette:brain"]', '[data-testid="canvas-surface"]')
  await expect(brains).toHaveCount(n + 1)
  await expect(page.getByText(/cambios? sin aplicar/i)).toBeVisible()

  // Discard → the composed node reverts and the counter clears.
  await page.getByRole('button', { name: /descartar/i }).click()
  await expect(brains).toHaveCount(n)
  await expect(page.getByText(/cambios? sin aplicar/i)).toHaveCount(0)

  // The served config never changed (discard never POSTs).
  const after = await (
    await page.request.get('/api/config', { headers: { Authorization: `Bearer ${TOKEN}` } })
  ).text()
  expect(after).toBe(before)
})
