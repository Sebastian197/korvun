// The eleven browser scenarios of the approvals screen — the ones the jsdom
// suite cannot hold because they need real focus, real layout and real keys.
//
// Everything here runs against the REAL pipeline: the harness's own core, its
// own store, and an approval parked through POST /__test/park, which goes in
// by the real factory (envelope, bound request, sealed preview, canonical
// params). No fixture is hand-written into the database: a row inserted by
// hand would sail past the belts this screen exists to surface.
import { expect, test } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'
import { installBindings } from './bindings'
import { APPROVALS_BASE } from './util'

const SHOT = (name: string): string => `../../../design-drafts/approvals/${name}`

interface Parked {
  approval_id: string
  action_id: string
  digest: string
}

/** Boots the chrome on a running core with ONE request parked, and opens it. */
async function openOneParked(page: import('@playwright/test').Page): Promise<Parked> {
  await installBindings(page)
  await page.goto(APPROVALS_BASE + '/')
  await page.request
    .post(APPROVALS_BASE + '/__test/bindings/Start', { data: [] })
    .catch(() => undefined)
  const res = await page.request.post(APPROVALS_BASE + '/__test/park', { data: {} })
  expect(res.ok(), `park failed: ${res.status()} ${await res.text()}`).toBe(true)
  const parked = (await res.json()) as Parked
  await page.getByRole('button', { name: 'Aprobaciones' }).click()
  await page
    .getByRole('button', { name: /tool\/webhook_call/ })
    .first()
    .click()
  await expect(page.getByTestId('approval-parameters')).toBeVisible()
  return parked
}

const approve = (page: import('@playwright/test').Page) =>
  page.getByRole('button', { name: 'Aprobar y ejecutar' })
const reject = (page: import('@playwright/test').Page) =>
  page.getByRole('button', { name: 'Rechazar' })

// AS-2 — the buttons live BELOW the parameters block on the page. The operator
// reads what will run before he can reach the control that runs it.
test('AS-2 · the decision block sits below the parameters, at 1100x760', async ({ page }) => {
  await page.setViewportSize({ width: 1100, height: 760 })
  await openOneParked(page)
  const params = await page.getByTestId('approval-parameters').boundingBox()
  const yes = await approve(page).boundingBox()
  expect(params, 'parameters box').not.toBeNull()
  expect(yes, 'approve box').not.toBeNull()
  expect(yes!.y).toBeGreaterThan(params!.y + params!.height - 1)
  await page.screenshot({ path: SHOT('as2-decision-below-parameters.png'), fullPage: true })
})

// AS-51 — Tab from the reason reaches Rechazar, then the arming field, then
// Aprobar, and Aprobar is LAST. The keyboard order is the same ladder the eye
// walks: the cheap control first, the expensive one behind a typed gate.
test('AS-51 · tab order: reason, Rechazar, arming, Aprobar — and Aprobar is last', async ({
  page,
}) => {
  const parked = await openOneParked(page)
  // Aprobar is DISABLED until the tail is typed, and a disabled button is not
  // in the tab ring at all — correctly so, per §7. So the order is only
  // observable on an ARMED document; testing it unarmed would prove that Tab
  // skips a control nobody can press, which is not the guarantee.
  const gate = page.getByLabel(/reteclea los seis últimos caracteres del digest/i)
  await gate.focus()
  for (const ch of parked.digest.slice(-6)) await page.keyboard.press(ch)
  await expect(approve(page)).toBeEnabled()

  const reason = page.getByRole('textbox', { name: /motivo/i })
  await reason.focus()
  const seen: string[] = []
  for (let i = 0; i < 3; i++) {
    await page.keyboard.press('Tab')
    seen.push(
      await page.evaluate(() => {
        const el = document.activeElement as HTMLElement | null
        if (el === null) return 'none'
        // An <input> carries neither aria-label nor text: its name is its
        // associated <label>, which is what a screen reader announces too.
        const label =
          el.id !== '' ? (document.querySelector(`label[for="${el.id}"]`)?.textContent ?? '') : ''
        return (el.getAttribute('aria-label') ?? '')
          .concat(' ', label, ' ', el.textContent ?? '')
          .trim()
          .slice(0, 60)
      }),
    )
  }
  expect(seen[0]).toMatch(/Rechazar/)
  expect(seen[1]).toMatch(/digest|reteclea/i)
  expect(seen[2]).toMatch(/Aprobar/)
})

