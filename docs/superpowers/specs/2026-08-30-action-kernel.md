# Trust Layer Etapa 1 — Action Kernel: Design Spec

> **Status: VERIFIED — lotes 1-3 implemented, whole gates green, and the
> stage closed by the director's mini-bash over the packaged build
> (2026-08-30): identical chat plus a real customs receipt on his own
> profile. Shipped as v0.11.0.**
> Original approval: approved for TDD — **APROBADO POR CHANO 2026-08-30**, with
> the three `[NEEDS CLARIFICATION]` forks resolved by his call:
> **(1)** SAME SQLite file as conversations, with the action tables under
> their OWN migration versioning and lifecycle;
> **(2)** retention with a generous automatic row cap and NO new config
> surface — the bounded-growth invariant rules;
> **(3)** Etapa 1 is an INVISIBLE motor — zero UI; the mini Activity row
> arrives later with its mockup and his yes (Sixth Law).
> Governing frame: `design-drafts/trust-layer-master-plan.md` (PLAN MAESTRO
> APROBADO POR CHANO 2026-08-29) and the blueprint
> `docs/blueprints/2026-08-15-execution-trust-layer.md` (§9, §10.5, §12,
> §22.1, §24, §26). Governing ADRs: ADR-0021 (tool seam), ADR-0041
> (governed tools, two-point gate), ADR-0042 (native lane), ADR-0024
> (metadata-only surfaces).
> External-docs note: this spec uses ONLY the standard library
> (`crypto/sha256`, `encoding/json`, `time`) plus existing `internal/`
> packages and the already-adopted `modernc.org/sqlite` (in `go.mod` since
> the conversation store) — no new external dependency, so no Context7
> verification is required. If any lote later needs a new library, it stops
> and goes through Context7 + ADR first, per house law.

## Goal

