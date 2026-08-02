// SP2b gates: scroll storytelling as PROGRESSIVE ENHANCEMENT.
//
// THE LAW (AS-4b/4c): the page is complete from the first pixel. With
// reduced motion, with JavaScript disabled, or on a browser without
// IntersectionObserver / scroll-driven support, NOTHING is ever hidden —
// the choreography only exists once JS arms it (html.k-motion) and motion
// is welcome. These tests assert the law, not the implementation.
import { expect, test, type Locator, type Page } from '@playwright/test'

const PAGE = '/korvun/'
// The story content that must never be hidden.
const STORY = ['.VPHero', '.VPFeatures .item', '.k-privacy', '.k-clip']

const opacityOf = (loc: Locator) =>
  loc.evaluate((el) => parseFloat(getComputedStyle(el).opacity))

// Computed opacity of EVERY story element WITHOUT scrolling — offscreen
// included. This is the strong form of the law: not "it appears when you
// get there" but "it was never styled hidden at all".
async function assertFullyPresentWithoutScroll(page: Page) {
  for (const sel of STORY) {
    const els = page.locator(sel)
    const n = await els.count()
    expect(n, `${sel} must exist on the landing`).toBeGreaterThan(0)
    for (let i = 0; i < n; i++) {
      expect(await opacityOf(els.nth(i)), `${sel}[${i}] opacity`).toBe(1)
    }
  }
}

test.describe('AS-4b: reduced motion — complete and still', () => {
  test.use({ contextOptions: { reducedMotion: 'reduce' } })

  test('all story content fully visible with NO scroll; zero animations', async ({
    page,
  }) => {
    await page.goto(PAGE)
    await page.waitForLoadState('networkidle')
    await assertFullyPresentWithoutScroll(page)
    expect(await page.evaluate(() => document.getAnimations().length)).toBe(0)
  })
})

test.describe('AS-4c: JavaScript disabled — complete', () => {
  test.use({ javaScriptEnabled: false })

  test('all story content fully visible with NO scroll', async ({ page }) => {
    await page.goto(PAGE)
    await assertFullyPresentWithoutScroll(page)
  })
})

test.describe('storytelling with motion (the enhanced path)', () => {
  test('every scene completes its reveal in view — once, never re-hiding', async ({
    page,
  }) => {
    await page.goto(PAGE)
    for (const sel of STORY) {
      const els = page.locator(sel)
      expect(await els.count()).toBeGreaterThan(0)
      for (let i = 0; i < (await els.count()); i++) {
        await els.nth(i).scrollIntoViewIfNeeded()
        await expect
          .poll(() => opacityOf(els.nth(i)), { timeout: 4000 })
          .toBeGreaterThanOrEqual(0.99)
      }
    }
    // Animate ONCE: back at the top, every IO-revealed scene stays
    // revealed (no re-hide, no re-animation on the way up).
    await page.evaluate(() => window.scrollTo(0, 0))
    await page.waitForTimeout(600)
    for (const sel of ['.VPFeatures .item', '.k-privacy']) {
      const els = page.locator(sel)
      for (let i = 0; i < (await els.count()); i++) {
        expect(await opacityOf(els.nth(i))).toBeGreaterThanOrEqual(0.99)
      }
    }
    // The video FRAME is the one scroll-DRIVEN piece (view() timeline):
    // its presence is scrubbed by scroll position on purpose, so offscreen
    // it parks DIMMED — never hidden (≥ 0.4 floor) — and completes again
    // on every approach (asserted above). Only the frame scrubs: the
    // caption text never dims (AA), and the block reveals once like every
    // other scene.
    expect(
      await opacityOf(page.locator('.k-clip video')),
    ).toBeGreaterThanOrEqual(0.4)
  })

  test('AS-perf: no layout thrash during the scroll pass (CLS ≈ 0)', async ({
    page,
  }) => {
    await page.goto(PAGE)
    await page.waitForLoadState('networkidle')
    const cls = await page.evaluate(async () => {
      let total = 0
      const po = new PerformanceObserver((list) => {
        for (const e of list.getEntries() as PerformanceEntry[] & { hadRecentInput?: boolean; value?: number }[]) {
          const shift = e as unknown as { hadRecentInput: boolean; value: number }
          if (!shift.hadRecentInput) total += shift.value
        }
      })
      // NOT buffered on purpose: load-time shifts are a different problem;
      // this gate is about the SCROLL choreography staying on the
      // compositor (transform/opacity only ⇒ zero layout shift).
      po.observe({ type: 'layout-shift' })
      const h = document.documentElement.scrollHeight
      for (let y = 0; y <= h; y += 100) {
        window.scrollTo(0, y)
        await new Promise((r) => requestAnimationFrame(r))
      }
      await new Promise((r) => setTimeout(r, 400))
      return total
    })
    expect(cls).toBeLessThan(0.02)
  })

  test('micro-interactions: cards lift on hover; CTA keyboard focus is visible', async ({
    page,
  }) => {
    await page.goto(PAGE)
    const card = page.locator('.VPFeature').first()
    await card.scrollIntoViewIfNeeded()
    await page.waitForTimeout(800) // let the reveal settle first
    const before = await card.evaluate((el) => getComputedStyle(el).transform)
    await card.hover()
    await expect
      .poll(() => card.evaluate((el) => getComputedStyle(el).transform), {
        timeout: 2000,
      })
      .not.toBe(before)
    // The brand CTA must be reachable by keyboard with a visible ring.
    const cta = page.locator('.VPHero .actions a.VPButton').first()
    let focused = false
    for (let i = 0; i < 25 && !focused; i++) {
      await page.keyboard.press('Tab')
      focused = await cta.evaluate((el) => el === document.activeElement)
    }
    expect(focused, 'brand CTA reachable via Tab').toBe(true)
    expect(
      await cta.evaluate((el) => getComputedStyle(el).outlineStyle),
    ).not.toBe('none')
  })
})

test.describe('the privacy scene — the differential gets its moment', () => {
  for (const [path, label] of [
    ['/korvun/', 'EN'],
    ['/korvun/es/', 'ES'],
  ] as const) {
    test(`${label}: the gray dashed exclusion is on stage`, async ({
      page,
    }) => {
      await page.goto(path)
      const scene = page.locator('.k-privacy')
      await expect(scene).toHaveCount(1)
      await scene.scrollIntoViewIfNeeded()
      await expect(scene.locator('h2')).toBeVisible()
      await expect(scene.locator('[role="img"]')).toHaveAttribute(
        'aria-label',
        /.+/,
      )
      // Both cables: the live violet one and the gray dashed exclusion.
      await expect(scene.locator('.k-cable-live')).toBeVisible()
      await expect(scene.locator('.k-cable-excluded')).toBeVisible()
    })
  }
})