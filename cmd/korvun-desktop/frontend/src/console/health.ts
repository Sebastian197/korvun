// N6 (bug-bash 2026-08-23): a brain whose models all failed their boot probe
// answered nothing and the chat gave no hint — the user discovered the broken
// model mid-conversation. The chat now resolves the serving brain of the open
// conversation (config route: channel → brain) and, when the core OBSERVED
// every one of its models unreachable (/api/brains health, v0.9.2), says so
// before the user types into the void. Parsing is defensive throughout: any
// unexpected shape resolves to "no warning", never a crash.

import { brainOfConversationId } from './brainid'

interface RouteLike {
  channel?: unknown
  brain?: unknown
}

interface ModelLike {
  health?: unknown
}

interface BrainLike {
  name?: unknown
  models?: unknown
}

/** The channel type of a conversation key ("console::chat-1" → "console"). */
export function channelOfKey(key: string): string {
  const sep = key.indexOf('::')
  return sep === -1 ? key : key.slice(0, sep)
}

/** The conversation id of a key ("console::chat-1" → "chat-1"). */
function idOfKey(key: string): string {
  const sep = key.indexOf('::')
  return sep === -1 ? '' : key.slice(sep + 2)
}

/** Resolve the brain a channel routes to, or null when unknowable. */
export function brainForChannel(routes: unknown, channel: string): string | null {
  if (!Array.isArray(routes)) return null
  for (const r of routes as RouteLike[]) {
    if (
      r !== null &&
      typeof r === 'object' &&
      r.channel === channel &&
      typeof r.brain === 'string'
    ) {
      return r.brain
    }
  }
  return null
}

/** True only when the brain HAS models and the core observed every one of
 * them unreachable. Unknown/unprobed models keep this false — the warning
 * never cries wolf on absence of evidence. */
export function hasNoLiveModels(brain: unknown): boolean {
  if (brain === null || typeof brain !== 'object') return false
  const models = (brain as BrainLike).models
  if (!Array.isArray(models) || models.length === 0) return false
  return (models as ModelLike[]).every(
    (m) => m !== null && typeof m === 'object' && m.health === 'unreachable',
  )
}

/** Resolve the conversation's serving brain and return its name when it has
 * no live models — the chat's warning trigger. Null = no warning. */
export async function deadBrainForConversation(
  key: string,
  fetcher: typeof fetch,
): Promise<string | null> {
  try {
    const [cfgResp, brainsResp] = await Promise.all([
      fetcher('/api/config', { cache: 'no-store' }),
      fetcher('/api/brains', { cache: 'no-store' }),
    ])
    if (!cfgResp.ok || !brainsResp.ok) return null
    const cfg = (await cfgResp.json()) as { routes?: unknown }
    const brains = (await brainsResp.json()) as unknown
    // B9 (FR-B9-5): a brain-addressed conversation is served by the brain
    // its ID names — the route only resolves legacy ids.
    const brainName =
      brainOfConversationId(idOfKey(key)) ?? brainForChannel(cfg?.routes, channelOfKey(key))
    if (brainName === null || !Array.isArray(brains)) return null
    const brain = (brains as BrainLike[]).find(
      (b) => b !== null && typeof b === 'object' && b.name === brainName,
    )
    return brain !== undefined && hasNoLiveModels(brain) ? brainName : null
  } catch {
    return null
  }
}
