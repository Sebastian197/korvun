# ADR-0008: Telegram channel lifecycle — webhook + long-polling, transport ownership, integration with router shutdown

> **Status:** accepted
> **Date:** 2026-06-14
> **Deciders:** Sebastián Moreno Saavedra

## Context

Phase 2E.8 of Stage 2-EXT is the last phase of the Telegram-completion
plan. It is structurally distinct from 2E.1–2E.7: those phases extended
the pure converters from text to media, locations, keyboards, commands,
edits and reactions, and each addition was a function from `*models.X`
to `*envelope.Envelope` (or the reverse). They never touched the
network, never opened a port, never owned any goroutine.

Phase 2E.8 introduces the missing transport — the code that actually
receives Telegram updates, validates them at the HTTP boundary, runs
them through `InboundFromUpdate`, and emits canonical Envelopes through
the `channel.Channel` contract; plus the Send side that takes Envelopes
and calls the appropriate `bot.Bot.SendXxx` method against Telegram's
HTTPS API. It is also the phase that pays back the package-rename debt
documented in STAGE-02 (move `internal/channels/telegram/` to
`internal/channel/telegram/`).

Five structural questions must be pinned before any lifecycle code
lands, because each one shapes APIs that the router and the main
process will rely on for the rest of the project's life:

1. What is the shape of `channel.Channel` for Telegram? Specifically,
   how do `Receive` and `Send` (from Phase 2.1) fit together with a
   transport that is fundamentally request/response (webhook) or
   polling-driven (`getUpdates`)?
2. Which transports does Phase 2E.8 implement — webhook only, polling
   only, or both?
3. How is the webhook secret token
   (`X-Telegram-Bot-Api-Secret-Token`) configured and validated?
4. What is the HTTP-server lifecycle (startup, registration with
   Telegram, graceful shutdown), and how does it integrate with the
   per-brain isolation + drain that ADR-0003 fixed for the router?
5. The legacy `internal/channels/telegram/` path needs to move to the
   canonical `internal/channel/telegram/` (open debt tracked in
   STAGE-02 §Open Debt). Does the rename land inside Phase 2E.8, and
   as one commit or separately?

### External verification (per CLAUDE.md non-negotiable)

`github.com/go-telegram/bot v1.21.0` was queried via Context7 before
this ADR was written. Relevant API facts confirmed at that version:

- **Polling mode (library-driven).** `func (b *Bot) Start(ctx
  context.Context)` blocks until ctx is cancelled. Updates flow through
  `WithDefaultHandler` callbacks the library invokes from its internal
  worker pool.
- **Polling primitive (caller-driven).** `func (b *Bot) GetUpdates(ctx,
  *bot.GetUpdatesParams) ([]*models.Update, error)` exposes the same
  `getUpdates` API method as a one-shot call, callable without going
  through `b.Start`. Params include `Offset`, `Timeout`,
  `AllowedUpdates`.
- **Webhook mode (library-driven).** `func (b *Bot) WebhookHandler()
  http.HandlerFunc` returns an HTTP handler that validates the
  `X-Telegram-Bot-Api-Secret-Token` header against the secret given
  to `bot.WithWebhookSecretToken(secret)`, parses the body into a
  `*models.Update`, and puts it on the library's internal updates
  channel. The library's secret-token check **returns silently** when
  validation fails — Telegram is not told. `func (b *Bot)
  StartWebhook(ctx context.Context)` is the worker pool that drains
  that internal channel and calls `WithDefaultHandler`.
