import { defineConfig, devices } from '@playwright/test'

// E2E for the builder. The control API is mocked deterministically per-test with
// page.route (no real Korvun binary), so the reload state machine — including the
// ECONNREFUSED-during-cutover retry (2b.2b) — is exercised end-to-end without flake.
// This job is SEPARATE from the Go pipeline (ADR-0029 §6): Node/Playwright never
// gates the cross-compile or the release.
export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: 'list',
  use: {
    baseURL: 'http://localhost:4173',
    trace: 'on-first-retry',
  },
  webServer: {
    command: 'npm run build && npm run preview -- --port 4173 --strictPort',
    url: 'http://localhost:4173/builder/',
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
})
