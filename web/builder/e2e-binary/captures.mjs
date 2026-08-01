// SP4 criterion k: side-by-side captures of the REAL canvas against the
// final-6 mockups, into design-drafts/ (the copilot's review inbox — the SP6
// screenshot precedent). Run with the binary serving (the suite's webServer
// command, or by hand):
//   KORVUN_ADMIN_TOKEN=e2e-canvas-admin GROQ_E2E_KEY=dummy \
//     ./korvun serve --config web/builder/e2e-binary/korvun.e2e.run.json
//   node e2e-binary/captures.mjs [origin]
import { chromium } from '@playwright/test'

const ORIGIN = process.argv[2] ?? 'http://127.0.0.1:2112'
const OUT = '../../design-drafts'
const TOKEN = 'e2e-canvas-admin'

const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } })
await page.goto(`${ORIGIN}/builder/`)
await page.getByLabel('admin bearer token').fill(TOKEN)
await page.getByRole('button', { name: 'Load' }).click()
await page.getByTestId('canvas-surface').waitFor()
await page.evaluate(() => document.fonts.ready.then(() => undefined))
await page.waitForTimeout(400) // fitView settle

await page.screenshot({ path: `${OUT}/sp4-canvas-dark.png` })

await page.getByTestId('brain:0').click()
await page.getByTestId('properties-panel').waitFor()
await page.screenshot({ path: `${OUT}/sp4-canvas-panel.png` })

await page.evaluate(() => {
  document.documentElement.dataset.theme = 'light'
})
await page.waitForTimeout(200)
await page.screenshot({ path: `${OUT}/sp4-canvas-light.png` })

await browser.close()
console.log('captures: sp4-canvas-dark.png, sp4-canvas-panel.png, sp4-canvas-light.png → design-drafts/')
