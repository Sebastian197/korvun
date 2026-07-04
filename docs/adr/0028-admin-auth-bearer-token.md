# ADR-0028: Stage 14 Phase 2a — admin auth (bearer token)

> **Status:** accepted
> **Date:** 2026-07-04
> **Deciders:** Sebastián Moreno Saavedra (+ copilot review)

## Context

Phase 2a gives the control API a **mutation** endpoint
([ADR-0027](0027-config-mutation-reload-and-rebuild.md)): it can now rewrite the
config and rebuild the running system. That is the trigger for auth, and it is
kept in a **separate ADR from the mutation** deliberately — auth is a
**surface/attack-surface risk**, the mutation is a **blast-radius risk**;
different lenses, the same split discipline that separated Stage 9 (interface vs
engine) and Stage 14 Phase 1 (bus vs live-view).

Until now Korvun's admin surface is **read-only on loopback with no auth** (Stage
12 `/metrics` + `/healthz`, Stage 13 `/api/brains` + `/api/channels`, Stage 14 P1
`/api/events` + `/ui`). That calculus held **because nothing mutated state**:
ADR-0022 made "read-only keeps loopback-no-auth valid" an explicit load-bearing
decision. The moment a request can change what the process does, loopback-no-auth
is insufficient — a local process, an SSRF pivot, or a browser CSRF could mutate.
This is the project's **first real attack surface**.

### External-docs / mechanism note (boring-by-default)

A bearer token in the `Authorization` header, compared in constant time, is the
standard minimal auth for a single-operator self-hosted service. It needs no
identity provider, no session store, no key custody, and it fits the project's
existing **env-only secret discipline** (ADR-0010: a secret is named by an
env-var in config, its value lives only in the environment, never in argv, the
config file, logs, or errors).

## Decision

### 1. Bearer token, env-only, gating every mutation endpoint

- The config names the env-var that holds the admin token (e.g. an `admin`
  block with `token_env`, mirroring `token_env` / `api_key_env` — the value is
  **read from the environment at boot**, never stored in the file). Suggested
  var: `KORVUN_ADMIN_TOKEN`.
- Every **mutation** endpoint (the `POST`/`PUT` config route of ADR-0027) is
  wrapped by a middleware that requires `Authorization: Bearer <token>`.
  Missing/wrong token → `401`. **Constant-time compare, correctly (F12):**
  `crypto/subtle.ConstantTimeCompare` early-returns `0` on a length mismatch, so
  comparing the raw variable-length tokens directly still **leaks the token length**
  through timing — "no timing oracle" was over-stated. The fix is to compare
  **fixed-length hashes**: `subtle.ConstantTimeCompare(sha256(got), sha256(want))`,
  so both inputs are always 32 bytes and neither length nor content leaks.
- **No token configured ⇒ the mutation endpoints are NOT MOUNTED.** Mutation does
  not exist without a token. The safe default is exactly today's behavior: a
  read-only Korvun on loopback. Auth is therefore not an "optional off switch"
  over an open mutation surface — the surface only comes into being when the
  operator sets the token.
  - **Token-value rotation needs a restart, not a reload (F9 — correcting an
    earlier over-claim).** The token *value* lives in the process environment,
    resolved by `os.Getenv` at `Build` and **fixed for the process lifetime**
    (ADR-0010: secrets never arrive through the API or the config file). A
    reload-and-rebuild re-runs `os.Getenv`, but it reads the environment the process
    was **launched** with, so it returns the **same** value. What a live config edit
    *can* rotate is the **env-var NAME** the config points at; rotating the actual
    secret **value** requires re-launching the process with a new environment.
    (Enabling the token for the first time — naming a var that is already exported —
    does take effect on reload.)
  - **Self-lock refusal (F11, ties to ADR-0027 §flow step 3).** Naming a token
    env-var that resolves **empty** yields "no token ⇒ not mounted", so a reload
    that drops the token would disable the control plane **and persist that state** —
    not even a restart brings it back via the API. ADR-0027's mutation handler
    therefore **refuses** a config that would leave the surface unmounted while it is
    currently mounted, naming the manual recovery (edit `-config` + restart).

### 2. Read-only endpoints stay loopback-open (justified, and its limit named)

The gate covers **writes only**. `/api/brains`, `/api/channels`, `/api/events`,
`/metrics`, `/healthz`, and `/ui` stay open on loopback, unchanged.

