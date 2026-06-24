# ADR-0020: Observability — structured logs, a Metrics seam, and an admin HTTP server (Stage 12)

> **Status:** accepted
> **Date:** 2026-06-24
> **Deciders:** Sebastián Moreno Saavedra (+ copilot review)

## Context

Korvun boots, serves Telegram live, and remembers across restarts (Stages 0–7,
9, 11). What it cannot do yet is let an operator **see what it is doing**: there
is no metrics surface, no liveness endpoint, and the structured logs that exist
carry inconsistent fields. For a single binary meant to run unattended from a
Raspberry Pi to the cloud, "is it alive, what is failing, how slow is it" is the
gap this stage closes.

### The framing line — the 80% already exists; instrument the funnels, do not open the hot path

The decisive finding of the Stage 12 framing (`/office-hours` alternatives +
`/plan-eng-review` lens, reviewed by copilot) is that the measurement points are
**already funnels**, so instrumenting them has near-zero blast radius:

- **Logs are already on `log/slog`** everywhere on the hot path: `WithLogger`
  seams in `internal/app` (`app.go:85`), `internal/router`
  (`WithErrorHandler`, `app.go:125`), `internal/brain`
  (`orchestrator.go:99`), `internal/channel/telegram` (`options.go:200`);
  default `slog.Default()`; `cmd/korvun/main.go:33` mounts a `JSONHandler`.
  What is missing is **field consistency**, not a logging library.
- **Per-provider latency is already captured**: `fanout.Outcome.Latency`
  (`fanout/fanout.go:24-31`) records the wall-clock time spent inside
  `Model.Generate` — "observability that costs nothing to capture at this
  layer." Metrics **read** it after `coord.Run`; they do not time the hot path.
- **Failures already funnel through one hook**: the router's
  `WithErrorHandler(func(RouterError))` with a typed `RouterError.Kind`
  (`InboundDispatch` / `Handle` / `OutboundSaturated` / `Send`). `app.go:125`
  already wires it to a log; a metric increment is one more line in that single
  closure, with **zero `router` change**.
- **Drops are already counted**: `telegram.DroppedCount() uint64`
  (`adapter.go:223`) is an atomic accessor, ready to be exposed as a
  pull-collected metric.

The single genuinely new moving part is an **HTTP server** to expose `/metrics`
and `/healthz`. The binary has none today (Telegram runs in polling, the webhook
adapter produces an `http.Handler` but nothing serves it). The real risk of this
stage is the **server lifecycle and its place in the ADR-0008 shutdown order**,
not the counters. This ADR decides that order (§4).

### External-docs verification (per CLAUDE.md non-negotiable)

Verified via Context7 against `/prometheus/client_golang` (the only new
dependency) before writing this ADR — not from memory:

- **Custom registry, not the global default.** `prometheus.NewRegistry()` plus
  `reg.MustRegister(...)`. The package's `DefaultRegisterer` auto-registers
  process/Go collectors in an `init()` (`prometheus/registry.go`) — a mutable
  global, which CLAUDE.md forbids. Korvun owns a private registry; runtime and
  process collectors are registered **explicitly** on it via the `collectors`
  subpackage (`collectors.NewGoCollector()`, `collectors.NewProcessCollector(...)`).
- **Instruments.** `NewCounterVec` / `NewGaugeVec` / `NewHistogram(Vec)` with
  `prometheus.HistogramOpts{Buckets: ...}`; labels via `WithLabelValues(...)`
  (order-sensitive) or `With(prometheus.Labels{...})`.
- **Exposition.** `promhttp.HandlerFor(reg prometheus.Gatherer, promhttp.HandlerOpts{})`
  returns an `http.Handler`, mounted on a stdlib `http.ServeMux` at `/metrics`.
  `HandlerOpts{}`'s zero value defaults to `HTTPErrorOnError`, which writes the
  collected-metric error into the response body; we set `HandlerOpts.ErrorLog`
  to a small `slog` adapter so collection errors are logged, and keep the
  default error mode (acceptable: a metrics-collection error is operator-facing,
  not secret).

