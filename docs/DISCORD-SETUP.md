# Korvun on Discord — bot setup

The Discord counterpart of Telegram's BotFather steps (see
[QUICKSTART.md](QUICKSTART.md) for the Telegram path and the Ollama/model setup).
Discord needs a few clicks in the **Developer Portal** and **one manual switch that is
easy to miss** — the Message Content intent. Everything else is the usual config +
environment variable.

Korvun receives Discord messages over the **Gateway** (a WebSocket) and replies over
**REST**; both live in one adapter. Your bot token is a secret: it goes in an
**environment variable**, never in the config file (the config only names the variable).

## What you need first

- A Discord account and a server (**guild**) you own or can manage, to invite the bot
  into.
- Korvun built or installed (see [QUICKSTART.md](QUICKSTART.md), Step 1), and a model
  backend running (e.g. Ollama).

## Step 1 — Create the application and bot

1. Go to the **Discord Developer Portal**: <https://discord.com/developers/applications>.
2. **New Application** → give it a name (this is your bot's app) → **Create**.
3. Open the **Bot** tab in the left sidebar.

## Step 2 — Copy the bot token (into the environment, NEVER the config)

On the **Bot** tab, under **Token**, click **Reset Token** (or **Copy** if shown) to
reveal the token, and copy it.

> **The token is a secret.** Put it in an environment variable; do **not** paste it into
> `korvun.*.json`, a commit, or a chat. The config only stores the **name** of the env
> var (`token_env`). If a token ever leaks, come back here and **Reset Token** to
> revoke it.

## Step 3 — Turn ON the Message Content intent ⚠️ (the one manual step)

> **This is the single non-obvious step. Without it Korvun connects but receives every
> message with empty text, so the bot looks "deaf".**

Still on the **Bot** tab, scroll to **Privileged Gateway Intents** and enable:

- **MESSAGE CONTENT INTENT** → toggle **ON** → **Save Changes**.

`MESSAGE CONTENT` is a *privileged* intent: Discord requires you to flip it on manually
for the app (it is self-serve while your bot is under 10,000 users — every Korvun user
runs their own bot, so you are far below that by construction; ADR-0033 §3). Korvun
already requests it in its Gateway Identify (intents bitfield `37377`); this switch is
Discord's side of the handshake.

(You do **not** need Presence or Server Members intents — Korvun does not use them.)

## Step 4 — Invite the bot to your server (OAuth2 URL Generator)

1. Open the **OAuth2** tab → **URL Generator**.
2. Under **Scopes**, check **`bot`**.
3. Under **Bot Permissions**, check:
   - **View Channels** — required (to see + receive messages).
   - **Send Messages** — required (to reply).
   - **Read Message History** — optional but recommended (harmless; useful if replies
     ever become referenced messages).
4. Copy the **Generated URL** at the bottom, open it in your browser, pick your server,
   and **Authorize**.

The bot now appears (offline) in your server's member list.

## Step 5 — Configure Korvun

Add a `discord` channel and a route to your config (e.g. `korvun.local.json`). `mode`
is always `"gateway"`; `token_env` is the **name** of the env var from Step 2:

```json
{
  "channels": [
    { "type": "discord", "mode": "gateway", "token_env": "DISCORD_BOT_TOKEN" }
  ],
  "routes": [
    { "channel": "discord", "brain": "default" }
  ],
  "brains": [
    {
      "name": "default",
      "sensitivity": "private",
      "policy": { "kind": "priority" },
      "models": [
        { "provider": "ollama", "model_id": "llama3.2", "locality": "local" }
      ]
    }
  ]
}
```

`korvun.example.json` at the repo root stays the minimal Telegram example; the Discord
block lives here. See [CONFIGURATION.md](CONFIGURATION.md) for every field.

## Step 6 — Export the token and check the config

```bash
export DISCORD_BOT_TOKEN=<your-bot-token>
korvun config check --preflight korvun.local.json
```

Plain `korvun config check` validates the file's structure offline (no secrets read).
Adding **`--preflight`** also resolves the env-only secret — it confirms
`DISCORD_BOT_TOKEN` is set and constructs the adapter (no network, no port opened). It
never prints the token value.

## Step 7 — Run Korvun

```bash
korvun serve --config korvun.local.json
```

Korvun connects to the Gateway (identify → ready), keeps a heartbeat, and resumes
automatically if the connection drops. It serves until `Ctrl-C`.

## Step 8 — Expected round-trip

In a channel the bot can see (or a DM to the bot), send a message. Korvun should route
it through your brain and reply in the same channel.

> **Hardware-verified (2026-07-19).** A real end-to-end Discord round-trip was
> validated on real hardware (Intel iMac, macOS 13; Piece 4 SP6 Half B): a human
> message in a guild channel was routed through the brain (Ollama `llama3.2:1b`,
> local) and the reply was posted in the same channel within the same minute —
> inbound (Message Content intent), routing, outbound, and the anti-loop drop (the
> bot never answers itself) all confirmed.

## Troubleshooting

- **The bot is online but never replies / replies are blank.** The Message Content
  intent is almost certainly still off — redo **Step 3** and **Save Changes**, then
  restart `korvun serve`.
- **`config check --preflight` (or `serve`) fails naming the env var.**
  `DISCORD_BOT_TOKEN` (or whatever you named it) is not exported in the shell running
  Korvun — redo **Step 6**. (Plain `config check`, without `--preflight`, does not read
  secrets and will not catch this.)
- **The bot authenticates but is in no server** — `GET /users/@me` works, yet
  `GET /users/@me/guilds` returns an empty list and posting fails with **403 Missing
  Access**. The OAuth2 invitation was never completed: the generated URL was opened
  but the flow did not reach **Authorize** (or the wrong server was picked). Redo
  **Step 4** end to end — open the generated URL, pick your server, click
  **Authorize** — then confirm the bot appears in the server's member list.
- **The bot cannot see a channel / cannot post.** Re-invite with the **View Channels**,
  **Send Messages**, and **Read Message History** permissions (**Step 4**), and make
  sure the channel's permissions allow the bot's role.
- **A token leaked.** Developer Portal → **Bot** → **Reset Token**, then re-export the
  new value (**Step 6**).

## Notes on scope (v1)

Text in and text out, guild channels and DMs. Attachments/media, threads, slash
commands, reactions, and edits are out of v1 scope (ADR-0033 §8) — parity with the
Telegram v1 surface. Model replies can never ping `@everyone`/`@here`/roles: Korvun
sets `allowed_mentions` to none by default (ADR-0033 §6).
