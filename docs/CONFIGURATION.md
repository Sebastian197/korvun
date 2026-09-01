# Korvun configuration reference

Korvun reads **one JSON file**, passed with `--config` (default `korvun.json`):

```sh
korvun serve --config /etc/korvun/korvun.json
```

> **Legacy form.** The pre-CLI invocation `korvun -config <path>` still works — a
> retrocompat shim routes it to `serve` unchanged — but `korvun serve --config
> <path>` is the canonical form and what the docs use from here on. (This is the
> only place the old form is mentioned.)

The field shape below is a **contract** (ADR-0017 §1): once you write a config,
the field names and structure are stable. The format is standard-library
`encoding/json` (YAML is a deferred decode path over the same schema). Start from
a profile in [`configs/`](../configs/) and adjust.

> **Secrets are environment variables, by NAME — never by value.** Fields ending
> in `_env` (`token_env`, `api_key_env`) hold the **name** of an environment
> variable; Korvun reads the value from the environment at boot. A secret is never
> read from argv, the config file, logs, or error messages (ADR-0010 §3). A
> missing secret is a loud, named boot error. On Korvun Desktop, the
> **SECRETOS** card in Ajustes manages the values behind those names over
> the OS keychain — write-only (no value is ever displayed or returned),
> presence shown per name, environment winning over keychain — and it works
> even while the core is stopped.

## Top-level

| Field | Type | Required | Meaning |
|-------|------|----------|---------|
| `channels` | array | **yes** (≥1) | Messaging channels to run. |
| `brains` | array | **yes** (≥1) | Orchestrating brains. |
| `routes` | array | **yes** (≥1) | Bindings of a channel to a brain. |
| `storage` | object | no | Durable conversation store. **Absent ⇒ stateless.** |
| `observability` | object | no | Admin HTTP server. **Absent ⇒ ON (loopback).** |
| `admin` | object | no | Enables the write/mutation surface (the no-code builder). **Absent ⇒ read-only.** |

Note the deliberate asymmetry: an absent `storage` block means *off* (run
stateless), while an absent `observability` block means *on* with safe loopback
defaults (observability is safe on loopback and always useful). An absent `admin`
block means *read-only* (no mutation, the safe default).

## `channels[]`

| Field | Type | Required | Values / meaning |
|-------|------|----------|------------------|
| `type` | string | **yes** | Adapter. Supported: `telegram`, `discord`, `webhook`, `console`. |
| `mode` | string | conditional | Transport. `telegram` → `polling`; `discord` → `gateway`. **`webhook` and `console` take no `mode`** (a non-empty value is rejected). |
| `token_env` | string | conditional | **Name** of the env var holding the channel's inbound secret (bot token for telegram/discord; the shared Bearer secret for webhook). **`console` takes no secret** (a non-empty value is rejected). |
| `webhook` | object | **yes** for `webhook` | The webhook block (below). Rejected on any other type. |

A channel registers under its `type` as its name (the value `routes` reference).

### `telegram`

```json
{ "type": "telegram", "mode": "polling", "token_env": "TELEGRAM_BOT_TOKEN" }
```

### `discord`

Receives over the Discord Gateway (a WebSocket) and replies over REST. `mode` is
always `"gateway"` (the single v1 transport). The bot token's env var holds a token
of the form Discord issues in the Developer Portal → **Bot** tab.

```json
{ "type": "discord", "mode": "gateway", "token_env": "DISCORD_BOT_TOKEN" }
```

