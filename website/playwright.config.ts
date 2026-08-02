import { defineConfig } from '@playwright/test'

// Landing e2e over `vitepress preview` — the BUILT site served under the
// real '/korvun/' project-page base (spec AS-1 realism: never a root dev
// server). The ADR-0029 §5 pattern rides on this config: the same-origin
// assertion in e2e/landing.spec.ts is the zero-CDN gate, not a text grep.
export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: 0,
  // OWN port on purpose: a long-lived manual `npm run preview` (4173, for
  // human review) serves stale in-memory assets after a rebuild, and
  // reuseExistingServer would trust it — the e2e must never share a port
  // with a server it did not start (bitten twice by this).
  use: {
    baseURL: 'http://localhost:4174',
  },
  webServer: {
    command: 'npm run preview -- --port 4174',
    url: 'http://localhost:4174/korvun/',
    reuseExistingServer: !process.env.CI,
  },
})
