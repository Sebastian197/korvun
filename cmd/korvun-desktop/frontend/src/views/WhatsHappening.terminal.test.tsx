// v0.16.2 · train E, the pre-PR gate, finding B: one outcome, one sentence.
//
// The packaged pass (TE50) caught the row saying «Hecho. El cambio está en
// marcha y guardado en tu perfil.» and, right after it, the applying answer's
// «aplicando; tu perfil en disco todavía no ha cambiado», over a change that had
// been applied and saved. The screen kept the POST's `detail` once the poll
// reached a terminal state. The moulds that poll never saw it: their applying
// answers carried no `detail`, and the real door always sends one.
//
// The contract (director's order, 2026-09-27): when the poll reaches a terminal
// state, the row shows ONLY that outcome's sentence, and the applying `detail`
// does not survive it. A failed or rolled-back cutover keeps its own sentence,
// the one the polled state maps to. The founding door is covered too: its
// applying `detail` also names the ledger's path, and after the poll the path is
// not shown. That is a known limit; train G's planned cure moves the path to a
// field of its own.
//
// The applying answers are the real doors' bodies. The Go half,
// internal/controlapi/whats_happening_applying_wire_test.go, takes them from the
// handlers with the supervisor held in `pending` and compares them with
// ./fixtures. The expected sentences are copied here, not imported from the
// screen, so a change to OUTCOME_ES reddens this file as well.
//
// Evidence level: jsdom, with fetch replaced; in-process. The packaged app with
// a real core is TE50's.
//
// PROBING MUTATION: keep the POST's `detail` when the poll reaches a terminal
// state (drop its reset in `press`). Every case reddens on the row's text.
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { WhatsHappening } from './WhatsHappening'

interface Applying {
  outcome: string
  applied: boolean
  profile_unchanged: boolean
  handle: string
  detail: string
  act: { action_id: string }
}

/** A real applying answer, as the Go half took it from the door. */
function applying(name: 'applying' | 'applying-founding'): Applying {
  return JSON.parse(
    readFileSync(join(__dirname, 'fixtures', `whats-happening-${name}.json`), 'utf8'),
  ) as Applying
}

// Each outcome's sentence, verbatim.
const DONE = 'Hecho. El cambio está en marcha y guardado en tu perfil.'
const NOT_APPLIED = 'No se aplicó. Tu perfil sigue exactamente como estaba.'
const NOT_SAVED =
  'El cambio está en marcha pero NO se pudo guardar: al reiniciar Korvun volverá atrás.'
/** What the applying answer says, and no terminal row may keep. */
const STALE = 'todavía no ha cambiado'

/** The contract as the read door serves it, trimmed to the rows these moulds
 * touch. The shape is the Go struct's JSON, not an invention. */
const CONTRACT = {
  rows: [
    {
      rule: 'store',
      label: 'Almacén de acciones',
      profile_key: 'storage.path',
      has_button: true,
      door: 'enable-storage',
      why: 'crea el libro en la carpeta de usuario de Korvun',
    },
    {
      rule: 'approvals_disabled',
      label: 'Aprobaciones',
      profile_key: 'approvals.enabled',
      has_button: true,
      door: 'enable-approvals',
    },
    {
      rule: 'effect_ceiling',
      label: 'Techo de efecto',
      profile_key: 'brains[].agent.effect_ceiling',
      has_button: true,
      door: 'set-ceiling',
    },
    {
      rule: 'tool_shadowed',
      label: 'Herramienta en sombra',
      profile_key: 'brains[].agent.governance[].mode',
      has_button: true,
      door: 'lift-shadow',
    },
    {
      rule: 'cage',
      label: 'Jaula de la herramienta',
      profile_key: 'brains[].agent.<tool>.allow_hosts · .root',
      has_button: true,
      door: 'allow-host',
      after_approval: true,
    },
    {
      rule: 'private_network_shield',
      label: 'Escudo de red privada',
      profile_key: 'se deriva de brains[].sensitivity y del atributo network de la herramienta',
      has_button: false,
      after_approval: true,
      why: 'NO tiene clave propia',
    },
    {
      rule: 'deny',
      label: 'Herramienta denegada',
      profile_key: 'brains[].agent.governance[].mode',
      has_button: false,
      why: 'una denegación explícita se levanta mirando el perfil',
    },
    {
      rule: 'ledger_foreign_profile',
      label: 'Libro de otro perfil',
      profile_key: 'storage.path',
      has_button: true,
      door: 'adopt-ledger',
      why: 'el libro lo fundó o adoptó otro perfil',
    },
  ],
  conditions: {
    store: 'store',
    agent: 'agent',
    ceiling: 'effect_ceiling',
    tool: 'effect_undeclared',
    governance_denies: 'deny',
    tool_shadowed: 'tool_shadowed',
  },
}

