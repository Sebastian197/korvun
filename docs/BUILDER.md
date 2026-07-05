# The Korvun builder — configure Korvun visually, no JSON

The **builder** is a visual panel, served by Korvun itself, where you edit your
configuration — brains, channels, routes, policies, and models — from your browser
instead of hand-writing the JSON file. You make your changes in forms, click one
button, and Korvun applies them **without restarting**.

This guide is for someone who wants to run and configure Korvun **without editing
JSON by hand**. You do not need to be a developer.

## Builder vs. the read-only view

Korvun's admin server exposes two things in the browser:

- **`/ui`** — a **read-only** live view: watch messages flow through Korvun in real
  time. It never changes anything.
- **`/builder`** — the **editable** visual panel this guide is about: change your
  configuration and apply it live.

The builder only appears when you turn on editing (below). **Without an admin token,
`/builder` is not served at all** — you still have the read-only `/ui`.

## 1. Turn on editing (the admin token)

Editing is protected by an **admin bearer token** — a secret you choose, like a
password. Korvun only mounts the editable builder when a token is configured; this
keeps a fresh install read-only and safe by default.

Two steps:

**a) Name the token's environment variable in your config.** Add an `admin` block to
`korvun.local.json` (see [`QUICKSTART.md`](QUICKSTART.md) for the full file). The
block holds the **name** of an environment variable — never the secret itself:

```json
{
  "channels": [ /* ... */ ],
  "brains":   [ /* ... */ ],
  "routes":   [ /* ... */ ],
  "admin": { "token_env": "KORVUN_ADMIN_TOKEN" }
}
```

- **`admin.token_env`** = `"KORVUN_ADMIN_TOKEN"` — the **name** of the env var that
  holds your admin token. (Field name verified against the config parser,
  `internal/config`.)

**b) Export the token before starting Korvun.** Choose any hard-to-guess secret as
the value and put it in that environment variable:

```sh
export KORVUN_ADMIN_TOKEN="a-long-random-secret-you-choose"
./korvun -config korvun.local.json
```

> ⚠️ **The admin token is a secret.** Anyone who has it can change how Korvun runs.
> Keep it out of the config file, out of chats, screenshots, and logs — it lives only
> in the environment. If it ever leaks, change the value and restart. This is the same
> discipline as the bot token in the quickstart.

If the `admin` block is absent, or the named variable is empty, Korvun starts
**read-only**: the builder is simply not served, and nothing can be mutated.

## 2. Open the builder

With Korvun running (with the `admin` block above), open your browser at:

```
http://127.0.0.1:2112/builder
```

`127.0.0.1:2112` is the admin server's default address (loopback — reachable only
from the same machine). When you open the builder you will see a field asking you to
**paste your admin token** ("paste to load the raw config"). Paste the value of
`KORVUN_ADMIN_TOKEN` and click **Load**.

The token is **held in memory only** — sent with each request as an
`Authorization: Bearer` header, never stored, never a cookie. If you reload the page,
you paste it again.

Once loaded, your current configuration appears as editable **forms**: your brains,
their models, your channels, and your routes.

## 3. Edit and save

1. Change what you need — for example, edit a brain, adjust a model's `model_id`, or
   change a route.
2. Click **Save and reload** (in the bar at the bottom; it is enabled once you have
   unsaved changes, and shows *"unsaved changes"* next to it). To throw your edits
   away instead, click **Discard**.

### What "Save and reload" does, in plain terms

Korvun applies your new configuration **live, without restarting the process**:

- The form **locks** while the change happens (the button reads **"Reloading…"**), so
  you cannot edit mid-swap.
- You see the progress: **reloading → reload succeeded**. Under the hood these are the
  real states Korvun reports (`pending`, `cutover-in-progress`, `succeeded`).
- On success, Korvun **rewrites your config file on disk** to match. *(This is exactly
  what was seen working: changing a brain's dispatch, clicking Save and reload, and the
  running binary rewriting the config file on disk.)*

### If the reload fails

If the new configuration cannot start (for example an invalid combination), Korvun
**rolls back**: it keeps running on the **previous** configuration and shows **reload
rolled-back** (or **reload failed**). Your on-disk config file is **not** overwritten
with a config that could not come up — the old, working one is preserved. Fix the
form and try again.

> A brief network blip while Korvun swaps itself is normal and treated as a retry, not
> a failure — the panel keeps polling until it hears a final result.

## 4. Add and remove

The forms are add/edit/remove throughout:

- **Add a brain** with **Add brain**; add a model inside a brain with its **Add model**
  button; add channels and routes the same way.
- **Remove** an item with its remove control (removing a brain asks you to confirm —
  **"Remove brain?"** — so you cannot delete one by accident).

Every add/remove is just another edit: nothing is applied until you click **Save and
reload**, and it goes through the same safe live-reload.

## 5. Security — please read

The builder gives **mutation control** over a running Korvun, so treat it accordingly:

- **The admin token is sensitive.** Guard it like a password (section 1).
- **The admin server binds loopback (`127.0.0.1`) by default**, so the builder is
  reachable only from the machine Korvun runs on. That is deliberate: a bearer token
  over plain HTTP is only safe when it never crosses a network (ADR-0028 / ADR-0020).
- **Do not expose the admin server to the network** (e.g. binding `0.0.0.0`) without
  putting TLS and access control in front of it. If you open the builder from anywhere
  other than loopback or HTTPS, the panel itself **warns you** that a pasted token
  would travel in cleartext.

## Next

- **Write the config by hand instead** → [`CONFIGURATION.md`](CONFIGURATION.md)
- **Get Korvun running first** → [`QUICKSTART.md`](QUICKSTART.md)
- **Install** → [`packaging/INSTALL.md`](packaging/INSTALL.md)
