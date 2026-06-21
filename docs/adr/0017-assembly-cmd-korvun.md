# ADR-0017: The real assembly (Stage 11) — `cmd/korvun` wires channel → router → brain → channel into one long-running binary

> **Status:** accepted
> **Date:** 2026-06-21
> **Deciders:** Sebastián Moreno Saavedra
> **Builds on:** [ADR-0008](0008-telegram-channel-lifecycle.md) (channel-owned
> `Start`/`Stop`; the shutdown rule "`Stop` is what `main.go` calls BEFORE
> `router.Shutdown`"; `Receive()` returns the inbound channel, closed once at
> `Stop` so a drainer sees a clean "no more updates" signal; polling needs no
> public URL),
> [ADR-0003](0003-router-gateway-core.md) (the router owns concurrency — worker
> pools, bounded queues, enqueue/handler/send timeouts, the async error hook;
> `DispatchInbound` is the inbound entry point),
> [ADR-0010](0010-groq-cloud-provider.md) (API keys env-only: env > Option >
> error, never argv, never logged, never in an error message, never in a config
> file),
> [ADR-0011](0011-model-fanout.md) (`fanout.Coordinator`, `Result`/`Outcome`,
> the mechanism/policy boundary),
> [ADR-0014](0014-brain-orchestrator.md) (the `Orchestrator` is stateless glue,
> NOT structural concurrency; `models`/`policy` are interfaces so it can be
> wrapped; `WithModelID` copies the request per provider),
> [ADR-0015](0015-pre-dispatch-selector.md) (`policy.SelectModels(catalog,
> sensitivity)` runs ONCE at construction; `Locality` is DECLARED in the catalog,
> not read from `model.Model`), and
> [ADR-0016](0016-sequential-coordinator.md) (`sequential.Coordinator` —
> cost-saving fail-over, returns the SAME `*fanout.Result`; the cost story Korvun
> sells for self-hosting).

## Context

Stages 0–7 built every piece of Korvun and proved each one works: the Telegram
channel (ADR-0002/0008), the router (ADR-0003), the model layer with two
differently-shaped providers and two dispatch shapes (ADR-0009/0010/0011/0016),
the two-phase policy engine (ADR-0012/0013/0015), and the Brain that orchestrates
them (ADR-0014). The differentiator — privacy/cost/consensus-aware multi-model
dispatch — exists end-to-end **in code** and is demonstrated by four disposable
demos (`demo-policy`, `demo-brain`, `demo-selector`, `demo-sequential`).

What does not exist is a **product**. Every demo calls `Brain.Handle` (or a
reducer, or `SelectModels`) **directly**. There is no `main.go` that boots a
channel, drains its inbound stream into the router, lets the router hand each
message to a Brain backed by real models, and ships the reply back — and keeps
doing that until a signal stops it. That single-binary assembly is Stage 11, and
it closes V1 checklist criterion 1: *a real message in and out through a real
binary, not a demo* (`ROADMAP-V1.md` §5).

This ADR was framed with `/office-hours` (Premise Challenge + forced
alternatives) and stress-tested with `/plan-eng-review` (reversibility /
blast-radius, boring-by-default, own-your-code-in-production, essential-vs-
accidental-complexity) before any code, because it is a structural integration
point with real, hard-to-reverse design decisions: the config schema, the
router's relationship to its channels, the Brain's coupling to a concrete
coordinator, and boot-time failure behaviour.

### The finding that reframed the whole stage

> **The router is wired only halfway today, and the four demos hid it.**
> `RegisterChannel` starts the **outbound** worker (`router.go:120`,
> `runChannelWorker` drains the outbound queue and calls `Channel.Send`). But the
> **inbound** side has no owner: `router.DispatchInbound` (`router.go:180`) is
> **called from tests only** — verified by grep, nothing in production drains
> `Channel.Receive()` into it. The single most important decision of Stage 11 is
> *who owns the inbound pump*, and the honest answer is: the router already owns
> outbound, so it must own inbound too. Closing that asymmetry is the heart of
> the assembly.

### The single line that frames the whole ADR

