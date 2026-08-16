# Webhook

The webhook channel turns Korvun into an **HTTP endpoint**: anything that can
POST JSON — a form backend, a CRM, an IoT hub, another service of yours —
sends a message in, Korvun routes it through a brain, and posts the reply to
a URL you choose. No provider account, no SDK — just HTTP.

Unlike Telegram (polling) or Discord (Gateway), this channel **listens on a
socket you own**: loopback-only by default, authenticated on every request
with a shared secret you generate.

## Step 1 — Configure the channel

The webhook channel takes **no `mode`**. `token_env` names the env var
holding the inbound secret; `outbound_url` is where replies are POSTed:

```json
{
  "channels": [
    {
      "type": "webhook",
      "token_env": "KORVUN_WEBHOOK_SECRET",
      "webhook": {
        "bind": "127.0.0.1:8090",
        "path": "/webhook",
        "outbound_url": "http://127.0.0.1:9099/replies"
      }
    }
  ],
  "routes": [
    { "channel": "webhook", "brain": "assistant" }
  ]
}
```

`bind` defaults to `127.0.0.1:8090` and `path` to `/webhook` — shown
explicitly above.

## Step 2 — Generate the secret and export it

```sh
export KORVUN_WEBHOOK_SECRET=$(openssl rand -hex 32)
korvun config check --preflight korvun.local.json   # confirms it resolves
```

> **The secret is a secret.** It lives in the environment, never in the
> config file, a commit, or a chat — the config only stores the variable's
> **name**. If it leaks, generate a new one and re-export.

## Step 3 — Run and send a message in

```sh
korvun serve --config korvun.local.json
curl -s http://127.0.0.1:8090/healthz   # liveness, no auth needed -> 200
```

Every inbound request needs the Bearer secret and a JSON content type:

```sh
curl -i http://127.0.0.1:8090/webhook \
  -H "Authorization: Bearer $KORVUN_WEBHOOK_SECRET" \
  -H "Content-Type: application/json" \
  -d '{"sender_id": "alice", "text": "hello korvun"}'
```

A `200` means Korvun accepted the message and is routing it. The brain's
reply goes to your `outbound_url` — not in this HTTP response.

## Step 4 — Watch the replies

A throwaway receiver that prints whatever it gets (Python stdlib only):

```sh
python3 -c 'import http.server as h; \
c=type("C",(h.BaseHTTPRequestHandler,),{"do_POST":lambda s:(print(s.rfile.read(int(s.headers["Content-Length"])).decode()),s.send_response(200),s.end_headers())}); \
h.HTTPServer(("127.0.0.1",9099),c).serve_forever()'
```

Point `outbound_url` at `http://127.0.0.1:9099/replies` (as in step 1), send
a message, and the reply JSON prints in the receiver's terminal.

## conversation_id and field mapping

Korvun maps your JSON fields to its message shape; override only what
differs, in `webhook.mapping`:

| Korvun field | Default JSON key | Meaning |
|---|---|---|
| sender id | `sender_id` | **required** — who sent it |
| sender name | `sender_name` | optional display name |
| text | `text` | the message body |
| media url | `media_url` | optional attachment URL |
| media type | `media_type` | optional MIME type |
| conversation id | `conversation_id` | groups messages into one thread |

**`conversation_id` is what gives a brain memory across messages** — reuse
the same value and Korvun treats them as one conversation. If omitted, the
sender id is the fallback, so each sender still gets a stable thread. The
reply carries the same `conversation_id` back out.

```json
"webhook": {
  "outbound_url": "http://127.0.0.1:9099/replies",
  "mapping": { "conversation_id": "thread", "text": "message" }
}
```

## What each rejection means

| Status | Meaning | Fix |
|---|---|---|
| `401` | Missing/wrong `Authorization: Bearer`. | Send the exact secret from step 2. |
| `415` | Content-Type is not `application/json`. | Add the header (`; charset=utf-8` is fine). |
| `413` | Body over 1 MiB. | Send a smaller payload. |
| `400` | Invalid JSON, or no sender / no content. | Fix the JSON; include `sender_id` and text or media. |
| `503` | Inbound buffer momentarily full. | **Retry later** — your request was fine. |

## Exposing it to the outside (reverse proxy + TLS)

The default bind is loopback, so nothing is reachable off the machine. If
you bind a non-loopback address, **Korvun warns at boot**: the Bearer secret
would cross the network in cleartext. Korvun terminates no TLS itself — put
a TLS-terminating reverse proxy in front and keep Korvun on loopback:

```nginx
server {
    listen 443 ssl;
    server_name korvun.example.com;

    ssl_certificate     /etc/letsencrypt/live/korvun.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/korvun.example.com/privkey.pem;

    location /webhook {
        proxy_pass http://127.0.0.1:8090/webhook;
    }
}
```

## Next

- [Telegram](/channels/telegram) · [Discord](/channels/discord)
- Every webhook field → [configuration reference](/reference/configuration)
