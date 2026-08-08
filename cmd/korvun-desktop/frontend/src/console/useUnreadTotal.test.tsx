// FR-UNREAD: the tab total updates from the inbox and honors last-read.
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, it } from 'vitest'
import { markRead } from './unread'
import { useUnreadTotal } from './useUnreadTotal'

function Probe({ fetcher }: { fetcher: typeof fetch }): React.JSX.Element {
  const total = useUnreadTotal(fetcher, 60_000)
  return <output aria-label="unread total">{total}</output>
}

const ROWS = [
  { key: 'tg::a', active_session: 1, session_count: 1, turn_count: 3, taken_over: false },
  { key: 'dc::b', active_session: 1, session_count: 1, turn_count: 2, taken_over: false },
]

const fetcher = (async () =>
  new Response(JSON.stringify(ROWS), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })) as typeof fetch

beforeEach(() => localStorage.clear())

describe('useUnreadTotal', () => {
  it('totals unseen turns across conversations', async () => {
    render(<Probe fetcher={fetcher} />)
    await screen.findByText('5')
  })
  it('honors the last-read marks', async () => {
    markRead('tg::a', 3)
    render(<Probe fetcher={fetcher} />)
    await screen.findByText('2')
  })
})