`promhttp` and `collectors` are subpackages of the same `client_golang` module —
they add **no further direct dependency**.

## Decision

Ship **Alternative B in one stage, one ADR**: standardized `slog` fields on the
existing funnels (zero new deps) **and** a `Metrics` seam with a Prometheus
implementation behind it, exposed via a small admin HTTP server that also serves
`/healthz`. `slog`-fields and the `Metrics` seam are the **same cut** (an
operator needs both to operate), so unlike Stage 9 they do not earn separate
ADRs. Traces are deferred.

### 1. Standardized structured-log fields (no new dependency)

The minimal, mechanical first step. Make the existing `slog` calls on the hot
path carry a consistent field vocabulary so logs are filterable and correlate
with metrics:

| Field         | Meaning                                  |
|---------------|------------------------------------------|
| `envelope_id` | the `envelope.Envelope` id               |
| `channel`     | source/target channel name               |
| `brain`       | routed brain name                        |
| `provider`    | model provider (`Model.Name()`)          |

Applied at the existing call sites only: the model-wired and channel-started
logs in `app.go`, the router-error closure (`app.go:125`), the no-answer log in
`brain.Orchestrator`, and the drop warning in `telegram`. No new log statements
on tight loops; no behavior change. This step is shippable before any dependency
is added.

### 2. The `Metrics` seam — interface in the domain, Prometheus in a leaf

The reversibility decision, and the project's established pattern
(`conversation.Store`, `model.Model`). The domain depends on an **interface**;
the only package that imports `prometheus` is a single leaf implementation.

```
   domain: brain, app's router-error closure, (telegram via pull)
        │ depends on the INTERFACE, never on prometheus
        ▼
   internal/metrics.Metrics          ◄── push interface (+ Nop default)
        ▲                      ▲
        │ impl                 │ impl (tests / default)
   prometheusMetrics        Nop
   (only pkg importing      (no-op; the safe default, like
    client_golang)          slog DiscardHandler — domain never nil-checks)
```

```go
// Package metrics owns the observability seam: a push interface the domain
// records through, plus a Nop default so a nil backend is never possible.
package metrics

// Metrics records domain observations. Implementations MUST be safe for
// concurrent use (the router's N brain workers and the fan-out's per-provider
// goroutines record concurrently) — the same concurrency discipline model.Model
// and conversation.Store carry. The Prometheus impl is concurrency-safe by
// construction (its instruments are).
type Metrics interface {
    // MessageProcessed counts one inbound message handed to a brain.
    MessageProcessed(channel string)
    // ProviderObserved records one provider call: its latency and whether it
    // succeeded. Sourced from fanout.Outcome (Latency + Err) AFTER coord.Run —
    // off the hot path, no contention added.
    ProviderObserved(provider string, latency time.Duration, ok bool)
    // RouterError counts one async router failure, by RouterError.Kind.String().
    RouterError(kind string)
    // TurnsPersisted counts turns durably appended (the AppendTurns group size).
    TurnsPersisted(n int)
}
```

`Nop` is the default (injected like `slog.Default()`), so every domain object
holds a non-nil `Metrics` and never guards a nil. `internal/metrics` is a leaf
(imports only the stdlib); the `prometheusMetrics` impl lives in its own leaf
that imports `client_golang` + the interface; `cmd/korvun`/`internal/app` wire
the concrete impl. Dependency direction documented: nothing in the domain
imports `prometheus`.

### 3. The minimal metric set

Prometheus naming conventions (snake_case, `_total` for counters, base unit
`_seconds`, the `korvun_` namespace):

