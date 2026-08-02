# Discord

Korvun receives Discord messages over the **Gateway** (a WebSocket, with
automatic reconnect and resume) and replies over REST. Setup is a few clicks
in the Developer Portal — plus **one manual switch that is easy to miss**:
the Message Content intent.

## Step 1 — Create the application and bot

1. Open the [Discord Developer Portal](https://discord.com/developers/applications).
2. **New Application** → name it → **Create**.
3. Open the **Bot** tab in the left sidebar.

## Step 2 — Copy the bot token

On the **Bot** tab, under **Token**, click **Reset Token** to reveal it, and
copy it.

> **The token is a secret.** It goes in an environment variable — never in
> the config file, a commit, or a chat. If it leaks, come back here and
> **Reset Token**.

## Step 3 — Turn ON the Message Content intent ⚠️

**The single non-obvious step. Without it Korvun connects but receives every
message with empty text — the bot looks "deaf".**

Still on the **Bot** tab, scroll to **Privileged Gateway Intents**:

- **MESSAGE CONTENT INTENT** → toggle **ON** → **Save Changes**.

It is self-serve while your bot is under 10,000 users — and every Korvun
user runs their own bot, so you are far below that by construction. Presence
and Server Members intents are **not** needed.

## Step 4 — Invite the bot to your server

1. **OAuth2** tab → **URL Generator**.
2. Under **Scopes**, check **`bot`**.
3. Under **Bot Permissions**: **View Channels** and **Send Messages**
   (required); **Read Message History** (recommended).
4. Open the **Generated URL** in your browser, pick your server, and
   **Authorize** — all the way to the end.

The bot now appears (offline) in your server's member list.

## Step 5 — Configure, export, run

`mode` is always `"gateway"`; `token_env` is the env var **name**:

```json
{
  "channels": [
    { "type": "discord", "mode": "gateway", "token_env": "DISCORD_BOT_TOKEN" }
  ],
  "routes": [
    { "channel": "discord", "brain": "assistant" }
  ]
}
```

```sh
export DISCORD_BOT_TOKEN=<your-bot-token>
korvun config check --preflight korvun.local.json
korvun serve --config korvun.local.json
```

Send a message in a channel the bot can see (or a DM). Korvun routes it
through your brain and replies in the same place. This round-trip is
hardware-verified end to end.

## Troubleshooting

- **Online but never replies / replies blank** → the Message Content intent
  is still off. Redo step 3, **Save Changes**, restart `korvun serve`.
- **`config check --preflight` fails naming the env var** → the token is not
  exported in the shell running Korvun. (Plain `config check` does not read
  secrets, so it cannot catch this.)
- **Authenticates but posting fails with 403 Missing Access** → the OAuth2
  invitation never completed. Redo step 4 end to end and confirm the bot is
  in the member list.
- **Cannot see a channel / cannot post** → re-invite with the step 4
  permissions and check the channel allows the bot's role.

## Good to know

- Text in and out, guild channels and DMs. Attachments, threads, slash
  commands, reactions, and edits are out of the v1 surface.
- Model replies can never ping `@everyone`/`@here`/roles — Korvun blocks
  mentions by default.

## Next

- [Telegram](/channels/telegram) · [Webhook](/channels/webhook)
