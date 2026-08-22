/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// Served from /builder on the Korvun admin server (ADR-0030 §4); `base` makes the
// built asset URLs resolve under that path. Output goes to dist/, which the Go
// binary embeds via go:embed (ADR-0029 §4). Zero CDN: fonts and CSS are bundled
// locally by Vite, no external <link>/<script>.
export default defineConfig({
  base: '/builder/',
  plugins: [react(), tailwindcss()],
  build: { outDir: 'dist', emptyOutDir: true },
  // Dev-only (option b: `npm run dev` with HMR): proxy the control API to a real
  // Korvun admin server on loopback :2112, so the UI talks to a live backend while
  // you tinker. Does NOT affect the production build (which is same-origin, zero CDN).
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:2112',
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    include: ['src/**/*.{test,spec}.{ts,tsx}'],
    coverage: {
      provider: 'v8',
      // Production code only: tests and declaration files are out; config
      // files (vite/eslint/playwright) live outside src/ already.
      include: ['src/**'],
      exclude: ['src/**/*.{test,spec}.{ts,tsx}', 'src/**/*.d.ts'],
      // Same rule as the desktop chrome (SP6b): floors sit just under the
      // measured baseline (86.09/78.53/76.88/86.83, 2026-08-22) and only
      // ratchet up — a real regression fails CI, noise does not.
      thresholds: { statements: 83, branches: 74, functions: 72, lines: 83 },
    },
  },
})
