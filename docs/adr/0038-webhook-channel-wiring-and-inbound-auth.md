# ADR-0038: Webhook channel wiring & inbound authentication (generic webhook as a core channel)

> **Status:** accepted
> **Date:** 2026-07-26
> **Deciders:** Sebastián Moreno Saavedra
>
> **Proposed 2026-07-26, clarifications resolved by the copilot.** Design spec:
> `docs/superpowers/specs/2026-07-26-webhook-channel-wiring-design.md`. This ADR pins
> a CONTRACT (network surface + inbound auth + config schema + saturation policy); it
> introduces **no new dependency** — the piece is stdlib-only (`net/http`,
> `crypto/sha256`, `crypto/subtle`) over existing `internal/` packages, so `go.mod` is
> untouched. Extends the deferral/second-channel line of **ADR-0002**.

## Context

Korvun has a generic webhook adapter from Stage 2 (`internal/channel/webhook`) that
maps arbitrary JSON payloads to/from Envelopes with a configurable `FieldMapping`. It
satisfies the router-facing `channel.Channel` seam (`Name`/`Manifest`/`Send`/
`Receive`) but is **never wired into a running Korvun**: it owns no HTTP server, has
no `Start`/`Stop` lifecycle (so it does not satisfy the app-facing `app.Channel`), has
**no inbound authentication**, does **not** set `conversation.id` (breaking memory
continuity, ADR-0019), enqueues inbound **blocking** (no drop/count, unlike
telegram/discord), and does no edge input validation. MASTER.md Fase 2.2 always
specified it as a first-class channel (HTTP JSON → Envelope, configurable mapping,
outbound POST); this piece delivers that wiring and hardens it, **without a rewrite** —
the Stage-2 mapping/`Send`/mime code stays.

The load-bearing reality shaping the design: a webhook is an **inbound HTTP server**
(external systems POST to it), so unlike Discord (Gateway WebSocket) or Telegram
(polling/webhook against a bot API) it needs its own listening socket, and that socket
is a network attack surface the moment it leaves loopback. Auth and bind-safety are
therefore central, not incidental. The primitive for auth already exists in-repo: the
admin mutation gate (**ADR-0028 §1**) verifies `Authorization: Bearer <token>` with
`subtle.ConstantTimeCompare(sha256(got), sha256(want))`, and the Telegram webhook
mode uses `crypto/subtle` for its secret-token header — this channel reuses that
mechanism rather than inventing one.

## Decision

### 1. Config contract (schema)

- **`type: "webhook"`**, registered under the type name **`"webhook"`** — **one
  instance per binary** in beta (parity with telegram/discord; routes bind to
  `"webhook"`).
- **No `mode`.** Webhook has a single transport; `Validate` requires `mode` to be
  **empty** for a webhook channel and rejects a non-empty value with a named
  field-path error (`channels[i].mode: webhook takes no mode`).
- **Inbound secret = the channel-level `token_env`** (the existing `ChannelConfig`
  field, exactly like telegram/discord). It is the NAME of the env var holding the
  inbound shared secret; the value is resolved at boot, never stored/logged (ADR-0010).
  A webhook config still requires a non-empty `token_env`.
- **Nested `webhook` block, a `*WebhookConfig` pointer** (presence detection):
  `channels[i].webhook.{bind, path, outbound_url, outbound_token_env, mapping}`.
  - **Required when `type == "webhook"`** (field-path error if absent); a
    field-path error if **present under any other type**
    (`channels[i].webhook: only valid for type "webhook"`).
  - `bind` — listen address; **default `127.0.0.1:8090`** (loopback) so a fresh boot
    exposes nothing (ADR-0020 §4).
  - `path` — the inbound POST path; **default `/webhook`**.
  - `outbound_url` — where brain replies are POSTed. **REQUIRED** when
    `type == "webhook"`: an absent or empty value is a field-path error
    (`channels[i].webhook.outbound_url`). A webhook with no outbound path could not
    deliver a reply, so a missing one is a misconfiguration, not a silent no-op
    (the receive-only "sink" shape is a future extension, below).
  - `outbound_token_env` — **OPTIONAL** env-var NAME for the OUTBOUND downstream
    secret (see §4).
  - `mapping` — **OPTIONAL**, with canonical defaults: `sender_id`, `sender_name`,
    `text`, `media_url`, `media_type`, `conversation_id`. An operator overrides only
    the fields they need.
- **Two distinct secrets, both env-var NAME only:** `token_env` (inbound) and
  `webhook.outbound_token_env` (outbound). Additive to the schema: every existing
  telegram/discord/config parses, validates, and round-trips through `GET /api/config`
  byte-identically.

### 2. Network surface & lifecycle

- The adapter gains **`Start(ctx)` / `Stop(ctx)`** (satisfying `app.Channel`) and owns
  its own **`*http.Server`**, mirroring the Telegram webhook-mode lifecycle: own mux,
  the existing `InboundHandler()` mounted at `path`, a running-gated **`/healthz`**,
  `ReadHeaderTimeout` set, serve in a background goroutine (serve error logged after
  Start per ADR-0008 §4a).
- `Start` is **all-or-nothing** (a bind failure rewinds to un-started and returns a
  named error — the golden rule). `Stop` is **idempotent**, shuts the server down
  bounded by `ctx`, and closes the inbound channel exactly once so the router pump
  drains — the ADR-0008 order (channels stopped before the router).
