// v0.15.1 block B — the screen moulds. RED: written before any cure.
//
// P2-10 (docs/HANDOFF.md «Tren de la v0.15.1», Codex's reproduction): the
// screen does not validate the POST response protocol. Four registered names
// (not_found, unavailable, disabled, params_digest_mismatch) lose their names
// when they arrive from approve or reject, and any 200 whose `outcome` is not
// exactly `failed` is painted «Ejecutada».
//
// P2-1: an approval id is untrusted bytes from the core. It is printed through
// the escape alphabet like every other field, and a row whose id does not have
// the minted shape ("apr_" + 32 lowercase hex) is not turned into a URL.
//
// COPY. Every literal asserted here is approved: UX v36 (docs/superpowers/
// specs/2026-09-08-approvals-screen-ux.md, E3 and E9) for not_found,
// unavailable and disabled, and the four literals the copilot approved on
// 2026-09-19 under the director's speed order (Chano may veto them later):
// params_digest_mismatch from a POST, an unrecognised 2xx, a malformed-id row,
// and loopback_only. No literal is invented here.
//
// Evidence level, for EVERY test in this file: jsdom, fetch stubbed, Wails
// bindings stubbed. Proves what the screen paints and requests; never what the
// core did.
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Approvals } from './Approvals'

const HEX = 'a3f91c7d0b2e4f6a8c1d3e5f7a9b0c2d4e6f8a0b1c3d5e7f9a1b3c5d7e9f1b60'
const DIGEST = `sha256:${HEX}`
const TAIL = HEX.slice(-6)
const NOW = new Date('2026-09-08T14:00:00Z')
const ID = 'apr_' + 'ab'.repeat(16)

const ROW = {
  id: ID,
  action_id: 'act_1',
  operation: 'tool/webhook_call',
  effect_class: 'write_irreversible',
  expires_at: '2026-09-08T14:31:00Z',
  digest: DIGEST,
  origin: 'telegram',
}
const DETAIL = {
  ...ROW,
  purpose: 'avisar al webhook de pedidos',
  principal_id: 'brain:asistente',
  reversibility: 'write_irreversible — irreversible, no documented undo',
  tool_cage: 'webhook_call',
  required_rule: 'require_approval',
  law_digest: 'sha256:aab9b0d7' + 'c'.repeat(56),
  parameters: 'https://hooks.acme.io/pedidos {"id":1}',
  parameters_state: 'present',
}
const GATE = { approvals_enabled: true, brains_total: 3, brains_can_park: 2 }

type Call = { method: string; url: string }
let calls: Call[] = []
function json(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  })
}
type Route = (c: Call) => Response
function stubFetch(route: Route): void {
  calls = []
  vi.stubGlobal('fetch', (input: RequestInfo | URL, init?: RequestInit) => {
    const url =
      typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url
    const call: Call = { method: (init?.method ?? 'GET').toUpperCase(), url }
    calls.push(call)
    return Promise.resolve(route(call))
  })
}
function router(
  rows: unknown[],
  detail: unknown,
  post: Partial<Record<'approve' | 'reject', () => Response>>,
): Route {
  return (c) => {
    const path = c.url.replace(/^https?:\/\/[^/]+/, '')
    if (c.method === 'GET' && path === '/api/approvals') return json(200, { gate: GATE, rows })
    if (c.method === 'GET' && path === `/api/approvals/${ID}`) return json(200, detail)
    if (c.method === 'POST' && path === `/api/approvals/${ID}/approve` && post.approve)
      return post.approve()
    if (c.method === 'POST' && path === `/api/approvals/${ID}/reject` && post.reject)
      return post.reject()
    return new Response('<html>not here</html>', { status: 404 })
  }
}
async function openDetail(route: Route): Promise<void> {
  stubFetch(route)
  render(<Approvals onGoHome={() => undefined} />)
  const row = await screen.findByRole('button', { name: /tool\/webhook_call/ })
  fireEvent.click(row)
  await waitFor(() => expect(calls.some((c) => c.url.endsWith(`/api/approvals/${ID}`))).toBe(true))
}
function arm(): void {
  const input = screen.getByLabelText(/reteclea los seis últimos caracteres del digest/i)
  for (const ch of TAIL) fireEvent.keyDown(input, { key: ch })
}
const approveBtn = () => screen.getByRole('button', { name: 'Aprobar y ejecutar' })
const rejectBtn = () => screen.getByRole('button', { name: 'Rechazar' })
const UNKNOWN = 'Respuesta que esta pantalla no reconoce.'

