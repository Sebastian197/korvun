// B9 — the direct-brain conversation-id contract, frontend half (spec
// 2026-08-29-b9-brain-selector-new-chat.md). A console conversation whose
// id is "b:<encodeURIComponent(brain)>:<rest>" was born addressed to that
// brain; the router honors it on the console channel only. These helpers
// are the ONE place the format lives on this side: creation (New chat),
// badge derivation (header + inbox row), and the N6 health resolution all
// go through them. encodeURIComponent escapes ":" so the two delimiters
// are unambiguous for any legal brain name.

/** Build a fresh direct-chat conversation KEY, optionally brain-addressed. */
export function newDirectChatKey(brain: string | null): string {
  const rest = `chat-${crypto.randomUUID().slice(0, 8)}`
  if (brain === null || brain === '') return `console::${rest}`
  return `console::b:${encodeURIComponent(brain)}:${rest}`
}

/** The brain a conversation id encodes, or null for ordinary ids. Mirrors
 *  the router's directBrainFromConversationID: both segments non-empty,
 *  the name percent-decoded; anything malformed is ordinary data. */
export function brainOfConversationId(id: string): string | null {
  if (!id.startsWith('b:')) return null
  const rest = id.slice(2)
  const sep = rest.indexOf(':')
  if (sep <= 0 || sep === rest.length - 1) return null
  try {
    const name = decodeURIComponent(rest.slice(0, sep))
    return name === '' ? null : name
  } catch {
    return null
  }
}

/** The id as shown to the user: the brain prefix stripped when present. */
export function displayIdOf(id: string): string {
  if (brainOfConversationId(id) === null) return id
  return id.slice(2 + id.slice(2).indexOf(':') + 1)
}
