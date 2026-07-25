# Piece 5 SP4 — the asset seam (same-origin admin proxy): Design Spec

> **Status:** approved for TDD (SP4).
> Governing ADRs: ADR-0035 §3(b) (same-origin proxy, gate RESOLVED by the SP1
> spike), §3(c) (shell chrome survives core shutdown; single embed), §4
> (bearer delivery: injected as the `Authorization` header at the proxy, so
> the token never enters the DOM), §6 (ephemeral admin port — the proxy must
> follow the effective address per cycle). External-docs note: the proxy uses
> ONLY stdlib (`net/http`, `net/http/httputil`) + existing `internal/`
> packages — no new external API surface, so no Context7 pass applies (SP2
> precedent). The streaming property `httputil.ReverseProxy` must provide is
> not trusted from memory: AS-4 is the automated in-repo restatement of the
> SP1 spike's verdict, and it fails red if the stdlib semantics are other
> than assumed. Inherited law: the `internal/shell` doc.go contract (**no
> Wails import in this package**); the `cmd/korvun-desktop` skeleton's static
> page stays as-is (the real chrome is SP6).

## Goal

The desktop WebView gains a same-origin path to the core's admin surface:
an `http.Handler` living entirely in `internal/shell` (framework-free,
`-race`-testable) that `cmd/korvun-desktop` mounts as
`assetserver.Options.Handler`. It reverse-proxies the admin routes
(`/api/*`, `/builder/*`, `/ui/*`, `/healthz`, `/metrics`) to the CURRENT
cycle's effective admin address, streams SSE incrementally, injects the
per-cycle bearer server-side, and — with the core stopped — answers an
honest, stable 503 instead of a hung or refused connection. Out of scope,
deferred: the real shell chrome and its start/stop UI (SP6), first-run
onboarding (SP5), packaging (SP7). The headless `cmd/korvun` is not touched
at all.

## Functional requirements

- **FR-PXY-1 Surface** — `(*Controller).ProxyHandler() http.Handler` in
  `internal/shell` (new file `proxy.go`). The handler serves ONLY the admin
  surface: exact `/healthz`, exact `/metrics`, and the `/api/`, `/builder`,
  `/ui` subtrees (segment-aware: `/ui` and `/ui/...` match, `/uix` does
  not). Anything else answers `404` with the small JSON body of FR-PXY-5 —
  the shell's own static assets are Wails' `Assets` side of the
  AssetServer, never this handler's job (ADR-0035 §3c). Traces to ADR-0035
  §3(b) and the copilot's SP4 direction: all logic inside `internal/shell`,
  `cmd/korvun-desktop` only mounts.
- **FR-PXY-2 Dynamic target** — every request resolves the destination at
  request time from the Controller's current cycle (mutex-guarded, through
  the same reap path as `Status`): the effective ephemeral admin address of
  THIS cycle (ADR-0035 §6). The address is NEVER cached across cycles; after
  Stop→Start the proxy forwards only to the new port.
- **FR-PXY-3 Streaming intact** — the forwarded response reaches the client
  incrementally (`httputil.ReverseProxy` with `FlushInterval: -1`, flush
  after every write): the SSE live-view (`/api/events`) must not buffer.
  This turns the SP1 spike's verdict (ADR-0035 §3(b) gate) into a
  repeatable in-repo test (AS-4) — deadlock-deterministic, not
  timing-based. Honesty note: the in-repo test proves the handler-side
  property over real HTTP; on the actual Wails mount the AssetServer's
  ResponseWriter is not an `http.Flusher`, so live delivery rides the
  platform writers (proven live by the SP1 spike on WKWebView) and the SP8
  hardware validation re-checks the live view end to end.
- **FR-PXY-4 Bearer injection (ADR-0035 §4, the F9 amendment)** — when the
  cycle has an admin bearer, the proxy sets
  `Authorization: Bearer <token>` on EVERY forwarded request, always
  OVERWRITING any client-supplied value (a page-supplied header is
  discarded — the DOM neither holds nor needs the token); when the cycle
  has NO bearer, a client-supplied `Authorization` is STRIPPED, so the
  page's headers never reach the core as credentials. The token value
  reaches the proxy through a package-private, mutex-guarded Controller
  accessor over per-cycle state set by `Start` and cleared by `Stop`/reap —
  never through the DOM, a served asset, a response header, a query
  parameter, or a log.
- **FR-PXY-5 Honest degraded mode (ADR-0035 §3c)** — with the core stopped
  (or observability disabled, so no admin server exists), proxied routes
  answer `503` + `Content-Type: application/json` + the small stable body
  `{"error":"core stopped"}` — paintable by SP6's chrome as "the gateway is
  stopped". A forwarding failure mid-flight (core died between check and
  dial) answers the same `503` shape with `{"error":"core unreachable"}`
  via the ReverseProxy `ErrorHandler` — never a raw refused connection,
  hang, or panic. Non-matching paths: `404` + `{"error":"not found"}`.
- **FR-PXY-6 Mount (`cmd/korvun-desktop`, behind the `desktop` tag)** —
  `main.go` builds the SP2/SP3 Controller (logger + `keyring.New()` store)
  and mounts `ctrl.ProxyHandler()` as `assetserver.Options.Handler`
  alongside the existing `Assets` embed. The skeleton's static page stays;
  no bindings, no chrome (SP6). Headless binary, default suite, and the ×6
  pipeline byte-untouched (no new deps; `internal/shell` stays stdlib-only).

## Acceptance scenarios (Given / When / Then)

