# Korvun quickstart

Zero to a Telegram bot answering from a **local** model, end to end. This is the
**install-a-release** path (no Go toolchain); building from source is a short note
at the end.

> **Validated end to end on real hardware** (iMac, Intel `x86_64`, macOS 13, Ollama
> with `llama3.2:1b`): installed the `v0.1.0` binary, wrote the config below, wired
> a real Telegram bot to the local model, and `hola` → `Hola. ¿En qué puedo
> ayudarte?` came back from the bot — answered by the local model, **zero cloud**.

## What you need first

- **The `korvun` binary installed.** Follow the macOS walkthrough in
  [`packaging/INSTALL.md`](packaging/INSTALL.md#macos--full-walkthrough-validated-on-intel-hardware)
  (download → verify checksum → run). Confirm with `korvun --version`.
- **Ollama running, with the model pulled** (see step 2).
- **A Telegram bot token** from [@BotFather](https://t.me/BotFather) (`/newbot`).

## Step 1 — Install the binary

Covered in [`packaging/INSTALL.md`](packaging/INSTALL.md). At the end you have a
working `korvun` (`./korvun --version` prints `korvun v0.1.0 (…)`).

## Step 2 — Start Ollama and pull the model

Korvun talks to a local [Ollama](https://ollama.com) at `http://127.0.0.1:11434`
(the default). In a **separate terminal window**, keep Ollama running:

```sh
ollama serve
```

Then pull the model this quickstart uses:

```sh
ollama pull llama3.2:1b
```

> **Warm the model once** (strongly recommended — see [Troubleshooting](#troubleshooting)):
> a cold model can be too slow to load on the first request and time out. Warm it
> with one interactive run, then quit it:
>
> ```sh
> ollama run llama3.2:1b   # type a word, get a reply, then /bye
> ```

## Step 3 — Create `korvun.local.json`

> The `v0.1.0` release archive does **not** ship an example config, so create this
> file yourself. Every field name below is exact — verified against the config
> parser (`internal/config`). It is the canonical minimal config: one Telegram
> channel, one brain, one local model.

```json
{
  "channels": [
    { "type": "telegram", "mode": "polling", "token_env": "TELEGRAM_TOKEN" }
  ],
  "brains": [
    {
      "name": "assistant",
      "sensitivity": "public",
      "policy": { "kind": "priority" },
      "models": [
        { "provider": "ollama", "model_id": "llama3.2:1b", "locality": "local" }
      ]
    }
  ],
  "routes": [
    { "channel": "telegram", "brain": "assistant" }
  ]
}
```

What each field is:

- **`channels[].type`** = `"telegram"` — the channel adapter (the only one this build
  wires). A telegram channel registers under this **type name**.
- **`channels[].mode`** = `"polling"` — the transport (the only one supported here).
- **`channels[].token_env`** = `"TELEGRAM_TOKEN"` — the **NAME** of the environment
  variable holding the bot token, never the token itself (step 4).
- **`brains[].name`** = `"assistant"` — a unique brain name the route points to.
- **`brains[].sensitivity`** = `"public"` — **required.** Privacy constraint:
  `public` = no filter; `private` = drop cloud models before dispatch. With only a
  local model both boot the same; use `private` to guarantee nothing ever leaves the
  box.
- **`brains[].policy`** = `{ "kind": "priority" }` — **required, and it is an OBJECT,
  not a string.** `priority` picks the reply from the highest-priority provider that
  answered (here, the only one). *(Passing `"policy": "priority"` is rejected —
  `policy` is a `PolicyConfig` object.)*
- **`brains[].models[].provider`** = `"ollama"` — the provider (`ollama` | `groq`).
- **`brains[].models[].model_id`** = `"llama3.2:1b"` — the model name at the provider.
  *(The field is `model_id`, not `model`.)*
- **`brains[].models[].locality`** = `"local"` — declared, not derived; the privacy
  selector routes on it (`local` | `cloud`).
- **`routes[].channel`** = `"telegram"` — **the channel's type name**, not an invented
  name (a telegram channel registers as `"telegram"`).
- **`routes[].brain`** = `"assistant"` — the brain name above.

Omitted on purpose (all optional): `dispatch` (defaults to `fanout`),
`models[].base_url` (defaults to `http://127.0.0.1:11434`), `storage` (absent ⇒
stateless), `observability` (absent ⇒ on, loopback `127.0.0.1:2112`), `admin`.
Every field is documented in [`CONFIGURATION.md`](CONFIGURATION.md).

## Step 4 — Export the token by environment variable

The config names the env var (`token_env`); the **value** goes in the environment,
**never in the JSON**:

```sh
export TELEGRAM_TOKEN=<your-bot-token>
```

> ### ⚠️ The bot token is a secret — do not expose it
>
> Anyone with your token can control your bot. When you export it, make sure no one
> can see your screen or shell history, and **never paste it into a file, a chat, a
> screenshot, or a log.** If it is ever exposed, **revoke it immediately**: in
> [@BotFather](https://t.me/BotFather) → `/mybots` → select your bot → **API Token**
> → **Revoke current token**, then export the new one.

## Step 5 — Run Korvun

```sh
./korvun serve --config korvun.local.json
```

Korvun loads the config, resolves the env-only token, runs a boot health-check, and
serves until `SIGINT`/`SIGTERM` (`Ctrl-C`), shutting down cleanly.

## Step 6 — Message the bot

Open your bot in Telegram and send `hola`. The local model's reply comes back in the
chat — no cloud involved.

---

## Troubleshooting

### Check your config before running

Validate a config **offline** — structure, enum values, and timeouts, with no
network and no secrets read:

```sh
korvun config check korvun.local.json
```

A clean config prints an `OK` line (exit 0); a malformed or invalid one prints the
first offending field path (exit 2). Add `--preflight` to also resolve the env-only
secrets, run the per-brain privacy selector, and reach the channel/providers:

```sh
korvun config check --preflight korvun.local.json
```

### Inspect a running Korvun

While Korvun is serving, see its live wiring — brains, the models that survived the
privacy selector, channels, and drop counts — through the read-only admin API:

```sh
korvun status
```

Point it elsewhere with `--addr host:port` (default `127.0.0.1:2112`). If it prints
`admin API not reachable`, Korvun is not running, or observability is disabled.

### The first message fails / "Sorry, no answer is available" / `context deadline exceeded`

If the bot replies **"Sorry, no answer is available right now. Please try again."**
or the log shows something like:

```
brain: no usable answer ... "model: provider unavailable:
Post http://127.0.0.1:11434/api/chat: context deadline exceeded"
```

…the model was **too slow to load on a cold start**. Korvun's timeout to the
provider (~5s) is shorter than the time a first-time model load can take on some
hardware — Ollama logs `client connection closed before llama-server finished
loading` and cancels the `POST /api/chat` at ~5s.

**Fix for the quickstart:** warm the model once and retry:

```sh
ollama run llama3.2:1b   # type a word, get a reply, then /bye
```

With the model already warm, the bot answers immediately. *(This cold-start timeout
is a known product limitation, tracked as motivation for configurable/retrying
provider timeouts — see `ROAD-TO-BETA.md`, Pieza 2. It is not a config error on your
side.)*

### A `DeleteWebhook` WARN at startup

On startup in polling mode you may see a warning mentioning
`telegram: DeleteWebhook safety-net call failed`. It is **expected and harmless** —
Korvun proactively clears any leftover webhook before polling; on a bot that never
had one, the safety-net call can warn without affecting polling. You can ignore it.

### Verification / Gatekeeper issues on install

Checksum, cosign, and macOS Gatekeeper are covered in
[`packaging/INSTALL.md`](packaging/INSTALL.md).

---

## Alternative: build from source

Requires **Go 1.26.4+** (see [`go.mod`](../go.mod)):

```sh
make build          # or: go build ./cmd/korvun
```

From a source checkout you can also start from the profiles in
[`configs/`](../configs/) (e.g. [`configs/edge.json`](../configs/edge.json),
[`configs/cloud.json`](../configs/cloud.json)) instead of writing the config by hand.

## Next

- **Configure it visually, no JSON** → once Korvun is running, edit brains, models,
  and routes from the browser with the no-code builder → [`BUILDER.md`](BUILDER.md)
- **What every config field does** → [`CONFIGURATION.md`](CONFIGURATION.md)
- **Install / verify / run as a service** → [`packaging/INSTALL.md`](packaging/INSTALL.md)
- **Why it is built this way** → the ADRs in [`adr/`](adr/)
- **What is closed, stage by stage** → [`stages/`](stages/)