> **An assembly is not new behaviour — it is the wiring that turns five proven
> parts into one process that boots, serves, and stops cleanly. Get the seams
> right (the inbound pump in the router, the coordinator behind an interface),
> get the config SHAPE right (it is the one-way door), fail loud on
> configuration and quietly degrade on a downed provider, and keep `main` thin
> enough that everything but `main` is testable.**

### External-docs verification (per CLAUDE.md non-negotiable)

This ADR adds **no new external dependency**. The minimal cut is pure standard
library:

- **Config parsing:** `encoding/json` (stdlib). YAML is explicitly **deferred**
  precisely because it would be a second direct dependency (§2).
- **Signal handling / lifecycle:** `os/signal` + `signal.NotifyContext`
  (stdlib), `context`.
- **Catalog / coordinators / brain / router:** existing internal packages.

`go.mod` therefore stays at its **single direct dependency**
(`github.com/go-telegram/bot`). Context7 covers third-party code libraries, of
which this ADR adopts none, so Context7 verification is not applicable here. The
one stdlib seam worth naming — `signal.NotifyContext(ctx, os.Interrupt,
syscall.SIGTERM)` returning a ctx cancelled on the first signal — is the standard
Go idiom for graceful shutdown and is used exactly as documented.

## Decision

### 0. Three layers: `internal/config` + `internal/app` + a thin `cmd/korvun`

`func main` cannot be unit-tested. So the boot logic that *can* fail — parsing,
validation, catalog assembly, wiring — must live below `main`, where tests reach
it. Two new internal packages plus a ~20-line `main`:

```
┌────────────────────────────────────────────────────────────────────────┐
│ cmd/korvun/main.go        (~20 lines, NOT unit-tested — nothing to test) │
│   parse flags (--config path)                                            │
│   cfg, err := config.Load(path)        ── err → fatal, named, exit≠0     │
│   application, err := app.New(cfg)      ── err → fatal, named, exit≠0     │
│   ctx,stop := signal.NotifyContext(...) (Interrupt, SIGTERM)             │
│   err := application.Run(ctx)           ── blocks until ctx cancelled    │
│   application.Shutdown(shutdownCtx)                                       │
└───────────────┬──────────────────────────────────────┬─────────────────┘
                │ build                                  │ build
                ▼                                        ▼
┌──────────────────────────────┐        ┌──────────────────────────────────────┐
│ internal/config              │        │ internal/app                          │
│  Load(path) (*Config, error) │        │  New(*Config) (*App, error)           │
│  parse (encoding/json)       │ Config │   build models (ollama/groq adapters) │
│  validate → typed Config     │──────► │   SelectModels(catalog, sensitivity)  │
│  resolve env-var REFERENCES  │        │   NewOrchestrator(coord, models, pol) │
│  (NOT the secret values)     │        │   router.New + RegisterBrain/Channel  │
│  fail loud, name the field   │        │   + Route                             │
└──────────────────────────────┘        │  Run(ctx) — Start channel, run, wait  │
                                         │  Shutdown(ctx) — Stop then router     │
                                         └──────────────────────────────────────┘
```

- `internal/config` knows file formats and validation. It knows **nothing** about
  `ollama`, `groq`, `telegram`, the router, or the brain — it produces a typed,
  validated `Config` and stops there.
- `internal/app` knows how to turn a `Config` into a wired, ready-to-run system.
  **The catalog math and the env-var resolution live here**, not in `main` and
  not in `config`. This is the layer tests exercise to assert "this config boots"
  / "this bad config fails with this message".
- `main` is glue: flags in, fatal-or-run out. It does **not** do the catalog math
  or own the pump inline — if it did, neither would be testable.

This is boring-by-default and systems-over-heroes: a tired operator at 3am gets a
named error from a tested code path, not a stack trace from `main`.

### 1. CONFIG — get the SCHEMA right now; the FORMAT is stdlib and swappable; YAML is deferred

Two things people conflate, separated here because they have opposite reversibility:

| | Cost to revert | Why |
|---|---|---|
| **The config *schema*** (what is declarable, the field names and shape) | **HIGH — one-way door** | The moment an operator writes a config file, the field names and structure are a contract. Renaming `model_id` later breaks every deployment. |
| **The config *format/parser*** (JSON vs YAML vs flags) | **LOW — two-way door** | Swappable while the schema is unchanged: the same `Config` struct can be decoded from JSON today and YAML tomorrow. |

