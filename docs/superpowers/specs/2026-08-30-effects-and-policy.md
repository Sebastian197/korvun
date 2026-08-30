# Trust Layer Etapa 3 — Effects and per-action policy: Design Spec

> **Status:** APROBADO POR CHANO — sealed for TDD, 2026-08-30. **Batches
> 1-5 IMPLEMENTED + VERIFIED locally (2026-08-30): AS-1 through AS-8
> green** — the five blueprint-mandatory tests included (args force a new
> decision; reload never alters recorded decisions; read vs write under
> one pinned law; undeclared operations die at preflight AND at the gate;
> the current channel/sensitivity/locality rules untouched by the whole
> pre-existing suite). Classified hot-path toll ~0.9-1.7 ms/op (machine
> noise included) under the 5 ms ceiling. Stage acceptance pending
> Chano's review and the mini-bash mandate. The two
> forks resolved with the house votes: **NC-1** `memory_note` =
> `write_reversible` («la escalera mide consecuencia, no distancia de
> red: una nota que persiste ES una escritura»); **NC-2** = option (b) —
> `require_approval` LIVES under limited grants already and dies
> `approval_unavailable` until E5 (whoever bounds authority gets the
> full §10.6 treatment from today; the root stays byte-for-byte; the E5
> estreno turns those noes into approval cards). The four vocabulary
> rules ADJUDICATED AND APPROVED: `effect_undeclared`, `effect_ceiling`,
> `approval_unavailable`, `prepare_unavailable`.
> Governing frame: `design-drafts/trust-layer-master-plan.md` (sealed) and
> the blueprint `docs/blueprints/2026-08-15-execution-trust-layer.md`
> (§7.4, §7.5, §7.10, §9.7, §10.6, §10.7 subset, §13, Etapa 3, §23, §24).
> Governing ADRs: ADR-0041 (governed tools, two-point gate), ADR-0021
> (tool seam), ADR-0024 (metadata-only surfaces).
> External-docs note: ONLY stdlib + existing `internal/` packages +
> the already-adopted `modernc.org/sqlite`. No new dependency, no
> Context7 need.
> Seam audit performed on real code (2026-08-30): `action.Digest` covers
> Operation + canonical params ONLY — `Effect` does NOT participate
> (`action.go:127-138`), so waking the effect class cannot move a single
> receipt digest; `NewEnvelope` pins `Effect{Class: "unclassified"}`
> (`action.go:161`); the store persists `effect_class` as a row column;
> grants carry the E2-RESERVED `effect_ceiling` slot (reserved then
> because "an incomparable ceiling would be theater" — this stage makes
> it comparable); `SelectTools` rules in force: `deny`, `not_granted`,
> `channel`, `sensitive_locality`, `private_network_shield`, `cage`
> (`policy/tools.go`). Builtin catalog (7): `time`, `echo`, `calc`,
> `memory_note`, `read_file` (Sensitive), `http_fetch` (Network),
> `webhook_call` (Network).

## Goal

Korvun decides on the CONSEQUENCE of an action, not only on its name.
After this stage every operation carries a DECLARED, registry-verified
Effect Descriptor with a finite, totally-ordered class; the envelope's
`unclassified` placeholder wakes to the real class (receipts untouched);
every gate decision pins the exact policy version/digest that took it;
grants gain a comparable `effect_ceiling` and attenuation gains its tenth
dimension; and the decision vocabulary gains `require_approval` /
`require_prepare` — which, with no approval workflow until E5, fail
CLOSED with stable rules. The engine NEVER infers effects from model
text (§9.7); today's flows under the root intent do not change behavior
by one pixel — classification is recorded, the experience does not move.
Out of scope (deferred, declared below): preview/approvals (E5), signed
ledger (E4), transactions/idempotency (E6), broker (E7), Console
surfaces (E8), fine-grained data restrictions beyond today's cages.

## Functional requirements

### FR-DESC — Effect Descriptor and the total order (§10.6)

- **FR-DESC-1** `internal/action` gains `EffectClass` — the FINITE enum
  `pure`, `read_external`, `write_reversible`, `write_compensatable`,
  `write_irreversible`, `critical` — with a DECLARED total order
  (`Rank() int`: pure=0 … critical=5). The order is the law that makes a
  ceiling comparable; an unknown class ranks ABOVE critical (fail-closed:
  garbage is never cheaper than the worst known class).
