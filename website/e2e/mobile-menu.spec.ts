import { expect, test } from '@playwright/test'

// Regression pin for the mobile drawer collapse: backdrop-filter applied to
// .navbar made the navbar the containing block for its fixed-position
// children, so .navbar-sidebar (a fixed child of the navbar in Docusaurus)
// resolved top/bottom against the 60px bar and rendered as an empty sliver.
// The pin asserts the drawer's real geometry, then the full user journey:
// open → navigate from the drawer → auto-close → reopen → close with the X.
const locales = [
  {
    label: 'EN',
    docPath: '/korvun/guide/quickstart/',
    installHref: '/korvun/guide/install/',
  },
  {
    label: 'ES',
    docPath: '/korvun/es/guide/quickstart/',
    installHref: '/korvun/es/guide/install/',
  },
] as const

test.describe('390 px navbar drawer', () => {
  test.use({ viewport: { width: 390, height: 844 } })

  for (const locale of locales) {
    test(`${locale.label} drawer opens full-height, navigates, and closes`, async ({
      page,
    }) => {
      await page.goto(locale.docPath)

      await page.locator('.navbar__toggle').click()
      const drawer = page.locator('.navbar-sidebar')
      await expect(drawer).toBeVisible()

      // The containing-block regression: a broken drawer still gets the
      // --show class but measures the navbar's height, not the viewport's.
      const box = await drawer.boundingBox()
      expect(box!.height).toBeGreaterThanOrEqual(844)

      // On a docs page the drawer shows the doc sidebar; its links must be
      // actually tappable (the collapsed drawer left them unreachable).
      const installLink = page.locator(
        `.navbar-sidebar a[href="${locale.installHref}"]`,
      )
      await expect(installLink).toBeVisible()
      await installLink.click()
      await expect(page).toHaveURL(new RegExp(`${locale.installHref}$`))
      await expect(page.locator('.navbar-sidebar--show')).toHaveCount(0)

      // Reopen and close with the X.
      await page.locator('.navbar__toggle').click()
      await expect(page.locator('.navbar-sidebar--show')).toHaveCount(1)
      await page.locator('.navbar-sidebar__close').click()
      await expect(page.locator('.navbar-sidebar--show')).toHaveCount(0)
    })
  }
})
