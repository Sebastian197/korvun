# ADR-0001: Telegram Bot client library — `github.com/go-telegram/bot`

> **Status:** accepted
> **Date:** 2026-06-13
> **Deciders:** Sebastián Moreno Saavedra

## Context

Stage 2 (Channels) requires an adapter that:

1. Parses incoming Telegram `Update` payloads into the canonical Envelope.
2. Builds outbound Telegram messages from an Envelope.

Telegram Bot API is a HTTP+JSON API. Implementing a full client from the
standard library would force Korvun to track Telegram's evolving schema
(currently Bot API 10.0, dozens of struct types) just to convert payloads.
That work is repetitive and brittle, so a maintained binding is preferred
to a hand-rolled client — provided the binding does not drag in a wide
dependency tree (Korvun should remain a single static binary with a
minimal supply chain) and is license-compatible with the Korvun
distribution (Apache-2.0).

Korvun's internal Channel port (Stage 2) expects two pure conversion
functions per channel: `Inbound: native → Envelope` and
`Outbound: Envelope → native`. The chosen library must therefore expose
the native incoming/outgoing types as exported, JSON-serialisable structs,
and must allow update processing to be driven by an externally supplied
payload (long-poll, webhook handler, or in-memory test fixture). A client
that hides updates behind an internal channel/goroutine forces Korvun to
adopt that lifecycle and complicates TDD.

## Decision

Adopt **`github.com/go-telegram/bot`** at version **`v1.21.0`** as the
sole Telegram client dependency for Phase 2.3.

The adapter uses only the model and params types — not the HTTP client —
exposing two pure functions:

- `InboundFromUpdate(*models.Update) (envelope.Envelope, error)`
- `OutboundToSendMessage(envelope.Envelope) (*bot.SendMessageParams, error)`

The transport lifecycle (token, long-polling, webhook listener) is a
separate concern handled in a later phase, but the chosen library
already provides `bot.ProcessUpdate(ctx, *models.Update)`, which lets
the same adapter type be reused under any transport without rewriting.

## Comparison of Candidates

| Criterion                       | `go-telegram/bot` (chosen) | `go-telegram-bot-api/telegram-bot-api` v5 | `ovyflash/telegram-bot-api`        |
|---------------------------------|----------------------------|-------------------------------------------|------------------------------------|
| License                         | MIT (compatible Apache-2.0)| MIT (compatible Apache-2.0)               | MIT (compatible Apache-2.0)        |
| Latest tagged release           | **v1.21.0 — 2026-05-22**   | v5.5.1 — Dec 2021 (~4.5 years stale)      | v9.4.0 — 2026-02-19                |
| Bot API coverage                | **10.0 (current)**         | ~6.x (lags upstream)                      | 10.0 (current)                     |
| Module Go version               | 1.18                       | 1.16                                      | 1.23                               |
| Direct deps (`go.mod require`)  | **0**                      | 0                                         | 0                                  |
| Transitive deps                 | **0** (README: "zero-dependencies framework") | 0 | 0                                  |
| Context7 doc snippets / score   | **119 / 76.9**             | 28 / 59.8                                 | 63 / 35.0                          |
| Update parsing decoupled from transport | **Yes** — `bot.ProcessUpdate(ctx, *Update)` accepts any source | Partial — `UpdatesChannel` is the documented entry point | Same as v5 (fork) |
| Channel port fit                | **High** — pure parser usage idiomatic | Medium — pushed toward built-in long-poll loop | Medium — same as v5 |

### Why the chosen library wins each axis

- **License (all equal).** Three candidates are MIT, which is permissive
  and compatible with redistributing Korvun under Apache-2.0. No tie-break
  here, but worth confirming explicitly so the project's license-audit
  trail is clean.

- **Recent maintenance.** `go-telegram-bot-api/telegram-bot-api` last
  *tagged* a release in **December 2021** (`v5.5.1`); its `master`
  branch still receives commits but downstream consumers depending on
  SemVer pin to a 4.5-year-old tag that predates Bot API 7.x/8.x/9.x/10.x.
  `ovyflash` is a current fork (Feb 2026) and `go-telegram/bot` is even
  more recent (May 2026) — both close that gap; preferring the original
  project (`go-telegram/bot`) over a fork avoids supply-chain risk from
  a single-maintainer fork's bus factor.

- **Transitive dependencies.** All three declare zero `require` entries
  in `go.mod`, so SBOM impact is identical. The tie-break goes to the
  library whose maintainers *advertise* zero-deps as a project invariant
  (`go-telegram/bot` README); that is a forward-looking commitment, not
  just today's state.

- **Fit with Korvun's Channel port.** The decisive axis.
  `go-telegram/bot` exposes `bot.ProcessUpdate(ctx, *models.Update)` as
  a public, documented entry point for feeding pre-parsed updates from
  *any* source. That maps one-to-one onto Korvun's
  `Inbound: native → Envelope` interface: the transport produces the
  `*models.Update`, the adapter converts it to an Envelope, the
  in-memory test feeds a fixture instead of a real HTTP body — same
  code path. The legacy and forked libraries route updates through an
  `UpdatesChannel` driven by `GetUpdatesChan`, which is convenient when
  the library owns the lifecycle but is awkward when Korvun does.

## Consequences

- Adds one external Go module (`github.com/go-telegram/bot v1.21.0`) and
  no transitive runtime dependencies. SBOM impact: +1 entry.
- Adapter logic stays pure, so the adapter test suite needs no network
  and remains fast and deterministic.
- The Korvun Envelope is the source of truth: every Telegram-specific
  field not modelled by Envelope is preserved in `Envelope.Meta` rather
  than forced into the type.
- Apache-2.0 redistribution requires retaining MIT attribution for this
  dependency. Tracked via the SBOM artefact generated by Stage 0.3.

## Alternatives Considered (summary)

- **Hand-rolled HTTP+JSON client.** Rejected: re-implements Telegram's
  schema with no upside; high maintenance cost as Bot API evolves; no
  Context7 coverage; would defeat the point of having an ADR for an
  external dependency.
- **`github.com/go-telegram-bot-api/telegram-bot-api/v5`.** Rejected
  primarily on staleness of the last *tagged* release (Dec 2021,
  ~4.5 years old, pre Bot API 7.x→10.x) and on its lifecycle-owning
  design clashing with Korvun's Channel port. See comparison table.
- **`github.com/ovyflash/telegram-bot-api`.** Rejected on supply-chain
  grounds (single-maintainer fork increases bus-factor risk) and on the
  same lifecycle-owning design inherited from the original library.
