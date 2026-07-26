# Piece "Webhook channel in the core" — wiring & hardening the generic webhook adapter: Design Spec

> **Status:** draft — all 6 clarifications resolved (2026-07-26); pending ADR-0038
> acceptance before TDD.
> **Governing ADRs:** ADR-0002 (channels; deferred WhatsApp, generic webhook is the
> second channel), ADR-0008 (channel lifecycle / shutdown ordering), ADR-0010
> (secrets are env-var NAME only, never value), ADR-0017 §§1,4,5 (config schema is a
> one-way door; loud boot on a deaf channel; field-path errors), ADR-0020 §4
> (loopback-safe-by-default network surfaces), ADR-0031 (channel `DroppedCount`
> pull-metric seam), ADR-0028 §1 (the admin auth mechanism the inbound Bearer check
> reuses). A NEW short contract ADR —
> **ADR-0038 (`docs/adr/0038-webhook-channel-wiring-and-inbound-auth.md`, status
> proposed)** — is a deliverable of THIS piece and is drafted alongside it: it pins the
> webhook channel's network surface + inbound auth + config schema.
> **External-docs note:** this piece uses **ONLY the Go standard library**
> (`net/http`, `crypto/sha256` + `crypto/subtle` for the constant-time hashed-secret
> comparison, per ADR-0028 §1) and existing
> `internal/` packages (`channel`, `envelope`, `conversation`, `config`, `app`).
> **Zero new dependencies** — so the Context7-first rule does not trigger (no external
> library/SDK/API is programmed against). If any decision below is later found to
> require an external library, work STOPS and the library is raised for an ADR +
> Context7 pass first — it is never introduced in passing.
> **Inherited law:** the router-facing seam `channel.Channel`
> (`Name`/`Manifest`/`Send`/`Receive`) is FROZEN; the app-facing `app.Channel` adds
> `Start(ctx)`/`Stop(ctx)`. The existing `internal/channel/webhook` adapter satisfies
> `channel.Channel` today but NOT `app.Channel` — it owns no HTTP server and has no
> lifecycle. Closing that gap is the heart of this piece.

## Goal

Turn the existing Stage-2 generic webhook adapter
(`internal/channel/webhook/webhook.go`) — today a library object that maps JSON
payloads to/from Envelopes but is **never wired into a running Korvun** — into a
first-class core channel an operator can declare in the config file and that boots,
authenticates, serves, and shuts down cleanly like Telegram and Discord. After this
piece: a config with `type: "webhook"` starts a **dedicated, own-lifecycle HTTP
server** (loopback by default) that accepts authenticated inbound POSTs, converts
them to Envelopes keyed to a durable conversation, hands them to the router, and
posts brain replies back out via HTTP; a missing shared secret is a **named, loud
boot error** exactly like the other channels. This is **wiring + hardening the
existing adapter, NOT a rewrite**: the `FieldMapping`, `payloadToEnvelope`,
`envelopeToPayload`, `Send`, and mime mapping stay; what is ADDED is the config
schema, the `Start`/`Stop` server lifecycle, inbound authentication, a
`conversation.id` mapping, a saturation policy with `DroppedCount`, and edge input
validation. **Explicitly out of scope / deferred:** HMAC-body-signature verification
(provider-style signed webhooks), per-instance TLS termination in-process if
[NC-4] resolves to reverse-proxy, multiple named webhook instances if [NC-2]
resolves to single-instance, and the builder UI for editing a webhook block (a later
builder-canvas piece).

## Functional requirements

- **FR-CONFIG-1 — Additive `webhook` channel schema.** Extend `config.ChannelConfig`
  additively so `type: "webhook"` is accepted with its own block (bind address —
  default `127.0.0.1:8090`; inbound path — default `/webhook`; inbound shared-secret
  env-var NAME; field mapping; outbound URL). Seam: `internal/config` (`ChannelConfig`
  + a new nested
  `WebhookConfig`, likely a pointer for presence detection) + `validateChannels`
  gains a `case "webhook"`. **Blast radius:** `internal/config` (shared package) —
  strictly additive; every existing telegram/discord config MUST parse, validate,
  serialize (`GET /api/config`), and behave byte-for-byte as today. The exact field
  layout is a one-way-door contract → **[NC-1]**.
