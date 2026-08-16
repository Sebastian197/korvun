# Telegram

The fastest channel to try Korvun with: create a bot with BotFather, export
the token, add three lines of config. Korvun receives updates by **polling**,
so it needs no public URL, no TLS setup, nothing exposed — it works from a
laptop behind NAT.

## Step 1 — Create the bot with @BotFather

1. Open [@BotFather](https://t.me/BotFather) in Telegram.
2. Send `/newbot`, pick a display name and a unique username.
3. BotFather answers with the **bot token** — copy it.

> **The token is a secret.** Anyone with it controls your bot. It goes in an
> environment variable — never in the config file, a commit, or a chat. If
> it ever leaks, revoke it in @BotFather: `/mybots` → your bot →
> **API Token** → **Revoke current token**.

## Step 2 — Configure the channel

Add a `telegram` channel and a route to your config. `token_env` is the
**name** of the environment variable — the value never appears here:

```json
{
  "channels": [
    { "type": "telegram", "mode": "polling", "token_env": "TELEGRAM_TOKEN" }
  ],
  "routes": [
    { "channel": "telegram", "brain": "assistant" }
  ]
}
```

`mode` is always `"polling"`, and `routes[].channel` is the channel's **type
name** (`telegram`). The brain is whatever you configured — the
[quickstart](/guide/quickstart) has a complete minimal file.

## Step 3 — Export the token and run

```sh
export TELEGRAM_TOKEN=<your-bot-token>
korvun config check --preflight korvun.local.json   # confirms the var resolves
korvun serve --config korvun.local.json
```

Open your bot in Telegram and send a message — the reply comes back in the
chat, answered by whatever models the routed brain runs.

## Good to know

- **A `DeleteWebhook` WARN at startup is expected and harmless** — Korvun
  proactively clears any leftover webhook before polling; on a bot that
  never had one, the safety-net call can warn without affecting anything.
- Text in, text out is the v1 surface.

## Next

- [Discord](/channels/discord) · [Webhook](/channels/webhook)
- Every channel field → [configuration reference](/reference/configuration)
