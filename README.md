# Korvun

**Kernel for Orchestrated Routing — Versatile Unified Nodes**

A single self-hosted Go binary that is an AI messaging gateway, a multi-model
router, and a multi-brain orchestrator at once — driven by a configurable
dispatch policy engine (privacy / cost / consensus). The same binary runs on a
Raspberry Pi and scales in the cloud; only I/O pieces change by configuration.

[![Quality Gate](https://github.com/Sebastian197/korvun/actions/workflows/quality.yml/badge.svg)](https://github.com/Sebastian197/korvun/actions/workflows/quality.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/Sebastian197/korvun)](https://goreportcard.com/report/github.com/Sebastian197/korvun)
[![Go Version](https://img.shields.io/github/go-mod/go-version/Sebastian197/korvun)](go.mod)
[![License](https://img.shields.io/github/license/Sebastian197/korvun)](LICENSE)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/Sebastian197/korvun/badge)](https://scorecard.dev/viewer/?uri=github.com/Sebastian197/korvun)
[![Latest Release](https://img.shields.io/github/v/release/Sebastian197/korvun)](https://github.com/Sebastian197/korvun/releases)

> **Status:** in active, staged construction. The core path is live — a real
> message enters a channel, is routed, several models answer, a policy decides,
> and the reply goes back, all in one binary. See [`docs/stages/`](docs/stages/)
> for what is closed.

## Features

- **Universal messaging gateway.** Connects channels (Telegram today; a generic
  webhook channel for any web/API; WhatsApp, Discord, Slack and others planned)
  behind a normalized message shape (*Envelope*).
- **Multi-model routing.** One gate to local and cloud models (Ollama and Groq
  today) with automatic, per-message dispatch decisions.
- **Multi-brain orchestration.** Several orchestrators ("brains") coexist; each
  coordinates multiple models — in parallel fan-out or cost-saving sequential
  fail-over — selected from configuration.
- **Dispatch policy engine (the differentiator).** Privacy- and cost-aware
  routing with opt-in consensus, as policies of one engine: sensitive payloads
  stay on local models, trivial ones go to the cheapest, critical ones can be
  dispatched to several models for agreement. Every decision is audited.
- **Durable conversation memory.** Per-conversation history that survives
  restarts, including a graceful shutdown (SQLite by default, behind a `Store`
  seam — see [ADR-0018](docs/adr/0018-conversation-store-interface.md) /
  [ADR-0019](docs/adr/0019-sqlite-conversation-store.md)).
- **Self-hosted, cross-platform.** Linux, Windows and macOS; x86-64 and ARM64;
  pure-Go, no cgo. Secrets are environment-only by reference, never in config.

## Architecture

Korvun is one long-running process wiring a single path:

```
channel → router → brain → (model fan-out / sequential) → policy → channel
```

- **`internal/envelope`** — the canonical, channel-agnostic message event.
- **`internal/channel`** — channel abstraction; `telegram/` and `webhook/`
  adapters.
- **`internal/router`** — gateway core; owns the inbound pump, workers and
  conversation-key composition.
- **`internal/brain`** — the `Orchestrator` (stateless glue): translate →
  coordinate → apply policy → translate.
- **`internal/model`** — the `Model` interface and sentinel-error grammar;
  `ollama/`, `groq/`, `fanout/`, `sequential/` adapters/coordinators.
- **`internal/policy`** — `Policy`/`Decision` contract; `PriorityReducer`,
  `ConsensusReducer`, and the pre-dispatch privacy `SelectModels`.
- **`internal/conversation`** — the append-only `Store` seam, in-memory
  `MemStore`, and the durable `sqlite/` store.
- **`cmd/korvun`** — the thin binary: load config, build the app, serve until a
  signal.

Design rationale lives in the ADRs ([`docs/adr/`](docs/adr/)); each closed stage
has a closure doc ([`docs/stages/`](docs/stages/)). The road to a production V1
is tracked in [`docs/ROADMAP-V1.md`](docs/ROADMAP-V1.md).

## Quickstart

Requires **Go 1.26.4+** (see [`go.mod`](go.mod)).

```sh
# Build the binary
make build          # or: go build ./cmd/korvun

# Provide secrets by environment (never in the config file)
export TELEGRAM_BOT_TOKEN=...     # your Telegram bot token
export GROQ_API_KEY=...           # optional, only if you wire a Groq model

# Run against a config file
./korvun -config configs/korvun.example.json
```

`korvun` loads the JSON config, resolves env-only secrets, runs a boot
health-check, and serves until `SIGINT`/`SIGTERM`, then shuts down cleanly.

A minimal config wiring a Telegram channel to a brain with a local Ollama model
falling back to cloud Groq, choosing the reply by provider priority:

```json
{
  "channels": [
    { "type": "telegram", "mode": "polling", "token_env": "TELEGRAM_BOT_TOKEN" }
  ],
  "brains": [
    {
      "name": "default",
      "sensitivity": "public",
      "dispatch": "fanout",
      "policy": { "kind": "priority", "order": ["ollama", "groq"] },
      "models": [
        { "provider": "ollama", "model_id": "llama3.2", "locality": "local",
          "base_url": "http://localhost:11434" },
        { "provider": "groq", "model_id": "llama-3.3-70b-versatile",
          "locality": "cloud", "api_key_env": "GROQ_API_KEY" }
      ]
    }
  ],
  "routes": [ { "channel": "telegram", "brain": "default" } ]
}
```

## Configuration

- **Format:** a single JSON file, passed with `-config` (default `korvun.json`).
  See [`configs/korvun.example.json`](configs/korvun.example.json).
- **Secrets are environment-only by reference.** Fields like `token_env` and
  `api_key_env` hold the **name** of an environment variable, never the secret
  value. Secrets are never read from argv, the config file, logs, or error
  messages (ADR-0010 §3).
- **Conversation storage** is optional and additive: a top-level `storage.path`
  selects the SQLite file; empty falls back to `<os.UserConfigDir>/korvun/
  korvun.db`. With no store configured, Korvun runs stateless.
- **Same binary, different profile.** I/O pieces (persistence, future event bus)
  switch by configuration, not by recompiling — the basis for the planned
  `edge`/`cloud` profiles.

## Documentation

| Guide | What it covers |
|-------|----------------|
| [Quickstart](docs/QUICKSTART.md) | Zero to a running bot — install a release or build from source, configure, run. |
| [Configuration reference](docs/CONFIGURATION.md) | Every config field, distilled from the schema and ADRs. |
| [Install & run as a service](docs/packaging/INSTALL.md) | Download, checksum + signature verification, hardened systemd unit. |
| [Architecture Decision Records](docs/adr/) | Why each piece is built the way it is. |
| [Stage closure docs](docs/stages/) | What is closed, stage by stage. |
| [V1 roadmap](docs/ROADMAP-V1.md) | The road to a production V1. |

## Contributing

Contributions follow strict, non-negotiable conventions (TDD first, Context7
before any external library, Conventional Commits, `make quality` green with
`-race` before every commit). Read [CONTRIBUTING.md](CONTRIBUTING.md) before
opening a PR.

## Security

Please do **not** open public issues for vulnerabilities. See
[SECURITY.md](SECURITY.md) for the private reporting channel, supported versions,
and expected response times.

## License

Licensed under the Apache License 2.0 — see [LICENSE](LICENSE).
