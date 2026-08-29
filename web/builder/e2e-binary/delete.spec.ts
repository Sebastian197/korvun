import { test, expect, type Page } from '@playwright/test'
import { resetToFixture } from './reset'

// Deflake tanda (2026-08-29, patient a): every spec starts from the
// pristine fixture — the shared live core persists applied configs, and
// order-dependent leftovers were the root cause of the edges failures.
test.beforeEach(async ({ page }) => {
  await resetToFixture(page)
})

// SP5 RED (builder-canvas, e2e against the REAL binary): node deletion from
// the canvas reaches the served config, AND the page loads with a CLEAN
// console — zero CSP violations, zero 404s (the SP0 spike criterion returns as
// a PERMANENT guardian, now over the production canvas). RED today: the panel
// has no "Eliminar nodo…", the select chevron's data-URI trips CSP, and
// /favicon.ico 404s.
const TOKEN = 'e2e-canvas-admin'

async function openCanvas(page: Page) {
  await page.goto('/builder/')
  await page.getByLabel('admin bearer token').fill(TOKEN)
  await page.getByRole('button', { name: 'Load' }).click()
  await expect(page.getByTestId('canvas-surface')).toBeVisible({ timeout: 10_000 })
}

test('a. create+apply a brain → delete it from the panel → apply → the GET confirms it is gone', async ({
  page,
}) => {
  await openCanvas(page)

  // The e2e-binary suite shares ONE server whose config other specs mutate, so
  // the new brain's index = the current brain count (not a fixed brain:1).
  const n = await page.locator('[data-kind="brain"]').count()
  const nid = `brain:${n}`

  // Create + name a brain and APPLY it, so it genuinely exists in the served
  // config (create-then-delete in one shot would be a net-zero edit — Aplicar
  // would stay disabled, nothing to persist).
  await page.dragAndDrop('[data-testid="palette:brain"]', '[data-testid="canvas-surface"]')
  await page.getByTestId(nid).click()
  await page.getByLabel('name', { exact: true }).fill('efimero')
  // A brain needs >=1 model with a model_id to pass Validate (validateModels).
  await page.dragAndDrop('[data-testid="palette:model"]', `[data-testid="${nid}"]`)
  await page.getByTestId(`model:${n}.0`).click()
  await page.getByLabel('model_id').fill('llama3.2:1b')
  await page.getByRole('button', { name: /aplicar/i }).click()
  await expect(page.getByTestId('reload-succeeded')).toBeVisible({ timeout: 30_000 })

  const created = await (
    await page.request.get('/api/config', { headers: { Authorization: `Bearer ${TOKEN}` } })
  ).json()
  expect((created as { brains: Array<{ name: string }> }).brains.map((b) => b.name)).toContain(
    'efimero',
  )

  // Now DELETE it from the panel (with the confirmation gate) and apply again.
  // After the apply re-baselined, the appended brain is still the last index.
  await page.getByTestId(nid).click()
  await page.getByRole('button', { name: /eliminar nodo/i }).click()
  await page.getByRole('button', { name: /sí, eliminar|confirmar/i }).click()
  await page.getByRole('button', { name: /aplicar/i }).click()
  // Deflake tanda (2026-08-29): the FIRST apply's succeeded chip can still be
  // on screen when this wait runs, so the chip is an ambiguous signal for a
  // SECOND cycle — the race behind the cold-run delete failures. Wait on the
  // truth itself: the served config, polled through the real control API.
  await expect
    .poll(
      async () => {
        const r = await page.request.get('/api/config', {
          headers: { Authorization: `Bearer ${TOKEN}` },
        })
        if (!r.ok()) return ['(admin restarting)']
        return ((await r.json()) as { brains: Array<{ name: string }> }).brains.map((b) => b.name)
      },
      { timeout: 30_000 },
    )
    .not.toContain('efimero')
  await expect(page.getByTestId('reload-succeeded')).toBeVisible({ timeout: 30_000 })
})

test('b. the canvas loads with a CLEAN console — zero CSP violations, zero 404s (guardian)', async ({
  page,
}) => {
  const cspViolations: string[] = []
  const notFound: string[] = []
  await page.addInitScript(() => {
    ;(window as unknown as { __csp: string[] }).__csp = []
    document.addEventListener('securitypolicyviolation', (e) => {
      ;(window as unknown as { __csp: string[] }).__csp.push(
        `${e.violatedDirective} blocked ${e.blockedURI}`,
      )
    })
  })
  page.on('response', (r) => {
    if (r.status() === 404) notFound.push(r.url())
  })

  await openCanvas(page)
  // Open a node panel so the <select> chevrons render (the data-URI path).
  await page.getByTestId('brain:0').click()
  await page.waitForTimeout(400)

  cspViolations.push(
    ...(await page.evaluate(() => (window as unknown as { __csp: string[] }).__csp)),
  )
  expect(cspViolations, `CSP violations: ${cspViolations.join(', ')}`).toEqual([])
  expect(notFound, `404s: ${notFound.join(', ')}`).toEqual([])
})
