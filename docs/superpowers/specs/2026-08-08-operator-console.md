# Operator console SP1–SP4 — inbox, sessions, manual reply, takeover: Design Spec

> **Status:** approved for TDD (2026-08-08 — the three clarifications
> resolved by Chano: (1) console surface = **Korvun Desktop** (his
> 2026-07-26 decision, "chat en la app de escritorio"; whether it shares
> the embedded builder shell is an SP4 technical detail, to confirm on
> disk then); (2) operator turns = **new role `operator` persisted
> as-is, mapped to `assistant`** in the provider translation; (3) real
> time = **the recommended shape**: SSE stays secret-free as change
> signal + bearer REST re-fetch — zero new ADR, ADR-0024 §1 intact).
> Amended 2026-08-08 (scope expansion by Chano: chat SESSIONS,
> OpenClaw-style).
> Session-model provenance: the semantics were decided by Chano and
> verified by him against OpenClaw's official doc
> (docs.openclaw.ai/concepts/session, 2026-08-08). There is NO code
> dependency on OpenClaw — the model below is self-contained, so no
> Context7/library verification applies to it.
> Governing ADRs: ADR-0017 (router owns inbound and outbound), ADR-0018 /
> Stage 9 ADR-A (conversation memory: Key/Turn/Store seam), ADR-0024 §1
> (live-view frames are secret-free — NEVER message content on the
> existing SSE), ADR-0026-era control-API conventions (bearer auth on
> mutations, read-only GETs on the admin server), Stage 14 phase 1 (the
> in-process bus + SSE live-view).
> External-docs note: the Go side uses **stdlib + existing `internal/`
> packages only** — no new dependencies, nothing to verify via Context7.
> The UI side CANNOT be spec'd to code level until clarification #1
> (console surface) is resolved; whichever surface is chosen, its
> libraries get Context7 verification at that point, per CLAUDE.md.
> Verified on disk before writing (2026-08-08, graph first, then source):
>
> - **Conversation store** (`internal/conversation`, sqlite impl): schema
>   `turns(key TEXT, seq INTEGER, role TEXT, content TEXT, ts INTEGER,
>   PRIMARY KEY(key, seq)) WITHOUT ROWID`, WAL, serialized writes; `Key`
>   = `channel + "::" + Meta["conversation.id"]`; roles today are
>   `user|assistant|system`. The `Store` seam is `LoadRecent / Append /
>   AppendTurns` — **there is NO conversation enumeration**; an inbox
>   needs an additive read (FR-STORE-1).
> - **Control API** (`internal/controlapi`): `GET /api/brains`,
>   `GET /api/channels`, `POST/GET /api/config` (bearer), `GET
>   /api/reload/{handle}` — **no conversation reads exist**. Rides the
>   admin server (`127.0.0.1:2112` default).
> - **SSE** (`internal/liveview`): `GET /api/events` streams bus events
>   (`message_received | reply_sent | message_dropped | handle_failed`)
>   as **secret-free frames — message content is outlawed on this stream
>   by ADR-0024 §1** (constraint honored by FR-RT-1).
> - **Brain inhibition point: none exists.** The router path is
>   `DispatchInbound → brainWorker → handleAndReply (brain.Handle) →
>   sendReply → channelWorker → Channel.Send`; `sendReply` is private and
>   there is **no public outbound entry** — the operator reply and the
>   takeover gate are both additive router surface (FR-OUT-1, FR-TAKE-1).

## Goal