- **FR-CONFIG-2 — Secret is env-var NAME only (ADR-0010).** The inbound shared secret
  is referenced by the NAME of an environment variable, never by value; the value is
  resolved at boot in `internal/app`, never stored in the config struct, never logged,
  never in an error string (only the env-var name may appear). Mirrors `token_env` /
  `api_key_env`. Whether it REUSES `token_env` or adds a dedicated field → **[NC-1a]**.
- **FR-CONFIG-3 — Validation is field-path and additive (ADR-0017 §5).** A malformed
  webhook block fails `Validate()` with an `ErrInvalidConfig`-wrapped, field-path
  error (`channels[i].webhook.…`), consistent with the existing validators. Zero
  webhook-specific required fields may break the channel-less first-run shape
  (validate only what is present; ADR-0035 §5 / SP5 NC-1).
- **FR-LIFECYCLE-1 — Own HTTP server via `Start`/`Stop` (app.Channel).** The adapter
  gains `Start(ctx) error` and `Stop(ctx) error` so it satisfies `app.Channel`. `Start`
  builds an `*http.Server` bound to the configured address, mounts the (existing,
  reused) `InboundHandler()` at the configured path plus a `/healthz` reporting OK
  only while running, and serves in a background goroutine — mirroring the Telegram
  `startWebhook`/`stopWebhook` pattern (own mux, `ReadHeaderTimeout` set, serve error
  logged via `WarnContext` after Start returns per ADR-0008 §4a). `Start` is
  all-or-nothing (a bind failure rewinds to un-started and returns a named error, the
  golden rule); `Stop` is idempotent, shuts the server down bounded by `ctx`, and
  closes the inbound channel exactly once so the router pump drains — the ADR-0008
  ordering (channels stopped before the router). Seam: `internal/channel/webhook`
  (new `lifecycle.go`); the router-facing `channel.Channel` methods are untouched.
- **FR-AUTH-1 — Mandatory inbound shared-secret authentication, constant-time.** Every
  inbound request must carry the shared secret in a designated header; the handler
  compares it against the boot-resolved secret with `crypto/subtle.ConstantTimeCompare`
  (stdlib, no timing oracle) and rejects a missing/wrong secret with `401
  Unauthorized` BEFORE reading/parsing the body. The scheme is **plain shared-secret
  equality by header** (per the piece brief), NOT HMAC-over-body. Header name →
  **[NC-3]**.
- **FR-AUTH-2 — Missing secret is a named boot error (parity with other channels).**
  If the secret env-var is unset at boot, `defaultChannelFactory` returns
  `%w: %q (webhook inbound secret)` wrapping `ErrMissingSecret` — the same loud,
  named failure Telegram and Discord give (app.go `defaultChannelFactory`), never a
  silently-unauthenticated server. Seam: `internal/app.defaultChannelFactory` gains a
  `case webhook.ChannelName`.
- **FR-MAP-1 — `conversation.id` in the inbound mapping, with a defined fallback.**
  `payloadToEnvelope` sets `env.Meta[conversation.MetaConversationID]` so webhook
  messages are keyed to a durable conversation (today it does NOT — a hardening gap
  that breaks memory continuity). Decision + justification in *Decisions folded in*
  (map from a configurable payload field; fall back to the sender ID). Seam:
  `internal/channel/webhook` + `mapping` gains a `ConversationID` field (additive to
  `FieldMapping`).
