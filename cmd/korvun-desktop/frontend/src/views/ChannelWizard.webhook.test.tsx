// v0.9.1 RED (app-audit 2026-08-23, añadido 2): the wizard offered only
// Telegram and Discord, so for a user the release's webhook channel did not
// exist — the Builder path (generic block → node → type dropdown) is
// invisible. The wizard gains a third card whose action is the breadcrumb:
// a two-line instruction plus a CTA that lands on the Builder. The full
// webhook wizard step (bind/path/outbound_url) stays a v0.10 item.
import { render, screen, fireEvent } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { ChannelWizard } from './ChannelWizard'

interface FakeWindow {
  go?: { shell?: { Desktop?: Record<string, unknown> } }
}

beforeEach(() => {
  delete (window as unknown as FakeWindow).go
  localStorage.clear()
})

describe('ChannelWizard — the webhook card (v0.9.1)', () => {
  it('the third card is offered next to Telegram and Discord', () => {
    render(
      <ChannelWizard existingTypes={[]} onClose={() => undefined} onDone={() => undefined} />,
    )
    expect(screen.getByRole('button', { name: /Webhook/ })).toBeTruthy()
  })

  it('picking it shows the Builder instruction instead of advancing the token flow', () => {
    render(
      <ChannelWizard existingTypes={[]} onClose={() => undefined} onDone={() => undefined} />,
    )
    fireEvent.click(screen.getByRole('button', { name: /Webhook/ }))
    expect(screen.getByText(/se configura en el Builder/i)).toBeTruthy()
    const next = screen.getByRole('button', { name: 'Siguiente' }) as HTMLButtonElement
    expect(next.disabled).toBe(true)
  })

  it('the CTA lands on the Builder and closes the wizard', () => {
    let openedBuilder = false
    let closed = false
    render(
      <ChannelWizard
        existingTypes={[]}
        onClose={() => {
          closed = true
        }}
        onDone={() => undefined}
        onOpenBuilder={() => {
          openedBuilder = true
        }}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: /Webhook/ }))
    fireEvent.click(screen.getByRole('button', { name: /Ir al Builder/ }))
    expect(openedBuilder).toBe(true)
    expect(closed).toBe(true)
  })

  it('an already-connected webhook is disabled with the honest note', () => {
    render(
      <ChannelWizard
        existingTypes={['webhook']}
        onClose={() => undefined}
        onDone={() => undefined}
      />,
    )
    const card = screen.getByRole('button', { name: /Webhook/ }) as HTMLButtonElement
    expect(card.disabled).toBe(true)
    expect(card.textContent).toMatch(/ya está conectado/)
  })
})
