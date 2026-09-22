// Captures of the AUTORIDAD block for the v0.16.0 public surfaces (README and
// website). NOT a guarantee test: it asserts only what it needs to know that it
// photographed the right thing, and it writes files.
//
// The request it photographs is parked through ParkAuthorization — the door
// production parks through — over an authority the harness builds with the
// store's exported doors, so the requester, the intent, the principal chain and
// the remainder in the picture are what the STORE established and signed, not
// strings this spec posted. The same screen the packaged app serves: one
// `dist`, one chrome.
//
// The theme is chosen the way the product chooses it — `data-theme` from the
// stored choice — because emulateMedia does not move it here (the 2026-09-13
// paper, pass 1).
import { expect, test } from '@playwright/test'
import type { Page } from '@playwright/test'
import { installBindings } from './bindings'
import { APPROVALS_BASE } from './util'

// WRITES INTO THE TRACKED TREE, and therefore only when asked. Every other
// screenshot spec here writes through SHOT() into the gitignored
// design-drafts/; these two PNGs are published by the README, so a plain
// `npx playwright test` — or the CI e2e check — must not silently re-render
// them with another machine's fonts. Set KORVUN_CAPTURES=1 to take them.
const OUT = '../../../docs/assets/captures/v0.16.0'
const TAKE = process.env.KORVUN_CAPTURES === '1'

interface Parked {
  approval_id: string
  action_id: string
  digest: string
}

async function boot(page: Page): Promise<void> {
  await installBindings(page)
  await page.goto(APPROVALS_BASE + '/')
  // The harness opens on a STOPPED core (-start=false), and a stopped core has
  // registered no identity: the park would refuse the owner's own act as
  // unreadable evidence. Start it the way the other specs do, through the
  // bindings bridge.
  await page.request
    .post(APPROVALS_BASE + '/__test/bindings/Start', { data: [] })
    .catch(() => undefined)
}

test('AUTORIDAD block, light and dark, for the public surfaces', async ({ page }) => {
  test.skip(!TAKE, 'capture run only: set KORVUN_CAPTURES=1')
  await page.setViewportSize({ width: 1100, height: 820 })
  await boot(page)
  const res = await page.request.post(APPROVALS_BASE + '/__test/park', {
    data: {
      purpose: 'Avisar al webhook de pedidos cuando el equipo lo pida',
      authority: {
        intent_id: 'int_pedidos',
        intent_purpose: 'Avisar al webhook de pedidos cuando el equipo lo pida',
        budget: { kind: 'finite', total: 3 },
        spend_after_park: 0,
      },
    },
  })
  expect(res.ok(), `park failed: ${res.status()} ${await res.text()}`).toBe(true)
  const parked = (await res.json()) as Parked

  await page.getByRole('button', { name: 'Aprobaciones' }).click()
  await page.getByRole('button', { name: new RegExp(parked.approval_id) }).click()
  await expect(page.getByTestId('approval-parameters')).toBeVisible()

  const authority = page.getByTestId('approval-authority')
  await expect(authority).toBeVisible()
  // What the picture must show, so a caption cannot outrun it.
  await expect(authority).toContainText('int_pedidos')
  await expect(authority).toContainText('máximo 3 inicios')

  for (const theme of ['light', 'dark'] as const) {
    await page.evaluate((t) => {
      localStorage.setItem('korvun.chrome.theme', t)
      document.documentElement.dataset.theme = t
    }, theme)
    await expect
      .poll(() => page.evaluate(() => getComputedStyle(document.body).backgroundColor))
      .toBe(theme === 'light' ? 'rgb(250, 250, 252)' : 'rgb(15, 15, 22)')
    await authority.screenshot({ path: `${OUT}/authority-block-${theme}.png` })
  }
})
