# ADR-0011: Model fan-out — wait-all mechanism, deterministic-order outcomes, mechanism-not-policy

> **Status:** accepted
> **Date:** 2026-06-20
> **Deciders:** Sebastián Moreno Saavedra
> **Amends:** [ADR-0009](0009-model-interface-and-ollama.md) (additively — uses the `Model` interface as-is, defers the Registry once more) and [ADR-0010](0010-groq-cloud-provider.md) (reads, does not extend, the cloud-shaped sentinel grammar)

## Context

Phase 4.3 closes Stage 4 (Models) by adding the **mechanism** every
multi-provider reasoning component in Korvun will eventually use: a
fan-out coordinator that takes N `model.Model` values, dispatches a
single `*model.Request` to each in parallel, and collects the
outcomes. It is the first piece of heavyweight concurrency in the
project after the router (Stage 3) and the Telegram channel
lifecycle (Phase 2E.8); both of those landed clean under `-race`,
and that bar carries forward.

### The single line that frames the whole ADR

**4.3 ships the mechanism of fan-out. It does NOT ship the policy of
what to do with the results.** That distinction is the load-bearing
constraint of every decision below. Choosing a "winning" response,
voting / consensus, privacy-aware filtering, cost-bounded budgeting,
fail-over chains — all of those are dispatch-policy concerns and
belong to the policy engine of Stages 5–6 driven by the no-code
visual builder. The fan-out coordinator in 4.3 has one job: launch
the calls, collect the results, hand them back to the caller. The
caller (today: the live skeleton; tomorrow: the policy engine; later:
the Brain) decides what to do with them.

Crossing that line in 4.3 — e.g. exposing a "strategy" flag with
`first` / `majority` / `consensus` modes — would either commit to
policy semantics this ADR has no authority to pin (Stages 5–6 own
that surface), or expose multiple modes that the policy engine will
have to re-wrap anyway. Either move wastes the abstraction. The
sharper move is one mechanism with no policy semantics, and let
Stages 5–6 layer policy on top.

### What is already on master that this design must mesh with

- `internal/model.Model` — `Generate(ctx, *Request) (*Response, error)`
  + `Name() string`. Two production-shaped implementations:
  `internal/model/ollama` (local) and `internal/model/groq` (cloud).
  The contract requires ctx propagation all the way through the
  underlying HTTP request (ADR-0009 §3); both adapters honour this.
- `internal/model.Response{Message, Provider, ModelName}`. The
  `Provider` field exists precisely so a fan-out result keeps its
  attribution without a side channel (ADR-0009 "Consequences"
  acknowledged the apparent redundancy with `Model.Name()` and
  deliberately accepted it because of this future use).
- `internal/model.ValidateRequest` — the universal upstream invariant
  check. Adapters already call it as the first thing inside
  `Generate`; the fan-out also calls it once at the top, so callers
  with a malformed request never spawn goroutines.
- The cloud-shaped error grammar from ADR-0010 §1:
  `ErrAuthInvalid`, `ErrRateLimited` (+ `*RateLimitError{Provider,
  RetryAfter}`), `ErrProviderUnavailable`, `ErrProviderResponse`.
  Per provider; the fan-out preserves these unchanged — it never
  invents a new per-provider error and never strips one.
- `internal/router` — Stage 3's concurrency reference. Worker
  pools, bounded queues, `sync.WaitGroup` + `context.CancelFunc`
  for coordinated shutdown, ctx-bounded handler timeouts. The same
  primitives carry over.
- `internal/channel/telegram` — Phase 2E.8's reference for "close a
  shared channel exactly once, after the WaitGroup waits". Failing
  that invariant produces the class of `send on closed channel`
  panic + late-arrival race that 2E.8 was explicitly designed to
  avoid. The fan-out faces the same temptation (a collection
  channel) and the same trap; §3 below picks the layout that
  removes the question.

### External-docs verification (per CLAUDE.md non-negotiable)

The fan-out coordinator is written in **stdlib Go** — `sync`,
`context`, `time`, plus the existing `model` package. Stdlib needs
no version pin and no Context7 lookup for shape (the API is part of
the language toolchain Korvun already depends on at `go 1.26.4`).

The one external candidate considered — `golang.org/x/sync/errgroup`
— was verified via Context7 (`/websites/pkg_go_dev_golang_org_x_sync`,
benchmark score 86, source reputation High). Verified surface
that matters to this decision:

- `errgroup.Group.Go(f func() error)` — spawns a goroutine in the
  group; **the first non-nil error returned by any `f` cancels the
  associated `context.Context`** and is the value returned by
  `Wait()`.
- `errgroup.WithContext(ctx)` — returns the derived context whose
  cancellation is triggered by the same first-error rule above (or
  by `Wait` returning, whichever comes first).
- `Group.SetLimit(n)` — caps concurrent goroutines.

The fail-fast cancellation semantics — the first model error
aborts every other in-flight call — are **the exact opposite** of
what a fan-out of LLM providers wants. If Ollama returns 500 while
Groq is still generating, we want Groq's result; we do not want to
cancel Groq because of Ollama's failure. errgroup is the right tool
for "the first of these N operations to fail dooms the whole task";
fan-out is the inverse case. The decision is therefore stdlib
`sync.WaitGroup` + `context.WithCancel`, mirroring the router
(Stage 3) for symmetry. §2 below picks this with the full rationale.

`go.mod` today has exactly one direct dependency (`go-telegram/bot
v1.21.0`); 4.1 and 4.2 both held the "minimal supply chain"
principle by hand-rolling against stdlib. 4.3 holds the same line.

### Structural questions to answer

Six decisions need to be pinned before any 4.3 code lands.

1. **Collection shape.** What does the coordinator return — wait-all
   (every result), first-OK (a race), majority, configurable?
   Crossed by the mechanism-vs-policy line above.
2. **Timeouts and cancellation.** Total deadline vs per-model
   deadline; what happens to a slow provider when the deadline
   elapses; how late results never arrive at the caller.
3. **Partial-failure shape.** 3 of 5 respond, 2 fail with mixed
   reasons (one 429, one network down) — what does the caller see,
   and how does the existing sentinel grammar (`ErrAuthInvalid`,
   `*RateLimitError`, `ErrProviderUnavailable`, `ErrProviderResponse`)
   survive untouched.
