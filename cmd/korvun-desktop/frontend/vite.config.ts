/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// The desktop chrome (SP6a). Served from the Wails AssetServer root ('/');
// output goes to dist/, embedded via //go:embed all:frontend/dist (ADR-0029 §4
// stub pattern keeps a clean clone compiling). Zero CDN: Geist fonts and CSS
// are bundled locally by Vite (ADR-0029 §5), no external <link>/<script>.
export default defineConfig({
  base: '/',
  plugins: [react(), tailwindcss()],
  build: { outDir: 'dist', emptyOutDir: true },
  // Dev-only: `npm run dev` proxies the admin surface to a running harness
  // (`go run ../e2e-harness`) so the chrome talks to a real core while you
  // tinker. Does NOT affect the production build.
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:43117',
      '/healthz': 'http://127.0.0.1:43117',
      '/metrics': 'http://127.0.0.1:43117',
      '/builder': 'http://127.0.0.1:43117',
      '/ui': 'http://127.0.0.1:43117',
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['src/test.setup.ts'],
    include: ['src/**/*.{test,spec}.{ts,tsx}'],
  },
})