- **FR-DESC-2** `EffectDescriptor` carries the §10.6 v1 subset:
  `Class EffectClass`, `ReadsExternalState`, `WritesExternalState`,
  `DataEgress`, `Reversible`, `Compensatable bool`. The remaining §10.6
  fields (`financial`, `credential_use`, `criticality`,
  `prepare_supported`, `status_query_supported`,
  `idempotency_supported`) stay RESERVED with written reasons (E5/E6/E7
  wake them) — the E2 discipline: an unreadable field is a field nobody
  can misuse.
- **FR-DESC-3** Data sensitivity stays a SEPARATE dimension (§10.6
  closing law): the existing `tool.Attrs` (Sensitive/Network) and their
  rules are NOT folded into the class. A read may be more sensitive than
  a write; the two dimensions compose, never merge.

### FR-REG — the validated registry (§9.7)

- **FR-REG-1** Every builtin declares its descriptor in
  `internal/tool` beside `BuiltinAttrs` (same mold:
  `BuiltinEffects(name) (action.EffectDescriptor, bool)`), with HONEST
  semantics (§7.10):
  `time`/`echo`/`calc` → `pure`; `read_file` → `read_external` with
  `DataEgress` true (what is read enters a model prompt);
  `http_fetch` → `read_external` with `DataEgress` true (the request
  travels to an external host); `webhook_call` → `write_irreversible`
  with `WritesExternalState`+`DataEgress` true (an arbitrary downstream
  POST has no documented undo and no known compensation — honesty over
  optimism); `memory_note` → see `[NEEDS CLARIFICATION]` 1.
