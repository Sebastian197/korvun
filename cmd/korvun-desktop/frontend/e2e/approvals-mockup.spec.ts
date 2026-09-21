// The approvals screen brought to its approved mockup — the browser moulds of
// the train of 2026-09-13.
//
// Pre-test paper: docs/superpowers/specs/2026-09-13-approvals-screen-to-mockup-pretest.md
// (G1 visibility, G2 asymmetry, G3/G4 the six cells, G5 autofocus, G6 IME and
// Esc, G7 the bar's expiry, G9 one scroller). Each test names the guarantee and
// the attack it forces.
//
// Evidence level: real Chromium via Playwright against the approvals harness —
// its own core, its own store, requests parked by the real factory through
// POST /__test/park. The CDP moulds drive Chromium's input emulation through a
// Playwright CDP session; they are not a system IME. None of this is WKWebView,
// WebView2 or WebKitGTK.
import { expect, test } from '@playwright/test'
import type { Locator, Page } from '@playwright/test'
import { installBindings } from './bindings'
import { APPROVALS_BASE } from './util'

interface Parked {
  approval_id: string
  action_id: string
  digest: string
}

const LABEL = /reteclea los seis últimos caracteres del digest/i
const MIN_EDGE = 1.15

async function boot(page: Page, theme: 'dark' | 'light' = 'dark'): Promise<void> {
  await page.addInitScript((t) => {
    try {
      localStorage.setItem('korvun.chrome.theme', t)
    } catch {
      // storage refused: the theme assertion below will say so
    }
  }, theme)
  await installBindings(page)
  await page.goto(APPROVALS_BASE + '/')
  await page.request
    .post(APPROVALS_BASE + '/__test/bindings/Start', { data: [] })
    .catch(() => undefined)
  await expect.poll(() => page.evaluate(() => document.documentElement.dataset.theme)).toBe(theme)
}

async function park(page: Page, ttlSeconds?: number, params?: string): Promise<Parked> {
  const data: Record<string, unknown> = {}
  if (ttlSeconds !== undefined) data.ttl_seconds = ttlSeconds
  if (params !== undefined) data.params = params
  const res = await page.request.post(APPROVALS_BASE + '/__test/park', { data })
  expect(res.ok(), `park failed: ${res.status()} ${await res.text()}`).toBe(true)
  return (await res.json()) as Parked
}

async function openParked(page: Page, p: Parked): Promise<void> {
  await page.getByRole('button', { name: 'Aprobaciones' }).click()
  await page.getByRole('button', { name: new RegExp(p.approval_id) }).click()
  await expect(page.getByTestId('approval-parameters')).toBeVisible()
}

async function bootAndOpen(
  page: Page,
  opts: { theme?: 'dark' | 'light'; ttl?: number } = {},
): Promise<Parked> {
  await boot(page, opts.theme ?? 'dark')
  const p = await park(page, opts.ttl)
  await openParked(page, p)
  return p
}

const approve = (page: Page): Locator => page.getByRole('button', { name: 'Aprobar y ejecutar' })
const reject = (page: Page): Locator => page.getByRole('button', { name: 'Rechazar' })
const reason = (page: Page): Locator => page.getByRole('textbox', { name: /motivo/i })
const arming = (page: Page): Locator => page.getByLabel(LABEL)
const cells = (page: Page): Locator => page.getByTestId('arming-cell')
const status = async (page: Page): Promise<string> =>
  (await page.locator('#approvals-arming-status').textContent()) ?? ''

async function typeTail(page: Page, p: Parked): Promise<void> {
  await arming(page).focus()
  for (const ch of p.digest.slice(-6)) await page.keyboard.press(ch)
  await expect(approve(page)).toBeEnabled()
}

async function settle(loc: Locator): Promise<void> {
  await loc.evaluate(async (el) => {
    await Promise.all(el.getAnimations().map((a) => a.finished.catch(() => undefined)))
  })
}

/** G1's oracle: the stronger of the border and the background against the
 *  effective backdrop, composited through translucent layers. */
