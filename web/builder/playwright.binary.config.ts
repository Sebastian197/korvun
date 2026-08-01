import { defineConfig, devices } from '@playwright/test'

// SP4 e2e AGAINST THE REAL BINARY (builder-canvas): `make build` compiles the
// frontend into dist/, go:embed carries it, and `korvun serve` mounts /builder
// with the REAL CSP and the REAL control API — no vite preview, no mocked
// routes (that suite stays in ./e2e with its own config). Channel-less
// fixture + admin token so /builder mounts; the dummy env vars exist only so
// the fixture's secrets RESOLVE at boot — no network is ever exercised (the
// only channel the master flow creates is a loopback webhook).
export default defineConfig({
  testDir: './e2e-binary',
  fullyParallel: false,
  workers: 1, // one shared core: the master flow mutates the live config
  reporter: 'list',
  timeout: 30_000,
  use: {
    baseURL: 'http://127.0.0.1:2112',
    viewport: { width: 1440, height: 900 },
  },
  webServer: {
    // The core WRITES the applied config back to its file (atomic persist on
    // hot-reload), so the master flow would mutate the committed fixture. The
    // server therefore runs on a throwaway COPY; korvun.e2e.json stays
    // pristine and every run starts from the same state.
    command:
      "sh -c 'cd ../.. && make build && cp web/builder/e2e-binary/korvun.e2e.json web/builder/e2e-binary/korvun.e2e.run.json && KORVUN_ADMIN_TOKEN=e2e-canvas-admin GROQ_E2E_KEY=dummy-never-called KORVUN_E2E_HOOK=dummy-inbound ./korvun serve --config web/builder/e2e-binary/korvun.e2e.run.json'",
    url: 'http://127.0.0.1:2112/healthz',
    reuseExistingServer: !process.env.CI,
    timeout: 180_000,
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 900 } } }],
})
