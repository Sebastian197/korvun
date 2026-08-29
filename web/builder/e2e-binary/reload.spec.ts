import { test, expect, type Page } from '@playwright/test'
import { resetToFixture } from './reset'

// Deflake tanda (2026-08-29, patient a): every spec starts from the
// pristine fixture — the shared live core persists applied configs, and
// order-dependent leftovers were the root cause of the edges failures.
test.beforeEach(async ({ page }) => {
  await resetToFixture(page)
})

// v0.9.2 (B7, bug-bash 2026-08-23): a reload the log proved APPLIED (cutover
// 09:40:44, admin 56707→56799) was painted as "reload failed — the running
// config is unchanged". This spec drives the EXACT incident shape against the
// real binary: change an existing brain's model_id, apply, and race the two
// terminal renders — the failure banner must never win a happy cutover. The
// admin server tears down and rebinds mid-poll (the real reload machine), so
// the poll's cutover survival is exercised for real, not mocked.

const TOKEN = 'e2e-canvas-admin'

async function openCanvas(page: Page) {
  await page.goto('/builder/')
  await page.getByLabel('admin bearer token').fill(TOKEN)
  await page.getByRole('button', { name: 'Load' }).click()
  await expect(page.getByTestId('canvas-surface')).toBeVisible({ timeout: 10_000 })
}

test('a model_id change applies through the cutover without a phantom failure banner', async ({
  page,
}) => {
  await openCanvas(page)

  // The incident's gesture: retarget an EXISTING brain's model (no structural
  // change, config stays valid — a happy cutover by construction).
  await page.getByTestId('model:0.0').click()
  const modelField = page.getByLabel('model_id')
  await modelField.fill(`llama3.2:1b-b7-${Date.now() % 100000}`)

  await page.getByRole('button', { name: /aplicar/i }).click()

  // Race the terminal renders: failed/rolled-back (reload-terminal) is a
  // terminal phase, so if it ever paints, succeeded can never arrive — and
  // vice versa. Waiting for whichever appears first pins the contract.
  const winner = page.locator('[data-testid="reload-succeeded"], [data-testid="reload-terminal"]')
  await winner.first().waitFor({ state: 'visible', timeout: 30_000 })
  await expect(
    page.getByTestId('reload-terminal'),
    'phantom failure banner on a happy cutover (B7)',
  ).toHaveCount(0)
  await expect(page.getByTestId('reload-succeeded')).toBeVisible()

  // The reload was REAL: the served config carries the new model_id.
  const resp = await page.request.get('/api/config', {
    headers: { Authorization: `Bearer ${TOKEN}` },
  })
  expect(resp.ok()).toBe(true)
  const cfg = (await resp.json()) as {
    brains: Array<{ models: Array<{ model_id: string }> }>
  }
  expect(cfg.brains[0]?.models[0]?.model_id).toContain('-b7-')
})
