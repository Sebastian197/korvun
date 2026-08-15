import { test, expect, type Page, type Route } from '@playwright/test'

// SP6 AS-9 — the governance panel's full hot-promotion round trip against the
// REAL built UI (Vite preview + a mocked control API): open an agent brain, set
// a grant to Ensayo, Aplicar → the POSTed config carries mode:shadow and the
// re-GET returns it; then promote to Permitir, Aplicar → the POST carries
// mode:allow. This is the "shadow→allow promotion IS the usual Apply" contract,
// verified end to end (ADR-0041 §2, ADR-0035 §4 — no separate promote endpoint).

const AGENT_CONFIG = {
  channels: [{ type: 'telegram', mode: 'polling', token_env: 'KORVUN_TG' }],
  brains: [
    {
      name: 'soporte',
      sensitivity: 'private',
      policy: { kind: 'priority' },
      dispatch: 'fanout',
      models: [{ provider: 'ollama', model_id: 'llama3.2:1b', locality: 'local' }],
      agent: {
        tools: ['read_file', 'http_fetch'],
        max_iterations: 5,
        system_prompt: '',
        read_file: { root: '/docs' },
        http_fetch: { allow_hosts: ['api.github.com'] },
      },
    },
  ],
  routes: [{ channel: 'telegram', brain: 'soporte' }],
  admin: { token_env: 'KORVUN_ADMIN_TOKEN' },
}
const BRAINS = [{ name: 'soporte', sensitivity: 'private', policy: 'priority', dispatch: 'fanout', models: [] }]
const CHANNELS = [{ type: 'telegram', mode: 'polling', name: 'telegram' }]

function json(route: Route, status: number, body: unknown) {
  return route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })
}

async function loadAgentBrain(page: Page, current: () => typeof AGENT_CONFIG, onPost: (cfg: typeof AGENT_CONFIG) => void) {
  await page.route('**/api/brains', (r) => json(r, 200, BRAINS))
  await page.route('**/api/channels', (r) => json(r, 200, CHANNELS))
  await page.route('**/api/reload/r1', (r) => json(r, 200, { state: 'succeeded' }))
  await page.route('**/api/config', (r) => {
    if (r.request().method() === 'POST') {
      onPost(JSON.parse(r.request().postData() ?? '{}') as typeof AGENT_CONFIG)
      return json(r, 202, { handle: 'r1' })
    }
    return json(r, 200, current())
  })
  await page.goto('/builder/')
  await page.getByLabel('admin bearer token').fill('secret')
  await page.getByRole('button', { name: 'Load' }).click()
  await expect(page.getByTestId('canvas-surface')).toBeVisible()
  await page.getByTestId('brain:0').click()
  await expect(page.getByTestId('governance-section-0')).toBeVisible()
}

test('hot promotion: Ensayo → Aplicar → shadow persisted → Permitir → Aplicar → allow', async ({ page }) => {
  let served = structuredClone(AGENT_CONFIG)
  const posts: Array<typeof AGENT_CONFIG> = []
  const onPost = (cfg: typeof AGENT_CONFIG) => {
    posts.push(cfg)
    served = structuredClone(cfg) // the re-GET after a succeeded reload returns the applied config
  }
  await loadAgentBrain(page, () => served, onPost)

  // 1) read_file → Ensayo, then Aplicar.
  await page.getByTestId('tri-read_file-shadow').click()
  await page.getByRole('button', { name: /aplicar/i }).click()
  await expect(page.getByTestId('reload-succeeded')).toBeVisible({ timeout: 30_000 })

  expect(posts.at(-1)!.brains[0].agent!.governance).toEqual([{ tool: 'read_file', mode: 'shadow' }])

  // 2) After the succeeded reload the panel re-baselines from the served (shadow)
  // config: the Ensayo segment is active. Re-open the brain and promote.
  await page.getByTestId('brain:0').click()
  await expect(page.getByTestId('tri-read_file-shadow')).toHaveAttribute('aria-pressed', 'true')

  await page.getByTestId('tri-read_file-allow').click()
  await page.getByRole('button', { name: /aplicar/i }).click()
  await expect(page.getByTestId('reload-succeeded')).toBeVisible({ timeout: 30_000 })

  expect(posts.at(-1)!.brains[0].agent!.governance).toEqual([{ tool: 'read_file', mode: 'allow' }])
})
