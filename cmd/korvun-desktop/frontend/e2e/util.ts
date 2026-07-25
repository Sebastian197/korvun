// Shared e2e contract values and helpers (review finding: the harness
// origin and the screenshot destination lived in three drifting literals).
import type { Page } from '@playwright/test'

/** The harness's one loopback origin — playwright.config and specs share it. */
export const HARNESS_ADDR = '127.0.0.1:43117'
export const BASE = `http://${HARNESS_ADDR}`

/** Screenshot destination: design-drafts/, the copilot's review inbox. */
export const SHOT = (name: string): string => `../../../design-drafts/${name}`

/** Screenshots wait for the embedded Geist faces so a cold run never
 * captures the fallback font. */
export async function settleFonts(page: Page): Promise<void> {
  await page.evaluate(() => document.fonts.ready.then(() => undefined))
}