interface FakeWindow {
  go?: { shell?: { Desktop?: Record<string, unknown> } }
}
beforeEach(() => {
  vi.useFakeTimers({ now: NOW, shouldAdvanceTime: true })
  ;(window as unknown as FakeWindow).go = {
    shell: {
      Desktop: {
        Status: () =>
          Promise.resolve({
            Running: true,
            ConfigPath: '/p/korvun.json',
            AdminAddr: '',
            TokenEnv: 'X',
          }),
      },
    },
  }
})
afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  vi.useRealTimers()
  delete (window as unknown as FakeWindow).go
})

// ---------------------------------------------------------------------------
// P2-10 · the POST answer is judged by protocol
// ---------------------------------------------------------------------------
const L_PARAMS_MISMATCH =
  'No se ejecutó nada. Lo que se iba a ejecutar ya no coincide con lo que leíste: la huella cambió entre tu lectura y tu decisión. Vuelve a abrir la petición y léela de nuevo; si decides, será sobre la versión nueva.'
const L_UNRECOGNISED =
  'El servidor respondió, pero esta pantalla no sabe leer su respuesta. No afirma que se ejecutó ni que falló. Mira el libro antes de repetir nada.'
const L_MALFORMED_ROW =
  'Esta fila lleva un identificador que Korvun no puede leer. No se abre ni se decide desde aquí. Las demás filas no se ven afectadas.'
const L_LOOPBACK_ONLY =
  'Esta decisión solo puede tomarse desde la misma máquina que ejecuta Korvun. La petición llegó desde otro origen y se rechazó sin tocar nada.'

async function decide(verb: 'approve' | 'reject', answer: () => Response): Promise<void> {
  await openDetail(router([ROW], DETAIL, { [verb]: answer }))
  if (verb === 'approve') {
    arm()
    fireEvent.click(approveBtn())
  } else {
    fireEvent.click(rejectBtn())
  }
  // «Volver a pendientes» is on every terminal state (UX v36 §8): a screen
  // that blanks never shows it.
  await screen.findByRole('button', { name: 'Volver a pendientes' })
}
/** The three state titles that affirm an outcome: executed, rejected, failed. */
const neverAffirmed = (): void => {
  expect(screen.queryByText('Ejecutada')).toBeNull()
  expect(screen.queryByText(/^Rechazada\./)).toBeNull()
  expect(screen.queryByText('El intento falló')).toBeNull()
}

describe('P2-10 · el protocolo de la respuesta a un POST', () => {
  const named: Array<[string, number, string]> = [
    ['not_found', 404, 'No hay ninguna petición con ese identificador.'],
    [
      'unavailable',
      503,
      'El almacén no se pudo leer en este instante. Es transitorio y no dice nada sobre la evidencia.',
    ],
    ['disabled', 409, 'APROBACIONES APAGADAS EN ESTE PERFIL'],
    ['params_digest_mismatch', 409, L_PARAMS_MISMATCH],
    ['loopback_only', 403, L_LOOPBACK_ONLY],
  ]
  for (const verb of ['approve', 'reject'] as const) {
    for (const [name, status, literal] of named) {
      it(`P2-10 · ${name} desde ${verb}: su literal aprobado, exacto; nada afirmado`, async () => {
        await decide(verb, () => json(status, { error: name, message: 'x' }))
        expect(screen.getByText(literal)).toBeInTheDocument()
        expect(screen.queryByText(UNKNOWN)).toBeNull()
        neverAffirmed()
      })
    }
  }

  // The positive half, green today by design: ONLY the exact success shape
  // paints its state. It guards the cure from over-refusing.
  it('P2-10 · control: un 200 de approve exacto pinta «Ejecutada» con su recibo', async () => {
    await decide('approve', () =>
      json(200, { outcome: 'executed', digest: DIGEST, result: 'ok', receipt_id: 'rcp_ok' }),
    )
    expect(screen.getByText('Ejecutada')).toBeInTheDocument()
    expect(screen.getByText('Recibo rcp_ok')).toBeInTheDocument()
  })
  it('P2-10 · control: un 200 de reject exacto pinta «Rechazada» con su recibo', async () => {
    await decide('reject', () => json(200, { outcome: 'rejected', receipt_id: 'rcp_ok' }))
    expect(screen.getByText(/^Rechazada\./)).toBeInTheDocument()
    expect(screen.getByText('Recibo rcp_ok')).toBeInTheDocument()
  })

  // Every other 2xx is unrecognised: literal 2, nothing affirmed.
  const unrecognised: Array<['approve' | 'reject', string, number, unknown]> = [
    ['approve', 'outcome «rejected»', 200, { outcome: 'rejected', receipt_id: 'rcp_x' }],
    ['approve', 'outcome ausente', 200, { receipt_id: 'rcp_x' }],
    ['approve', 'outcome «EXECUTED»', 200, { outcome: 'EXECUTED', receipt_id: 'rcp_x' }],
    ['approve', 'result numérico', 200, { outcome: 'executed', result: 5, receipt_id: 'rcp_x' }],
    ['approve', 'receipt_id ausente', 200, { outcome: 'executed', result: 'ok' }],
    ['approve', 'receipt_id numérico', 200, { outcome: 'executed', result: 'ok', receipt_id: 7 }],
    [
      'approve',
      'un 201 con cuerpo de éxito',
      201,
      { outcome: 'executed', result: 'ok', receipt_id: 'rcp_x' },
    ],
    [
      'approve',
      'un 202 con cuerpo de éxito',
      202,
      { outcome: 'executed', result: 'ok', receipt_id: 'rcp_x' },
    ],
    ['reject', 'outcome «executed»', 200, { outcome: 'executed', receipt_id: 'rcp_x' }],
    ['reject', 'receipt_id ausente', 200, { outcome: 'rejected' }],
    ['reject', 'receipt_id numérico', 200, { outcome: 'rejected', receipt_id: 7 }],
    ['reject', 'un 201 con cuerpo de éxito', 201, { outcome: 'rejected', receipt_id: 'rcp_x' }],
  ]
  for (const [verb, label, status, body] of unrecognised) {
    it(`P2-10 · ${verb} con ${label}: el literal de respuesta no reconocida, nada afirmado`, async () => {
      await decide(verb, () => json(status, body))
      expect(screen.getByText(L_UNRECOGNISED)).toBeInTheDocument()
      neverAffirmed()
      expect(screen.queryByText(/Recibo (undefined|7)$/)).toBeNull()
    })
  }
})