| Metric | Type | Labels | Source (existing funnel) |
|--------|------|--------|--------------------------|
| `korvun_messages_processed_total` | counter | `channel` | `brain.Handle` entry |
| `korvun_provider_request_duration_seconds` | histogram | `provider`, `outcome` | `fanout.Outcome.Latency` (already measured) |
| `korvun_provider_failures_total` | counter | `provider` | `fanout.Outcome.Err != nil` |
| `korvun_router_errors_total` | counter | `kind` | router `WithErrorHandler` closure (`app.go:125`) |
| `korvun_channel_messages_dropped_total` | counter | `channel` | `telegram.DroppedCount()` via a pull Collector |
| `korvun_conversation_turns_persisted_total` | counter | — | `brain.persistTurns` after `AppendTurns` |

Plus the standard `go_*` and `process_*` series from the explicitly-registered
runtime/process collectors (§ External-docs verification).

**Histogram buckets are LLM-shaped, not HTTP-shaped.** `prometheus.DefBuckets`
top out at 10s — Korvun's provider calls run to the 30s
`DefaultPerModelTimeout` (`app.go:45`). Buckets are exponential out to ~30s so
the timeout tail is visible.

**`DroppedCount` is pulled, not pushed.** It is already a cumulative atomic;
re-incrementing a counter at the drop site would double-instrument. Instead a
small custom `Collector` reads `DroppedCount()` at scrape time (the
`Describe`/`Collect` pattern). This keeps the push interface (§2) free of the
drop concern and the adapter free of a metrics dependency — `app` wires the
collector with a reference to the telegram adapter at construction.

### 4. The admin HTTP server — seam shape now, Stage 13 functionality later

A new leaf, `internal/httpserver` (name TBD in review), owns one `http.Server`
over a stdlib `http.ServeMux`, with an ADR-0008-style `Start`/`Shutdown`
lifecycle that `App` drives. It serves exactly two routes in this stage:

```
   GET /metrics  -> promhttp.HandlerFor(reg, HandlerOpts{ErrorLog: slogAdapter})
   GET /healthz  -> 200 "ok" while the process is serving (liveness only)
```

This is the **shape of the seam Stage 13 (control API) will mount on** — the same
server and mux — but **no Stage 13 functionality is built**: no config endpoint,
no brain management, nothing beyond `/metrics` and `/healthz`.

**Default-on, loopback-bound (resolved in review).** The server is **on by
default**: observability you have to turn on by hand is never there when you need
it. It binds **`127.0.0.1:2112`** by default — loopback so a fresh boot exposes
nothing to the network (2112 is the conventional `client_golang` exporter port;
9090 is deliberately avoided because it is the Prometheus *server* port and would
confuse). An operator who wants `0.0.0.0` does it consciously, and owns the auth /
TLS / firewall that go with it (out of scope here, §Out of scope). Configuration
is an additive optional block:

```
observability { enabled?: bool (default true), addr?: string (default "127.0.0.1:2112") }
```

**Absent block = server ON with defaults.** The operator turns it OFF explicitly
(`observability.enabled = false`).

**The deliberate asymmetry with `Storage` (documented so it does not read as
incoherent).** `Storage` absent = persistence **OFF** (ADR-0019: absence
preserves Stage 11's stateless behavior; turning it on changes behavior, so it
must be opt-in). `observability` absent = server **ON** (it is safe on loopback,
exposes nothing externally, and is always useful for operating). The rule is the
same underneath — *absence preserves the safe/expected default* — it just points
in opposite directions because for storage the safe default is "behave as before"
and for observability the safe default is "be observable."

**`/healthz` is liveness only — NOT readiness, NOT provider health.** It returns
200 whenever the server is up. It MUST NOT flip red because a provider is
unreachable: ADR-0014 §3 makes a downed Ollama a non-fatal runtime condition
(the binary boots and falls back), so coupling `/healthz` to provider
reachability would let an orchestrator kill a healthy binary over a downed
optional model. Provider/brain health is a metrics concern (the failure
counters), not a liveness gate.

**Lifecycle and the ADR-0008 shutdown order (the real risk — decided here).**
The admin server is an *observer* of the pipeline, not part of it. It should be
the **first thing up and the last thing down**, so `/metrics` and `/healthz`
stay observable across the entire graceful drain:

