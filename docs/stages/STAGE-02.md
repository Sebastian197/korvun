# Stage 02 — Channels

> **Status:** open (pending Telegram `channel.Channel` wrapper, tracked in Phase 2E.8)
> **Started:** 2026-06-13
> **Closed:** —

## Objective

Build the messaging-gateway side of Korvun: define a canonical channel
contract, implement adapters for each supported transport, and translate
their native payloads to and from the canonical Envelope, with no
business logic leaking into the adapter and no transport coupling forced
on the Envelope.

## Phases

| Phase | Description                                       | Status |
|-------|---------------------------------------------------|--------|
| 2.1   | `channel.Channel` interface, `Manifest`, Registry | done   |
| 2.2   | Generic webhook adapter (`net/http` only)         | done   |
| 2.3   | Telegram adapter — pure converters (inbound + outbound, text) | done   |

## Path Convention

The canonical Go package layout for channels is **`internal/channel/`**
(singular). The interface, the Registry, the webhook adapter, and every
future channel implementation live under this path.

The existing `internal/channels/telegram/` (plural) directory predates
the merge of Phase 2.1 / 2.2 into master and is kept in place for now;
it will be moved to `internal/channel/telegram/` when its
`channel.Channel` wrapper is built. See "Open debt" below.

## Phase 2.1 — Channel Interface and Registry

### Deliverables (in master via merge of `stage-2/phase-2.1-channel-interface`)

- `internal/channel/channel.go`
  - `Channel` interface: `Name() string`,
    `Manifest() Manifest`,
    `Send(ctx, *envelope.Envelope) error`,
    `Receive(ctx) (<-chan *envelope.Envelope, error)`.
  - `Manifest` struct with capability flags
    (`Text`, `Image`, `Audio`, `Video`, `Buttons`).
- `internal/channel/registry.go`
  - `Registry` backed by `sync.RWMutex`: `Register`, `Unregister`,
    `Get`, `List`.

### Quality

- `internal/channel` coverage: 100.0% (verified on master after merge).
- `make quality`: green.

## Phase 2.2 — Generic Webhook Adapter

### Deliverables (in master via merge of `stage-2/phase-2.2-webhook-adapter`)

- `internal/channel/webhook/webhook.go` — generic webhook channel that
  converts arbitrary JSON payloads to/from Envelopes via configurable
  field mappings. Uses only Go stdlib `net/http`. Implements the
  `channel.Channel` interface introduced in Phase 2.1.
- `internal/channel/webhook/webhook_test.go` — table-driven suite.

### Quality

- `internal/channel/webhook` coverage: 91.4% (verified on master after
  merge, > 90% target for non-critical packages).
- `make quality`: green.

## Phase 2.3 — Telegram Adapter (pure converters)

### Deliverables

- `docs/adr/0001-telegram-client.md` — ADR justifying the external
  dependency `github.com/go-telegram/bot v1.21.0` against two
  alternatives on license compatibility with Apache-2.0, tagged-release
  cadence, transitive dependencies, and fit with the Korvun Channel
  port.
- `internal/channels/telegram/` package with four files:
  - `doc.go` — package overview and exported `ChannelName`,
    `MetaChatID`, `MetaChatType`, `MetaMessageID` constants.
  - `errors.go` — sentinel errors matchable via `errors.Is`.
  - `inbound.go` — `InboundFromUpdate(*models.Update) (*envelope.Envelope, error)`.
  - `outbound.go` — `OutboundToSendMessage(*envelope.Envelope) (*bot.SendMessageParams, error)`.
- `internal/channels/telegram/adapter_test.go` — black-box-style suite,
  table-driven, covering nominal and error paths for both conversions.
- `internal/channels/telegram/testdata/text_message.json` — Bot API
  Update fixture, decoded with `encoding/json` into `models.Update`.

### Scope and Boundaries

Phase 2.3 covers **text messages only**. Photos, documents, voice notes,
videos and inline keyboards are deliberately out of scope and rejected
with `ErrUnsupportedContent` (inbound) or `ErrNoTextPart` (outbound), so
that a later phase can extend coverage without breaking the existing
contract.

The adapter is **transport-agnostic**: it owns no HTTP client, no
long-poll loop, and no webhook server. Updates are fed in as
`*models.Update` from whichever transport the caller chooses; outbound
results are `*bot.SendMessageParams` ready to be passed to
`bot.SendMessage` by the same caller. This made TDD possible without a
real Telegram token, and is intentional: the Telegram adapter does
**not** yet implement the `channel.Channel` interface introduced in
Phase 2.1. The wrapper is open debt — see below.

### Key Design Decisions

- **Sender ID is the stringified Telegram int64 user ID.** Stable across
  username changes; the Envelope can rely on it as a primary key.
- **Sender display name prefers `@username`.** Falls back to
  `FirstName + " " + LastName` (trimmed) when no username is set.
- **Inbound timestamp comes from `Message.Date`** (UTC), not from
  `time.Now()`. The Envelope reflects when the user sent the message,
  not when Korvun received it.
- **Telegram-specific metadata is namespaced** under `telegram.`
  (`telegram.chat_id`, `telegram.chat_type`, `telegram.message_id`) so
  multiple channel adapters can coexist in the same Envelope without
  key collisions.
- **All error returns are sentinel `errors.Is`-matchable** — no string
  matching.

### Quality

- `internal/channels/telegram` coverage: 100.0%
- Tests run with `-race`.
- gosec G304 explicitly justified in `loadUpdateFixture` (test-only
  path read).

## Open Debt — Tracked in Phase 2E.8 (Telegram webhook lifecycle)

The following items are **not** new sub-phases of Stage 2. They are
deliberately deferred to **Phase 2E.8** of the Telegram-completion plan,
where the real Telegram transport (HTTP client, long-poll or webhook
listener) is built:

- **Move `internal/channels/telegram/` → `internal/channel/telegram/`**
  so the package sits under the canonical `internal/channel/` tree.
- **Add `telegram.Adapter` struct implementing `channel.Channel`** —
  wraps the existing pure converters with `Name() string`,
  `Manifest() Manifest`, `Send(ctx, *envelope.Envelope) error` (real
  HTTP via `bot.SendMessage`), and `Receive(ctx) (<-chan *envelope.Envelope, error)`
  (long-poll or webhook lifecycle).
- **Update import paths** in any caller (router included) once the move
  lands.

Stage 3 (router) does **not** depend on the Telegram wrapper: it
consumes `channel.Channel` and uses an in-test fake implementation that
satisfies the interface, so the router can be built and tested
independently of the Telegram wrapper landing.

## Key Decisions

- ADR-0001 — Telegram client library choice.
- Path convention `internal/channel/` (singular).
- Telegram `channel.Channel` wrapper deferred to Phase 2E.8.

## Quality Gate (Stage-wide, on master)

- `make quality`: pass
- Coverage by package:
  - `internal/channel` 100.0%
  - `internal/channel/webhook` 91.4%
  - `internal/channels/telegram` 100.0%
  - `internal/envelope` 97.8%
  - overall 95.9%
- Tests run with `-race`.

## Notes

- Phase 2.3 introduced the first external runtime dependency
  (`github.com/go-telegram/bot v1.21.0`, MIT, zero-transitive).
- The two-tier directory layout (`internal/channel/` for the canonical
  contract, `internal/channels/telegram/` for the legacy pure-converter
  package) is a known transient inconsistency that closes when 2E.8
  lands.