// AS-52 — area(Aprobar) <= 0.50 * area(Rechazar). The cheap door is the big
// one; the expensive door is deliberately small.
test('AS-52 · the approve button is at most half the reject button, at 1100x760', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1100, height: 760 })
  await openOneParked(page)
  await page.getByLabel(/reteclea los seis últimos caracteres del digest/i).focus()
  const yes = await approve(page).boundingBox()
  const no = await reject(page).boundingBox()
  expect(yes).not.toBeNull()
  expect(no).not.toBeNull()
  const aYes = yes!.width * yes!.height
  const aNo = no!.width * no!.height
  console.log(`AS-52 measured: approve=${aYes.toFixed(1)}px2 reject=${aNo.toFixed(1)}px2`)
  expect(aYes).toBeLessThanOrEqual(0.5 * aNo)
})

// AS-53 — horizontal separation >= 320 px, and the mould PRINTS the measured
// box: the published number is what the browser measured, never what §7
// computed on paper.
test('AS-53 · horizontal separation is at least 320 px, and the measurement is printed', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1100, height: 760 })
  await openOneParked(page)
  await page.getByLabel(/reteclea los seis últimos caracteres del digest/i).focus()
  const yes = await approve(page).boundingBox()
  const no = await reject(page).boundingBox()
  expect(yes).not.toBeNull()
  expect(no).not.toBeNull()
  const gap = yes!.x - (no!.x + no!.width)
  console.log(
    `AS-53 measured: reject=${JSON.stringify(no)} approve=${JSON.stringify(yes)} gap=${gap.toFixed(1)}px`,
  )
  expect(gap).toBeGreaterThanOrEqual(320)
})

// AS-54 — Rechazar precedes Aprobar in the DOM and on the X axis. Both, because
// a screen reader walks one and the hand walks the other.
test('AS-54 · Rechazar precedes Aprobar in the DOM and on the X axis', async ({ page }) => {
  await page.setViewportSize({ width: 1100, height: 760 })
  await openOneParked(page)
  await page.getByLabel(/reteclea los seis últimos caracteres del digest/i).focus()
  const order = await page.evaluate(() => {
    const btns = Array.from(document.querySelectorAll('button'))
    const no = btns.findIndex((b) => (b.textContent ?? '').includes('Rechazar'))
    const yes = btns.findIndex((b) => (b.textContent ?? '').includes('Aprobar y ejecutar'))
    return { no, yes }
  })
  expect(order.no).toBeGreaterThanOrEqual(0)
  expect(order.yes).toBeGreaterThan(order.no)
  const yesBox = await approve(page).boundingBox()
  const noBox = await reject(page).boundingBox()
  expect(noBox!.x).toBeLessThan(yesBox!.x)
})

// AS-55 — at 900x700 they stack, Rechazar first, with at least 120 px between
// them. The narrow window keeps the distance the wide one buys horizontally.
test('AS-55 · at 900x700 they stack, Rechazar first, at least 120 px apart', async ({ page }) => {
  await page.setViewportSize({ width: 900, height: 700 })
  await openOneParked(page)
  await page.getByLabel(/reteclea los seis últimos caracteres del digest/i).focus()
  const yes = await approve(page).boundingBox()
  const no = await reject(page).boundingBox()
  const gap = yes!.y - (no!.y + no!.height)
  console.log(
    `AS-55 measured: reject=${JSON.stringify(no)} approve=${JSON.stringify(yes)} vertical gap=${gap.toFixed(1)}px`,
  )
  expect(no!.y).toBeLessThan(yes!.y)
  expect(gap).toBeGreaterThanOrEqual(120)
})

// AS-63 — axe AA in BOTH themes. The document is the one place in the product
// where a contrast failure costs an irreversible effect.
test('AS-63 · axe AA in light and dark', async ({ page }) => {
  await openOneParked(page)
  for (const theme of ['light', 'dark'] as const) {
    await page.emulateMedia({ colorScheme: theme })
    const results = await new AxeBuilder({ page })
      .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'])
      .analyze()
    expect(
      results.violations,
      `${theme}: ${results.violations.map((v) => `${v.id} (${v.nodes.length})`).join(', ')}`,
    ).toEqual([])
  }
})

// AS-71 — [Volver a leer] leaves the container scroll at 0. A re-read that kept
// the old offset would show the operator a different part of a NEW document
// while he believes he is looking at the same one.
test('AS-71 · a re-read leaves the document scrolled to the top', async ({ page }) => {
  await openOneParked(page)
  const doc = page.locator('.approvals-doc')
  await doc.evaluate((el) => {
    el.scrollTop = el.scrollHeight
  })
  expect(await doc.evaluate((el) => el.scrollTop)).toBeGreaterThan(0)
  await page.getByRole('button', { name: 'Volver a leer la petición' }).click()
  await expect(page.getByTestId('approval-parameters')).toBeVisible()
  expect(await doc.evaluate((el) => el.scrollTop)).toBe(0)
})

