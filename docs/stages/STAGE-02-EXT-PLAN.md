# STAGE 2-EXT (PLAN) — Telegram full channel

> **Status:** planned, NOT yet executed.
> **Prerequisite:** Stage 3 (Router / gateway core) must be complete first,
> because Phase 2E.8 (webhook lifecycle) wires Telegram into the router via
> the `Channel` interface (Receive/Send).
> **Goal:** take the Telegram adapter from text-only (Phase 2.3) to full
> coverage of the features below.

## Methodology (applies to every phase)

For each phase, follow the non-negotiable cycle from CLAUDE.md:

1. **Verify external docs first.** `use context7` to verify the
   `github.com/go-telegram/bot v1.21.0` API before touching code — in
   particular the `models` types involved (PhotoSize, Voice, Document, Video,
   Audio, Location, CallbackQuery, MessageReaction, etc.). Do not code against
   these types from memory.
2. **Tests first (TDD).** Red tests with JSON fixtures, no real network, before
   implementation.
3. **Implementation.** Minimum code to go green. Inbound/outbound conversion
   functions stay pure (no network), as in Phase 2.3.
4. **Quality gate.** `make quality` green over the whole suite, with `-race`.
5. **Documentation.** Commit per phase, push on closing each phase.

Coverage target: ≥90% (channel code carries weight).

**Domain note:** several content types will likely require extending the
`envelope` domain (new `PartType` values for media, location, action/callback,
etc.). Any change to the `internal/envelope` package touches the core domain
that everything depends on — STOP and flag it before changing `envelope`, write
its own test, and record a design ADR. Whatever the Envelope does not model
goes into `Envelope.Meta` (with a `file_id` reference for media), keeping the
Envelope as the source of truth.

## Phases

### Phase 2E.1 — Inbound media
Map photos, audio, video, documents, and voice messages from an `Update` into
the Envelope (appropriate `PartType`, `file_id` reference in Meta).
Tests: one fixture per media type → expected Envelope.

### Phase 2E.2 — Outbound media
Build Telegram send params (`SendPhoto`, `SendDocument`, `SendVoice`,
`SendAudio`, `SendVideo`) from an Envelope carrying media.
Tests: Envelope with each media type → correct params.

### Phase 2E.3 — Locations
Inbound and outbound of `Location` (lat/long).
Tests: round-trip.

### Phase 2E.4 — Buttons / keyboards (callback queries)
Map an outbound `inline_keyboard` from the Envelope (via Meta or a domain
extension), and parse an inbound `CallbackQuery` as an action-type Envelope.
Tests: keyboard construction; callback parsing.

### Phase 2E.5 — Commands
Detect and normalize commands (`/start`, `/help`, etc.) on inbound, possibly
with a dedicated `PartType` or a Meta marker.
Tests: command parsing with and without arguments.

### Phase 2E.6 — Message editing
Inbound of `edited_message` and outbound `EditMessageText` /
`EditMessageCaption`.
Tests: edit update → Envelope; edit Envelope → params.

### Phase 2E.7 — Reactions
Inbound/outbound of reactions (`message_reaction`, `setMessageReaction`).
First confirm via Context7 that the client version supports them; if not,
document the limitation.
Tests: according to support.

### Phase 2E.8 — Webhook lifecycle (real reception)
The transport that actually receives updates: an HTTP server that receives
Telegram's webhook, validates the secret token, and produces Envelopes through
the converters built in the previous phases. Here the adapter implements the
`Channel` interface (Receive/Send), wiring the transport to the Stage 3 router.
Tests: simulated webhook request → dispatched Envelope; outbound send mocked.

## Closure

- `docs/stages/STAGE-02-EXT.md` written on completion.
- Coverage ≥90%.
- Design ADR(s) if the `envelope` domain was extended.