> **One manual operator step:** the bot needs the **Message Content** privileged
> intent turned ON in the Discord Developer Portal, and must be invited to your
> server. Full walkthrough: **[docs/DISCORD-SETUP.md](DISCORD-SETUP.md)** (the Discord
> counterpart of Telegram's BotFather steps).

### `webhook`

A generic HTTP endpoint: any system POSTs JSON in, Korvun replies by POSTing to a URL
you choose (ADR-0038). It takes **no `mode`**. `token_env` names the env var holding the
**inbound shared secret** every request must present as `Authorization: Bearer <secret>`
(constant-time compared). The nested `webhook` block is **required**:

| Field | Type | Required | Values / meaning |
|-------|------|----------|------------------|
| `bind` | string | no | Listen address. Default **`127.0.0.1:8090`** (loopback). A non-loopback bind warns at boot (secret crosses the network in cleartext without a TLS reverse proxy). |
| `path` | string | no | Inbound POST path. Default **`/webhook`**. |
| `outbound_url` | string | **yes** | Where brain replies are POSTed. |
| `outbound_token_env` | string | no | **Name** of the env var holding an OUTBOUND Bearer secret sent to `outbound_url`. Optional — but if NAMED, it must resolve at boot (an unset value is a loud error, never a silent unauthenticated outbound). |
| `mapping` | object | no | JSON field names → Envelope fields. Omitted fields use canonical defaults: `sender_id`, `sender_name`, `text`, `media_url`, `media_type`, `conversation_id`. |

Two distinct secrets, both env-var **names** only (never values): `token_env` (inbound)
and `webhook.outbound_token_env` (outbound). Edge validation on every request: `POST`
only (else 405), `application/json` (else 415), ≤ 1 MiB body (else 413); a full inbound
buffer answers `503` (retry later). `conversation_id` groups messages into one
conversation; when absent, it falls back to the sender id.

```json
{
  "type": "webhook",
  "token_env": "KORVUN_WEBHOOK_SECRET",
  "webhook": {
    "bind": "127.0.0.1:8090",
    "path": "/webhook",
    "outbound_url": "https://example.com/korvun/replies"
  }
}
```

> **Zero-to-round-trip walkthrough** (generate the secret, curl a message in, a one-line
> test receiver, and the reverse-proxy/TLS exposure path):
> **[docs/WEBHOOK-SETUP.md](WEBHOOK-SETUP.md)**.

### `console`

The internal direct-chat channel: the operator talks to the brains from the
desktop app's Chat tab, with **no network, no transport and no secret** —
messages enter through the control API and every turn is persisted like any
other conversation. It requires the `storage` **and** `session` blocks (the
direct chat IS persistence, and `/new` / `/reset` live in sessions). When no
route names it, it auto-routes to the **first brain**, so it never boots inert.

```json
{ "type": "console" }
```

See [`CHAT.md`](CHAT.md) for how the Chat tab uses it.

## `brains[]`

| Field | Type | Required | Values / meaning |
|-------|------|----------|------------------|
| `name` | string | **yes** | Unique brain name (referenced by `routes`). |
| `sensitivity` | string | **yes** | `public` \| `private`. `private` drops cloud models before dispatch (ADR-0015). |
| `dispatch` | string | no | `fanout` (default) \| `sequential` (cost-saving fail-over, ADR-0016). |
| `policy` | object | **yes** | The reducer that picks the reply. |
| `models` | array | **yes** (≥1) | The provider catalog for this brain. |
| `agent` | object | no | Mounts a tool-use `AgentBrain` instead of the default orchestrator (ADR-0021). |

**`sensitivity`** is the pre-dispatch privacy constraint: a `private` brain
excludes `cloud`-locality models *before* calling them (the privacy selector,
ADR-0015), so sensitive payloads never leave the box.

**`dispatch`** shapes how the models are called: `fanout` calls all in parallel
(every provider answers, then the policy reduces); `sequential` tries them in
order and stops at the first success (the real cost saving — a paid provider is
contacted only if the local one failed).

### `brains[].policy`

| Field | Type | Required | Values / meaning |
|-------|------|----------|------------------|
| `kind` | string | **yes** | `priority` \| `consensus`. |
| `order` | array of string | — | Provider priority list both reducers use. |

- **`priority`** (ADR-0012) — pick the reply from the highest-priority successful
  provider, in `order`.
- **`consensus`** (ADR-0013) — pick the answer a strict majority of successful
  providers agree on (floor of two; a tie or a lone success ⇒ no consensus).
  Compose consensus over priority by falling back to the trusted provider.

### `brains[].models[]`

| Field | Type | Required | Values / meaning |
|-------|------|----------|------------------|
| `provider` | string | **yes** | `ollama` \| `groq` \| `openai-compatible`. |
| `model_id` | string | **yes** | The provider's model name (e.g. `llama3.2`). |
| `locality` | string | **yes** | `local` \| `cloud`. **Declared**, not derived — the privacy selector routes on it (ADR-0015 §3). |
| `base_url` | string | provider-dependent | `ollama`/`groq`: optional override of the adapter default (e.g. `http://localhost:11434`). `openai-compatible`: **required** — see below. |
| `api_key_env` | string | provider-dependent | **Name** of the env var holding the API key. **Required for `groq`.** Optional for `openai-compatible` (see below); never the key value itself. |

#### `provider: "openai-compatible"` (ADR-0044)

Any endpoint speaking the OpenAI chat-completions wire — cloud or local —
becomes a Korvun model by config alone. Rules:

- `base_url` is **required** and is the **full prefix**: Korvun appends
  exactly `/chat/completions` and never guesses (`/v1` vs no `/v1` vs
  `/api/v1` differs per provider — see the examples below). It must be an
  absolute `http`/`https` URL with a host, and must **not** carry
  credentials, a query, or a fragment. Trailing `/` is tolerated (trimmed).
- `api_key_env` is **optional**: absent ⇒ no `Authorization` header (the
  local-server case). If you **do** name a variable, it must resolve at
  boot — a named-but-unset variable fails loudly with the variable's name.
- `locality` stays **declared** by you, exactly as for every provider: an
  entry declared `local` (LM Studio, llama.cpp) is eligible in private
  brains; one declared `cloud` is never contacted by them.
- Two `openai-compatible` entries in the **same brain** with the same
  `base_url` (after trailing-slash trim) **and** the same `model_id` are
  rejected at load — that is the same backend model wired twice.
- Redirects are refused: if the endpoint answers 3xx, the call fails
  rather than follow the conversation to an undeclared host.

`base_url` examples per provider (verified against each provider's
official documentation, 2026-08-22 — the spec's verification table):

| Target | `base_url` |
|--------|-----------|
| OpenAI | `https://api.openai.com/v1` |
| DeepSeek | `https://api.deepseek.com` (no `/v1` — the only documented form) |
| Moonshot / Kimi | `https://api.moonshot.ai/v1` (China platform: `https://api.moonshot.cn/v1`) |
| Gemini (OpenAI-compat mode) | `https://generativelanguage.googleapis.com/v1beta/openai` |
| OpenRouter | `https://openrouter.ai/api/v1` (model ids are `provider/model`) |
| LM Studio (local) | `http://localhost:1234/v1` |
| llama.cpp server (local) | `http://127.0.0.1:8080/v1` |

Example — a private brain on a local LM Studio plus a cloud fallback for
public brains:

```json
{
  "provider": "openai-compatible",
  "model_id": "qwen2.5-7b-instruct",
  "locality": "local",
  "base_url": "http://localhost:1234/v1"
}
```

```json
{
  "provider": "openai-compatible",
  "model_id": "deepseek-chat",
  "locality": "cloud",
  "base_url": "https://api.deepseek.com",
  "api_key_env": "DEEPSEEK_API_KEY"
}
```

Native tool calling IS supported over this provider (agent brains use it
automatically): choose a **tools-capable** model — Korvun does not probe
capability, so a model without tools support answers with the provider's
own honest 400 error. Every provider in the table above supports
OpenAI-style tools (llama.cpp needs its server started with `--jinja`).

Not in scope today (they fail or degrade per the spec): streaming and
models that require OpenAI's `developer` instruction role (the o1
family).

### `brains[].agent` (optional, ADR-0021 + ADR-0041)

Present ⇒ this brain is a bounded tool-use agent instead of the fan-out
orchestrator. Both satisfy `brain.Brain`, so routing is unchanged. Every
ADR-0041 field below is optional and additive: an agent block written before
governance existed behaves byte-for-byte as it always did.

| Field | Type | Required | Meaning |
|-------|------|----------|---------|
| `tools` | array of string | **yes** (≥1) | Built-in tools to register. Pure: `time`, `echo`, `calc`. Caged (each REQUIRES its cage block below): `read_file`, `http_fetch`, `webhook_call`, `memory_note`. |
| `max_iterations` | int | no | Hard loop cap. `0` ⇒ the AgentBrain default. |
| `system_prompt` | string | no | Operator prompt appended after the protocol block. |
| `governance` | array | no | Tri-state grants (see below). Absent ⇒ ungoverned: every listed tool allowed on every channel. |
| `tool_attrs` | object | no | Per-tool attrs OVERRIDES over the house defaults (`read_file` sensitive; `http_fetch`/`webhook_call` network). Keys must be listed in `tools`. |
| `read_file` | object | with the tool | The jail: `root` (**required**), `max_bytes` (`0` ⇒ 64 KiB). |
| `http_fetch` | object | with the tool | The cage: `allow_hosts` (**required**, exact host, optional `:port`), `max_bytes`, `max_redirects`. |
| `webhook_call` | object | with the tool | The cage: `allow_hosts` (**required**), `max_bytes`, `timeout_seconds` (`0` ⇒ 10s). |
| `memory` | object | with the tool | The memory block for `memory_note`: `scope` (`"conversation"` default \| `"brain"`), `max_notes` (`0` ⇒ 10, 1..100), `max_note_runes` (`0` ⇒ 200, 1..2000), `budget_runes` (`0` ⇒ 2000, must be ≥ `max_notes × max_note_runes`). Requires the `storage` block; `memory_note` also requires a `governance` grant covering it; `scope: "brain"` requires the brain's selected model to be **local**. |
| `effect_ceiling` | string | no | The brain's effect ceiling on the E3 ladder (`pure` \| `read_external` \| `write_reversible` \| `write_compensatable` \| `write_irreversible` \| `critical`). Absent ⇒ unbounded, today byte-for-byte. With `approvals.enabled`, `write_irreversible`/`critical` attempts under a ceilinged brain PARK for the operator's approval (`korvun approvals`). Unknown class ⇒ boot failure naming the ladder. |
| `skills_dir` | string | no | AgentSkills-compatible skills directory. Missing dir ⇒ boot failure; a malformed skill inside ⇒ skipped with a warning. |
| `skills_body_budget` | int | no | Total rune budget for injected skill bodies (`0` ⇒ 8192). |

Each `governance[]` entry:

| Field | Type | Required | Meaning |
|-------|------|----------|---------|
| `tool` | string | **yes** | A tool listed in `tools`. One grant per tool. |
| `mode` | string | **yes** | `allow` \| `shadow` \| `deny`. In `shadow` the tool is ANNOUNCED to the model but never executed — an honest simulation observation returns instead, and the attempt audits as `tool_shadowed`. |
| `channels` | array of string | no | Restricts the grant to those channels (exact match). Absent ⇒ every channel. |

Each `tool_attrs` value: `sensitive` / `network` (both optional booleans —
an absent field keeps the house default). A `sensitive` tool is excluded on
a cloud-model brain; a `network` tool on a **private** brain gets the
network shield (private addresses only, validated at the dial).

See `docs/TOOLS-AND-SKILLS.md` for the full guide: what each tool can
reach, how the gate composes, shadow-mode rehearsal, the shield, `/tools`,
and how to write a skill.

## `routes[]`

| Field | Type | Required | Meaning |
|-------|------|----------|---------|
| `channel` | string | **yes** | Name of a configured channel (`telegram`, `discord`, `webhook`). |
| `brain` | string | **yes** | Name of a configured brain. |

```json
{ "channel": "telegram", "brain": "default" }
```

## `approvals` (optional, Trust Layer Etapa 5)

| Field | Type | Required | Meaning |
|---|---|---|---|
| `enabled` | bool | no | Turns the human-approval workflow on. Absent/false ⇒ the E3 honest denial (`approval_unavailable`) stands byte-for-byte. |
| `ttl` | string | no | Request expiry window as a duration (`"1h"` default). Judged when the decision touches the request — an expired approval never executes. |

Requires the `storage` block. Decisions happen on the operator CLI
(`korvun approvals list|show|approve|reject`); an approved action
executes the EXACT stored request and its receipt seals the approval.

## `storage` (optional, ADR-0019)

| Field | Type | Required | Meaning |
|-------|------|----------|---------|
| `path` | string | no | SQLite database file. Empty ⇒ `<os.UserConfigDir>/korvun/korvun.db`. |

Present ⇒ durable, per-conversation memory that survives restarts (including a
graceful shutdown). Absent ⇒ stateless. Under the hardened systemd unit, set
`path` to `/var/lib/korvun/korvun.db` (the `StateDirectory`; see
[`packaging/INSTALL.md`](packaging/INSTALL.md)).

## `session` (optional, operator-console spec 2026-08-08)

| Field | Type | Required | Meaning |
|-------|------|----------|---------|
| `triggers` | string[] | no | Exact-match first-token reset commands. Omitted ⇒ `["/new", "/reset"]`. |
| `daily_at` | string | no | Local-time `"HH:MM"` boundary for daily expiry. Empty ⇒ no daily expiry. |
| `idle_min` | int | no | Idle expiry in whole minutes. `0` ⇒ no idle expiry. |
| `recall_max` | int | no | Enables `/recall` (`0` ⇒ disabled; 1..50): imports the previous session's tail as ONE quoted block — only into an empty session, only on demand. |

Present ⇒ session dispatch is on: a conversation is a series of **sessions**,
the newest one active. A trigger (or a lazy daily/idle expiry, evaluated at the
next inbound — no timers) cuts the context hard: the brain only sees the active
session. Old sessions stay stored and navigable from the Chat tab. **Requires
the `storage` block** (sessions live in the durable store). Absent ⇒ no session
behavior at all; upgrading changes nothing until configured. An empty block
(`"session": {}`) enables the default triggers with no automatic expiry —
that is what the desktop provisions on first run and on upgrade.

## `observability` (optional, ADR-0020)

| Field | Type | Required | Meaning |
|-------|------|----------|---------|
| `enabled` | bool | no | Unset ⇒ `true`. Set `false` to disable the admin server. |
| `addr` | string | no | Bind address. Empty ⇒ `127.0.0.1:2112`. |

The admin server exposes `/metrics` (Prometheus, six `korvun_*` series),
`/healthz` (liveness), the read-only control API (`/api/brains`, `/api/channels`,
ADR-0022), and the live-view SSE + UI (`/api/events`, `/ui`, ADR-0024). It binds
**loopback** by default so a fresh boot exposes nothing to the network. Binding
`0.0.0.0:PORT` is a conscious choice that puts auth/TLS/firewall on the operator
(ADR-0020 §4).

## `admin` (optional, ADR-0028)

| Field | Type | Required | Meaning |
|-------|------|----------|---------|
| `token_env` | string | **yes** (when the block is present) | **Name** of the env var holding the admin bearer token. |

The `admin` block turns on Korvun's **write/mutation surface** — the endpoint that
edits the running config (`POST /api/config`) and the **no-code builder** UI at
`/builder`. Like every other secret, the token is referenced by env-var **name**, and
the value is resolved from the environment at boot (never stored in the file):

```json
{ "admin": { "token_env": "KORVUN_ADMIN_TOKEN" } }
```

Behavior is deliberately safe-by-default (ADR-0028 §1):

- **No `admin` block, or the named variable is unset/empty ⇒ read-only.** The mutation
  endpoints and the builder are **not mounted** — `/builder` returns `404`, and only
  the read-only `/ui` and control API are served.
- **`admin.token_env` present and the variable resolves non-empty ⇒ editing enabled.**
  The builder is served at `/builder` and requests must carry the token as
  `Authorization: Bearer <token>` (constant-time checked, never a cookie).

The bearer token is only safe over the default **loopback** bind (or behind TLS); do
not expose the admin server to the network without it (ADR-0028 §3 / ADR-0020 §4). For
the full walkthrough of enabling and using the builder, see [`BUILDER.md`](BUILDER.md).

## Full example

See [`configs/edge.json`](../configs/edge.json) (Raspberry Pi, local-only,
`private`) and [`configs/cloud.json`](../configs/cloud.json) (server, local +
cloud fan-out). The canonical annotated file is
[`configs/korvun.example.json`](../configs/korvun.example.json).