async function edge(loc: Locator): Promise<{ ratio: number; detail: string; alpha: number }> {
  await settle(loc)
  return loc.evaluate((el) => {
    type C = [number, number, number, number]
    const parse = (c: string): C => {
      const m = /rgba?\(([^)]+)\)/.exec(c)
      if (m === null) return [0, 0, 0, 0]
      const p = m[1]
        .split(/[\s,/]+/)
        .filter((s) => s !== '')
        .map(Number)
      return [p[0], p[1], p[2], p.length > 3 ? p[3] : 1]
    }
    const over = (top: C, bottom: C): C => {
      const a = top[3]
      return [
        top[0] * a + bottom[0] * (1 - a),
        top[1] * a + bottom[1] * (1 - a),
        top[2] * a + bottom[2] * (1 - a),
        1,
      ]
    }
    const backdrop = (node: Element | null): C => {
      const layers: C[] = []
      for (let n = node; n !== null; n = n.parentElement) {
        const c = parse(getComputedStyle(n).backgroundColor)
        if (c[3] > 0) layers.push(c)
        if (c[3] >= 1) break
      }
      let acc: C = [255, 255, 255, 1]
      if (layers.length > 0 && layers[layers.length - 1][3] >= 1) acc = layers.pop() as C
      while (layers.length > 0) acc = over(layers.pop() as C, acc)
      return acc
    }
    const lum = (c: C): number => {
      const f = (v: number): number => {
        const x = v / 255
        return x <= 0.03928 ? x / 12.92 : Math.pow((x + 0.055) / 1.055, 2.4)
      }
      return 0.2126 * f(c[0]) + 0.7152 * f(c[1]) + 0.0722 * f(c[2])
    }
    const cr = (a: C, b: C): number => {
      const [hi, lo] = [lum(a), lum(b)].sort((x, y) => y - x)
      return (hi + 0.05) / (lo + 0.05)
    }
    const cs = getComputedStyle(el)
    const behind = backdrop(el.parentElement)
    const own = parse(cs.backgroundColor)
    const surface = own[3] > 0 ? over(own, behind) : behind
    let ratio = cr(surface, behind)
    if (parseFloat(cs.borderTopWidth) >= 1 && cs.borderTopStyle !== 'none') {
      ratio = Math.max(ratio, cr(over(parse(cs.borderTopColor), surface), behind))
    }
    const rgb = behind
      .slice(0, 3)
      .map((v) => Math.round(v))
      .join(', ')
    return {
      ratio,
      alpha: own[3],
      detail: `border ${cs.borderTopWidth} ${cs.borderTopStyle} ${cs.borderTopColor}, background ${cs.backgroundColor} over rgb(${rgb})`,
    }
  })
}

/** G1's clipping clause: centred into view, inside .main and below the bar. */
async function inView(loc: Locator): Promise<{ ok: boolean; detail: string }> {
  await loc.evaluate((el) => el.scrollIntoView({ block: 'center' }))
  return loc.evaluate((el) => {
    const main = document.querySelector('.main')
    if (main === null) return { ok: false, detail: 'no .main' }
    const m = main.getBoundingClientRect()
    const top = m.top + main.clientTop
    const left = m.left + main.clientLeft
    const bar = document.querySelector('.approvals-bar')
    const barBottom = bar === null ? top : bar.getBoundingClientRect().bottom
    const r = el.getBoundingClientRect()
    // Pass 5: an element taller than the visible band below the bar can never
    // fit; for it only the horizontal containment is judged, declared.
    const tall = r.height > top + main.clientHeight - Math.max(top, barBottom)
    const ok =
      (tall ||
        (r.top >= Math.max(top, barBottom) - 0.5 && r.bottom <= top + main.clientHeight + 0.5)) &&
      r.left >= left - 0.5 &&
      r.right <= left + main.clientWidth + 0.5
    return {
      ok,
      detail: `rect=[${r.left},${r.top},${r.right},${r.bottom}] main=[${left},${top},${left + main.clientWidth},${top + main.clientHeight}] barBottom=${barBottom}`,
    }
  })
}

/** G2's painted extent plus the structural rule's violations. */
async function extent(loc: Locator): Promise<{ area: number; structural: string }> {
  await settle(loc)
  return loc.evaluate((el) => {
    const cs = getComputedStyle(el)
    const r = el.getBoundingClientRect()
    let grow = 0
    if (cs.outlineStyle !== 'none') {
      grow = Math.max(grow, parseFloat(cs.outlineWidth) + parseFloat(cs.outlineOffset))
    }
    if (cs.boxShadow !== '' && cs.boxShadow !== 'none') {
      for (const part of cs.boxShadow.split(/,(?![^(]*\))/)) {
        if (/\binset\b/.test(part)) continue
        const nums = part
          .replace(/rgba?\([^)]*\)/g, '')
          .trim()
          .split(/\s+/)
          .map(parseFloat)
          .filter((n) => !Number.isNaN(n))
        const [ox = 0, oy = 0, blur = 0, spread = 0] = nums
        grow = Math.max(grow, Math.max(Math.abs(ox), Math.abs(oy)) + blur + spread)
      }
    }
    grow = Math.max(0, grow)
    const w = r.width + 2 * grow
    const h = r.height + 2 * grow
    const pseudo = (which: '::before' | '::after'): string => {
      const c = getComputedStyle(el, which).content
      return c !== 'none' && c !== 'normal' ? `${which} content ${c}` : ''
    }
    const prop = (name: string, neutral: string): string => {
      const v = cs.getPropertyValue(name)
      return v !== '' && v !== neutral ? `${name} ${v}` : ''
    }
    const structural = [
      pseudo('::before'),
      pseudo('::after'),
      prop('transform', 'none'),
      prop('scale', 'none'),
      prop('translate', 'none'),
      prop('rotate', 'none'),
      prop('filter', 'none'),
      prop('clip-path', 'none'),
      prop('zoom', '1'),
      prop('border-image-outset', '0'),
      prop('-webkit-box-reflect', 'none'),
    ]
      .filter((s) => s !== '')
      .join('; ')
    return { area: w * h, structural }
  })
}