```
 App.Run (startup):
   1. adminServer.Start    <- /healthz up before channels even connect;
                              an operator sees "process alive" during boot
   2. channels.Start (existing)

 App.Shutdown (ADR-0008 order, admin server appended LAST):
   1. stopChannels         (channel.Stop -> inbound closes -> router pump drains)
   2. router.Shutdown      (drains brain + outbound workers)
   3. store.Close          (only if the router fully drained — ADR-0019 §6)
   4. adminServer.Shutdown <- LAST: metrics/counters observable through the
                              whole drain; then the last network surface closes
```

Rationale for "last down": the operator most wants to watch `/metrics`
(drain progress, final counters) **during** shutdown; killing it first blinds
them exactly then. `http.Server.Shutdown(ctx)` drains in-flight requests, and
`/metrics`+`/healthz` are sub-millisecond handlers, so step 4 adds negligible
time and is bounded by the same shutdown ctx (`main.go:54`, 15s). A failed
admin-server shutdown is joined into the error set like a failing channel, never
masking the rest.

### 5. The dependency — `prometheus/client_golang`, four-axis test

go.mod goes from **2 to 3 direct dependencies**
(`+ github.com/prometheus/client_golang`).

| Axis | Assessment |
|------|------------|
| **Capability gained** | Counters, gauges, **histograms** (percentile-ready latency), and a pull `/metrics` surface — the canonical self-hosted metrics model. `expvar` (stdlib) was rejected: JSON over `/debug/vars`, **no histograms, no labels**, too weak for latency distributions. |
| **Dependency cost** | ~6–7 new indirect modules (`beorn7/perks`, `cespare/xxhash/v2`, `prometheus/client_model`, `prometheus/common`, `prometheus/procfs`, `google.golang.org/protobuf`, `munnerz/goautoneg`); `golang.org/x/sys` is already indirect via sqlite. All from the prometheus org or stdlib-adjacent. Pure-Go, no cgo — keeps the trivial Pi/ARM cross-compile (same constraint that chose `modernc.org/sqlite`). |
| **Reversibility** | High **because of the seam** (§2). The domain imports the `Metrics` interface, never `prometheus`. Swapping to OpenTelemetry, or back to `expvar`, is replacing one leaf impl with no hot-path change. The `/metrics` text format is a mild, standard commitment. |
| **Maturity** | **Layer 1, boring technology** — the de-facto standard for self-hosted pull metrics. Does **not** spend an innovation token. (Full OpenTelemetry — Alternative C — would: 20+ modules, context propagation plumbed through `envelope`/`router`/`brain`, heavier on a Pi. Rejected as premature.) |

## Consequences

### What this enables

- **An operator can actually operate.** `/healthz` for container/Pi liveness
  probes; `/metrics` for message throughput, per-provider latency and failure
  rate, router error kinds, drops, and persisted turns — plus Go runtime/process
  series for goroutines, memory, and GC.
- **Stage 13 has its server.** The control API mounts on the same
  `internal/httpserver` mux and lifecycle; this ADR builds the shape, not the
  endpoints.
- **The backend is swappable.** The `Metrics` seam makes the Prometheus choice a
  leaf-local decision, reversible without touching the domain.

### What this asks / costs

- **A new direct dependency** (3rd), with its indirect tree, accepted under the
  four-axis test (§5).
- **Two new leaf packages** (`internal/metrics` + its `prometheusMetrics` impl)
  and **one** (`internal/httpserver`), plus small edits: `slog` fields on
  existing logs, a metric line in the existing router-error closure, push calls
  in `brain`, a pull `Collector` for `DroppedCount`, and `App` owning the server
  through `Run`/`Shutdown`. The hot path itself is not re-shaped — measurements
  ride existing funnels.
- **The first inbound network surface.** Until now the binary only made outbound
  calls (Telegram polling). `/metrics`+`/healthz` open a port; the bind address
  is operator-configurable (default loopback, §4) and `/healthz` leaks nothing,
  `/metrics` exposes operational counters only.

