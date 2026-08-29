import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, type Page } from '@playwright/test'

const HERE = dirname(fileURLToPath(import.meta.url))

// Deflake tanda 2026-08-29, patient (a) — the ROOT CAUSE of the e2e-binary
// order interference: the suite shares ONE live core whose hot-reload
// PERSISTS every applied config (canvas.spec's master flow leaves a webhook
// channel + brain "nuevo" + its route behind), while later specs assume the
// pristine fixture. This reset re-applies the committed fixture through the
// REAL reload machine before each spec, so every spec starts from the same
// state regardless of order or of a reused server.

const TOKEN = 'e2e-canvas-admin'
const FIXTURE = JSON.parse(readFileSync(join(HERE, 'korvun.e2e.json'), 'utf-8')) as Record<
  string,
  unknown
>

/** Re-apply the pristine fixture config and wait for the reload to land. */
export async function resetToFixture(page: Page): Promise<void> {
  const auth = { Authorization: `Bearer ${TOKEN}` }
  const current = await page.request.get('/api/config', { headers: auth })
  expect(current.ok()).toBe(true)
  const cfg = (await current.json()) as Record<string, unknown>
  // Already pristine (fresh boot): skip the reload cycle entirely.
  if (JSON.stringify(cfg) === JSON.stringify(FIXTURE)) return

  const post = await page.request.post('/api/config', { headers: auth, data: FIXTURE })
  expect(post.status(), 'fixture reset must be accepted').toBe(202)
  const { handle } = (await post.json()) as { handle: string }
  await expect
    .poll(
      async () => {
        const r = await page.request.get(`/api/reload/${handle}`, { headers: auth })
        if (!r.ok()) return 'unknown'
        return ((await r.json()) as { state?: string }).state ?? 'unknown'
      },
      { timeout: 30_000 },
    )
    .toBe('succeeded')
}