- **FR-REG-2** The Effect Engine classifies ONLY from this registry —
  never from model text, never from parameters (§9.7 law, pinned by a
  tripwire test: the classifier's input is the operation name alone).
- **FR-REG-3** An operation WITHOUT a descriptor fails at boot preflight
  (a registered tool missing its declaration is a boot error, the E-11
  mold) and — defense in depth — an attempt reaching the gate for an
  undeclared operation is DENIED with the stable rule
  `effect_undeclared` (blueprint mandatory test 4). Both paths, both
  tested.

### FR-ENV — the envelope wakes its real Effect

- **FR-ENV-1** `buildActionEnvelope` (brain adapter) fills
  `Effect{Class}` from the registry; `"unclassified"` disappears from
  NEW rows (historical rows keep it — readable, never rewritten).
- **FR-ENV-2** **Receipts compatibility pinned by test**: `Effect` does
  not participate in `action.Digest` (verified at the seam,
  `action.go:127`), so the same logical action produces the SAME digest
  before and after this stage — asserted explicitly, the FR-ENV-2/E2
  discipline inherited.

### FR-POL — versioned Policy Decision (§10.7 subset)

- **FR-POL-1** Every gate decision pins the policy that took it:
  `policy_version` (monotonic per boot-loaded config) and
  `policy_digest` (canonical digest over the brain's effective governance
  + the effect registry snapshot), persisted with the decision row
  (additive columns, store migration v2→v3 on the E2 runner — one
  transaction per step, crash-proof by the same AS-8 mold).
- **FR-POL-2** A config reload NEVER alters already-authorized actions
  (blueprint mandatory test 2): decisions are immutable rows; the test
  reloads with a tightened policy and proves the recorded decision and
  its pinned digest stand, while the NEXT attempt gets the new digest.
- **FR-POL-3** Changed arguments change the digest and force a NEW
  decision (blueprint mandatory test 1): pinned end-to-end — two
  attempts differing in one argument byte record two decisions over two
  parameter digests.
- **FR-POL-4** Read and write can receive DIFFERENT policy (blueprint
  mandatory test 3): under one grant with `effect_ceiling =
  read_external`, `http_fetch` (read) is authorized while `webhook_call`
  (write) dies `effect_ceiling` — same brain, same channel, same intent,
  different consequence.

### FR-CEIL — the ceiling wakes (tenth dimension)

- **FR-CEIL-1** `AuthorityGrant` gains `EffectCeiling EffectClass`
  (empty = NO ceiling, the house zero-is-unlimited semantics — the root
  and all config-derived grants carry none, so today's flows cannot
  change). Persistence: additive `effect_ceiling` column on `grants`
  (the same v2→v3 migration); CLI: `--effect-ceiling <class>` on
  `grant issue` and `grant delegate` (flag value validated against the
  finite enum, usage error otherwise).
- **FR-CEIL-2** Attenuation gains its TENTH dimension over the total
  order: a child's ceiling must satisfy `Rank(child) <= Rank(parent)`;
  a child WITHOUT ceiling under a parent WITH one widens (the budget
  mold). The independent oracle, the seeded property rounds and
  `FuzzAttenuation` are ALL extended to the new dimension; the rejection
  names `effect_ceiling`.
- **FR-CEIL-3** Enforcement at the action gate: an attempt whose class
  ranks above the governing grant's ceiling is DENIED with the stable
  rule `effect_ceiling` — recorded like every denial, observation
  byte-shaped like today's denials.

### FR-REQ — require_approval / require_prepare, failing closed

- **FR-REQ-1** The gate's outcome vocabulary gains `require_approval`
  and `require_prepare` (blueprint E3). With NO approval/prepare
  workflow until E5, both fail CLOSED at this stage: the attempt records
  DENIED with the stable rules `approval_unavailable` /
  `prepare_unavailable` (honest names: the mechanism is absent, not the
  requirement). NEVER a pass for lack of machinery (§7.5).
- **FR-REQ-2** Reachability in this stage: see `[NEEDS CLARIFICATION]`
  2 (the one real fork of this spec). Whatever the answer, flows under
  the root intent never reach these outcomes — the exterior stands.

### FR-COMPAT — the outside does not move (master criterion)

- **FR-COMPAT-1** Today's flows under the root intent keep IDENTICAL
  behavior: same outcomes, same observations, same audit surfaces —
  classification lands in rows, the experience does not move one pixel.
  ZERO existing tests modified; the whole suite passes as-is; the §24
  16-point checklist re-runs via the existing gates.
- **FR-COMPAT-2** The toll is re-measured with classification and
  ceiling checks in the hot path — the 5 ms p95 ceiling stands; the
  benchmark mold of E1/E2 gains the classified variant.
- **FR-COMPAT-3** The five current SelectTools rules (channel,
  sensitivity×locality, shield, cage, grants) remain in force UNTOUCHED
  (blueprint mandatory test 5) — the action gate keeps wrapping, never
  replacing (the Etapa-1 law).

## Acceptance scenarios (Given / When / Then)

- **AS-1 (args force a new decision — mandatory)** Given one authorized
  action, When a second attempt differs in one argument byte, Then it
  records a NEW decision over a DIFFERENT parameters digest — no reuse.
- **AS-2 (reload immutability — mandatory)** Given an authorized action
  recorded under policy digest D1, When the config reloads with a
  tightened governance (digest D2), Then the recorded decision still
  pins D1 unchanged and the next attempt pins D2.
- **AS-3 (read vs write — mandatory)** Given a grant with
  `effect_ceiling = read_external`, When the same brain attempts
  `http_fetch` and `webhook_call`, Then the read is AUTHORIZED and the
  write is DENIED `effect_ceiling`, both recorded.
- **AS-4 (no descriptor — mandatory)** Given a tool registered without
  an effect declaration, When boot runs, Then preflight fails naming the
  tool; AND Given such an operation reaches the gate anyway, Then it is
  DENIED `effect_undeclared`, never executed.
- **AS-5 (current rules stand — mandatory)** Given the whole
  pre-existing suite, When the stage's gates run, Then channel,
  sensitivity and locality rules behave exactly as in v0.12.0 with ZERO
  test edits.
- **AS-6 (ceiling attenuation)** Given a parent grant with ceiling
  `write_reversible`, When a child requests ceiling
  `write_compensatable` (or no ceiling), Then attenuation rejects it
  naming `effect_ceiling`; a child at `read_external` passes — oracle,
  property rounds and fuzzer all agree.
- **AS-7 (fail-closed novelty)** Given a flow whose policy requires
  approval (per NC-2's resolution), When the attempt reaches the gate,
  Then it records DENIED `approval_unavailable` and the tool NEVER
  executes.
- **AS-8 (receipts stand)** Given one logical action executed in
  v0.12.0 and in this stage, When digests are compared, Then they are
  byte-identical (Effect is not a digest input — pinned).

## Success criteria

- Coverage ≥90% for everything new in `internal/action` and
  `internal/policy` (authorization tier); ≥85% for the store's new
  surface; `make quality` green with `-race` over the WHOLE suite,
  fuzz smoke included (extended `FuzzAttenuation`; new fuzz targets for
  any new parser/validator — none is expected beyond the class
  validator, which gets property+fuzz coverage of its total order).
- The five blueprint-mandatory tests present and green; §24 re-run;
  `go.mod` untouched; toll re-measured under the ceiling.
- Exit criterion (blueprint): Korvun can EXPLAIN why a specific action
  on a specific resource was allowed or denied — decision row + rule +
  policy digest + ceiling, readable from the aduana.

## Decisions folded in

1. **Unknown class ranks above critical** — fail-closed comparability:
  a corrupt or future class can never sneak under a ceiling.
2. **`webhook_call` is `write_irreversible`** — an arbitrary downstream
  POST has no documented undo and no known compensation; §7.10 honesty
  over optimism (revisit per-connector in E7).
3. **Ceiling absent = unlimited** — the house zero-semantics (budgets,
  expiry) extended; root and config-derived grants carry none, which is
  exactly what keeps today's flows untouched.
4. **New stable rules proposed** (vocabulary adjudication, the
  operator/attenuation_violated mold): `effect_undeclared`,
  `effect_ceiling`, `approval_unavailable`, `prepare_unavailable`.
  Finite, none reachable under the root.
5. **Policy digest covers governance + effect registry** — a change in
  EITHER is a different policy; the digest rides the existing canonical
  hasher (HashCanonical), no new machinery.
6. **Store migration v2→v3 on the E2 runner** — additive columns only
  (`grants.effect_ceiling`, decision policy pins), one transaction per
  step, the AS-8 crash mold re-armed for v3.

## `[NEEDS CLARIFICATION]`

1. **`memory_note`'s honest class.** It writes DURABLE local state (the
   brain's notes in the shared DB) with a documented undo
   (`/notes clear`). (a) `pure` — no EXTERNAL state is touched;
   (b) `write_reversible` with `WritesExternalState=false` — durable
   observable state is written, and calling that pure would lie; the
   class ladder measures consequence, not network distance. **My vote:
   (b)** — truth of consequence; zero behavior change either way (no
   ceiling governs it today).
2. **When does `require_approval` become reachable in E3?** (a) Never
   yet: the vocabulary + fail-closed path exist but no policy emits them
   until E5 wires real approvals — smallest surface, purely dormant;
   (b) `write_irreversible`/`critical` attempts under a LIMITED grant
   (never the root) already require approval → they die
   `approval_unavailable` until E5. **My vote: (b)** — an operator who
   bounds authority with a limited grant gets the full protection of the
   §10.6 treatment table TODAY, fail-closed and honest («your contract
   demands an approval mechanism Korvun does not have yet»), while the
   root keeps today's behavior byte-for-byte. The E5 estreno then turns
   denials into approvals — a visible, narratable upgrade.

## Batching (honest troceo — it smells like L, so five batches)

1. **Lote 1 — the effect domain (M):** `EffectClass` + total order +
   `EffectDescriptor` (+RESERVED reasons), the registry
   (`BuiltinEffects`) with the seven honest declarations, boot preflight
   validation, property tests of the order (antisymmetry,
   totality) and the unknown-above-critical law.
2. **Lote 2 — the envelope wakes (M):** adapter classification from the
   registry, `effect_undeclared` fail-closed at the gate, AS-8 digest
   compatibility pinned, toll re-measured.
3. **Lote 3 — the ceiling (L):** grant domain field + tenth attenuation
   dimension (oracle/property/fuzz extended), store migration v2→v3
   (ceiling column + AS-8 crash mold re-armed), CLI flag on
   issue/delegate, gate enforcement with the `effect_ceiling` rule.
4. **Lote 4 — versioned policy decision (M):** policy digest
   (governance + registry snapshot), decision pins persisted, the three
   mandatory tests (args/reload/read-vs-write) end to end.
5. **Lote 5 — the new outcomes + stage close (M):** `require_approval`/
   `require_prepare` per NC-2's resolution, fail-closed rules, the
   explain criterion on the aduana, §24, whole-suite master criterion,
   the operator's mini-act for Chano (a ceilinged grant seen denying a
   write while allowing the read).