- **Webhook registration with Telegram.** `b.SetWebhook(ctx,
  &bot.SetWebhookParams{URL, SecretToken, AllowedUpdates,
  DropPendingUpdates, MaxConnections, IPAddress, Certificate})`
  registers the bot's webhook URL with Telegram. `b.DeleteWebhook(ctx,
  &bot.DeleteWebhookParams{DropPendingUpdates})` unregisters it.
- **Direct dispatch into the library.** `bot.ProcessUpdate(ctx,
  *models.Update)` accepts a pre-parsed update from any source.
  ADR-0001 deliberately picked this library because of this seam.

The application owns the actual `*http.Server` either way: the library
provides the handler and the worker pool, but it does not bind a
listener on its own.

### Router context, recapped

Stage 3 (ADR-0003) fixed the router contract that Phase 2E.8 must wire
the Telegram adapter into:

- The router consumes `channel.Channel`. It calls `Name()`,
  `Manifest()`, `Send(ctx, *Envelope)`, and `Receive(ctx)` once per
  registered channel. It does **not** call any other method, so any
  lifecycle (start, stop, set webhook, delete webhook) is the
  adapter's own concern, driven by `main.go`.
- `router.Shutdown(ctx)` cancels the router's internal context, closes
  brain queues, drains in-flight handlers up to `ctx`, then returns.
  Phase 3.2 added a per-channel outbound queue per channel; one slow
  channel does not affect siblings.
- An adapter that keeps producing updates after `router.Shutdown` will
  see `DispatchInbound` return `ErrShutdown`. The cleanest pattern is
  therefore "stop the inflow first": adapter halts inbound, then
  router drains, then process exits.

## Decision

### 1. `channel.Channel` shape for Telegram — owned buffered inbound channel, transport-driven writers, lifecycle off-interface

The Telegram `*Adapter` holds **one buffered `chan *envelope.Envelope`**
(`DefaultInboundCapacity = 64`, identical to ADR-0003's per-brain queue
default and to the webhook adapter's existing buffer). Exactly one of
two writers feeds it, depending on the configured mode:

- **Webhook mode**: a hand-rolled `http.HandlerFunc` decodes the
  request body into `*models.Update`, runs `InboundFromUpdate`, and
  writes the resulting Envelope to the channel with bounded
  backpressure:

  ```go
  select {
  case a.inbound <- env:
      w.WriteHeader(http.StatusOK)
  case <-ctx.Done():
      // server is shutting down; ack so Telegram doesn't retry
      w.WriteHeader(http.StatusOK)
  case <-time.After(a.enqueueTimeout): // DefaultEnqueueTimeout = 250 ms
      // Korvun is saturated. Acknowledge anyway to avoid Telegram's
      // exponential retry storm; emit a structured warning instead.
      // (See "Backpressure under saturation" below.)
      a.logSaturation(ctx, env)
      w.WriteHeader(http.StatusOK)
  }
  ```

- **Polling mode**: a long-running goroutine loops on `bot.Bot.GetUpdates(ctx,
  &bot.GetUpdatesParams{Offset: a.nextOffset, Timeout: a.pollTimeoutSeconds,
  AllowedUpdates: a.allowedUpdates})`, dispatches each
  `*models.Update` to `InboundFromUpdate`, and writes to the channel
  with the same `select` shape. The polling goroutine carries its own
  offset bookkeeping in memory.

`Receive(ctx context.Context) (<-chan *envelope.Envelope, error)` is a
thin accessor: it returns the same buffered channel on every call, and
never errors. The `ctx` argument is **not** the lifecycle context (that
belongs to `Start`/`Stop`) — it is the router's own context, and
`Receive` deliberately does not bind to it. This matches the precedent
already set by `internal/channel/webhook/webhook.go`.

Lifecycle control lives on the adapter, NOT on the `channel.Channel`
interface:

```go
func New(opts ...Option) (*Adapter, error)         // build + validate config
func (a *Adapter) Start(ctx context.Context) error // open listener, set webhook, or start polling
func (a *Adapter) Stop(ctx context.Context) error  // graceful drain + delete webhook + close inbound
```

The `Adapter` struct (sketch — the implementation will refine field
shapes and naming):

```go
type Adapter struct {
    // Configuration (immutable after New).
    token             string
    mode              Mode             // ModePolling | ModeWebhook
    webhookURL        string           // ModeWebhook only — public HTTPS URL
    listenAddr        string           // ModeWebhook only — e.g. ":8443"
    webhookPath       string           // ModeWebhook only — defaults to "/telegram/webhook"
    secretToken       string           // ModeWebhook only — required if webhookURL is set
    allowedUpdates    []string         // both modes; empty = library default
    pollTimeoutSec    int              // ModePolling only — defaults to 30
    inboundCapacity   int              // both modes; defaults to 64
    enqueueTimeout    time.Duration    // both modes; defaults to 250 ms
    logger            *slog.Logger

    // Transport state (owned by Start/Stop).
    bot               *bot.Bot         // outbound HTTP client + SetWebhook/DeleteWebhook caller
    httpServer        *http.Server     // ModeWebhook only
    inbound           chan *envelope.Envelope
    nextOffset        int              // ModePolling only
    started           atomic.Bool
    stopOnce          sync.Once
    workers           sync.WaitGroup   // joins the polling goroutine, or pending request handlers
}
```

The Adapter satisfies `channel.Channel` via `Name()` (returns
`telegram.ChannelName` = `"telegram"`), `Manifest()` (lists the
content kinds the converters already support: text, image, audio,
video, buttons; file is also true), `Receive(ctx)` (returns the
buffered inbound channel), and `Send(ctx, env)` (dispatches via the
existing `OutboundParams(env)` → `*Outbound` tagged union, then calls
the matching `bot.Bot.SendXxx` method).

### 2. Both webhook AND polling, chosen at construction time

Phase 2E.8 ships **both** transports, gated by a `Mode` option:

```go
type Mode int
const (
    ModePolling Mode = iota + 1  // default
    ModeWebhook
)
```

The default is `ModePolling`. Three concrete reasons drove this:

- **MASTER.md positioning is explicit: "from Raspberry Pi to the
  cloud".** A self-hosted Pi behind a residential NAT almost never has
  a stable public HTTPS endpoint without a separate tunnel
  (Cloudflare, ngrok, Tailscale Funnel). Webhook would force every
  small-scale operator to set up reverse-proxy plumbing before Korvun
  could receive a single message. Polling has no such prerequisite —
  it works behind any NAT, on any port, with only outbound HTTPS to
  `api.telegram.org`.
- **The cost of shipping polling on top of webhook is small.** Both
  modes share Send, both share `InboundFromUpdate`, both share the
  buffered inbound channel and its backpressure rules. Only the input
  side of the adapter differs (an HTTP listener vs. a `GetUpdates`
  loop). Concretely: ~150 LOC of polling, vs. ~250 LOC of webhook +
  HTTP lifecycle. The duplicated cost is negligible relative to the
  flexibility gained.
- **Polling is the easier mode to TDD.** The polling loop can be
  tested with an in-test `bot.Bot` mock that returns a pre-canned
  slice of `*models.Update`. No `httptest.Server`, no certificate
  ceremony, no port-bind. The webhook tests reuse `httptest.Server`
  on top of the same adapter code path. So shipping both also keeps
  the test surface manageable.

Operationally, the recommendation is **webhook for production
(cloud), polling for development and self-hosted edge devices**. Both
read from the same buffered channel, both call the same `Send`, both
are stopped by the same `Stop(ctx)`. Switching modes is a config
change, not a code change.

### 3. Secret token — constant-time comparison, hand-rolled handler, explicit 401 on failure

The webhook HTTP handler is **hand-rolled**, not delegated to
`bot.Bot.WebhookHandler`. The library handler is fine on the happy
path, but on the rejected path it (a) compares the header value with
plain `==`, which is timing-attack territory, and (b) responds
silently rather than with an explicit HTTP status — operationally
invisible. Both are easy to fix by owning the handler:

```go
func (a *Adapter) webhookHandler() http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
            w.WriteHeader(http.StatusMethodNotAllowed)
            return
        }
        got := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
        if subtle.ConstantTimeCompare([]byte(got), []byte(a.secretToken)) != 1 {
            a.logger.WarnContext(r.Context(),
                "telegram: rejected webhook with invalid secret token",
                "remote_addr", r.RemoteAddr)
            w.WriteHeader(http.StatusUnauthorized)
            return
        }
        body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBodyBytes))
        if err != nil {
            w.WriteHeader(http.StatusBadRequest)
            return
        }
        var u models.Update
        if err := json.Unmarshal(body, &u); err != nil {
            w.WriteHeader(http.StatusBadRequest)
            return
        }
        env, err := InboundFromUpdate(&u)
        if err != nil {
            if errors.Is(err, ErrNoMessage) || errors.Is(err, ErrUnsupportedContent) {
                // Not interesting upward; ack so Telegram doesn't retry.
                w.WriteHeader(http.StatusOK)
                return
            }
            a.logger.WarnContext(r.Context(),
                "telegram: failed to convert update", "error", err)
            w.WriteHeader(http.StatusOK) // still ack — see saturation rule
            return
        }
        env.Meta[router.MetaConversationID] = env.Meta[telegram.MetaChatID]
        a.enqueueInbound(r.Context(), env, w)
    }
}
```

Three points worth pinning explicitly:

- **`crypto/subtle.ConstantTimeCompare`** is used so that an attacker
  brute-forcing the secret token cannot extract it via response-time
  side channels. The comparison is on byte slices; the slices may
  legitimately have different lengths (e.g. empty header), which
  `ConstantTimeCompare` handles by returning 0 — exactly the
  "reject" path. `crypto/subtle` is stdlib and adds zero dependencies.
- **`maxWebhookBodyBytes = 1 << 20` (1 MiB)** caps the body so a
  malicious or buggy peer cannot exhaust memory by streaming an
  arbitrarily large payload. Telegram's documented update payloads
  are well below this; an actual Telegram update will never come
  close.
- **`conversation.id` is set at this seam.** The Telegram-specific
  Meta key (`telegram.chat_id`) is already populated by
  `InboundFromUpdate`. ADR-0003 §Conversation correlation requires
  `Meta["conversation.id"]` to be non-empty; the adapter copies the
  chat ID into that canonical key here, so the router can dispatch
  without knowing Telegram exists. The same copy happens in the
  polling-mode dispatcher.

For `Mode == ModeWebhook`, the constructor refuses an empty
`secretToken` with `ErrMissingSecretToken`. There is no way to run
webhook mode without one — the secret is the only authentication
between Telegram and Korvun, and "no auth" is not a supported
deployment.

### 4. HTTP lifecycle and integration with router shutdown

#### 4a. `Start(ctx context.Context) error`

`Start` is idempotent on `already started` (returns `ErrAlreadyStarted`),
and bounded by the caller's `ctx`. Per mode:

- **`ModeWebhook`**:
  1. Construct a `*http.ServeMux`; mount `a.webhookHandler()` at
     `a.webhookPath`; mount a small `/healthz` reporting OK only
     after the adapter is `started`.
  2. Construct `a.httpServer = &http.Server{Addr: a.listenAddr, Handler: mux,
     ReadHeaderTimeout: 5 * time.Second}`. `ReadHeaderTimeout` is set
     to defuse Slowloris-style attacks; the rest of the timeouts can
     be tuned in a later phase.
  3. Spin up a goroutine that runs `a.httpServer.ListenAndServeTLS(certFile,
     keyFile)` (or `ListenAndServe` if `WithReverseProxyTermination()`
     is opted into — see Option `B`-class items below). Telegram
     requires HTTPS for webhooks, but TLS termination at a reverse
     proxy is the common production pattern, and the adapter must
     support both. The error from `ListenAndServe[TLS]` is reported
     via the logger and the adapter transitions to a failed state;
     `Stop` is still safe to call.
  4. Call `a.bot.SetWebhook(ctx, &bot.SetWebhookParams{URL:
     a.webhookURL, SecretToken: a.secretToken, AllowedUpdates:
     a.allowedUpdates, DropPendingUpdates: a.dropPendingOnStart})`.
     If the call fails, the adapter shuts down the just-opened HTTP
     server and returns the error from `Start`. This makes "Start
     failed" mean "no part of the transport survived" — a clean
     all-or-nothing semantic.

- **`ModePolling`**:
  1. Construct `a.bot` (already done in `New` actually).
  2. Spawn the polling goroutine. The goroutine carries its own ctx
     derived from a new background context that the adapter cancels in
     `Stop`. The ctx passed to `Start` is used only for the initial
     `SetWebhook(nil)` / `DeleteWebhook(...)` call (see below), not
     for the steady-state loop — the steady-state loop must outlive
     the `Start` ctx by definition.
  3. As a safety net, call `b.DeleteWebhook(ctx,
     &bot.DeleteWebhookParams{DropPendingUpdates: false})` once
     before the loop starts. This ensures polling never silently
     fights an old webhook registration left over from a previous
     deployment (Telegram returns 409 to `getUpdates` if a webhook is
     still active). On error, the call is logged and the loop starts
     anyway — `getUpdates` will surface the conflict explicitly.

In both modes, `Start` returns once the transport is established and
the inbound producer is running. It does **not** block on traffic.

#### 4b. `Stop(ctx context.Context) error`

`Stop` is the reverse of `Start`, also idempotent (`sync.Once`),
bounded by `ctx`:

- **`ModeWebhook`**:
  1. `_ = a.bot.DeleteWebhook(ctx, &bot.DeleteWebhookParams{DropPendingUpdates:
     false})` — tell Telegram to stop sending us anything. Errors
     logged but not returned (we still want to shut down).
  2. `_ = a.httpServer.Shutdown(ctx)` — stop accepting new
     connections, let in-flight handlers complete up to `ctx`. The
     returned error is logged but not returned, again so a slow
     shutdown does not mask cleanup downstream.
  3. Wait on `a.workers.Wait()` to ensure no handler is still mid-
     enqueue.
  4. `close(a.inbound)` — signal "no more updates" to the router.

- **`ModePolling`**:
  1. Cancel the polling goroutine's ctx.
  2. Wait on `a.workers.Wait()`.
  3. `close(a.inbound)`.

The router's `Phase 3.2 Shutdown` already handles a closed channel on
the inbound side: the per-brain workers drain the buffer, then exit
when the next `Receive` read fails. So Korvun's overall shutdown
sequence reads:

```text
SIGTERM  →  adapter.Stop(ctx1)  →  router.Shutdown(ctx2)  →  exit
              (stop inflow,       (drain in-flight,
               drain handlers,     return errors via hook)
               close inbound)
