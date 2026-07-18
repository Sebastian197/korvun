# ADR-0033: Discord channel (Piece 4, third channel)

> **Status:** accepted
> **Date:** 2026-07-18
> **Deciders:** Sebastián Moreno Saavedra
>
> **Accepted 2026-07-18, reviewed by the copilot.** Framing:
> `docs/notes/piece-4-framing.md`. Companion dependency ADR: ADR-0034 (the WebSocket
> client). `go.mod` is NOT touched by this ADR — the 4th dependency lands with the
> first TDD sub-phase.

## Context

Korvun has two channels behind the `channel.Channel` seam (Telegram + a generic
Webhook). Piece 4 adds a third. Chano's product decision (2026-07-18): the channel is
**Discord**. WhatsApp (business verification + per-message billing = onboarding
friction for a self-hosted tool) and Slack (since 2026-03-03, 1 req/min on
conversation reads for non-Marketplace apps; hostile to AI gateways) were rejected.
Discord: a free bot in minutes, and the Message Content privileged intent is
self-serve below **10,000 users** (official announcement 2026-06-11) — and every
Korvun user runs their OWN bot, so it never approaches the threshold **by
construction**. This extends the deferral line of **ADR-0002** (WhatsApp).

The Discord API was verified against the official docs via Context7
(`/discord/discord-api-docs`) before this ADR (CLAUDE.md non-negotiable). The
load-bearing reality: **receiving free-form messages is Gateway-WebSocket-only** —
`MESSAGE_CREATE` events are delivered solely over the Gateway; there is NO HTTP path
to poll incoming messages. Sending is plain REST. This shapes the whole design.

The `channel.Channel` seam (`Name()`, `Send(ctx, env)`, `Receive(ctx) (<-chan
*envelope.Envelope, error)`, plus `DroppedCount()`, `Manifest()`) is unchanged: the
Discord adapter is a new implementation behind the existing contract, not an
architecture change.

## Decision

### 1. Config contract

- **`type: "discord"`**, **`mode: "gateway"`** (the single v1 receive mode; REST send
  is not a separate mode — it rides inside the same adapter).
- **Secret is env-only:** `token_env` holds the NAME of the environment variable with
  the bot token (ADR-0010: never argv, config, logs, or error messages). A missing
  secret is a loud, named boot error. The token is sent only in the Gateway Identify
  payload and as the REST `Authorization: Bot <token>` header — never logged.

### 2. Receiving — the Gateway WebSocket lifecycle

The adapter opens a WSS connection (Gateway URL from `GET /gateway/bot`) via the
WebSocket client of ADR-0034, and runs this state machine on the adapter's goroutine:

1. Receive **Hello (op 10)** → read `heartbeat_interval` (ms).
2. Send **Identify (op 2)**: `token`, `properties {os, browser, device}`, `intents`
   (§3). On a resume-eligible reconnect, send **Resume (op 6)** instead.
3. Receive **Ready** → store `session_id` and `resume_gateway_url`.
4. Receive **Dispatch (op 0)** events → store the last sequence number `s`;
   `MESSAGE_CREATE` events become Envelopes (§4).
