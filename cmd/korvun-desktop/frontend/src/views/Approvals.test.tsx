// Aprobaciones — the RED suite for the desktop surface, one test per jsdom
// acceptance scenario of docs/superpowers/specs/2026-09-08-approvals-screen-ux.md
// (v36, veto lifted). Each test is named by its AS so the count reported is the
// count in this file and the mutation in §13 maps one-to-one.
//
// Browser scenarios (AS-2, 51-55, 63, 71, 73, 79, 99) are NOT here: they need
// the e2e harness with POST /__test/park (FR-TEST-1) and live in Playwright.
// Server scenarios (two real connections, crash-restart) are Go moulds.
//
// Evidence level: jsdom, fetch stubbed, Wails bindings stubbed. Proves what the
// screen paints and what it sends; never that the store or the executor did
// anything.
//
// Contract fixed by this file and not by the Go RED (it decodes only error
// bodies): POST /approve 200 → {outcome:"executed", digest, result, receipt_id};
// POST /reject 200 → {outcome:"rejected", receipt_id}; every named failure
// rides {error:<name>, message, current_law_digest?} on a non-2xx status.
import { render, screen, fireEvent, waitFor, within, act, cleanup } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Approvals } from './Approvals'
import { App } from '../App'

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const HEX = 'a3f91c7d0b2e4f6a8c1d3e5f7a9b0c2d4e6f8a0b1c3d5e7f9a1b3c5d7e9f1b60'
const DIGEST = `sha256:${HEX}` // sha256: + 64 lowercase hex, the form action.Digest produces
const TAIL = HEX.slice(-6)
const OTHER_DIGEST = 'sha256:' + '0'.repeat(58) + TAIL // same tail, different head (AS-43)
const NOW = new Date('2026-09-08T14:00:00Z')
const EXPIRES = '2026-09-08T14:31:00Z'

const ROW = {
  id: 'apr_1',
  action_id: 'act_1',
  operation: 'tool/webhook_call',
  effect_class: 'write_irreversible',
  expires_at: EXPIRES,
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
const LIST = { gate: GATE, rows: [ROW] }

type Call = { method: string; url: string; body: string | null }
let calls: Call[] = []

function json(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  })
}
function raw(status: number, text: string, type = 'text/html'): Response {
  return new Response(text, { status, headers: { 'content-type': type } })
}

type Route = (call: Call) => Response | Promise<Response>
function stubFetch(route: Route): void {
  calls = []
  vi.stubGlobal('fetch', (input: RequestInfo | URL, init?: RequestInit) => {
    const url =
      typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url
    const call: Call = {
      method: (init?.method ?? 'GET').toUpperCase(),
      url,
      body: typeof init?.body === 'string' ? init.body : null,
    }
    calls.push(call)
    return Promise.resolve(route(call))
  })
}

/** The default happy router: a list with one row, its detail, and 200s. */
function happy(overrides: Partial<Record<string, Route>> = {}): Route {
  return (c) => {
    const key = `${c.method} ${c.url.replace(/^https?:\/\/[^/]+/, '')}`
    for (const [k, r] of Object.entries(overrides)) if (k === key && r) return r(c)
    if (key === 'GET /api/approvals') return json(200, LIST)
    if (key === 'GET /api/approvals/apr_1') return json(200, DETAIL)
    if (key === 'POST /api/approvals/apr_1/approve')
      return json(200, { outcome: 'executed', digest: DIGEST, result: 'ok', receipt_id: 'rcp_t1' })
    if (key === 'POST /api/approvals/apr_1/reject')
      return json(200, { outcome: 'rejected', receipt_id: 'rcp_d1' })
    return raw(404, '<html>not here</html>')
  }
}

const posts = (path: string) => calls.filter((c) => c.method === 'POST' && c.url.endsWith(path))
const gets = (path: string) => calls.filter((c) => c.method === 'GET' && c.url.endsWith(path))

interface FakeWindow {
  go?: { shell?: { Desktop?: Record<string, unknown> } }
}
function bindings(d: Record<string, unknown> | undefined): void {
  const w = window as unknown as FakeWindow
  if (d === undefined) delete w.go
  else w.go = { shell: { Desktop: d } }
}
const RUNNING = {
  Status: () =>
    Promise.resolve({ Running: true, ConfigPath: '/p/korvun.json', AdminAddr: '', TokenEnv: 'X' }),
}
const STOPPED = {
  Status: () =>
    Promise.resolve({ Running: false, ConfigPath: '/p/korvun.json', AdminAddr: '', TokenEnv: 'X' }),
}

async function renderList(route: Route = happy()): Promise<void> {
  stubFetch(route)
  render(<Approvals onGoHome={() => undefined} />)
  await waitFor(() => expect(gets('/api/approvals').length).toBeGreaterThan(0))
}
async function openDetail(route: Route = happy()): Promise<void> {
  await renderList(route)
  const row = await screen.findByRole('button', { name: /tool\/webhook_call/ })
  fireEvent.click(row)
  await waitFor(() => expect(gets('/api/approvals/apr_1').length).toBe(1))
}
function armingInput(): HTMLInputElement {
  return screen.getByLabelText(
    /reteclea los seis últimos caracteres del digest/i,
  ) as HTMLInputElement
}
function typeKeys(input: HTMLElement, text: string): void {
  for (const ch of text) fireEvent.keyDown(input, { key: ch })
}
const approveBtn = () => screen.queryByRole('button', { name: 'Aprobar y ejecutar' })
const rejectBtn = () => screen.queryByRole('button', { name: 'Rechazar' })
const ESC_LINE = 'Esc rechaza mientras esta petición esté abierta y sin decidir.'
const NEVER = () => new Promise<Response>(() => undefined)

beforeEach(() => {
  vi.useFakeTimers({ now: NOW, shouldAdvanceTime: true })
  bindings(RUNNING)
})
afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  vi.useRealTimers()
  bindings(undefined)
})

