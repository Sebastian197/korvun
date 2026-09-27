// v0.16.2 · «¿Qué pasa hoy?» — the screen the seventh law demands.
//
// These moulds exist because the internal adversarial pass found the whole
// screen UNMOUNTED: the Go doors had no production caller and the window had no
// component. Every green in the train was green over a surface no operator
// could reach. Go's side is now pinned by a real-listener mount mould; this file
// pins the window's.
//
// What each one attacks:
//   · the screen must not draw a button the core does not have;
//   · the two doors that open REAL execution must not fire on one click;
//   · a park condition the contract cannot explain must SAY so, never render an
//     empty row the operator can neither read nor act on;
//   · a rolled-back cutover must not be reported as done.
//
// Evidence level, honest: in-process, jsdom, with fetch replaced. Nothing here
// proves a real core, a real supervisor or a real cutover — the Go moulds carry
// those, and the packaged pass carries the rest.
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { WhatsHappening } from './WhatsHappening'

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

describe('WhatsHappening', () => {
  beforeEach(() => {
    vi.unstubAllGlobals()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  // The state a migrated profile lands in: approvals off, with an explanation
  // and — until this train — no action on screen. The button is the whole point
  // of the release.
  //
  // PROBING MUTATION (executed): render the row without its button (drop the
  // `row.has_button` branch). This reddens on the missing control.
  it('offers the button in the state a migrated profile lands in', async () => {
    const { posts } = routes({
      '/api/whats-happening': CONTRACT_OK,
      '/api/approvals': {
        status: 200,
        body: { gate: { approvals_enabled: false, brains_total: 1, brains_can_park: 0 } },
      },
      '/api/whats-happening/enable-approvals': {
        status: 200,
        body: { outcome: 'applied', applied: true },
      },
    })
    render(<WhatsHappening />)

    const button = await screen.findByRole('button', { name: 'Encender aprobaciones' })
    await clickOnce(button)

    await waitFor(() => expect(posts).toHaveLength(1))
    expect(posts[0].url).toBe('/api/whats-happening/enable-approvals')
    expect(await screen.findByTestId('whats-outcome')).toHaveTextContent('Hecho')
  })

  // The director's ruling: «Levantar la sombra» is the only button that opens
  // real execution, so it asks in one step before firing. One click must reach
  // NO door — proved by the POST log being empty, which a door that was never
  // called cannot fake.
  //
  // It also pins WHICH tool travels. The screen used to hardcode
  // `webhook_call`, which is right on the profile the last demo used and wrong
  // on every other; the gate now names the tool and the screen must send that.
  //
  // PROBING MUTATIONS (both executed): make the shadow button call `press`
  // directly instead of `setConfirming` — the POST fires on the first click and
  // this reddens on the counter; or hardcode `tool: 'webhook_call'` in `press` —
  // this reddens on the body.
  it('does not open real execution on one click', async () => {
    const { posts } = routes({
      '/api/whats-happening': CONTRACT_OK,
      '/api/approvals': {
        status: 200,
        body: {
          gate: {
            approvals_enabled: true,
            brains_total: 1,
            brains_can_park: 0,
            // NOT webhook_call: the tool the last demo used is exactly the
            // one a hardcoded screen would send, and this profile shadows
            // another. The POST must carry what the CORE named.
            blocked: [{ brain: 'default', failed: ['tool_shadowed'], tool: 'http_fetch' }],
          },
        },
      },
      '/api/whats-happening/lift-shadow': {
        status: 200,
        body: { outcome: 'applied', applied: true },
      },
    })
    render(<WhatsHappening />)

    await clickOnce(await screen.findByRole('button', { name: 'Levantar la sombra' }))
    expect(posts).toHaveLength(0)

    // The confirmation says what it opens, in the words the director approved.
    const confirm = await screen.findByRole('button', {
      name: 'Sí, que pueda ejecutarse tras mi aprobación',
    })
    await clickOnce(confirm)
    await waitFor(() => expect(posts).toHaveLength(1))
    expect(posts[0].body).toMatchObject({ confirm: true, brain: 'default', tool: 'http_fetch' })
  })

  // A rolled-back cutover reports its own sentence. A screen that said «hecho»
  // here would send the operator away believing a change that never happened.
  //
  // PROBING MUTATION (executed): map `not_applied` to the «applied» sentence.
  // This reddens.
  it('says a rolled-back change did not apply', async () => {
    routes({
      '/api/whats-happening': CONTRACT_OK,
      '/api/approvals': {
        status: 200,
        body: { gate: { approvals_enabled: false, brains_total: 1, brains_can_park: 0 } },
      },
      '/api/whats-happening/enable-approvals': {
        status: 200,
        body: { outcome: 'not_applied', applied: false, profile_unchanged: true },
      },
    })
    render(<WhatsHappening />)

    await clickOnce(await screen.findByRole('button', { name: 'Encender aprobaciones' }))

    const said = await screen.findByTestId('whats-outcome')
    expect(said).toHaveTextContent('No se aplicó')
    expect(said).not.toHaveTextContent('Hecho')
  })

  // The half-applied state, which needs BOTH halves said: the new app IS
  // serving and the disk could NOT be updated.
  //
  // PROBING MUTATION (executed): shorten the sentence to «Hecho». This reddens
  // on the missing warning.
  it('says both halves when the change could not be saved', async () => {
    routes({
      '/api/whats-happening': CONTRACT_OK,
      '/api/approvals': {
        status: 200,
        body: { gate: { approvals_enabled: false, brains_total: 1, brains_can_park: 0 } },
      },
      '/api/whats-happening/enable-approvals': {
        status: 200,
        body: { outcome: 'applied_not_saved', applied: true, profile_unchanged: true },
      },
    })
    render(<WhatsHappening />)
    await clickOnce(await screen.findByRole('button', { name: 'Encender aprobaciones' }))

    const said = await screen.findByTestId('whats-outcome')
    expect(said).toHaveTextContent('en marcha')
    expect(said).toHaveTextContent('volverá atrás')
  })

  // No action store: the app mounts `/api/approvals` only when a store is open,
  // so a 404 there — and ONLY a 404 — is the store's absence. The screen must
  // name it from the contract's own row.
  //
  // THE NAME OF THIS MOULD USED TO LIE. It read «a missing approvals surface»
  // while the branch it exercised was `!gate.ok`, which covers 401, 409, 500 and
  // a garbage body identically — so it passed over a wire that had no 404
  // discriminator at all, and its declared mutation could not neutralise a
  // discriminator nobody had written. Third case in this train of a mutation
  // aimed at the wrong place; the official pass caught it by reading the cable.
  // The wire now discriminates, and the sibling mould below is the half that was
  // missing.
  //
  // PROBING MUTATION (executed): drop the `status === 404` test and take every
  // failure as the store's absence. This mould stays green and its SIBLING
  // reddens — which is exactly why one mould was not enough.
  it('reads a 404 from the approvals door as the store blocker', async () => {
    routes({ '/api/whats-happening': CONTRACT_OK })
    render(<WhatsHappening />)

    expect(await screen.findByTestId('whats-block-store')).toHaveTextContent('Almacén de acciones')
    expect(screen.queryByTestId('whats-unreadable')).toBeNull()
  })

  // A condition the contract cannot explain. The Go contract mould makes this
  // unreachable by construction — but «unreachable» is a claim about today's
  // code, and a screen that rendered an empty row would leave the operator
  // reading a blank where a blocker is.
  //
  // PROBING MUTATION (executed): return null for an unknown condition. The row
  // disappears and this reddens.
  it('says so when the core reports a blocker it cannot explain', async () => {
    routes({
      '/api/whats-happening': CONTRACT_OK,
      '/api/approvals': {
        status: 200,
        body: {
          gate: {
            approvals_enabled: true,
            brains_total: 1,
            brains_can_park: 0,
            blocked: [{ brain: 'default', failed: ['audit_pending'] }],
          },
        },
      },
    })
    render(<WhatsHappening />)

    const row = await screen.findByTestId('whats-unexplained')
    expect(row).toHaveTextContent('no sabe explicar')
    expect(row).toHaveTextContent('audit_pending')
  })

  // Korvun serving read-only: no admin token, so the door itself is not
  // mounted. That is a state with its own sentence, not an error.
  //
  // PROBING MUTATION (executed): fall through to `unreadable` on 404. This
  // reddens.
  it('names the read-only core instead of reporting a failure', async () => {
    routes({})
    render(<WhatsHappening />)

    expect(await screen.findByTestId('whats-no-door')).toHaveTextContent('admin.token_env')
  })

  // The rows with no button are the conservative half of the law: they must
  // still tell the operator WHERE to change the thing.
  //
  // PROBING MUTATION (executed): drop the profile key from the text-only rows.
  // This reddens.
  it('tells the operator where to change what has no button', async () => {
    routes({
      '/api/whats-happening': CONTRACT_OK,
      '/api/approvals': {
        status: 200,
        body: { gate: { approvals_enabled: true, brains_total: 1, brains_can_park: 1 } },
      },
    })
    render(<WhatsHappening />)

    const all = await screen.findByTestId('whats-all-rows')
    expect(all).toHaveTextContent('brains[].agent.governance[].mode')
    expect(all).toHaveTextContent('una denegación explícita se levanta mirando el perfil')
  })

  // A blocker the core reports WITHOUT naming a tool. `lift-shadow` acts on a
  // tool, so a button here would POST an edit aimed at an empty name — which
  // the door refuses, after telling the operator to press something that could
  // never work.
  //
  // PROBING MUTATION (executed): drop the `needsTool` guard. The button appears
  // and this reddens.
  it('draws no tool button when the core named no tool', async () => {
    routes({
      '/api/whats-happening': CONTRACT_OK,
      '/api/approvals': {
        status: 200,
        body: {
          gate: {
            approvals_enabled: true,
            brains_total: 1,
            brains_can_park: 0,
            blocked: [{ brain: 'default', failed: ['tool_shadowed'] }],
          },
        },
      },
    })
    render(<WhatsHappening />)

    // The row is there — the operator must still be told what blocks them and
    // where the key lives.
    const row = await screen.findByTestId('whats-block-tool_shadowed')
    expect(row).toHaveTextContent('brains[].agent.governance[].mode')
    expect(screen.queryByRole('button', { name: 'Levantar la sombra' })).toBeNull()
  })

  // The cage: the only dimension that lets a request park, lets the operator say
  // yes, and refuses AFTERWARDS. It is not in the park gate for that reason, so
  // the screen reads it from the config door the builder already uses and shows
  // it in the GREEN state — where it is the only thing still able to say no.
  //
  // Reproduction this comes from (director, 2026-09-23, his real profile): he
  // approved apr_843888a7 and the cage refused 127.0.0.1:5678 with receipt
  // rcpt_1f4f9a52.
  //
  // PROBING MUTATION (executed): stop reading /api/config. The cage row and its
  // button disappear and this reddens.
  it('shows the cage and its hosts in the green state, and widens it on confirmation', async () => {
    const { posts } = routes({
      '/api/whats-happening': CONTRACT_OK,
      '/api/approvals': {
        status: 200,
        body: { gate: { approvals_enabled: true, brains_total: 1, brains_can_park: 1 } },
      },
      '/api/config': {
        status: 200,
        body: {
          brains: [
            { name: 'default', agent: { webhook_call: { allow_hosts: ['127.0.0.1:8765'] } } },
          ],
        },
      },
      '/api/whats-happening/allow-host': {
        status: 200,
        body: { outcome: 'applied', applied: true },
      },
    })
    render(<WhatsHappening />)

    const row = await screen.findByTestId('whats-block-cage')
    expect(row).toHaveTextContent('puede hablar con:')
    expect(row).toHaveTextContent('127.0.0.1:8765')

    await clickOnce(await screen.findByRole('button', { name: 'Añadir host' }))
    // Still nothing sent: widening a cage opens real execution too.
    expect(posts).toHaveLength(0)

    fireEvent.change(screen.getByPlaceholderText('hooks.ejemplo.io'), {
      target: { value: 'hooks.acme.io' },
    })
    await clickOnce(
      screen.getByRole('button', { name: 'Sí, que pueda hablar con este host tras mi aprobación' }),
    )
    await waitFor(() => expect(posts).toHaveLength(1))
    expect(posts[0].body).toMatchObject({
      confirm: true,
      brain: 'default',
      tool: 'webhook_call',
      host: 'hooks.acme.io',
    })
  })

  // THE P1 OF THE OFFICIAL PASS. The core names corruption precisely — the
  // approvals door answers 409 with `{"error":"evidence_corrupt"}` — and the
  // screen used to throw the whole answer away and print «Almacén de acciones»,
  // the row whose own reason reads «sin almacén no se puede aparcar nada». The
  // operator was told a store was MISSING while holding one that exists and will
  // not verify. Absence and corruption do not share a sentence, and this repo has
  // that class filed for readers, verifiers and migrations; here the surface was
  // the narration.
  //
  // PROBING MUTATION (executed): take every failed read as the store's absence,
  // the way the first version did. This reddens on both halves — the missing
  // sentence and the `store` row that must NOT appear.
  it('says a corrupt store is unreadable, never missing', async () => {
    routes({
      '/api/whats-happening': CONTRACT_OK,
      '/api/approvals': {
        status: 409,
        body: {
          error: 'evidence_corrupt',
          message: 'the stored evidence for this request does not verify',
        },
      },
    })
    render(<WhatsHappening />)

    const row = await screen.findByTestId('whats-gate-unreadable')
    expect(row).toHaveTextContent('NO verifica')
    expect(row).toHaveTextContent('permanente')
    // The store's ABSENCE must not be claimed: the store is there.
    expect(screen.queryByTestId('whats-block-store')).toBeNull()
    // And an unreadable gate is not a green verdict either.
    expect(screen.queryByTestId('whats-green')).toBeNull()
  })

  // An error the core has no name for: `writeApprovalError` answers a plain-text
  // 500 when the sentinel is unbound. The screen must still not invent absence,
  // and must say the core named no motive rather than leave a blank.
  //
  // PROBING MUTATION (executed): fall back to the `store` row when the name is
  // empty. This reddens.
  it('says so when the core fails without naming a motive', async () => {
    routes({
      '/api/whats-happening': CONTRACT_OK,
      '/api/approvals': { status: 500, body: 'internal error' },
    })
    render(<WhatsHappening />)

    const row = await screen.findByTestId('whats-gate-unreadable')
    expect(row).toHaveTextContent('500')
    expect(row).toHaveTextContent('sin nombrar el motivo')
    expect(screen.queryByTestId('whats-block-store')).toBeNull()
  })

  // THE SECOND P2. The cages come from `/api/config`; a failed read left the
  // list EMPTY under the label «Lo que sigue decidiendo DESPUÉS de que digas que
  // sí:», so the operator read a promise of completeness with nothing under it —
  // «nothing else decides» where the truth is «we could not look».
  //
  // PROBING MUTATION (executed): print the label unconditionally and drop the
  // unreadable row. This reddens on both.
  it('does not promise completeness when the cages could not be read', async () => {
    routes({
      '/api/whats-happening': CONTRACT_OK,
      '/api/approvals': {
        status: 200,
        body: { gate: { approvals_enabled: true, brains_total: 1, brains_can_park: 1 } },
      },
      '/api/config': { status: 503, body: { error: 'no current config' } },
    })
    render(<WhatsHappening />)

    const green = await screen.findByTestId('whats-green')
    expect(green).not.toHaveTextContent('Lo que sigue decidiendo')
    const warn = screen.getByTestId('whats-cages-unreadable')
    expect(warn).toHaveTextContent('503')
    expect(warn).toHaveTextContent('No quiere decir que no haya ninguna')
  })

  // THE SECOND P3. `AfterApproval` on the Go side says those rows «belong in the
  // GREEN state too». Two rows carry it: the cage, which the rows above paint,
  // and the private network shield, which was reachable only inside the collapsed
  // list — so the green label enumerated less than the contract declares, and no
  // mould compared the two.
  //
  // PROBING MUTATION (executed): remove <AfterApprovalTextRows>. This reddens.
  it('shows every after-approval dimension in the green state, not only the cage', async () => {
    routes({
      '/api/whats-happening': CONTRACT_OK,
      '/api/approvals': {
        status: 200,
        body: { gate: { approvals_enabled: true, brains_total: 1, brains_can_park: 1 } },
      },
      '/api/config': { status: 200, body: { brains: [] } },
    })
    render(<WhatsHappening />)

    // Derived from the contract, not typed here: every after-approval row that
    // has no button must have a place in the green state. A row added to the Go
    // table reaches this assertion without anyone opening this file.
    const expected = CONTRACT.rows.filter((r) => r.after_approval === true && !r.has_button)
    expect(expected.length).toBeGreaterThan(0)
    for (const r of expected) {
      expect(await screen.findByTestId(`whats-after-${r.rule}`)).toHaveTextContent(r.label)
    }
  })

  // THE SEVENTH LAW APPLIED TO THE ACT ITSELF. Every change through this screen
  // is written into the action ledger with a receipt, and a capability the
  // operator cannot see is not finished — the law makes no exception for the
  // law's own machinery.
  //
  // PROBING MUTATION (executed): stop rendering the act block. This reddens.
  it('shows the operator act and how to check its receipt', async () => {
    routes({
      '/api/whats-happening': CONTRACT_OK,
      '/api/approvals': {
        status: 200,
        body: { gate: { approvals_enabled: false, brains_total: 1, brains_can_park: 0 } },
      },
      '/api/whats-happening/enable-approvals': {
        status: 200,
        body: {
          outcome: 'applied',
          applied: true,
          act: { action_id: 'act_abc123', receipt_id: 'rcpt_def456' },
        },
      },
    })
    render(<WhatsHappening />)
    await clickOnce(await screen.findByRole('button', { name: 'Encender aprobaciones' }))

    const act = await screen.findByTestId('whats-act')
    expect(act).toHaveTextContent('act_abc123')
    expect(act).toHaveTextContent('rcpt_def456')
    expect(act).toHaveTextContent('korvun receipt verify')
  })

  // A cutover still running answers with a handle and NO receipt, because a
  // receipt seals an outcome and there is none yet. The screen must ASK the status
  // door — the first version left the operator on «aplicando…» with no way
  // forward — and must not pretend it already has the receipt.
  //
  // PROBING MUTATION (executed): remove the poll. The outcome stays «applying»,
  // the receipt never arrives, and this reddens on both.
  it('polls for the outcome and the receipt when the change is still applying', async () => {
    const { posts } = routes({
      '/api/whats-happening': CONTRACT_OK,
      '/api/approvals': {
        status: 200,
        body: { gate: { approvals_enabled: false, brains_total: 1, brains_can_park: 0 } },
      },
      '/api/whats-happening/enable-approvals': {
        status: 200,
        body: {
          outcome: 'applying',
          applied: false,
          profile_unchanged: true,
          handle: 'reload-7',
          act: { action_id: 'act_pending' },
        },
      },
      '/api/reload/reload-7': {
        status: 200,
        body: { state: 'succeeded', action_id: 'act_pending', receipt_id: 'rcpt_late' },
      },
    })
    render(<WhatsHappening />)
    await clickOnce(await screen.findByRole('button', { name: 'Encender aprobaciones' }))

    expect(posts).toHaveLength(1)
    const said = await screen.findByTestId('whats-outcome')
    expect(said).toHaveTextContent('Hecho')
    const act = await screen.findByTestId('whats-act')
    expect(act).toHaveTextContent('act_pending')
    expect(act).toHaveTextContent('rcpt_late')
  })

  // And a change that is refused BEFORE the act is sealed must not claim one. The
  // difference matters: «nothing was attempted» and «it was attempted and failed»
  // are different things to read in a book.
  //
  // PROBING MUTATION (executed): render the act block whenever the door answered.
  // The refusal then shows an empty act id and this reddens.
  // The director's decision (2026-09-24): a profile with no action store cannot
  // be changed from here EXCEPT by founding the store, and that is the first row
  // with its own button. The row is about the PROFILE, not a brain, so the POST
  // carries no brain — a body with `brain: '—'` would be the screen inventing
  // a target the core never named.
  //
  // PROBING MUTATIONS (both to be executed): leave the button label out of
  // BUTTON_ES (the button reads `enable-storage` and the query reddens); send
  // `brain` for every door (the body assertion reddens).
  it('offers «Activar almacén» on the store row and founds the ledger', async () => {
    const { posts } = routes({
      '/api/whats-happening': CONTRACT_OK,
      '/api/whats-happening/enable-storage': {
        status: 200,
        body: {
          outcome: 'applied',
          applied: true,
          detail: 'el libro vive en /home/op/.config/korvun/korvun.db',
          act: { action_id: 'act_f0', receipt_id: 'rcpt_f0' },
        },
      },
    })
    render(<WhatsHappening />)

    expect(await screen.findByTestId('whats-block-store')).toHaveTextContent('Almacén de acciones')
    const button = await screen.findByRole('button', { name: 'Activar almacén' })
    await clickOnce(button)

    await waitFor(() => expect(posts).toHaveLength(1))
    expect(posts[0].url).toBe('/api/whats-happening/enable-storage')
    expect(posts[0].body).not.toHaveProperty('brain')
    expect(posts[0].body).not.toHaveProperty('tool')
    expect(await screen.findByTestId('whats-outcome')).toHaveTextContent('Hecho')
    expect(screen.getByTestId('whats-outcome')).toHaveTextContent('korvun.db')
    expect(screen.getByTestId('whats-act')).toHaveTextContent('act_f0')
  })

  // Two refusals the founding can answer, each with its own sentence: a file
  // already at the default path is NEVER adopted (it may be another profile's
  // book), and a directory that refuses writes founds nothing. Neither is «no se
  // hizo el cambio»: the operator needs the path in one case and the cause in
  // the other.
  //
  // PROBING MUTATION: drop both entries from OUTCOME_ES. The screen says
  // «Respuesta que esta pantalla no sabe leer» and both redden.
  it('names a ledger already there, and one that could not be created', async () => {
    for (const [outcome, status, mustSay] of [
      ['ledger_exists', 409, 'ya hay un libro'],
      ['ledger_not_created', 503, 'no se pudo crear'],
    ] as const) {
      vi.unstubAllGlobals()
      routes({
        '/api/whats-happening': CONTRACT_OK,
        '/api/whats-happening/enable-storage': {
          status,
          body: { outcome, applied: false, profile_unchanged: true, detail: '/x/korvun.db' },
        },
      })
      const { unmount } = render(<WhatsHappening />)
      const button = await screen.findByRole('button', { name: 'Activar almacén' })
      await clickOnce(button)
      const said = await screen.findByTestId('whats-outcome')
      expect(said).toHaveTextContent(mustSay)
      expect(said).toHaveTextContent('/x/korvun.db')
      expect(screen.queryByTestId('whats-act')).toBeNull()
      unmount()
    }
  })

  // A REAL cutover — build, start, persist — is still in flight when the
  // POST answers, so the first poll says «pending» and the second may say
  // «cutover-in-progress». The screen must keep asking until the supervisor
  // reports a terminal state; a single poll left the operator on «aplicando…»
  // for good (the internal pass's P3). The sequence below is the cadence the
  // real supervisor produces; the test with a single «succeeded» answer cannot
  // tell one poll from many.
  //
  // PROBING MUTATION: poll once. The outcome stays «applying», no receipt, and
  // this reddens on both.
  it('keeps polling until the supervisor reports a terminal state', async () => {
    const { gets } = routes({
      '/api/whats-happening': CONTRACT_OK,
      '/api/approvals': {
        status: 200,
        body: { gate: { approvals_enabled: false, brains_total: 1, brains_can_park: 0 } },
      },
      '/api/whats-happening/enable-approvals': {
        status: 200,
        body: { outcome: 'applying', handle: 'reload-7', act: { action_id: 'act_p7' } },
      },
      '/api/reload/reload-7': [
        { status: 200, body: { state: 'pending' } },
        { status: 503, body: { error: 'admin server between ports' } },
        { status: 200, body: { state: 'cutover-in-progress' } },
        { status: 200, body: { state: 'succeeded', action_id: 'act_p7', receipt_id: 'rcpt_p7' } },
      ],
    })
    render(<WhatsHappening />)

    const button = await screen.findByRole('button', { name: 'Encender aprobaciones' })
    await clickOnce(button)

    const said = await screen.findByTestId('whats-outcome', {}, { timeout: 4000 })
    expect(said).toHaveTextContent('Hecho')
    expect(screen.getByTestId('whats-act')).toHaveTextContent('rcpt_p7')
    expect(gets['/api/reload/reload-7']).toBe(4)
  })

  // The durable mark (director's order, 2026-09-24): a ledger another profile
  // founded blocks every change until this profile adopts it with an explicit
  // act. The read door says so (`ledger.standing`), the screen offers ONE
  // button, «Adoptar libro», behind a one-step confirmation — taking a book is
  // a consent — and the answer carries the adoption act with its receipt.
  //
  // PROBING MUTATIONS: draw the row without the button (the query reddens);
  // fire the POST on the first click (the POST log reddens); ignore
  // `ledger.standing` (the row never renders).
  it('offers «Adoptar libro» when the ledger belongs to another profile', async () => {
    const { posts } = routes({
      '/api/whats-happening': {
        status: 200,
        body: { ...CONTRACT, ledger: { standing: 'ledger_foreign_profile', owner: 'sha256:aa' } },
      },
      '/api/whats-happening/adopt-ledger': {
        status: 200,
        body: {
          outcome: 'adopted',
          applied: true,
          act: { action_id: 'act_ad', receipt_id: 'rcpt_ad' },
        },
      },
    })
    render(<WhatsHappening />)

    expect(await screen.findByTestId('whats-block-ledger_foreign_profile')).toHaveTextContent(
      'Libro de otro perfil',
    )
    await clickOnce(await screen.findByRole('button', { name: 'Adoptar libro' }))
    expect(posts).toHaveLength(0)
    await clickOnce(
      await screen.findByRole('button', { name: 'Sí, este perfil se queda con este libro' }),
    )
    await waitFor(() => expect(posts).toHaveLength(1))
    expect(posts[0].url).toBe('/api/whats-happening/adopt-ledger')
    expect(posts[0].body).toMatchObject({ confirm: true })
    expect(await screen.findByTestId('whats-outcome')).toHaveTextContent('adoptado')
    expect(screen.getByTestId('whats-act')).toHaveTextContent('rcpt_ad')
  })

  // A ledger older than this version has no mark: named, not a blocker, not
  // corruption. The screen says so as text and offers no button for it.
  //
  // PROBING MUTATION: paint legacy as the foreign row. The button appears and
  // this reddens.
  it('names a ledger older than this version as legacy, without a button', async () => {
    routes({
      '/api/whats-happening': {
        status: 200,
        body: { ...CONTRACT, ledger: { standing: 'legacy_unfounded', owner: '' } },
      },
    })
    render(<WhatsHappening />)

    const legacy = await screen.findByTestId('whats-ledger-legacy')
    expect(legacy).toHaveTextContent('anterior a esta versión')
    // The caption promises nothing this screen cannot do: no button marks it.
    expect(legacy).toHaveTextContent('esta pantalla no ofrece marcarlo')
    expect(legacy).not.toHaveTextContent('lo marcará')
    expect(screen.queryByTestId('whats-block-ledger_foreign_profile')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Adoptar libro' })).toBeNull()
  })

  // While the ledger is foreign, every other button answers by name, and the
  // screen tells the operator to adopt first rather than «no se hizo».
  //
  // PROBING MUTATION: drop `ledger_foreign_profile` from OUTCOME_ES. The
  // sentence falls to «Respuesta que esta pantalla no sabe leer» and this
  // reddens.
  it('names the foreign ledger when another button is refused', async () => {
    routes({
      '/api/whats-happening': {
        status: 200,
        body: { ...CONTRACT, ledger: { standing: 'ledger_foreign_profile', owner: 'sha256:aa' } },
      },
      '/api/approvals': {
        status: 200,
        body: { gate: { approvals_enabled: false, brains_total: 1, brains_can_park: 0 } },
      },
      '/api/whats-happening/enable-approvals': {
        status: 409,
        body: { outcome: 'ledger_foreign_profile', applied: false, profile_unchanged: true },
      },
    })
    render(<WhatsHappening />)
    await clickOnce(await screen.findByRole('button', { name: 'Encender aprobaciones' }))
    expect(await screen.findByTestId('whats-outcome')).toHaveTextContent('Adoptar libro')
  })

  // A read of the ledger that FAILED with no verdict of the store — the old
  // uncoded «database is locked» — is not «no ledger», not «no profile» and
  // not a broken book: the core names it `unavailable` with its cause, and the
  // screen says it could not check the ledger now, with that cause, and gives
  // none of the remedy that only a verdict earns (train E, C1-1).
  //
  // PROBING MUTATION: render `unavailable` as the unreadable remedy, or omit
  // its row. This reddens.
  it('says so when the core could not check the ledger now', async () => {
    routes({
      '/api/whats-happening': {
        status: 200,
        body: { ...CONTRACT, ledger: { standing: 'unavailable', owner: 'database is locked' } },
      },
    })
    render(<WhatsHappening />)
    const row = await screen.findByTestId('whats-ledger-unavailable')
    expect(row).toHaveTextContent('database is locked')
    expect(row).not.toHaveTextContent('sustituye')
    expect(screen.queryByRole('button', { name: 'Adoptar libro' })).toBeNull()
  })

  it('shows no act when the change was refused before one was sealed', async () => {
    routes({
      '/api/whats-happening': CONTRACT_OK,
      '/api/approvals': {
        status: 200,
        body: { gate: { approvals_enabled: false, brains_total: 1, brains_can_park: 0 } },
      },
      '/api/whats-happening/enable-approvals': {
        status: 503,
        body: {
          outcome: 'no_ledger',
          applied: false,
          profile_unchanged: true,
          detail: 'este perfil no tiene almacén de acciones',
        },
      },
    })
    render(<WhatsHappening />)
    await clickOnce(await screen.findByRole('button', { name: 'Encender aprobaciones' }))

    expect(await screen.findByTestId('whats-outcome')).toHaveTextContent('libro')
    expect(screen.queryByTestId('whats-act')).toBeNull()
  })
})