**Decision: design the schema now, forward-compatible; decode it with the
standard library (`encoding/json`) for the minimal cut; do NOT adopt YAML yet.**

Why not YAML now, even though `ROADMAP-V1` names `edge.yaml`/`cloud.yaml`: YAML is
a **second direct dependency** (`gopkg.in/yaml.v3` or similar) on the critical
path of the very first binary that boots. It trips CLAUDE.md's four-axis
dependency test and would need its own dependency ADR. That cost buys nothing the
minimal cut needs — `encoding/json` decodes the identical `Config` struct with
zero new dependencies. **YAML and its ADR are deferred to a later stage (likely
packaging, Stage 15), reusing the SAME schema struct**: when it lands it is a new
`Decode` path, not a new schema. The schema is the asset; the format is a detail.

The forward-compatible schema (field shape, not Go yet):

```
{
  "channels": [
    { "type": "telegram", "mode": "polling",
      "token_env": "TELEGRAM_BOT_TOKEN" }        // REFERENCE to an env var name
  ],
  "brains": [
    { "name": "default", "sensitivity": "public", // public | private (ADR-0015)
      "policy": { "kind": "priority",             // priority | consensus
                  "order": ["ollama", "groq"] },
      "dispatch": "fanout",                        // fanout | sequential (§3)
      "models": [
        { "provider": "ollama", "model_id": "llama3.2",
          "locality": "local",  "base_url": "http://localhost:11434" },
        { "provider": "groq",   "model_id": "llama-3.3-70b-versatile",
          "locality": "cloud",  "api_key_env": "GROQ_API_KEY" }  // REFERENCE
      ]
    }
  ],
  "routes": [ { "channel": "telegram", "brain": "default" } ]
}
```

Schema invariants the ADR fixes (so they survive a format change):

- **`locality` is declared per model** and required — it is NOT derivable from
  `model.Model` (ADR-0015 §3); `internal/app` reads it straight into
  `CatalogEntry.Locality`.
- **Secrets are referenced by env-var NAME, never by value** (ADR-0010). The
  config file carries `token_env` / `api_key_env`; `internal/config` (or
  `internal/app`) resolves `os.Getenv(name)` at boot. A config file that ever
  contains a secret value is a bug the validator should reject if detectable, and
  a documented prohibition regardless. (Groq already self-resolves
  `GROQ_API_KEY` inside `groq.New`; Telegram takes its token via
  `WithToken(string)`, so `app` reads `token_env` and passes it. The two patterns
  are reconciled in `app`, not pushed onto the operator.)
- **`dispatch` selects the coordinator shape** (`fanout` or `sequential`),
  enabled by §3.

### 2. PUMP INBOUND — the ROUTER owns it (close the outbound/inbound asymmetry)

Today the router owns one half of its job and not the other:

```
                       ROUTER  (owns concurrency — ADR-0003)
        ┌─────────────────────────────────────────────────────────┐
        │                                                           │
  OUTBOUND (owned today):                                           │
        │   RegisterChannel ─► go runChannelWorker ─► Channel.Send  │
        │                                                           │
  INBOUND (MISSING today):                                          │
        │   Channel.Receive() ──► ??? ──► DispatchInbound           │
        │                          ▲                                │
        │                  nothing drains this in production        │
        │                  (DispatchInbound: tests only)            │
        └─────────────────────────────────────────────────────────┘
```

**Decision: the router gains an inbound pump, symmetric with the outbound worker
it already starts.** `RegisterChannel` (or a sibling method it calls) starts a
goroutine that drains `Channel.Receive()` and calls `DispatchInbound` for each
Envelope. The same registration that already starts `runChannelWorker` for
outbound now also starts `runChannelPump` for inbound — one method, both
directions, so the router is wired symmetrically.

Why the router and not `main`/`app`: the inbound pump touches the **router's
contract** — it is the mirror of the outbound worker, it must respect the same
shutdown fences (`r.ctx`, the channel/brain `WaitGroup`s), and it must close
cleanly when `Channel.Stop` closes the inbound channel. Putting it in `app` or
`main` would split the router's two halves across two packages and make the
shutdown ordering an external concern again. It belongs where its sibling already
lives. Moving it later (from `app` into the router, or vice versa) would churn the
boot path and every test that asserts on it — so it is settled now, in the router,
where reversal is cheapest before code exists.

