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
  use: {
    baseURL: 'http://localhost:4173',
  },
  webServer: {
    command: 'npm run preview',
    url: 'http://localhost:4173/korvun/',
    reuseExistingServer: !process.env.CI,
  },
})
