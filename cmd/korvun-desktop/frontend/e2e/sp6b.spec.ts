// SP6b e2e: the cut's acceptance scenarios (AS-1, AS-2, AS-6) plus the
// screenshot gate — REAL pipeline end to end (built chrome + SP4 proxy +
// real no-network core + the harness's scripted channel/model), 1440x900,
// states provoked for real, captures into design-drafts/ for the copilot's
// side-by-side review. Serial: the harness's core is shared state.
import { AxeBuilder } from '@axe-core/playwright'
import { expect, test, type Page } from '@playwright/test'
import { installBindings } from './bindings'

const BASE = 'http://127.0.0.1:43117'
const SHOT = (name: string): string => `../../../design-drafts/${name}`

test.describe.configure({ mode: 'serial' })

async function settleFonts(page: Page): Promise<void> {
  await page.evaluate(() => document.fonts.ready.then(() => undefined))
}

async function post(page: Page, path: string, body?: unknown): Promise<void> {
  const resp = await page.request.post(BASE + path, {
    data: body ?? [],
  })
  if (resp.status() >= 400) {
    throw new Error(`${path} -> ${resp.status()}: ${await resp.text()}`)
  }
}

// Reset the harness to a stopped, healthy-scripts state, whatever an earlier
// test (or run) left behind.
test.beforeAll(async ({ request }) => {
  await request.post(`${BASE}/__test/model`, { data: { mode: 'ok' } })
  await request.post(`${BASE}/__test/channel`, { data: { send: 'ok' } })
  await request.post(`${BASE}/__test/bindings/Stop`, { data: [] }) // may 200-with-error; fine
})

test('AS-1: stopped core → the parado hero from the real 503 contract', async ({
  page,
}) => {
  await installBindings(page)
  await page.goto('/')
  await expect(page.getByText('El gateway está detenido')).toBeVisible()
  await expect(
    page.getByText(/no reciben ni responden mensajes mientras esté parado/),
  ).toBeVisible()
  const start = page.getByRole('button', { name: /Iniciar/ })
  await expect(start).toBeVisible()
  // The chip states the shell truth: stopped, with the loaded config's name.
  await expect(page.getByTestId('status-chip')).toContainText('Detenido')
  await expect(page.getByTestId('status-chip')).toContainText('korvun.json')
  await expect(page.getByTestId('healthz-badge')).toContainText('sin respuesta')
  // Accessibility (FR-WIN-7): the stopped Home passes WCAG A/AA checks.
  const axe = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa']).analyze()
  expect(axe.violations, JSON.stringify(axe.violations, null, 2)).toEqual([])
})

test('AS-2: Start from the UI → marcha with real data, no client-side bearer', async ({
  page,
}) => {
  await installBindings(page)
  const external: string[] = []
  const authed: string[] = []
  page.on('request', (req) => {
    const url = new URL(req.url())
    if (url.host !== '127.0.0.1:43117') external.push(req.url())
    const auth = req.headers()['authorization']
    if (auth !== undefined) authed.push(req.url())
  })
  await page.goto('/')
  await page.getByRole('button', { name: /Iniciar/ }).click()
  await expect(page.getByText('El gateway está en marcha')).toBeVisible({ timeout: 15000 })
  await expect(page.getByTestId('status-chip')).toContainText('En marcha')
  await expect(page.getByTestId('status-chip')).toContainText(/:\d+/)
  await expect(page.getByTestId('healthz-badge')).toContainText('OK')
  // Real control-API data: the scripted channel and the template brain.
  await expect(page.getByText('Telegram')).toBeVisible()
  await expect(page.getByText('Operativo')).toBeVisible()
  await expect(page.getByText('asistente')).toBeVisible()
  await expect(page.getByText('Privado')).toBeVisible()
  // Zero-CDN + bearer honesty: nothing leaves the origin, and no request
  // from the page carries an Authorization header (proxy-injected only).
  expect(external, `external requests: ${external.join(', ')}`).toEqual([])
  expect(authed, `authorized requests from the page: ${authed.join(', ')}`).toEqual([])
})

