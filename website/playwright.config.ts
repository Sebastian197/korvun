import { defineConfig } from '@playwright/test'

// Landing e2e over `vitepress preview` — the BUILT site served at the
// real domain-root base (the single canonical edition since the
// 2026-08-31 cutover; never a dev server). The ADR-0029 §5 pattern rides on this config: the same-origin
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
    url: 'http://localhost:4174/',
    reuseExistingServer: !process.env.CI,
  },
})
