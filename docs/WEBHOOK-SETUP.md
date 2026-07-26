# Korvun on a generic Webhook — channel setup

The webhook channel turns Korvun into an **HTTP endpoint**: any system that can POST
JSON (a form backend, a CRM, an IoT hub, another service of yours) sends a message in,
Korvun routes it through a brain, and posts the reply back out to a URL you choose. No
provider account, no SDK — just HTTP.

Unlike Telegram (polling) or Discord (Gateway), the webhook channel **listens on a
socket you own**. It is loopback-only by default, and authenticated on every request
with a shared secret you generate. See [CONFIGURATION.md](CONFIGURATION.md) for every
field; this guide is the zero-to-round-trip path.

## What you need first

- Korvun built or installed (see [QUICKSTART.md](QUICKSTART.md), Step 1), and a model
  backend running (e.g. Ollama).
- A way to POST JSON — `curl` is enough to test.

## Step 1 — Configure the channel

Add a `webhook` channel and a route to your config (e.g. `korvun.local.json`). The
webhook channel takes **no `mode`**; its transport is HTTP. `token_env` is the **name**
of the env var holding the inbound secret (Step 2). `outbound_url` is where brain
replies are POSTed.

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
    { "channel": "webhook", "brain": "default" }
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

`bind` defaults to `127.0.0.1:8090` and `path` to `/webhook` if omitted — shown
explicitly above. The field `mapping` is optional; omitting it uses the canonical field
names (Step 6).

## Step 2 — Generate the inbound secret and export it

The secret is a shared password every inbound request must present. Generate a strong
one and export it under the name your config gave in `token_env`:

```bash
export KORVUN_WEBHOOK_SECRET=$(openssl rand -hex 32)
```

> **The secret is a secret.** It lives in an environment variable, never in
> `korvun.*.json`, a commit, or a chat. The config only stores the **name** of the env
> var (`token_env`). If it ever leaks, generate a new one and re-export.

Check the config resolves it (no port opened, no value printed):

```bash
korvun config check --preflight korvun.local.json
```

## Step 3 — Run Korvun

```bash
korvun serve --config korvun.local.json
```

Korvun opens the webhook server on the configured bind. A quick liveness check
(`/healthz` is **not** behind the secret, so this needs no auth):

```bash
curl -s http://127.0.0.1:8090/healthz   # -> 200
```

## Step 4 — Send a message in (curl)

Every inbound request needs `Authorization: Bearer <secret>` and
`Content-Type: application/json`:

```bash
curl -i http://127.0.0.1:8090/webhook \
  -H "Authorization: Bearer $KORVUN_WEBHOOK_SECRET" \
  -H "Content-Type: application/json" \
  -d '{"sender_id": "alice", "text": "hello korvun"}'
```

A `200` means Korvun accepted the message and is routing it. The brain's reply is
delivered to your `outbound_url` (Step 5) — not in this HTTP response.

## Step 5 — See the replies with a one-line test receiver

To watch the brain's replies without building anything, run a throwaway HTTP receiver
that answers `200` and prints what it gets, using only the Python standard library:

```bash
python3 -c 'import http.server as h; \
c=type("C",(h.BaseHTTPRequestHandler,),{"do_POST":lambda s:(print(s.rfile.read(int(s.headers["Content-Length"])).decode()),s.send_response(200),s.end_headers())}); \
h.HTTPServer(("127.0.0.1",9099),c).serve_forever()'
```

Point `outbound_url` at `http://127.0.0.1:9099/replies` (as in Step 1), send a message
(Step 4), and the reply JSON prints in the receiver's terminal.

## Step 6 — conversation_id and field mapping

Korvun maps JSON fields to its internal message shape. The defaults are the obvious
names; override only what differs, in the `webhook.mapping` block:

| Envelope field | Default JSON key | Meaning |
|----------------|------------------|---------|
| sender id | `sender_id` | **required** — who sent it |
| sender name | `sender_name` | optional display name |
| text | `text` | the message body |
| media url | `media_url` | optional attachment URL |
| media type | `media_type` | optional MIME type |
| conversation id | `conversation_id` | groups messages into one conversation |

**`conversation_id`** is what gives a brain memory across messages: reuse the same value
and Korvun treats them as one thread. If your payload omits it, Korvun falls back to the
**sender id**, so each sender still gets a stable per-sender conversation. On the way
out, the reply carries the same `conversation_id` back to your `outbound_url`.

Example with a renamed field:

```json
"webhook": {
  "outbound_url": "http://127.0.0.1:9099/replies",
  "mapping": { "conversation_id": "thread", "text": "message" }
}
```

## What each rejection means

| Status | Meaning | Fix |
|--------|---------|-----|
| `401` | Missing/wrong `Authorization: Bearer <secret>`. | Send the exact secret from Step 2. |
| `415` | Content-Type is not `application/json`. | Add the header (a `; charset=utf-8` suffix is fine). |
| `413` | Body larger than 1 MiB. | Send a smaller payload. |
| `400` | Body was not valid JSON, or had no sender / no text or media. | Fix the JSON / include `sender_id` and content. |
| `503` | The inbound buffer is momentarily full (a slow brain). | **Retry later** — the message was dropped, not an error in your request. |

## Exposing it to the outside (reverse proxy + TLS)

The default bind is **loopback** (`127.0.0.1`), so nothing is reachable off the machine
and the shared secret never crosses a network. If you bind a non-loopback address to
receive callbacks from other hosts, Korvun **warns at boot**:

> `webhook: non-loopback bind — the Bearer secret crosses the network in cleartext
> unless a TLS-terminating reverse proxy fronts this endpoint`

Korvun terminates no TLS itself. Put a reverse proxy in front that terminates TLS and
forwards to Korvun's loopback bind. A minimal, generic nginx snippet:

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

Keep Korvun's `bind` on `127.0.0.1:8090` (the proxy reaches it over loopback); only the
proxy listens on the public interface. Anything beyond this basic `proxy_pass` (rate
limits, request buffering, mTLS) is proxy-specific — **verify the directives against the
current nginx docs (Context7) before adding them**, rather than copying from memory.

## Notes on scope (v1)

One webhook instance per binary, registered under the name `webhook` (ADR-0038). Inbound
auth is a shared-secret Bearer (constant-time compared); HMAC-over-body signing,
in-process TLS, multiple named instances, and a configurable auth header are recorded as
future extensions in ADR-0038. Outbound auth is optional: name `outbound_token_env` and
Korvun sends `Authorization: Bearer <token>` to your `outbound_url` — but a named var
that does not resolve at boot is a loud error, never a silent unauthenticated outbound.
