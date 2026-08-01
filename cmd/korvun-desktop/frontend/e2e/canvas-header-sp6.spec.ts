// SP6 RED (builder-canvas, 2b): inside the desktop iframe the builder must
// show a SINGLE header — the chrome already titles "Builder", so the builder's
// own "korvun · builder" bar must not render (window.self !== window.top).
// RED today: the own bar renders inside the iframe → two headers.
import { expect, test, type Page } from '@playwright/test'
import { installBindings } from './bindings'
import { BASE } from './util'

test.describe.configure({ mode: 'serial' })

async function ensureRunning(page: Page): Promise<void> {
  await page.request.post(`${BASE}/__test/bindings/Start`, { data: [] }).catch(() => undefined)
  await expect(page.getByTestId('healthz-badge')).toContainText('OK', { timeout: 15000 })
}

test.beforeEach(async ({ request }) => {
  await request.post(`${BASE}/__test/model`, { data: { mode: 'ok' } })
  await request.post(`${BASE}/__test/channel`, { data: { send: 'ok' } })
  await request.post(`${BASE}/__test/reset-config`, { data: [] })
})

test.afterAll(async ({ request }) => {
  await request.post(`${BASE}/__test/core-exit`, { data: [] }).catch(() => undefined)
})

test('el lienzo embebido muestra UNA sola cabecera (sin la barra propia del builder)', async ({
  page,
}) => {
  await installBindings(page)
  await page.goto('/')
  await ensureRunning(page)
  await page.getByRole('button', { name: 'Builder', exact: true }).click()
  const frame = page.frameLocator('iframe[title="Builder"]')
  await frame.getByLabel('admin bearer token').fill('x')
  await frame.getByRole('button', { name: 'Load' }).click()
  await expect(frame.getByTestId('canvas-surface')).toBeVisible({ timeout: 15000 })

  // The builder's own crumb bar must be absent inside the iframe — the desktop
  // chrome's "Builder" title is the single header.
  await expect(frame.getByText('builder', { exact: true })).toHaveCount(0)
})
