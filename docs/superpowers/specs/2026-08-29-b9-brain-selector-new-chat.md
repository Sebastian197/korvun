# B9 — brain selector on New chat: Design Spec (the short engine spec)

> **Status:** approved for TDD. UX contract: `design-drafts/ola2-designs.md`
> §1 — APROBADO POR CHANO 2026-08-23, sealed decisions: cancel = Esc AND
> click-outside; microload = «Cargando cerebros…» (the list rides the N6
> `/api/brains` fetch). The approved maqueta IS the visual contract.
> Governing ADRs: ADR-0017 §2 (DispatchInbound owns routing), ADR-0022 §3
> (Reader seam), operator-console spec FR-CONS-1/3. External-docs note:
> stdlib + existing internal packages + existing frontend stack only.

## Goal

A direct (console) conversation can be born addressed to a CHOSEN brain
without touching channel routes. The conversation id transports the brain;
the router honors it for enabled channels only; a vanished brain falls
back to the route default with an honest in-conversation notice. Existing
conversations and every network channel are untouched. Out of scope
(sealed): Telegram conversation start, changing the brain of an existing
conversation, persisting the Activity feed.

## The id format (the engine decision)

`b:<encodeURIComponent(brainName)>:<rest>` as the console conversation id
(key `console::b:openrouter:chat-ab12cd34`). Rationale: the id IS the
persisted identity (the store keys turns by it), so the brain choice
survives restarts with ZERO storage schema change; ids without the prefix
keep today's behavior byte-for-byte (retrocompat). Percent-encoding makes
parsing unambiguous for any legal brain name (config only requires
non-empty + unique): encodeURIComponent escapes `:` (%3A), so the first
and second `:` delimit reliably; Go decodes with `url.PathUnescape`.

## Functional requirements

- **FR-B9-1 (router, gated)** — `Router` gains a wiring option
  `WithDirectBrainChannel(name)`. ONLY for envelopes of an enabled
  channel, `DispatchInbound` parses the conversation id; a `b:` id naming
  a registered brain dispatches to THAT brain (queue, session pre-dispatch
  labels, publishReceived all carry the effective brain). PRIVACY
  INVARIANT: channels not enabled never honor the prefix — a webhook
  `conversation_id` shaped like `b:private-brain:x` must not bypass its
  route. The app wires the option for the console channel only.
- **FR-B9-2 (fallback + notice)** — an enabled-channel `b:` id naming a
  brain NOT registered falls back to the route's brain, and the router
  sends ONE ack envelope (new `envelope.AckBrainFallback`) with the sealed
  Spanish text «El cerebro "X" ya no existe — esta conversación continúa
  con el cerebro por defecto.» The console channel already persists acks
  as system turns. The notice is deduplicated per conversation in a
  BOUNDED window (the dedupWindow mechanism) — never one per message
  burst, never unbounded state.
- **FR-B9-3 (frontend, New chat)** — with >1 brain in `/api/brains`, the
  New chat button opens the sealed dropdown («¿Con qué cerebro?», radio
  rows name + Privado/Público from sensitivity, route default
  preselected via the existing `brainForChannel`); [Crear chat] creates
  `console::b:<enc(name)>:chat-<uuid8>`; Esc AND click-outside cancel
  leaving no trace; while the brains list is in flight the dropdown shows
  «Cargando cerebros…». With ≤1 brain: today's one-click behavior
  byte-for-byte.
- **FR-B9-4 (badge)** — a conversation whose id encodes a brain shows it
  in the pane header (`Console · <brain> · <rest>`) and in the inbox row.
  Ids without the prefix render exactly as today.
- **FR-B9-5 (N6 coherence)** — the dead-brain warning resolves the
  serving brain of an explicit-brain conversation as THAT brain (id
  first, route fallback — `deadBrainForConversation` honors the same
  parse).

## Acceptance scenarios

- **AS-1 (the case that birthed the piece)** Given two brains (one local
  route-default, one cloud) wired in a router, When an inbound console
  envelope carries conversation id `b:cloud:chat-1`, Then the CLOUD
  brain's queue receives it and the received event names the cloud brain
  — and with id `chat-2` (no prefix), the route default receives it.
- **AS-2** Given the prefix names a brain that does not exist, Then the
  route default handles the message AND exactly one AckBrainFallback
  envelope with the sealed text reaches the channel — a second message in
  the same conversation does not re-send it; a DIFFERENT conversation
  gets its own.
- **AS-3 (privacy)** Given the same `b:cloud:...` id on a NON-enabled
  channel, Then routing ignores the prefix entirely (route brain, no ack).
- **AS-4 (frontend flow)** open → choose → [Crear chat] opens a
  conversation keyed `console::b:<enc>:chat-*`; Esc cancels; click-outside
  cancels; pre-list-arrival shows «Cargando cerebros…»; single-brain
  config never shows the dropdown.
- **AS-5 (badge)** header and inbox row show the brain for `b:` ids;
  legacy ids render unchanged.

## Success criteria

- `make quality` green with `-race`; router/brain coverage floors (≥90)
  intact; desktop-frontend-check + its coverage gate green.
- Zero changes to routes semantics, network channels, storage schema, or
  the console POST body.

## Decisions folded in

- Router option over Meta-injection at the control API: one parser, one
  owner of routing semantics, and the enable-list keeps the router free
  of channel-name literals.
- Fallback notice dedup via the existing bounded dedupWindow mechanism —
  the resource-bound invariant over per-conversation mutable state.
- The ghost brain's name appears in the notice text (conversation
  content, operator-authored id) — never in metric labels.

## `[NEEDS CLARIFICATION]`

None — the two open points of the UX draft were sealed by Chano
(Esc+click-outside; «Cargando cerebros…») per the lote mandate.