4. **Result type.** A flat `[]*Response`, an outcome struct per
   model, or something stream-shaped. Crossed by the attribution
   problem (which provider said what) and the race-safety problem
   (how does it get assembled without a collection channel close).
5. **Registry.** ADR-0009 §5 deferred the Registry "until 4.3 needs
   it"; does 4.3 actually need it, or does it consume `[]Model`
   directly?
6. **Package location.** Sibling of `internal/model/ollama` and
   `internal/model/groq`, or somewhere else? And: does the
   coordinator itself satisfy `model.Model`?

## Decision

### 1. One mechanism: wait-all-with-deadline. No strategy flag, no policy semantics.

The coordinator dispatches the request to every supplied `model.Model`
in parallel and returns when every one of them has returned (with
either a response or an error), or when the caller's `context`
deadline fires — whichever happens first. The result carries one
`Outcome` per supplied model, in input order. Late results never
surface (see §2).

```go
package fanout

// Outcome is one model's contribution to a fan-out call. Exactly one
// of Response or Err is non-nil. Provider is always Model.Name() of
// the source adapter (NOT the provider-self-declared
// model.Response.Provider) so attribution is consistent between
// success and failure outcomes and so the policy layer can route by
// the identity Korvun controls, not the value the adapter declares
// about itself. The self-declared label remains available at
// Outcome.Response.Provider for callers that want it. Latency is the
// wall-clock time the goroutine spent inside Model.Generate; it is
// observability that costs nothing to capture at this layer and is
// too useful to throw away.
type Outcome struct {
    Provider string
    Response *model.Response
    Err      error
    Latency  time.Duration
}

// Result is what FanOut.Run returns when the call itself succeeded
// (mechanism-wise: input was valid, the goroutines launched, the
// coordinator gathered every outcome). Per-model failures live in
// Outcome.Err — they are not Result-level errors.
//
// Outcomes is the same length as the input []Model, in the same
// order. Callers can therefore correlate input-position to outcome
// without a name match: outcomes[i] is the result for models[i].
type Result struct {
    Outcomes []Outcome
}

type Coordinator struct { /* unexported configuration */ }

type Option func(*Coordinator)

func WithPerModelTimeout(d time.Duration) Option

func New(opts ...Option) *Coordinator

// Run dispatches req to every model in parallel. Returns *Result when
// the mechanism completed (every goroutine returned, ctx may or may
// not have fired during the call). Returns a non-nil error ONLY for
// mechanism-level problems: nil ctx, nil request, validation failure
// on req, empty models slice. A model that itself failed surfaces
// as Outcomes[i].Err, never as Run's error.
func (c *Coordinator) Run(
    ctx context.Context,
    req *model.Request,
    models []model.Model,
) (*Result, error)
```

#### Why wait-all and not first / majority / configurable

- **Wait-all covers every policy as a degenerate case.** A "first
  OK" policy reads Outcomes in order and stops at the first
  `Response != nil`. A "majority" policy buckets Outcomes by
  semantic equivalence and counts. A "consensus" policy synthesises
  from the slice. None of these need the mechanism to make the
  choice — they all need to see the slice. The mechanism that
  always produces the full slice is the smallest superset of every
  policy Stages 5–6 might want to compose.

- **First-of-N as a *mechanism* would commit to policy now.** The
  hard question of "first what?" — first 2xx, first non-rate-limit,
  first with content length above a floor — is policy. A mechanism
  that picks "first to return any value" is rarely what a real
  caller wants, and a mechanism that filters on quality is policy
  with a fig leaf. The clean shape is: no filter in the mechanism,
  filtering lives one layer up.

- **Majority / consensus require both the slice AND policy.** They
  are not even cleanly separable: "majority of what?" depends on
  whether two slightly different completions count as agreeing.
  Stages 5–6 will own that judgement; the mechanism would only get
  it wrong.

- **Configurable strategies are the worst of both worlds.** They
  exposed an API surface to Stages 5–6 that Stages 5–6 will have to
  re-wrap anyway (because the policy engine speaks "policies", not
  "fan-out strategies"). They commit the mechanism layer to API
  decisions the policy layer is better placed to make.

The strict discipline: **one method, one shape, no mode flag.**
Stages 5–6 wrap the mechanism with whatever policy they need; the
mechanism never branches on a strategy name.

#### Mechanism boundary, explicitly stated

For the record so a future reader can audit drift:

| In scope (mechanism — this ADR) | Out of scope (policy — Stages 5–6) |
|---|---|
| Launching N parallel calls to `Model.Generate` | Choosing which provider gets called at all |
| Propagating ctx cancellation to every child call | Picking a "winning" response out of N outcomes |
| Bounding per-model and total wall-clock time | Voting, consensus, semantic equivalence checks |
| Surfacing every per-model outcome with attribution | Privacy-aware filtering (e.g. "no cloud for personal data") |
| Preserving the existing sentinel grammar | Cost / quota-aware budgeting and fail-over chains |
| Capturing per-model latency for observability | Retry policy (the adapter / policy decides, not the mechanism) |
| Determinism: same input order → same outcome order | Naming providers, looking them up by string, registry |

When in doubt, the test is: does this require knowing *what* the
caller wants out of the call, beyond "give me everyone's answer"?
If yes, it is policy. If no, it is mechanism.

### 2. Timeouts and cancellation — total deadline via caller's ctx; optional per-model deadline; late results never surface

Two clocks, with a strict propagation chain.

- **Total deadline = the caller's `ctx`.** No new "total fan-out
  timeout" option is added. Callers that want a total deadline pass
  `context.WithTimeout(ctx, d)`; this is the idiom every other
  Korvun call site already uses (router handler bound, channel send
  bound, model Generate bound). Adding a parallel "fan-out total
  timeout" option would invent a second way to do the same thing.

