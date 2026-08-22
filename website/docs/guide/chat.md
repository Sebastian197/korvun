# The operator console

The **Chat** tab of Korvun Desktop is the operator console: read every
conversation flowing through your channels, answer them yourself when you
want to, and talk to your brains directly — all from the app, all persisted
in the same durable store.

It needs the `storage` and `session` blocks from the
[configuration reference](/reference/configuration). The desktop provisions
both automatically on first run **and** when upgrading an existing config,
so normally there is nothing to do. Direct chat with the AI additionally
needs a `console` channel entry:

```json
{ "type": "console" }
```

## The inbox

The left pane lists every conversation in the store, newest activity first:

- **Filter as you type** — the box narrows the list instantly by
  conversation key, and also searches message **content** on the server.
- **Unread badges** — each conversation shows how many turns arrived since
  you last opened it; opening a conversation marks it read.
- **TAKEN OVER** marks conversations where you hold the takeover (below).

Turns are role-styled: the end user, the brain (**Korvun**), you
(**Operator**, right-aligned), and dashed system lines such as session-reset
acknowledgements. Attachments arrive **announced**: a photo persists and
renders as an `[image]` marker.

## Sessions, `/new` and `/reset`

A conversation is a series of **sessions**; only the newest is active and
only the active one is the brain's context. A session ends when:

- the end user (or you, in direct chat) sends a reset trigger — `/new` or
  `/reset` by default — as the exact first token; the channel answers with
  a fixed acknowledgement and a fresh session opens; or
- a configured `daily_at` / `idle_min` expiry passes, applied lazily at the
  next inbound message.

Old sessions stay stored: the session tabs above the transcript navigate
them read-only. Resetting never deletes anything — it only cuts the context.

## Recovering a cut: `/recall`

The cut is recoverable **on purpose, never by accident**. `/recall`
(enabled by `session.recall_max`) quotes the previous session's tail back
into the active one as ONE clearly-marked block — the header names its
source session, the acknowledgement names how many turns came back. It only
works on an EMPTY active session; on a non-empty one it refuses and points
you to `/new` first. Deliberate recovery, provenance visible, duplication
impossible by construction. The full design is on the
[governed memory](/guide/memory) page.

## Notes and `/notes`

A brain with memory configured also keeps **notes** — short facts the model
stores through the governed `memory_note` tool (see
[governed tools and skills](/guide/tools-and-skills)). Notes are not
history: they SURVIVE session resets on purpose, riding the brain's context
only within their scope. `/notes` lists them numbered; `/notes clear` wipes
the scope — both are instant system commands, no model call, and clearing
notes never touches the transcript.

## Answering as the operator (takeover)

On a network channel (Telegram, Discord, webhook), replying by hand first
requires **Take over**: while you hold it, the brain is silenced for that
conversation — inbound turns still persist and still appear live, but no
model answers. Your replies go out through the channel's own adapter (cost
zero — no model call) and persist as operator turns, so when you
**Release**, the brain resumes *with your replies in its context*. Takeover
is per-conversation, survives session resets, and is not durable across a
core restart (fail-open: the brain answers again).

## Direct chat with the AI (console channel)

With a `console` channel configured, **New chat** starts a conversation
between you and a brain — you are the *user* here, so there is no takeover.
Messages run the full pipeline (policy, routing, persistence);
**Thinking…** shows while the brain works. Sessions, `/new`, deletion,
search and unread badges behave exactly as everywhere else. With no
explicit route, the console channel talks to the first configured brain.

From the console channel, `/tools` returns the gatekeeper report of that
conversation's brain — its effective tool grants and recent tool activity.

## Deleting

- **Delete conversation** removes the whole conversation — every session,
  every turn — from disk, and releases any takeover first.
- **Delete session** removes one **archived** session; the active session
  cannot be deleted (use `/new` first if you really want to cut it).

Both are permanent. There is no undo.

## Security notes

- The desktop UI never holds the admin bearer: the shell injects it
  server-side, and the live event stream stays secret-free.
- Secrets are only ever env-var names in the config; values live in the
  environment or the OS keychain.
