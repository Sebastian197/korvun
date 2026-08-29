# Configuration reference

Korvun reads **one JSON file**, passed with `--config` (default
`korvun.json`):

```sh
korvun serve --config /etc/korvun/korvun.json
```

The field shape is a **contract**: once you write a config, the names and
structure are stable across releases. Validate any file offline with
`korvun config check <file>` (add `--preflight` to also resolve secrets and
reach the providers).

> **Secrets are environment variables, by NAME — never by value.** Fields
> ending in `_env` (`token_env`, `api_key_env`) hold the **name** of an
> environment variable; Korvun reads the value at boot. A secret is never
> read from the command line, the config file, logs, or error messages. A
> missing secret is a loud, named boot error. On Korvun Desktop, the
> **SECRETOS** card in Ajustes manages the values behind those names over
> the OS keychain — write-only (no value is ever displayed or returned),
> with presence shown per name; a value set in the environment wins over
> the keychain, and the card says so. It works even while the core is
> stopped — exactly when a boot broken by a missing secret needs it.

## Top level

| Field | Type | Required | Meaning |
|---|---|---|---|
| `channels` | array | **yes** (≥1) | Messaging channels to run. |
| `brains` | array | **yes** (≥1) | Orchestrating brains. |
| `routes` | array | **yes** (≥1) | Bindings of a channel to a brain. |
| `storage` | object | no | Durable conversation store. **Absent ⇒ stateless.** |
| `observability` | object | no | Admin HTTP server. **Absent ⇒ ON (loopback).** |
| `admin` | object | no | The write surface + the builder. **Absent ⇒ read-only.** |

The asymmetry is deliberate: no `storage` means *off* (stateless), no
`observability` means *on* with safe loopback defaults, no `admin` means
*read-only* — each the safe default.

## `channels[]`

| Field | Type | Required | Values / meaning |
|---|---|---|---|
| `type` | string | **yes** | `telegram`, `discord`, or `webhook`. |
| `mode` | string | conditional | `telegram` → `polling`; `discord` → `gateway`; **`webhook` takes no `mode`**. |
| `token_env` | string | **yes** | **Name** of the env var holding the channel's secret (bot token, or the webhook's inbound Bearer secret). |
| `webhook` | object | webhook only | The webhook block (below). |

A channel registers under its `type` as its name — that is the value
`routes` reference.

```json
{ "type": "telegram", "mode": "polling", "token_env": "TELEGRAM_BOT_TOKEN" }
```

```json
{ "type": "discord", "mode": "gateway", "token_env": "DISCORD_BOT_TOKEN" }
```

Discord needs one manual switch in the Developer Portal — the
[Discord guide](/channels/discord) walks through it.

### The `webhook` block

| Field | Type | Required | Values / meaning |
|---|---|---|---|
| `bind` | string | no | Listen address. Default **`127.0.0.1:8090`** (loopback). A non-loopback bind warns at boot. |
| `path` | string | no | Inbound POST path. Default **`/webhook`**. |
| `outbound_url` | string | **yes** | Where brain replies are POSTed. |
| `outbound_token_env` | string | no | **Name** of the env var holding an outbound Bearer secret. If named, it must resolve at boot. |
| `mapping` | object | no | Your JSON field names → Korvun's message fields. |

Edge validation on every request: `POST` only (405), `application/json`
(415), body ≤ 1 MiB (413), full buffer answers 503 (retry later). The
[webhook guide](/channels/webhook) is the zero-to-round-trip path.

## `brains[]`

| Field | Type | Required | Values / meaning |
|---|---|---|---|
| `name` | string | **yes** | Unique name, referenced by `routes`. |
| `sensitivity` | string | **yes** | `public` \| `private`. **`private` drops cloud models before dispatch** — sensitive payloads never leave the box. |
| `dispatch` | string | no | `fanout` (default: all models in parallel) \| `sequential` (in order, stop at first success — a paid provider is only contacted if the local one failed). |
| `policy` | object | **yes** | The reducer that picks the reply (below). |
| `models` | array | **yes** (≥1) | The provider catalog for this brain. |
| `agent` | object | no | Mounts a bounded tool-use agent instead of the default orchestrator. |

### `brains[].policy`

| Field | Type | Required | Values / meaning |
|---|---|---|---|
| `kind` | string | **yes** | `priority` \| `consensus`. |
| `order` | array | no | Provider priority list both reducers use. |

- **`priority`** — the reply from the highest-priority provider that
  answered, in `order`.
- **`consensus`** — the answer a strict majority of successful providers
  agree on (needs at least two; a tie or a lone success ⇒ no consensus).

### `brains[].models[]`