- **Per-model deadline = opt-in via `WithPerModelTimeout(d)`.** When
  set with `d > 0`, each child goroutine derives its own ctx as
  `context.WithTimeout(runCtx, d)` so a slow provider returns an
  error (`ctx.Err()` wrapped through whatever the adapter wraps
  ctx-derived failures in — typically `ErrProviderUnavailable`)
  while the rest keep running. **`d ≤ 0` is rejected.** The option
  is a no-op when given a non-positive value, leaving the coordinator
  at its "no per-model timeout" default. Reason: passing
  `WithPerModelTimeout(0)` or a negative duration to
  `context.WithTimeout` returns an already-cancelled ctx, which would
  fire `ctx.Err()` on every child at zero ms before any HTTP request
  goes out — every Outcome would be a `ctx.DeadlineExceeded`-wrapped
  failure with no upstream signal. The foot-gun is too easy to hit
  silently; the option refuses the input instead. When unset (or
  reset by a non-positive value), every child shares the same per-Run
  ctx derived from the caller's ctx alone.

- **The per-Run ctx is `context.WithCancel(callerCtx)`.** The
  coordinator unconditionally derives a cancellable child ctx and
  `defer cancel()` it before returning. Reason: when `Run` returns
  (normal path, ctx cancel, or otherwise), any goroutine that is
  *still inside* `Model.Generate` receives the cancellation
  immediately and can wind down its HTTP request. ADR-0009 §3 made
  ctx propagation a hard requirement of every `Model` implementation,
  so this propagation lands at the network socket.

- **Late results NEVER surface.** The coordinator writes outcomes
  into a pre-allocated slice indexed by input position (see §4).
  When `Run` returns, the slice is the result; any goroutine that
  has not yet written is reflected as an Outcome whose `Err` is the
  ctx error, *because* the goroutine wrote it before returning. The
  classic race — "result arrives after the receiver gave up" — is
  structurally impossible because the receiver never gives up: it
  waits for every goroutine via `sync.WaitGroup.Wait()`.

#### The cooperative-cancellation invariant, made explicit

`Run` blocks until every spawned goroutine has returned. This is by
design and the price of correctness:

- An adapter that ignores ctx (a bug in the adapter, not in
  fan-out) blocks the goroutine, which blocks `WaitGroup.Wait()`,
  which blocks `Run`. The alternative — abandoning the goroutine
  after some grace period — leaks goroutines, leaks file
  descriptors, leaks live HTTP connections. That is *worse* than
  blocking, because it is invisible. Failure should be loud.
- Both production adapters (ollama, groq) honour ctx via
  `http.NewRequestWithContext`. Their tests exercise mid-flight ctx
  cancellation. This invariant is therefore enforced upstream of
  fan-out, by the adapter test suites.
- The invariant is documented on `Coordinator.Run` and on the
  fan-out package doc. Future adapters that violate it will hang
  the fan-out under load; this is the right failure mode (it
  surfaces fast and points at the offending adapter).

#### Panic isolation — every goroutine recovers, panics become Outcomes

The coordinator's goroutines wrap the `Model.Generate` call in a
`defer ... recover()`. A panic inside an adapter (or inside
`Model.Name()`) does NOT propagate up the goroutine stack to the
runtime; it surfaces as `Outcome.Err = fmt.Errorf("fanout:
provider panicked: %v", r)` in that one slot, with the other
slots untouched. The goroutine's `wg.Done()` still fires via
defer chain; `Run` returns the populated `Result` like any other
partial-failure case.

This is the *raison d'être* of fan-out: isolate provider failures
from each other AND from the host process. A buggy adapter that
panics on a malformed response would, without `recover`, take down
Korvun entirely; with `recover`, it produces one failed Outcome
the policy can treat just like any other provider failure (it's a
recoverable-or-not call the policy makes, not a process-level
fatality the mechanism imposes).

The cooperative-cancellation invariant from the previous bullet is
NOT relaxed by this: an adapter that *blocks indefinitely* still
blocks `Run` (no panic occurs, so no recover triggers). Panic
isolation handles the "adapter throws" failure mode; ctx-respect
handles the "adapter hangs" failure mode. Different defenses for
different bugs.

Note: this deliberately catches user-mode panics. It does NOT
catch runtime fatalities (out-of-memory, unrecoverable signals);
those terminate the process by Go's design and no library can
recover from them.

#### Per-model timeout chained under the caller's ctx, not above it

`WithPerModelTimeout(d)` derives `context.WithTimeout(runCtx, d)`,
not `context.WithTimeout(callerCtx, d)`. Consequence: if the caller
hands in `ctx` with a 10 s deadline and the per-model timeout is
30 s, the per-model ctx still fires at 10 s (the inner deadline
wins via the parent chain). This is the standard semantics of
`context.WithTimeout` — the documentation says so; we just lean on
it instead of papering over with a custom rule.

### 3. Partial failure shape — preserve the existing sentinel grammar, one `Outcome` per model, no new per-provider sentinels

The coordinator does not invent error categories. Every `Outcome.Err`
is whatever the adapter returned, untouched. The grammar that the
caller already speaks (after 4.1 + 4.2) carries over verbatim:

| `Outcome.Err` is one of | Source | Policy interpretation |
|---|---|---|
| `nil` | success | `Outcome.Response` is set; use it |
| wraps `ErrRateLimited` (via `*RateLimitError{Provider, RetryAfter}`) | provider returned 429 | recoverable by waiting; policy may retry |
| wraps `ErrAuthInvalid` | provider returned 401 / 403 | not recoverable; page the operator |
| wraps `ErrProviderUnavailable` | network down, ctx cancel, 5xx | recoverable soon; policy may retry |
| wraps `ErrProviderResponse` | malformed body, empty content, 4xx mis-shape | not recoverable on the provider side; this is a config / shape bug |
| wraps `ErrNilRequest` / `ErrEmptyModel` / etc. (validation) | the request itself was malformed | the policy NEVER sees this — `Run` rejects validation upstream of any goroutine spawn (§4 + §6) |

The fan-out package introduces TWO new sentinels, both **mechanism-
level**, never per-model:

```go
// internal/model/fanout/errors.go (new)

var (
    // ErrNoModels is returned by Coordinator.Run when the models
    // slice is empty. A fan-out with zero participants has no
    // meaningful outcome; this is a caller-side configuration bug.
    ErrNoModels = errors.New("fanout: no models")

    // ErrNilModel is returned by Coordinator.Run when any element of
    // the models slice is nil. Same category as ErrNoModels: caller-
    // side configuration bug, surfaced before any goroutine spawns.
    ErrNilModel = errors.New("fanout: nil model entry")
)
```

