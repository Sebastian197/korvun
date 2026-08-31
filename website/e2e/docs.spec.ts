import { expect, test } from '@playwright/test'

const pages: Array<[string, RegExp]> = [
  ['guide/what-is-korvun', /what is korvun/i],
  ['guide/install', /install/i],
  ['guide/quickstart', /quickstart/i],
  ['guide/builder', /builder/i],
  ['reference/configuration', /configuration/i],
  ['channels/telegram', /telegram/i],
  ['channels/discord', /discord/i],
  ['channels/webhook', /webhook/i],
  ['releases', /releases/i],
]

test('every English documentation route renders real content', async ({ page }) => {
  for (const [route, title] of pages) {
    await page.goto(`/${route}/`)
    await expect(page.locator('main h1').first(), route).toHaveText(title)
    await expect(page.locator('main')).not.toContainText(/Stub \(SP|Stub — SP/)
  }
})

test('documentation navigation reaches the complete public map', async ({ page }) => {
  await page.goto('/guide/quickstart/')
  const hrefs = await page.locator('nav a, aside a').evaluateAll((links) =>
    links.map((link) => link.getAttribute('href') ?? ''),
  )

  for (const [route] of pages) {
    expect(
      hrefs.some((href) => href.includes(`/${route}`)),
      `${route} must be reachable from the documentation navigation`,
    ).toBe(true)
  }
})

test('documentation does not make third-party runtime requests', async ({ page, baseURL }) => {
  const externalRequests: string[] = []
  page.on('request', (request) => {
    if (!request.url().startsWith(baseURL!)) externalRequests.push(request.url())
  })

  await page.goto('/guide/quickstart/')
  await page.waitForLoadState('networkidle')
  expect(externalRequests).toEqual([])
})