| Field | Type | Required | Values / meaning |
|---|---|---|---|
| `provider` | string | **yes** | `ollama` \| `groq`. |
| `model_id` | string | **yes** | The provider's model name (e.g. `llama3.2`). |
| `locality` | string | **yes** | `local` \| `cloud` — **declared**, not derived; the privacy selector routes on it. |
| `base_url` | string | no | Override the adapter default (Ollama: `http://127.0.0.1:11434`). |
| `api_key_env` | string | cloud only | **Name** of the env var holding the API key. **Required for `groq`.** |

### `brains[].agent` (optional)

Present ⇒ the brain is a bounded tool-use agent.

| Field | Type | Required | Meaning |
|---|---|---|---|
| `tools` | array | **yes** (≥1) | Built-in tools to register. Pure: `time`, `echo`, `calc`. Caged (each REQUIRES its cage block): `read_file`, `http_fetch`, `webhook_call`, `memory_note`. |
| `max_iterations` | int | no | Hard loop cap. |
| `system_prompt` | string | no | Operator prompt appended after the protocol block. |
| `governance` | array | no | Tri-state grants — `tool`, `mode` (`allow` \| `shadow` \| `deny`), optional `channels`. Absent ⇒ ungoverned: every listed tool allowed on every channel. |
| `read_file` | object | with the tool | The jail: `root` (**required**), `max_bytes`. |
| `http_fetch` | object | with the tool | The cage: `allow_hosts` (**required**), `max_bytes`, `max_redirects`. |
| `webhook_call` | object | with the tool | The cage: `allow_hosts` (**required**), `max_bytes`, `timeout_seconds`. |
| `memory` | object | with the tool | The memory block for `memory_note`: `scope` (`conversation` default \| `brain`), `max_notes`, `max_note_runes`, `budget_runes`. Requires `storage`; `scope: "brain"` requires the brain's selected model to be **local**. |
| `skills_dir` | string | no | AgentSkills-compatible skills directory. |
| `skills_body_budget` | int | no | Total rune budget for injected skill bodies. |

The full operating guide — what each tool can reach, shadow rehearsal, the
network shield, `/tools`, writing a skill — is on
[governed tools and skills](/guide/tools-and-skills); notes and recall are
on [governed memory](/guide/memory).

## `routes[]`

| Field | Type | Required | Meaning |
|---|---|---|---|
| `channel` | string | **yes** | A configured channel's type name. |
| `brain` | string | **yes** | A configured brain's name. |

```json
{ "channel": "telegram", "brain": "assistant" }
```

## `storage` (optional)

| Field | Type | Required | Meaning |
|---|---|---|---|
| `path` | string | no | SQLite file. Empty ⇒ `<os user config dir>/korvun/korvun.db`. |

Present ⇒ durable per-conversation memory that survives restarts. Absent ⇒
stateless. Under the hardened systemd unit, use `/var/lib/korvun/korvun.db`.

## `session` (optional)

| Field | Type | Required | Meaning |
|---|---|---|---|
| `triggers` | array | no | Exact-match first-token reset commands. Omitted ⇒ `["/new", "/reset"]`. |
| `daily_at` | string | no | Local-time `"HH:MM"` boundary for daily expiry. Empty ⇒ none. |
| `idle_min` | int | no | Idle expiry in whole minutes. `0` ⇒ none. |
| `recall_max` | int | no | Enables `/recall` (`0` ⇒ disabled; 1..50): imports the previous session's tail as ONE quoted block — only into an empty session, only on demand. |

Present ⇒ session dispatch is on: a conversation is a series of sessions,
the newest one active, and a trigger (or a lazy daily/idle expiry) cuts the
context hard. **Requires the `storage` block.** Absent ⇒ no session
behavior at all. An empty block (`"session": {}`) enables the default
triggers with no automatic expiry — that is what the desktop provisions.
The operator's view of sessions, `/recall` and `/notes` is on
[the operator console](/guide/chat).

## `observability` (optional)

| Field | Type | Required | Meaning |
|---|---|---|---|
| `enabled` | bool | no | Unset ⇒ `true`. |
| `addr` | string | no | Bind address. Empty ⇒ `127.0.0.1:2112`. |

The admin server exposes `/metrics` (Prometheus), `/healthz`, the read-only
control API, and the live view at `/ui`. It binds **loopback** by default so
a fresh boot exposes nothing to the network; binding `0.0.0.0` is a conscious
choice that puts auth/TLS/firewall on you.

## `admin` (optional)

| Field | Type | Required | Meaning |
|---|---|---|---|
| `token_env` | string | **yes** (when present) | **Name** of the env var holding the admin bearer token. |

The `admin` block turns on the write surface and the
[visual builder](/guide/builder) at `/builder`. No block, or the variable
unset ⇒ read-only: the builder is not mounted at all. The token travels as
`Authorization: Bearer` (constant-time checked, never a cookie) and is only
safe over the default loopback bind or behind TLS.

```json
{ "admin": { "token_env": "KORVUN_ADMIN_TOKEN" } }
```
