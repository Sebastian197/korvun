// E2E of the 2026-08-09 pegas 1-3 flow over the REAL pipeline (harness core):
// send → the user's turn echoes IMMEDIATELY on the OWN side → Thinking… →
// the real pair reconciles with no duplicates.
import { expect, test } from '@playwright/test'
import { installBindings } from './bindings'

const SHOT = (name: string): string => `../../../design-drafts/console/${name}`

test('direct chat echoes instantly, thinks honestly, reconciles cleanly', async ({ page }) => {
  await installBindings(page)
  await page.goto('/')
  await page.request.post('/__test/bindings/Start', { data: [] }).catch(() => undefined)
  await page.getByRole('button', { name: 'Chat' }).click()
  await page.getByRole('button', { name: 'New chat' }).click()

  const pane = page.locator('.console')
  const chatBox = page.getByRole('textbox', { name: /message korvun/i })
  await chatBox.fill('eco inmediato de prueba')
  await chatBox.press('Enter')

  // PEGA 2: the echo is on stage at once — no waiting for the brain.
  const echo = pane.locator('.console-turn-content', { hasText: 'eco inmediato de prueba' })
  await expect(echo.first()).toBeVisible({ timeout: 2_000 })

  // PEGA 3: the user's turn sits on the OWN (right, violet) side.
  const ownTurn = pane.locator('.console-turn[data-side="own"]', {
    hasText: 'eco inmediato de prueba',
  })
  await expect(ownTurn.first()).toBeVisible({ timeout: 2_000 })
  await page.screenshot({ path: SHOT('11-echo-immediate-own-side.png'), fullPage: true })

  // PEGA 4-UI: the honest wait shows from the send instant (auto-wait law).
  // The harness's fake model may answer fast — accept either the caption or
  // an already-arrived assistant reply, but never neither.
  const thinking = page.getByText(/thinking…/i)
  const reply = pane.locator('[data-role="assistant"]').first()
  await expect(thinking.or(reply)).toBeVisible({ timeout: 5_000 })

  // Reconciliation: once the real pair lands, EXACTLY ONE copy of the turn.
  await expect(reply).toBeVisible({ timeout: 15_000 })
  await expect(
    pane.locator('.console-turn-content', { hasText: 'eco inmediato de prueba' }),
  ).toHaveCount(1)
  await page.screenshot({ path: SHOT('12-reconciled-no-duplicates.png'), fullPage: true })

  // Leave no state behind: the suite shares ONE harness core, so this spec
  // deletes its conversation (real deletion, no undo) before handing over.
  await page.getByRole('button', { name: 'Delete conversation' }).click()
  await page.getByRole('button', { name: /^Delete$/ }).click()
  await expect(pane.getByText('No conversations yet.')).toBeVisible({ timeout: 10_000 })
})