**Pump error handling — log-and-continue, never crash:** `DispatchInbound`
returns errors (`ErrBrainSaturated`, `ErrNoRoute`, `ErrNoConversationID`,
`ErrShutdown`, …). For a long-running daemon, a single bad Envelope must **not**
take down the process. The pump logs the error via the structured logger (and may
feed the existing async error hook for symmetry with the outbound path) and moves
to the next Envelope. The pump exits cleanly when `Receive()`'s channel is closed
(the ADR-0008 drain signal) or `r.ctx` is cancelled (`Shutdown`).

```
runChannelPump(cw):
  for {
    select {
      case env, ok := <-inbound:
        if !ok { return }                 // Stop closed it → clean exit
        if err := DispatchInbound(ctx, env); err != nil {
          log + (optional) error hook     // never panic, never return
        }
      case <-r.ctx.Done(): return         // Shutdown
    }
  }
```

This is the one piece of Stage 11 that is **new structural concurrency** (a new
goroutine inside the concurrency-owning component) — see §7 for how it is handled.

### 3. COORDINATOR — widen `Orchestrator.coord` to an interface from the first commit

`Orchestrator.coord` is the **concrete** `*fanout.Coordinator`
(`orchestrator.go:41`, `:79`). Verified that `fanout.Coordinator.Run` and
`sequential.Coordinator.Run` have the **identical** signature
`(ctx, *model.Request, []model.Model) (*fanout.Result, error)`. So a one-method
interface captures both:

```go
// package brain — the dispatch seam the Orchestrator runs the request through.
// Both fanout.Coordinator and sequential.Coordinator already satisfy it
// unchanged (verified: identical Run signature, both return *fanout.Result).
type Coordinator interface {
    Run(ctx context.Context, req *model.Request, models []model.Model) (*fanout.Result, error)
}
```

The interface lives in `brain` (the **consumer** side, idiomatic Go). `brain`
already imports `fanout` for the `Result` type, so returning `*fanout.Result`
from the interface introduces no import cycle. `Orchestrator.coord` becomes
`brain.Coordinator`; `NewOrchestrator`'s first parameter changes from
`*fanout.Coordinator` to `brain.Coordinator`. That is ~1 line plus the interface
declaration.

**Why from the first commit, not later:** this is cheap now (pre-code) and
expensive once the binary and its tests assume the concrete `fanout` type. The
**product** reason is decisive: without this widening, the first real binary
**cannot use the sequential fail-over** (ADR-0016) — the dispatch shape that
actually saves money by not calling Groq when Ollama already answered. That
cost-saving fail-over is Korvun's selling point for self-hosting. The binary must
be able to mount **fan-out OR sequential from config** (`dispatch: "fanout" |
"sequential"`, §1). Shipping the binary locked to fan-out and widening later would
mean the headline cost feature is unreachable in v1 of the product — unacceptable
for the stage whose whole point is "a real product that boots."

### 4. TELEGRAM TOKEN — a boot-time `getMe` health-check that fails LOUD

The requirement "a real message in and out" includes "the binary refuses to
pretend it is healthy when it is deaf." Today it can pretend:

- `telegram.New` validates the token is **non-empty** but does **no network
  check** (`adapter.go:160`) — it only builds the `*bot.Bot`.
- In polling mode, `startPolling` calls `DeleteWebhook` and, on failure, **only
  logs a warning and proceeds** (`lifecycle.go:211-217`): *"the polling loop will
  surface any conflict."*

Net effect with an **invalid token in polling mode**: `Start` returns `nil`, the
binary reports a successful boot, and then the polling loop fails forever
internally. The binary "started" but is silently deaf — a direct violation of
"fail with a clear message, not silence."

