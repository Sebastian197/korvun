# Quickstart

Zero to a Telegram bot answering from a **local** model — no cloud involved.
This flow was validated end to end on real hardware, and it is the same on
Linux, macOS, and Windows.

## What you need first

- **The `korvun` binary installed** — see [Install](/guide/install). Confirm
  with `korvun --version`.
- **[Ollama](https://ollama.com)** for the local model (step 1).
- **A Telegram bot token** from [@BotFather](https://t.me/BotFather)
  (`/newbot`).

## Step 1 — Start Ollama and pull a model

Korvun talks to a local Ollama at `http://127.0.0.1:11434` (the default).
Keep Ollama running in a separate terminal:

```sh
ollama serve
ollama pull llama3.2:1b
```

## Step 2 — Create `korvun.local.json`

The release archive bundles `korvun.example.json` — the same minimal config —
so you can copy and adapt it instead of typing. One Telegram channel, one
brain, one local model:

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

The fields that matter most:

- **`token_env`** — the **name** of the environment variable holding the bot
  token. The token itself never goes in this file (step 3).
- **`sensitivity`** — `public` (no filter) or `private` (cloud models are
  dropped before dispatch: nothing leaves the box). With only a local model
  both behave the same; use `private` to make the guarantee explicit.
- **`policy`** — an **object**, not a string: `{ "kind": "priority" }` picks
  the reply from the highest-priority provider that answered.
- **`routes[].channel`** — the channel's **type name** (`telegram`), not an
  invented label.

Every field is documented in the
[configuration reference](/reference/configuration).

## Step 3 — Export the token

The config names the variable; the **value** lives only in the environment:

```sh
export TELEGRAM_TOKEN=<your-bot-token>
```

On Windows (PowerShell): `$env:TELEGRAM_TOKEN = "<your-bot-token>"`.

> **The bot token is a secret.** Anyone with it controls your bot. Never
> paste it into a file, a chat, a screenshot, or a log. If it is ever
> exposed, revoke it in @BotFather (`/mybots` → your bot → **API Token** →
> **Revoke current token**) and export the new one.

## Step 4 — Check, run, message

Validate the config offline first — structure and values, no network, no
secrets read:

```sh
korvun config check korvun.local.json      # OK -> exit 0
```

Add `--preflight` to also confirm the env var resolves and the providers are
reachable. Then run:

```sh
korvun serve --config korvun.local.json
```

Open your bot in Telegram and send it a message. The local model's reply
comes back in the chat — zero cloud. Korvun serves until `Ctrl-C` and shuts
down cleanly.

## Troubleshooting

- **See the live wiring** of a running Korvun — brains, models that survived
  the privacy selector, channels:

  ```sh
  korvun status
  ```

  Default address `127.0.0.1:2112`; point elsewhere with `--addr host:port`.

- **A `DeleteWebhook` WARN at startup** is expected and harmless — Korvun
  proactively clears any leftover Telegram webhook before polling.

- **The first message is slow on modest hardware?** Korvun warms local models
  at boot and retries transient failures with generous timeouts, so a cold
  model load is handled for you. If the very first answer still takes a few
  seconds, that is the model loading — the next ones are immediate.

## Building from source instead

Requires **Go 1.26.5+**:

```sh
make build          # or: go build ./cmd/korvun
```

## Next

- Configure it **visually, no JSON** → [the builder](/guide/builder)
- Add [Discord](/channels/discord) or a [webhook](/channels/webhook)
- What every field does → [configuration reference](/reference/configuration)
