<p align="center">
  <img src="assets/brand/korvun-logo-hero.svg" width="150" alt="Korvun logo">
</p>

<h1 align="center">Korvun</h1>

<p align="center">
  <strong>Kernel for Orchestrated Routing — Versatile Unified Nodes</strong><br>
  A self-hosted AI messaging gateway, multi-model router, and multi-brain
  orchestrator in a single Go binary.
</p>

<p align="center">
  <a href="https://github.com/Sebastian197/korvun/actions/workflows/quality.yml"><img src="https://github.com/Sebastian197/korvun/actions/workflows/quality.yml/badge.svg" alt="Quality Gate"></a>
  <a href="https://github.com/Sebastian197/korvun/actions/workflows/codeql.yml"><img src="https://github.com/Sebastian197/korvun/actions/workflows/codeql.yml/badge.svg" alt="CodeQL"></a>
  <a href="https://scorecard.dev/viewer/?uri=github.com/Sebastian197/korvun"><img src="https://api.scorecard.dev/projects/github.com/Sebastian197/korvun/badge" alt="OpenSSF Scorecard"></a>
  <a href="https://github.com/Sebastian197/korvun/releases"><img src="https://img.shields.io/github/v/release/Sebastian197/korvun" alt="Latest release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-blue" alt="License: Apache-2.0"></a>
</p>

---

## What is Korvun?

Korvun is one self-hosted Go binary that is a **messaging gateway**, a
**multi-model router**, and a **multi-brain orchestrator** at once. A real message
enters a channel, is routed to a brain, one or more models answer, a policy decides
the reply, and it goes back — all in a single process. The same static binary runs
on a Raspberry Pi and scales in the cloud; only I/O pieces change by configuration.

**The differentiator is the dispatch policy engine.** Routing is privacy- and
cost-aware, with opt-in consensus, as policies of one engine:

- **Privacy** — a brain marked `private` never sends its payload to a cloud model;
  the privacy selector drops cloud providers *before* they are called. What is
  sensitive does not leave the machine.
- **Cost** — everything else is routed to the cheapest capable model, with a
  sequential fail-over that pays a cloud provider only when the local one fails.
- **Consensus (opt-in)** — critical brains can fan out to several models and pick
  the reply by agreement.

## Features

Everything below is **on `master` today** — no roadmap item is counted as present.

- **Channels** — Telegram (polling) and a generic Webhook channel, behind one
  normalized message shape (*Envelope*).
- **Multi-brain orchestration** — several brains coexist; each coordinates multiple
  models in parallel fan-out or cost-saving sequential fail-over, from config.
- **Model providers** — local **Ollama** and cloud **Groq**, behind one `Model`
  interface and a shared sentinel-error grammar.
- **Policy engine** — the privacy / cost / consensus routing above (`PriorityReducer`,
  `ConsensusReducer`, and the pre-dispatch privacy `SelectModels`).
- **Resilience** ([ADR-0031](docs/adr/0031-resilience-timeouts-retry-and-degradation.md))
  — two-layer per-attempt timeouts, retry with differentiated fallback, and an
  optional boot warmup that resolves the cold-start model-load stall.
- **No-code builder** — configure brains, models, and routes visually in the
  browser, no JSON by hand ([BUILDER.md](docs/BUILDER.md)).
- **Observability** — structured `slog`, a Prometheus `/metrics` endpoint, and a
  `/healthz` liveness probe on a loopback admin server.
- **Durable memory** — per-conversation history that survives restarts, graceful
  shutdown included (SQLite by default, behind a `Store` seam).
- **Cross-platform** — one static, pure-Go binary (no cgo) for Linux, macOS, and
  Windows on x86-64 and ARM64.
- **Signed releases** — each release ships cosign keyless signatures over the
  checksums and a per-artifact SBOM (SPDX via Syft).
- **First-class CLI** — `serve`, `config check`, `status`, `version`, `help`:

  ```sh
  korvun serve --config korvun.json            # load config, wire, serve
  korvun config check --preflight korvun.json  # validate offline (+ online checks)
  korvun status                                # live wiring of a running instance
  korvun version                               # build identity
  korvun help                                  # usage
  ```

## Quick start

Grab a signed binary from [releases](https://github.com/Sebastian197/korvun/releases),
then:

```sh
# 1. korvun.example.json — a minimal, valid starting config. It lives in this repo,
#    and is bundled in the release archive from the next release.
korvun config check korvun.example.json      # validate it (offline, no secrets)

# 2. Provide the bot token by environment (never in the config file)
export TELEGRAM_BOT_TOKEN=<your-bot-token>

# 3. Run
korvun serve --config korvun.example.json
```

Full walkthrough — install and verify, configure, message the bot — in
[QUICKSTART.md](docs/QUICKSTART.md). The legacy `korvun -config <path>` invocation
still works via a retrocompat shim; `korvun serve --config …` is canonical.

## Documentation

| Guide | What it covers |
|-------|----------------|
| [Quickstart](docs/QUICKSTART.md) | Zero to a running bot. |
| [Configuration](docs/CONFIGURATION.md) | Every config field, from the schema and ADRs. |
| [No-code builder](docs/BUILDER.md) | Configure Korvun visually in the browser. |
| [Install & run as a service](docs/packaging/INSTALL.md) | Download, verify, hardened systemd unit. |
| [Architecture Decision Records](docs/adr/) | Why each piece is built the way it is (incl. [ADR-0032](docs/adr/0032-cli-interface-contract.md), the CLI contract). |
| [Stage closure docs](docs/stages/) | What is closed, stage by stage. |

## Verifying a release

Releases are signed keyless with [cosign](https://github.com/sigstore/cosign)
(Sigstore) and ship an SBOM. Verify the checksums signature, then check your archive
against the verified `checksums.txt`:

```sh
cosign verify-blob checksums.txt \
  --signature checksums.txt.sig \
  --certificate checksums.txt.pem \
  --certificate-identity-regexp 'https://github.com/Sebastian197/korvun/.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

## Status

**`v0.1.0` is published.** `master` additionally carries production error handling
(ADR-0031 resilience) and the full CLI, both validated on real hardware. Korvun is
in active, staged construction toward a production beta — see
[ROADMAP-V1.md](docs/ROADMAP-V1.md) and [ROAD-TO-BETA.md](docs/ROAD-TO-BETA.md) for
what is closed and what comes next.

## Contributing

Contributions follow strict, non-negotiable conventions (TDD first, Context7 before
any external library, Conventional Commits, `make quality` green with `-race` before
every commit). Read [CONTRIBUTING.md](CONTRIBUTING.md) first.

## Security

Please do **not** open public issues for vulnerabilities. See
[SECURITY.md](SECURITY.md) for the private reporting channel and supported versions.

## License

Licensed under the Apache License 2.0 — see [LICENSE](LICENSE).
