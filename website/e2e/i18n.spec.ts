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
  await page.goto('/')
  // The locale links live inside the navbar dropdown; open it first.
  await page.locator('nav .dropdown').hover()
  await page.locator('nav a[href="/es/"]').click()
  await expect(page).toHaveURL(/\/es\/$/)
  await expect(page.locator('main h1')).toContainText('Un binario')

  await page.locator('nav .dropdown').hover()
  await page.locator('nav a[href="/"]').click()
  await expect(page).toHaveURL(/\/$/)
  await expect(page.locator('main h1')).toContainText('One binary')
})

test('every public English route has a Spanish mirror', async ({ page }) => {
  for (const route of mirroredPages) {
    const en = await page.request.get(`/${route}/`)
    const es = await page.request.get(`/es/${route}/`)
    expect(en.status(), `English route ${route}`).toBe(200)
    expect(es.status(), `Spanish route ${route}`).toBe(200)
  }
})

test('English and Spanish documentation expose the same route tree', async ({ page }) => {
  const collect = async (url: string, prefix: string) => {
    await page.goto(url)
    const routes = await page.locator('nav a, aside a').evaluateAll(
      (links, routePrefix) =>
        links
          .map((link) => link.getAttribute('href') ?? '')
          .filter((href) => href.startsWith(routePrefix as string))
          .map((href) => href.slice((routePrefix as string).length))
          .filter(Boolean),
      prefix,
    )
    // The invariant is the SET of exposed routes: Docusaurus legitimately
    // renders the same href more than once (navbar plus the mobile sidebar
    // clone, whose presence depends on hydration timing), so raw lists
    // flake while the route tree itself is stable.
    return [...new Set(routes)].sort()
  }

  const en = (await collect('/guide/quickstart/', '/')).filter(
    (route) => !route.startsWith('es/'),
  )
  const es = await collect('/es/guide/quickstart/', '/es/')
  expect(es).toEqual(en)
})