`ErrNoModels` and `ErrNilModel` are the *only* errors `Run` returns
outside of `model.Validate*` propagation and direct ctx errors at
entry. They both surface BEFORE any goroutine spawns: the
coordinator validates inputs once, up front, then either errors out
cleanly or commits to producing a full `Result`. The principle is
"once `Run` is past validation, it always returns a `Result`" — no
half-spawn-half-error states.

Justification for not adding a "fan-out-level partial failure"
sentinel:

- "Some succeeded, some failed" is not an error; it is the *normal
  shape* of every fan-out call where one provider is healthier than
  another. Wrapping it in an error sentinel forces every caller to
  string-match through `errors.Is` to discover the obvious. The
  result type already encodes the shape (Outcomes has nil + non-nil
  Err fields per slot).
- Stages 5–6 will categorise outcomes by their existing sentinels
  (`ErrRateLimited` → retry, `ErrAuthInvalid` → page operator, etc.).
  Adding a "fan-out-partial" sentinel layered on top is policy-
  shaped vocabulary that the mechanism has no place imposing.

### 4. Result type — pre-allocated slice of `Outcome`, deterministic order, no collection channel

The internal layout is:

```go
// Coordinator is constructed via New. The zero value is not usable
// directly; Run defends against it by lazily defaulting the clock,
// but callers should prefer New for explicit configuration.
type Coordinator struct {
    perModelTimeout time.Duration   // 0 = unset (no per-model bound)
    now             func() time.Time // package-private clock; defaults to time.Now via New (and Run if zero value)
}

func (c *Coordinator) Run(
    ctx context.Context, req *model.Request, models []model.Model,
) (*Result, error) {
    if ctx == nil { return nil, errors.New("fanout: nil ctx") } // mechanism-level
    if err := model.ValidateRequest(req); err != nil { return nil, err }
    if len(models) == 0 { return nil, ErrNoModels }
    for _, m := range models {
        if m == nil { return nil, ErrNilModel }
    }
    if err := ctx.Err(); err != nil { return nil, err } // caller already cancelled

    // Zero-value defense: a Coordinator constructed by `var c Coordinator`
    // (skipping New) has c.now == nil; lazily default rather than panic
    // on the first c.now() call inside callOne. New still sets it eagerly
    // so explicit construction is observably equivalent.
    if c.now == nil { c.now = time.Now }

    runCtx, cancel := context.WithCancel(ctx)
    defer cancel()

    outcomes := make([]Outcome, len(models))
    var wg sync.WaitGroup
    wg.Add(len(models))

    for i, m := range models {
        go c.callOne(runCtx, req, m, &outcomes[i], &wg)
    }
    wg.Wait()

    return &Result{Outcomes: outcomes}, nil
}

func (c *Coordinator) callOne(
    runCtx context.Context, req *model.Request, m model.Model,
    out *Outcome, wg *sync.WaitGroup,
) {
    defer wg.Done()
    // Panic isolation: a panicking adapter surfaces as a failed Outcome,
    // never as a process-level fatality. The recover MUST sit between
    // wg.Done (above, runs last) and any field write that depends on
    // m.Name() succeeding (since Name() is one of the things that could
    // panic). out.Provider is set unconditionally below to m.Name(); if
    // Name() itself panics, recover catches that too and the slot lands
    // with Provider == "" plus the panic error. The policy can tell
    // panic-failures from regular failures via the "fanout: provider
    // panicked:" prefix in Err.
    defer func() {
        if r := recover(); r != nil {
            out.Err = fmt.Errorf("fanout: provider panicked: %v", r)
            // Provider may already be set if the panic came from Generate
            // rather than Name(); leave it.
        }
    }()
    // Attribution comes from m.Name() — the identity Korvun controls,
    // not the value the adapter self-declares via resp.Provider. The
    // self-declared label remains available at out.Response.Provider
    // for callers that want it.
    out.Provider = m.Name()
    callCtx := runCtx
    var cancel context.CancelFunc
    if c.perModelTimeout > 0 {
        callCtx, cancel = context.WithTimeout(runCtx, c.perModelTimeout)
        defer cancel()
    }
    start := c.now()
    resp, err := m.Generate(callCtx, req)
    elapsed := c.now().Sub(start)
    out.Latency = elapsed
    if err != nil {
        out.Err = err
        return
    }
    out.Response = resp
}
```

#### Test seam for `now func() time.Time`

The `now` field is **package-private**, not surfaced through an
Option. A `WithNow(...)` exported Option would be testing-leakage in
the production API: real callers have no reason to inject a clock,
and the only thing they would mistake the knob for is rate-limiting
or scheduling — neither of which the fan-out does. Instead, tests
that need a deterministic or controllable clock live in the same
package (`package fanout` declared in `fanout_test.go`) and assign
the field directly:

```go
// fanout_test.go
package fanout

func TestRun_latency(t *testing.T) {
    c := New()
    fake := newFakeClock()
    c.now = fake.Now // package-private field access; same-package test
    // ...
}
```

This is the same shape Go's own stdlib uses for clock-injection in
internal tests (e.g. `time.afterFunc` testing in `runtime`). The
production seam is `New(...)` and `Option`s; the test seam is field
access from the same package. The two never collide.

#### Why a pre-allocated indexed slice, not a collection channel

The race trap from Phase 2E.8 (`close(a.inbound)` after `Stop` waits
for workers) is a *general* shape: any time you have N writers and
one reader that wants to know "are we done", a channel needs an
explicit close-once protocol or the reader sees `send on closed
channel` panics, or stale late results, or both.

A pre-allocated `outcomes[i]` slice removes the question entirely:

- Each goroutine writes ONLY to `outcomes[i]` for the unique `i` it
  was launched with. **No two goroutines write to the same memory
  location**, so there is no data race on the slot — even though
  the slice as a whole is shared, the access pattern is partitioned.
