// SP6c onboarding e2e (the FRESH-install harness on its own origin): no
// config loaded, so EnsureDefaultConfig's created=true is REAL and the
// onboarding mounts. The model check probes the machine's real Ollama port
// (127.0.0.1:11434), so EITHER honest outcome is valid — the contract this
// test locks is that a result appears ONLY after the click (never a fake
// unchecked success). The full channel/Start path is covered by the unit
// tests (bindings mocked) since CI has no Ollama.
import { expect, test } from '@playwright/test'
import { installBindings } from './bindings'
import { FRESH_BASE, SHOT, settleFonts } from './util'

// fresh-reset deletes the config under the fresh harness's temp HOME so
// created=true is re-establishable — the onboarding mounts on every attempt,
// not just the harness process's first (review finding: one-shot fresh state
// defeated the Playwright retry budget).
test.beforeEach(async ({ request }) => {
  await request.post(`${FRESH_BASE}/__test/fresh-reset`, { data: [] })
})

test('Onboarding primera vez: el chequeo de modelo es honesto', async ({ page }) => {
  await installBindings(page)
  await page.goto(`${FRESH_BASE}/`)
  await expect(page.getByTestId('onboarding')).toBeVisible({ timeout: 15000 })
  await expect(page.getByText('Bienvenido a Korvun')).toBeVisible()
  // No result string before the click — the check is real, never faked.
  await expect(page.getByText(/listo|Ollama no responde/)).toHaveCount(0)
  await page.getByRole('button', { name: /Comprobar modelo/ }).click()
  await expect(page.getByText(/listo|Ollama no responde/)).toBeVisible({ timeout: 10000 })
  await settleFonts(page)
  await page.screenshot({ path: SHOT('sp6c-onboarding.png'), animations: 'disabled' })
})