```

`main.go` is responsible for owning that order. The two contexts
`ctx1` / `ctx2` can share a deadline budget (e.g. 10 s total, 5 s
each) so a misbehaving brain can't pin the process forever. ADR
notwithstanding, the actual budget is a tuning knob for the
deployment, not a contract.

#### 4c. Backpressure under saturation

When the inbound buffer is full *and* the 250 ms enqueue timeout
elapses, the rule is **acknowledge and warn**, not block or 5xx. Why:

- Telegram retries webhooks with exponential backoff on any non-2xx
  response and after a few failures may **drop the bot's webhook**
  entirely until manual `SetWebhook` is called again — a self-inflicted
  outage. 4xx/5xx is the wrong tool here; the right operational signal
  is structured logs (`slog.Warn`) and a metric counter
  (`telegram_adapter_inbound_dropped_total`).
- The router's `ErrBrainSaturated` is for the brain side: when there
  is no brain that can take this conversation, that's a hard error.
  The inbound buffer being full is a softer, transient signal —
  Korvun was healthy a moment ago and will be again. The right
  response is to surface it for monitoring, not to break the contract
  with Telegram.

Polling mode applies the same rule by symmetry: the polling loop
*drops* the converted Envelope after `enqueueTimeout`, increments
the same counter, logs, and moves on to the next update. The next
`GetUpdates` call uses the advanced `Offset`, so the dropped update
is **not** re-fetched. This is consistent: the adapter's contract is
"best-effort delivery from Telegram to the Envelope channel under
the configured buffer", and saturation is a misconfiguration, not a
data-loss bug.

The "drop and warn" rule is only operationally safe if the warn side
is actually observed. The counter
`telegram_adapter_inbound_dropped_total` is emitted via `slog` in
Phase 2E.8 with sufficient structured fields to be scraped into a
Prometheus gauge later; turning that scrape into a paging alert is a
**hard prerequisite** carried by Stage 12 (observability) before any
production deployment relies on `ModeWebhook` at non-trivial volume.
The dependency is recorded as such in §Open follow-ups; a deployment
that runs Korvun at scale without the observability layer in place is
running with silent data loss masquerading as silent success.

#### 4d. Polling-mode restart behavior — at-least-once bounded by Telegram's 24 h server-side buffer

`ModePolling` keeps `nextOffset` in memory only. On a Korvun restart
`nextOffset` is reset to 0 and the first `GetUpdates` call asks
Telegram for the oldest unacknowledged update. Per the Telegram Bot
API, updates are retained server-side for **up to 24 hours** until
acknowledged — calling `GetUpdates` with `Offset = N` acts as the
acknowledgement of every update with `update_id < N`. The four
restart cases therefore are:

- **Clean restart with no unprocessed updates pending.** No
  re-delivery; the first post-restart `GetUpdates` returns nothing
  until a new user action.
- **Restart with updates Telegram had already delivered but for
  which `nextOffset` had not yet advanced past them on a subsequent
  poll.** Telegram redelivers them. Because `InboundFromUpdate` is
  pure, the resulting Envelope is byte-identical to the one the
  previous process produced — downstream side-effect handlers MUST
  be idempotent or guarded by the conversation/state layer
  (Stage 7+) to avoid double-execution.
- **Restart with updates in flight at the time of SIGTERM.** Same
  as above: Telegram redelivers anything not yet acknowledged via
  the next `Offset` bump.
- **Long outage (process down for more than 24 h).** Telegram drops
  updates older than the retention window. The Bot API does not
  surface this as an error; the long outage simply manifests as "no
  historical updates returned". Operationally, a Korvun deployment
  unattended for > 24 h is expected to lose messages from the early
  part of the outage. This is upstream behavior, not a Korvun
  decision.

The operational guarantee in plain terms is **"best-effort
exactly-once shape, at-least-once delivery on restart, no delivery
beyond 24 h of downtime"**. It is deliberately surfaced here so
operators understand the boundary rather than learn it from
production surprises.

`ModeWebhook` has a different restart profile: Telegram retries the
webhook with exponential backoff on the HTTP transport layer for a
bounded window, and Korvun acknowledges with 200 OK at the HTTP
boundary. A restart loses requests Telegram had open at the moment
of SIGTERM but had not yet received a 200 OK for; Telegram's retry
behavior recovers them within the retry budget.

Both modes inherit the §4c "saturation → drop + warn" rule, so
sustained overload can lose individual updates in either mode. The
two behaviors are independent: restart-replay is bounded by
Telegram's 24 h server-side buffer (polling) or the webhook retry
budget (webhook); saturation drops are unbounded but emit a counter.
A persistent `nextOffset` store would tighten the polling-mode bound
toward exactly-once at the cost of one I/O per poll-batch; see
§Open follow-ups.

### 5. Package rename — separate refactor commit, performed first in Phase 2E.8

`internal/channels/telegram/` (plural) moves to
`internal/channel/telegram/` (singular) in **one commit, at the start
of Phase 2E.8, before any new code**. The rename ships as part of
Phase 2E.8's closure, not as a separate phase, but it is its own commit
so a reviewer can read the lifecycle diff without the rename noise.

The rename is verifiably safe and trivial because the current package
has **zero external imports** in the repository (`grep -rn
"internal/channels/telegram" --include="*.go"` returns nothing
outside the package's own directory). The package's exported identifiers
(`InboundFromUpdate`, `OutboundParams`, `ChannelName`, the `Meta*`
constants, the sentinel errors) are only referenced by the package's
own tests, which use bare `package telegram` and do not import the
canonical path. A `git mv` over the directory plus a directory-level
`gofmt -w .` is sufficient — no import-statement edits.

The commit shape:

```text
refactor(telegram): move internal/channels/telegram to internal/channel/telegram
```

Followed by the regular Phase 2E.8 commit sequence (red tests, green
implementation, docs).

## Consequences

### What this enables

- The Telegram adapter becomes the **first** `channel.Channel`
  implementation backed by a real transport in master. Stage 3's
  router, which has been exercised only against test fakes, can be
  wired up end-to-end with a Telegram bot from Phase 2E.8 forward.
  `internal/channel/webhook/webhook.go` was previously the only
  `channel.Channel` shipped, but it is generic JSON-over-HTTP, not
  bound to any concrete provider.
- Operators can run the same binary against Telegram in two
  deployment shapes — webhook for cloud, polling for the Pi — without
  rebuilding, just by changing config.
- The router's `Phase 3.2` isolation guarantees (slow channel doesn't
  block siblings, slow brain doesn't block sibling brains) extend to
  Telegram naturally: outbound goes through the router's per-channel
  outbound queue, inbound respects the same backpressure shape.

### What this asks of `main.go` and the bootstrap

`main.go` (forthcoming, Stage 5 territory) must:

1. Construct the adapter with `New(opts...)`.
2. Register the adapter with the router via `router.RegisterChannel("telegram", adapter)`.
3. Call `adapter.Start(ctx)` once (and bail if it errors).
4. On shutdown: call `adapter.Stop(ctx)` BEFORE `router.Shutdown(ctx)`,
   so the inflow stops before the router begins to drain. Phase 2E.8
   updates the relevant docs in `STAGE-02-EXT.md` to record this
   ordering as the canonical pattern for any future stateful channel.

### What this does NOT do

- **No webhook autodiscovery / no built-in TLS provisioning.** Phase
  2E.8 expects `WithWebhookURL("https://...")`, `WithListenAddr(":8443")`,
  and either a TLS keypair or a `WithReverseProxyTermination()` opt-in.
  An ACME (Let's Encrypt) integration is a later stage's concern.
- **No persistent polling offset.** See §4d for the precise
  restart-behavior contract: at-least-once delivery on restart,
  bounded by Telegram's 24 h server-side retention. A persistent
  offset store is out of scope for Phase 2E.8.
- **No `bot.Bot.Start` / `bot.Bot.StartWebhook` / `bot.Bot.WebhookHandler`
  usage.** The library is used for outbound (`SendMessage`, `SendPhoto`,
  etc.), for webhook lifecycle calls (`SetWebhook`, `DeleteWebhook`),
  and for the `*models.*` types — but not for owning the inbound
  pipeline. See Alternatives §Library-driven inbound.
- **No update-routing inside the library** (`WithDefaultHandler` is
  not used). The router is the only routing layer; the adapter is a
  thin transport that talks the `channel.Channel` contract.
- **No new fields on `Envelope`.** The lifecycle is transport, not
  domain. `Envelope` is unchanged.
- **No mutation of ADR-0001's "transport-agnostic pure converter"
  decision.** `InboundFromUpdate` and `OutboundParams` keep the same
  signatures and remain side-effect-free. The lifecycle wraps them
  but does not replace them.

### Coverage and TDD plan (for the implementation phase)

For each commit landing within Phase 2E.8, red tests precede green
code per the repo's non-negotiable cycle. The test surface breaks down
roughly as:

1. **Adapter construction.** Invalid configs (webhook URL set in
   polling mode, secret token empty in webhook mode, bad listen
   address, etc.) → `New` returns a typed error. Happy paths in both
   modes.
2. **Webhook HTTP handler (table-driven `httptest`).**
   - 200 on valid POST + valid secret + valid update.
   - 401 on missing or wrong secret (with a constant-time check on
     wall-clock variance bounded across the rows — best-effort).
   - 405 on non-POST.
   - 400 on body decode failure / oversized body.
   - 200 on `ErrNoMessage` / `ErrUnsupportedContent` (silent-ack
     path).
   - 200 + warn-counter increment on enqueue-timeout (saturation
     path).
3. **Polling loop.** Driven by an in-test `BotPoller` interface that
   stands in for `bot.Bot.GetUpdates`, so the loop logic is exercised
   with no network. Cases: empty result; result advances offset;
   converter error skips with logged counter; ctx cancellation exits
   cleanly; saturated buffer drops with counter.
4. **`Send` round-trip.** Driven by an in-test `BotSender` interface
   stand-in for `bot.Bot.Send*`. Cases: each `OutboundKind` dispatches
   to the matching `Sender.Send*` method; sender errors propagate;
   nil envelope; wrong channel; wrong direction.
5. **Lifecycle.** Start/Stop idempotency; Start-failure cleans up the
   HTTP server; Stop closes the inbound channel exactly once; Stop
   before Start is a no-op; Stop after Start in `ModePolling` cancels
   the loop within a bounded deadline.
6. **Conversation correlation.** Inbound Envelopes carry
   `Meta["conversation.id"]` populated from `telegram.chat_id` — the
   one piece of ADR-0003 wiring that lives in this adapter.

Coverage target: ≥90 % for `internal/channel/telegram`, the
package-level threshold inherited from STAGE-02-EXT-PLAN.md and
upheld by every prior 2E phase.

### Trade-offs accepted

- **Two transports doubles the inbound test matrix.** Webhook tests
  use `httptest.Server`; polling tests use an interface stand-in.
  Sharing the post-conversion path (the buffered channel + the
  saturation rule) keeps duplication low, but the surface is still
  bigger than picking one mode. The flexibility is worth the cost,
  per §2.
- **The library's `WebhookHandler` is reimplemented in the adapter**
  (constant-time secret check + explicit status codes). The duplicated
  code is ~30 LOC; the operational gain (auditability, no timing
  side-channel, loud 401s) is worth more than re-using the library's
  version.
- **Polling mode uses an in-memory `nextOffset`.** The precise
  restart contract is fixed in §4d: at-least-once delivery bounded
  by Telegram's 24 h server-side buffer, with > 24 h of downtime
  losing the early portion of the outage. Converters are pure, so
  re-delivered Envelopes are byte-identical to the originals; any
  side-effect handler downstream must be idempotent or guarded by
  the conversation/state layer (Stage 7+). A persistent
  `nextOffset` is a deferred follow-up, not a missing feature in
  this phase.
- **Drop-on-saturation acknowledged at the HTTP layer.** Reasoned in
  §4c; the alternative (non-2xx → exponential retry → webhook
  removal by Telegram) is the worse failure mode. Saturation drops
  are loud (logger + metric counter), not silent.
- **`Mode` is a config flag rather than two distinct types.** A
  `WebhookAdapter` and `PollingAdapter` split would type-encode the
  invariant "you can't mix the two modes per instance", but it would
  also force two parallel implementations of every shared
  responsibility (Send, Manifest, Stop, Receive). The shared adapter
  with a `Mode` enum is the pragmatic call — wrong-mode method use
  is prevented by the constructor refusing invalid combinations of
  options.
- **`Start` errors surface synchronously; `ListenAndServeTLS` errors
  after Start are surfaced via the logger only.** The asymmetry is
  unavoidable: post-Start errors are inherently async, and converting
  them into a synchronous return would require either a polling API
  (`adapter.Err()`) or a side channel (`adapter.Errors() <-chan error`).
  Phase 2E.8 keeps the logger; a structured error channel can be
  added later if a real consumer (the supervisor) needs it.

## Alternatives Considered

### Transport — A1: webhook only

Drop polling support entirely; webhook is what production deployments
use anyway.

**Rejected.** Forces every self-hosted operator to wire up a public
HTTPS endpoint (reverse proxy, TLS certs, DNS) before they can use
Korvun against Telegram. That is contrary to the MASTER.md
positioning "from Raspberry Pi to the cloud" and would gate the most
permissive deployment shape behind the most demanding infrastructure
requirement.

### Transport — A2: polling only

Polling is more universal, so just ship that.

**Rejected.** Polling makes long-lived HTTP requests against Telegram
every 30 s, which (a) accumulates latency on inbound updates compared
to webhook's push semantics, (b) is incompatible with running multiple
Korvun replicas against the same bot (only one polling instance can
hold the `getUpdates` lock at a time; webhook fans out cleanly via a
load balancer), and (c) wastes Telegram's API quota at scale. Webhook
is the right shape for the cloud profile and shipping it now (as a
sibling, not a replacement) is the lowest-friction time.

### Transport — A3: library owns inbound (`b.Start` / `b.StartWebhook` + `WithDefaultHandler`)

Construct `bot.Bot` with `WithDefaultHandler(func(ctx, b, u) { /* push envelope to chan */ })`,
call `b.Start(ctx)` (polling) or `b.StartWebhook(ctx) + http.HandleFunc(path, b.WebhookHandler())`
(webhook).

**Rejected** for the chosen-design fit reasons enumerated in ADR-0001
and reinforced in §3 here:

- The library handler validates the secret with `==` and responds
  silently on rejection. Both are operational regressions vs. the
  hand-rolled handler.
- The library's `StartWebhook` adds a second buffer (its internal
  updates channel) between the HTTP handler and our `InboundFromUpdate`.
  We'd then have to drain that channel via `WithDefaultHandler` into
  *our* channel — two buffers, one converter, no clarity gain.
- Library-owned lifecycle means `Stop` becomes "cancel ctx, hope it
  returns soon". The adapter-owned design lets us serialise "stop HTTP
  server → DeleteWebhook → close channel" with explicit ordering and
  predictable error handling.
- `bot.ProcessUpdate` (the seam ADR-0001 picked the library for) is
  still available if we ever need it. The chosen design uses
  `InboundFromUpdate` directly, which is even thinner.

The chosen design uses `bot.Bot` only as: (a) an outbound HTTP client
for the `Send*` calls, and (b) a thin caller of `SetWebhook` /
`DeleteWebhook` / `GetUpdates`. This is exactly the boundary the
library README advertises as supported, and matches ADR-0001's
"transport-agnostic adapter" principle.

### Lifecycle — B1: Start/Stop on the `channel.Channel` interface itself

Add `Start(ctx) error` and `Stop(ctx) error` to `channel.Channel`,
so the router can drive every channel's lifecycle uniformly.

**Rejected.** Three reasons:

- The router does not need to own channel lifecycles; `main.go`
  already does. Pushing lifecycle into the router would entangle two
  concerns (routing + transport ownership) that were deliberately
  kept separate in Stage 3.
- Not every channel adapter has a lifecycle. The generic webhook
  adapter (Phase 2.2) has no Start/Stop in any meaningful sense —
  the HTTP handler is registered into an externally owned mux. Forcing
  `Start`/`Stop` onto the interface would make those adapters carry
  no-op methods purely for the contract.
- The `Receive`/`Send` contract is the routing contract. Lifecycle is
  a different layer. The webhook adapter precedent is the right model
  here.

### Lifecycle — B2: HTTP server lives outside the adapter

`main.go` owns the `*http.Server`; the adapter only exposes
`InboundHandler() http.Handler` (mirroring the existing webhook
adapter's API), and `main.go` mounts it into a shared mux.

**Tempting**, and explicitly supported as a future evolution — the
adapter implementation can refactor toward this if Korvun ever needs
multiple webhook-flavored channels (Telegram + Slack + Meta + ...) to
share a single listener. For Phase 2E.8, however, the adapter owns
its own `*http.Server` because:

- Single-channel-per-process is the deployment shape Stage 5 will
  ship first.
- Sharing a listener across channels needs a routing-by-path policy
  that ADR-0008 should not pre-decide. Defer until evidence.
- The internal `http.Server` keeps `Start`/`Stop` end-to-end testable
  with `httptest` rather than requiring a separate harness.

When the multi-channel-on-one-port story becomes real, an
`InboundHandler()` method can be added without breaking the existing
`Start`/`Stop` API.

### Saturation — C1: 503 Service Unavailable on a full buffer

Return 503 to Telegram when the inbound buffer is full and the
enqueue timeout expires. Telegram retries with backoff, so messages
aren't lost, just delayed.

**Rejected.** Telegram's documented webhook contract states that
after enough non-2xx responses, **the webhook is removed** until the
bot re-registers. A saturated buffer is a transient overload, not a
permanent fault, but a sustained overload would brick the deployment
in a way that takes a manual `SetWebhook` to recover. Acknowledge-and-warn
is the right shape: surface the saturation via metrics and logs,
keep the contract with Telegram intact.

### Saturation — C2: block until ctx canceled

Drop the enqueue timeout; `select { case a.inbound <- env: ...
case <-ctx.Done(): ... }`. The HTTP handler simply waits.

**Rejected.** Telegram has a per-request timeout on its end; long
handler latencies trigger the same backoff-and-removal failure mode
as 503. The 250 ms timeout matches the router's `DefaultEnqueueTimeout`
deliberately — once the router is saturated, the channel is
saturated, and the right answer is to bound the wait, ack, and warn.

### Rename — D1: do the rename in the same commits as the lifecycle work

Land the rename inline with the first lifecycle commit, so master
never carries the dual `internal/channels/` + `internal/channel/`
state across a commit.

**Rejected.** The rename has zero behavioral impact (verified: no
external imports) but a large diff footprint. Bundling it with
lifecycle work would make the code-review diff unreviewable as
"lifecycle changes" — every renamed line would show up as
deletion + addition. A separate `refactor(telegram): ...` commit
isolates the noise. Master already carries the dual state today
(`internal/channels/telegram/` + `internal/channel/`); one more
commit's worth of additional carry is harmless.

### Rename — D2: defer the rename to a later cleanup phase

Land Phase 2E.8 against `internal/channels/telegram/` and rename in
a future maintenance pass.

**Rejected.** Phase 2E.8 is when the package is touched the most
since its creation (new files, the adapter, the lifecycle); landing
all of that under a path the STAGE-02 doc has already labelled
"open debt" entrenches the inconsistency further. Renaming first is
the smallest possible window — one mechanical commit at the start of
the phase, before any new file is added.

## Open follow-ups (not blockers for Phase 2E.8)

- **Saturation-counter observability hookup — hard dependency on
  Stage 12.** §4c's drop-and-warn rule is only operationally safe if
  `telegram_adapter_inbound_dropped_total` is actually monitored —
  a silent counter is functionally equivalent to silent loss. Phase
  2E.8 emits the counter via `slog` with structured fields; the
  Prometheus exposition + paging alert wiring is a Stage 12
  (observability) deliverable and **must** land before Korvun is
  recommended for any production deployment that uses `ModeWebhook`
  at non-trivial volume. This is a hard dependency, not a soft one:
  drop-on-saturation without a working alert path is silent data
  loss.
- **Multi-channel-on-one-port.** A `Handler() http.Handler` method on
  the adapter would let `main.go` mount it into a shared mux. Defer
  until a second webhook-flavored channel ships in the same Korvun
  process.
- **Persistent polling offset.** Per §4d, `ModePolling` is at-least-
  once-on-restart, bounded by Telegram's 24 h server-side buffer.
  Writing `nextOffset` to a small KV (sqlite, file) at every
  successful poll-batch would tighten the bound toward exactly-once
  at the cost of one I/O per batch. Defer until the at-least-once
  boundary is observed to cause real downstream churn.
- **TLS automation.** `WithACMEAutocert(domain)` for Let's Encrypt
  termination directly in the adapter. Defer until the bootstrap
  layer (Stage 5+) tells us where TLS config wants to live.
- **Per-handler metrics.** Beyond the saturation counter, expose
  inbound-by-update-kind, send-latency-by-`OutboundKind`, etc.
  Belongs to the observability stage, not this one.
- **Webhook IP allow-list.** Telegram publishes the IP range its
  webhooks originate from; an opt-in allow-list at the handler would
  reject unauthenticated probes earlier. Defer until requested.
