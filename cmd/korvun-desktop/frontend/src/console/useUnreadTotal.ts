// The nav-badge hook (FR-UNREAD: the tab total, alive even while the Chat
// view is closed): re-fetches the inbox on the feed's change signal plus a
// slow catch-up poll, and diffs against the shell's last-read state. Mounted
// at App level so the badge lights up with the chat CLOSED.
import { useEffect, useState } from 'react'
import { listConversations } from './api'
import { readLastSeen, unreadTotal } from './unread'
import { useFeed } from '../feed/store'

const NAV_POLL_MS = 15_000

export function useUnreadTotal(fetcher: typeof fetch = fetch, pollMs: number = NAV_POLL_MS): number {
  const feed = useFeed()
  const c = feed.counters
  const feedVersion = c.received + c.replied + c.dropped + c.failed + (feed.live ? 1 : 0)
  const [total, setTotal] = useState(0)
  const [tick, setTick] = useState(0)

  useEffect(() => {
    const id = setInterval(() => setTick((t) => t + 1), pollMs)
    return () => clearInterval(id)
  }, [pollMs])

  useEffect(() => {
    let ignore = false
    void listConversations(fetcher).then((rows) => {
      if (!ignore) setTotal(unreadTotal(rows, readLastSeen()))
    })
    return () => {
      ignore = true
    }
  }, [fetcher, feedVersion, tick])

  return total
}