// ---------------------------------------------------------------------------
// P1 · the list
// ---------------------------------------------------------------------------
describe('P1 · lista', () => {
  it('AS-20 · V1 con «2 de 3» y cero filas', async () => {
    await renderList(happy({ 'GET /api/approvals': () => json(200, { gate: GATE, rows: [] }) }))
    expect(await screen.findByText('No hay nada aparcado.')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Las aprobaciones están encendidas y 2 de 3 cerebros pueden aparcar acciones irreversibles: si uno de ellos lo intenta, aparecerá aquí.',
      ),
    ).toBeInTheDocument()
  })

  it('AS-19 · E4 con brains_can_park=0: literal de E4, «No hay nada aparcado» ausente', async () => {
    await renderList(
      happy({
        'GET /api/approvals': () => json(200, { gate: { ...GATE, brains_can_park: 0 }, rows: [] }),
      }),
    )
    expect(
      await screen.findByText('APROBACIONES ENCENDIDAS · NINGÚN CEREBRO PUEDE APARCAR'),
    ).toBeInTheDocument()
    expect(screen.getByText('Nada puede llegar a esta lista')).toBeInTheDocument()
    expect(screen.queryByText('No hay nada aparcado.')).toBeNull()
    expect(
      screen.getByRole('button', { name: 'Abrir la carpeta de configuración' }),
    ).toBeInTheDocument()
  })

  it('AS-18 · 409 disabled: literal de E3; «No hay nada aparcado» y «Nada puede llegar a esta lista» ausentes', async () => {
    await renderList(
      happy({
        'GET /api/approvals': () =>
          json(409, { error: 'disabled', message: 'approvals are disabled' }),
      }),
    )
    expect(await screen.findByText('APROBACIONES APAGADAS EN ESTE PERFIL')).toBeInTheDocument()
    expect(
      screen.getByText('Con las aprobaciones apagadas ya no se retiene nada nuevo'),
    ).toBeInTheDocument()
    expect(screen.getByText(/approvals\.enabled/)).toBeInTheDocument()
    expect(screen.queryByText('No hay nada aparcado.')).toBeNull()
    expect(screen.queryByText('Nada puede llegar a esta lista')).toBeNull()
  })

  it('AS-15 · 503 core stopped con Running=false: literal de E1, cero filas, «No hay nada aparcado» ausente', async () => {
    bindings(STOPPED)
    await renderList(happy({ 'GET /api/approvals': () => json(503, { error: 'core stopped' }) }))
    expect(await screen.findByText('El núcleo está parado')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Arrancar el núcleo' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Ir a Inicio' })).toBeInTheDocument()
    expect(screen.queryByText('No hay nada aparcado.')).toBeNull()
    expect(screen.queryByText(/tool\/webhook_call/)).toBeNull()
  })

  it('AS-59 · E1 no imprime ningún número de peticiones', async () => {
    bindings(STOPPED)
    await renderList(happy({ 'GET /api/approvals': () => json(503, { error: 'core stopped' }) }))
    const state = await screen.findByRole('status')
    expect(state.textContent).not.toMatch(/\d+ (petici|aparcad)/)
  })

  it('AS-16 · 503 core unreachable en GET: literal de E2-GET; E1 y «No hay nada aparcado» ausentes', async () => {
    await renderList(
      happy({ 'GET /api/approvals': () => json(503, { error: 'core unreachable' }) }),
    )
    expect(
      await screen.findByText(
        'El núcleo no responde. El proceso figura en marcha pero no contesta. Ninguna decisión ha salido de esta ventana.',
      ),
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Reintentar' })).toBeInTheDocument()
    expect(screen.queryByText('El núcleo está parado')).toBeNull()
    expect(screen.queryByText('No hay nada aparcado.')).toBeNull()
  })

  it('AS-30 · nombre desconocido: imprime «pepino» y «algo raro» crudos; no pinta lista vacía', async () => {
    await renderList(
      happy({ 'GET /api/approvals': () => json(418, { error: 'pepino', message: 'algo raro' }) }),
    )
    expect(await screen.findByText('Respuesta que esta pantalla no reconoce.')).toBeInTheDocument()
    expect(screen.getByText(/pepino/)).toBeInTheDocument()
    expect(screen.getByText(/algo raro/)).toBeInTheDocument()
    expect(screen.getByText(/418/)).toBeInTheDocument()
    expect(screen.queryByText('No hay nada aparcado.')).toBeNull()
  })

  it('AS-31 · fetch rechazado: literal de la última fila de E9; cero filas y «No hay nada aparcado» ausente', async () => {
    stubFetch(() => Promise.reject(new TypeError('Failed to fetch')) as unknown as Response)
    render(<Approvals onGoHome={() => undefined} />)
    expect(await screen.findByText('El núcleo no ha contestado nada legible.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Reintentar' })).toBeInTheDocument()
    expect(screen.queryByText('No hay nada aparcado.')).toBeNull()
  })

  it('AS-32 · cuerpo no-JSON con 200: mismo estado que AS-31', async () => {
    await renderList(
      happy({ 'GET /api/approvals': () => raw(200, 'not json at all', 'text/plain') }),
    )
    expect(await screen.findByText('El núcleo no ha contestado nada legible.')).toBeInTheDocument()
    expect(screen.getByText(/not json at all/)).toBeInTheDocument()
    expect(screen.queryByText('No hay nada aparcado.')).toBeNull()
  })

  it('AS-33 · 404 con cuerpo ajeno a la API: literal de «superficie no montada»', async () => {
    await renderList(
      happy({
        'GET /api/approvals': () => raw(404, '<html><body>404 page not found</body></html>'),
      }),
    )
    expect(
      await screen.findByText(/Esta ventana no encuentra la puerta de aprobaciones en el núcleo/),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Abrir la carpeta de configuración' }),
    ).toBeInTheDocument()
    expect(screen.queryByText('No hay ninguna petición con ese identificador.')).toBeNull()
  })

  it('AS-93 · forbidden (401): su literal, [Ir a Inicio], y sin la frase del efecto', async () => {
    await renderList(
      happy({
        'GET /api/approvals': () => json(401, { error: 'forbidden', message: 'bad bearer' }),
      }),
    )
    expect(
      await screen.findByText(
        'La ventana no ha podido autenticarse contra el núcleo. No ha salido ninguna decisión.',
      ),
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Ir a Inicio' })).toBeInTheDocument()
    expect(screen.queryByText(/No sabemos si el efecto llegó a ocurrir/)).toBeNull()
  })

  it('AS-10 · Esc en la lista: cero llamadas', async () => {
    await renderList()
    const before = calls.length
    fireEvent.keyDown(document.body, { key: 'Escape' })
    await act(async () => {})
    expect(calls.length).toBe(before)
  })

  it('AS-81 · la lista no dispara una segunda petición en 30 s', async () => {
    await renderList()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000)
    })
    expect(gets('/api/approvals').length).toBe(1)
  })

  it('AS-82 · una respuesta tardía de la lista no pinta si ya se navegó al detalle', async () => {
    let release: (r: Response) => void = () => undefined
    const late = new Promise<Response>((res) => (release = res))
    let first = true
    await renderList(
      happy({
        'GET /api/approvals': () => {
          if (first) {
            first = false
            return json(200, LIST)
          }
          return late
        },
      }),
    )
    fireEvent.click(screen.getByRole('button', { name: 'Actualizar' }))
    fireEvent.click(await screen.findByRole('button', { name: /tool\/webhook_call/ }))
    await screen.findByText('✓ el almacén devolvió parámetros que re-derivan este digest')
    release(json(200, { gate: GATE, rows: [] }))
    await act(async () => {})
    expect(screen.queryByText('No hay nada aparcado.')).toBeNull()
    expect(
      screen.getByText('✓ el almacén devolvió parámetros que re-derivan este digest'),
    ).toBeInTheDocument()
  })

  it('AS-40 · operation con U+202E: la fila de la lista pinta el escape y no el carácter', async () => {
    await renderList(
      happy({
        'GET /api/approvals': () =>
          json(200, { gate: GATE, rows: [{ ...ROW, operation: 'tool/‮hook' }] }),
      }),
    )
    const row = await screen.findByRole('button', { name: /tool\// })
    expect(row.textContent).toContain('<U+202E>')
    expect(row.textContent).not.toContain('‮')
  })

  it('AS-43 · dos filas con la misma cola: digests enteros y el aviso de FR-UI-12', async () => {
    await renderList(
      happy({
        'GET /api/approvals': () =>
          json(200, {
            gate: GATE,
            rows: [ROW, { ...ROW, id: 'apr_2', action_id: 'act_2', digest: OTHER_DIGEST }],
          }),
      }),
    )
    expect(
      await screen.findByText('Dos peticiones muestran la misma cola de digest; aquí van enteros'),
    ).toBeInTheDocument()
    expect(screen.getByText(DIGEST)).toBeInTheDocument()
    expect(screen.getByText(OTHER_DIGEST)).toBeInTheDocument()
  })

  it('AS-44 · digest vacío y digest malformado: «digest ilegible», sin Aprobar, fuera de la colisión', async () => {
    await renderList(
      happy({
        'GET /api/approvals': () =>
          json(200, {
            gate: GATE,
            rows: [
              { ...ROW, id: 'apr_e', digest: '' },
              { ...ROW, id: 'apr_m', digest: 'nosoyundigest' },
              { ...ROW, id: 'apr_m2', digest: 'nosoyundigest' },
            ],
          }),
      }),
    )
    expect((await screen.findAllByText('digest ilegible')).length).toBe(3)
    // Two rows share the malformed value ON PURPOSE: an unreadable digest must
    // stay out of the tail detector, so the assertion is plural.
    expect(screen.getAllByText('nosoyundigest').length).toBe(2)
    expect(
      screen.queryByText('Dos peticiones muestran la misma cola de digest; aquí van enteros'),
    ).toBeNull()
  })

  it('AS-45 · expires_at vacío: «no caduca», sin cuenta atrás', async () => {
    await renderList(
      happy({
        'GET /api/approvals': () => json(200, { gate: GATE, rows: [{ ...ROW, expires_at: '' }] }),
      }),
    )
    expect(await screen.findByText('no caduca')).toBeInTheDocument()
    expect(screen.queryByText(/quedan|faltan .* min/)).toBeNull()
  })

  it('AS-74 · fila cuyo preview no se puede leer: «SIN CLASE LEGIBLE», id y digest; no desaparece', async () => {
    await renderList(
      happy({
        'GET /api/approvals': () =>
          json(200, { gate: GATE, rows: [{ ...ROW, operation: '', effect_class: '' }] }),
      }),
    )
    expect(await screen.findByText('SIN CLASE LEGIBLE')).toBeInTheDocument()
    expect(screen.getByText(/apr_1/)).toBeInTheDocument()
    expect(screen.queryByText('IRREVERSIBLE')).toBeNull()
  })

  it('AS-61 · OpenConfigFolder que rechaza: «No se ha podido abrir la carpeta» + la ruta', async () => {
    bindings({ ...RUNNING, OpenConfigFolder: () => Promise.reject(new Error('EPERM')) })
    await renderList(
      happy({ 'GET /api/approvals': () => json(409, { error: 'disabled', message: 'x' }) }),
    )
    fireEvent.click(
      await screen.findByRole('button', { name: 'Abrir la carpeta de configuración' }),
    )
    expect(await screen.findByText(/No se ha podido abrir la carpeta/)).toBeInTheDocument()
    expect(screen.getByText(/\/p\/korvun\.json/)).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// P2 · the document
// ---------------------------------------------------------------------------
describe('P2 · detalle', () => {
  it('AS-1 · el digest es el primer bloque y ambos botones van después de los parámetros en el DOM', async () => {
    await openDetail()
    const doc = await screen.findByRole('article')
    const digestBlock = within(doc).getByText('EL DIGEST — EXACTAMENTE ESTO SE EJECUTARÁ')
    const params = within(doc).getByText('PARÁMETROS — LITERALES')
    const reject = rejectBtn()!
    const approve = approveBtn()!
    expect(doc.firstElementChild!.contains(digestBlock)).toBe(true)
    expect(params.compareDocumentPosition(reject) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(params.compareDocumentPosition(approve) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(reject.compareDocumentPosition(approve) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('AS-3 · sin teclear: Aprobar disabled, Rechazar habilitado', async () => {
    await openDetail()
    expect(approveBtn()).toBeDisabled()
    expect(rejectBtn()).toBeEnabled()
  })

  it('AS-4 · seis correctos: «✓ coincide» y Aprobar habilitado', async () => {
    await openDetail()
    typeKeys(armingInput(), TAIL)
    expect(await screen.findByText('✓ coincide')).toBeInTheDocument()
    expect(approveBtn()).toBeEnabled()
  })

  it('AS-5 · paste: campo vacío, «Pegar no arma…», Aprobar disabled', async () => {
    await openDetail()
    fireEvent.paste(armingInput(), { clipboardData: { getData: () => TAIL } })
    expect(await screen.findByText('Pegar no arma: teclea los seis caracteres')).toBeInTheDocument()
    expect(armingInput().value).toBe('')
    expect(approveBtn()).toBeDisabled()
  })

  it('AS-6 · drop: idéntico a AS-5', async () => {
    await openDetail()
    fireEvent.drop(armingInput(), { dataTransfer: { getData: () => TAIL } })
    expect(await screen.findByText('Pegar no arma: teclea los seis caracteres')).toBeInTheDocument()
    expect(armingInput().value).toBe('')
    expect(approveBtn()).toBeDisabled()
  })

  it('AS-7 · clase write_compensatable: banda ANOMALÍA con su frase exacta y armado presente', async () => {
    await openDetail(
      happy({
        'GET /api/approvals/apr_1': () =>
          json(200, { ...DETAIL, effect_class: 'write_compensatable' }),
      }),
    )
    expect(await screen.findByText(/ANOMALÍA · write_compensatable/)).toBeInTheDocument()
    expect(
      screen.getByText(
        'Esta petición no debería existir: el gate solo aparca irreversible y crítico. Trátala como sospechosa.',
      ),
    ).toBeInTheDocument()
    expect(armingInput()).toBeInTheDocument()
  })

  it('AS-8 · clase "garabato": «CLASE DESCONOCIDA» y armado presente', async () => {
    await openDetail(
      happy({
        'GET /api/approvals/apr_1': () => json(200, { ...DETAIL, effect_class: 'garabato' }),
      }),
    )
    expect(await screen.findByText('CLASE DESCONOCIDA')).toBeInTheDocument()
    expect(
      screen.getByText('fuera de la escalera: se trata por encima de crítico'),
    ).toBeInTheDocument()
    expect(armingInput()).toBeInTheDocument()
  })

  it('AS-46 · reversibility vacío: «el registro no declara reversibilidad» y el armado sigue en una irreversible', async () => {
    await openDetail(
      happy({ 'GET /api/approvals/apr_1': () => json(200, { ...DETAIL, reversibility: '' }) }),
    )
    expect(await screen.findByText('el registro no declara reversibilidad')).toBeInTheDocument()
    expect(armingInput()).toBeInTheDocument()
  })

  it('AS-56 · irreversible y crítica: botones con idéntica caja (FR-UI-9)', async () => {
    await openDetail()
    const a1 = { cls: approveBtn()!.className, style: approveBtn()!.getAttribute('style') }
    const r1 = { cls: rejectBtn()!.className, style: rejectBtn()!.getAttribute('style') }
    cleanup()
    await openDetail(
      happy({
        'GET /api/approvals/apr_1': () => json(200, { ...DETAIL, effect_class: 'critical' }),
      }),
    )
    await screen.findByText('CRÍTICO')
    expect({ cls: approveBtn()!.className, style: approveBtn()!.getAttribute('style') }).toEqual(a1)
    expect({ cls: rejectBtn()!.className, style: rejectBtn()!.getAttribute('style') }).toEqual(r1)
  })

  it('AS-57 · ningún role="dialog" en ningún estado', async () => {
    await openDetail()
    expect(screen.queryByRole('dialog')).toBeNull()
    fireEvent.click(rejectBtn()!)
    await screen.findByText(/Rechazada\./)
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('AS-58 · ninguno de los dos botones lleva la clase del gradiente de identidad', async () => {
    await openDetail()
    expect(approveBtn()!.className).not.toMatch(/gradient/i)
    expect(rejectBtn()!.className).not.toMatch(/gradient/i)
  })

  it('AS-60 · la línea de Esc es la misma cadena exacta en irreversible, crítica y anomalía', async () => {
    for (const cls of ['write_irreversible', 'critical', 'write_compensatable']) {
      cleanup()
      await openDetail(
        happy({ 'GET /api/approvals/apr_1': () => json(200, { ...DETAIL, effect_class: cls }) }),
      )
      expect(await screen.findByText(ESC_LINE)).toBeInTheDocument()
    }
  })

  it('AS-76 · el detalle no imprime law_version en ningún estado', async () => {
    await openDetail(
      happy({ 'GET /api/approvals/apr_1': () => json(200, { ...DETAIL, law_version: 'v3' }) }),
    )
    await screen.findByText('LA LEY QUE LO EXIGIÓ')
    expect(document.body.textContent).not.toMatch(/law_version|\bv3\b/)
  })

  it('AS-38 · parámetros con U+202E: el DOM contiene <U+202E> y no el carácter crudo', async () => {
    await openDetail(
      happy({
        'GET /api/approvals/apr_1': () =>
          json(200, { ...DETAIL, parameters: 'https://hooks.acme.io‮oi.emca' }),
      }),
    )
    const params = await screen.findByTestId('approval-parameters')
    expect(params.textContent).toContain('<U+202E>')
    expect(params.textContent).not.toContain('‮')
  })

  it('AS-72 · parámetros con U+200B y U+FEFF: escapes visibles, no los crudos', async () => {
    await openDetail(
      happy({ 'GET /api/approvals/apr_1': () => json(200, { ...DETAIL, parameters: 'a​b﻿c' }) }),
    )
    const params = await screen.findByTestId('approval-parameters')
    expect(params.textContent).toContain('<U+200B>')
    expect(params.textContent).toContain('<U+FEFF>')
    expect(params.textContent).not.toMatch(/[\u200B\uFEFF]/)
  })

  it('AS-39 · purpose con U+202E: idéntico en el bloque ORIGEN', async () => {
    await openDetail(
      happy({ 'GET /api/approvals/apr_1': () => json(200, { ...DETAIL, purpose: 'ver‮reb' }) }),
    )
    const origin = await screen.findByTestId('approval-origin')
    expect(origin.textContent).toContain('<U+202E>')
    expect(origin.textContent).not.toContain('‮')
  })

  it('AS-41 · parámetros con una URL: cero elementos <a> en el documento', async () => {
    await openDetail()
    const doc = await screen.findByRole('article')
    expect(doc.querySelectorAll('a').length).toBe(0)
  })

  it('AS-42 · detalle abierto 30 s: una sola petición del detalle', async () => {
    await openDetail()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000)
    })
    expect(gets('/api/approvals/apr_1').length).toBe(1)
  })

  it('AS-36 · parameters_state too_large: su literal, Aprobar ausente, la CLI nombrada', async () => {
    await openDetail(
      happy({
        'GET /api/approvals/apr_1': () =>
          json(200, { ...DETAIL, parameters: '', parameters_state: 'too_large' }),
      }),
    )
    expect(
      await screen.findByText(
        /esta pantalla no puede enseñarte esta petición entera, así que no te ofrece el sí/,
      ),
    ).toBeInTheDocument()
    expect(approveBtn()).toBeNull()
    expect(screen.getByText(/korvun approvals show/)).toBeInTheDocument()
  })

  it('AS-64 · parameters_state empty: su literal y Aprobar ausente', async () => {
    await openDetail(
      happy({
        'GET /api/approvals/apr_1': () =>
          json(200, { ...DETAIL, parameters: '', parameters_state: 'empty' }),
      }),
    )
    expect(
      await screen.findByText(
        /esta acción se aparcó sin parámetros, y así no se puede ejecutar: el claim rechaza la fila vacía/,
      ),
    ).toBeInTheDocument()
    expect(approveBtn()).toBeNull()
  })

  it('AS-37 · present con parámetros vacíos: respuesta ilegible del núcleo, sin Aprobar', async () => {
    await openDetail(
      happy({
        'GET /api/approvals/apr_1': () =>
          json(200, { ...DETAIL, parameters: '', parameters_state: 'present' }),
      }),
    )
    expect(await screen.findByText('El núcleo no ha contestado nada legible.')).toBeInTheDocument()
    expect(approveBtn()).toBeNull()
  })

  it('AS-34 · parameters_state purged: respuesta ilegible del núcleo, nunca un estado normal', async () => {
    await openDetail(
      happy({
        'GET /api/approvals/apr_1': () =>
          json(200, { ...DETAIL, parameters: '', parameters_state: 'purged' }),
      }),
    )
    expect(await screen.findByText('El núcleo no ha contestado nada legible.')).toBeInTheDocument()
    expect(approveBtn()).toBeNull()
    expect(rejectBtn()).toBeNull()
  })

  it('AS-78 · 200 con brain_gone:true: documento entero, Rechazar habilitado, Aprobar ausente, sin estado terminal', async () => {
    await openDetail(
      happy({ 'GET /api/approvals/apr_1': () => json(200, { ...DETAIL, brain_gone: true }) }),
    )
    expect(await screen.findByText('PARÁMETROS — LITERALES')).toBeInTheDocument()
    expect(rejectBtn()).toBeEnabled()
    expect(approveBtn()).toBeNull()
    expect(screen.queryByRole('button', { name: 'Volver a pendientes' })).toBeNull()
  })

  it('AS-80 · fila EXPIRED ⇒ expired; fila REJECTED ⇒ already_decided: dos literales', async () => {
    await openDetail(
      happy({
        'GET /api/approvals/apr_1': () => json(409, { error: 'expired', message: EXPIRES }),
      }),
    )
    expect(
      await screen.findByText('Esta petición caducó y ya no se puede decidir'),
    ).toBeInTheDocument()
    cleanup()
    await openDetail(
      happy({
        'GET /api/approvals/apr_1': () =>
          json(409, { error: 'already_decided', message: 'decided' }),
      }),
    )
    expect(
      await screen.findByText(
        /Esta petición ya no está esperando decisión, y esta ventana no la ha decidido\./,
      ),
    ).toBeInTheDocument()
  })

  it('AS-92 · params_digest_mismatch en el GET: literal de E6-ter, sin decisión, sin «Es transitorio» ni «no reconoce»', async () => {
    await openDetail(
      happy({
        'GET /api/approvals/apr_1': () =>
          json(409, { error: 'params_digest_mismatch', message: 'x' }),
      }),
    )
    expect(
      await screen.findByText('Los parámetros guardados no reproducen el digest de esta petición'),
    ).toBeInTheDocument()
    expect(
      screen.getByText('Esto no es transitorio. Guarda el identificador y mira el libro.'),
    ).toBeInTheDocument()
    expect(approveBtn()).toBeNull()
    expect(rejectBtn()).toBeNull()
    expect(screen.queryByText(/Es transitorio/)).toBeNull()
    expect(screen.queryByText('Respuesta que esta pantalla no reconoce.')).toBeNull()
  })

  it('AS-23 · 409 invalidated en el GET: literal de E7, Rechazar presente, Aprobar ausente, «Es transitorio» ausente', async () => {
    await openDetail(
      happy({
        'GET /api/approvals/apr_1': () =>
          json(409, {
            error: 'invalidated',
            message: 'law moved',
            current_law_digest: 'sha256:31c0f7ae' + 'd'.repeat(56),
          }),
      }),
    )
    expect(
      await screen.findByText('La ley bajo la que se aparcó esta petición ya no existe'),
    ).toBeInTheDocument()
    expect(
      screen.getByText('Rechazarla sí funciona: retirar autoridad es seguro bajo cualquier ley.'),
    ).toBeInTheDocument()
    expect(screen.getByText(/sha256:31c0f7ae/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Rechazar esta petición' })).toBeInTheDocument()
    expect(approveBtn()).toBeNull()
    expect(screen.queryByText(/Es transitorio/)).toBeNull()
  })

  it('AS-24 · 409 evidence_corrupt con message preview_effect_mismatch: literal de E8, cinturón impreso, sin botones', async () => {
    await openDetail(
      happy({
        'GET /api/approvals/apr_1': () =>
          json(409, { error: 'evidence_corrupt', message: 'preview_effect_mismatch' }),
      }),
    )
    expect(
      await screen.findByText('La evidencia de esta petición no cuadra consigo misma'),
    ).toBeInTheDocument()
    expect(screen.getByText(/preview_effect_mismatch/)).toBeInTheDocument()
    expect(approveBtn()).toBeNull()
    expect(rejectBtn()).toBeNull()
    expect(screen.queryByRole('button', { name: 'Rechazar esta petición' })).toBeNull()
  })

  it('AS-47 · unavailable: su literal; «no existe» ausente', async () => {
    await openDetail(
      happy({
        'GET /api/approvals/apr_1': () =>
          json(503, { error: 'unavailable', message: 'store busy' }),
      }),
    )
    expect(
      await screen.findByText(
        'El almacén no se pudo leer en este instante. Es transitorio y no dice nada sobre la evidencia.',
      ),
    ).toBeInTheDocument()
    expect(screen.queryByText(/no existe/)).toBeNull()
    expect(screen.getByRole('button', { name: 'Reintentar' })).toBeInTheDocument()
  })

  it('AS-90 · nombre desconocido en un GET: [Reintentar] presente', async () => {
    await openDetail(
      happy({ 'GET /api/approvals/apr_1': () => json(500, { error: 'zumo', message: 'raro' }) }),
    )
    expect(await screen.findByText('Respuesta que esta pantalla no reconoce.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Reintentar' })).toBeInTheDocument()
  })
})

// ---------------------------------------------------------------------------
// P3 · the decision, Esc, arming
// ---------------------------------------------------------------------------
describe('P3 · decisión', () => {
  it('AS-9 · Esc con motivo: un POST a /reject con ese comentario y cero a /approve', async () => {
    await openDetail()
    fireEvent.change(screen.getByLabelText('Motivo del rechazo (opcional)'), {
      target: { value: 'no en la ceremonia' },
    })
    fireEvent.keyDown(document.body, { key: 'Escape' })
    await waitFor(() => expect(posts('/reject').length).toBe(1))
    expect(JSON.parse(posts('/reject')[0].body!)).toEqual({ comment: 'no en la ceremonia' })
    expect(posts('/approve').length).toBe(0)
  })

  it('AS-11 · Esc durante «Ejecutando»: cero llamadas', async () => {
    await openDetail(happy({ 'POST /api/approvals/apr_1/approve': NEVER }))
    typeKeys(armingInput(), TAIL)
    fireEvent.click(approveBtn()!)
    expect(
      await screen.findByText('Ejecutando la acción aprobada. No cierres la ventana.'),
    ).toBeInTheDocument()
    const before = calls.length
    fireEvent.keyDown(document.body, { key: 'Escape' })
    await act(async () => {})
    expect(calls.length).toBe(before)
    expect(posts('/reject').length).toBe(0)
  })

  it('AS-50 · el cuerpo de todo POST a /approve tiene exactamente la clave digest con el digest servido', async () => {
    await openDetail()
    typeKeys(armingInput(), TAIL)
    fireEvent.click(approveBtn()!)
    await waitFor(() => expect(posts('/approve').length).toBe(1))
    expect(JSON.parse(posts('/approve')[0].body!)).toEqual({ digest: DIGEST })
  })

  it('AS-25 · dos clics en Aprobar: un solo POST y el desenlace de esa petición, no already_decided', async () => {
    let release: (r: Response) => void = () => undefined
    await openDetail(
      happy({
        'POST /api/approvals/apr_1/approve': () => new Promise<Response>((res) => (release = res)),
      }),
    )
    typeKeys(armingInput(), TAIL)
    const btn = approveBtn()!
    fireEvent.click(btn)
    fireEvent.click(btn)
    expect(btn).toBeDisabled()
    release(json(200, { outcome: 'executed', digest: DIGEST, result: 'ok', receipt_id: 'rcp_t1' }))
    expect(await screen.findByText(/rcp_t1/)).toBeInTheDocument()
    expect(posts('/approve').length).toBe(1)
    expect(screen.queryByText(/ya no está esperando decisión/)).toBeNull()
  })

  it('AS-12 · 409 digest_mismatch: literal de E6 y ningún control que apruebe', async () => {
    await openDetail(
      happy({
        'POST /api/approvals/apr_1/approve': () =>
          json(409, { error: 'digest_mismatch', message: 'x' }),
      }),
    )
    typeKeys(armingInput(), TAIL)
    fireEvent.click(approveBtn()!)
    expect(
      await screen.findByText('La petición cambió entre que la leíste y que la aprobaste'),
    ).toBeInTheDocument()
    expect(screen.getByText('No se ha ejecutado nada y nada se ha consumido.')).toBeInTheDocument()
    expect(approveBtn()).toBeNull()
    expect(screen.getByRole('button', { name: 'Volver a leer la petición' })).toBeInTheDocument()
  })

  it('AS-13 · 409 digest_mismatch: el digest nuevo no aparece en el DOM', async () => {
    const NEW = 'sha256:' + 'e'.repeat(64)
    await openDetail(
      happy({
        'POST /api/approvals/apr_1/approve': () =>
          json(409, { error: 'digest_mismatch', message: `stored is ${NEW}` }),
      }),
    )
    typeKeys(armingInput(), TAIL)
    fireEvent.click(approveBtn()!)
    await screen.findByText('La petición cambió entre que la leíste y que la aprobaste')
    expect(document.body.textContent).not.toContain(NEW)
    expect(document.body.textContent).not.toContain('e'.repeat(16))
  })

  it('AS-14 · [Volver a leer] tras E6: se repite el GET y el armado queda vacío', async () => {
    await openDetail(
      happy({
        'POST /api/approvals/apr_1/approve': () =>
          json(409, { error: 'digest_mismatch', message: 'x' }),
      }),
    )
    typeKeys(armingInput(), TAIL)
    fireEvent.click(approveBtn()!)
    fireEvent.click(await screen.findByRole('button', { name: 'Volver a leer la petición' }))
    await waitFor(() => expect(gets('/api/approvals/apr_1').length).toBe(2))
    expect(armingInput().value).toBe('')
    expect(approveBtn()).toBeDisabled()
  })

  it('AS-70 · el armado sobrevive a un blur de la ventana y muere al navegar a la lista', async () => {
    await openDetail()
    typeKeys(armingInput(), TAIL)
    fireEvent.blur(window)
    expect(armingInput().value).toBe(TAIL)
    expect(approveBtn()).toBeEnabled()
    fireEvent.click(screen.getByRole('button', { name: '← Pendientes' }))
    await screen.findByRole('button', { name: /tool\/webhook_call/ })
    fireEvent.click(screen.getByRole('button', { name: /tool\/webhook_call/ }))
    await screen.findByText('✓ el almacén devolvió parámetros que re-derivan este digest')
    expect(armingInput().value).toBe('')
  })

  it('AS-21 · reloj adelantado sobre un PENDING vigente: Aprobar retirado, armado borrado, Rechazar habilitado, «caducó» ausente', async () => {
    await openDetail()
    typeKeys(armingInput(), TAIL)
    await act(async () => {
      await vi.advanceTimersByTimeAsync(32 * 60_000)
    })
    expect(
      await screen.findByText(
        'El reloj de esta ventana dice que esta petición ya ha caducado. Quien lo juzga es el servidor, en el toque de la decisión.',
      ),
    ).toBeInTheDocument()
    expect(approveBtn()).toBeNull()
    expect(rejectBtn()).toBeEnabled()
    expect(screen.queryByText(/Esta petición caducó/)).toBeNull()
  })

  it('AS-22 · 409 expired: literal de E5 con el instante UTC; ambos botones ausentes', async () => {
    await openDetail(
      happy({
        'POST /api/approvals/apr_1/approve': () => json(409, { error: 'expired', message: 'x' }),
      }),
    )
    typeKeys(armingInput(), TAIL)
    fireEvent.click(approveBtn()!)
    expect(
      await screen.findByText('Esta petición caducó y ya no se puede decidir'),
    ).toBeInTheDocument()
    expect(screen.getByText(new RegExp(`Caducó a las ${EXPIRES}`))).toBeInTheDocument()
    expect(screen.getByText('DIGEST — YA NO ACCIONABLE')).toBeInTheDocument()
    expect(approveBtn()).toBeNull()
    expect(rejectBtn()).toBeNull()
  })

  it('AS-17 · 503 core unreachable en POST: desenlace desconocido; «Ninguna decisión ha salido de esta ventana» ausente', async () => {
    await openDetail(
      happy({
        'POST /api/approvals/apr_1/approve': () => json(503, { error: 'core unreachable' }),
      }),
    )
    typeKeys(armingInput(), TAIL)
    fireEvent.click(approveBtn()!)
    expect(await screen.findByText(/No sabemos si el efecto llegó a ocurrir/)).toBeInTheDocument()
    expect(screen.queryByText(/Ninguna decisión ha salido de esta ventana/)).toBeNull()
  })

  it('AS-26 · respuesta de /reject perdida: desenlace desconocido simétrico; «Rechazada» ausente', async () => {
    await openDetail(
      happy({
        'POST /api/approvals/apr_1/reject': () =>
          Promise.reject(new TypeError('Failed to fetch')) as never,
      }),
    )
    fireEvent.click(rejectBtn()!)
    expect(
      await screen.findByText('El rechazo salió de esta ventana y no hemos recibido su desenlace.'),
    ).toBeInTheDocument()
    expect(screen.queryByText(/Rechazada\./)).toBeNull()
  })

  it('AS-94 · brain_gone en un POST: literal de E9-bis, Rechazar habilitado, Aprobar ausente', async () => {
    await openDetail(
      happy({
        'POST /api/approvals/apr_1/approve': () => json(409, { error: 'brain_gone', message: 'x' }),
      }),
    )
    typeKeys(armingInput(), TAIL)
    fireEvent.click(approveBtn()!)
    expect(
      await screen.findByText('El cerebro que pidió esta acción ya no está en el perfil'),
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Rechazar esta petición' })).toBeEnabled()
    expect(approveBtn()).toBeNull()
  })

  it('AS-89 · invalidated en un POST antes del commit: E7 con Rechazar; «La decisión quedó registrada» ausente', async () => {
    await openDetail(
      happy({
        'POST /api/approvals/apr_1/approve': () =>
          json(409, {
            error: 'invalidated',
            message: 'x',
            current_law_digest: 'sha256:31c0f7ae' + 'd'.repeat(56),
          }),
      }),
    )
    typeKeys(armingInput(), TAIL)
    fireEvent.click(approveBtn()!)
    expect(
      await screen.findByText('La ley bajo la que se aparcó esta petición ya no existe'),
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Rechazar esta petición' })).toBeInTheDocument()
    expect(screen.queryByText(/La decisión quedó registrada/)).toBeNull()
  })

  it('AS-90 · nombre desconocido en un POST: desenlace desconocido y [Reintentar] ausente', async () => {
    await openDetail(
      happy({
        'POST /api/approvals/apr_1/approve': () => json(500, { error: 'zumo', message: 'raro' }),
      }),
    )
    typeKeys(armingInput(), TAIL)
    fireEvent.click(approveBtn()!)
    expect(await screen.findByText('Respuesta que esta pantalla no reconoce.')).toBeInTheDocument()
    expect(screen.getByText(/zumo/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Reintentar' })).toBeNull()
  })

  it('AS-69 · Esc en E5, E6, E6-bis, E7, E8 y E9: cero llamadas en los seis', async () => {
    const states: Array<[string, number]> = [
      ['expired', 409],
      ['digest_mismatch', 409],
      ['params_belt_failed', 409],
      ['invalidated', 409],
      ['evidence_corrupt', 409],
      ['already_decided', 409],
    ]
    for (const [name, status] of states) {
      cleanup()
      await openDetail(
        happy({
          'POST /api/approvals/apr_1/approve': () => json(status, { error: name, message: 'x' }),
        }),
      )
      typeKeys(armingInput(), TAIL)
      fireEvent.click(approveBtn()!)
      await screen.findByRole('button', { name: 'Volver a pendientes' })
      const before = calls.length
      fireEvent.keyDown(document.body, { key: 'Escape' })
      await act(async () => {})
      expect(calls.length, name).toBe(before)
    }
  })
})

// ---------------------------------------------------------------------------
// P4 / P5 · after yes / after no
// ---------------------------------------------------------------------------
describe('P4 · desenlaces', () => {
  async function approveWith(name: string, message = 'x'): Promise<void> {
    await openDetail(
      happy({ 'POST /api/approvals/apr_1/approve': () => json(409, { error: name, message }) }),
    )
    typeKeys(armingInput(), TAIL)
    fireEvent.click(approveBtn()!)
    await screen.findByRole('button', { name: 'Volver a pendientes' })
  }

  it('AS-27 · ejecución fallida (herramienta): literal de unknown_outcome; «korvun approvals execute» ausente', async () => {
    await approveWith('unknown_outcome', 'tool failed: 500')
    expect(
      screen.getByText(
        /La decisión salió de esta ventana\. No sabemos si el efecto llegó a ocurrir/,
      ),
    ).toBeInTheDocument()
    expect(document.body.textContent).not.toContain('korvun approvals execute')
  })

  it('AS-66 · unknown_outcome: «no llegó a salir» ausente, sea cual sea el texto del error', async () => {
    await approveWith('unknown_outcome', 'exec: not started')
    expect(document.body.textContent).not.toContain('no llegó a salir')
  })

  it('AS-28 · close_failed: «esta ejecución no pudo cerrar el registro» presente; las frases a secas ausentes', async () => {
    await approveWith('close_failed', 'finish: invalid transition')
    expect(screen.getByText(/esta ejecución no pudo cerrar el registro/)).toBeInTheDocument()
    expect(document.body.textContent).not.toContain('la ejecución falló')
    expect(document.body.textContent).not.toMatch(/(^|[^:]) el registro no se cerró/)
  })

  it('AS-29 · desenlace desconocido tras aprobar: «korvun approvals execute» y «no se ejecutó» ausentes', async () => {
    await approveWith('unknown_outcome')
    expect(document.body.textContent).not.toContain('korvun approvals execute')
    expect(document.body.textContent).not.toContain('no se ejecutó')
  })

  it('AS-77 · not_started_params_held: «korvun approvals execute» presente con su condicional; la frase del cierre ausente', async () => {
    await approveWith('not_started_params_held', 'database is locked (5)')
    expect(screen.getByText(/korvun approvals execute/)).toBeInTheDocument()
    expect(screen.getByText(/una vez restaurada la causa/)).toBeInTheDocument()
    expect(document.body.textContent).not.toContain('el próximo arranque la cerrará')
  })

  it('AS-103-bis · not_started_params_held: sin promesa de exclusividad, con «sobre otros ejecutores esta pantalla no se pronuncia»', async () => {
    await approveWith('not_started_params_held', 'database is locked (5)')
    expect(
      screen.getByText(/Sobre otros ejecutores esta pantalla no se pronuncia/),
    ).toBeInTheDocument()
    expect(document.body.textContent).not.toContain('y así se queda')
  })

  it('AS-95 · not_started_params_gone: la frase del cierre presente y condicionada; «korvun approvals execute» ausente', async () => {
    await approveWith('not_started_params_gone', 'already claimed or closed')
    expect(
      screen.getByText(
        /si la acción sigue en el libro, el próximo arranque la cerrará como desenlace desconocido/,
      ),
    ).toBeInTheDocument()
    expect(document.body.textContent).not.toContain('korvun approvals execute')
  })

  it('AS-97 · params_unaccounted: su literal; «Otra ejecución se llevó esta petición» ausente', async () => {
    await approveWith('params_unaccounted', 'x')
    expect(
      screen.getByText(/esta pantalla no puede afirmar que la petición no se haya ejecutado/),
    ).toBeInTheDocument()
    expect(document.body.textContent).not.toContain('Otra ejecución se llevó esta petición')
  })

  it('AS-91 · los nueve desenlaces sin comando: «korvun approvals execute» ausente en todos', async () => {
    for (const name of [
      'not_decided',
      'already_closed',
      'params_unreadable',
      'decided_evidence_corrupt',
      'params_unaccounted',
      'params_belt_failed',
      'unknown_outcome',
      'close_failed',
      'not_started_params_gone',
    ]) {
      cleanup()
      await approveWith(name, 'x')
      expect(document.body.textContent, name).not.toContain('korvun approvals execute')
    }
  })

  it('AS-68 · E6-bis no reutiliza el texto de E6: «léela otra vez, entera» ausente', async () => {
    await approveWith('params_belt_failed', 'x')
    expect(document.body.textContent).not.toContain('leerla otra vez, entera')
    expect(document.body.textContent).not.toContain('léela otra vez, entera')
  })

  it('AS-109 (literal) · not_decided: «sigue esperando decisión» presente; «dejó de estar esperando» ausente', async () => {
    await approveWith('not_decided', 'approval apr_1 is PENDING')
    expect(screen.getByText(/sigue esperando decisión/)).toBeInTheDocument()
    expect(document.body.textContent).not.toContain('dejó de estar esperando')
  })

  it('AS-106 (literal) · already_closed no atribuye el cierre a nadie', async () => {
    await approveWith('already_closed', 'SUCCEEDED')
    expect(screen.getByText(/SUCCEEDED/)).toBeInTheDocument()
    expect(document.body.textContent).not.toContain('otra ejecución o el barrendero')
  })

  it('AS-113 (literal) · decided_evidence_corrupt: sin «korvun approvals execute», sin «restaura», con la decisión sellada', async () => {
    await approveWith('decided_evidence_corrupt', 'preview_policy_mismatch')
    expect(screen.getByText(/quedó registrada y sellada/)).toBeInTheDocument()
    expect(screen.getByText(/preview_policy_mismatch/)).toBeInTheDocument()
    expect(document.body.textContent).not.toContain('korvun approvals execute')
    expect(document.body.textContent).not.toContain('restaura')
  })

  it('AS-111 (literal) · params_unreadable: «pudo llevárselos otra ejecución» ausente', async () => {
    await approveWith('params_unreadable', 'driver: i/o error')
    expect(screen.getByText(/el almacén no se pudo leer/)).toBeInTheDocument()
    expect(document.body.textContent).not.toContain('pudo llevárselos otra ejecución')
  })

  it('P5 · rechazo con 200: «Rechazada.» y el recibo de la decisión', async () => {
    await openDetail()
    fireEvent.click(rejectBtn()!)
    expect(
      await screen.findByText('Rechazada. La acción aparcada se cierra con su recibo sellado.'),
    ).toBeInTheDocument()
    expect(screen.getByText(/rcp_d1/)).toBeInTheDocument()
    expect(document.body.textContent).not.toMatch(/se le avis|tendrá que volver a pedirla/)
  })
})

// ---------------------------------------------------------------------------
// Shell · NAV
// ---------------------------------------------------------------------------
describe('NAV', () => {
  // The shell renders WITHOUT Wails bindings here, which is a supported path
  // (lib/go.ts: outside the real window every accessor degrades honestly) and
  // keeps these four about the nav entry rather than about a stub of the whole
  // Desktop surface.
  beforeEach(() => {
    bindings(undefined)
  })

  it('AS-48 · con aprobaciones apagadas, la entrada «Aprobaciones» sigue en el NAV', async () => {
    stubFetch(happy({ 'GET /api/approvals': () => json(409, { error: 'disabled', message: 'x' }) }))
    render(<App />)
    const nav = screen.getByRole('navigation', { name: 'Secciones' })
    expect(within(nav).getByRole('button', { name: 'Aprobaciones' })).toBeInTheDocument()
    expect(within(nav).getByRole('button', { name: 'Aprobaciones' })).not.toHaveAttribute(
      'disabled',
    )
  })

  it('AS-62 · la entrada del NAV no lleva data-unread', () => {
    stubFetch(happy())
    render(<App />)
    const nav = screen.getByRole('navigation', { name: 'Secciones' })
    expect(within(nav).getByRole('button', { name: 'Aprobaciones' })).not.toHaveAttribute(
      'data-unread',
    )
  })

  it('AS-75 · la entrada «Aprobaciones» contiene un svg, entre Actividad y Ajustes', () => {
    stubFetch(happy())
    render(<App />)
    const nav = screen.getByRole('navigation', { name: 'Secciones' })
    const labels = Array.from(nav.querySelectorAll('button')).map((b) => b.textContent?.trim())
    expect(labels).toEqual([
      'Inicio',
      'Builder',
      'Chat',
      'Canales',
      'Actividad',
      'Aprobaciones',
      'Ajustes',
    ])
    expect(
      within(nav).getByRole('button', { name: 'Aprobaciones' }).querySelector('svg'),
    ).not.toBeNull()
  })

  it('AS-49 · con aprobaciones apagadas, el StatusChip global no cambia de etiqueta', async () => {
    stubFetch(happy({ 'GET /api/approvals': () => json(409, { error: 'disabled', message: 'x' }) }))
    render(<App />)
    const chipBefore = screen.getByTestId('status-chip').textContent
    fireEvent.click(screen.getByRole('button', { name: 'Aprobaciones' }))
    await screen.findByText('APROBACIONES APAGADAS EN ESTE PERFIL')
    expect(screen.getByTestId('status-chip').textContent).toBe(chipBefore)
  })
})

// ---------------------------------------------------------------------------
// MUT · las cuatro ramas que la batería de mutaciones encontró SIN VIGILAR
//
// Dieciocho mutaciones probatorias sobre la pantalla; catorce enrojecieron.
// Estas cuatro no, y ninguna era una mutación mal hecha: eran ramas que el
// código sí tiene y que ningún molde recorría. La doctrina lo llama por su
// nombre — un test cuya mutación no lo enrojece ES el hallazgo — así que la
// rama se vigila aquí, con la mutación que cada molde debe enrojecer escrita
// al lado. Nivel de evidencia: en proceso, jsdom, contra un fetch de mentira.
// ---------------------------------------------------------------------------
describe('MUT · ramas sin vigilar', () => {
  // Mutación: `typeof body !== 'object'` degradando a `{kind:'ok', value:{}}`.
  it('MUT-1 · cuerpo JSON que no es un objeto: ilegible, nunca documento vacío', async () => {
    await renderList(happy({ 'GET /api/approvals': () => json(200, 42) }))
    expect(await screen.findByText('El núcleo no ha contestado nada legible.')).toBeInTheDocument()
    expect(screen.queryByText('No hay nada aparcado.')).toBeNull()
  })

  // Mutación: anular la segunda rama del 404 en `ask` (la del cuerpo que SÍ
  // parsea). AS-33 solo recorre la primera, la del cuerpo que no es JSON.
  it('MUT-2 · 404 con un error nuestro y sin message: «superficie no montada»', async () => {
    await renderList(
      happy({ 'GET /api/approvals': () => json(404, { error: '404 page not found' }) }),
    )
    expect(
      await screen.findByText(/Esta ventana no encuentra la puerta de aprobaciones en el núcleo/),
    ).toBeInTheDocument()
    expect(screen.queryByText('Respuesta que esta pantalla no reconoce.')).toBeNull()
  })

  // Mutación: ignorar `gate.approvals_enabled` con cero filas. AS-18 fija el
  // camino del 409; este fija el cinturón de al lado — un 200 con el gate
  // apagado jamás se pinta como bandeja vacía, que es el fail-open de V1.
  it('MUT-3 · gate apagado en un 200 con cero filas: literal de E3, nunca V1', async () => {
    await renderList(
      happy({
        'GET /api/approvals': () =>
          json(200, { gate: { ...GATE, approvals_enabled: false }, rows: [] }),
      }),
    )
    expect(await screen.findByText('APROBACIONES APAGADAS EN ESTE PERFIL')).toBeInTheDocument()
    expect(screen.queryByText('No hay nada aparcado.')).toBeNull()
  })

  // Mutación: cambiar la agrupación (cuatro de dieciséis en vez de ocho de
  // ocho). FR-UI-54 la llama «la ÚNICA agrupación del documento» y nada la
  // sujetaba.
  it('MUT-4 · FR-UI-54: los 64 hex en ocho grupos de ocho, y el aria-label los repite', async () => {
    await openDetail()
    const groups = Array.from({ length: 8 }, (_, i) => HEX.slice(i * 8, i * 8 + 8))
    const hex = document.querySelector('.approvals-digest-hex')
    expect(hex).not.toBeNull()
    expect(hex?.getAttribute('aria-label')).toBe(groups.join(' '))
    expect(
      Array.from(hex?.querySelectorAll('span') ?? []).map((s) => s.textContent?.trim()),
    ).toEqual(groups)
  })
})

// ---------------------------------------------------------------------------
// LO QUE SE CURA, SE LEE. Dos hechos que el servidor ya produce y que la
// pantalla no pintaba: un campo curado que nadie lee sigue sin curar.
// ---------------------------------------------------------------------------
describe('MUT · lo curado llega al operador', () => {
  // El almacén salta la fila que no puede servir entera, la cuenta y la nombra.
  // Si la pantalla no lo pinta, el operador pierde una petición aparcada en
  // silencio — que es exactamente el fallo que saltarla existe para evitar.
  it('MUT-5 · rows_skipped se pinta con su cuenta, y cero no dice nada', async () => {
    await renderList(
      happy({
        'GET /api/approvals': () => json(200, { gate: { ...GATE, rows_skipped: 2 }, rows: [ROW] }),
      }),
    )
    expect(
      await screen.findByText(/2 peticiones aparcadas no se han podido leer/i),
    ).toBeInTheDocument()
  })

  it('MUT-6 · sin filas saltadas no aparece ningún aviso', async () => {
    await renderList()
    expect(screen.queryByText(/no se han podido leer/i)).toBeNull()
  })

  // Una ejecución que FALLA es un desenlace conocido: el efecto salió, dijo que
  // no, y el registro se cerró con su recibo. Pintarla como ejecutada sería
  // mentir; pintarla como «no se sabe» también.
  it('MUT-7 · outcome failed: se dice que falló, con su recibo, y no «ejecutada»', async () => {
    await openDetail(
      happy({
        'POST /api/approvals/apr_1/approve': () =>
          json(200, {
            outcome: 'failed',
            digest: DIGEST,
            result: 'dial tcp: connection refused',
            receipt_id: 'rcp_f1',
          }),
      }),
    )
    typeKeys(armingInput(), TAIL)
    fireEvent.click(approveBtn()!)
    expect(await screen.findByText(/La acción se ejecutó y falló/i)).toBeInTheDocument()
    // Y NO afirma que el efecto saliera: exec.Run también falla cuando la
    // herramienta rechaza sus argumentos sin tocar nada.
    expect(screen.queryByText(/El efecto salió de esta ventana/)).toBeNull()
    expect(screen.getByText(/rcp_f1/)).toBeInTheDocument()
    expect(screen.getByText(/dial tcp: connection refused/)).toBeInTheDocument()
    expect(screen.queryByText('No sabemos si el efecto llegó a ocurrir.')).toBeNull()
  })
})
