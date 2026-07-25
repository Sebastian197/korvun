import { defineConfig, devices } from '@playwright/test'

// E2E for the desktop chrome (SP6a). The webServer is the Go harness: built
// chrome + the SP4 proxy + a REAL no-network core (SP5 channel-less
// template) on one loopback origin — the real pipeline, no mocks. Separate
// from the Go pipeline (ADR-0029 §6). Viewport 1440x900 is the screenshot
// contract of the copilot's side-by-side review.
export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: 'list',
  use: {
    baseURL: 'http://127.0.0.1:43117',
    trace: 'on-first-retry',
    viewport: { width: 1440, height: 900 },
  },
  webServer: {
    // -start=false: 6a's screenshots are the STOPPED chrome — the parado
    // hero must come from the real 503 contract, not a mock.
    command: 'go run ../e2e-harness -addr 127.0.0.1:43117 -dist ./dist -start=false',
    url: 'http://127.0.0.1:43117/',
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
})