**Decision: the assembly performs an explicit token health-check at boot
(`getMe`) and treats failure as a fatal boot error**, with a clear message
("Telegram token rejected by the API: <reason>") and a non-zero exit. This is
part of *"the binary really boots"*, not deferrable debt. (Mechanism: an explicit
`getMe` call during `app.Run`'s channel-start step, or a small adapter addition;
the exact placement is an implementation detail for the spec, but the behaviour —
loud fatal on an invalid token — is decided here.)

> **Reconciliation note (Stage 11 close, 2026-06-21 — status stays `accepted`).**
> The premise above ("`telegram.New` does no network check") was **partially
> wrong**, verified against the `go-telegram/bot` v1.21.0 docs via Context7:
> `bot.New` **already calls `getMe`** (with a 5-second timeout) and returns an
> error on an invalid token, UNLESS `WithSkipGetMe()` is passed. The Korvun
> adapter does **not** pass that option, so `telegram.New` → `bot.New` →
> `getMe` already validates the token over the network at construction. The
> getMe boot health-check is therefore **not a new addition** — it lives in the
> library, and `internal/app`'s channel construction (`telegram.New`) surfaces
> its failure, which `app.Build` propagates as a fatal boot error. **The §4
> intent — "fail loud on an invalid token" — is met by construction**, leaning
> on the existing `getMe`, with no redundant explicit call added. Verified live
> at Stage 11 close: with a bogus `TELEGRAM_BOT_TOKEN`, the `korvun` binary
> fails at boot with `telegram: bot.New: error call getMe, unauthorized` and
> exits non-zero (see `docs/stages/STAGE-11.md`). The "silently deaf binary"
> for an invalid token does not occur. (The separate `startPolling`
> `DeleteWebhook`-only-logs behaviour concerns webhook *conflicts*, not token
> validity, and is unaffected.)

### 5. The golden rule — config/boot errors are FATAL and loud; runtime provider errors DEGRADE, never fatal

The whole error model of the binary, fixed in one rule:

