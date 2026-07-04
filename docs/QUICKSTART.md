# Korvun quickstart

Zero to a running bot. Two paths: install a **release** (no toolchain) or **build
from source**. Then configure and run.

## A. Install a release (recommended)

Korvun is a single static binary — no runtime dependencies, no Go toolchain.

```sh
# 1. Download the archive for your OS/arch + the checksums (VERSION e.g. v0.1.0).
gh release download VERSION --pattern 'korvun_*_linux_arm64.tar.gz'
gh release download VERSION --pattern 'checksums.txt*'

# 2. Verify (checksum + signature — see packaging/INSTALL.md for cosign details).
sha256sum -c checksums.txt --ignore-missing

# 3. Extract and place on PATH.
tar -xzf korvun_*_linux_arm64.tar.gz
sudo install -m755 korvun /usr/local/bin/korvun
korvun --version
```

Targets: `linux` / `darwin` / `windows`, each `amd64` + `arm64` (a 64-bit
Raspberry Pi is `linux/arm64`). Full detail — signature verification, service
install — in [`packaging/INSTALL.md`](packaging/INSTALL.md).

## B. Build from source

Requires **Go 1.26.4+** (see [`go.mod`](../go.mod)).

```sh
make build          # or: go build ./cmd/korvun
```

## Configure

Korvun reads one JSON file. Start from a profile in [`configs/`](../configs/):

- [`configs/edge.json`](../configs/edge.json) — Raspberry Pi / small box: one
  local Ollama model, `sensitivity: private` (dispatch stays local-only), memory on.
- [`configs/cloud.json`](../configs/cloud.json) — server / VM: local Ollama + a
  cloud Groq model in fan-out, memory on, observability on loopback.

Every field is documented in [`CONFIGURATION.md`](CONFIGURATION.md).

## Run

Secrets are environment variables, **by name** — export them, never inline:

```sh
export TELEGRAM_BOT_TOKEN=...   # the value the config's "token_env" points to
export GROQ_API_KEY=...         # only if a Groq model is configured
korvun -config configs/cloud.json
```

Korvun loads the config, resolves env-only secrets, runs a boot health-check, and
serves until `SIGINT`/`SIGTERM`, then shuts down cleanly (draining durable memory).
Send a message to your Telegram bot and the model's reply comes back in the chat.

To run it as a hardened service on Linux, see the systemd unit in
[`packaging/INSTALL.md`](packaging/INSTALL.md) §5.

## Next

- **What every config field does** → [`CONFIGURATION.md`](CONFIGURATION.md)
- **Install / verify / run as a service** → [`packaging/INSTALL.md`](packaging/INSTALL.md)
- **Why it is built this way** → the ADRs in [`adr/`](adr/)
- **What is closed, stage by stage** → [`stages/`](stages/)