// AS-73 — the browser's own autofill over the arming field. The field stays
// empty and Aprobar stays disabled: the arming proves a HAND typed the tail,
// and anything the browser fills in proves nothing about a hand.
test('AS-73 · browser autofill does not arm the approval', async ({ page }) => {
  await openOneParked(page)
  const field = page.getByLabel(/reteclea los seis últimos caracteres del digest/i)
  // What autofill does: it sets .value and fires `input`, never a key event.
  await field.evaluate((el) => {
    const input = el as HTMLInputElement
    const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')?.set
    setter?.call(input, 'abc123')
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })
  await expect(field).toHaveValue('')
  await expect(approve(page)).toBeDisabled()
})

// ---------------------------------------------------------------------------
// The two FR-UI-63 branches, and what they were really asserting.
//
// Both specs asserted ONLY absences — toHaveCount(0) four times between them —
// which the sixth adversarial pass caught: replacing either branch with an
// empty fragment left them green over a blank screen.
//
// Driving that cure found the worse half. Every spec above starts the shared
// harness core through openOneParked, so by the time these two ran the door
// answered 200 and the screen painted the INBOX. Neither test ever reached the
// branch its name promises; the four absences held because the sentences live
// in a component that was not on the page at all.
//
// So each one now stops the core first, and asserts what its branch must PAINT.
// ---------------------------------------------------------------------------

/** Stops the shared harness core so the approvals door answers its real 503. */
async function stopTheCore(page: import('@playwright/test').Page): Promise<void> {
  const res = await page.request.post(APPROVALS_BASE + '/__test/bindings/Stop', { data: [] })
  expect(res.ok(), `Stop failed: ${res.status()}`).toBe(true)
}

// AS-79 — a 503 «core stopped» while Status() says Running = true. The two
// disagree, and the screen believes the DOOR, not the binding: it prints the
// third branch of FR-UI-63 and never offers [Arrancar el núcleo], because
// starting a core that says it is already running fixes nothing.
test('AS-79 · a 503 against a Running=true binding paints the honest branch', async ({ page }) => {
  await stopTheCore(page)
  await installBindings(page)
  await page.addInitScript(() => {
    const w = window as unknown as { go: { shell: { Desktop: Record<string, unknown> } } }
    w.go.shell.Desktop.Status = () =>
      Promise.resolve({
        Running: true,
        ConfigPath: '/p/korvun.json',
        AdminAddr: '',
        TokenEnv: 'KORVUN_ADMIN_TOKEN',
      })
  })
  await page.goto(APPROVALS_BASE + '/')
  // The core is NOT started, so the proxy answers the real 503.
  await page.getByRole('button', { name: 'Aprobaciones' }).click()
  // What it MUST paint. The first shape of this test asserted only the two
  // absences, so replacing this whole branch with an empty fragment left it
  // green over a blank screen — no title, no explanation, no way out.
  await expect(page.getByText('El proceso está en marcha', { exact: false })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Reintentar' })).toBeVisible()
  // And what it must NOT.
  await expect(page.getByText('El núcleo está parado')).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Arrancar el núcleo' })).toHaveCount(0)
})

// AS-99 — Status() failing, or no bindings at all. The third branch again, and
// neither of the two confident sentences may appear: the window does not know
// what the process is doing and says so.
test('AS-99 · with no bindings the screen claims nothing about the process', async ({ page }) => {
  await stopTheCore(page)
  await page.goto(APPROVALS_BASE + '/')
  await page.getByRole('button', { name: 'Aprobaciones' }).click()
  // Same lesson as AS-79: the branch has to be THERE, saying the one thing it
  // is entitled to say.
  await expect(
    page.getByText('tampoco ha podido preguntar al núcleo en qué estado está', {
      exact: false,
    }),
  ).toBeVisible()
  await expect(page.getByRole('button', { name: 'Reintentar' })).toBeVisible()
  await expect(page.getByText('El núcleo está parado')).toHaveCount(0)
  await expect(page.getByText('El proceso está en marcha')).toHaveCount(0)
})

// The two above leave the harness core stopped on purpose. Anything added after
// them must start it itself — openOneParked already does.
