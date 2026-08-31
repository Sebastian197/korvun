import { expect, test } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'
import { releaseFacts } from '../src/releaseFacts'

const locales = [
  {
    label: 'EN',
    path: '/',
    hero: ['One binary.', 'Your models.', 'Your rules.'],
    headings: [
      'Kernel for Orchestrated Routing — Versatile Unified Nodes',
      'Install with proof, not promises.',
      'Eight pillars. One binary.',
      'Sensitive data never leaves the machine.',
      'A cloud model, governed tools, one config.',
      'Run it today.',
    ],
    quickstart: '/guide/quickstart/',
    mediaPrefix: '/media/',
  },
  {
    label: 'ES',
    path: '/es/',
    hero: ['Un binario.', 'Tus modelos.', 'Tus reglas.'],
    headings: [
      'Núcleo de Orquestación y Routing sobre Nodos Versátiles y Unificados',
      'Instala con pruebas, no promesas.',
      'Ocho pilares. Un binario.',
      'Los datos sensibles no salen de tu equipo.',
      'Un modelo de nube, herramientas gobernadas, una sola config.',
      'Ponlo en marcha hoy.',
    ],
    quickstart: '/es/guide/quickstart/',
    mediaPrefix: '/es/media/',
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
      await expect(sections).toHaveCount(7)
      const sectionNames = await sections.evaluateAll((elements) =>
        elements.map((element) => element.getAttribute('data-k-section') ?? ''),
      )
      expect(sectionNames.every((name) => name.length > 0)).toBe(true)

      const headings = await page.locator('main h2').allTextContents()
      expect(headings.map((heading) => heading.replace(/\s+/g, ' ').trim())).toEqual(
        locale.headings,
      )
    })

    test('publishes current, verifiable release facts', async ({ page }) => {
      await page.goto(locale.path)
      // textContent, not innerText: the source facts are asserted as authored,
      // independent of CSS text-transform (badges render uppercase).
      const main = (await page.locator('main').textContent()) ?? ''

      expect(main).toContain(releaseFacts.tag)
      expect(main).toContain('Beta')
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
      // Audit the settled page: with reduced motion the reveal system stays
      // off, so axe measures final colors instead of mid-transition blends.
      await page.emulateMedia({ reducedMotion: 'reduce' })
      await page.goto(locale.path)
      const video = page.locator('main video')
      await expect(video).toHaveCount(1)
      await expect(video).toHaveAttribute('controls', '')
      const source = await video.locator('source').getAttribute('src')
      // Docusaurus serves static assets under each locale's baseUrl.
      expect(source?.startsWith(locale.mediaPrefix)).toBe(true)

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
  await page.goto('/')
  const animated = page.locator('[data-motion]').first()
  await expect(animated).toBeVisible()
  const timing = await animated.evaluate((element) => {
    const styles = getComputedStyle(element)
    return {
      animationDuration: styles.animationDuration,
      transitionDuration: styles.transitionDuration,
    }
  })
  // Chromium serializes 0.01ms as "1e-05s"; compare numerically in seconds.
  expect(parseFloat(timing.animationDuration)).toBeLessThanOrEqual(0.001)
  expect(parseFloat(timing.transitionDuration)).toBeLessThanOrEqual(0.001)
})

test('below-fold storytelling reveals once on viewport entry', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'no-preference' })
  await page.goto('/')

  const hero = page.locator('[data-k-section="hero"] [data-motion]').first()
  const firstCapability = page
    .locator('[data-k-section="capabilities"] [data-motion]')
    .first()

  await expect
    .poll(() =>
      firstCapability.evaluate((element) => element.classList.contains('k-reveal')),
    )
    .toBe(true)
  // The reveal CSS must actually apply (regression: an html-level gate that
  // Docusaurus clobbered left below-fold content visible with no animation).
  expect(
    await firstCapability.evaluate((element) => getComputedStyle(element).opacity),
  ).toBe('0')
  expect(
    await hero.evaluate((element) => element.classList.contains('k-reveal')),
  ).toBe(false)
  expect(
    await firstCapability.evaluate((element) => element.classList.contains('k-in')),
  ).toBe(false)

  await firstCapability.scrollIntoViewIfNeeded()
  await expect
    .poll(() => firstCapability.evaluate((element) => element.classList.contains('k-in')))
    .toBe(true)
})

// ---- Brand-motion routing journey (brand-motion spec, Task 4) --------------

test('routes through every landing block in order', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'no-preference' })
  await page.goto('/')
  const journey = page.locator('[data-k-journey]')
  await expect(journey).toHaveCount(1)
  await expect(page.locator('[data-k-route-port]')).toHaveCount(6)
  for (const name of ['hero', 'install', 'capabilities', 'privacy', 'demo', 'final']) {
    const section = page.locator(`[data-k-section="${name}"]`)
    await section.scrollIntoViewIfNeeded()
    await expect.poll(() => section.getAttribute('data-k-route-state')).toMatch(/active|complete/)
  }
})

test('the journey switches to the mobile rail at 390px without overflow', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.emulateMedia({ reducedMotion: 'no-preference' })
  await page.goto('/')
  await expect(page.locator('[data-k-journey]')).toHaveAttribute('data-k-journey-mode', 'mobile')
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
  )
  expect(overflow).toBeLessThanOrEqual(0)
})

test('reduced motion shows the complete static circuit with no signal head', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.goto('/')
  await expect(page.locator('[data-k-journey]')).toHaveAttribute('data-k-static', 'true')
  await expect(page.locator('[data-k-route-signal]')).toBeHidden()
})

test('client-side navigation back home re-arms the routing journey', async ({ page }) => {
  // The director's finding (2026-08-29): landing on a docs page and then
  // clicking the navbar brand must yield a LIVE landing — the circuit
  // re-measures, draws and follows the scroll exactly like a fresh load.
  await page.goto('/guide/what-is-korvun/')
  await page.click('.navbar__brand')
  await expect(page.locator('[data-k-journey]')).toHaveAttribute('data-k-journey-mode', /desktop|mobile/)
  const base = page.locator('[data-k-route-path-base]')
  await expect.poll(async () => ((await base.getAttribute('d')) ?? '').length).toBeGreaterThan(10)
  await page.evaluate(() => window.scrollTo(0, 2400))
  await expect
    .poll(async () =>
      page.evaluate(() =>
        Number(
          document
            .querySelector<HTMLElement>('[data-k-journey]')
            ?.style.getPropertyValue('--k-route-progress') || '0',
        ),
      ),
    )
    .toBeGreaterThan(0.2)
})
