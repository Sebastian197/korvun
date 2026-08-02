# What is Korvun?

Korvun is **one self-hosted Go binary** that is three things at once:

- a **messaging gateway** — Telegram, Discord, and generic webhooks in and out;
- a **multi-model router** — local Ollama and cloud Groq behind one contract;
- a **multi-brain orchestrator** — each conversation routed to a brain with its
  own models, policy, and behavior.

A real message enters a channel, is routed to a brain, one or more models
answer, a policy decides the reply, and it goes back — all in a single
process. The same static binary runs on a Raspberry Pi and scales in the
cloud; only the I/O pieces change, by configuration.

## The differentiator: the dispatch policy engine

Most gateways forward messages. Korvun **decides** — routing is privacy- and
cost-aware, with opt-in consensus, all as policies of one engine:

- **Privacy** — a brain marked `private` never sends its payload to a cloud
  model. The privacy selector drops cloud providers *before* they are called,
  so what is sensitive does not leave the machine. The visual builder even
  draws the exclusion: a gray dashed cable to every excluded cloud model.
- **Cost** — with `sequential` dispatch, models are tried in order and a paid
  provider is only contacted when the local one fails.
- **Consensus (opt-in)** — critical brains can fan out to several models and
  pick the reply a strict majority agrees on.

## What ships today

Everything here is in the current release — nothing on this page is roadmap:

- **Channels**: Telegram (polling), Discord (Gateway in, REST out), and a
  generic [webhook endpoint](/channels/webhook) for anything that can POST
  JSON.
- **Models**: local [Ollama](https://ollama.com) and cloud Groq, mixable in
  one brain.
- **Brains**: several coexist, each with parallel fan-out or cost-saving
  sequential fail-over.
- **The no-code builder**: configure channels, brains, and models
  [visually in the browser](/guide/builder) — including a drag-and-drop
  canvas — with safe live reload, no restart.
- **Korvun Desktop**: the same core behind a native window for macOS,
  Windows, and Linux — onboarding included, secrets in the OS keychain.
- **Durable memory**: per-conversation history that survives restarts
  (SQLite), optional.
- **Observability**: structured logs, Prometheus `/metrics`, `/healthz` —
  on a loopback-only admin server by default.
- **Signed releases**: every artifact covered by a cosign-signed checksum
  manifest plus an SBOM, for six platforms.

## Where next

- Get a bot answering from a local model in minutes → [Quickstart](/guide/quickstart)
- Install the binary or the desktop app → [Install](/guide/install)
- Every config field, explained → [Configuration reference](/reference/configuration)