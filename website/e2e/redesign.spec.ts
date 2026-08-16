import { expect, test } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'

const locales = [
  {
    label: 'EN',
    path: '/korvun/',
    hero: ['One binary.', 'Your models.', 'Your rules.'],
    headings: [
      'Install with proof, not promises.',
      'Seven pillars. One binary.',
      'Sensitive data never leaves the machine.',
      'Korvun in 27 seconds.',
      'Run it today.',
    ],
    quickstart: '/korvun/guide/quickstart',
  },
  {
    label: 'ES',
    path: '/korvun/es/',
    hero: ['Un binario.', 'Tus modelos.', 'Tus reglas.'],
    headings: [
      'Instala con pruebas, no promesas.',
      'Siete pilares. Un binario.',
      'Los datos sensibles no salen de tu equipo.',
      'Korvun en 27 segundos.',
      'Ponlo en marcha hoy.',
    ],
    quickstart: '/korvun/es/guide/quickstart',
  },
] as const

for (const locale of locales) {
  test.describe(`${locale.label} redesigned landing`, () => {
    test('presents the selected story in the intended order', async ({ page }) => {
      await page.goto(locale.path)

      const h1 = page.locator('main h1')
      await expect(h1).toBeVisible()
      for (const line of locale.hero) await expect(h1).toContainText(line)

      const sections = page.locator('main [data-k-section]')
      await expect(sections).toHaveCount(6)
      await expect(sections).toHaveAttribute('data-k-section', /.+/)

      const headings = await page.locator('main h2').allTextContents()
      expect(headings.map((heading) => heading.replace(/\s+/g, ' ').trim())).toEqual(
        locale.headings,
      )
    })

    test('publishes current, verifiable release facts', async ({ page }) => {
      await page.goto(locale.path)
      const main = await page.locator('main').innerText()

      expect(main).toContain('v0.7.0')
      expect(main).toContain('Apache-2.0')
      expect(main).toContain('6')
      expect(main).toMatch(/cosign/i)
      expect(main).toContain('SBOM')

      expect(main).not.toContain('MIT')
      expect(main).not.toContain('install.sh')
      expect(main).not.toContain('brew install')
      expect(main).not.toContain('apt-get install')
      expect(main).not.toContain('scoop install')
    })

    test('has real destinations for every landing action', async ({ page }) => {
      await page.goto(locale.path)
      const links = page.locator('main a')
      expect(await links.count()).toBeGreaterThan(8)

      const hrefs = await links.evaluateAll((anchors) =>
        anchors.map((anchor) => anchor.getAttribute('href') ?? ''),
      )
      expect(hrefs).not.toContain('')
      expect(hrefs).not.toContain('#')
      expect(hrefs).toContain(locale.quickstart)
      expect(hrefs).toContain('https://github.com/Sebastian197/korvun')
    })

    test('uses same-origin media and passes the WCAG A/AA scan', async ({ page }) => {
      await page.goto(locale.path)
      const video = page.locator('main video')
      await expect(video).toHaveCount(1)
      await expect(video).toHaveAttribute('controls', '')
      const source = await video.locator('source').getAttribute('src')
      expect(source).toMatch(/^\/korvun\/media\//)

      const results = await new AxeBuilder({ page })
        .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'])
        .analyze()
      expect(results.violations).toEqual([])
    })
  })
}

test.describe('390 px landing', () => {
  test.use({ viewport: { width: 390, height: 844 } })

  for (const locale of locales) {
    test(`${locale.label} remains within the viewport`, async ({ page }) => {
      await page.goto(locale.path)
      await page.waitForLoadState('networkidle')

      const widths = await page.evaluate(() => ({
        viewport: document.documentElement.clientWidth,
        page: document.documentElement.scrollWidth,
      }))
      expect(widths.page).toBeLessThanOrEqual(widths.viewport)

      const terminal = page.locator('.k-terminal')
      await terminal.scrollIntoViewIfNeeded()
      await expect(terminal).toBeVisible()
      const box = await terminal.boundingBox()
      expect(box!.x).toBeGreaterThanOrEqual(0)
      expect(box!.x + box!.width).toBeLessThanOrEqual(390)
    })
  }
})

test('reduced motion removes meaningful animation', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.goto('/korvun/')
  const animated = page.locator('[data-motion]').first()
  await expect(animated).toBeVisible()
  const timing = await animated.evaluate((element) => {
    const styles = getComputedStyle(element)
    return {
      animationDuration: styles.animationDuration,
      transitionDuration: styles.transitionDuration,
    }
  })
  expect(timing.animationDuration).toMatch(/^(0s|0\.001s|0\.01ms)$/)
  expect(timing.transitionDuration).toMatch(/^(0s|0\.001s|0\.01ms)$/)
})