- Wiring: `internal/app.defaultChannelFactory` gains a `case webhook.ChannelName` that
  resolves the inbound `token_env` env-only and returns
  `%w: %q (webhook inbound secret)` wrapping **`ErrMissingSecret`** when unset — the
  same loud, named boot failure as telegram/discord (never a silently-unauthenticated
  server).

### 3. Inbound authentication (mandatory)

- Every inbound request must carry **`Authorization: Bearer <secret>`**. The handler
  verifies it with the **ADR-0028 §1 mechanism**:
  `subtle.ConstantTimeCompare(sha256(got), sha256(want)) == 1` — fixed-length hashes,
  so the length-mismatch early-return leaks nothing. A missing/wrong secret → **`401`
  Unauthorized**, and the body is **not read or parsed**. The secret never appears in a
  log or error (only its env-var name may).
- Auth is **unconditional** (applies on loopback too — an inbound write path is
  dangerous regardless of bind).

### 4. Outbound authentication

- When `webhook.outbound_token_env` is set, its resolved value is sent as
  `Authorization: Bearer <token>` on the outbound POST (env-only, ADR-0010, never
  stored/logged). When absent, the outbound POST carries no auth (today's behavior).
- A `outbound_token_env` that is NAMED but does not resolve at boot is a loud, named
  boot error (`ErrMissingSecret`, naming the var) — an operator who asked for outbound
  auth must never boot with an un-authenticated outbound in silence.

### 5. Saturation policy

- Inbound enqueue is **non-blocking** (`select … default`); a full buffer increments a
  **`DroppedCount`** (`atomic.Uint64`, the existing `droppedCounter` seam →
  `registerDroppedSources` pull metric) and returns **`503` Service Unavailable**
  (retryable). The HTTP goroutine never blocks on router health. Matches
  telegram/discord's drop-and-count.

### 6. Edge input validation

- Method `POST` only (`405`); `Content-Type: application/json` required (`415`);
  request-body cap via `http.MaxBytesReader` (**1 MiB**, `413` on exceed); malformed
  JSON `400`.

### 7. Bind safety (TLS)

- Korvun terminates **no TLS** for the webhook. Default bind is loopback (TLS moot). A
  **non-loopback bind emits a loud boot warning** that the Bearer secret crosses the
  network in cleartext without a fronting TLS-terminating reverse proxy — the ADR-0028
  F10 / ADR-0020 §4 precedent. TLS is delegated to an operator-run reverse proxy and
  documented.

## Consequences

**Easier:** an operator declares `type: "webhook"` and gets an authenticated,
own-lifecycle inbound endpoint that boots, drains, and shuts down like the other
channels; memory continuity works (conversation.id is set); a slow brain can never
deadlock the webhook server (non-blocking drop); the auth story is one mechanism
across the admin gate and this channel (ADR-0028), so there is one thing to reason
about and one place to harden. Zero new dependencies keeps the single-binary /
supply-chain posture intact.

**Harder / accepted costs:** the config schema grows a nested block (a permanent
one-way-door contract — mitigated by making it strictly additive and field-path
validated); the inbound secret and the outbound secret are two separate env vars (more
to document, but each is a genuinely distinct trust boundary); real external use
requires the operator to stand up a TLS-terminating reverse proxy (documented, warned
at boot) rather than getting in-process TLS for free; only one webhook instance exists
per binary in beta (multi-endpoint deployments wait for the future `name` extension).

## Alternatives Considered

- **Dedicated `secret_env` for inbound (instead of reusing `token_env`)** — rejected:
  `token_env` already means "the channel's inbound credential env-var name" for every
  channel; reusing it keeps the schema uniform and the validator unchanged. The
  outbound secret, a genuinely different boundary, gets its own field.
- **HTTP backpressure (block the request until the router drains)** — rejected: it
  couples provider HTTP latency to brain health and turns a slow brain into a
  webhook-server deadlock and a trivial DoS. Non-blocking drop + `503` lets a
  well-behaved provider retry without ever blocking the goroutine (spec, ratified).
- **HMAC-over-body signature verification (provider-style signed webhooks)** —
  deferred (future extension): the beta contract is a plain shared-secret Bearer, the
  same primitive as the admin gate. HMAC is a larger, provider-specific contract
  (canonicalization, replay windows) not needed for the generic beta channel.
- **In-process TLS termination (cert/key config, mirroring Telegram's `WithTLS`)** —
  deferred (future extension): loopback default makes it moot for the common case; a
  documented reverse proxy is the standard self-hosted pattern and keeps the channel
  small.
- **`mode: "http"` for symmetry with telegram/discord** — rejected: webhook has a
  single transport, so a required mode would be ceremony; exempting it (and rejecting
  a stray mode value) is clearer.
- **Multiple named webhook instances now** — rejected for beta: one instance matches
  telegram/discord and keeps route-binding simple; a `name` field is a clean additive
  extension when a real multi-endpoint need appears.

## Future extensions (annotated, not built now)

- **`name` field** for multiple webhook instances per binary (multi-endpoint /
  multi-tenant inbound), additive to the schema and the route-binding key.
- **HMAC-over-body** signature verification as an alternative/additional inbound auth
  mode (provider-style signed webhooks).
- **In-process TLS termination** (cert/key config fields) for operators who do not
  front the endpoint with a reverse proxy.
- **Operator-configurable auth header name** (beyond `Authorization: Bearer`).
- **Sink mode** (receive-only webhook: `outbound_url` optional with explicit reply
  discard) if a real demand appears — the inverse of the required-outbound default.
