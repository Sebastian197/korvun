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
> missing secret is a loud, named boot error.

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
| `type` | string | **yes** | Adapter. Supported: `telegram`, `discord`. |
| `mode` | string | **yes** | Transport. `telegram` → `polling`; `discord` → `gateway`. |
| `token_env` | string | **yes** | **Name** of the env var holding the bot token. |

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
| `provider` | string | **yes** | `ollama` \| `groq`. |
| `model_id` | string | **yes** | The provider's model name (e.g. `llama3.2`). |
| `locality` | string | **yes** | `local` \| `cloud`. **Declared**, not derived — the privacy selector routes on it (ADR-0015 §3). |
| `base_url` | string | no | Override the adapter default (e.g. `http://localhost:11434`). |
| `api_key_env` | string | cloud only | **Name** of the env var holding the API key. **Required for `groq`.** |

### `brains[].agent` (optional, ADR-0021)

Present ⇒ this brain is a bounded tool-use agent instead of the fan-out
orchestrator. Both satisfy `brain.Brain`, so routing is unchanged.

| Field | Type | Required | Meaning |
|-------|------|----------|---------|
| `tools` | array of string | **yes** (≥1) | Built-in tools to register: `time`, `echo`, `calc` (the safe, pure set). |
| `max_iterations` | int | no | Hard loop cap. `0` ⇒ the AgentBrain default. |
| `system_prompt` | string | no | Operator prompt appended after the protocol block. |

## `routes[]`

| Field | Type | Required | Meaning |
|-------|------|----------|---------|
| `channel` | string | **yes** | Name of a configured channel (`telegram`, `discord`). |
| `brain` | string | **yes** | Name of a configured brain. |

```json
{ "channel": "telegram", "brain": "default" }
```

## `storage` (optional, ADR-0019)

| Field | Type | Required | Meaning |
|-------|------|----------|---------|
| `path` | string | no | SQLite database file. Empty ⇒ `<os.UserConfigDir>/korvun/korvun.db`. |

Present ⇒ durable, per-conversation memory that survives restarts (including a
graceful shutdown). Absent ⇒ stateless. Under the hardened systemd unit, set
`path` to `/var/lib/korvun/korvun.db` (the `StateDirectory`; see
[`packaging/INSTALL.md`](packaging/INSTALL.md)).

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