interface Route {
  status: number
  body: unknown
}

/** Replaces fetch with a table of routes, and records every POST. A route may
 * be a SEQUENCE of answers, consumed in order (the last one repeats): how a
 * status door that says «pending» twice and then «succeeded» is stood up. */
function routes(table: Record<string, Route | Route[]>): {
  posts: { url: string; body: unknown }[]
  gets: Record<string, number>
} {
  const posts: { url: string; body: unknown }[] = []
  const gets: Record<string, number> = {}
  const cursors: Record<string, number> = {}
  vi.stubGlobal('fetch', (url: string, init?: RequestInit) => {
    if (init?.method === 'POST') {
      posts.push({ url, body: JSON.parse(String(init.body)) as unknown })
    } else {
      gets[url] = (gets[url] ?? 0) + 1
    }
    const entry = table[url]
    let r: Route
    if (entry === undefined) r = { status: 404, body: { error: 'not found' } }
    else if (Array.isArray(entry)) {
      const i = cursors[url] ?? 0
      r = entry[Math.min(i, entry.length - 1)]
      cursors[url] = i + 1
    } else r = entry
    return Promise.resolve(new Response(JSON.stringify(r.body), { status: r.status }))
  })
  return { posts, gets }
}

const CONTRACT_OK: Route = { status: 200, body: CONTRACT }

/** One click, with React's queue drained. The element is resolved BEFORE the
 * act() wrapper on purpose: findBy* polls, and polling inside act fights the
 * microtask flush — the query times out on a button that is right there. */
async function clickOnce(el: HTMLElement): Promise<void> {
  await act(async () => {
    fireEvent.click(el)
  })
}

describe('WhatsHappening · one outcome, one sentence', () => {
  beforeEach(() => {
    vi.unstubAllGlobals()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  for (const [state, sentence] of [
    ['succeeded', DONE],
    ['rolled-back', NOT_APPLIED],
    ['failed', NOT_APPLIED],
    ['persist-failed', NOT_SAVED],
  ] as const) {
    it(`an ordinary door polled to «${state}» says only that outcome's sentence`, async () => {
      const body = applying('applying')
      // Calibration: the answer the screen polls from does carry the sentence
      // under attack; without it this case would pass over nothing.
      expect(body.outcome).toBe('applying')
      expect(body.detail).toContain(STALE)
      routes({
        '/api/whats-happening': CONTRACT_OK,
        '/api/approvals': {
          status: 200,
          body: { gate: { approvals_enabled: false, brains_total: 1, brains_can_park: 0 } },
        },
        '/api/whats-happening/enable-approvals': { status: 200, body },
        [`/api/reload/${body.handle}`]: [
          { status: 200, body: { state: 'pending' } },
          {
            status: 200,
            body: { state, action_id: body.act.action_id, receipt_id: 'rcpt_b' },
          },
        ],
      })
      render(<WhatsHappening />)
      await clickOnce(await screen.findByRole('button', { name: 'Encender aprobaciones' }))

      const row = await screen.findByTestId('whats-outcome', {}, { timeout: 4000 })
      expect(row.textContent).toBe(sentence)
      expect(row.textContent).not.toContain(STALE)
    })
  }

  it('the founding door polled to «succeeded» says only its sentence', async () => {
    const body = applying('applying-founding')
    expect(body.outcome).toBe('applying')
    expect(body.detail).toContain(STALE)
    expect(body.detail).toContain('El libro vive en')
    routes({
      '/api/whats-happening': CONTRACT_OK,
      '/api/whats-happening/enable-storage': { status: 200, body },
      [`/api/reload/${body.handle}`]: [
        { status: 200, body: { state: 'pending' } },
        {
          status: 200,
          body: { state: 'succeeded', action_id: body.act.action_id, receipt_id: 'rcpt_f' },
        },
      ],
    })
    render(<WhatsHappening />)
    await clickOnce(await screen.findByRole('button', { name: 'Activar almacén' }))

    const row = await screen.findByTestId('whats-outcome', {}, { timeout: 4000 })
    expect(row.textContent).toBe(DONE)
    expect(row.textContent).not.toContain(STALE)
  })
})