test('Actividad vacía: designed empty state, En vivo', async ({ page }) => {
  await installBindings(page)
  await page.goto('/')
  await page.getByRole('button', { name: 'Actividad' }).click()
  await expect(page.getByText('Sin actividad todavía')).toBeVisible()
  await expect(page.getByText('En vivo')).toBeVisible({ timeout: 10000 })
  await settleFonts(page)
  await page.screenshot({ path: SHOT('sp6b-actividad-vacia.png'), animations: 'disabled' })
})

test('Ajustes: filas reales en oscuro y claro', async ({ page }) => {
  await installBindings(page)
  await page.goto('/')
  await page.getByRole('button', { name: 'Ajustes' }).click()
  await expect(page.getByText('Fichero de configuración')).toBeVisible()
  await expect(page.getByText(/korvun\.json/).first()).toBeVisible()
  await expect(page.getByText(/asignado al arrancar/)).toBeVisible()
  await expect(page.getByText(/KORVUN_ADMIN_TOKEN/)).toBeVisible()
  await expect(page.getByText('automático · se rota al arrancar')).toBeVisible()
  // No API → not painted (FR-WIN-4): the mock's Datos section stays out.
  await expect(page.getByText('Vaciar memoria')).toHaveCount(0)
  await expect(page.getByText('Mostrar en carpeta')).toHaveCount(0)
  // The Copy button writes the effective address (clipboard permission is
  // granted per-context in Playwright's Chromium).
  await settleFonts(page)
  await page.screenshot({ path: SHOT('sp6b-ajustes-oscuro.png'), animations: 'disabled' })
  await page.getByRole('button', { name: 'Claro' }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
  await settleFonts(page)
  await page.screenshot({ path: SHOT('sp6b-ajustes-claro.png'), animations: 'disabled' })
  // Persistence + accessibility in the light theme.
  await page.reload()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
  await page.getByRole('button', { name: 'Ajustes' }).click()
  const axe = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa']).analyze()
  expect(axe.violations, JSON.stringify(axe.violations, null, 2)).toEqual([])
  await page.getByRole('button', { name: 'Oscuro' }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
})

test('marcha con datos reales: inyección → tarjetas + capture', async ({ page }) => {
  await installBindings(page)
  await page.goto('/')
  await expect(page.getByTestId('healthz-badge')).toContainText('OK', { timeout: 10000 })
  // The feed is WINDOW-scoped: confirm the stream is live (via Actividad's
  // indicator) before injecting, then return to Inicio — the store is
  // module-level, the stream survives the view swap.
  await page.getByRole('button', { name: 'Actividad' }).click()
  await expect(page.getByText('En vivo')).toBeVisible({ timeout: 10000 })
  await page.getByRole('button', { name: 'Inicio' }).click()
  await post(page, '/__test/inject', { text: 'hola korvun' })
  await post(page, '/__test/inject', { text: 'resume mi día' })
  await post(page, '/__test/inject', { text: 'gracias' })
  await expect(page.getByTestId('card-recibidos')).toContainText('3', { timeout: 10000 })
  await expect(page.getByTestId('card-procesados')).toContainText('3', { timeout: 10000 })
  await expect(page.getByText(/desde que se abrió la ventana/).first()).toBeVisible()
  await settleFonts(page)
  await page.screenshot({ path: SHOT('sp6b-inicio-marcha.png'), animations: 'disabled' })
  // Accessibility on the running Home.
  const axe = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa']).analyze()
  expect(axe.violations, JSON.stringify(axe.violations, null, 2)).toEqual([])
})

test('Actividad con feed real: filas + filtros + capture', async ({ page }) => {
  await installBindings(page)
  await page.goto('/')
  await page.getByRole('button', { name: 'Actividad' }).click()
  // The feed is WINDOW-scoped: wait for the stream to be live before
  // injecting, or the frame honestly never reaches this window.
  await expect(page.getByText('En vivo')).toBeVisible({ timeout: 10000 })
  await post(page, '/__test/inject', { text: 'otro mensaje' })
  await expect(page.getByText('Mensaje recibido').first()).toBeVisible({ timeout: 10000 })
  await expect(page.getByText('Respuesta enviada').first()).toBeVisible()
  await expect(page.getByText('→ asistente').first()).toBeVisible()
  await expect(page.getByText('En vivo')).toBeVisible()
  // Type filter narrows honestly.
  await page.getByRole('button', { name: 'Respuestas' }).click()
  await expect(page.getByText('Mensaje recibido')).toHaveCount(0)
  await page.getByRole('button', { name: 'Todos los eventos' }).click()
  await settleFonts(page)
  await page.screenshot({ path: SHOT('sp6b-actividad-feed.png'), animations: 'disabled' })
  const axe = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa']).analyze()
  expect(axe.violations, JSON.stringify(axe.violations, null, 2)).toEqual([])
})

test('incidencia honesta: message_dropped real → banner ámbar + capture; AS-6 recovery', async ({
  page,
}) => {
  await installBindings(page)
  await page.goto('/')
  await expect(page.getByTestId('healthz-badge')).toContainText('OK', { timeout: 10000 })
  await page.getByRole('button', { name: 'Actividad' }).click()
  await expect(page.getByText('En vivo')).toBeVisible({ timeout: 10000 })
  await page.getByRole('button', { name: 'Inicio' }).click()
  await post(page, '/__test/channel', { send: 'fail' })
  await post(page, '/__test/inject', { text: 'este se pierde' })
  await expect(page.getByText('En marcha — incidencia')).toBeVisible({ timeout: 10000 })
  await expect(page.getByText(/Mensaje descartado en telegram/)).toBeVisible()
  await settleFonts(page)
  await page.screenshot({ path: SHOT('sp6b-inicio-incidencia.png'), animations: 'disabled' })
  await post(page, '/__test/channel', { send: 'ok' })
  // AS-6 recovery: a clean stop + start clears the banner.
  await page.getByRole('button', { name: /Detener/ }).click()
  await expect(page.getByText('El gateway está detenido')).toBeVisible({ timeout: 15000 })
  // The stopped Home with the SESSION's last data, dimmed (final-1's frame):
  // this is the cut's parado capture.
  await expect(page.getByTestId('home-stale-data')).toBeVisible()
  await settleFonts(page)
  await page.screenshot({ path: SHOT('sp6b-inicio-parado.png'), animations: 'disabled' })
  await page.getByRole('button', { name: /Iniciar/ }).click()
  await expect(page.getByText('El gateway está en marcha')).toBeVisible({ timeout: 15000 })
  await expect(page.getByText('En marcha — incidencia')).toHaveCount(0)
})

test('AS-6: el core muere solo → banner rojo honesto, recuperado con Start limpio', async ({
  page,
}) => {
  await installBindings(page)
  await page.goto('/')
  await expect(page.getByTestId('healthz-badge')).toContainText('OK', { timeout: 10000 })
  // The core vanishes WITHOUT the UI asking (the reap-shaped signal).
  await post(page, '/__test/core-exit')
  await expect(page.getByText('El núcleo se detuvo inesperadamente')).toBeVisible({
    timeout: 10000,
  })
  await expect(page.getByText(/el motivo queda en el registro del shell/)).toBeVisible()
  await page.getByRole('button', { name: /Iniciar/ }).click()
  await expect(page.getByText('El gateway está en marcha')).toBeVisible({ timeout: 15000 })
  await expect(page.getByText('El núcleo se detuvo inesperadamente')).toHaveCount(0)
  // Leave the harness stopped for any later suite run.
  await page.getByRole('button', { name: /Detener/ }).click()
  await expect(page.getByText('El gateway está detenido')).toBeVisible({ timeout: 15000 })
})