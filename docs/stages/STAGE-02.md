# Stage 02 — Channels

> **Status:** open
> **Started:** 2026-06-13
> **Closed:** —

## Objective

Build the messaging-gateway side of Korvun: a set of channel adapters
that translate native channel payloads (Telegram, etc.) into the
canonical Envelope, and vice versa, with no business logic leaking into
the adapter and no transport coupling forced on the Envelope.

## Phases

| Phase | Description                            | Status |
|-------|----------------------------------------|--------|
| 2.1   | Channel port and design (Master Plan)  | done   |
| 2.2   | Channels inventory and selection       | done   |
| 2.3   | Telegram adapter (inbound + outbound)  | done   |

## Phase 2.3 — Telegram Adapter

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
  table-driven, table-row coverage of nominal and error paths for both
  conversions.
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
`bot.SendMessage` by the same caller. This is what made TDD possible
without a real Telegram token.

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

### Phase 2.3 Workflow Compliance

| Step                         | Result                                   |
|------------------------------|------------------------------------------|
| External docs verified first | Context7 (`/go-telegram/bot`), source repo |
| ADR written before code      | `docs/adr/0001-telegram-client.md`       |
| TDD red before green         | Tests committed in a separate red commit |
| `make quality` green         | yes                                      |
| Stage doc updated            | this document                            |

### Quality Gate

- `make quality`: pass
- Coverage: `internal/channels/telegram` 100.0%, `internal/envelope`
  97.8%, overall 98.8%
- Tests run with `-race`
- gosec G304 explicitly justified in `loadUpdateFixture` (test-only
  path read)

## Key Decisions

- ADR-0001 — Telegram client library choice.

## Notes

- Phase 2.3 is the first phase to bring an external runtime dependency
  into the binary. The dependency is zero-transitive, license MIT
  (Apache-2.0 compatible), and recorded in the SBOM artefact generated
  by Stage 0.3.
- Subsequent Telegram phases (media parts, edits, callback queries,
  webhook lifecycle) will extend this package without rewriting the
  conversion contract.
