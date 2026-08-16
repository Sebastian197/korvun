import { expect, test } from '@playwright/test'

const mirroredPages = [
  'guide/what-is-korvun',
  'guide/install',
  'guide/quickstart',
  'guide/builder',
  'reference/configuration',
  'channels/telegram',
  'channels/discord',
  'channels/webhook',
  'releases',
]

test('the locale switcher connects both real homepages', async ({ page }) => {
  await page.goto('/korvun/')
  await page.locator('nav a[href="/korvun/es/"]').click()
  await expect(page).toHaveURL(/\/korvun\/es\/$/)
  await expect(page.locator('main h1')).toContainText('Un binario')

  await page.locator('nav a[href="/korvun/"]').click()
  await expect(page).toHaveURL(/\/korvun\/$/)
  await expect(page.locator('main h1')).toContainText('One binary')
})

test('every public English route has a Spanish mirror', async ({ page }) => {
  for (const route of mirroredPages) {
    const en = await page.request.get(`/korvun/${route}/`)
    const es = await page.request.get(`/korvun/es/${route}/`)
    expect(en.status(), `English route ${route}`).toBe(200)
    expect(es.status(), `Spanish route ${route}`).toBe(200)
  }
})

test('English and Spanish documentation expose the same route tree', async ({ page }) => {
  const collect = async (url: string, prefix: string) => {
    await page.goto(url)
    return page.locator('nav a, aside a').evaluateAll(
      (links, routePrefix) =>
        links
          .map((link) => link.getAttribute('href') ?? '')
          .filter((href) => href.startsWith(routePrefix as string))
          .map((href) => href.slice((routePrefix as string).length))
          .filter(Boolean)
          .sort(),
      prefix,
    )
  }

  const en = (await collect('/korvun/guide/quickstart/', '/korvun/')).filter(
    (route) => !route.startsWith('es/'),
  )
  const es = await collect('/korvun/es/guide/quickstart/', '/korvun/es/')
  expect(es).toEqual(en)
})
