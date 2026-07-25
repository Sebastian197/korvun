// The Builder view (SP6c, AS-5): the EXISTING web/builder bundle in a
// same-origin iframe at /builder/ (possible after the commit-0 CSP change),
// the chrome around it. With the core stopped the tab paints the honest
// stopped state, never a broken iframe.
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { pollOnce } from '../status/store'
import { BuilderEmbed } from './BuilderEmbed'

async function seedCore(running: boolean): Promise<void> {
  await pollOnce((() =>
    Promise.resolve(
      running
        ? new Response('ok')
        : new Response(JSON.stringify({ error: 'core stopped' }), { status: 503 }),
    )) as typeof fetch)
}

describe('BuilderEmbed', () => {
  it('running: renders the same-origin iframe at /builder/', async () => {
    await seedCore(true)
    render(<BuilderEmbed />)
    const frame = screen.getByTitle('Builder')
    expect(frame.tagName).toBe('IFRAME')
    expect(frame.getAttribute('src')).toBe('/builder/')
  })

  it('stopped: paints the honest stopped state, NO iframe', async () => {
    await seedCore(false)
    render(<BuilderEmbed />)
    expect(screen.queryByTitle('Builder')).toBeNull()
    expect(screen.getByText(/Arranca el gateway/)).toBeInTheDocument()
  })
})
