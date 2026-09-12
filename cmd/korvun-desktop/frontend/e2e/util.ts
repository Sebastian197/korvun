// Shared e2e contract values and helpers (review finding: the harness
// origin and the screenshot destination lived in three drifting literals).
import type { Page } from '@playwright/test'

/** The harness's one loopback origin — playwright.config and specs share it. */
export const HARNESS_ADDR = '127.0.0.1:43117'
export const BASE = `http://${HARNESS_ADDR}`

/** A SECOND harness in fresh-install mode (no config loaded) for the SP6c
 * onboarding e2e — created=true is real there. Its own port so it never
 * collides with the running-core harness above. */
export const FRESH_ADDR = '127.0.0.1:43118'
export const FRESH_BASE = `http://${FRESH_ADDR}`

/** A THIRD harness for the approvals screen: it carries the parking brain and
 * the approvals surface, and it needs its own instance because that second
 * brain is visible to every spec on the shared one — the existing specs were
 * written against the profile without it. */
export const APPROVALS_ADDR = '127.0.0.1:43119'
export const APPROVALS_BASE = `http://${APPROVALS_ADDR}`

/** Screenshot destination: design-drafts/, the copilot's review inbox. */
export const SHOT = (name: string): string => `../../../design-drafts/${name}`

/** Screenshots wait for the embedded Geist faces so a cold run never
 * captures the fallback font. */
export async function settleFonts(page: Page): Promise<void> {
  await page.evaluate(() => document.fonts.ready.then(() => undefined))
}