### Trade-offs accepted

- **No distributed tracing.** A single binary's logs+metrics answer "alive, what
  failed, how slow" without spans. Traces (and the OTel weight they carry) are
  deferred to a later stage if a multi-hop story ever needs them.
- **`/healthz` is liveness, not readiness.** It cannot express "draining" or
  "degraded." Accepted: Telegram inbound is polling, not HTTP-routed, so there is
  no load balancer to drain via readiness; liveness is the useful signal.
- **Cross-message metric attribution is best-effort.** Same as the memory
  contract (ADR-0018): per-provider observations are recorded post-`Run` without
  added synchronization, so a histogram may reflect a one-scrape-stale view.
  Acceptable for operational metrics.

## Alternatives Considered

### A — `slog`-only minimal (no metrics)
Standardize log fields, add nothing else. **Rejected:** zero new deps, but the
operator gets no time series — cannot graph latency or alert on error rate
without scraping and parsing logs. Under-engineered for "let the operator
operate," which is the whole value of this stage.

### C — Full OpenTelemetry (logs + metrics + traces)
Vendor-neutral, future-proof. **Rejected as premature:** 20+ modules; the
Prometheus exporter itself pulls `client_golang` (so OTel-with-pull is strictly
heavier than B); traces require context propagation plumbed through
`envelope`/`router`/`brain` — real hot-path surgery. Spends an innovation token
on a Pi-hostile stack before there is a multi-service story to justify it. The
`Metrics` seam (§2) keeps OTel a future drop-in if that story arrives.

### expvar instead of Prometheus (within B)
Stdlib, zero deps. **Rejected:** no histograms and no labels — it cannot express
a latency distribution or per-provider breakdown, the two things the minimal set
most needs. The seam keeps it available as a fallback impl if the dependency ever
becomes a problem.

### Push the global `DefaultRegisterer`
The library's default path. **Rejected:** it is mutable global state (its
`init()` auto-registers collectors), which CLAUDE.md forbids. A private
`NewRegistry()` with explicit collector registration is the clean equivalent.

## Out of scope (recorded, not silently dropped)

- **Distributed tracing / OpenTelemetry / span propagation** — deferred; the
  `Metrics` seam is the future drop-in point.
- **Dashboards, alerting rules, Grafana/Prometheus-server config** — operator
  side, not the binary's concern.
- **Stage 13 control API** — config/brain-management endpoints. This ADR builds
  only the server *shape* (`/metrics` + `/healthz`); Stage 13 mounts the rest on
  the same mux.
- **Readiness/degraded health semantics, per-provider health endpoints** — later,
  if an HTTP-routed inbound or an orchestrator that consumes readiness appears.
- **Auth/TLS on the admin server** — the minimal cut binds operator-side; TLS and
  access control land with Stage 13's externally-exposed control surface (or via
  a reverse proxy), and are named here so the minimal cut stays minimal.

## Resolved in review (copilot, 2026-06-24)

1. **Admin server — default-on.** Confirmed (§4). Observability you must enable by
   hand is absent exactly when needed. Off only via explicit
   `observability.enabled = false`.
2. **Bind — loopback `127.0.0.1:2112`.** Confirmed (§4). Safe by default (nothing
   exposed to the network); the operator who wants `0.0.0.0` does it consciously
   and owns auth/TLS/firewall. Port `2112` (conventional Go exporter port), not
   `9090` (the Prometheus *server* port — avoided to prevent confusion).
3. **Config shape — `observability { enabled?, addr? }`, absent = ON with
   defaults.** Confirmed (§4), with the deliberate asymmetry vs `Storage`
   documented there (absence preserves the *safe* default, which points opposite
   ways for the two blocks).
4. **Package name — `internal/httpserver`.** Confirmed. It describes the
   mechanism and is reusable by Stage 13; not `admin` (presupposes semantics it
   does not have) nor `observability` (would tie a general HTTP server to
   metrics). The seam package stays `internal/metrics`.
