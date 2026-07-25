// SP6a e2e: the chrome shell against the REAL pipeline (harness = built
// chrome + SP4 proxy + stopped core), plus the cut's screenshot gate
// (1440x900, states provoked for real, files into design-drafts/ for the
// copilot's side-by-side review — no screenshots, no review, no approval).
import { expect, test } from '@playwright/test'

const SHOT = (name: string): string => `../../../design-drafts/${name}`

// Screenshots wait for the embedded Geist faces so a cold run never captures
// the fallback font.
async function settleFonts(page: import('@playwright/test').Page): Promise<void> {
  await page.evaluate(() => document.fonts.ready.then(() => undefined))
}

test('the stopped chrome: real 503 contract → parado hero, same-origin only', async ({
  page,
}) => {
  const external: string[] = []
  page.on('request', (req) => {
    const url = new URL(req.url())
    if (url.host !== '127.0.0.1:43117') external.push(req.url())
  })

  await page.goto('/')
  // The hero is driven by the status store parsing the proxy's real
  // {"error":"core stopped"} body — no mock anywhere.
  await expect(page.getByTestId('home-parado')).toBeVisible()
  await expect(page.getByText('Gateway detenido')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Iniciar Korvun' })).toBeVisible()

  // Zero-CDN gate (ADR-0029 §5): nothing may leave the origin.
  expect(external, `external requests: ${external.join(', ')}`).toEqual([])

  await settleFonts(page)
  await page.screenshot({ path: SHOT('sp6a-inicio-parado-minimo.png'), animations: 'disabled' })
})

test('sidebar navigation: five sections, active state, honest empty views', async ({
  page,
}) => {
  await page.goto('/')
  for (const label of ['Inicio', 'Canales', 'Actividad', 'Builder', 'Ajustes']) {
    await expect(page.getByRole('button', { name: label })).toBeVisible()
  }
  await page.getByRole('button', { name: 'Canales' }).click()
  await expect(page.getByText(/Canales estará disponible/)).toBeVisible()
  await expect(page.getByRole('button', { name: 'Canales' })).toHaveAttribute(
    'aria-current',
    'page',
  )
  await settleFonts(page)
  await page.screenshot({ path: SHOT('sp6a-shell-navegacion.png'), animations: 'disabled' })
})

test('theme swap: light theme persists and repaints the token table', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByTestId('home-parado')).toBeVisible()
  await page.getByTestId('theme-toggle').click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
  // Persistence: the choice survives a reload (localStorage, FR-WIN-5).
  await page.reload()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')
  await expect(page.getByTestId('home-parado')).toBeVisible()
  await settleFonts(page)
  await page.screenshot({ path: SHOT('sp6a-tema-claro.png'), animations: 'disabled' })
})
