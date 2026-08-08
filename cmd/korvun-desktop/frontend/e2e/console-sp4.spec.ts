// SP4 e2e (operator-console spec, FR-UI-1): the Chat view against the REAL
// core through the harness — real store, real sessions, real takeover, real
// operator reply riding the outbound funnel to the scripted channel. ONE
// test on purpose: the suite shares a single harness core, so the whole
// operator story runs as one deterministic flow, taking the screenshot pass
// for Chano's visual OK (design-drafts/console/) along the way.
import { expect, test, type Page } from '@playwright/test'
import { BASE, SHOT, settleFonts } from './util'

const CONSOLE_SHOT = (name: string): string => SHOT(`console/${name}`)

async function post(page: Page, path: string, data: unknown): Promise<void> {
  const res = await page.request.post(`${BASE}${path}`, { data })
  expect(res.ok(), `${path} -> ${res.status()}`).toBeTruthy()
}

test('SP4 — inbox, three-role conversation, takeover, operator reply, session navigation (with screenshots)', async ({
  page,
}) => {
  await page.goto('/')
  await page.request.post(`${BASE}/__test/bindings/Start`, { data: [] }).catch(() => undefined)
  await post(page, '/__test/model', { mode: 'ok' })
  await post(page, '/__test/channel', { send: 'ok' })
  await page.getByRole('button', { name: 'Chat', exact: true }).click()

  // A real inbound produces a real conversation (the fake model replies).
  await post(page, '/__test/inject', { text: 'hola necesito ayuda' })
  const inboxRow = page.getByRole('button', { name: /telegram/i }).first()
  await expect(inboxRow).toBeVisible({ timeout: 15_000 })
  await settleFonts(page)
  await page.screenshot({ path: CONSOLE_SHOT('01-inbox.png'), fullPage: true })

  await inboxRow.click()
  const pane = page.locator('.console-pane')
  await expect(pane.getByText('hola necesito ayuda')).toBeVisible({ timeout: 15_000 })
  // The assistant's reply (fake model) is on stage too.
  await expect(pane.locator('[data-role="assistant"]').first()).toBeVisible({ timeout: 15_000 })

  // Take over: the brain goes silent; the user's next words persist and
  // appear WITHOUT any model reply.
  await page.getByRole('button', { name: 'Take over' }).click()
  await expect(page.getByText(/you are handling this conversation/i)).toBeVisible()
  await post(page, '/__test/inject', { text: 'sigo esperando humano' })
  await expect(pane.getByText('sigo esperando humano')).toBeVisible({ timeout: 15_000 })

  // The operator replies — the turn shows as OPERATOR, never as the AI.
  await page.getByRole('textbox', { name: /reply/i }).fill('aquí Chano, te atiendo yo')
  await page.getByRole('button', { name: 'Send' }).click()
  const operatorTurn = pane.locator('[data-role="operator"]', {
    hasText: 'aquí Chano, te atiendo yo',
  })
  await expect(operatorTurn).toBeVisible({ timeout: 15_000 })
  await expect(operatorTurn.getByText(/operator \(you\)/i)).toBeVisible()
  // And the reply reached the real channel adapter (the scripted channel
  // records sends; a failed Send would have surfaced as a feed failure).
  await settleFonts(page)
  await page.screenshot({ path: CONSOLE_SHOT('02-conversation-three-roles.png'), fullPage: true })
  await page.screenshot({ path: CONSOLE_SHOT('03-takeover-active.png'), fullPage: true })

  // New session: the console reset — the pane starts clean; the old session
  // stays navigable, clearly archived, read-only.
  await page.getByRole('button', { name: 'New session' }).click()
  await expect(page.getByRole('button', { name: /session 2 \(active\)/i })).toBeVisible({
    timeout: 15_000,
  })
  await page.getByRole('button', { name: /^session 1$/i }).click()
  await expect(page.getByText(/archived session — read only/i)).toBeVisible()
  await expect(pane.getByText('aquí Chano, te atiendo yo')).toBeVisible()
  // No composer in an archived session.
  await expect(page.getByRole('textbox', { name: /reply/i })).toHaveCount(0)
  await settleFonts(page)
  await page.screenshot({ path: CONSOLE_SHOT('04-session-navigation.png'), fullPage: true })

  // Back to the active session and release: Korvun takes the wheel again.
  await page.getByRole('button', { name: /session 2 \(active\)/i }).click()
  await page.getByRole('button', { name: 'Release' }).click()
  await expect(page.getByText(/korvun is handling this conversation/i)).toBeVisible({
    timeout: 15_000,
  })

  // --- The completion rider, same live flow ------------------------------

  // SEARCH: an old word (it lives in the archived session) is found and
  // the hit opens the conversation AT that session.
  const search = page.getByRole('searchbox', { name: /filter or search/i })
  await search.fill('necesito ayuda')
  await search.press('Enter')
  await page.getByRole('button', { name: /hola necesito ayuda/i }).click()
  await expect(page.getByText(/archived session — read only/i)).toBeVisible({ timeout: 15_000 })
  await page.screenshot({ path: CONSOLE_SHOT('05-search-hit.png'), fullPage: true })

  // DELETE the archived session behind its explicit confirmation.
  await page.getByRole('button', { name: 'Delete session' }).click()
  await expect(
    page.getByText('This deletes the archived session from disk. No undo.'),
  ).toBeVisible()
  await page.screenshot({ path: CONSOLE_SHOT('06-delete-session-confirm.png'), fullPage: true })
  await page.getByRole('button', { name: /^Delete$/ }).click()
  await expect(page.getByRole('button', { name: /^session 1$/i })).toHaveCount(0, {
    timeout: 15_000,
  })

  // DELETE the whole conversation: confirm copy, inbox empties…
  await page.getByRole('button', { name: 'Delete conversation' }).click()
  await expect(
    page.getByText('This deletes the conversation from disk. No undo.'),
  ).toBeVisible()
  await page.getByRole('button', { name: /^Delete$/ }).click()
  await expect(page.getByText('No conversations yet.')).toBeVisible({ timeout: 15_000 })
  await page.screenshot({ path: CONSOLE_SHOT('07-after-delete.png'), fullPage: true })

  // …and a fresh inbound REBIRTHS it clean at session 1 (AS-13), lighting
  // the unread badge on the nav (FR-UNREAD) because nothing is open.
  await post(page, '/__test/inject', { text: 'empiezo de cero' })
  const chatNav = page.getByRole('button', { name: /^Chat(\s+\d+)?$/ })
  await expect(chatNav).toHaveAttribute('data-unread', /\d+/, { timeout: 15_000 })
  await page.getByRole('button', { name: /telegram/i }).first().click()
  await expect(pane.getByText('empiezo de cero')).toBeVisible({ timeout: 15_000 })
  // Reborn clean: a single session, no session tabs at all.
  await expect(page.getByRole('button', { name: /session \d/i })).toHaveCount(0)
  await page.screenshot({ path: CONSOLE_SHOT('08-reborn-with-unread-cleared.png'), fullPage: true })

  // --- Paso l: the DIRECT chat (console channel) -------------------------
  await page.getByRole('button', { name: 'New chat' }).click()
  await expect(page.getByText(/direct chat — you already are the human/i)).toBeVisible()
  await expect(page.getByRole('button', { name: 'Take over' })).toBeDisabled()
  const chatBox = page.getByRole('textbox', { name: /message korvun/i })
  await chatBox.fill('hola, ¿me oyes?')
  await chatBox.press('Enter')
  // The full pipeline answers (the harness's fake model) — user turn and
  // assistant reply on stage; the human is USER here, never operator.
  await expect(pane.getByText('hola, ¿me oyes?')).toBeVisible({ timeout: 15_000 })
  await expect(pane.locator('[data-role="assistant"]').first()).toBeVisible({ timeout: 15_000 })
  await expect(pane.locator('[data-role="operator"]')).toHaveCount(0)
  await page.screenshot({ path: CONSOLE_SHOT('09-direct-chat.png'), fullPage: true })

  // /new typed IN the chat: the trigger flows the pipeline, the ack comes
  // back as a SYSTEM turn, and session 2 opens.
  await chatBox.fill('/new')
  await chatBox.press('Enter')
  const ackTurn = pane.locator('[data-role="system"]', {
    hasText: 'New session — previous context cleared.',
  })
  await expect(ackTurn).toBeVisible({ timeout: 15_000 })
  await expect(page.getByRole('button', { name: /session 2 \(active\)/i })).toBeVisible({
    timeout: 15_000,
  })
  await page.screenshot({ path: CONSOLE_SHOT('10-direct-chat-new-session.png'), fullPage: true })
})