| Failure | Class | Behaviour | Grounding |
|---|---|---|---|
| Malformed config file | config | **FATAL**, name the field, exit≠0, no panic | `internal/config` validate |
| Missing required env var (token, or a declared cloud model's key) | config/boot | **FATAL**, name the var | `groq.New` already errors; `app` checks `token_env` |
| Invalid Telegram token | boot | **FATAL** via `getMe` health-check (§4) | new in this ADR |
| Unknown provider / sensitivity / policy kind in config | config | **FATAL**, name the bad value | `SelectModels` already errors on unknown sensitivity |
| Ollama unreachable at boot | runtime provider | **DEGRADE** — boot OK; first message → Brain fallback; optional reachability log line; **never fatal** | `ollama.New` only builds an HTTP client; failure is per-request; ADR-0014 §3 |
| A provider fails mid-conversation | runtime provider | **DEGRADE** — fan-out/sequential records the outcome; policy → fallback | ADR-0011/0014/0016 |
| One bad Envelope in the pump | runtime | **DEGRADE** — log-and-continue | §2 |

The line: **a misconfiguration the operator can fix should stop the binary
immediately and tell them exactly what to fix; a provider being down is a runtime
condition the running system absorbs gracefully.** Ollama not being up yet is the
normal Raspberry-Pi case (Korvun may start before Ollama) — making it fatal would
be wrong. An invalid token is an operator error — making it silent is wrong.
No `panic` on any normal path (CLAUDE.md).

### 6. Lifecycle — startup and shutdown order (pinned by ADR-0008/0010, documented here)

```
STARTUP (internal/app):                         SHUTDOWN (SIGINT / SIGTERM):
  1. resolve env-var refs (token, keys)           1. signal.NotifyContext cancels ctx
  2. build model adapters (ollama.New, groq.New)  2. channel.Stop(ctx)   ── closes Receive()'s chan
  3. SelectModels(catalog, sensitivity)           3. inbound pump sees closed chan → exits (§2)
  4. NewOrchestrator(coord, selected, policy)     4. router.Shutdown(ctx) ── drains brain workers
  5. router.New                                       + outbound channel workers
  6. RegisterBrain / RegisterChannel / Route      (ADR-0008 §4b: channel.Stop BEFORE router.Shutdown)
  7. channel.Start(ctx)  ── getMe health-check (§4); inbound begins
  8. inbound pump runs (started by RegisterChannel, §2)
  9. block until ctx cancelled
```

`channel.Stop` before `router.Shutdown` is the ADR-0008 ordering rule, restated:
stopping the channel closes the inbound channel, which lets the pump drain to a
clean stop **before** the router tears down the workers that consume what it
produced. `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)` is the
stdlib idiom for step 1.

### 7. Domain & branching — the router/pump change goes on a feature branch with `/review`; config/app/main land with TDD

This stage spans two natures of work, and they get different treatment:

- **`internal/config`, `internal/app`, `cmd/korvun`, the `brain.Coordinator`
  interface** are **sequential glue** — parse, validate, assemble, a one-method
  interface, a thin `main`. No new goroutines. Following the precedent of
  ADR-0014 (Brain) and ADR-0016 (sequential coordinator), sequential glue lands
  **directly with TDD**, `make quality` green under `-race`.

- **The router inbound pump (§2) is genuinely new structural concurrency**: a new
  goroutine inside the component that owns concurrency, sharing the router's
  shutdown fences. The project's own history says this is the dangerous class —
  Phase 2E.8's `close(channel)`-after-`Wait` race and Phase 4.3's
  zero-value-clock race both lived at the intersection of two individually-correct
  concurrency features (HANDOFF). **Decision: the router/pump change goes on a
  feature branch and gets `/review` on the code before merge**, exactly the
  treatment the project reserves for structural-concurrency phases. The
  pump must be proven under `-race` to: drain in order, exit cleanly on a closed
  inbound channel, exit cleanly on `Shutdown`, and never send into a torn-down
  router (the same fence family `runChannelWorker` already respects).

Sequencing follows "make the change easy, then make the easy change": land the
`brain.Coordinator` interface (pure refactor, behaviour-preserving) and the
router pump first; then `config` + `app` + `main` assemble over the stable seams.

### 8. The minimal cut

The smallest thing that closes V1 criterion 1:

- **1 channel:** Telegram in **polling** mode (the Pi has no public URL —
  ADR-0008).
- **1 brain:** sensitivity `public` (exercises the Selector while admitting both
  providers; `private` is a config flip away to demo local-only).
- **Catalog:** Ollama (`local`) + Groq (`cloud`).
- **Dispatch + policy:** `fanout` + `PriorityReducer{Order:["ollama","groq"]}`
  for the first boot; `sequential` reachable by a one-word config change thanks to
  §3.
- **Outcome:** a real Telegram message enters, is routed, models respond, the
  policy decides, and the reply returns — all in `cmd/korvun`, no demo.

On stage close, **the four demos are deleted** (`demo-policy`, `demo-brain`,
`demo-selector`, `demo-sequential`) — the real binary supersedes them
(ADR-0014/0015/0016 each flagged its demo as deleted here).

## Consequences

### What this enables

- **A product that boots** — V1 checklist criterion 1 closed: a real message in
  and out through a real binary.
- **Config-by-file** without recompiling (the schema; the format follows).
- **Both dispatch shapes from config** — fan-out OR the cost-saving sequential
  fail-over, because the coordinator is now an interface (§3). The headline cost
  feature is reachable in v1 of the product.
- **A symmetric router** — it owns inbound and outbound, closing an asymmetry
  that the demos had been hiding (§2).
- **A boot that tells the truth** — fatal-and-named on misconfiguration, graceful
  on a downed provider (§4/§5).
- **Testable boot logic** — everything but `main` is exercised by tests
  (§0).

### What this asks / costs

- Two new packages (`internal/config`, `internal/app`), a real `cmd/korvun`, a
  one-method interface in `brain`, and a new goroutine in `router`.
- The router change is structural concurrency → a feature branch + `/review`
  (§7).
- `internal/config` carries validation tests to the project bar (every fatal path
  named and asserted); `internal/app` carries assembly tests (good config boots;
  each bad config fails with the right message); the router pump carries
  `-race` concurrency tests (§7). Coverage ≥85% core, the router stays ≥ its
  current bar.
- A documented prohibition (and, where detectable, a validator rejection) against
  secret values appearing in the config file.

### Trade-offs accepted

- **stdlib JSON now, YAML later.** Accepted: the schema is the one-way door and is
  fixed now; the format is swappable and JSON costs zero dependencies. YAML's nicer
  ergonomics do not justify a second direct dependency on the first binary's
  critical path (§1). The `ROADMAP-V1` `edge.yaml`/`cloud.yaml` vision is honoured
  later by adding a decode path over the same schema, not by changing the schema.
- **Pump in the router, not in `app`.** Accepted: it is the mirror of the outbound
  worker and shares the router's fences; splitting the two halves across packages
  would re-export the shutdown ordering as an external concern (§2).
- **Coordinator interface in `brain`, returning `*fanout.Result`.** Accepted: a
  consumer-side interface is idiomatic and avoids an import cycle; the slight
  oddity that a `brain` interface names a `fanout` type is the honest cost of
  keeping `Result` where it already lives (ADR-0016 A3 made the same call).
- **A boot-time `getMe` round-trip.** Accepted: one network call at startup buys
  the difference between "boots and serves" and "boots and is silently deaf" (§4).

## Alternatives Considered

### A1 — Put the inbound pump in `internal/app` (or inline in `main`)
**Rejected (§2).** The pump is the mirror of the outbound worker the router
already owns and must share the router's shutdown fences. Hosting it in `app`
splits the router's two halves across packages and turns the ADR-0008 shutdown
ordering back into an external concern. Inlining it in `main` makes it untestable.
The symmetric home is the router.

### A2 — Adopt YAML now (match `edge.yaml`/`cloud.yaml` immediately)
**Rejected (§1).** A second direct dependency on the first binary's critical path,
tripping the four-axis dependency test and needing its own ADR, for ergonomics the
minimal cut does not need. The schema — the part that is expensive to revert — is
designed now; the YAML decode path is an additive later change over the same
struct.

### A3 — Keep `Orchestrator.coord` concrete; widen to an interface only when sequential is wired
**Rejected (§3).** Cheap now, expensive once the binary and its tests assume the
concrete `fanout` type. More importantly it would ship the first product binary
unable to mount the sequential fail-over — disabling Korvun's headline cost
feature in the very release whose purpose is "a real product."

### A4 — Treat the invalid-token-silent-failure as known debt, defer the health-check
**Rejected (§4).** A binary that reports a healthy boot and then silently receives
nothing violates the stage's own success criterion and the "fail with a clear
message, not silence" requirement. A `getMe` at boot is a few lines and is part of
"the binary really boots," not a later hardening task.

### A5 — One big `main.go` that wires everything inline (no `config`/`app` split)
**Rejected (§0).** `func main` is not unit-testable, so all the boot logic that
can fail (parse, validate, catalog assembly, env resolution) would be unreachable
by tests. The two-layer split keeps `main` thin and everything that can fail
tested — boring-by-default, systems-over-heroes.

### A6 — A standalone `internal/runtime` package owning the pump, separate from the router
**Rejected (§2).** Same defect as A1 in a different wrapper: it places a goroutine
that depends on the router's internal fences outside the router. The router already
owns concurrency (ADR-0003); the inbound pump is concurrency; it stays inside the
router.

## Out of scope (recorded, not silently dropped)

- **YAML config + its dependency ADR** — deferred to packaging (Stage 15),
  reusing this schema (§1).
- **Multiple channels / multiple brains in one binary** — the schema is shaped as
  arrays to allow it, but the minimal cut wires one of each (§8). Multi-brain
  resource limits are Stage 7-followup / V1 §4.
- **Persistence / conversation memory** — Stage 9; the Orchestrator stays
  stateless (ADR-0014).
- **Observability system** (Prometheus, OTel, the `DroppedCount` saturation
  metric) — Stage 12; this ADR uses `slog` only.
- **Control API / hot reload of config** — Stage 13; config is read once at boot.
- **The no-code builder** — Stage 14; policies are expressed in the config schema,
  not visually, in this cut.
- **Secret managers beyond env vars** — V1 §2 security; env-only stands
  (ADR-0010).
- **Retry / backoff / circuit breakers** for downed providers — a retry *policy*,
  its own future design (ADR-0016 out-of-scope); this cut degrades to the Brain
  fallback.
- **WhatsApp and other channels** — ADR-0002 deferral stands.