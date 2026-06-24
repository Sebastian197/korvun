# Stage 12 — Observability

> **Status:** closed
> **Started:** 2026-06-24
> **Closed:** 2026-06-24
> **ADR:** [ADR-0020](../adr/0020-observability.md) (accepted)

## Objective

Let an operator **see what the binary is doing**. Korvun boots, serves Telegram
live, and remembers across restarts (Stages 0–7, 9, 11) — but with no metrics
surface, no liveness endpoint, and structured logs that carried inconsistent
fields. For a single binary meant to run unattended from a Raspberry Pi to the
cloud, "is it alive, what is failing, how slow is it" was the gap this stage
closes.

The stage was framed with `/office-hours` (alternatives generation) and
`/plan-eng-review` (blast radius, boring-by-default + innovation tokens,
reversibility, essential-vs-accidental complexity), then pinned by ADR-0020
before any code.

## The finding that reframed the stage

**The 80% already existed; the measurement points are already funnels**, so
instrumenting them has near-zero blast radius:

- **Logs are already on `log/slog`** everywhere on the hot path (`WithLogger`
  seams in app, router, brain, telegram). What was missing was *field
  consistency*, not a logging library.
- **Per-provider latency is already captured**: `fanout.Outcome.Latency`
  ("observability that costs nothing"). Metrics READ it post-`coord.Run`, off
  the hot path.
- **Failures already funnel through one hook**: the router's
  `WithErrorHandler(func(RouterError))` with a typed `RouterError.Kind`.
- **Drops are already counted**: `telegram.DroppedCount() uint64` (atomic).

The single genuinely new moving part is an **HTTP server** to expose `/metrics`
and `/healthz` — the binary had none (Telegram polls). The real risk of the
stage was the **server lifecycle and its place in the ADR-0008 shutdown order**,
not the counters.

## The cut (Alternative B, one stage, one ADR)

Standardized `slog` fields on the existing funnels (zero new deps) **and** a
`Metrics` seam with a Prometheus implementation behind it, exposed via a small
admin HTTP server that also serves `/healthz`. `slog`-fields and the seam are
the same cut (an operator needs both to operate), so unlike Stage 9 they did not
earn separate ADRs. Traces deferred.

```
   domain: router, fan-out, brain, telegram          internal/httpserver
        │ depends on the metrics.Metrics INTERFACE         (generic server,
        ▼ (never on Prometheus)                             /healthz built-in,
   internal/metrics.Metrics  ── Nop default                 Handle for /metrics)
        ▲                                                        ▲
        │ impl (only pkg importing client_golang)               │ App owns it:
   internal/metrics/prom  ── private NewRegistry,               start FIRST in Run,
        Handler() (promhttp), CounterFunc drop pull   ─────────▶ stop LAST in Shutdown
```

## The four decisions (ADR-0020, resolved in review)

