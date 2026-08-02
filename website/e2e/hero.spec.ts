// SP2c gates: the cinematic hero entrance.
//
// THE LAW (unchanged, AS-4b/4c): with reduced motion or without JS the
// hero simply IS — complete, sharp, still, from the first pixel. The
// welcome choreography (the K landing with weight and shadow, the name
// composing, the cascade) exists only on the armed motion path, plays
// once at load (~1.2s), and settles into a slow, subtle float.
import { expect, test, type Locator } from '@playwright/test'

const PAGE = '/korvun/'
// The K mark's animation target is the OUTER .image div — the only box in
// the hero image tree with no static theme transform at ANY breakpoint.
// Found red-first, twice: .image-src carries translate(-50%,-50%), and
// .image-container carries translate(-32px,-32px) at ≥960px — animating
// either overrides the theme transform and displaces the K.
const K = '.VPHero .image'
const CASCADE = [
  '.VPHero .name.clip',
  '.VPHero .text',
  '.VPHero .tagline',
  '.VPHero .actions',
]
const ALL_PARTS = [K, ...CASCADE]

const styleOf = (loc: Locator) =>
  loc.evaluate((el) => {
    const s = getComputedStyle(el)
    return { opacity: parseFloat(s.opacity), transform: s.transform, filter: s.filter }
  })

// "Still" = fully opaque and untransformed. (A resting shadow is allowed
// anywhere; motion is not.)
async function assertHeroStill(page: import('@playwright/test').Page) {
  for (const sel of ALL_PARTS) {
    const { opacity, transform, filter } = await styleOf(page.locator(sel))
    expect(opacity, `${sel} opacity`).toBe(1)
    expect(transform, `${sel} transform`).toBe('none')
    expect(filter, `${sel} must be sharp (no blur)`).not.toContain('blur(')
  }
}

test.describe('AS-4b: reduced motion — the hero simply IS', () => {
  test.use({ contextOptions: { reducedMotion: 'reduce' } })

  test('complete, sharp and still from the first pixel; zero animations', async ({
    page,
  }) => {
    await page.goto(PAGE)
    await assertHeroStill(page)
    expect(await page.evaluate(() => document.getAnimations().length)).toBe(0)
  })
})

test.describe('AS-4c: JavaScript disabled — the hero simply IS', () => {
  test.use({ javaScriptEnabled: false })

  test('complete, sharp and still from the first pixel', async ({ page }) => {
    await page.goto(PAGE)
    await assertHeroStill(page)
  })
})

test.describe('the welcome (motion path)', () => {
  test('the K lands with weight: settled by ~1.5s, resting violet shadow, alive float', async ({
    page,
  }) => {
    await page.goto(PAGE)
    await page.waitForTimeout(1800) // arm + land + cascade complete
    // Everything settled and fully present.
    for (const sel of ALL_PARTS) {
      expect((await styleOf(page.locator(sel))).opacity, sel).toBe(1)
    }
    // The K sits essentially at rest — the float may hold it within its
    // gentle ±3px orbit, never more, never scaled.
    const k = await page.locator(K).evaluate((el) => {
      const t = getComputedStyle(el).transform
      if (t === 'none') return { scale: 1, ty: 0 }
      const m = t.match(/matrix\(([^)]+)\)/)
      const [a, , , d, , ty] = m![1].split(',').map(Number)
      return { scale: (a + d) / 2, ty }
    })
    expect(Math.abs(k.ty)).toBeLessThanOrEqual(4)
    expect(k.scale).toBeGreaterThan(0.99)
    expect(k.scale).toBeLessThan(1.01)
    // The projected shadow rests with it — violet-tinted drop-shadow.
    expect((await styleOf(page.locator(K))).filter).toContain('drop-shadow')
    // The float is ALIVE: an infinite animation runs on the K itself.
    expect(
      await page
        .locator(K)
        .evaluate((el) =>
          el
            .getAnimations()
            .some((a) => a.effect?.getTiming().iterations === Infinity),
        ),
    ).toBe(true)
  })

  test('the cascade: name first (~150ms), then text → tagline → buttons rising in order', async ({
    page,
  }) => {
    await page.goto(PAGE)
    await page.waitForTimeout(600) // armed; delays are computed style
    const delays: number[] = []
    for (const sel of CASCADE) {
      delays.push(
        await page
          .locator(sel)
          .evaluate((el) => parseFloat(getComputedStyle(el).animationDelay)),
      )
    }
    expect(delays[0], 'the name leads (~150ms)').toBeGreaterThanOrEqual(0.1)
    for (let i = 1; i < delays.length; i++) {
      expect(delays[i], `${CASCADE[i]} after ${CASCADE[i - 1]}`).toBeGreaterThan(
        delays[i - 1],
      )
    }
  })

  test('the buttons arrive with their resting shadow', async ({ page }) => {
    await page.goto(PAGE)
    await page.waitForTimeout(1800)
    expect(
      await page
        .locator('.VPHero .actions .VPButton.brand')
        .evaluate((el) => getComputedStyle(el).boxShadow),
    ).not.toBe('none')
  })

  test('the settled K sits exactly where the still K sits — motion never displaces', async ({
    browser,
  }, testInfo) => {
    // The invariant that would have caught the -32px displacement: the
    // welcome may move THROUGH space, but it must land on the exact spot
    // the reduced-motion page renders statically.
    const base = testInfo.project.use.baseURL!
    const still = await browser.newContext({ reducedMotion: 'reduce' })
    const moving = await browser.newContext()
    const p1 = await still.newPage()
    const p2 = await moving.newPage()
    await p1.goto(base + PAGE)
    await p2.goto(base + PAGE)
    await p2.waitForTimeout(1800) // settled, float within its ±3px orbit
    const b1 = await p1.locator('.VPHero .image-src').boundingBox()
    const b2 = await p2.locator('.VPHero .image-src').boundingBox()
    expect(Math.abs(b1!.x - b2!.x)).toBeLessThanOrEqual(1)
    expect(Math.abs(b1!.y - b2!.y)).toBeLessThanOrEqual(4)
    await still.close()
    await moving.close()
  })

  test('the welcome never replays on scroll — it is a load moment', async ({
    page,
  }) => {
    await page.goto(PAGE)
    await page.waitForTimeout(1800)
    await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight))
    await page.waitForTimeout(300)
    await page.evaluate(() => window.scrollTo(0, 0))
    await page.waitForTimeout(300)
    // Back at the top: still settled — no re-entrance, no re-hide.
    for (const sel of ALL_PARTS) {
      expect((await styleOf(page.locator(sel))).opacity, sel).toBe(1)
    }
  })
})
