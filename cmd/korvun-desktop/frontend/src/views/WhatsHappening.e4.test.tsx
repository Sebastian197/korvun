// v0.16.2 · train E, batch 4 (GE7) — the screen's half of the ledger's three
// blocked states: TE49, TE55 (the renderer), TE56 (the renderer), TE60 and
// TE66 (plan v3, §6 and §7, and the director's order for batch 4).
//
// The copy is the approved one, verbatim: D2 (plan §7) with the sentence «Este
// fichero contiene el libro de actos y las conversaciones de este perfil.»
// before its replacement instruction; D3-entorno ending «Mientras tanto, los
// botones de esta pantalla no registran nada.»; D3-del-momento (plan §7), with
// the controls active. The three ledger bodies are the real app's GET
// /api/whats-happening, captured by the Go mould TestE4_TE49_theRealGETIsWhat-
// TheScreenRenders into ./fixtures, byte for byte after the ledger's
// directory is replaced by /perfil.
//
// Evidence level: jsdom, with fetch replaced; the bodies are the real wire,
// the rest of each route (the gate, the config) is this file's fixture. A
// component-level mould is labelled as such.
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { BlockedRow, WhatsHappening } from './WhatsHappening'

interface Ledger {
  standing: string
  owner: string
  path?: string
}
interface Body {
  rows: { rule: string; label: string; has_button: boolean; door?: string; profile_key: string }[]
  conditions: Record<string, string>
  ledger?: Ledger
}

/** The real GET body the Go half captured for state. */
function realBody(state: 'unreadable' | 'environment' | 'unavailable'): Body {
  return JSON.parse(
    readFileSync(join(__dirname, 'fixtures', `whats-happening-${state}.json`), 'utf8'),
  ) as Body
}

// The approved copy, verbatim.
const D2 = (causa: string, ruta: string): string =>
  `El libro no se puede leer: ${causa}. Korvun no lo repara ni lo recrea. Este fichero contiene el libro de actos y las conversaciones de este perfil. Detén Korvun y sustituye ${ruta} por una copia tomada antes del fallo; si tu copia es un solo fichero, borra también ${ruta}-wal y ${ruta}-shm. Si no tienes copia, aparta esos ficheros y Korvun empezará un libro nuevo, sin el historial de este perfil.`
const D3_ENVIRONMENT = (causa: string, ruta: string): string[] => [
  'El libro no se puede usar en esta máquina.',
  `El fichero puede estar bien; lo que falla es el disco o los permisos: ${causa}.`,
  `Libera espacio o revisa los permisos de ${ruta} y vuelve a abrir Korvun.`,
  'Mientras tanto, los botones de esta pantalla no registran nada.',
]
const D3_MOMENT = (causa: string): string =>
  `No se pudo comprobar el libro ahora. Otro proceso lo está usando o el sistema no respondió: ${causa}. Vuelve a abrir esta pantalla en un momento; si persiste, cierra la otra ventana o proceso de Korvun.`

/** The words of D2's remedy, which no other state may say. */
const D2_REMEDY = ['sustituye', 'copia', 'aparta', '-wal', '-shm', 'repara']

interface Route {
  status: number
  body: unknown
}

function routes(table: Record<string, Route>): { posts: string[] } {
  const posts: string[] = []
  vi.stubGlobal('fetch', (url: string, init?: RequestInit) => {
    if (init?.method === 'POST') posts.push(url)
    const r = table[url] ?? { status: 404, body: { error: 'not found' } }
    return Promise.resolve(new Response(JSON.stringify(r.body), { status: r.status }))
  })
  return { posts }
}

const APPROVALS_OFF: Route = {
  status: 200,
  body: { gate: { approvals_enabled: false, brains_total: 1, brains_can_park: 0 } },
}

/** Logs, for the report, the string the screen rendered and the approved one. */
function report(id: string, rendered: string | null, approved: string): void {
  console.info('E4TEXT ' + JSON.stringify({ id, rendered, approved, same: rendered === approved }))
}

