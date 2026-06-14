# Stage 02-EXT — Telegram full channel

> **Status:** closed (pending final review on `feat/2e8-webhook-lifecycle`)
> **Started:** 2026-06-13
> **Closed:** 2026-06-14

## Objective

Take the Telegram channel from the text-only converter shipped in
Phase 2.3 to a full `channel.Channel` implementation against
Telegram, covering every content kind the canonical Envelope models
plus the side-effect operations introduced in ADR-0006 / ADR-0007,
and finally the transport layer that actually receives Telegram
updates (webhook + polling) and dispatches them to the router from
Stage 3.

## Phases

| Phase  | Description                                                              | Status |
|--------|--------------------------------------------------------------------------|--------|
| 2E.1   | Inbound media (photo, voice, audio, video, document)                     | done   |
| 2E.2   | Outbound media                                                           | done   |
| 2E.3   | Locations (inbound + outbound)                                           | done   |
| 2E.4   | Buttons / keyboards (callback queries inbound, inline keyboard outbound) | done   |
| 2E.5   | Commands (bot_command MessageEntity → `Meta`)                            | done   |
| 2E.6   | Message editing + `Envelope.Operation`                                   | done   |
| 2E.7   | Reactions (inbound `PartType.Reaction` + outbound `OpSetReaction`)       | done   |
| 2E.8   | Webhook lifecycle + polling + `channel.Channel`                          | done   |

## Phase 2E.1 through 2E.7 — Pure-converter extensions

These seven phases are recapped in the per-ADR documents and in the
master document; their commits sit on master with full per-phase
coverage. Each shipped pure functions `InboundFromUpdate(*models.X)`
and / or extensions to `OutboundParams(*envelope.Envelope)`, with
JSON fixture tests and no network. The cumulative effect by the end
of 2E.7 was an adapter that could convert every Telegram update kind
Korvun models, in both directions, with full coverage of the side-
effect ADRs:

- **ADR-0004** envelope `PartType.Location`.
- **ADR-0005** `Envelope.Keyboard` + `PartType.Callback`.
- **ADR-0006** `Envelope.Operation` (`OpEditText`, `OpEditCaption`,
  `OpDelete`, `OpCallbackAck`).
- **ADR-0007** `PartType.Reaction` + `OperationKind.OpSetReaction`.

By 2E.7 the only missing piece was the transport itself: there was
no `*Adapter` implementing `channel.Channel`, no HTTP server, no
polling loop, no `Send` against `bot.Bot`. The whole pure-converter
arc was usable from tests but not from a real Korvun process.

## Phase 2E.8 — Webhook lifecycle (real reception + sending)

### Deliverables

- `docs/adr/0008-telegram-channel-lifecycle.md`, with status
  `accepted` after the user-approved corrections to the polling
  design once Context7 + source-read of `github.com/go-telegram/bot
  v1.21.0` revealed that `getUpdates` is unexported and the only
  public seam is `b.Start(ctx) + WithDefaultHandler`. Commits:
  - `1b62e64` — original ADR draft.
  - `986566c` — ADR accepted with the user's two amendments
    (polling restart behavior + Stage 12 observability hard
    dependency).
  - `d9f8c60` — polling design correction.
- `refactor(telegram): move internal/channels/telegram to
  internal/channel/telegram` (commit `e83fb59`). Closes the open
  debt from STAGE-02 §Open Debt by moving the package under the
  canonical singular path. Trivially safe: grep showed zero
  external imports of the legacy path before the move.