- `WaitGroup.Wait()` gives the reader a happens-before fence: every
  goroutine's `wg.Done()` synchronises-before `wg.Wait()` returns
  in the Go memory model, which means every write to
  `outcomes[i]` is visible to the post-`Wait` read. This is the
  same guarantee `sync.WaitGroup` was designed for. (Verified
  against the language spec memory model: `sync.WaitGroup.Done` →
  `Wait` is an explicit synchronisation point.)
- No channel is opened, closed, ranged-over, or selected-on for
  collection. Zero surface for the close-race class of bug.

A naive alternative (`results := make(chan Outcome, len(models))`
+ N senders + one receiver doing `for i := 0; i < N; i++ {
results <- … }`) WOULD also work and is race-free if implemented
strictly — but it adds a second collection device alongside the
WaitGroup that we need anyway (for cooperative-cancellation
correctness, see §2). Two synchronisation primitives where one
will do is a smell, especially when the channel adds the
"remember to read exactly N times" rule that's easy to break on
later refactor (e.g. someone adds an early-exit path and the
sender writes to a buffer no one is reading; the buffer
backpressure silently changes the timing). The slice is simpler
and more rigid.

The tests under `-race` will be the proof. The package's coverage
target includes a `TestRun_concurrent_no_races` that fan-outs over
N goroutines and inspects each `Outcome` post-`Wait` — under
`go test -race -count=10`, the goroutine partition above produces
zero failures.

#### Why deterministic order (input position → outcome position)