- **Why:** their threat model did not change. They expose the same read-only data
  on the same loopback bind that Stage 12/13/14-P1 already accepted; adding auth to
  them mitigates no new risk while breaking the existing vanilla `/ui` and SSE
  live-view, which are designed for no-auth loopback (2b's React UI would then also
  have to authenticate every read, scope creep with no security gain). Keeping the
  Stage 12/13 profile **exactly intact** is the point.
- **Its limit (named honestly):** if the operator consciously binds the admin
  server to a non-loopback address (`observability.addr = 0.0.0.0:PORT`, the
  documented operator choice in ADR-0020 §4), the read-only surface leaks
  brain/channel metadata to the network. That responsibility is **unchanged and
  still the operator's** (ADR-0020 §4 already puts auth/TLS/firewall on them for a
  non-loopback bind). The **mutation** gate, by contrast, is **unconditional** —
  it applies regardless of bind address, because a write is dangerous on loopback
  too. **But the gate is only as strong as the transport (F10):** Korvun terminates
  no TLS, so on a non-loopback bind the bearer token crosses the network **in
  cleartext**, where it can be sniffed and replayed into a full config-takeover —
  strictly worse than the read-only metadata leak above. A non-loopback mutation
  bind therefore **requires a TLS terminator in front** (reverse proxy / tunnel);
  without it the gate is theater. On the default loopback bind this does not arise.
  This is the operator's responsibility, same as ADR-0020 §4, but stated explicitly
  for the write path because the failure here is **credential compromise**, not just
  disclosure. A future "require the token for all of `/api`" toggle is left as a
  possible follow-up, not built now (no new risk on the default loopback bind).

### 3. CSRF neutralized by construction

The token travels in an **explicit `Authorization: Bearer` header, NEVER a
cookie**. A cross-site page can auto-send cookies, but it **cannot set a custom
`Authorization` header** on a cross-origin request without a CORS preflight, and
Korvun sets **no permissive CORS**. So a browser CSRF against the mutation
endpoint cannot forge credentials — the header-not-cookie choice defends it by
construction, no CSRF token machinery needed.

How the 2b UI obtains the token: the **operator pastes it** into the UI. Whether
it lives in `sessionStorage` vs in-memory is a **2b decision** (its own ADR); the
**contract fixed here** is that the UI sends it as `Authorization: Bearer <token>`
and never as a cookie.

## Consequences

### Reversibility (explicit)

Additive. Auth is a **middleware gate** wrapping only the write handlers, plus one
config field naming an env-var. Removing the field (or leaving the token unset)
collapses to today's read-only, no-auth loopback Korvun — mutation simply isn't
mounted. No schema, no change to the read-only handlers, no data migration.

### Trade-offs accepted

- **One shared token, no identities.** A single operator does not need per-user
  auth; a shared bearer is the right size. Multi-user is out of scope.
- **The operator manages the token** (generation, storage in their env/secret
  manager, rotation by editing the config). This matches how they already manage
  `TELEGRAM_BOT_TOKEN` / `GROQ_API_KEY` (ADR-0010).

## Alternatives Considered

- **Bearer token from env (CHOSEN)** — boring, standard, zero new deps, matches
  the env-only secret discipline, right-sized for one operator.
- **mTLS** — **rejected:** client-cert provisioning/rotation is heavyweight for a
  single operator, and a browser UI cannot easily present a client cert.
- **HTTP Basic auth** — **rejected:** base64 credentials on every request, invites
  browser credential caching / cookie storage, and awkward native prompts; a
  header bearer token is cleaner and header-not-cookie gives the CSRF property.
- **OAuth / OIDC** — **rejected:** requires an identity provider, redirect flows,
  and session management — massive over-engineering with no multi-user need.

## Out of scope (recorded)

Multi-user accounts, RBAC / per-endpoint scopes, sessions/cookies, a token
rotation UI, protecting the read-only surface by default, and CORS for
cross-origin API use.

## Delivery

Auth ships **with** Phase 2a (ADR-0027) on the same **branch** with **`/review`**:
it is the gate on the mutation the review is already scrutinizing. Tests cover the
`401` paths (missing/wrong/empty token, constant-time compare), the "no token ⇒
not mounted" default, and that the read-only handlers stay reachable without a
token. `make quality` green; `go.mod` stays at 3 deps (`net/http` +
`crypto/subtle`, stdlib).