| # | Decision | Why |
|---|----------|-----|
| 1 DEFAULT-ON | The admin server is **on by default** | Observability you must enable by hand is absent exactly when you need it. Off only via explicit `observability.enabled = false`. |
| 2 LOOPBACK | Bind **`127.0.0.1:2112`** by default | Safe by default: a fresh boot exposes nothing to the network. Port 2112 (conventional Go exporter port), not 9090 (the Prometheus *server* port). An operator who wants `0.0.0.0` does it consciously and owns auth/TLS/firewall (out of scope). |
| 3 ABSENT=ON | An absent `observability` block means **ON with defaults** | The **deliberate asymmetry with `Storage`**: absent Storage = OFF (absence preserves Stage 11's stateless behavior); absent observability = ON (safe on loopback, always useful). The rule underneath is the same — *absence preserves the safe default* — it just points opposite ways. |
| 4 PKG | The server lives in **`internal/httpserver`** | Names the mechanism and is reusable by Stage 13; not `admin` (presupposes semantics) nor `observability` (would tie a general HTTP server to metrics). The seam package stays `internal/metrics`. |

**The lifecycle (the real risk, decided in §4 of the ADR).** The admin server is
an *observer* of the pipeline, not part of it, so it is the **first thing up and
the last thing down**, keeping `/metrics` and `/healthz` observable across the
whole graceful drain:

```
 Run startup:   1. adminServer.Start   (binds synchronously: a bind failure is a
                                         loud boot error, not lost in a goroutine)
                2. channels.Start

 Shutdown:      1. channels  2. router  3. store  4. adminServer  (LAST)
                its error joins errors.Join, never masking the rest;
                on a channel-Start failure the admin server is rolled back.
```

`/healthz` is **liveness only** — 200 while serving, decoupled from provider /
brain health: a downed Ollama is non-fatal (ADR-0014 §3), so coupling liveness to
it would let an orchestrator kill a healthy binary.

## What landed

Implemented in four TDD steps (red-first each), the order ADR-0020 set:

1. **`feat` slog funnel fields `2937a43`** — `envelope_id` + `channel` on the
   telegram drop funnel, `envelope_id` on the router-error funnel; extracted
   `logRouterError` so the field vocabulary is unit-testable. Zero deps.
2. **`feat` Metrics seam + Prometheus impl `3892b5c`** — `internal/metrics`
   (interface + `Nop`); `internal/metrics/prom` (the only package importing
   `client_golang`: private `NewRegistry`, explicit Go/process collectors,
   LLM-shaped latency buckets, drop via pull `NewCounterFunc`). `go.mod` 2→3
   direct deps; cross-compile ×6 `CGO_ENABLED=0` confirmed green with the dep.
3. **`feat` admin HTTP server `0e80292`** — `internal/httpserver` (generic
   server, `/healthz`, concurrency-safe `Addr`, `ReadHeaderTimeout` vs
   Slowloris); config `observability{enabled?,addr?}`; `prom.Handler()` keeps
   `promhttp` in the leaf; App owns the lifecycle.
4. **`feat` wire metrics to funnels `0d4e79f`** — `brain.WithMetrics`;
   `IncMessages`, `ObserveProviderDuration`/`IncProviderFailure` from
   `fanout.Outcome`, `ObserveTurnsPersisted`; router-error funnel also
   increments `IncRouterError`; `registerDroppedSources` wires telegram's
   `DroppedCount` as a pull source.

Plus the review fix (step 5):

5. **`fix` Register-not-MustRegister `51b86cb`** — see review findings below.

## What is observed (the six metrics, by funnel)

| Metric | Type | Source funnel |
|--------|------|---------------|
| `korvun_messages_processed_total{channel}` | counter | `brain.Handle` entry |
| `korvun_provider_request_duration_seconds{provider,outcome}` | histogram | `fanout.Outcome.Latency` (already measured) |
| `korvun_provider_failures_total{provider}` | counter | `fanout.Outcome.Err != nil` |
| `korvun_router_errors_total{kind}` | counter | router `WithErrorHandler` funnel |
| `korvun_channel_messages_dropped_total{channel}` | counter (pull) | `telegram.DroppedCount()` via `NewCounterFunc` |
| `korvun_conversation_turns_persisted_total` | counter | `brain.persistTurns` after `AppendTurns` |

Plus the standard `go_*` and `process_*` series from the explicitly-registered
runtime/process collectors.

## Live verification (on integrated master)

The full `korvun` binary cannot boot without a real Telegram token (the `getMe`
boot health-check), so the live check used the exact production packages
(`internal/httpserver` + `internal/metrics/prom`) wired the way `internal/app`
wires them, over a real socket on `127.0.0.1:2112`:

- `GET /healthz` → **200**, body `ok`.
- `GET /metrics` → **200**, exposing all six `korvun_*` series with correct
  values (e.g. `korvun_channel_messages_dropped_total{channel="telegram"} 3`
  read live via the pull collector; histogram `groq/ok` summed 0.24s, `ollama/error`
  5s).

## Review findings (`/review`)

The adversarial pass confirmed all six focus areas correct (lifecycle order,
rollback, error-join, seam concurrency + `Nop` nil-safety, `boundAddr` mutex,
pull collector, `/healthz` liveness-only, config defaulting) and found two latent
issues:

- **F2 (P2) — FIXED (`51b86cb`).** `RegisterDroppedSource` used `MustRegister`,
  which would **panic boot** if two channels ever shared a name. Switched to
  `Register`, returning the error; app logs and skips. A metrics registration
  must never take down the serve path. Unreachable today (the router rejects
  duplicate channel names) but hardens against a future second channel type.
- **F1 (P2) — deferred.** `httpserver.Server.Start` is not re-entrant.
  Unreachable (App calls it once); the reviewer recommended ship-as-is. No
  speculative state guard added.

## Process incident and lesson (honest record)

During step 1, a `git add -A` swept the **parked `CLAUDE.md`** (a "Design spec
first" edit on hold) and the **untracked `.gstack/`** tooling dir into the first
commit, violating their parked status. Caught during `/review` (they showed in
the diff stat). The branch was rewritten with `git filter-branch` to drop both
paths from all commits and restore `CLAUDE.md` to its parked (modified,
uncommitted) state; the rewrite preceded the first push, so origin never held the
dirty version (no force-push). `.gstack/` is regenerable gstack tooling — nothing
of repo value was lost.

**Lesson, now standing:** use selective `git add <paths>`, never `git add -A`,
on any branch that has parked / on-hold files in the working tree.

## What is NOT in scope (recorded, not silently dropped)

- **Distributed tracing / OpenTelemetry** — deferred; the `Metrics` seam is the
  future drop-in point if a multi-service story arrives.
- **Dashboards, alerting rules, Grafana/Prometheus-server config** — operator
  side, not the binary's concern.
- **Stage 13 control API** — config/brain-management endpoints. This stage built
  only the server *shape* (`/metrics` + `/healthz`); Stage 13 mounts the rest on
  the same `internal/httpserver` mux.
- **Auth / TLS on the admin server, readiness/degraded health** — land with
  Stage 13's externally-exposed control surface (or a reverse proxy).

## Dependency

`go.mod` went from **2 to 3 direct deps** (`+ github.com/prometheus/client_golang
v1.23.2`). Layer 1, boring technology — does not spend an innovation token. Pure
Go (no cgo), keeping the trivial Pi/ARM cross-compile. `client_model` stays
indirect (the test parses the metric model only through `testutil`, no direct
import). Reversible: the domain depends on the `metrics.Metrics` interface, never
on Prometheus.

## Quality gate

`make quality` green with `-race` over the whole tree on integrated master
(coverage **92.5%**; `httpserver` 95.5%, `router` 96.0%, `policy` 100%, core
packages ≥ 85%, the ≥90% packages — policy/router/envelope/brain — met).
Cross-compile ×6 `CGO_ENABLED=0` green. The seam invariant is verified: only
`internal/metrics/prom` imports `client_golang`.