After this piece, a human operator can SEE Korvun's conversations and TALK
through Korvun: an inbox lists conversations from the store with their
recent turns; the operator can answer any conversation manually from the
console — the reply leaves through the SAME channel adapter the message
came from, costing zero model calls — and can take a conversation over,
which silences the brain for that conversation while the human speaks and
resumes it on release. Everything the operator says is persisted in the
conversation history, so the brain regains full context afterwards.
**Conversations are organized in SESSIONS (Chano's 2026-08-08 expansion,
OpenClaw-style):** every conversation key holds a series of sessions, the
brain only ever loads the ACTIVE session, a session can be reset manually
from the channel (exact triggers) or from the console, or automatically by
policy (daily/idle) — and old sessions are preserved and navigable. The
console updates in real time by riding the existing SSE live-view without
violating its secret-free law. Explicitly out: multi-operator presence,
operator identities/permissions beyond the existing bearer token, canned
responses, any change to dispatch policy semantics, **session compaction**
(history is kept whole, never summarized away), and **`/new <model>`**
(model choice belongs to the policy engine, not to a chat command).

## Functional requirements

### Sessions (the 2026-08-08 expansion — semantics fixed by Chano)

- **FR-SESS-1 — Session model.** Every conversation `Key` owns an ordered
  series of sessions; each session has its own id and every turn belongs
  to exactly one session. At any moment exactly ONE session per key is
  ACTIVE; new turns land in it. The store schema scopes turns by session
  (see FR-STORE-1's migration note).
- **FR-SESS-2 — The brain sees only the active session.** `LoadRecent`
  is scoped to the key's active session: a reset is a hard context cut —
  the brain never receives turns from a previous session. Old sessions
  remain fully stored and readable (FR-API-1b), just never fed to the
  model again.
- **FR-SESS-3 — Manual reset, from the channel.** Exact-match triggers
  `"/new"` and `"/reset"` (the set is configurable; exact match on the
  first token, case-sensitive) close the active session and open a new
  one BEFORE dispatch. Any text remaining after the trigger is processed
  as the first message of the new session; a bare trigger produces a
  fixed, non-model acknowledgement through the normal outbound funnel
  (exact copy at the SP2 review). Trigger messages themselves are not
  fed to the brain.
- **FR-SESS-4 — Manual reset, from the console.** A bearer-auth endpoint
  (and its console button, FR-UI-1) opens a new session for a key —
  same semantics as the channel trigger, no acknowledgement message.
- **FR-SESS-5 — Automatic reset by policy.** Per-config session policy:
  `none` (default) | `daily` (at a configured local-time hour) | `idle`
  (after N minutes without inbound). `daily` and `idle` are combinable —
  **first to fire wins**. Expiry is evaluated lazily AT THE NEXT INBOUND
  (no timers, no background goroutines): if the active session expired
  under any configured rule, a new session opens and the inbound becomes
  its first message.
- **FR-SESS-6 — History is preserved and navigable.** Sessions are never
  deleted or compacted by this piece; the inbox exposes the session list
  per conversation and the detail API reads any session by id
  (FR-API-1/1b). Takeover (FR-TAKE-1) stays PER CONVERSATION and spans
  sessions — a reset does not release a takeover.

### Store (schema + reads — blast radius: `internal/conversation`, coverage ≥ 85 %; sqlite + memstore both)

- **FR-STORE-1 — Sessions in the schema + enumeration.** The `turns`
  schema gains session scoping (turn identity becomes key + session +
  seq) plus the session bookkeeping needed for "active" and for listing.
  **Migration:** existing rows become session 1 of their key, active —
  an upgrade never loses or reorders a turn (asserted by test on a
  pre-migration fixture). The enumeration read lists conversations
  (key, active-session id, session count, last-activity timestamp,
  last-turn role, newest-activity first, with a limit) and, per key, its
  sessions. `Append`/`AppendTurns` keep their atomicity contract,
  writing to the active session; `LoadRecent` is scoped per FR-SESS-2.
- **FR-STORE-2 — Operator turns persist in history.** Operator messages
  are appended to the ACTIVE session of the conversation via the
  existing atomic seam so the brain's next `LoadRecent` sees them
  (continuity). **Encoding (resolved 2026-08-08): a new
  `Role("operator")`, persisted as-is** (the schema takes any TEXT — no
  migration), **mapped to the provider's `assistant` role** in the
  brains' role translation (one explicit switch arm, guard-tested): a
  history containing operator turns loads and dispatches cleanly.

### Control API (bearer-auth conventions of the existing surface)

- **FR-API-1 — Inbox reads.** `GET /api/conversations` (the FR-STORE-1
  listing, active session marked) and `GET /api/conversations/{key}`
  (recent turns of the ACTIVE session, newest N, content included).
  Message content now leaves the process through an HTTP response for
  the first time: every endpoint here REQUIRES bearer auth (unlike the
  existing secret-free GETs) and stays on the admin (loopback-default)
  server. Traces to ADR-0024's posture.
- **FR-API-1b — Session navigation reads.** `GET
  /api/conversations/{key}/sessions` (the key's session list) and `GET
  /api/conversations/{key}/sessions/{id}` (any session's turns — old
  sessions included, FR-SESS-6). Bearer, same posture as FR-API-1.
- **FR-API-1c — New session.** `POST /api/conversations/{key}/sessions`
  (bearer): the FR-SESS-4 console reset.
- **FR-API-2 — Operator reply.** `POST /api/conversations/{key}/reply`
  (bearer): body carries the operator text; the handler builds an
  **operator Envelope** targeted at the conversation's origin channel +
  conversation id and hands it to the router's outbound entry (FR-OUT-1).
  Zero model-pool involvement by construction — the brain never runs.
  Persistence per FR-STORE-2 happens atomically with the send path's
  accounting (exact ordering in the spec review: persist-then-send, and a
  failed send is surfaced to the console AND recorded).
- **FR-API-3 — Takeover control.** `POST /api/conversations/{key}/takeover`
  and `.../release` (bearer), flipping the router's per-conversation gate
  (FR-TAKE-1). Idempotent; state readable in FR-API-1 responses.

### Router (additive seams — blast radius: `internal/router`, coverage ≥ 90 %)

- **FR-OUT-1 — Public outbound entry.** An exported router method (the
  operator path's front door) that enqueues an outbound Envelope to the
  target channel's existing outbound worker — same queue, same
  saturation/drop semantics, same `ReplySent`/`MessageDropped` bus events
  as brain replies. No parallel delivery path: one outbound funnel.
- **FR-TAKE-1 — Takeover gate.** A per-conversation-key gate consulted on
  the dispatch path: while taken over, inbound envelopes for that key are
  **persisted as user turns but NOT handed to the brain** (the human is
  the brain); a bus event still marks the message so the console updates.
  On release, the gate lifts and the next inbound flows to the brain,
  which now loads a history containing the operator's turns (FR-STORE-2).
  The gate is in-memory (a restart releases all takeovers — folded
  decision below).

### Real time

- **FR-RT-1 — SSE reuse WITHOUT breaking the secret-free law.** The
  console subscribes to the existing `GET /api/events`. Frames remain
  exactly as they are (typed, secret-free, no content); the console
  treats them as change signals — on an event for a watched key it
  re-fetches via FR-API-1 (bearer). NO content ever rides the SSE
  stream. If clarification #3 chooses a content-bearing stream instead,
  that is a new ADR, not an amendment of this FR.

### Console UI

- **FR-UI-1 — The console lives in KORVUN DESKTOP** (resolved
  2026-08-08, per Chano's 2026-07-26 decision): inbox (list + session
  navigation), the "new session" button, the reply box, and the takeover
  switch, live-updating per FR-RT-1. Whether the implementation shares
  the embedded builder shell is an SP4 technical detail, confirmed on
  disk at SP4 kickoff (with its own Context7 verifications then). The
  backend FRs above stay surface-agnostic on purpose.

## Acceptance scenarios (Given / When / Then)

- **AS-1 (inbox lists reality)** Given three conversations in the store
  across two channels, When `GET /api/conversations` is called with a
  valid bearer, Then all three appear newest-activity first with correct
  counts/timestamps; without a bearer, Then 401 and no content leaks.
- **AS-2 (detail)** Given a conversation with N turns, When its detail
  endpoint is called, Then the most recent turns arrive oldest-first with
  role, content, and seq — byte-equal to what `LoadRecent` returns.
- **AS-3 (manual reply, zero model cost)** Given a conversation that
  arrived via a (fake) channel adapter, When the operator posts a reply,
  Then the adapter's `Send` receives an Envelope carrying the operator
  text targeted at the right conversation id; the model layer records
  ZERO calls; a `ReplySent` bus event fires; and the turn is in the store.
- **AS-4 (takeover silences the brain)** Given an active takeover on key
  K, When an inbound envelope for K arrives, Then the brain's `Handle` is
  NOT invoked, the user turn IS persisted, and an event still reaches the
  bus; Given a release, When the next envelope arrives, Then the brain
  runs and its loaded history contains the user turns from the takeover
  window AND the operator's replies, in order.
- **AS-5 (send failure is honest)** Given the origin adapter fails
  `Send`, When the operator posts a reply, Then the API responds with the
  failure, a `MessageDropped` event fires, and the console can see the
  reply did not deliver.
- **AS-6 (secret-free SSE unchanged)** Given the console connected to
  `/api/events` during replies and takeovers, When frames are captured,
  Then no frame contains message content — byte-audited in the test.
- **AS-7 (concurrency)** Given concurrent operator replies and inbound
  messages on the same key, When they race, Then the store's group
  atomicity holds (no interleaved pairs), `-race` is clean, and no
  committed turn is lost.
- **AS-8 (auth on every new mutation/read)** Given any FR-API endpoint,
  When called without/with a wrong bearer, Then 401 with a body that
  leaks nothing.
- **AS-9 (manual reset cuts context)** Given a conversation with turns
  in its active session, When a reset happens (channel trigger OR the
  console endpoint) and a new inbound arrives, Then the brain's model
  call contains ZERO turns from the previous session (asserted at the
  fake model's received-history level), and the previous session's turns
  are still fully readable via FR-API-1b.
- **AS-10 (`/new` passes the rest of the message)** Given an inbound
  `"/new hola"`, When it is dispatched, Then a new session exists, the
  brain receives exactly `"hola"` as the first (and only) user turn of
  that session, and no turn containing `"/new"` was persisted or fed to
  the brain; Given a bare `"/reset"`, Then a new session exists and the
  fixed acknowledgement leaves through the outbound funnel with zero
  model calls.
- **AS-11 (expiry fires at next inbound, first-rule-wins)** Given policy
  `idle: 30m` and an active session whose last inbound is 31 minutes old
  (injected clock), When the next inbound arrives, Then it opens and
  lands in a new session; Given policy `daily` at hour H with the clock
  crossing H since the last inbound, Then likewise; Given both rules
  configured, Then a single new session opens (whichever rule fired
  first) — never two.
- **AS-12 (session history integrity)** Given a key that accumulated
  three sessions (two resets), When the store and the FR-API-1b reads
  are inspected, Then every turn of every session is present, scoped to
  its session, in order, with no leakage between sessions — and the
  pre-migration fixture's turns live intact as session 1.

## Success criteria

- Coverage: ≥ 90 % in `internal/router` additions; ≥ 85 % in
  `internal/conversation` and `internal/controlapi` additions.
- `make quality` green with `-race` over the whole suite; `go.mod`
  untouched (stdlib only).
- The single-binary posture intact: no new server, everything rides the
  existing admin server; website/desktop pipelines untouched.
- Every AS above green in CI; AS-6's audit proves ADR-0024 §1 survived;
  AS-12's fixture proves an upgrade loses no history (existing turns
  land intact as session 1).

## Decisions folded in (veto-able)

- **Takeover state is in-memory, per-process** — a restart releases all
  takeovers (fail-open to the brain, never to silence). Durable takeover
  is a future additive if operators ask for it.
- **Session ids are store-assigned, monotonic per key** (1, 2, 3… — the
  migration's "existing turns become session 1" falls out naturally).
- **Trigger matching is first-token, exact, case-sensitive** on the
  configured set (default `["/new", "/reset"]`, both with identical
  semantics); the trigger message itself is never persisted as a turn.
- **`daily` default hour 04:00, server-local time,** configurable
  (`session.daily_at`); `idle` in whole minutes (`session.idle_min`).
  Policy default is `none` — upgrading changes nothing until configured.
- **Expiry is lazy** (evaluated at next inbound) — no timers, no
  background goroutines, nothing to shut down; an idle Korvun stays as
  quiet as today.
- **Release is manual-only in v1** — no auto-timeout; the console shows
  takeover state so a forgotten one is visible (per FR-API-1/AS-1).
- **Persist-then-send** for operator replies (the history is the source
  of truth; a failed send is recorded AND surfaced — AS-5).
- **One outbound funnel** — operator replies share the brain replies'
  queue and events; no priority lane.
- **Inbox pagination by simple limit** in v1 (newest first); cursors are
  additive later.

## SP breakdown (amended 2026-08-08 — BLOCKED until clarifications close)

- **SP1 — Store: sessions in the schema + enumeration + operator-turn
  encoding.** Turns scoped by session, migration (existing turns →
  session 1, fixture-asserted), enumeration with active session,
  `LoadRecent` bounded to the active session. Red: AS-1/AS-2/AS-12 at
  store level + the role-translation guard from FR-STORE-2.
- **SP2 — Dispatch + router seams: triggers, lazy expiry, outbound
  entry, takeover gate.** Channel trigger handling and policy expiry
  evaluation on the inbound path (FR-SESS-3/5), the public outbound
  entry and the per-conversation takeover gate. Red: AS-3/AS-4/AS-7/
  AS-9/AS-10/AS-11 at router level with fake channel/brain/model and an
  injected clock.
- **SP3 — Control API: conversation + session endpoints + SSE audit.**
  FR-API-1/1b/1c/2/3 over HTTP. Red: AS-1/2/3/4/5/6/8 (+ AS-9's console
  half) over HTTP.
- **SP4 — Console UI on the chosen surface:** inbox, session navigation,
  "new session" button, reply box, takeover switch (scoped after
  clarification #1; its own red set per the surface's harness).

## Review checklist (template gate — green before "approved for TDD")

- [x] Goal stated behaviorally, with explicit exclusions (compaction,
      `/new <model>`, presence, canned responses).
- [x] Every FR traces to a governing decision (Chano 2026-08-08 ×2 +
      2026-07-26, ADR-0017/0018/0024) and names its seam and blast
      radius.
- [x] Acceptance scenarios assertable; unhappy paths covered (auth,
      failed send, saturation semantics, migration fixture, no-leak SSE
      audit, race on same key).
- [x] Success criteria measurable (coverage floors, `-race`, `go.mod`
      untouched, AS-6/AS-12 proofs).
- [x] External-docs verification stated: Go side stdlib+internal only;
      session model self-contained (no OpenClaw code dependency); UI
      verifications deferred to SP4 kickoff by design.
- [x] `[NEEDS CLARIFICATION]` empty — **all three resolved 2026-08-08;
      checklist green.**

## `[NEEDS CLARIFICATION]`

1. ~~Console surface.~~ **Resolved (Chano, 2026-08-08): Korvun Desktop**
   — his 2026-07-26 decision; builder-shell sharing is an SP4 detail to
   confirm on disk. Folded into FR-UI-1.
2. ~~Operator turn encoding.~~ **Resolved (Chano, 2026-08-08): new role
   `operator` persisted as-is, mapped to `assistant` in the provider
   translation** (the spec's recommendation, accepted). Folded into
   FR-STORE-2.
3. ~~Real-time transport for content.~~ **Resolved (Chano, 2026-08-08):
   the recommended shape** — SSE intact and secret-free as change
   signal, bearer REST re-fetch for content; zero new ADR, ADR-0024 §1
   untouched. Folded into FR-RT-1.

No open points remain.