- **FR-SAT-1 — Inbound saturation is a counted, non-blocking drop.** The inbound
  enqueue is non-blocking (`select … default`) and increments a `DroppedCount`
  (`atomic.Uint64`) on a full buffer, matching Discord/Telegram and the existing
  `droppedCounter` seam (`registerDroppedSources` → `korvun_channel_dropped_*` pull
  metric). It MUST NOT block the HTTP goroutine on router health. HTTP status on a
  drop → decided in *Decisions folded in* (return `503`, retryable). Seam:
  `internal/channel/webhook` (replaces the current blocking `a.inbound <- env`);
  implements `DroppedCount() uint64`.
- **FR-EDGE-1 — Input validation at the boundary.** The handler enforces: method
  `POST` only (`405`, already present); `Content-Type: application/json` required
  (`415`); a request-body size cap via `http.MaxBytesReader` (`413` on exceed);
  malformed JSON `400` (already present). Constants (size cap) folded below. Seam:
  `internal/channel/webhook` `InboundHandler`.
- **FR-COMPAT-1 — Preserve `SetOutboundURL` / `InboundHandler` (internal-API compat).**
  Both existing exported methods stay (they carry the whole Stage-2 test suite and are
  the seam the builder may later mount). `Start` mounts `InboundHandler()` on its own
  mux internally; the outbound URL is configured from the config block at construction
  (`New` grows a config/option) while `SetOutboundURL` remains valid for tests and
  late binding. No existing test signature breaks; changes are additive.
- **FR-OBS-1 — Minimal observability.** `DroppedCount` exposed as the existing pull
  metric (FR-SAT-1); structured `slog` on: server listening (with bound addr), each
  drop (with reason, secret-free), auth rejections (count/log without echoing the
  secret), and clean shutdown. No new metric families beyond the drop counter for
  beta; reconnect-count does not apply (no long-lived upstream socket).
- **FR-ADR-1 — Contract ADR (deliverable).** Ship **ADR-0038** (short, contract-only):
  the webhook channel's network surface (own server, loopback default, path,
  `/healthz`), inbound auth (shared-secret header, constant-time, mandatory,
  env-only), and the config schema (the [NC-1] resolution). Status `proposed` in the
  spec phase; `accepted` before TDD, per the project cycle.

## Acceptance scenarios (Given / When / Then)

- **AS-1 (schema additive)** Given an existing telegram-only config, When it is
  loaded after the webhook schema lands, Then it parses, validates, and
  round-trips through `GET /api/config` byte-identically to today (no regression).
- **AS-2 (webhook boots)** Given a valid `type: "webhook"` config with the secret
  env-var set, When the app starts, Then the webhook HTTP server is listening on the
  configured (loopback-default) address, `/healthz` returns `200`, and the boot log
  names the bound address.
- **AS-3 (missing secret is loud)** Given a webhook config whose secret env-var is
  UNSET, When the app boots, Then `Start`/wiring fails with an error wrapping
  `ErrMissingSecret` naming the env-var (never the value), and no server is left
  listening — parity with the telegram/discord missing-token boot error.
- **AS-4 (auth: wrong/absent secret)** Given a running webhook server, When a POST
  arrives with a missing or incorrect secret header, Then the response is `401`, the
  body is NOT parsed, no Envelope is enqueued, and the rejection is logged without
  echoing the secret. (Tripwire: constant-time compare is used — asserted by calling
  the compare seam, not by timing.)
- **AS-5 (happy path → router → out)** Given a running webhook wired to a brain, When
  a valid authenticated JSON payload is POSTed, Then an Envelope is produced with the
  mapped sender, text/media, and `conversation.id` set, it is enqueued, and (with a
  fake brain) the reply is delivered as an outbound HTTP POST to the configured URL.
- **AS-6 (conversation_id fallback)** Given a payload with NO conversation field
  mapped/present, When it is converted, Then `conversation.id` falls back to the
  sender ID (justified below), so a stateless sender still gets a stable conversation
  key; asserted on `env.Meta[conversation.MetaConversationID]`.