- **AS-1 (allowlist)** Given any Controller, When the handler receives
  `/favicon.ico` (or any non-admin path, including the near-miss `/uix`),
  Then `404` with the FR-PXY-5 JSON body — the request never reaches the
  core.
- **AS-2 (stopped → honest 503)** Given a stopped Controller, When any of
  `/healthz`, `/metrics`, `/api/brains`, `/builder/`, `/ui/` is requested,
  Then `503`, `application/json`, body exactly `{"error":"core stopped"}` —
  no hang, no panic, no refused connection.
- **AS-3 (running → forwarded)** Given the SP2 real-app-no-network pattern
  (fake channel factory, `httptest` ollama) with the core started, When the
  proxy receives `/healthz`, `/metrics`, `/api/brains`, `/builder/` and
  `/ui/`, Then each answers the core's own response (200; `/builder/`
  serves the embedded builder HTML, asserted by its `Korvun · builder`
  marker — the single `web/builder/dist` embed reached through the proxy,
  no second copy), and a non-admin path still answers the FR-PXY-5 404.
- **AS-4 (streaming tripwire)** Given an SSE upstream test server that
  flushes event 1 and then BLOCKS until the client acknowledges receipt,
  When the stream is consumed through the proxy, Then event 1 arrives while
  the upstream is still holding the stream open (then events 2 and 3 flow
  the same way). A buffering proxy deadlocks and the test fails by timeout
  — the automated SP1-spike verdict.
- **AS-5 (bearer, ADR-0035 §4)** Given the running core with the admin
  block mounted, When the proxy receives `GET /api/config` with NO
  client-side `Authorization` header, Then `200` (the proxy injected the
  cycle bearer); When the client supplies a BOGUS `Authorization`, Then
  still `200` (proxy overwrites, client headers cannot defeat or replace
  the injection); and the token value appears in NO response body the
  handler ever serves (asserted over the 404/503 bodies and the proxied
  responses of AS-3/AS-5).
- **AS-6 (full cycle, ADR-0035 §6)** Given Start → the proxy forwards (AS-3
  green, effective addr A), When Stop, Then proxied routes answer the AS-2
  503 and port A no longer accepts connections; When Start again (effective
  addr B ≠ A, fresh bearer), Then the proxy forwards to B only —
  `/healthz` 200 and `GET /api/config` 200 with the NEW bearer prove no
  stale addr/token survives the cycle.
- **AS-7 (dead upstream mid-flight)** Given a target that stops listening
  after the stopped-check passes (simulated at the package seam with a dead
  address), When a proxied request is made, Then `503` +
  `{"error":"core unreachable"}` — the ErrorHandler path, not a transport
  error surfaced raw.
- **AS-8 (non-GET forwarding)** Given the running core, When the proxy
  receives `POST /api/config` with an invalid JSON body and no client
  `Authorization`, Then the core's own `400` comes back — a `401` would
  mean the injection failed, a `404`/`503` that routing failed, anything
  else that the body was not forwarded (the builder's save flow is a POST).
- **AS-9 (race tripwire)** Given concurrent proxied requests hammering the
  handler while `Stop` runs, When observed under `-race`, Then every
  response is either the live core's `200` or one of the two stable 503
  bodies — no race report, panic, or off-contract response.

## Success criteria

- New `internal/shell` proxy code covered ≥ 85% (house floor); package
  coverage stays ≥ 85%.
- `make quality` green with `-race` over the WHOLE suite.
- Headless `cmd/korvun` and its pipeline untouched: no `go.mod` change (the
  proxy is stdlib-only), `internal/shell` still free of any Wails import
  (doc.go contract), the `cmd/korvun-desktop` diff rides entirely behind
  the `desktop` build tag. Desktop binary proven to COMPILE on this Mac
  (`CGO_LDFLAGS="-framework UniformTypeIdentifiers" go build -tags
  desktop,production ./cmd/korvun-desktop` — the SP1 environment finding).

## Decisions folded in

- **Proxy construction is package-internal to `internal/shell`** (a
  `Controller` method + private accessor for addr/bearer), not a separate
  package: the bearer must not cross a public seam, and the accessor rides
  the existing mutex + reap discipline. The copilot's mount-only-main
  direction validated as spec'd.
- **The bearer accessor reads per-cycle Controller state, not the env**:
  `Start` records the token value it generated (cleared on Stop/reap), so
  the proxy never re-reads the environment — one writer, one reader, no
  Getenv coupling.
- **Uniform injection**: the bearer goes on every forwarded request (also
  `/builder/`, `/ui/`, `/healthz`) rather than a per-route table — nothing
  downstream objects to an extra header, and route tables drift.
- **Error bodies are a two-value stable contract** (`core stopped` /
  `core unreachable`), both 503: SP6's chrome switches on them; anything
  richer (retry hints, cycle ids) waits for a real consumer.
- **404 for non-admin paths** (not passthrough): the Wails AssetServer
  consults `Assets` first, so any path reaching the handler is either admin
  surface or a miss; answering 404 keeps the seam honest and testable.
- **No WebSocket handling**: the admin surface has none (SSE only); a WS
  route would be a new ADR-level surface, not a proxy afterthought.

## `[NEEDS CLARIFICATION]`

None arose — every open point resolves inside ADR-0035's accepted decisions
(§3b proxy path already gate-resolved by the SP1 spike, §4 bearer delivery,
§6 port policy) plus the copilot's explicit SP4 direction (handler inside
`internal/shell`, stdlib-only, mount-only `main.go`, static skeleton page
retained). TDD may proceed.