const scrollMainTo = (page: Page, where: 'top' | 'bottom'): Promise<number> =>
  page.evaluate((w) => {
    const main = document.querySelector('.main') as HTMLElement
    main.scrollTop = w === 'top' ? 0 : main.scrollHeight
    return main.scrollTop
  }, where)

const activeId = (page: Page): Promise<string> =>
  page.evaluate(() => (document.activeElement as HTMLElement | null)?.id ?? '')

// ---------------------------------------------------------------------------
// G1 · the listed controls paint a visible edge, in dark and light
// ---------------------------------------------------------------------------
for (const theme of ['dark', 'light'] as const) {
  test(`G1 · the listed controls paint an edge ≥ ${MIN_EDGE}:1, unclipped, in ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1100, height: 760 })
    const p = await bootAndOpen(page, { theme })
    const failures: string[] = []
    const judge = async (name: string, loc: Locator): Promise<void> => {
      const e = await edge(loc)
      if (e.ratio < MIN_EDGE)
        failures.push(`${name}[${theme}]: edge ${e.ratio.toFixed(2)}:1 < ${MIN_EDGE} (${e.detail})`)
      const v = await inView(loc)
      if (!v.ok) failures.push(`${name}[${theme}]: clipped or under the bar (${v.detail})`)
    }
    const barEdge = await edge(page.locator('.approvals-bar'))
    if (barEdge.alpha < 1) failures.push(`bar[${theme}]: background alpha ${barEdge.alpha} < 1`)
    await judge('Volver a leer', page.getByRole('button', { name: 'Volver a leer la petición' }))
    const sections = page.locator('article section')
    for (let i = 0; i < (await sections.count()); i++) await judge(`card ${i}`, sections.nth(i))
    // Counted, not asserted: an absent gate is one failure among the others, so
    // the doors and the reason field are still judged and printed.
    const cellCount = await cells(page).count()
    if (cellCount !== 6) failures.push(`cells[${theme}]: ${cellCount} cells, expected 6`)
    for (let i = 0; i < cellCount; i++) await judge(`cell ${i}`, cells(page).nth(i))
    await judge('motivo', reason(page))
    await judge('Rechazar', reject(page))
    await judge('Aprobar (disabled)', approve(page))
    await typeTail(page, p)
    await judge('Aprobar (enabled)', approve(page))
    await approve(page).hover()
    await judge('Aprobar (hover)', approve(page))
    expect(failures, failures.join('\n')).toEqual([])
  })
}

// ---------------------------------------------------------------------------
// G2 · the asymmetry at 1100×760 and 900×700, in four focus/hover states
// ---------------------------------------------------------------------------
for (const size of [
  { width: 1100, height: 760 },
  { width: 900, height: 700 },
]) {
  test(`G2 · extent(Aprobar) ≤ 0.50 × extent(Rechazar) in four states, at ${size.width}x${size.height}`, async ({
    page,
  }) => {
    await page.setViewportSize(size)
    const p = await bootAndOpen(page)
    await typeTail(page, p)
    const failures: string[] = []
    const measure = async (state: string): Promise<void> => {
      const a = await extent(approve(page))
      const r = await extent(reject(page))
      if (a.structural !== '') failures.push(`Aprobar[${state}]: ${a.structural}`)
      if (r.structural !== '') failures.push(`Rechazar[${state}]: ${r.structural}`)
      if (a.area > 0.5 * r.area)
        failures.push(
          `Aprobar[${state}]: extent ${a.area.toFixed(0)} > 0.50 × ${r.area.toFixed(0)}`,
        )
    }
    await page.mouse.move(0, 0)
    await measure('none')
    // New ladder (FR-UI-57, director's decision 3): arming → reason → Rechazar →
    // Aprobar. A wrong ladder is recorded, not thrown, so the ring is measured.
    const tabTo = async (target: Locator, name: string, expectedSteps: number): Promise<void> => {
      for (let steps = 1; steps <= 8; steps++) {
        await page.keyboard.press('Tab')
        if (await target.evaluate((el) => el === document.activeElement)) {
          if (steps !== expectedSteps)
            failures.push(`ladder: ${name} reached in ${steps} Tab(s), expected ${expectedSteps}`)
          return
        }
      }
      failures.push(`ladder: ${name} not reached in 8 Tabs`)
    }
    await arming(page).focus()
    await tabTo(reject(page), 'Rechazar', 2)
    await measure('Rechazar focused')
    await tabTo(approve(page), 'Aprobar', 1)
    await measure('Aprobar focused')
    await reason(page).focus()
    await approve(page).hover()
    await measure('Aprobar hovered')
    if (size.width < 1100) {
      const yes = (await approve(page).boundingBox())!
      const no = (await reject(page).boundingBox())!
      const gap = yes.y - (no.y + no.height)
      if (gap < 120) failures.push(`stack: vertical gap ${gap.toFixed(1)} < 120`)
      if (Math.abs(yes.width - 0.5 * no.width) > 1)
        failures.push(`stack: Aprobar width ${yes.width} is not 50 % of ${no.width}`)
    }
    expect(failures, failures.join('\n')).toEqual([])
  })
}

// ---------------------------------------------------------------------------
// G3 · six cells in a full-width row above the reason field
// ---------------------------------------------------------------------------
test('G3 · six 32×42 cells above the reason field, spanning the doors, prefix bound, focus shown on the cells', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1100, height: 760 })
  const p = await bootAndOpen(page)
  await expect(cells(page)).toHaveCount(6)
  for (let i = 0; i < 6; i++) {
    const b = (await cells(page).nth(i).boundingBox())!
    expect(Math.abs(b.width - 32), `cell ${i} width ${b.width}`).toBeLessThanOrEqual(0.5)
    expect(Math.abs(b.height - 42), `cell ${i} height ${b.height}`).toBeLessThanOrEqual(0.5)
  }
  const row = (await page.getByTestId('arming-row').boundingBox())!
  const why = (await reason(page).boundingBox())!
  const no = (await reject(page).boundingBox())!
  const yes = (await approve(page).boundingBox())!
  expect(row.y + row.height, 'arming row above the reason field').toBeLessThanOrEqual(why.y)
  expect(row.x, 'row starts at the doors row').toBeLessThanOrEqual(no.x + 1)
  expect(row.x + row.width, 'row reaches the far door').toBeGreaterThanOrEqual(
    yes.x + yes.width - 1,
  )
  const prefix = (await page.getByTestId('arming-prefix').textContent()) ?? ''
  expect(prefix.replace(/[…\s]/g, '')).toBe(p.digest.slice(7).slice(48, 58))
  const edgeColor = (): Promise<string> =>
    cells(page)
      .first()
      .evaluate((el) => {
        const cs = getComputedStyle(el)
        return cs.outlineStyle !== 'none' ? cs.outlineColor : cs.borderTopColor
      })
  const unfocused = await edgeColor()
  await arming(page).focus()
  await settle(cells(page).first())
  const focused = await edgeColor()
  const ratio = await page.evaluate(
    ([a, b]) => {
      const lum = (c: string): number => {
        const m = /rgba?\(([^)]+)\)/.exec(c)
        const p = m === null ? [0, 0, 0] : m[1].split(/[\s,/]+/).map(Number)
        const f = (v: number): number => {
          const x = v / 255
          return x <= 0.03928 ? x / 12.92 : Math.pow((x + 0.055) / 1.055, 2.4)
        }
        return 0.2126 * f(p[0]) + 0.7152 * f(p[1]) + 0.0722 * f(p[2])
      }
      const [hi, lo] = [lum(a), lum(b)].sort((x, y) => y - x)
      return (hi + 0.05) / (lo + 0.05)
    },
    [focused, unfocused] as const,
  )
  expect(ratio, `focus indicator on the cells: ${unfocused} → ${focused}`).toBeGreaterThanOrEqual(
    MIN_EDGE,
  )
})

// ---------------------------------------------------------------------------
// G4 · no real character before it is typed (positional)
// ---------------------------------------------------------------------------
test('G4 · cells, value, placeholder, pseudo-content and attributes carry no untyped character', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1100, height: 760 })
  await boot(page)
  const first = await park(page)
  // Pass 5: two parks with the default body share one digest, so the identity
  // clause compared a digest with itself. The second park carries other params.
  const second = await park(page, undefined, '{"url":"https://hooks.acme.io/otro","body":"2"}')
  expect(second.digest, 'precondition: two different digests').not.toBe(first.digest)
  const ALLOWED = new Set([
    'id',
    'class',
    'type',
    'maxlength',
    'autocomplete',
    'inputmode',
    'spellcheck',
    'value',
    'role',
    'for',
    'data-testid',
  ])
  const signature = (): Promise<string[]> =>
    page.getByTestId('arming-row').evaluate((row) => {
      const out: string[] = []
      for (const el of [row, ...Array.from(row.querySelectorAll('*'))]) {
        const attrs = Array.from(el.attributes)
          .filter((a) => a.name !== 'value')
          .map((a) => `${a.name}=${a.value}`)
          .sort()
        out.push(`${el.tagName}[${attrs.join(' ')}]`)
      }
      return out
    })
  const attributeNames = (): Promise<string[]> =>
    page
      .getByTestId('arming-row')
      .evaluate((row) =>
        [row, ...Array.from(row.querySelectorAll('*'))].flatMap((el) =>
          Array.from(el.attributes).map((a) => a.name),
        ),
      )

  await openParked(page, first)
  await expect(page.getByTestId('arming-row'), 'the arming row').toHaveCount(1)
  const tail = first.digest.slice(-6)
  const sigA = await signature()
  const names = await attributeNames()
  const stray = names.filter((n) => !ALLOWED.has(n) && !n.startsWith('aria-'))
  expect(stray, 'attributes outside the allow-list').toEqual([])
  await expect(cells(page)).toHaveText(['–', '–', '–', '–', '–', '–'])
  await expect(arming(page)).toHaveValue('')
  expect(await arming(page).getAttribute('placeholder')).toBeFalsy()
  const pseudo = await cells(page).evaluateAll((els) =>
    els.flatMap((el) =>
      (['::before', '::after'] as const)
        .map((w) => getComputedStyle(el, w).content)
        .filter((c) => c !== 'none' && c !== 'normal'),
    ),
  )
  expect(pseudo, 'cell pseudo-content').toEqual([])
  await arming(page).focus()
  await page.keyboard.press(tail[0])
  await page.keyboard.press(tail[1])
  await expect(cells(page)).toHaveText([tail[0], tail[1], '–', '–', '–', '–'])
  await expect(arming(page)).toHaveValue(tail.slice(0, 2))
  expect(await status(page)).toBe('faltan 4')

  await page.getByRole('button', { name: '← Pendientes' }).click()
  await page.getByRole('button', { name: new RegExp(second.approval_id) }).click()
  await expect(page.getByTestId('approval-parameters')).toBeVisible()
  expect(await signature(), 'attributes must not depend on the digest').toEqual(sigA)
})

// ---------------------------------------------------------------------------
// G5 · the autofocus against a held key, a second load and a focused field
// ---------------------------------------------------------------------------
test('G5a/d/e · one programmatic focus per load, no scroll, and a held hex key does not arm', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1100, height: 760 })
  await bootAndOpen(page)
  const inputId = await arming(page).evaluate((el) => el.id)
  await page.evaluate((id) => {
    const w = window as unknown as { __armFocus: number }
    w.__armFocus = 0
    document.getElementById(id)?.addEventListener('focus', () => (w.__armFocus += 1))
    ;(document.activeElement as HTMLElement | null)?.blur()
  }, inputId)
  expect(await activeId(page), 'no autofocus before the end is reached').not.toBe(inputId)
  // A6 — a hex key held on the body before the focus lands.
  await page.keyboard.down('e')
  const st = await scrollMainTo(page, 'bottom')
  await expect.poll(() => activeId(page), { timeout: 3_000 }).toBe(inputId)
  expect(
    await page.evaluate(() => (document.querySelector('.main') as HTMLElement).scrollTop),
  ).toBe(st)
  await page.keyboard.down('e')
  await page.keyboard.down('e')
  await page.keyboard.up('e')
  expect(await status(page)).toBe('faltan 6')
  await expect(cells(page)).toHaveText(['–', '–', '–', '–', '–', '–'])
  // A11 — away and back again does not refocus. The row must really LEAVE the
  // view between the two scrolls: two scrolls inside one frame did not let the
  // observer see it go, and the once-flag mutation stayed green that way.
  await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur())
  await scrollMainTo(page, 'top')
  await expect
    .poll(() =>
      page.getByTestId('arming-row').evaluate((row) => {
        const main = document.querySelector('.main') as HTMLElement
        const m = main.getBoundingClientRect()
        const r = row.getBoundingClientRect()
        return r.top >= m.top + main.clientHeight
      }),
    )
    .toBe(true)
  await page.waitForTimeout(400)
  await scrollMainTo(page, 'bottom')
  await page.waitForTimeout(600)
  expect(await page.evaluate(() => (window as unknown as { __armFocus: number }).__armFocus)).toBe(
    1,
  )
  // A12 — a re-read is a new load: one more autofocus, and the arming is empty.
  await page.evaluate(() => {
    const b = Array.from(document.querySelectorAll('button')).find(
      (x) => x.textContent === 'Volver a leer la petición',
    )
    b?.click()
  })
  await expect(page.getByTestId('approval-parameters')).toBeVisible()
  await scrollMainTo(page, 'bottom')
  await expect.poll(() => activeId(page), { timeout: 3_000 }).toBe(inputId)
  await expect(arming(page)).toHaveValue('')
})

test('G5c · the autofocus does not take the focus from the reason field or Rechazar', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1100, height: 760 })
  await bootAndOpen(page)
  await reason(page).evaluate((el) => (el as HTMLInputElement).focus({ preventScroll: true }))
  await page.keyboard.type('motivo x')
  await scrollMainTo(page, 'bottom')
  await page.waitForTimeout(400)
  await expect(reason(page)).toBeFocused()
  await expect(reason(page)).toHaveValue('motivo x')
  await scrollMainTo(page, 'top')
  await reject(page).evaluate((el) => (el as HTMLButtonElement).focus({ preventScroll: true }))
  await scrollMainTo(page, 'bottom')
  await page.waitForTimeout(400)
  await expect(reject(page)).toBeFocused()
})

// ---------------------------------------------------------------------------
// G5e / G6 · IME and keyCode 229, through Chromium's CDP input emulation
// ---------------------------------------------------------------------------
test('G5e · a CDP composition, insertText and a 229 keydown do not arm', async ({ page }) => {
  await bootAndOpen(page)
  const cdp = await page.context().newCDPSession(page)
  await arming(page).focus()
  await cdp.send('Input.imeSetComposition', { text: 'e', selectionStart: 1, selectionEnd: 1 })
  await cdp.send('Input.insertText', { text: 'e' })
  expect(await status(page), 'composition + insertText').toBe('faltan 6')
  await cdp.send('Input.dispatchKeyEvent', {
    type: 'keyDown',
    key: 'e',
    code: 'KeyE',
    text: 'e',
    windowsVirtualKeyCode: 229,
    nativeVirtualKeyCode: 229,
  })
  await cdp.send('Input.dispatchKeyEvent', {
    type: 'keyUp',
    key: 'e',
    code: 'KeyE',
    windowsVirtualKeyCode: 229,
  })
  expect(await status(page), 'keydown with keyCode 229').toBe('faltan 6')
})

test('G6 · an Esc that belongs to an IME composition in the reason field does not reject', async ({
  page,
}) => {
  const p = await bootAndOpen(page)
  const rejects: string[] = []
  page.on('request', (r) => {
    if (r.method() === 'POST' && r.url().endsWith(`/api/approvals/${p.approval_id}/reject`))
      rejects.push(r.url())
  })
  await reason(page).evaluate((el) => {
    const w = window as unknown as { __ime: string[] }
    w.__ime = []
    for (const t of ['compositionstart', 'compositionend'])
      el.addEventListener(t, () => w.__ime.push(t))
  })
  const ime = (): Promise<string[]> =>
    page.evaluate(() => (window as unknown as { __ime: string[] }).__ime)
  const cdp = await page.context().newCDPSession(page)
  await reason(page).focus()
  // A8b — Escape reported with keyCode 229.
  await cdp.send('Input.dispatchKeyEvent', {
    type: 'keyDown',
    key: 'Escape',
    code: 'Escape',
    windowsVirtualKeyCode: 229,
    nativeVirtualKeyCode: 229,
  })
  await cdp.send('Input.dispatchKeyEvent', { type: 'keyUp', key: 'Escape', code: 'Escape' })
  await page.waitForTimeout(300)
  expect(rejects, 'Esc with keyCode 229').toEqual([])
  // A8 — Esc while a composition is open.
  await cdp.send('Input.imeSetComposition', { text: 'no', selectionStart: 2, selectionEnd: 2 })
  await expect
    .poll(ime, { message: 'precondition: compositionstart' })
    .toContain('compositionstart')
  await page.keyboard.press('Escape')
  await page.waitForTimeout(300)
  expect(rejects, 'Esc during a composition').toEqual([])
  await cdp.send('Input.insertText', { text: 'no' })
  await expect.poll(ime, { message: 'precondition: compositionend' }).toContain('compositionend')
  await page.keyboard.press('Escape')
  await expect.poll(() => rejects.length, { message: 'a plain Esc still rejects' }).toBe(1)
})

// ---------------------------------------------------------------------------
// G7 (browser half) · the bar's expiry and the list row
// ---------------------------------------------------------------------------
test('G7 · the bar says «caduca en 1h 29m · HH:MM:SSZ» in one element; the row says «caduca · HH:MM:SSZ»', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1100, height: 760 })
  await bootAndOpen(page, { ttl: 5_400 })
  const ownText = (root: Locator, re: RegExp): Promise<string[]> =>
    root.evaluate((el, source) => {
      const rx = new RegExp(source)
      return [el, ...Array.from(el.querySelectorAll('*'))]
        .map((n) =>
          Array.from(n.childNodes)
            .filter((c) => c.nodeType === Node.TEXT_NODE)
            .map((c) => c.textContent ?? '')
            .join('')
            .trim(),
        )
        .filter((t) => rx.test(t))
    }, re.source)
  expect(
    await ownText(page.locator('.approvals-bar'), /^caduca en 1h (29|30)m · \d{2}:\d{2}:\d{2}Z$/),
  ).toHaveLength(1)
  await page.getByRole('button', { name: '← Pendientes' }).click()
  const row = page.locator('.approvals-row').first()
  expect(await ownText(row, /^caduca · \d{2}:\d{2}:\d{2}Z$/)).toHaveLength(1)
  expect(await ownText(row, /caduca en/)).toHaveLength(0)
})

// ---------------------------------------------------------------------------
// G9 · one scroll container, and the bar stays pinned inside it
// ---------------------------------------------------------------------------
test('G9 · .main is the only scroller and the bar stays inside it from top to bottom', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1100, height: 760 })
  await bootAndOpen(page)
  const scrollers = await page.locator('article').evaluate((el) => {
    const out: string[] = []
    for (let n = el.parentElement; n !== null; n = n.parentElement) {
      const cs = getComputedStyle(n)
      if (/(auto|scroll)/.test(cs.overflowY) && n.scrollHeight > n.clientHeight)
        out.push(n.classList.contains('main') ? 'main' : n.className || n.tagName)
    }
    return out
  })
  expect(scrollers).toEqual(['main'])
  for (const where of ['top', 'bottom'] as const) {
    await scrollMainTo(page, where)
    const inside = await page.evaluate(() => {
      const main = document.querySelector('.main') as HTMLElement
      const m = main.getBoundingClientRect()
      const b = (document.querySelector('.approvals-bar') as HTMLElement).getBoundingClientRect()
      return (
        b.top >= m.top + main.clientTop - 0.5 &&
        b.bottom <= m.top + main.clientTop + main.clientHeight + 0.5
      )
    })
    expect(inside, `bar inside .main at ${where}`).toBe(true)
  }
})

// ---------------------------------------------------------------------------
// G7 (pass 5) · the one-shot withdrawal timer is not armed past setTimeout's range
// ---------------------------------------------------------------------------
// setTimeout fires at once for a delay of 2^31 ms or more, and approvals.ttl has
// no ceiling. Green today (no such timer exists); it guards the cure, and its
// probing mutation is removing the clamp.
test('G7 · no setTimeout is armed with a delay above 2147483647 ms, and a 35-day request keeps Aprobar', async ({
  page,
}) => {
  await page.addInitScript(() => {
    const w = window as unknown as { __delays: number[] }
    w.__delays = []
    const original = window.setTimeout.bind(window)
    ;(window as unknown as { setTimeout: unknown }).setTimeout = (
      fn: TimerHandler,
      delay?: number,
      ...rest: unknown[]
    ): number => {
      w.__delays.push(Number(delay ?? 0))
      return original(fn, delay, ...rest)
    }
  })
  const p = await bootAndOpen(page, { ttl: 3_000_000 })
  await page.waitForTimeout(2_000)
  const over = await page.evaluate(() =>
    (window as unknown as { __delays: number[] }).__delays.filter((d) => d > 2147483647),
  )
  expect(over, 'delays beyond setTimeout range').toEqual([])
  await typeTail(page, p)
  await expect(approve(page)).toBeEnabled()
})

// ---------------------------------------------------------------------------
// G9b · long untrusted tokens wrap: .main does not scroll sideways
// ---------------------------------------------------------------------------
// Found by the adversary's review of the green diff: a 400-character canonical
// body is one unbroken token, it overflowed its card, and .main — the only
// scroller — scrolled sideways and took the sticky bar with it.
test('G9b · long unbroken literals wrap inside their cards and rows; the bar stays in view', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1100, height: 760 })
  await boot(page)
  const res = await page.request.post(APPROVALS_BASE + '/__test/park', {
    data: {
      params: `{"body":"${'a'.repeat(400)}","url":"https://hooks.acme.io/pedidos"}`,
      purpose: 'p'.repeat(300),
      channel: 'c'.repeat(200),
    },
  })
  expect(res.ok(), `park failed: ${res.status()} ${await res.text()}`).toBe(true)
  const p = (await res.json()) as Parked
  const sideways = (): Promise<number> =>
    page.evaluate(() => {
      const m = document.querySelector('.main') as HTMLElement
      return m.scrollWidth - m.clientWidth
    })
  await page.getByRole('button', { name: 'Aprobaciones' }).click()
  await expect(page.getByRole('button', { name: new RegExp(p.approval_id) })).toBeVisible()
  expect(await sideways(), 'list: .main horizontal overflow').toBeLessThanOrEqual(1)
  await page.getByRole('button', { name: new RegExp(p.approval_id) }).click()
  await expect(page.getByTestId('approval-parameters')).toBeVisible()
  expect(await sideways(), 'detail: .main horizontal overflow').toBeLessThanOrEqual(1)
  const barInside = await page.evaluate(() => {
    const m = document.querySelector('.main') as HTMLElement
    m.scrollLeft = 2000
    const r = m.getBoundingClientRect()
    const b = (document.querySelector('.approvals-bar') as HTMLElement).getBoundingClientRect()
    return (
      b.left >= r.left + m.clientLeft - 0.5 &&
      b.right <= r.left + m.clientLeft + m.clientWidth + 0.5
    )
  })
  expect(barInside, 'bar inside .main after a sideways scroll attempt').toBe(true)
})

// ---------------------------------------------------------------------------
// G5b · the trigger is the row FULLY in view
// ---------------------------------------------------------------------------
// Found by the adversary's review of the green diff: an autofocus firing on the
// first visible pixel left every other G5 mould green.
test('G5b · a row only partly in view does not take the focus; fully in view does', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1100, height: 760 })
  await bootAndOpen(page)
  const inputId = await arming(page).evaluate((el) => el.id)
  await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur())
  await page.getByTestId('arming-row').evaluate((row) => {
    const main = document.querySelector('.main') as HTMLElement
    const m = main.getBoundingClientRect()
    const r = row.getBoundingClientRect()
    // All but the last 3 px in view: a trigger below «fully in view» (0.5, 0.9)
    // fires here, and only the real one waits.
    main.scrollTop += r.bottom - 3 - (m.top + main.clientTop + main.clientHeight)
  })
  await page.waitForTimeout(500)
  const partly = await page.getByTestId('arming-row').evaluate((row) => {
    const main = document.querySelector('.main') as HTMLElement
    const bottom = main.getBoundingClientRect().top + main.clientTop + main.clientHeight
    const r = row.getBoundingClientRect()
    return r.top < bottom && r.bottom > bottom && (bottom - r.top) / r.height >= 0.9
  })
  expect(partly, 'precondition: the row is at least 90 % but not fully in view').toBe(true)
  expect(await activeId(page), 'no focus while partly in view').not.toBe(inputId)
  await scrollMainTo(page, 'bottom')
  await expect.poll(() => activeId(page), { timeout: 3_000 }).toBe(inputId)
})

// ---------------------------------------------------------------------------
// G9b (state paragraphs) · a long start error wraps in «El núcleo está parado»
// ---------------------------------------------------------------------------
// Found by the adversary's pass on the cures: the start error is a <p> of the
// state, outside the wrapping rule, and pushed .main 4024 px sideways.
test('G9b · a long start error in the stopped-core state wraps; .main does not scroll sideways', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1100, height: 760 })
  await page.request
    .post(APPROVALS_BASE + '/__test/bindings/Stop', { data: [] })
    .catch(() => undefined)
  await installBindings(page)
  await page.addInitScript(
    (long) => {
      const w = window as unknown as { go: { shell: { Desktop: Record<string, unknown> } } }
      w.go.shell.Desktop.Status = () =>
        Promise.resolve({
          Running: false,
          ConfigPath: '/p/korvun.json',
          AdminAddr: '',
          TokenEnv: 'KORVUN_ADMIN_TOKEN',
        })
      w.go.shell.Desktop.Start = () => Promise.reject(new Error(long))
    },
    '/' + 'x'.repeat(500),
  )
  await page.goto(APPROVALS_BASE + '/')
  await page.getByRole('button', { name: 'Aprobaciones' }).click()
  await page.getByRole('button', { name: 'Arrancar el núcleo' }).click()
  await expect(page.getByText('x'.repeat(40), { exact: false })).toBeVisible()
  const sideways = await page.evaluate(() => {
    const m = document.querySelector('.main') as HTMLElement
    return m.scrollWidth - m.clientWidth
  })
  expect(sideways, 'stopped-core state: .main horizontal overflow').toBeLessThanOrEqual(1)
})

// ---------------------------------------------------------------------------
// FR-UI-68 · what is painted is what is sealed: repeated spaces are not collapsed
// ---------------------------------------------------------------------------
// Found out of the delta by the adversary's pass on the cures, and entered in
// this train by the director: with white-space: normal, «pagar  100 EUR» painted
// as «pagar 100 EUR» while the digest sealed both spaces.
test('FR-UI-68 · two consecutive spaces in the parameters and the purpose are painted as two', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1100, height: 760 })
  await boot(page)
  const res = await page.request.post(APPROVALS_BASE + '/__test/park', {
    data: {
      params: '{"body":"pagar  100 EUR","url":"https://hooks.acme.io/pedidos"}',
      purpose: 'avisar  al webhook',
    },
  })
  expect(res.ok(), `park failed: ${res.status()} ${await res.text()}`).toBe(true)
  const p = (await res.json()) as Parked
  await openParked(page, p)
  for (const id of ['approval-parameters', 'approval-origin']) {
    const seen = await page.getByTestId(id).evaluate((el) => ({
      painted: (el as HTMLElement).innerText,
      text: el.textContent ?? '',
    }))
    expect(seen.text, `${id}: precondition, the stored bytes carry two spaces`).toMatch(/ {2}/)
    expect(seen.painted, `${id}: painted text equals the text`).toBe(seen.text)
  }
})

// ---------------------------------------------------------------------------
// AS-AUTH-UI-04 · signed parked authority, exact bytes, loopback only
// ---------------------------------------------------------------------------
// The scenario posts only what an operator CONFIGURES: the intent's id, its
// purpose and its budget, and how many ordinary starts to spend AFTER the park.
// The harness parks through the production door (ParkAuthorization), so the
// requester, the principal chain and the remainder painted here are the ones the
// STORE established and signed — an earlier shape posted them itself and read
// its own strings back. One start is spent after the park, so the live
// remainder is 1 while the parked snapshot says 2: a screen or a detail showing
// live data as if it were the snapshot fails the «máximo 2 inicios» line.
test('AS-AUTH-UI-04 · real Chromium paints the stored authority snapshot and stays on loopback', async ({
  page,
}) => {
  await page.setViewportSize({ width: 1100, height: 760 })
  const harnessOrigin = new URL(APPROVALS_BASE).origin
  const outside: string[] = []
  page.on('request', (request) => {
    if (new URL(request.url()).origin !== harnessOrigin) outside.push(request.url())
  })
  await boot(page)
  const res = await page.request.post(APPROVALS_BASE + '/__test/park', {
    data: {
      authority: {
        intent_id: 'int_supplier_payments_v3',
        intent_purpose: 'Pay\u202e approved supplier invoices',
        budget: { kind: 'finite', total: 2 },
        spend_after_park: 1,
      },
    },
  })
  expect(res.ok(), `park failed: ${res.status()} ${await res.text()}`).toBe(true)
  const parked = (await res.json()) as Parked
  await openParked(page, parked)

  const authority = page.getByTestId('approval-authority')
  await expect(authority).toBeVisible()
  await expect(authority).toContainText('principal_channel_telegram')
  await expect(authority).toContainText('int_supplier_payments_v3')
  await expect(authority).toContainText('Pay<U+202E> approved supplier invoices')
  await expect(authority).toContainText('principal_brain_asistente → principal_brain_operaciones')
  await expect(authority).toContainText('máximo 2 inicios')
  expect(outside).toEqual([])
})