// ---------------------------------------------------------------------------
// P2-1 · the approval id is untrusted bytes
// ---------------------------------------------------------------------------
describe('P2-1 · el identificador de la aprobación', () => {
  it('P2-1 · la fila imprime el id por el alfabeto de escape', async () => {
    stubFetch(router([{ ...ROW, id: ID + '\u2060' }], DETAIL, {}))
    render(<Approvals onGoHome={() => undefined} />)
    const row = await screen.findByText(L_MALFORMED_ROW)
    expect(document.body.textContent).toContain('<U+2060>')
    expect(document.body.textContent).not.toContain('\u2060')
    expect(row).toBeInTheDocument()
  })

  for (const [label, bad] of [
    ['path traversal', 'apr_x/../../reject'],
    ['query', `${ID}?x=1`],
    ['upper-case', 'APR_' + 'AB'.repeat(16)],
  ] as const) {
    it(`P2-1 · una fila con id mal formado (${label}) se lista con su literal y no se abre`, async () => {
      stubFetch(
        router(
          [
            { ...ROW, id: bad },
            { ...ROW, id: ID, operation: 'tool/otra' },
          ],
          DETAIL,
          {},
        ),
      )
      render(<Approvals onGoHome={() => undefined} />)
      expect(await screen.findByText(L_MALFORMED_ROW)).toBeInTheDocument()
      // The healthy row stays openable: «Las demás filas no se ven afectadas».
      expect(screen.getByRole('button', { name: /tool\/otra/ })).toBeInTheDocument()
      expect(screen.queryByRole('button', { name: /tool\/webhook_call/ })).toBeNull()
      const detailGets = calls.filter(
        (c) => c.method === 'GET' && /\/api\/approvals\/./.test(c.url),
      )
      expect(detailGets).toEqual([])
    })
  }

  it('P2-1 · la línea «korvun approvals show <id>» escapa el id que sirvió el detalle', async () => {
    await openDetail(
      router(
        [ROW],
        { ...DETAIL, id: ID + '\u202e', parameters_state: 'too_large', parameters: '' },
        {},
      ),
    )
    const line = await screen.findByText(/korvun approvals show/)
    expect(line.textContent).toContain('<U+202E>')
    expect(line.textContent).not.toContain('\u202e')
  })

  it('P2-1 · hermana: el recibo tras una decisión se imprime por el alfabeto de escape', async () => {
    await openDetail(
      router([ROW], DETAIL, {
        reject: () => json(200, { outcome: 'rejected', receipt_id: 'rcp_\u202ex' }),
      }),
    )
    fireEvent.click(rejectBtn())
    const receipt = await screen.findByText(/^Recibo /)
    expect(receipt.textContent).toContain('<U+202E>')
    expect(receipt.textContent).not.toContain('\u202e')
  })
})