- **AS-7 (saturation drop, non-blocking)** Given an inbound buffer that is full (router
  not draining), When another authenticated payload arrives, Then the HTTP goroutine
  does NOT block, `DroppedCount` increments by one, the drop is logged, and the
  response is `503` (retryable). (Tripwire: a `-race` test with a stalled consumer
  proves no goroutine blocks on `a.inbound`.)
- **AS-8 (edge validation)** Given a running webhook, When a request uses a non-POST
  method → `405`; a non-JSON `Content-Type` → `415`; a body over the size cap →
  `413`; malformed JSON → `400`; each without enqueuing an Envelope.
- **AS-9 (clean shutdown, ordering)** Given a running webhook under load, When
  `Shutdown` runs, Then `Stop` shuts the HTTP server down bounded by ctx and closes
  the inbound channel exactly once so the router pump drains, with no leaked
  goroutines (`-race`, goroutine count stable) — ADR-0008 order (channel before
  router).

## Success criteria

- New/changed webhook package coverage ≥ 85% (house floor); the auth,
  saturation-drop, and shutdown paths each carry a test.
- `make quality` green with `-race` over the WHOLE suite; webhook package
  `-race -count=5` stable (per the standing per-package tripwire).
- `go.mod` stays at its current direct-dependency count — **zero new dependencies**
  (proven: no `go.mod`/`go.sum` diff beyond none). STDLIB-only is a hard gate.
- The **headless binary and existing pipelines are untouched in behavior**: every
  existing telegram/discord/config test passes unchanged; a `go version -m` diff (or
  equivalent) shows no unexpected module change.
- ADR-0038 is `accepted` before any test is written (design-spec → ADR → TDD).

## Decisions folded in

- **Saturation = non-blocking counted drop + HTTP 503, NOT HTTP backpressure.**
  *(RATIFIED by the copilot, 2026-07-26.)* The
  router pump is the inbound channel's single consumer; a blocking `a.inbound <- env`
  couples the provider's HTTP request latency to router/brain health and turns a slow
  brain into a webhook-server deadlock (and a trivial DoS). Telegram and Discord
  already drop-and-count via the `droppedCounter` seam; matching them keeps one
  observability story. To still let a well-behaved provider recover, the handler
  returns `503 Service Unavailable` (retryable) on a full-buffer drop rather than a
  false `200` — never blocking the goroutine either way. (Alternative considered:
  block with a timeout — rejected, it reintroduces the coupling.)
- **conversation.id mapping = a configurable payload field, fallback to sender ID.**
  *(RATIFIED by the copilot, 2026-07-26.)* Add `ConversationID` to `FieldMapping`; if the mapped field is present, use it;
  otherwise fall back to the sender ID. Rationale: a generic webhook has no universal
  conversation concept, but memory continuity (ADR-0019) needs a stable key. A caller
  that models threads supplies one; a caller that does not still gets a per-sender
  conversation — the same "a DM keys on its channel/sender" intuition Discord uses.
  Empty sender is already rejected upstream (`missing sender ID`), so the fallback is
  never empty.
- **Edge validation constants:** body cap via `http.MaxBytesReader` at **1 MiB**
  (generous for JSON control payloads, cheap DoS floor; revisable), `Content-Type`
  must be `application/json` (charset-suffix tolerated). These are constants in the
  webhook package, not config, to keep the schema small; promotable to config later
  if a real need appears.
- **`/healthz` on the webhook's own server**, running-gated, mirroring Telegram's
  webhook mode — an operator/proxy liveness probe distinct from the admin
  `/healthz`.
- **Keep `New` additive:** `New` grows to accept the resolved config (outbound URL,
  bind addr, path, secret, mapping) via an options-style or config-struct arg while
  the current `New(name, mapping)` behavior is preserved for the existing tests
  (either kept as-is with setters, or wrapped) — no Stage-2 test signature breaks.

## Clarifications resolved (2026-07-26)

> All six open points were resolved by the copilot. TDD is unblocked once ADR-0038 is
> `accepted`. Each resolution is now law for the implementation; the FR `[NC-x]`
> pointers above read against these.

