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
  // Auto-waiting assertion first: the SPA swap takes a beat, and a raw
  // innerText read races it (caught empirically).
  await expect(page.locator('.VPHero .text')).toContainText('Un binario')
  const body = await page.locator('body').innerText()
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

test('SP4b: the switcher is PER-PAGE — every EN page offers its ES twin', async ({
  page,
}) => {
  // Full mirror (Chano 2026-08-02): from any EN page the switcher goes to
  // the SAME page in Spanish, and it exists.
  for (const path of [
    'guide/install.html',
    'reference/configuration.html',
    'channels/webhook.html',
  ]) {
    await page.goto(`/korvun/${path}`)
    const flyout = await openSwitcher(page)
    const href = await flyout
      .locator('.items a[href*="/es/"]')
      .first()
      .getAttribute('href')
    expect(href, path).toContain(`/korvun/es/${path}`)
    const res = await page.request.get(href!)
    expect(res.status(), `${href} must exist`).toBe(200)
  }
})

test('SP4b: the ES nav/sidebar is a structural mirror of the EN tree', async ({
  page,
}) => {
  const collect = async (path: string, prefix: string) => {
    await page.goto(path)
    return new Set(
      await page.evaluate(
        (pre) =>
          Array.from(
            document.querySelectorAll('.VPNavBar a, .VPSidebar a'),
          )
            .map((a) => a.getAttribute('href') ?? '')
            .filter((h) => h.startsWith(pre))
            .map((h) => h.slice(pre.length)),
        prefix,
      ),
    )
  }
  const en = await collect('/korvun/guide/quickstart.html', '/korvun/')
  const es = await collect('/korvun/es/guide/quickstart.html', '/korvun/es/')
  en.delete('') // the EN logo/home link normalizes to ''
  es.delete('')
  // Same tree, page for page (locale-switcher entries excluded by prefix).
  expect([...es].sort()).toEqual(
    [...en].sort().filter((h) => !h.startsWith('es/')),
  )
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