// SP4 gates: the ES layer (AS-5, the layered-bilingual decision).
// EN is the complete source of truth; ES covers landing + quickstart.
// The contract: the switcher works BOTH ways, the ES nav never points at
// a page that does not exist, the ES content is real (no stub markers),
// and the local search speaks Spanish on /es/.
import { expect, test } from '@playwright/test'

const openSwitcher = async (page: import('@playwright/test').Page) => {
  const flyout = page.locator('.VPNavBarTranslations')
  await expect(flyout).toBeVisible()
  await flyout.hover()
  return flyout
}

test('AS-5: the switcher lands on a REAL ES landing (no stubs), and back', async ({
  page,
}) => {
  await page.goto('/korvun/')
  const flyout = await openSwitcher(page)
  await flyout.locator('a[href$="/korvun/es/"]').click()
  await expect(page).toHaveURL(/\/korvun\/es\/$/)
  // Real content, in Spanish, no stub markers anywhere on the page.
  const body = await page.locator('body').innerText()
  expect(body).toContain('Un binario')
  expect(body).not.toMatch(/Stub \(SP|Stub — SP/)
  // And back to EN from the ES landing.
  const back = await openSwitcher(page)
  await back.locator('a[href$="/korvun/"]').first().click()
  await expect(page).toHaveURL(/\/korvun\/$/)
})

test('AS-5: the ES nav reaches the ES quickstart — and nothing dead', async ({
  page,
}) => {
  await page.goto('/korvun/es/')
  // Every nav/sidebar/menu href on the ES landing must resolve — pages
  // without an ES layer simply do not appear.
  const hrefs = await page.evaluate(() =>
    Array.from(
      document.querySelectorAll('.VPNavBar a, .VPSidebar a, .VPHero a'),
    )
      .map((a) => a.getAttribute('href') ?? '')
      .filter((h) => h.startsWith('/korvun/')),
  )
  expect(hrefs.some((h) => h.includes('/es/guide/quickstart'))).toBe(true)
  for (const href of hrefs) {
    const res = await page.request.get(href)
    expect(res.status(), `${href} must exist`).toBe(200)
  }
  // The ES quickstart is real content with the SAME technical truth.
  await page.goto('/korvun/es/guide/quickstart.html')
  const body = await page.locator('body').innerText()
  expect(body).not.toMatch(/Stub \(SP|Stub — SP/)
  expect(body).toContain('korvun serve --config korvun.local.json')
  expect(body).toContain('TELEGRAM_TOKEN')
})

test('AS-5: returning from an EN-only page breaks nothing', async ({
  page,
}) => {
  // /guide/install has NO ES counterpart: the switcher goes to the ES
  // locale ROOT (i18nRouting: false), never to a dead /es/guide/install.
  await page.goto('/korvun/guide/install.html')
  const flyout = await openSwitcher(page)
  const href = await flyout
    .locator('.items a[href*="/es/"]')
    .first()
    .getAttribute('href')
  expect(href).toMatch(/\/korvun\/es\/$/)
  const res = await page.request.get(href!)
  expect(res.status()).toBe(200)
})

test('AS-5: the local search speaks Spanish on /es/', async ({ page }) => {
  await page.goto('/korvun/es/')
  // The configured es translations must reach the UI: the navbar search
  // button reads "Buscar" (default: "Search").
  await expect(page.locator('#local-search')).toContainText('Buscar')
  await page.locator('#local-search button').click()
  // And the modal footer strings are Spanish too.
  await expect(page.locator('.VPLocalSearchBox')).toContainText(/navegar/i)
})