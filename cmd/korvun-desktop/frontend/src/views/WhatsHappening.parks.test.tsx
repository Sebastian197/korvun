// v0.16.2 · train E, the pre-PR gate: the green row over a ledger that blocks.
//
// Over a profile with approvals ON, the screen painted D2 and, with it, the
// green row «Una acción irreversible se aparca y te espera» — «1 de 1 cerebros
// pueden aparcar». With the ledger unreadable, or its environment failing, no
// act can be recorded, so no irreversible action would park: the green sentence
// contradicted D2 on the same screen. The batch-4 moulds never saw it: they
// stood up approvals OFF, where the green row is not drawn, and their one leg
// with approvals ON asserted the controls only.
//
// The contract (director's order, 2026-09-27): with the two blocking states,
// `unreadable` and `environment`, the green row is not drawn and the cages are
// drawn with their controls disabled. With `unavailable` nothing changes: the
// green row stays and the controls stay active.
//
// The three ledger bodies are the real app's GET /api/whats-happening, which
// TE49's Go half captured into ./fixtures. The gate and the config are this
// file's fixtures, written by hand.
//
// Evidence level: jsdom, with fetch replaced; in-process. The packaged app is
// TE50's.
//
// PROBING MUTATION: draw the green row whenever nothing blocks the park,
// without looking at the ledger. The unreadable and environment cases redden.
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { WhatsHappening } from './WhatsHappening'

interface Body {
  rows: unknown[]
  conditions: Record<string, string>
  ledger?: { standing: string; owner: string; path?: string }
}

/** The real GET body the Go half captured for state. */
function realBody(state: 'unreadable' | 'environment' | 'unavailable'): Body {
  return JSON.parse(
    readFileSync(join(__dirname, 'fixtures', `whats-happening-${state}.json`), 'utf8'),
  ) as Body
}

interface Route {
  status: number
  body: unknown
}

/** Replaces fetch with a table of routes. */
function routes(table: Record<string, Route>): void {
  vi.stubGlobal('fetch', (url: string) => {
    const r = table[url] ?? { status: 404, body: { error: 'not found' } }
    return Promise.resolve(new Response(JSON.stringify(r.body), { status: r.status }))
  })
}

/** Approvals ON, one brain that can park, nothing blocking the park. */
const APPROVALS_ON: Route = {
  status: 200,
  body: { gate: { approvals_enabled: true, brains_total: 1, brains_can_park: 1, blocked: [] } },
}

/** One brain with one cage: a single «Añadir host». */
const CONFIG: Route = {
  status: 200,
  body: { brains: [{ name: 'b', agent: { webhook_call: { allow_hosts: ['a.io'] } } }] },
}

const GREEN = 'Una acción irreversible se aparca y te espera'
const PARKS = 'se aparca'

/** Renders the screen over state's real body and returns its text once the
 * cage's button is on screen. */
async function over(
  state: 'unreadable' | 'environment' | 'unavailable',
): Promise<{ text: string; host: HTMLElement }> {
  const body = realBody(state)
  // Calibration: the body is the state it names.
  expect(body.ledger?.standing).toBe(state)
  routes({
    '/api/whats-happening': { status: 200, body },
    '/api/approvals': APPROVALS_ON,
    '/api/config': CONFIG,
  })
  const { container } = render(<WhatsHappening />)
  const host = await screen.findByRole('button', { name: 'Añadir host' })
  return { text: container.textContent ?? '', host }
}

describe('WhatsHappening · the green row over a ledger that blocks', () => {
  beforeEach(() => {
    vi.unstubAllGlobals()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('unreadable (D2): no «se aparca», and the cage is drawn disabled', async () => {
    const { text, host } = await over('unreadable')
    expect(screen.getByTestId('whats-ledger-d2')).toBeInTheDocument()
    expect(text).not.toContain(PARKS)
    expect(host).toBeDisabled()
  })

  it('environment (D3-entorno): no «se aparca», and the cage is drawn disabled', async () => {
    const { text, host } = await over('environment')
    expect(screen.getByTestId('whats-ledger-environment')).toBeInTheDocument()
    expect(text).not.toContain(PARKS)
    expect(host).toBeDisabled()
  })

  it('unavailable (D3-del-momento): the green row stays, and the cage stays active', async () => {
    const { text, host } = await over('unavailable')
    expect(screen.getByTestId('whats-ledger-unavailable')).toBeInTheDocument()
    expect(text).toContain(GREEN)
    expect(host).not.toBeDisabled()
  })
})