5. Send **Heartbeat (op 1)** every `heartbeat_interval` (with initial jitter, per
   Discord's guidance); expect **Heartbeat ACK (op 11)**. A missing ACK before the
   next cycle means a **zombied connection** → force reconnect.
6. **Reconnect (op 7)** / **Invalid Session (op 9, `d=true`)** / a resume-eligible
   close code / a disconnect with no close code → reconnect to `resume_gateway_url`
   and **Resume (op 6)** with `session_id` + `seq` (NO re-Identify). If the resume
   fails (Invalid Session `d=false`) → fresh Identify from scratch. Reconnects use
   exponential backoff to avoid a reconnect storm.

### 3. Intents and the manual operator step

The Identify intents bitfield is **37377**:

| Intent | Bit | Value |
|---|---|---|
| `GUILDS` | `1<<0` | 1 |
| `GUILD_MESSAGES` | `1<<9` | 512 |
| `DIRECT_MESSAGES` | `1<<12` | 4096 |
| `MESSAGE_CONTENT` (privileged) | `1<<15` | 32768 |

`MESSAGE_CONTENT` is a **privileged intent**: it must be enabled **manually in the
Discord Developer Portal** for the bot before it can be requested. This is a one-time
operator step — documented in the channel's setup docs exactly like the Telegram
BotFather token step (it is operator configuration, not code). Self-serve below
10,000 users (see the framing note `[NC-1]`); Korvun-per-user is far below by
construction.

### 4. `MESSAGE_CREATE` → Envelope

- `channel = "discord"`; **`conversation.id = <Discord channel.id>`** (guild channel
  or DM channel), so conversation memory keys the same way Telegram does.
- Author: the message author's id + display name.
- Text: `content`.
- **Guild channels AND DMs** are both in v1 (DMs add only the `DIRECT_MESSAGES`
  intent, already in 37377, plus reading the DM channel id).
- **Ignore messages authored by the bot itself** (compare author id to the bot's
  application/user id) — otherwise the bot's own replies would loop back in.
- Attachments/media are OUT of v1 (parity with Telegram v1).
- **Input validation at the channel edge** (project security rule): reject/skip
  malformed or oversized events, count them via `DroppedCount`.

### 5. Sending — REST, with the existing rate-limit grammar

- `POST /channels/{channel.id}/messages` over `net/http`, body `{content,
  allowed_mentions}`. `content` is bounded to Discord's 2000-character limit.
- **429 → `model.RateLimitError{Provider, RetryAfter}`** (the existing grammar Groq
  and Ollama already use): honor the `Retry-After` header / `retry_after` float, and
  the global bucket. Per-route buckets are respected via the standard header set.

### 6. Safety — `allowed_mentions` defaults to none

Outbound messages set **`allowed_mentions: { "parse": [] }` by default**, so model
output can never trigger a mass ping (`@everyone`/`@here`/roles). This is a security
default; it can be made configurable later, but the safe default ships first.

### 7. Lifecycle and the seam

- `Start` spawns the Gateway goroutine (its own context; heartbeat; resume/reconnect
  with backoff). `Stop` closes the WS connection cleanly and drains. `DroppedCount` is
  an atomic counter (as Telegram). The adapter implements `Receive`/`Send`/`Name`/
  `Manifest` — the existing `channel.Channel` seam, no change to the router or brain.

### 8. v1 scope — and what is explicitly OUT

**In v1:** receive text from guild channels + DMs via the Gateway; reply text via REST
`createMessage`; the full Gateway lifecycle (identify, heartbeat, resume/reconnect);
429 handling; `DroppedCount`; clean Start/Stop; input validation; `allowed_mentions`
safety default.

**Explicitly OUT of v1** (future scope, not omissions): threads/forums, voice; slash
commands / interactions (need an interactions endpoint + Ed25519 signature); rich
embeds, components (buttons/selects/modals); attachments/media (in and out);
reactions, message edits/deletes; **sharding** (only required past ~2500 guilds — the
per-user-bot model never approaches it, so v1 is single-shard by construction);
presence / guild member lists; Gateway compression (`zlib-stream`/`zstd`).

## Consequences

- Korvun gains a third channel behind the unchanged seam; the router/brain/policy path
  is untouched.
- The adapter owns real network-protocol state (Gateway resume/reconnect, heartbeat) —
  more moving parts than Telegram's library-driven polling. Bounded by TDD with a fake
  Gateway (`httptest`-driven WS server) and `-race` on the lifecycle.
- Outbound reuses the existing `RateLimitError`/`Retry-After` grammar — no new error
  vocabulary.
- Does NOT close a V1 criterion (the 6 are already met); Piece 4 is more reach, not a
  beta requirement.

## Alternatives Considered

- **WhatsApp Business Cloud API** — rejected (ADR-0002 line): business verification +
  per-message billing is unacceptable onboarding friction for a self-hosted tool.
  Future candidate post-traction.
- **Slack** — rejected: the 2026-03-03 non-Marketplace 1 req/min conversation-read cap
  makes a real-time AI gateway impractical.
- **HTTP-only Discord receive** — does not exist. Message events are Gateway-only, so
  the WebSocket client (ADR-0034) is mandatory, not a preference.
- **A full Discord SDK (e.g. discordgo)** — rejected: pulls a large surface for
  features Korvun does not use (voice, sharding, the whole command framework). A thin
  adapter over a minimal WS client + `net/http` REST keeps the dependency footprint to
  one small, auditable WebSocket library (ADR-0034), consistent with the hand-rolled
  Ollama/Groq REST adapters.

## Risks and mitigations

- **Gateway resume/reconnect** (the main risk): a state machine over `session_id` +
  `seq` + `resume_gateway_url`; handle op 7 / op 9(`d=true`) / recoverable close codes;
  fall back to Identify when resume fails.
- **Zombied connection**: heartbeat + ACK tracking; missing ACK → force reconnect.
- **Rate limits**: REST 429 → `RateLimitError`; Gateway send limit (~120/60s) and
  Identify limits respected; exponential backoff on reconnect.
- **Discord policy changes** (intent thresholds): the per-user-bot model keeps every
  instance below any threshold; the manual intent-enable step is documented for the
  operator.
- **Mass mentions from model output**: `allowed_mentions.parse=[]` default (§6).
