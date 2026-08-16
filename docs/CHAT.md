# Chat — the operator console in Korvun Desktop

The **Chat** tab of Korvun Desktop is the operator console: read every
conversation flowing through your channels, answer them yourself when you
want to, and talk to your brains directly — all from the app, all persisted
in the same durable store.

Requirements: the `storage` and `session` config blocks (see
[`CONFIGURATION.md`](CONFIGURATION.md)). The desktop provisions both
automatically on first run **and** when upgrading an existing config, so
normally there is nothing to do. Direct chat with the AI additionally needs
a `{ "type": "console" }` channel entry.

## The inbox

The left pane lists every conversation in the store, newest activity first:

- **Filter as you type** — the box narrows the list instantly by
  conversation key, and also searches message **content** on the server
  (matches from other conversations appear below the filtered list).
- **Unread badges** — each conversation shows how many turns arrived since
  you last opened it; the Chat tab in the sidebar carries the total. Opening
  a conversation marks it read. The read state is local to your desktop.
- **TAKEN OVER** marks conversations where you hold the takeover (below).

## Reading a conversation

Turns are role-styled: the end user, the brain (**Korvun**), you
(**Operator (you)**, violet, right-aligned), and system lines (dashed) such
as session-reset acknowledgements. Attachments arrive **announced**: a
photo persists and renders as an `[image]` marker (media rendering is
post-beta by design). Timestamps are relative; the pane autoscrolls unless
you have scrolled up to read history.

## Sessions, `/new` and `/reset`

A conversation is a series of **sessions**; only the newest is active and
only the active one is the brain's context. A session ends when:

- the end user (or you, in direct chat) sends a reset trigger — `/new` or
  `/reset` by default (`session.triggers`) — as the exact first token; the
  channel answers with a fixed acknowledgement and a fresh session opens; or
- a configured `daily_at` / `idle_min` expiry passes, applied lazily at the
  next inbound message (no timers).

Old sessions stay stored: the session tabs above the transcript navigate
them read-only. Resetting never deletes anything — it only cuts the context.

The cut is recoverable **on purpose, never by accident**: `/recall` (enabled
by `session.recall_max`) quotes the previous session's tail back into the
active one as ONE clearly-marked block — the header names its source
session, the acknowledgement names how many turns came back. It only works
on an EMPTY active session; on a non-empty one it refuses and points you to
`/new` first. Deliberate recovery, provenance visible, duplication
impossible.

### Notes and `/notes`

A brain with memory configured also keeps **notes** — short facts the model
stores through the governed `memory_note` tool (`docs/TOOLS-AND-SKILLS.md`).
Notes are not history: they SURVIVE session resets on purpose, riding the
brain's context only within their scope (per-conversation by default).
`/notes` lists them numbered; `/notes clear` wipes the scope — both are
instant system commands, no model call, and clearing notes never touches
the transcript.

## Answering as the operator (takeover)

On a network channel (telegram, discord, webhook), replying by hand first
requires **Take over**: while you hold it, the brain is silenced for that
conversation — inbound turns still persist and still appear live, but no
model answers. Your replies go out through the channel's own adapter
(cost zero — no model call) and persist as operator turns, so when you
**Release**, the brain resumes *with your replies in its context*.
Takeover is per-conversation, survives session resets, and is not durable
across a core restart (fail-open: the brain answers again).

## Direct chat with the AI (console channel)

With a `console` channel configured, **New chat** starts a conversation
between you and a brain — you are the *user* here, so there is no takeover
and no Take over button (the composer explains this). Messages run the full
pipeline (policy, routing, persistence); **Thinking…** shows while the brain
works. Sessions, `/new`, deletion, search and unread badges behave exactly
as everywhere else. With no explicit route, the console channel talks to the
first configured brain.

## Deleting

- **Delete conversation** removes the whole conversation — every session,
  every turn — from disk, and releases any takeover first. The confirmation
  is explicit: *"This deletes the conversation from disk. No undo."*
- **Delete session** removes one **archived** session; the active session
  cannot be deleted (use `/new` first if you really want to cut it).

Both are permanent. There is no undo.

## Security notes

- The desktop UI never holds the admin bearer: the shell injects it
  server-side (ADR-0024 §1 / ADR-0028), and the SSE stream stays
  secret-free — real time is "SSE says something changed, REST re-fetches".
- Secrets are only ever env-var names in the config; values live in the
  environment or the OS keychain.
