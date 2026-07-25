import { defineConfig, devices } from '@playwright/test'
import { BASE, FRESH_ADDR, FRESH_BASE, HARNESS_ADDR } from './e2e/util'

// E2E for the desktop chrome (SP6). The webServer is the Go harness: built
// chrome + the SP4 proxy + a REAL no-network core carrying one scripted
// telegram channel and a toggleable fake model endpoint (see the harness's
// package comment), all on one loopback origin — the real pipeline, no
// mocks. Separate from the Go pipeline (ADR-0029 §6). Viewport 1440x900 is
// the screenshot contract of the copilot's side-by-side review.
export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  // ONE worker: every spec shares the single harness core (its lifecycle IS
  // the state under test) — parallel files would fight over it.
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: 'list',
  use: {
    baseURL: BASE,
    trace: 'on-first-retry',
    viewport: { width: 1440, height: 900 },
  },
  webServer: [
    {
      // -start=false: the suite opens on the STOPPED chrome — the parado
      // hero must come from the real 503 contract, not a mock; tests that
      // need a running core start it through the bindings bridge.
      command: `go run ../e2e-harness -addr ${HARNESS_ADDR} -dist ./dist -start=false`,
      url: `${BASE}/`,
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
    },
    {
      // The fresh-install harness (SP6c onboarding): no config loaded, so
      // EnsureDefaultConfig's created=true is real.
      command: `go run ../e2e-harness -addr ${FRESH_ADDR} -dist ./dist -fresh`,
      url: `${FRESH_BASE}/`,
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
    },
  ],
  projects: [
    {
      name: 'chromium',
      // The viewport must ride INSIDE the project use: the Desktop Chrome
      // device spread carries its own 1280x720 and would silently override
      // the top-level 1440x900 screenshot contract (found in 6b: the 6a
      // captures shipped 1280x720 because of exactly this).
      use: {
        ...devices['Desktop Chrome'],
        viewport: { width: 1440, height: 900 },
      },
    },
  ],
})
