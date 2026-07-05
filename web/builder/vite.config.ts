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
  test: {
    environment: 'jsdom',
    globals: true,
    include: ['src/**/*.{test,spec}.{ts,tsx}'],
  },
})