describe('WhatsHappening · the ledger that cannot be used (batch 4)', () => {
  beforeEach(() => {
    vi.unstubAllGlobals()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  // TE49 + TE66 · unreadable: the one canonical D2, with the real cause and
  // the real path, as one text; every mutation control disabled; mounting
  // again renders the same, with no timer.
  //
  // PROBING MUTATIONS (MU49, MU66): every error rendered as D2; the path or
  // the cause dropped; the shared-file sentence omitted; the old UX-card
  // remedy restored → reddens.
  it('TE49/TE66: renders D2 over a verdict, exactly, and disables the controls', async () => {
    const body = realBody('unreadable')
    const { owner, path } = body.ledger as Required<Ledger>
    routes({ '/api/whats-happening': { status: 200, body }, '/api/approvals': APPROVALS_OFF })
    const first = render(<WhatsHappening />)
    const d2 = await screen.findByTestId('whats-ledger-d2')
    report('D2', d2.textContent, D2(owner, path))
    expect(d2.textContent).toBe(D2(owner, path))
    expect(screen.getByRole('button', { name: 'Encender aprobaciones' })).toBeDisabled()
    first.unmount()
    render(<WhatsHappening />)
    expect((await screen.findByTestId('whats-ledger-d2')).textContent).toBe(D2(owner, path))
  })

  // TE49 · environment: D3-entorno, line by line, with the real cause and
  // path; every mutation control disabled.
  it('TE49: renders D3-entorno over the environment, line by line, and disables the controls', async () => {
    const body = realBody('environment')
    const { owner, path } = body.ledger as Required<Ledger>
    routes({ '/api/whats-happening': { status: 200, body }, '/api/approvals': APPROVALS_OFF })
    render(<WhatsHappening />)
    const lines = (await screen.findAllByTestId('whats-ledger-environment-line')).map(
      (e) => e.textContent,
    )
    D3_ENVIRONMENT(owner, path).forEach((approved, i) =>
      report(`D3-entorno line ${i + 1}`, lines[i] ?? null, approved),
    )
    expect(lines).toEqual(D3_ENVIRONMENT(owner, path))
    expect(screen.getByRole('button', { name: 'Encender aprobaciones' })).toBeDisabled()
  })

  // TE49 + TE55 · unavailable: D3-del-momento, exactly; the controls stay
  // active; not the environment's no-recording sentence; and no word of D2's
  // remedy beyond what the same screen with a healthy ledger already says,
  // counted word by word against that screen (option A, adjudicated
  // 2026-09-27: the approved row «Operación que exige preparación» carries
  // «repara» on every screen, so a bare «not contained» could never hold).
  //
  // PROBING MUTATIONS (MU55): unavailable rendered as D2 instead of D3 →
  // reddens; D2 rendered in addition to D3 → the counts differ → reddens.
  it('TE49/TE55: renders D3-del-momento over any other failure, keeps the controls, gives no D2 remedy', async () => {
    const body = realBody('unavailable')
    const { owner } = body.ledger as Required<Ledger>
    routes({ '/api/whats-happening': { status: 200, body }, '/api/approvals': APPROVALS_OFF })
    const { container, unmount } = render(<WhatsHappening />)
    const moment = await screen.findByTestId('whats-ledger-unavailable')
    report('D3-del-momento', moment.textContent, D3_MOMENT(owner))
    expect(moment.textContent).toBe(D3_MOMENT(owner))
    expect(screen.getByRole('button', { name: 'Encender aprobaciones' })).toBeEnabled()
    const text = container.textContent ?? ''
    expect(text).not.toContain('Mientras tanto, los botones de esta pantalla no registran nada.')
    expect(screen.queryByTestId('whats-ledger-d2')).toBeNull()
    unmount()
    routes({
      '/api/whats-happening': {
        status: 200,
        body: { ...body, ledger: { standing: 'ok', owner: 'sha256:aa' } },
      },
      '/api/approvals': APPROVALS_OFF,
    })
    const healthy = render(<WhatsHappening />)
    await waitFor(() => {
      expect(healthy.container.querySelector('[data-testid="whats-all-rows"]')).not.toBeNull()
      expect(healthy.container.textContent).toContain('Encender aprobaciones')
    })
    const base = healthy.container.textContent ?? ''
    const count = (s: string, w: string): number => s.split(w).length - 1
    for (const word of D2_REMEDY) expect(count(text, word)).toBe(count(base, word))
  })

  // TE56 (the renderer) · a ledger with no path: never «{ruta}», never a
  // guessed path; D2 and D3-entorno keep every sentence that needs no path,
  // verbatim, and omit the ones that do.
  it('TE56: with no path from the core, invents none', async () => {
    const unreadable = realBody('unreadable')
    const { owner } = unreadable.ledger as Required<Ledger>
    delete (unreadable.ledger as Ledger).path
    routes({
      '/api/whats-happening': { status: 200, body: unreadable },
      '/api/approvals': APPROVALS_OFF,
    })
    const { container, unmount } = render(<WhatsHappening />)
    const d2 = await screen.findByTestId('whats-ledger-d2')
    expect(d2.textContent).toBe(
      `El libro no se puede leer: ${owner}. Korvun no lo repara ni lo recrea. Este fichero contiene el libro de actos y las conversaciones de este perfil.`,
    )
    for (const guess of ['{ruta}', 'undefined', 'korvun.db'])
      expect(container.textContent).not.toContain(guess)
    unmount()
    const environment = realBody('environment')
    const env = environment.ledger as Required<Ledger>
    delete (environment.ledger as Ledger).path
    routes({
      '/api/whats-happening': { status: 200, body: environment },
      '/api/approvals': APPROVALS_OFF,
    })
    render(<WhatsHappening />)
    const lines = (await screen.findAllByTestId('whats-ledger-environment-line')).map(
      (e) => e.textContent,
    )
    const [l1, l2, , l4] = D3_ENVIRONMENT(env.owner, 'x')
    expect(lines).toEqual([l1, l2, l4])
  })

  // TE66 · the editorial source check: the canonical sentences are in the
  // component's source once each; the stale UX-card alternatives and the old
  // row are not.
  it('TE66: the source carries the canonical copy once and none of the stale ones', () => {
    const src = readFileSync(join(__dirname, 'WhatsHappening.tsx'), 'utf8')
    for (const canonical of [
      'Korvun no lo repara ni lo recrea.',
      'Este fichero contiene el libro de actos y las conversaciones de este perfil.',
      'por una copia tomada antes del fallo; si tu copia es un solo fichero, borra también',
      'Si no tienes copia, aparta esos ficheros y Korvun empezará un libro nuevo, sin el historial de este perfil.',
      'El libro no se puede usar en esta máquina.',
      'El fichero puede estar bien; lo que falla es el disco o los permisos:',
      'y vuelve a abrir Korvun.',
      'Mientras tanto, los botones de esta pantalla no registran nada.',
      'No se pudo comprobar el libro ahora. Otro proceso lo está usando o el sistema no respondió:',
      'Vuelve a abrir esta pantalla en un momento; si persiste, cierra la otra ventana o proceso de Korvun.',
    ]) {
      expect(src.split(canonical).length - 1).toBe(1)
    }
    for (const stale of [
      'Mientras tanto no se registra ni se ejecuta nada.',
      'Detén Korvun y restaura tu copia',
      'Libro ilegible',
      'No se pudo saber de qué perfil es el libro',
      'El núcleo no pudo leer la marca del libro de acciones',
    ]) {
      expect(src).not.toContain(stale)
    }
  })
})

/** A ledger object for a state, over the real rows; with none, the body has
 * no ledger key (the same JSON as a key holding undefined). */
function bodyWith(ledger: Ledger | undefined): Body {
  const base = realBody('unreadable')
  return ledger === undefined
    ? { rows: base.rows, conditions: base.conditions }
    : { rows: base.rows, conditions: base.conditions, ledger }
}

const BLOCKED: Record<string, Ledger> = {
  unreadable: { standing: 'unreadable', owner: 'a verdict', path: '/perfil/korvun.db' },
  environment: { standing: 'environment', owner: 'a disk', path: '/perfil/korvun.db' },
}

/** The five legs of TE60's closed inventory: the routes that draw each group
 * of controls, the controls, and how each one is pressed to a POST. */
interface Leg {
  name: string
  approvals: Route
  config?: Route
  healthyLedger?: Ledger
  controls: { name: string; confirm?: string; host?: string }[]
}
const LEGS: Leg[] = [
  {
    name: 'approvals off, with ceiling and shadow blockers',
    approvals: {
      status: 200,
      body: {
        gate: {
          approvals_enabled: false,
          brains_total: 1,
          brains_can_park: 0,
          blocked: [{ brain: 'b', failed: ['ceiling', 'tool_shadowed'], tool: 'webhook_call' }],
        },
      },
    },
    controls: [
      { name: 'Encender aprobaciones' },
      { name: 'Poner el techo' },
      { name: 'Levantar la sombra', confirm: 'Sí, que pueda ejecutarse tras mi aprobación' },
    ],
  },
  {
    name: 'an otherwise unblocked park gate with its cages',
    approvals: {
      status: 200,
      body: { gate: { approvals_enabled: true, brains_total: 1, brains_can_park: 1, blocked: [] } },
    },
    config: {
      status: 200,
      body: {
        brains: [
          {
            name: 'b',
            agent: { webhook_call: { allow_hosts: ['a.io'] }, http_fetch: { allow_hosts: [] } },
          },
        ],
      },
    },
    controls: [
      {
        name: 'Añadir host',
        confirm: 'Sí, que pueda hablar con este host tras mi aprobación',
        host: 'hooks.e4.io',
      },
    ],
  },
  {
    name: 'the store-absent row',
    approvals: { status: 404, body: { error: 'not found' } },
    controls: [{ name: 'Activar almacén' }],
  },
  {
    name: 'the foreign ledger and its adoption',
    approvals: APPROVALS_OFF,
    healthyLedger: { standing: 'ledger_foreign_profile', owner: 'sha256:aa' },
    controls: [
      { name: 'Encender aprobaciones' },
      { name: 'Adoptar libro', confirm: 'Sí, este perfil se queda con este libro' },
    ],
  },
]

describe('WhatsHappening · every mutation control, healthy then blocked (TE60)', () => {
  beforeEach(() => {
    vi.unstubAllGlobals()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  for (const leg of LEGS) {
    // The control fixture: every control of the leg exists and reaches its
    // POST (through its confirmation, with a host where it asks for one).
    it(`TE60 healthy · ${leg.name}: every control exists and reaches its door`, async () => {
      const table: Record<string, Route> = {
        '/api/whats-happening': { status: 200, body: bodyWith(leg.healthyLedger) },
        '/api/approvals': leg.approvals,
      }
      if (leg.config !== undefined) table['/api/config'] = leg.config
      for (const door of [
        'enable-approvals',
        'set-ceiling',
        'lift-shadow',
        'allow-host',
        'enable-storage',
        'adopt-ledger',
      ])
        table[`/api/whats-happening/${door}`] = { status: 200, body: { outcome: 'applied' } }
      const { posts } = routes(table)
      render(<WhatsHappening />)
      for (const c of leg.controls) {
        const before = posts.length
        const buttons = await screen.findAllByRole('button', { name: c.name })
        expect(buttons[0]).toBeEnabled()
        await act(async () => {
          fireEvent.click(buttons[0])
        })
        if (c.confirm !== undefined) {
          const confirm = c.confirm
          if (c.host !== undefined) {
            fireEvent.change(await screen.findByPlaceholderText('hooks.ejemplo.io'), {
              target: { value: c.host },
            })
          }
          await act(async () => {
            fireEvent.click(screen.getByRole('button', { name: confirm }))
          })
        }
        await waitFor(() => expect(posts.length).toBe(before + 1))
      }
    })

    for (const [state, ledger] of Object.entries(BLOCKED)) {
      // The same leg, the ledger unusable: every control that exists is
      // disabled, and no click, no Enter and no Space reaches a door.
      //
      // PROBING MUTATION (MU60): one control's disabled binding removed →
      // reddens.
      it(`TE60 ${state} · ${leg.name}: every control is disabled and nothing is posted`, async () => {
        const table: Record<string, Route> = {
          '/api/whats-happening': { status: 200, body: bodyWith(ledger) },
          '/api/approvals': leg.approvals,
        }
        if (leg.config !== undefined) table['/api/config'] = leg.config
        const { posts } = routes(table)
        render(<WhatsHappening />)
        await screen.findByTestId(
          state === 'unreadable' ? 'whats-ledger-d2' : 'whats-ledger-environment',
        )
        for (const c of leg.controls) {
          for (const button of screen.queryAllByRole('button', { name: c.name })) {
            expect(button).toBeDisabled()
            await act(async () => {
              fireEvent.click(button)
              fireEvent.keyDown(button, { key: 'Enter' })
              fireEvent.keyDown(button, { key: ' ' })
            })
          }
          expect(screen.queryByRole('button', { name: c.confirm ?? '—' })).toBeNull()
        }
        expect(posts).toEqual([])
      })
    }
  }

  // TE60, the dialog (component-level evidence) · a confirmation already
  // open when the ledger becomes unusable: its «sí» is disabled — with a host
  // typed, too — and a click reaches no press.
  //
  // PROBING MUTATION (MU60): the confirmation's disabled binding removed →
  // reddens.
  for (const c of [
    {
      door: 'lift-shadow',
      rule: 'tool_shadowed',
      confirm: 'Sí, que pueda ejecutarse tras mi aprobación',
      host: '',
    },
    {
      door: 'allow-host',
      rule: 'cage',
      confirm: 'Sí, que pueda hablar con este host tras mi aprobación',
      host: 'hooks.e4.io',
    },
    {
      door: 'adopt-ledger',
      rule: 'ledger_foreign_profile',
      confirm: 'Sí, este perfil se queda con este libro',
      host: '',
    },
  ]) {
    it(`TE60 dialog · ${c.door}: the open confirmation is disabled once the ledger is blocked`, async () => {
      const press = vi.fn(() => Promise.resolve())
      const row = { rule: c.rule, label: c.rule, profile_key: 'k', has_button: true, door: c.door }
      const brain = c.door === 'adopt-ledger' ? '' : 'b'
      const tool = c.door === 'adopt-ledger' ? '' : 'webhook_call'
      render(
        <BlockedRow
          row={row}
          brain={brain}
          tool={tool}
          confirming={`${brain}:${tool}:${c.rule}`}
          setConfirming={() => undefined}
          host={c.host}
          setHost={() => undefined}
          press={press}
          pressed={null}
          blocked="unreadable"
        />,
      )
      const yes = screen.getByRole('button', { name: c.confirm })
      expect(yes).toBeDisabled()
      await act(async () => {
        fireEvent.click(yes)
      })
      expect(press).not.toHaveBeenCalled()
    })
  }
})
