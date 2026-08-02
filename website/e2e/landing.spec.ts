// Landing gates (design spec AS-3 / AS-4 / AS-8 + the FR-LAND-1..3 content
// contract). Selectors verified against the INSTALLED vitepress 1.6.4
// theme source (VPHero.vue: the hero name is `.name.clip`; VPFeature.vue:
// one `.VPFeature` per features entry).
import { AxeBuilder } from '@axe-core/playwright'
import { expect, test } from '@playwright/test'

const PAGE = '/korvun/'

test.describe('landing — brand, clip, pillars (FR-LAND-1..3)', () => {
  test('AS-3: every request is same-origin — zero external hosts', async ({
    page,
    baseURL,
  }) => {
    const external: string[] = []
    page.on('request', (req) => {
      if (!req.url().startsWith(baseURL!)) external.push(req.url())
    })
    await page.goto(PAGE)
    await page.waitForLoadState('networkidle')
    // Interact: scroll to the clip and start playback, so media/fonts that
    // load lazily are exercised too.
    const video = page.locator('video').first()
    await video.scrollIntoViewIfNeeded()
    await video.evaluate((v: HTMLVideoElement) => v.play())
    await page.waitForTimeout(500)
    expect(external).toEqual([])
  })

  test('FR-LAND-1: hero mark, gradient identity moment, Geist self-hosted', async ({
    page,
  }) => {
    await page.goto(PAGE)
    // The K terminal mark as the hero image.
    await expect(
      page.locator('.VPHero img[src*="korvun-logo-hero"]'),
    ).toBeVisible()
    // The teal→violet gradient ONLY as the hero identity moment.
    // (VPHero.vue: class="name clip" — one element, both classes.)
    const bg = await page
      .locator('.VPHero .name.clip')
      .evaluate((el) => getComputedStyle(el).backgroundImage)
    expect(bg).toContain('linear-gradient')
    // Geist actually applied and actually loaded (self-hosted woff2 —
    // external hosts are already outlawed by AS-3).
    const font = await page.evaluate(
      () => getComputedStyle(document.body).fontFamily,
    )
    expect(font).toMatch(/Geist/)
    const geistLoaded = await page.evaluate(async () => {
      await document.fonts.ready
      return [...document.fonts].some(
        (f) => f.family.includes('Geist') && f.status === 'loaded',
      )
    })
    expect(geistLoaded).toBe(true)
  })

  test('FR-LAND-2: the launch clip — poster, click-to-play, full-demo link', async ({
    page,
  }) => {
    await page.goto(PAGE)
    const video = page.locator('video').first()
    await video.scrollIntoViewIfNeeded()
    await expect(video).toBeVisible()
    await expect(video).toHaveAttribute('poster', /.+/)
    expect(await video.evaluate((v: HTMLVideoElement) => v.autoplay)).toBe(
      false,
    )
    expect(await video.evaluate((v: HTMLVideoElement) => v.paused)).toBe(true)
    // Playable on demand: play() resolves and the element leaves paused —
    // proving the committed mp4 resolves under the base.
    await video.evaluate((v: HTMLVideoElement) => v.play())
    await page.waitForTimeout(700)
    expect(await video.evaluate((v: HTMLVideoElement) => v.paused)).toBe(false)
    // The 43 s demo as the secondary link.
    await expect(
      page.locator('a[href*="korvun-v060-demo-full-1080p.mp4"]'),
    ).toBeVisible()
  })

  test('FR-LAND-3: the seven real pillars', async ({ page }) => {
    await page.goto(PAGE)
    await expect(page.locator('.VPFeature')).toHaveCount(7)
  })

  test('AS-8: axe wcag2a/wcag2aa clean on the landing', async ({ page }) => {
    await page.goto(PAGE)
    // Let the entrance animations FINISH before auditing: axe samples
    // computed colors, and a mid-fade element blends with the page bg
    // (23 phantom color-contrast hits when scanned during the fade —
    // found empirically). Steady state is what users read.
    await page.evaluate(() =>
      Promise.all(document.getAnimations().map((a) => a.finished)),
    )
    const axe = await new AxeBuilder({ page })
      .withTags(['wcag2a', 'wcag2aa'])
      .analyze()
    expect(axe.violations).toEqual([])
  })
})

test.describe('AS-4: prefers-reduced-motion', () => {
  // Verified against the installed playwright 1.61.1 types: reducedMotion
  // rides in contextOptions.
  test.use({ contextOptions: { reducedMotion: 'reduce' } })

  test('no non-essential animation runs; the clip never starts by itself', async ({
    page,
  }) => {
    await page.goto(PAGE)
    await page.waitForLoadState('networkidle')
    await page.waitForTimeout(400)
    // Every entrance/identity animation must be gated behind
    // (prefers-reduced-motion: no-preference): at idle, zero running
    // animations document-wide.
    const running = await page.evaluate(() => document.getAnimations().length)
    expect(running).toBe(0)
    const video = page.locator('video').first()
    await video.scrollIntoViewIfNeeded()
    await page.waitForTimeout(400)
    expect(await video.evaluate((v: HTMLVideoElement) => v.paused)).toBe(true)
  })
})
