// SP0 canvas spike evidence script (ADR-0039). NOT part of the playwright
// suite (not *.spec.ts) — run by hand against the REAL Go binary:
//   KORVUN_ADMIN_TOKEN=... ./korvun serve --config <minimal config with admin>
//   node e2e/spike-evidence.mjs [origin] [shot-dir]
// Exercises /builder/?spike=flow and prints a JSON verdict for criteria a-d:
// CSP violations, external requests, node drag, connection create+validate,
// light/dark theme, axe.
import { chromium } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'

const ORIGIN = process.argv[2] ?? 'http://127.0.0.1:2112'
const SHOTS = process.argv[3] ?? '.'

const browser = await chromium.launch()
const context = await browser.newContext({ viewport: { width: 1280, height: 800 } })
const page = await context.newPage()

const external = []
page.on('request', (req) => {
  const u = req.url()
  if (!u.startsWith(ORIGIN) && !u.startsWith('data:') && !u.startsWith('blob:')) external.push(u)
})
const consoleIssues = []
page.on('console', (m) => {
  if (m.type() === 'error' || m.type() === 'warning') consoleIssues.push(`${m.type()}: ${m.text()}`)
})
page.on('pageerror', (e) => consoleIssues.push(`pageerror: ${e.message}`))
await page.addInitScript(() => {
  window.__cspViolations = []
  document.addEventListener('securitypolicyviolation', (e) => {
    window.__cspViolations.push(
      `${e.violatedDirective} blocked=${e.blockedURI} at ${e.sourceFile}:${e.lineNumber}`,
    )
  })
})

const resp = await page.goto(`${ORIGIN}/builder/?spike=flow`)
const cspHeader = resp.headers()['content-security-policy'] ?? null
await page.waitForSelector('[data-testid="flow-spike"]')
await page.waitForSelector('.react-flow__node[data-id="b"]')
await page.waitForTimeout(300) // let fitView settle

const center = (box) => ({ x: box.x + box.width / 2, y: box.y + box.height / 2 })
const dragBetween = async (from, to) => {
  await page.mouse.move(from.x, from.y)
  await page.mouse.down()
  await page.mouse.move(to.x, to.y, { steps: 12 })
  await page.mouse.up()
  await page.waitForTimeout(150)
}

const nodeCount = await page.locator('.react-flow__node').count()
const initialEdges = await page.locator('.react-flow__edge').count()

// b1 — drag node b and verify it actually moved
const nodeB = page.locator('.react-flow__node[data-id="b"]')
const beforeBox = await nodeB.boundingBox()
const start = center(beforeBox)
await dragBetween(start, { x: start.x + 90, y: start.y + 60 })
const afterBox = await nodeB.boundingBox()
const dragDelta = { dx: afterBox.x - beforeBox.x, dy: afterBox.y - beforeBox.y }
const dragWorks = Math.abs(dragDelta.dx) > 40 && Math.abs(dragDelta.dy) > 20

// b2 — create a valid connection b(source) -> a(target)
const bSource = await page
  .locator('.react-flow__node[data-id="b"] .react-flow__handle.source')
  .boundingBox()
const aTarget = await page
  .locator('.react-flow__node[data-id="a"] .react-flow__handle.target')
  .boundingBox()
await dragBetween(center(bSource), center(aTarget))
const edgesAfterConnect = await page.locator('.react-flow__edge').count()

// b3 — invalid self-connection a(source) -> a(target) must be rejected
const aSource = await page
  .locator('.react-flow__node[data-id="a"] .react-flow__handle.source')
  .boundingBox()
await dragBetween(center(aSource), center(aTarget))
const edgesAfterInvalid = await page.locator('.react-flow__edge').count()

// c — theme: dark (default) then light, canvas class follows, house token flips
const readTheme = () =>
  page.evaluate(() => ({
    dataTheme: document.documentElement.dataset.theme,
    base: getComputedStyle(document.documentElement).getPropertyValue('--base').trim(),
    canvasClass: document.querySelector('.react-flow')?.className ?? '',
  }))
const darkState = await readTheme()
await page.screenshot({ path: `${SHOTS}/spike-dark.png` })
await page.click('[data-testid="theme-toggle"]')
await page.waitForTimeout(200)
const lightState = await readTheme()
await page.screenshot({ path: `${SHOTS}/spike-light.png` })

// d — axe on the spike view (in light; rerun in dark after toggling back)
const axeLight = await new AxeBuilder({ page }).include('.flow-spike').analyze()
await page.click('[data-testid="theme-toggle"]')
await page.waitForTimeout(200)
const axeDark = await new AxeBuilder({ page }).include('.flow-spike').analyze()

const cspViolations = await page.evaluate(() => window.__cspViolations)

const summary = {
  origin: ORIGIN,
  cspHeader,
  criteria: {
    a_cspViolations: cspViolations,
    a_externalRequests: external,
    a_consoleIssues: consoleIssues,
    b_nodeCount: nodeCount,
    b_initialEdges: initialEdges,
    b_dragDelta: dragDelta,
    b_dragWorks: dragWorks,
    b_edgesAfterConnect: edgesAfterConnect,
    b_edgesAfterInvalidAttempt: edgesAfterInvalid,
    c_dark: darkState,
    c_light: lightState,
    d_axeViolationsDark: axeDark.violations,
    d_axeViolationsLight: axeLight.violations,
  },
  pass:
    cspViolations.length === 0 &&
    external.length === 0 &&
    nodeCount === 2 &&
    initialEdges === 1 &&
    dragWorks &&
    edgesAfterConnect === 2 &&
    edgesAfterInvalid === 2 &&
    darkState.dataTheme === 'dark' &&
    lightState.dataTheme === 'light' &&
    darkState.base !== lightState.base &&
    axeDark.violations.length === 0 &&
    axeLight.violations.length === 0,
}
console.log(JSON.stringify(summary, null, 2))
await browser.close()
process.exit(summary.pass ? 0 : 1)
