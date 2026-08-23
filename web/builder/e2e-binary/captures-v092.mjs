// v0.9.2 wave-1 "after" captures against the real binary, into design-drafts/
// (the copilot's review inbox — the SP4/SP6 captures precedent). The paired
// "before" is the audit set design-drafts/builder-audit/v2-0*.png. Run with a
// serving binary whose model is warmup:true against a dead port (the N6
// fixture), e.g.:
//   KORVUN_ADMIN_TOKEN=e2e-canvas-admin ./korvun serve --config <fixture>
//   node e2e-binary/captures-v092.mjs [origin]
import { chromium } from '@playwright/test'

const ORIGIN = process.argv[2] ?? 'http://127.0.0.1:2113'
const OUT = '../../design-drafts'
const TOKEN = 'e2e-canvas-admin'

const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } })
await page.goto(`${ORIGIN}/builder/`)
await page.getByLabel('admin bearer token').fill(TOKEN)
await page.getByRole('button', { name: 'Load' }).click()
await page.getByTestId('canvas-surface').waitFor()
await page.evaluate(() => document.fonts.ready.then(() => undefined))

// N6: the dead model's badge (the fixture's warmup fails fast on a dead port).
await page.getByTestId('badge-health').waitFor({ timeout: 15_000 })
await page.waitForTimeout(400) // fitView settle
await page.screenshot({ path: `${OUT}/v092-n6-badge-unreachable.png` })

// B7: a harmless model_id change applied through the REAL cutover — the
// truthful succeeded chip (captured BEFORE the B8 pending edits below, which
// deliberately stay unapplied).
await page.getByTestId('model:0.0').click()
await page.getByLabel('model_id').fill('llama3.2:1b-v092')
await page.getByRole('button', { name: /aplicar/i }).click()
await page.getByTestId('reload-succeeded').waitFor({ timeout: 30_000 })
await page.screenshot({ path: `${OUT}/v092-b7-reload-succeeded.png` })

// B8: cable a route and SELECT it — the accent stroke that used to be
// swallowed. The fixture is channel-less: create one from the palette first.
await page.dragAndDrop('[data-testid="palette:channel"]', '[data-testid="canvas-surface"]')
await page.getByTestId('channel:0').waitFor()
const src = await page
  .locator('[data-testid="channel:0"] ~ .react-flow__handle.source, [data-testid="channel:0"] .react-flow__handle.source')
  .first()
  .boundingBox()
const dst = await page.locator('[data-testid="brain:0"] .react-flow__handle.target').first().boundingBox()
if (!src || !dst) throw new Error('handles not measurable')
await page.mouse.move(src.x + src.width / 2, src.y + src.height / 2)
await page.mouse.down()
await page.mouse.move(dst.x + dst.width / 2, dst.y + dst.height / 2, { steps: 12 })
await page.mouse.up()
const route = page.locator('.react-flow__edge[data-id^="route:"]').first()
await route.waitFor()
const pt = await route.locator('.react-flow__edge-path').first().evaluate((p) => {
  const path = p
  const mid = path.getPointAtLength(path.getTotalLength() / 2)
  const ctm = path.getScreenCTM()
  const sp = new DOMPoint(mid.x, mid.y).matrixTransform(ctm)
  return { x: sp.x, y: sp.y }
})
await page.mouse.click(pt.x, pt.y)
await page.locator('.react-flow__edge.selected[data-id^="route:"]').waitFor()
await page.screenshot({ path: `${OUT}/v092-b8-selected-cable.png` })

// The B8 edits stay PENDING on purpose (the run serves a throwaway config
// copy); the served config keeps only the applied B7 model_id change.
await browser.close()
console.log(
  'captures: v092-n6-badge-unreachable.png, v092-b8-selected-cable.png, v092-b7-reload-succeeded.png → design-drafts/',
)
