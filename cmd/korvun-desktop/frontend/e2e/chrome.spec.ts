// SP6a e2e (kept green through 6b): the chrome shell against the REAL
// pipeline (harness = built chrome + SP4 proxy + stopped core), plus the 6a
// screenshot set (regenerated on every run; the 6b set lives in
// sp6b.spec.ts). The theme control moved into Ajustes in 6b (FR-WIN-5).
import { expect, test } from '@playwright/test'
import { HARNESS_ADDR, SHOT, settleFonts } from './util'

test('the stopped chrome: real 503 contract → parado hero, same-origin only', async ({ page }) => {
  const external: string[] = []
  page.on('request', (req) => {
    const url = new URL(req.url())
    if (url.host !== HARNESS_ADDR) external.push(req.url())
  })

  await page.goto('/')
  // The hero is driven by the status store parsing the proxy's real
  // {"error":"core stopped"} body — no mock anywhere.
  await expect(page.getByTestId('home-parado')).toBeVisible()
  await expect(page.getByText('El gateway está detenido')).toBeVisible()
  await expect(page.getByRole('button', { name: /Iniciar/ })).toBeVisible()

  // Zero-CDN gate (ADR-0029 §5): nothing may leave the origin.
  expect(external, `external requests: ${external.join(', ')}`).toEqual([])

  await settleFonts(page)
  await page.screenshot({
    path: SHOT('sp6a-inicio-parado-minimo.png'),
    animations: 'disabled',
  })
})

test('sidebar navigation: six sections in design order, active state, real views', async ({
  page,
}) => {
  await page.goto('/')
  const labels = await page
    .getByRole('navigation', { name: 'Secciones' })
    .getByRole('button')
    .allTextContents()
  // SP4 (operator-console spec): Chat joins third — the same evolution the
  // App.test.tsx guard records.
  expect(labels.map((l) => l.trim())).toEqual([
    'Inicio',
    'Builder',
    'Chat',
    'Canales',
    'Actividad',
    'Ajustes',
  ])
  await page.getByRole('button', { name: 'Canales' }).click()
  // Canales is a real view (SP6c); stopped, its snapshot is empty → the
  // honest "no channels yet" state, never a placeholder.
  await expect(page.getByTestId('canales')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Canales' })).toHaveAttribute(
    'aria-current',
    'page',
  )
  await settleFonts(page)
  await page.screenshot({
    path: SHOT('sp6a-shell-navegacion.png'),
    animations: 'disabled',
  })
})

test('theme swap (Ajustes): light theme persists and repaints the token table', async ({
  page,
}) => {
  await page.goto('/')
  await page.getByRole('button', { name: 'Ajustes' }).click()
  await page.getByRole('button', { name: 'Claro' }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
  // Persistence: the choice survives a reload (localStorage, FR-WIN-5).
  await page.reload()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
  await expect(page.getByTestId('home-parado')).toBeVisible()
  await settleFonts(page)
  await page.screenshot({
    path: SHOT('sp6a-tema-claro.png'),
    animations: 'disabled',
  })
})