After this stage, every tool call an AgentBrain makes — on BOTH lanes,
textual and native — is reified as a canonical **ActionEnvelope v1** with a
deterministic digest, receives a persisted **decision** BEFORE anything
executes, walks an explicit **state machine**, and reaches `Tool.Execute`
through exactly ONE component (the **Executor Registry**). The outside
experience does not change at all: same chat, same observations, same audit
events, same denials, same shadow behavior — that sameness is itself an
acceptance criterion. Identity, intents, authority, approvals,
transactions, credential brokering, MCP/A2A and every visible surface stay
OUT (stages 2+; any future visible surface takes the Sixth Law road:
mockup + Chano's yes).

## Anchors in the real tree (verified 2026-08-30)

- `internal/brain/agent.go:633` — `runTool` is TODAY's single execution
  seam: unknown-tool denial, two-point gate (shadow simulation / denial
  observations), per-tool timeout, cage reclassification, `auditTool`
  (metrics + bus, metadata only).
- `internal/brain/agent_native.go:127,138` — the native lane calls THE SAME
  `runTool` (args serialized by `nativeArgs`), so one insertion point
  covers both lanes.
- `internal/brain/agent.go:481` — the B13 channel-exit guard
  (`toolCallShape`) sits AFTER the loop, at the lanes' convergence; the
  kernel must leave it untouched.
- `internal/policy/tools.go:121` — `SelectTools` is the pure per-Handle
  policy; `effectiveTools` returns nil decisions when governance is off
  (today's behavior byte-for-byte).
- `internal/tool/tool.go` — `Tool.Execute(ctx, args string)`; args is a RAW
  STRING parsed by each tool; `Registry` is read-only after construction.
- `internal/conversation/sqlite/` — the house SQLite mold: bootstrap
  `CREATE TABLE IF NOT EXISTS`, transactional migrations, boot-fatal on
  failure, `modernc.org/sqlite`.

## Functional requirements

### FR-DOM — the action domain package

- **FR-DOM-1** New leaf package `internal/action` (see Decisions folded in
  for the naming call): ActionEnvelope v1, canonicalization, digest, and
  the state machine. Leaf discipline like `internal/tool`: stdlib only, no
  imports of brain/model/envelope — the brain adapts TO it.
- **FR-DOM-2** ActionEnvelope v1 is the blueprint §10.5 SUBSET for this
  stage:

  ```text
  schema_version      = 1
  action_id           (generated, "act_" + random)
  correlation_id      (the inbound envelope.Envelope.ID)
  source              { kind: "agent_brain", protocol: "text" | "native", channel }
  operation           { namespace: "tool", name, version: 1 }
  parameters_digest   ("sha256:<hex>", over the canonical args)
  effect              { class: "unclassified" }   // placeholder until Etapa 3
  requested_at        (UTC, from the brain's injected clock)
  ```

  The §10.5 fields NOT in v1 are documented in the package as RESERVED with
  their arrival stage: `intent_id`, `principal`, `authority_refs` (E2),
  `resource`, full `effect` (E3), `transaction_id`, `idempotency_key`,
  `expires_at` (E6), `tenant_id` (E10), `protected_parameters_ref` (E4).
  The raw tool name is stored bounded (the `boundedArgs` discipline); the
  envelope carries NO secret and NO free prompt text.
- **FR-DOM-3** Canonicalization is deterministic and stdlib-only: if the
  raw args parse as JSON, decode with `json.Decoder.UseNumber()` (numeric
  literals survive verbatim — no float64 mangling) and re-marshal —
  `encoding/json` emits object keys SORTED at every nesting level, giving a
  stable byte form; duplicate keys resolve last-wins and this is
  documented. If the args are not JSON, the canonical form is the raw
  string bytes as-is. The digest is SHA-256 over
  `operation.namespace | operation.name | operation.version | canonical-args`
  with an unambiguous length-prefixed concatenation, rendered
  `"sha256:<hex>"` — the algorithm identifier is pinned in the string so a
  future algorithm can coexist (blueprint §10.5 shape). RFC 8785 (JCS) via
  an external lib was considered and deferred: no new dependency for E1;
  revisit at E4 when receipts need cross-implementation verification
  (noted as a reserved decision, with ADR if adopted).
- **FR-DOM-4** State machine, blueprint §12 SUBSET:

  ```text
  RECEIVED -> NORMALIZED -> DENIED               (terminal)
                         -> SHADOWED             (terminal; NEVER executes)
                         -> AUTHORIZED -> SUCCEEDED | FAILED   (terminal)
  ```

  Invalid transitions return a sentinel error (property-tested).
  `PENDING_APPROVAL`, `PREPARING`, `OUTCOME_UNKNOWN`, `COMPENSATING` and
  the rest of §12 are RESERVED enum values documented for later stages —
  not reachable in E1.

### FR-EXEC — the Executor Registry

- **FR-EXEC-1** New `action.Executor` wraps `tool.Registry` and becomes the
  ONLY caller of `Tool.Execute` / `ScopedTool.ExecuteScoped` in the
  codebase. It preserves runTool's execution semantics exactly: per-tool
  timeout, Scope pass-through, cage reclassification hooks, latency
  measurement.
- **FR-EXEC-2** A tripwire test (AST/grep over the tree, the house pattern)
  asserts no call site of `Execute(`/`ExecuteScoped(` on the tool seam
  exists outside `internal/action` — the single-path invariant is enforced
  by CI, not by convention.

### FR-ADAPT — runTool becomes the adapter

- **FR-ADAPT-1** `runTool` keeps its signature and its OBSERVATION
  contract, but its body becomes: build envelope (RECEIVED→NORMALIZED) →
  wrap the EXISTING decision (the `decisions` map from `SelectTools`, nil
  meaning ungoverned-allow exactly as today) into a persisted action
  decision → transition (DENIED / SHADOWED / AUTHORIZED) → on AUTHORIZED,
  execute through the Executor → SUCCEEDED/FAILED. `SelectTools`, the
  two-point gate, the observation texts, the audit events, bounded-args
  logging and the unknown-tool grammar stay byte-identical on the outside.
- **FR-ADAPT-2** Every attempt persists — allowed, denied, shadowed AND
  unknown-tool (blueprint: "cada intento produce una decisión explicable y
  un registro"). Unknown-tool actions store the finite operation name
  `"unknown"` on shared surfaces and the bounded raw name in the row (DB
  rows are not metric labels; cardinality law applies to metrics/bus only).
- **FR-ADAPT-3** The six existing tools (time, echo, calc, read_file,
  http_fetch, webhook_call — plus memory_note where wired) run through the
  kernel with ZERO changes to their own code: the adapter covers them at
  the seam; no per-tool adaptation files are needed in E1 (their
  Describe/Effect declarations arrive in E3).

### FR-STORE — minimal persistence

- **FR-STORE-1** New store `internal/action/sqlite`: tables `actions` and
  `action_decisions`, keyed by `action_id`, indexed by `correlation_id` and
  `requested_at`. Same house mold as the conversation store (WAL, pragmas,
  transactional migration, boot-fatal on failure) but with its OWN
  migration versioning and lifecycle, SEPARATE from the conversation
  tables (§22.1) — the conversation store's migrations never learn about
  actions and vice versa.
- **FR-STORE-2** Terminal states are durable: a write of a terminal
  transition commits before the observation returns to the model loop.
  After a crash+restart, no action row rests in a non-terminal state
  without being marked (recovery pass on Open: non-terminal rows from a
  previous run are closed as `FAILED` with a `crash_recovered` marker —
  honest, visible, never silently re-executed).
- **FR-STORE-3** Resource bound (house invariant — no unbounded growth): a
  retention default prunes action rows beyond a configured cap at Open and
  periodically; see `[NEEDS CLARIFICATION]` 2 for the fork on the default.

### FR-COMPAT — the outside does not move

- **FR-COMPAT-1** The 16 points of blueprint §24 are the no-regression
  checklist of this stage, run as the existing whole-suite gates plus the
  targeted assertions named in AS-7.
- **FR-COMPAT-2** The B13 guard (`toolCallShape` at the channel exit)
  remains in force on the new path — pinned by re-running its existing
  tests unchanged (the approved-red law: those tests are contracts and are
  not edited).
- **FR-COMPAT-3** Hot-path overhead is measured and bounded: a committed
  Go benchmark compares `runTool` on a pure builtin tool BEFORE (baseline
  captured at the RED phase) and AFTER; the kernel's added cost
  (canonicalize + digest + state walk + persistence) must stay ≤ 5 ms p95
  on the reference machine, and the numbers are recorded in the stage
  close.

### FR-DOM addendum — native fuzzing (Chano's mandate, 2026-08-30)

- **FR-DOM-5** (adenda sellada — MORE rigor over the approved scope, no
  scope change): the kernel's parsers are born fuzzed, with Go's native
  fuzzing and zero new dependencies. `FuzzCanonicalize` asserts that
  arbitrary input NEVER panics and that canonicalization is deterministic
  (two runs, same bytes) and idempotent; `FuzzDigest` asserts digest
  stability (same tuple, same digest) and that shifting bytes across the
  length-prefixed field boundaries never collides. Seed corpus in the
  tree (the spec's edge list: enormous numeric literals, deep nesting,
  duplicate keys, unicode, non-JSON, empty). A SHORT fuzz smoke (seconds)
  joins `make quality` as a gate; long campaigns are a documented
  manual/nightly task — the general pre-1.0 fuzzing campaign over the
  hostile-input borders stays where the security plan puts it.

## Acceptance scenarios (Given / When / Then)

- **AS-1 (reify everything)** Given any governed or ungoverned brain, When
  the model calls any of the six tools on either lane, Then an action row
  and a decision row exist BEFORE `Tool.Execute` runs, with
  `parameters_digest` set — asserted by a fake store recording order.
- **AS-2 (lane equivalence)** Given the same logical tool call issued via
  the textual protocol and via the native lane, When both are normalized,
  Then the two envelopes carry the same `operation` and the SAME
  `parameters_digest` (only `source.protocol` differs).
- **AS-3 (digest stability)** Given args that are semantically identical
  JSON with different key order, whitespace, or re-serialization, When
  canonicalized, Then the digest is identical; and property tests assert:
  changing any parameter value changes the digest; non-JSON args digest by
  raw bytes; numeric literals survive verbatim.
- **AS-4 (shadow never executes)** Given a tool granted `shadow`, When the
  model calls it, Then the action terminates SHADOWED, `Tool.Execute` is
  NEVER invoked (fake tool asserts zero calls), and the model receives the
  same simulation observation text as today.
- **AS-5 (unknown fails closed)** Given a hallucinated tool name, When
  called, Then the action terminates DENIED with the `unknown_tool` rule,
  the shared audit surfaces carry the finite `"unknown"` name, and the
  observation text is byte-identical to today's.
- **AS-6 (crash recovery)** Given an action persisted AUTHORIZED whose
  process dies before the terminal write, When the store reopens, Then the
  row is closed FAILED with the `crash_recovered` marker and nothing
  re-executes.
- **AS-7 (the outside does not move)** Given the pre-stage suite, When the
  whole gates run (channels, brains, sessions, desktop, governed tools,
  reload, B13 tests untouched), Then everything passes with zero test
  edits to existing contracts; and the demo-estrella baseline (today's
  chat + tools flow) behaves identically end to end.
- **AS-8 (single path)** Given the full tree, When the tripwire runs, Then
  the only `Execute` call sites on the tool seam live in
  `internal/action` — and the tripwire itself fails RED before the
  Executor exists.

## Success criteria

- Coverage: ≥ 90 % for `internal/action` (it joins the
  policy/router/envelope/brain tier — it IS authorization machinery);
  ≥ 85 % for `internal/action/sqlite`.
- `make quality` green with `-race` over the WHOLE suite; govulncheck
  clean before the rehearsal, as always.
- The 16-point §24 checklist green via the existing suites; B13 tests
  untouched and green; `go.mod` untouched (no new dependency).
- The overhead benchmark committed with before/after numbers, within the
  declared ≤ 5 ms p95 bound.
- Chano sees (per the master plan, Etapa 1): the app behaving EXACTLY as
  before, plus the test-demo that prints a real call's envelope + decision
  ("esta acción exacta, este digest, esta decisión") — no new UI in this
  stage (the mini Activity row of the master plan §7 moves to a later
  lote WITH its Sixth-Law mockup, see NC-3).

## Decisions folded in

1. **Package name `internal/action`** — house style is short nouns
   (`tool`, `policy`, `envelope`); alternatives considered:
   `internal/kernel` (grandiose, ambiguous with OS kernels),
   `internal/actionkernel` (long, redundant inside `internal/`). The
   doc.go names it "the Action Kernel (Trust Layer Etapa 1)". This bounds
   the §30 naming decision for this stage.
2. **Stdlib canonicalization now, JCS reconsidered at E4** — no new
   dependency for a digest that today only needs INTERNAL determinism;
   cross-implementation canonicalization matters when receipts are
   verified by third parties (E4), and gets its ADR then.
3. **Digest input is length-prefixed concatenation** of operation triple +
   canonical args — unambiguous by construction (no delimiter collisions).
4. **Denied/shadowed/unknown attempts persist too** — the blueprint's
   "every attempt leaves an explainable decision" is taken literally from
   E1, so the E4 ledger inherits complete history semantics.
5. **Recovery closes as FAILED, never re-executes** — blueprint §16.4's
   honesty rule applied early: the kernel must not invent idempotency it
   does not have yet.

## `[NEEDS CLARIFICATION]`

1. **Storage placement:** (a) the SAME SQLite file as conversations, with
   separate tables + OWN migration versioning (fewer moving pieces, one
   backup story; my vote for E1 — the E4 ledger can still move out when
   hash-chaining and retention arrive), or (b) a second database file
   (`korvun-actions.db`) from day one (cleaner lifecycle, but a new
   storage path in config and in every backup/restore/runbook TODAY).
2. **Retention default for action rows (resource-bound invariant):**
   (a) prune beyond a row cap with a generous default (e.g. keep the last
   100k actions), config override later — my vote: growth is bounded from
   day one without new config surface; or (b) unbounded with documented
   operator guidance until E4 defines real retention (simpler, but leaves
   an unbounded-growth path open, against the house invariant).
3. **The mini "acción decidida" row in Actividad** (master plan §7,
   surface 1): (a) OUT of E1 entirely — E1 ships with zero UI and the row
   arrives with its mockup + Chano's yes as a small follow-up piece (my
   vote: keeps E1 pure motor and the Sixth Law clean), or (b) inside E1's
   last lote, gated on a mockup + his yes mid-stage.

## Troceo en lotes (tamaño honesto)

- **Lote 1 — el dominio (M):** `internal/action` puro: envelope v1,
  canonicalización + digest, máquina de estados. Property tests (digest y
  transiciones) + unitarios. Sin tocar el brain.
- **Lote 2 — la persistencia (M):** `internal/action/sqlite` con el molde
  de la casa, migración propia, recovery de terminales, retención (según
  NC-2). Crash-tests con fallo inyectado antes/después de cada escritura.
- **Lote 3 — el cuello adaptado (L):** Executor Registry + tripwire +
  `runTool` como adaptador + equivalencia de carriles + suite §24 entera +
  B13 intacta + benchmark antes/después. Es el lote delicado: toca el
  camino caliente y cierra el criterio de salida de la etapa.

Total honesto de la etapa: **L** (tres lotes, cada uno con su ciclo
RED→GREEN→quality completo y su revisión).