- **NC-1a — Inbound secret REUSES `token_env`.** The inbound shared secret is carried
  by the existing `ChannelConfig.token_env` field (channel level, exactly like
  telegram/discord), NOT a new `secret_env`. The nested `webhook` block carries **no
  inbound secret**. `validateChannels` keeps requiring a non-empty `token_env` for
  `type: "webhook"`.
- **NC-1b — Nested `WebhookConfig` by pointer.** A new
  `channels[i].webhook.{bind, path, outbound_url, outbound_token_env, mapping}` block,
  a `*WebhookConfig` pointer for presence detection. It is **required when
  `type == "webhook"`** (a field-path error if absent) and is a **field-path error if
  present under any other type** (`channels[i].webhook: only valid for type "webhook"`).
  Exact defaults (ADR-0038 §1): `bind` → `127.0.0.1:8090`, `path` → `/webhook`.
- **NC-1c — Webhook is EXEMPT from `mode`.** Unlike telegram/discord, `type: "webhook"`
  takes no transport mode. `Validate` requires `mode` to be **empty** for webhook; a
  non-empty `mode` is a named field-path error
  (`channels[i].mode: webhook takes no mode`).
- **NC-1d — `mapping` OPTIONAL with canonical defaults.** When
  `webhook.mapping` is omitted (or a field within it is empty), the canonical defaults
  apply: `sender_id`, `sender_name`, `text`, `media_url`, `media_type`,
  `conversation_id`. An operator overrides only the fields they need.
- **NC-2 — ONE instance per binary in beta, registered as `"webhook"`.** Parity with
  telegram/discord: a single webhook channel, registered under the type name
  `"webhook"` (routes bind to `"webhook"`). A per-instance `name` field is a **future
  additive extension** (annotated in ADR-0038), not built now.
- **NC-3 — `Authorization: Bearer <secret>`, verified with the ADR-0028 mechanism.**
  The inbound secret rides the `Authorization: Bearer` header and is verified with the
  **same mechanism as the admin mutation gate (ADR-0028 §1)**:
  `subtle.ConstantTimeCompare(sha256(got), sha256(want))` (fixed-length hashes so the
  length-mismatch early-return leaks nothing), `401` on mismatch, before the body is
  read. **HMAC-over-body is out of beta scope**, recorded as a future extension in
  ADR-0038. (In-repo precedent for the primitive: `internal/controlapi/mutation.go`
  and `internal/channel/telegram/webhook.go`.)
- **NC-4 — TLS delegated to a documented reverse proxy.** Korvun terminates no TLS for
  the webhook. Default bind is loopback (TLS moot). If the operator binds a
  non-loopback address, boot emits a **loud warning** that the Bearer secret crosses
  the network in cleartext without a fronting TLS-terminating reverse proxy — the same
  precedent as ADR-0028 F10 / ADR-0020 §4 (the mutation-gate non-loopback warning).
  **In-process TLS termination (cert/key fields) is a future extension** in ADR-0038,
  not built now.
- **NC-5 — `outbound_token_env` OPTIONAL in the webhook block.** When present, its
  resolved value is sent as `Authorization: Bearer <token>` on the outbound POST (a
  second env-var, env-only per ADR-0010, never stored/logged); when absent, the
  outbound POST carries no auth (today's behavior). Resolution happens at boot in
  `internal/app` alongside the inbound `token_env`.
- **NC-6 — ADR-0038 confirmed free.** Verified on disk: `docs/adr/` runs through
  `0037-keychain-dependency.md`; `0038` is unclaimed. The contract ADR is
  `docs/adr/0038-webhook-channel-wiring-and-inbound-auth.md`.

**Schema note (resolved shape).** `token_env` (channel level) = the INBOUND secret;
`webhook.outbound_token_env` (block level, optional) = the OUTBOUND downstream secret.
Two distinct secrets, both env-var NAME only (ADR-0010). This supersedes any earlier
FR wording that implied the inbound secret lived inside the nested block.
