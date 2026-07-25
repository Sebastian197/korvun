// SP6c e2e (the running-core harness): the wizard end-to-end (keychain
// double via the harness memSecrets + POST + reload), the Builder same-origin
// iframe, the "Entendido" dismiss, and Canales — REAL pipeline, 1440x900,
// captures into design-drafts/ for the copilot's side-by-side review.
// axe-core over the new views. Serial: the harness core is shared state.
import { AxeBuilder } from '@axe-core/playwright'
import { expect, test, type Page } from '@playwright/test'
import { installBindings } from './bindings'
import { BASE, SHOT, settleFonts } from './util'

test.describe.configure({ mode: 'serial' })

async function ensureRunning(page: Page): Promise<void> {
  await page.request.post(`${BASE}/__test/bindings/Start`, { data: [] }).catch(() => undefined)
  await expect(page.getByTestId('healthz-badge')).toContainText('OK', { timeout: 15000 })
}

async function post(page: Page, path: string, body?: unknown): Promise<void> {
  const resp = await page.request.post(BASE + path, { data: body ?? [] })
  if (resp.status() >= 400) throw new Error(`${path} -> ${resp.status()}: ${await resp.text()}`)
}

// Each test (and each Playwright retry) starts from the known one-telegram
// config — no test-body cleanup that a mid-flight failure would skip (review
// finding). reset-config stops the core and restores the pristine config.
test.beforeEach(async ({ request }) => {
  await request.post(`${BASE}/__test/model`, { data: { mode: 'ok' } })
  await request.post(`${BASE}/__test/channel`, { data: { send: 'ok' } })
  await request.post(`${BASE}/__test/reset-config`, { data: [] })
})

test('Canales: lista con salud real + detalle honesto', async ({ page }) => {
  await installBindings(page)
  await page.goto('/')
  await ensureRunning(page)
  await page.getByRole('button', { name: 'Canales' }).click()

  await expect(page.getByText('Telegram').first()).toBeVisible()
  await expect(page.getByText(/polling · KORVUN_HARNESS_TELEGRAM_TOKEN/)).toBeVisible()
  await expect(page.getByText('Operativo')).toBeVisible()
  await settleFonts(page)
  await page.screenshot({ path: SHOT('sp6c-canales-lista.png'), animations: 'disabled' })

  // Detail: route chip + the change-in-builder link + secret honesty note.
  await page
    .getByRole('button', { name: /Telegram/ })
    .first()
    .click()
  await expect(page.getByText(/cambiar en el Builder/)).toBeVisible()
  await expect(page.getByText(/El valor vive en tu entorno/)).toBeVisible()
  await settleFonts(page)
  await page.screenshot({ path: SHOT('sp6c-canales-detalle.png'), animations: 'disabled' })

  const axe = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa']).analyze()
  expect(axe.violations, JSON.stringify(axe.violations, null, 2)).toEqual([])
})

test('El asistente en 3 pasos: llavero + Comprobar entorno + POST + reload', async ({ page }) => {
  await installBindings(page)
  await page.goto('/')
  await ensureRunning(page)
  await page.getByRole('button', { name: 'Canales' }).click()
  await expect(page.getByText('Telegram').first()).toBeVisible() // snapshot loaded
  await page.getByRole('button', { name: /Añadir canal/ }).click()

  // Step 1: telegram is taken (harness config) → disabled; pick discord.
  // Scope to the wizard dialog (the Canales list behind it also has buttons).
  const wiz = page.getByRole('dialog', { name: 'Añadir un canal' })
  await expect(wiz.getByRole('button', { name: /Telegram/ })).toBeDisabled()
  await wiz.getByRole('button', { name: /Discord/ }).click()
  await wiz.getByRole('button', { name: /Siguiente/ }).click()
  await page.screenshot({ path: SHOT('sp6c-wizard-paso1.png'), animations: 'disabled' })
  await wiz.getByRole('button', { name: /Siguiente/ }).click()

  // Step 3: the masked field → SetSecret (harness memSecrets, the keychain
  // double) → Comprobar entorno resolves it in the keychain (presence, never
  // the value).
  await wiz.getByPlaceholder(/una sola vez/).fill('e2e-discord-token-value')
  await wiz.getByRole('button', { name: /Guardar en el llavero/ }).click()
  await expect(wiz.getByText(/guardado en el llavero/)).toBeVisible()
  await wiz.getByRole('button', { name: /Comprobar entorno/ }).click()
  await expect(wiz.getByText(/DISCORD_BOT_TOKEN (en el llavero|detectada)/)).toBeVisible()
  await settleFonts(page)
  await page.screenshot({ path: SHOT('sp6c-wizard-paso3-llavero.png'), animations: 'disabled' })

  // Finish → POST /api/config + reload → the channel appears.
  await wiz.getByRole('button', { name: /Conectar canal/ }).click()
  await expect(page.getByRole('dialog')).toBeHidden({ timeout: 15000 })
  await expect(page.getByText('Discord').first()).toBeVisible({ timeout: 15000 })

  // The secret value never reached localStorage. Cleanup is beforeEach's job
  // (reset-config), so a failure here never pollutes the next test/retry.
  const ls = await page.evaluate(() => JSON.stringify(localStorage))
  expect(ls).not.toContain('e2e-discord-token-value')
})

test('Builder: iframe same-origin a /builder/ con el core en marcha', async ({ page }) => {
  await installBindings(page)
  await page.goto('/')
  await ensureRunning(page)
  await page.getByRole('button', { name: 'Builder' }).click()
  const frame = page.getByTitle('Builder')
  await expect(frame).toHaveAttribute('src', '/builder/')
  // The iframe genuinely loads same-origin (the frame-ancestors 'self' change):
  // its document resolves, not a CSP refusal.
  await expect(async () => {
    const doc = await frame.contentFrame()
    expect(doc).not.toBeNull()
    await expect(doc!.locator('body')).toBeAttached()
  }).toPass({ timeout: 15000 })
  await settleFonts(page)
  await page.screenshot({ path: SHOT('sp6c-builder.png'), animations: 'disabled' })
})

test('Builder: con el core parado pinta el estado honesto, jamás un iframe roto', async ({
  page,
}) => {
  await installBindings(page)
  await page.goto('/')
  await ensureRunning(page)
  await page.getByRole('button', { name: 'Builder' }).click()
  await page.request.post(`${BASE}/__test/core-exit`, { data: [] })
  await expect(page.getByTestId('builder-stopped')).toBeVisible({ timeout: 10000 })
  await expect(page.getByTitle('Builder')).toBeHidden()
})

test('Incidencia de evento: "Entendido" despeja el banner', async ({ page }) => {
  await installBindings(page)
  await page.goto('/')
  await ensureRunning(page)
  await page.getByRole('button', { name: 'Actividad' }).click()
  await expect(page.getByText('En vivo')).toBeVisible({ timeout: 10000 })
  await page.getByRole('button', { name: 'Inicio' }).click()
  await post(page, '/__test/channel', { send: 'fail' })
  await post(page, '/__test/inject', { text: 'este se pierde' })
  await expect(page.getByText('En marcha — incidencia')).toBeVisible({ timeout: 10000 })
  await settleFonts(page)
  await page.screenshot({ path: SHOT('sp6c-incidencia-entendido.png'), animations: 'disabled' })
  await page.getByRole('button', { name: 'Entendido' }).click()
  await expect(page.getByText('El gateway está en marcha')).toBeVisible()
  await expect(page.getByText('En marcha — incidencia')).toBeHidden()
})
