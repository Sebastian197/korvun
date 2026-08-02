# The visual builder

The **builder** is a visual panel, served by Korvun itself, where you edit
your configuration — channels, brains, routes, policies, and models — from
your browser instead of hand-writing JSON. Since v0.6.0 it is a **canvas**:
drag blocks from a palette, wire them with cables, edit each piece in its
panel, and apply everything live. You do not need to be a developer.

Korvun's admin server exposes two things in the browser:

- **`/ui`** — a read-only live view: watch messages flow in real time.
- **`/builder`** — the editable panel this guide is about.

Without an admin token, `/builder` is not served at all — a fresh install is
read-only and safe by default.

## 1. Turn on editing (the admin token)

Editing is protected by an **admin bearer token** — a secret you choose.
Two steps:

**a)** Name the token's environment variable in your config — the **name**,
never the secret itself:

```json
{ "admin": { "token_env": "KORVUN_ADMIN_TOKEN" } }
```

**b)** Export the token before starting Korvun:

```sh
export KORVUN_ADMIN_TOKEN="a-long-random-secret-you-choose"
korvun serve --config korvun.local.json
```

> **The admin token is a secret.** Anyone who has it can change how Korvun
> runs. It lives only in the environment — never in the config file, chats,
> screenshots, or logs. If it leaks, change the value and restart.

## 2. Open the builder

With Korvun running, open:

```
http://127.0.0.1:2112/builder
```

`127.0.0.1:2112` is the admin server's default address — loopback, reachable
only from the same machine. Paste your admin token when asked. The token is
held **in memory only**, sent as an `Authorization: Bearer` header — never
stored, never a cookie. Reload the page, paste again.

## 3. Compose on the canvas

Your configuration appears as blocks and cables:

- **Drag** channels, brains, and models from the palette onto the canvas.
- **Wire** a channel to a brain — the only manual cable; the canvas validates
  what can connect to what.
- **Edit** any block in its properties panel — a brain's persona included.
- **Privacy is visible**: mark a brain `private` and every cloud model shows
  a **gray dashed cable** — excluded before dispatch, drawn on the canvas
  instead of buried in JSON.
- **Delete** removes a block and everything that only made sense with it
  (its cables), with confirmation.

## 4. Save and reload — live, with a safety net

Nothing is applied until you click **Save and reload**. Then Korvun applies
the new configuration **without restarting**:

- The form locks while the change happens; you watch the real states:
  **reloading → reload succeeded**.
- On success, Korvun rewrites your config file on disk to match.
- **If the new config cannot start, Korvun rolls back**: it keeps running on
  the previous configuration, shows **reload rolled-back**, and your on-disk
  file is not overwritten with something broken. Fix and try again.

Click **Discard** to throw away unsaved edits instead.

## 5. Security — please read

- The admin server binds **loopback by default**, so the builder is only
  reachable from the machine Korvun runs on. That is deliberate: a bearer
  token over plain HTTP is only safe when it never crosses a network.
- **Do not expose the admin server to the network** (`0.0.0.0`) without TLS
  and access control in front. If you open the builder from anywhere other
  than loopback or HTTPS, the panel itself warns you that a pasted token
  would travel in cleartext.

## Next

- Write the config by hand instead → [configuration reference](/reference/configuration)
- Get Korvun running first → [quickstart](/guide/quickstart)