- A policy layer wants to know "what did the *first* provider I
  passed say". The first provider has a privileged role in some
  policies (e.g. "use Ollama for personal data; cloud only as
  fallback"). Position-stable outcomes make that lookup an array
  index, not a name match.
- Tests benefit too: assertions are `outcomes[0].Err == nil &&
  outcomes[1].Err == something` instead of "find the outcome whose
  provider == 'ollama'", which couples test code to provider
  strings.
- The alternative (chronological order — fastest provider first)
  would lose this and only buy back a property no caller has asked
  for. Chronological ordering is policy-relevant in racing
  strategies; mechanism does not need it.

### 5. No Registry in 4.3. `Run` takes `[]model.Model` directly.

ADR-0009 §5 deferred the Registry "to 4.3 fan-out, sized to its
consumers". Examined here: **4.3 has zero consumers that need
name-based lookup.**

- The mechanism takes a slice of `Model` values. The caller already
  holds those values (the live skeleton constructs them; Stages 5–6
  will construct them from configuration).
- "Look up a model by name" is a need that arrives with the
  configuration layer (operator says "use the model named `ollama-
  3.2`"). That layer is Stage 5+; it is the natural home for the
  Registry because it is what reads operator config in the first
  place.
- Building a Registry now would invent a name → Model mapping with
  no consumer to constrain its shape (eager vs lazy construction,
  concurrent registration, identity vs interchangeability for
  same-name re-registrations, …). All of those are speculative
  without a real driver.

The Registry therefore defers ONE MORE TIME, until the configuration /
bootstrap stage. ADR-0009 §"Open follow-ups" already named it as a
follow-up; this ADR just re-affirms.

### 6. Package location: `internal/model/fanout/`. The coordinator does NOT implement `model.Model`.

The package layout becomes:

```
internal/
  model/
    doc.go
    model.go
    errors.go
    model_test.go
    ollama/
      doc.go
      ollama.go
      ollama_test.go
    groq/
      doc.go
      groq.go
      groq_test.go
    fanout/                # new in 4.3
      doc.go
      fanout.go            # Coordinator, Option, Run
      errors.go            # ErrNoModels, ErrNilModel
      fanout_test.go       # exhaustive table-driven + -race
```

Reasons:

- Mirrors the precedent set by ADR-0009 §5 and applied by ADR-0010.
  The `internal/model/` namespace is the home of every provider /
  coordinator that speaks `model.Request` → `model.Response`. A
  fan-out coordinator is the same shape; it belongs in the family.
- A top-level `internal/fanout/` would suggest generality (fan-out
  for *anything*). Korvun has no such generality need; the only
  thing fan-out is fanning out over is models.
- A sub-package keeps `internal/model` itself small (the interface
  + types, plus the validation seam). Cohesion stays high.

#### Why `Coordinator` does NOT satisfy `model.Model`

`model.Model.Generate` returns a single `*Response`. A fan-out
coordinator wrapped in `Model` would have to *choose* one
`*Response` out of N outcomes. That choice is policy. The fan-out
mechanism does not own it.

If a future caller wants a "fan-out that looks like a single Model"
— e.g. a Brain that does not yet know about multi-provider — the
right shape is to compose a fan-out with a policy in a separate
type, e.g. `policy.FirstOK(fanout, []model.Model{...})`, which itself
satisfies `Model`. That composition lives wherever the policy
lives (Stage 5+ or wherever the no-code engine plumbs it).
Predating that need with a "default policy baked into fan-out"
would either bake in a choice that's wrong for most callers, or
require a flag that's the configurable-strategy anti-pattern §1
already rejected.

### 7. Stdlib only (`sync`, `context`, `time`). No new dependency.

Decision restated for emphasis. `go.mod` does not grow. The
single-direct-dependency line (`go-telegram/bot`) that's held since
Stage 2 holds through the end of Stage 4. Two model adapters and a
fan-out coordinator have all been built without adding a Go module.

Why the equilibrium matters:

- It validates the four-axis dependency calculus ADR-0009 §4 and
  ADR-0010 §2 used: dep size vs hand-roll cost vs API volatility vs
  test-surface gain. Three independent applications and zero
  modules added.
- The next addition (a real one — a SQLite driver, a Prometheus
  client, an OpenTelemetry SDK) will pass the same test or it
  won't ship.

## The 4.3 plan — what implementation will look like after this ADR

Three commit-shaped pieces of work, on a dedicated feature branch
`feat/4.3-fanout` (this phase is structural concurrency; ADR per
master, code per branch, per the project workflow):

1. **`internal/model/fanout` mechanism (red→green).** Sentinels
   (`ErrNoModels`, `ErrNilModel`) and tests for them. `Coordinator`
   type, `New(opts ...)`, `WithPerModelTimeout(d)`, `Run(ctx, req,
   models)`. Tests, table-driven, exhaustive, all under `-race`:
   - happy path with N=2 models (one ollama-shaped fake, one
     groq-shaped fake), both succeed, outcomes preserved in input
     order;
   - happy path with N=5, asserting determinism over 100 runs;
   - one model fails with each sentinel category
     (`ErrAuthInvalid`, `*RateLimitError` with non-zero RetryAfter,
     `ErrProviderUnavailable`, `ErrProviderResponse`); the others
     succeed; per-outcome attribution and sentinel preservation
     verified via `errors.Is` / `errors.As`;
   - all models fail with mixed sentinels — `Result` still returned,
     no `Run`-level error;
   - validation failure on `req` rejects before any goroutine
     spawns (assert via a fake that panics on call — if Generate
     is called, the test fails);
   - empty `[]models` → `ErrNoModels`, no goroutine spawn;
   - any nil entry → `ErrNilModel`;
   - nil ctx → mechanism error;
   - ctx already cancelled at entry → ctx.Err() returned, no
     goroutine spawn;
   - ctx cancelled mid-flight → every in-flight `Generate` sees
     cancellation; outcomes have ctx-derived errors per adapter;
     `Run` returns the slice with errors; **no goroutine leak**
     (asserted by goroutine-count comparison before/after Run
     plus a small grace window);
   - per-model timeout fires for one slow model, others finish OK;
     slow model's Outcome has the adapter's ctx-deadline error;
   - per-model timeout shorter than total ctx — slow model fires
     first; total ctx longer — fast models still complete;
   - per-model timeout longer than total ctx — total ctx wins
     (verifying the parent-chain semantics);
   - latency capture: each Outcome.Latency > 0 and bounded above
     by the slowest of (per-model timeout, total ctx);
   - concurrent `Run` invocations on the same Coordinator do not
     interfere (a Coordinator instance is safe for reuse);
   - **panic isolation:** a fake adapter that panics from `Generate`
     produces an Outcome with `Err` containing `"fanout: provider
     panicked:"` and the original recover value's String; the other
     Outcomes are unaffected; the test process does NOT die; no
     goroutine leaks; the slot's `Provider` is `m.Name()` (set
     before the call) when Generate panics;
   - **panic from `Name()`:** a fake adapter whose `Name()` panics
     (rarer but possible) lands its Outcome with `Err` containing
     the panic prefix; `Provider` may be empty (Name() never
     returned); the rest of the fan-out is unaffected;
   - **zero-value Coordinator:** `var c fanout.Coordinator;
     c.Run(...)` works — `c.now` defaults to `time.Now` on the first
     Run rather than panicking; subsequent Runs on the same value
     keep the defaulted clock;
   - **`WithPerModelTimeout(0)` and `WithPerModelTimeout(-1*time.Second)`
     are no-ops:** the Coordinator's `perModelTimeout` field stays at
     its previous value (zero for a freshly-`New`-ed Coordinator),
     and `Run` does not wrap child ctxs in `WithTimeout` for them;
   - **test seam for `now`:** at least one test asserts that
     assigning `c.now = fakeClock` from a same-package test makes
     `Outcome.Latency` deterministic;
   - `-race -count=10` clean on every table case.

2. **Live skeleton `cmd/demo-fanout/main.go` (build-only commit).**
   Reads both `OLLAMA_HOST` and `GROQ_API_KEY` from the environment;
   refuses to start if either is missing (clear messages for each).
   Constructs both adapters, hands them to `fanout.New().Run(ctx,
   req, []model.Model{ollamaAdapter, groqAdapter})`. Prints, per
   Outcome:
   `provider=… latency=… ok|err: <content snippet | error>`.
   Same temporary-binary disposition as the previous two demos —
   it gets deleted (or rewritten as an integration test) when the
   real `cmd/korvun` boots in Stage 5+.

3. **`docs/stages/STAGE-04.md` — Stage 4 closure doc.** Covers 4.1,
   4.2, 4.3. Reads like STAGE-02-EXT.md: deliverables, behaviour
   established, what was NOT shipped, workflow compliance table,
   quality-gate coverage table. Closes Stage 4.

`make quality` green over the WHOLE tree, with `-race`, before
closing each step. Coverage target: ≥90% for
`internal/model/fanout` (the pure-Go shape makes this trivial; the
table-driven tests above already cover almost every line).

## Consequences

### What this enables

- Phase 5+ (policy engine) lands with a stable mechanism to drive.
  Every policy shape Stages 5–6 want — first-OK, majority, consensus,
  cost-bounded, privacy-aware — is "read this `Result`, decide".
  The mechanism does not flex to accommodate them; they all consume
  the same shape.
- `cmd/demo-fanout` becomes the third operator self-check: it
  proves both providers work *together* in one process, with both
  env shapes satisfied, in one round-trip. When `cmd/korvun`
  proper lands in Stage 5+, all three temporary demos delete in
  the same commit.
- The sentinel grammar from ADR-0010 §1 earns its keep at the
  policy layer: outcomes carry their original error categories
  untouched, so the policy code that branches on
  `errors.Is(err, ErrRateLimited)` is identical whether the call
  came from a single-provider Brain or a multi-provider fan-out.
- The "minimal supply chain" principle has now survived three
  consecutive provider-related ADRs. `go.mod`'s single direct
  dependency line stands as documented project posture.

### What this asks of every future adapter

- Honour ctx propagation all the way through. The fan-out's
  cooperative-cancellation invariant assumes it. Adapters that
  ignore ctx hang the fan-out; the failure is loud (a hung
  goroutine in `Run`) and points at the adapter, but a CI-level
  goroutine-leak detector at the fan-out test layer cannot save a
  rogue adapter — the test is at the *adapter* layer.
- Preserve the sentinel grammar. An adapter that wraps a 429
  response into a generic `errors.New(...)` instead of
  `*RateLimitError` makes the policy layer's life harder for
  nothing. Both shipping adapters do the right thing; future
  adapters cite this as the contract.
- Be safe for concurrent calls. `Model.Generate` may be invoked
  from multiple goroutines simultaneously (fan-out always calls it
  in parallel; the policy layer may parallelise further). The
  ollama and groq adapters are stateless (the http.Client is
  the only shared state, and that's safe by design); future
  adapters should match.

### What this does NOT do

- **No policy.** Re-stated: nothing in this ADR chooses among
  outcomes. Stages 5–6 own that.
- **No retry.** A `*RateLimitError{RetryAfter: 30s}` Outcome is
  data, not an instruction. The policy decides whether to wait and
  re-Run.
- **No streaming.** When `StreamingModel` lands (ADR-0009 §2), a
  sibling `(*Coordinator).RunStream(...)` is the natural shape
  (chunks per provider, with provider attribution per chunk). 4.3
  ships only `Run`.
- **No Registry.** Deferred again, now to whatever stage owns
  configuration / bootstrap.
- **No top-level metrics.** Per-Outcome `Latency` is captured;
  surfacing it as Prometheus histograms is Stage 12 observability
  work. The mechanism captures the datum; it does not export it.
- **No `Coordinator`-as-`Model`.** §6 above: composition with a
  policy is the right shape when a caller needs "fan-out that
  looks like a Model".
- **No cross-Outcome correlation.** "Outcome A semantically agrees
  with Outcome B" is policy. The mechanism does not check or hint.
- **No work-stealing, no priority, no fail-over chaining.** Every
  model is called, every model is awaited. Fail-over is policy
  composed of two `Run` calls plus a decision between them.

### Trade-offs accepted

- **Wall-clock = slowest model's latency.** A fast model that
  finishes in 200 ms still waits for a slow model that takes 8 s
  (unless `WithPerModelTimeout` cuts the slow one). This is the
  correct mechanism cost; policy that needs sooner answers uses
  the per-model timeout knob, or composes a fail-over.
- **No `Run`-level partial-failure error.** Callers MUST inspect
  `Outcomes[i].Err` per slot. Acknowledged: this is a one-line
  loop on the policy side and removes the need to handle a partial-
  success "error" that is not, in fact, an error.
- **Goroutine cost is N per Run.** A fan-out over 5 providers
  spawns 5 goroutines per call. Goroutines are cheap; the network
  call dominates wall-clock by orders of magnitude; this is not a
  cost worth optimising in the mechanism.
- **Cooperative cancellation only.** A buggy adapter can hang `Run`
  past its caller's ctx deadline (because `Wait()` waits for the
  buggy goroutine that ignores ctx). This is by design — the
  alternative is goroutine leaks. Adapter test suites are the
  enforcement point. §2 above documents this prominently so a
  future adapter author has been warned.
- **Pre-allocated slice over collection channel.** Acknowledged:
  this looks "less Go-idiomatic" at first glance because the
  mainstream patterns use channels for fan-in. The trade-off is in
  favour of removing the close-race class of bug entirely; the
  WaitGroup gives the synchronisation, the slot partition gives
  data isolation, the memory model guarantees visibility. This is
  a stdlib `sync.WaitGroup` pattern, just not the channel-based
  one.
- **Two new sentinels (`ErrNoModels`, `ErrNilModel`) in the fanout
  package.** Mechanism-level, used once each at the start of `Run`.
  Adds two error identities to the surface; without them, callers
  have to either string-match or accept a generic `error`. Both
  are caller-side configuration bugs, deserve identities.

## Alternatives Considered

### Collection — C1: configurable strategy (`wait-all` / `first` / `majority` / `quorum`)

`fanout.New(WithStrategy(StrategyFirstOK))`, `WithStrategy(StrategyMajority)`,
etc.

**Rejected** per §1. Either commits the mechanism to policy
semantics this ADR has no authority to pin (Stages 5–6 own that),
or expands the API surface that Stages 5–6 will re-wrap anyway. The
strict "one shape" discipline keeps mechanism and policy on
opposite sides of the boundary.

### Collection — C2: `RunFirst(ctx, req, models)` returning the first OK and cancelling the rest

A second method that races and cancels.

**Rejected.** "First OK" hides three policy decisions: (a) "what is
OK" — first 2xx, first non-rate-limit, first with quality above a
floor — is policy; (b) "what about ties" — two return in the same
ms, who wins — is policy; (c) "do we want the others if the first
is bad" — racing with no slow-tail readback is wasted
parallelism. Stages 5–6 wraps `Run` with their own first-OK rule
when they have one.

### Collection — C3: stream `Outcome` over a channel as they arrive

`Run` returns `<-chan Outcome` plus a sentinel close.

**Rejected.** Reintroduces the channel-close race surface §4 was
careful to remove. Also commits the mechanism to "results streamed
in order of completion", which a policy that wants "everything,
then decide" has to re-buffer. Streaming is the right answer for
`RunStream` (chunks per provider, ADR-0009 §2 future work); it is
not the right answer for non-streaming `Generate` outcomes.

### Collection — C4: callback-streaming on top of wait-all (`Run(ctx, req, models, fn func(Outcome))`)

A variation of C3 that keeps wait-all semantics (the goroutine
parent still calls `WaitGroup.Wait()` and still returns the full
`*Result` at the end) but **also** invokes a caller-supplied `fn`
once per Outcome as it lands. Caller use case: policy that wants
to start downstream work (log lines, metrics streaming, partial
UI hint to the user, etc.) as soon as the first provider answers,
without giving up the post-Run guarantee that every Outcome is in
the returned slice.

**Rejected.** Streaming to the consumer *is* a policy concern even
when wait-all is preserved underneath:

- "Start work as soon as something arrives" is a latency-vs-
  completeness trade-off the policy makes, not the mechanism. A
  policy that wants this composes it: wrap the mechanism's `Run` in
  a goroutine, fire the wrapper, then read partial state from a
  shared structure the policy itself maintains. The mechanism does
  not need to know.
- The `fn` callback adds a second synchronisation contract (when
  exactly is `fn` invoked? in which goroutine? what if it blocks?
  what if it panics — does the goroutine recover catch that too,
  and if so does that contaminate the `Outcome` slot?). Each of
  those is a new edge case the mechanism would have to specify and
  test. The wait-all-then-return shape has none of them.
- Streaming-as-a-policy-feature lives more cleanly at the layer
  that *is* the policy. Stages 5–6 own that layer; the mechanism
  ships without prescribing how policy notifies its consumers.
- Acknowledged: this option is a strictly larger surface than the
  one shipped — it does NOT compromise the wait-all guarantee. The
  rejection is on mechanism-vs-policy grounds, not race-safety
  ones (C4 is race-safe; C3 is the race-unsafe variant). The line
  the ADR holds is "the mechanism returns ALL outcomes in one
  call; signalling them in arrival order is policy's job."

Re-evaluable if a policy in Stages 5–6 turns out to need a hot path
for partial-result handling that wrapping `Run` in a goroutine
cannot serve cleanly. The additive shape (a second method,
`RunNotifying`, sibling to `Run`) is available then.

### Concurrency — N1: `golang.org/x/sync/errgroup`

Use `errgroup.WithContext(ctx)` + `g.Go(...)`.

**Rejected.** Verified semantics: the first error in any
goroutine cancels the derived ctx and aborts the rest (Context7
`/websites/pkg_go_dev_golang_org_x_sync`). That is the *inverse*
of what fan-out wants — one provider's failure must not abort the
others. errgroup is the right tool for "all of these must succeed
or the whole task is dead"; fan-out is "show me every result
regardless of which ones failed". stdlib `sync.WaitGroup` +
`context.WithCancel` is one screen of code with the right
semantics.

### Concurrency — N2: hand-rolled `chan Outcome` + counter

`results := make(chan Outcome, N)`; goroutines send; receiver loops
`for i := 0; i < N; i++ { <-results }`.

**Rejected.** Adds a second synchronisation primitive (channel)
alongside the WaitGroup that's already there for the
cooperative-cancellation invariant (§2). The "remember to read
exactly N times" rule is a hidden invariant that the §4 indexed-
slice approach removes. Also makes the "preserve input order"
property §4 needs harder — receiver has to assemble outputs by
provider name or input position, which means decorating Outcomes
with their index, which is the indexed slice with extra steps.

### Concurrency — N3: a worker pool with bounded fan-out width

`Coordinator` carries a max-in-flight cap; if the caller passes
N > cap, the rest queue.

**Rejected.** Models in a fan-out are by definition a small N
(2–5 in any plausible policy). A worker-pool layer adds machinery
for a problem nobody has. If a future use case fans out over 100
models, that's the moment to add `WithMaxInFlight(n)` — additive,
no break to today's API.

### Failure shape — F1: `Run` returns `(*Result, error)` where `error` is non-nil if ANY model failed

The "anything went wrong" surface.

**Rejected.** Forces every caller to inspect both `err` and
`Result.Outcomes`, which is strictly worse than inspecting only
`Outcomes` (and reading per-Outcome errors). Also makes
`errors.Is` against the right partial-failure category ambiguous:
joined errors mix categories with different retry semantics.
Better to leave per-model errors in their own slots.

### Failure shape — F2: introduce an `ErrPartialFailure` sentinel

`Run` returns `ErrPartialFailure` plus the populated `Result` when
some outcomes failed.

**Rejected.** The sentinel adds no information the `Result`
doesn't already carry, costs a pattern every caller has to handle,
and creates a third category of "is this an error" alongside
`Run`-mechanism errors and per-Outcome errors. The shape
`(*Result, nil)` regardless of per-Outcome failures is sharper.

### Registry — R1: add Registry as part of 4.3

Build the name→Model registry now; `Run` accepts a Registry +
names instead of `[]Model`.

**Rejected.** The fan-out has no naming need that the caller's
own `[]Model` doesn't already cover. A Registry's hard questions
(eager vs lazy construction, identity vs interchangeability) need
the consumer (configuration / bootstrap) to constrain them. Two
phases of YAGNI deferrals do not get a Registry; the right driver
hasn't arrived.

### Registry — R2: `Run` accepts a `map[string]Model` (names included, no Registry type)

Half-step toward naming: still no separate Registry, but the
input gains string keys so attribution is more explicit.

**Rejected.** Provider name is already on `model.Response.Provider`
and `Model.Name()`. Forcing the caller to supply a parallel name
mapping is redundant for the well-behaved adapter and brittle when
two distinct Model instances happen to share a name (e.g. two
Groq adapters pointing at different free-tier API keys). Slot
identity by input position is sharper.

### Package — P1: `internal/fanout/` (top-level)

Pull it up out of `internal/model/` so future fan-out-shapes
(e.g. fan-out across channels, fan-out across brains) can land
next to it.

**Rejected.** Speculative generality. The only thing Korvun fans
out today is models; the next plausible fan-out target (router,
brain) has very different cancellation semantics. Coloring the
package by use case (`internal/model/fanout`) keeps it discoverable
and prevents the "what does this fanout do" reading at every
import site.

### Package — P2: define `MultiModel` that implements `model.Model`

The fan-out IS a `Model`; it picks a default winner internally.

**Rejected** per §6. The "default winner" is policy. Burying it
inside the mechanism hides it from the layer that should own it
(Stages 5–6) and bakes a default that's almost certainly wrong
for some caller.

## Open follow-ups (not blockers for Phase 4.3)

- **Registry, once configuration / bootstrap exists.** Stage 5+
  owns this when it owns the config layer.
- **`(*Coordinator).RunStream(...)`** when `StreamingModel` ships.
  Streams `Outcome`-like chunks; preserves per-provider
  attribution per chunk; same cooperative-cancellation invariant.
- **Per-Outcome metrics.** `Latency` is captured; surfacing as
  Prometheus histograms is Stage 12.
- **`WithMaxInFlight(n)`** if a real consumer ever fans out over
  enough models for goroutine count to matter (likely never).
- **Policy-layer wrappers** — `firstok.Coordinator`,
  `majority.Coordinator`, `consensus.Coordinator`, etc., each
  implementing `model.Model` over a `fanout.Coordinator`. Stages
  5–6 produce these; the mechanism is fixed.
- **CI smoke-test conversion of `cmd/demo-fanout`.** Same
  consideration as the previous two demos — requires a CI strategy
  for both `OLLAMA_HOST` and `GROQ_API_KEY`. Deferred until
  Korvun has a CI worth wiring through.
- **Goroutine-leak watchdog as a test helper.** The 4.3 tests
  inline a goroutine-count delta check; if more fan-out tests
  arrive, lifting it into a shared `testhelper` is the right
  refactor. Out of scope until that critical mass exists.