- The Phase 2E.8 implementation arc on
  `feat/2e8-webhook-lifecycle`, broken into five sub-phase
  commits so the diff is reviewable in pieces:
  - **sub-A — Adapter struct + Options + New** (`5fae774`).
    `Adapter` type with `cfg`, `client`, `inbound`, `dropped`;
    sixteen functional options covering token, mode, webhook URL
    / listen addr / path / secret token, allowed updates, inbound
    capacity, enqueue timeout, read-header timeout,
    drop-pending-on-start, TLS or reverse-proxy termination,
    logger, library escape hatch, plus the internal
    `withInjectedBotForTests`; per-mode validation in `validate`
    surfacing the eleven new sentinel errors; `Name()`,
    `Manifest()`, `Receive()`, `Mode()`, `DroppedCount()`. The
    `botClient` interface lifts the *bot.Bot methods Send and the
    webhook lifecycle need behind an interface so tests inject
    without an HTTPS round-trip.
  - **sub-B — Send dispatch** (`c0b7f2f`). `Adapter.Send` routes
    via `OutboundParams` and dispatches to the matching
    `botClient.Send*` / `Edit*` / `Delete*` / `AnswerCallback*` /
    `SetMessageReaction` method. Twelve test cases, one per
    `OutboundKind`. `capturingBotClient` records the params each
    method received and forces an error on demand for the
    error-wrap test.
  - **sub-C — `dispatchUpdate` with backpressure** (`33cefd1`).
    Per-update conversion + bounded enqueue with the
    `select{ inbound, ctx.Done, time.After(enqueueTimeout) }`
    rule fixed by ADR-0008 §4c. Silent-skip cases (`ErrNoMessage`,
    `ErrUnsupportedContent`) do not log; conversion errors emit a
    structured `WarnContext`; saturation drop logs the structured
    warning AND increments `dropped`. `conversation.id` populated
    from `telegram.chat_id` so the router (ADR-0003) can dispatch
    without knowing Telegram exists.
  - **sub-D — Webhook hand-rolled HTTP handler** (`3f3293a`).
    `webhookHandler() http.HandlerFunc` with constant-time secret
    token comparison via `crypto/subtle.ConstantTimeCompare`, 1
    MiB body cap via `io.LimitReader`, explicit 401 on auth
    failure (vs the library's silent rejection), 405 on non-POST,
    400 on JSON decode failure or oversized body, 200 OK on every
    accepted path including silent-skip and saturation drop. The
    saturation 200 is the deliberate operational choice argued
    in ADR-0008 §4c — a non-2xx would push Telegram into
    exponential backoff and risk webhook removal.
  - **sub-E — `Start`/`Stop` lifecycle** (`9413e4f`). Adapter-
    owned, all-or-nothing, idempotent (`ErrAlreadyStarted` /
    `sync.Once`-guarded `Stop`). ModeWebhook builds the mux,
    spawns `*http.Server` via `ListenAndServeTLS` (`WithTLS`) or
    plain `ListenAndServe` (`WithReverseProxyTermination`), calls
    `SetWebhook`; rollback on `SetWebhook` failure leaves no
    half-running state. ModePolling calls `DeleteWebhook` as a
    safety net then spawns the goroutine running
    `runner.Start(loopCtx)`. Stop reverses both, in the right
    order, then `close(a.inbound)` exactly once. Adapter `Stop()`
    is what `main.go` calls BEFORE `router.Shutdown()`.

### Behaviour fixed in Phase 2E.8

- **`channel.Channel` integration.** The adapter's `Send`,
  `Receive`, `Name` and `Manifest` are exactly the four methods the
  router calls. Lifecycle (`Start`, `Stop`) is owned by `main.go`
  per ADR-0008 §1, not by the router; the precedent matches
  `internal/channel/webhook`.
- **Polling delegates to the library** (`b.Start(ctx)` +
  `WithDefaultHandler(a.handleLibraryUpdate)`), reflecting the
  v1.21.0 reality that `getUpdates` is unexported. The
  `dispatchUpdate` body is shared with the webhook path, so there
  is one seam, one backpressure rule, and one drop counter across
  modes.
- **Webhook is hand-rolled** (constant-time secret token, explicit
  status codes, 1 MiB body cap), per the operational and security
  arguments in ADR-0008 §3.
- **Single saturation seam.** Both modes drop at the
  `dispatchUpdate → a.inbound` boundary after `enqueueTimeout`
  expires. The same `telegram_adapter_inbound_dropped_total`
  counter is incremented either way. ADR-0008 §4c includes the
  two-buffer diagram (library internal channel sized via
  `bot.WithUpdatesChannelCap(inboundCapacity)`, then `a.inbound`
  sized to the same default of 64).
- **Polling restart contract.** ADR-0008 §4d documents the
  at-least-once-on-restart bound (Telegram retains updates for up
  to 24 h until acknowledged via offset advancement). Persistent
  offset is a deferred Open Follow-Up that would use
  `bot.WithInitialOffset(n)` as the seam.
- **Shutdown ordering.** `adapter.Stop(ctx)` → `router.Shutdown(ctx)`
  → exit. Stop closes `inbound` exactly once after waiting for
  workers; the router treats the closed channel as "channel gone,
  drain".

### What this stage does NOT do

- **No `main.go` wiring yet.** Phase 2E.8 lands the adapter and the
  lifecycle; the executable that constructs it and registers it
  with the router lives in Stage 5+ territory.
- **No persistent polling offset.** At-least-once-on-restart
  bounded by Telegram's 24 h server-side buffer; a persistent
  offset is deferred to a follow-up ADR.
- **No ACME / TLS automation.** Adapter expects either a TLS
  keypair via `WithTLS` or a reverse proxy in front via
  `WithReverseProxyTermination`. ACME is a bootstrap-stage
  decision.
- **No multi-channel-on-one-port.** A future `Handler() http.Handler`
  method would let `main.go` mount the webhook handler on a shared
  mux. Deferred until a second webhook-flavored channel ships in
  the same process.
- **No production metric exposure.** The `dropped` atomic is
  surfaced via `DroppedCount()`. Prometheus exposition + alert
  wiring is a **hard dependency** carried by Stage 12
  (observability) — ADR-0008 §Open follow-ups records this
  explicitly because drop-on-saturation without a working alert
  path is silent data loss.

### Workflow compliance

| Step                         | Result                                                                 |
|------------------------------|------------------------------------------------------------------------|
| External docs verified first | Context7 + `go doc` for `github.com/go-telegram/bot v1.21.0`; the source-read that uncovered the `getUpdates` private seam triggered an ADR correction BEFORE any code was written against the bad assumption |
| ADR written before code      | ADR-0008, then accepted, then corrected before implementation started  |
| TDD red before green         | Each sub-phase (A–E) was a red-then-green cycle landing as one commit  |
| `make quality` green         | yes, after every sub-phase, with `-race`                               |
| Stage doc updated            | this document                                                          |

### Quality gate (end of 2E.8 on `feat/2e8-webhook-lifecycle`)

| Package                            | Coverage |
|------------------------------------|----------|
| `internal/channel`                 | 100.0%   |
| `internal/channel/telegram`        | **90.6%** (target 90% met) |
| `internal/channel/webhook`         | 91.4%    |
| `internal/envelope`                | 97.0%    |
| `internal/router`                  | 96.3%    |
| **total**                          | **93.2%** |

`make quality` green with `-race`. Tests use only `httptest` and
in-test fakes; no real Telegram credentials or network round-trips.

## Closure

- ADRs that drove this stage: ADR-0001 (Telegram client choice),
  ADR-0004 (`Location`), ADR-0005 (`Keyboard` + `Callback`),
  ADR-0006 (`Operation`), ADR-0007 (`Reaction`), ADR-0008
  (channel lifecycle).
- `internal/channels/telegram/` path is gone; the canonical path
  is `internal/channel/telegram/`.
- `make quality` green over the whole tree at 93.2% coverage with
  the telegram package above the 90% per-package target.
- `feat/2e8-webhook-lifecycle` is the branch holding the Phase
  2E.8 implementation. Master holds ADR-0008 and the rename; the
  branch holds the lifecycle. Merge to master pending the final
  end-of-stage review.

## Notes

- The decision to ship both `ModePolling` (default) and
  `ModeWebhook` from the same phase is grounded in MASTER.md's
  "from Raspberry Pi to the cloud" range: polling needs only
  outbound HTTPS to `api.telegram.org`, webhook needs public
  HTTPS ingress.
- The asymmetry "polling delegates to the library, webhook is
  hand-rolled" is a v1.21.0 fact of life argued in ADR-0008 §A3.
  A future library version that exposes a public `GetUpdates`
  seam may unify the two paths via a follow-up ADR.
- `Stage 3` (router) was specified before Phase 2E.8 was
  implemented and continues to be the consumer of
  `channel.Channel`. The router has not changed in this stage;
  the adapter just became the first real channel it can speak to.