// SP3 gates: the EN documentation map (FR-DOCS-1) — fully populated,
// reachable from nav+sidebar, searchable locally, and self-contained
// (search must never call out to any external origin: AS-2).
import { expect, test } from '@playwright/test'

// The nine documented pages of the FR-DOCS-1 map (the landing is the
// tenth and has its own suites).
const PAGES: Array<[string, RegExp]> = [
  ['guide/what-is-korvun', /what is korvun/i],
  ['guide/install', /install/i],
  ['guide/quickstart', /quickstart/i],
  ['guide/builder', /builder/i],
  ['reference/configuration', /configuration/i],
  ['channels/telegram', /telegram/i],
  ['channels/discord', /discord/i],
  ['channels/webhook', /webhook/i],
  ['releases/', /releases/i],
]

const urlOf = (path: string) =>
  `/korvun/${path}${path.endsWith('/') ? '' : '.html'}`

test('every EN page is real content — zero stubs left in the map', async ({
  page,
}) => {
  for (const [path, title] of PAGES) {
    await page.goto(urlOf(path))
    await expect(page.locator('h1').first(), path).toHaveText(title)
    const body = await page.locator('body').innerText()
    expect(body, `${path} still carries a stub marker`).not.toContain(
      'Stub (SP1',
    )
  }
})

test('nav + sidebar reach the whole map from a guide page', async ({
  page,
}) => {
  await page.goto(urlOf('guide/quickstart'))
  const hrefs = await page.evaluate(() =>
    Array.from(document.querySelectorAll('.VPNavBar a, .VPSidebar a')).map(
      (a) => a.getAttribute('href') ?? '',
    ),
  )
  for (const [path] of PAGES) {
    expect(
      hrefs.some((h) => h.includes(path.replace(/\/$/, ''))),
      `${path} reachable from nav/sidebar`,
    ).toBe(true)
  }
})

test('AS-2: local search finds the quickstart — and never leaves the origin', async ({
  page,
  baseURL,
}) => {
  const external: string[] = []
  page.on('request', (req) => {
    if (!req.url().startsWith(baseURL!)) external.push(req.url())
  })
  await page.goto('/korvun/')
  await page.locator('#local-search button').click()
  await page.locator('.VPLocalSearchBox input').fill('quickstart')
  // The result anchor IS `.result` (verified in the installed
  // VPLocalSearchBox.vue markup), not a child of it.
  const first = page.locator('.VPLocalSearchBox a.result').first()
  await expect(first).toBeVisible()
  await first.click()
  await expect(page).toHaveURL(/\/korvun\/guide\/quickstart/)
  expect(external, 'search emitted external requests').toEqual([])
})