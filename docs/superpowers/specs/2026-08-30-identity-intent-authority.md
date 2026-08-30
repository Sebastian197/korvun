# Trust Layer Etapa 2 — Identidad, Intención y Autoridad: Design Spec

> **Status:** approved for TDD — **APROBADO POR CHANO 2026-08-30**, with
> the four forks resolved by his call:
> **(1)** the operator ROOT INTENT is AUTO-created at boot — idempotent,
> deterministic digest;
> **(2)** config grants are DERIVED, never materialized — config stays the
> single source of truth, the kernel explains without becoming a second
> judge;
> **(3)** limited intents ship via CLI in this stage — the Console card
> arrives later as its own piece with mockup + Sixth Law;
> **(4)** ONE principal PER CHANNEL — the individual sender stays
> data/subject in the evidence, never an identity card.
> Governing frame: `design-drafts/trust-layer-master-plan.md` (PLAN MAESTRO
> APROBADO 2026-08-29; Etapa 2 = "las acciones quedan atadas a un actor
> autenticado, a un Intent Contract vigente y a una autoridad que solo
> puede REDUCIRSE") and the blueprint §9.2-9.5, §10.1-10.4, §14, §23, §24.
> Governing ADRs: ADR-0041 (governed tools — today's grants), ADR-0021
> (tool seam), ADR-0019 (storage), plus the sealed Etapa-1 spec
> (2026-08-30-action-kernel, VERIFIED) whose store and envelope this stage
> extends. External-docs note: stdlib + existing internal packages + the
> already-adopted modernc.org/sqlite ONLY — no new dependency, no Context7
> needed; any lote that later wants a library stops for Context7 + ADR.

## Goal

After this stage, every action the kernel records carries WHO asked
(an authenticated Principal resolved from transport provenance — never
from text), UNDER WHAT authorization (an ACTIVE Intent Contract and an
Authority Grant chain whose every delegation is a strict subset of its
parent), and the envelope's reserved identity fields wake up — while
today's flows behave byte-for-byte identically: the single operator's
standing authority becomes an explicit ROOT INTENT that is always active,
so nothing that works today changes outcome, latency class or surface.
Effects/policy-per-action (E3), signed ledger (E4), approvals (E5),
transactions (E6), broker (E7), mature Console surfaces (E8), MCP/A2A
(E9) and multitenancy (E10) stay OUT.

## Anchors in the real tree (verified 2026-08-30)

- `internal/envelope/envelope.go:96,116` — `Sender` is
  `Participant{ID, Name}`: free strings the CHANNEL adapter sets. They are
  DATA, not identity (§14.1) — today nothing downstream treats them as
  authorization, and this stage keeps it that way.
- Channel provenance and its transport auth, per adapter:
  - `internal/channel/telegram/inbound.go` builds Sender from the Update;
    authenticity rests on the bot-token session (config `token_env`).
  - `internal/channel/webhook/webhook.go:235,242` maps SenderID/SenderName
    from the CLIENT body — client-controlled strings — while the REQUEST
    is authenticated by the shared inbound Bearer (`token_env`); the
    body's sender is a subject claim UNDER that transport, never more.
  - Discord: gateway session authenticated by the bot token.
  - Console: in-process desktop (loopback + the shell's admin injection);
    the operator's own hands.
- `internal/config/config.go` (agent block, ADR-0041) — today's authority:
  `Tools []string`, per-tool attrs and cages, tri-state grants with
  channel restrictions; `policy.ToolGrant`/`SelectTools`
  (`internal/policy/tools.go:121`) evaluate them per message.
- Etapa 1, live on master (`a81ffc4`, shipped v0.11.0): the kernel records
  every attempt (AUTHORIZED/DENIED/SHADOWED) with digest and lanes;
  `internal/action/action.go` documents THIS stage's reserved fields
  (`intent_id`, `principal`, `authority_refs`); `internal/action/sqlite`
  owns its schema lifecycle (`action_schema` v1) — this stage performs its
  FIRST real migration (v1→v2); `internal/brain` runTool is the adapter
  where the identity context must flow.

## Functional requirements

### FR-PRIN — Principal Resolver (§9.2, §14.1)

- **FR-PRIN-1** New domain in `internal/action` (leaf discipline intact):
  `Principal{PrincipalID, Type, DisplayName, CreatedAt, DisabledAt}` with
  types `operator_human`, `agent_brain`, `channel_peer`. `DisplayName` is
  NEVER used for authorization (§10.1). `tenant_id` stays a documented
  RESERVED field (single fixed local tenant until E10).
- **FR-PRIN-2** The resolver derives the principal from AUTHENTICATED
  PROVENANCE ONLY: the channel's config-pinned identity and type decide
  the principal TYPE and namespace; the envelope's `Sender.ID` is only the
  channel-scoped SUBJECT under that provenance. THE test (blueprint,
  mandatory): a crafted envelope whose `Sender.ID` claims the operator's
  id via a network channel still resolves to a `channel_peer` principal
  with that channel's evidence — a forged Sender can NEVER change the
  principal.
- **FR-PRIN-3** Resolution is kernel-side (see Decisions folded in):
  channel adapters change ZERO lines; the resolver reads
  (channel name → channel type/credential kind) from a provenance registry
  the app wires from config at boot.
- **FR-PRIN-4** The console channel resolves to the `operator_human`
  principal (the desktop's in-process/loopback provenance IS the
  operator's hands today); brains act as `agent_brain` principals whose
  `responsible_human_id` is the operator (§14.2 chain of responsibility,
  single-operator form).

### FR-EVID — Identity Evidence (§10.2)

- **FR-EVID-1** `IdentityEvidence{EvidenceID, Provider, Subject,
  CredentialType, IssuedAt, TransportBinding, ClaimsDigest}` records HOW a
  request authenticated: provider = channel type; credential types are a
  FINITE enum (`bot_token_session`, `inbound_bearer`, `gateway_session`,
  `loopback_inprocess`); ClaimsDigest reuses the lote-1 canonicalizer over
  the non-secret claims. NO secret material is ever persisted (§10.2) —
  evidence references credentials by NAME/kind, never value.
- **FR-EVID-2** Evidence rows are written by the kernel store alongside
  the action (same transaction discipline as attempt+decision).

### FR-INT — Intent Contract v1 (§10.3)

- **FR-INT-1** `IntentContract{IntentID, SchemaVersion, OwnerPrincipalID,
  Purpose, AllowedOperations, AllowedResources, Budgets, ValidFrom,
  ExpiresAt, Status, Version, ContractDigest}` with the state machine
  `DRAFT → ACTIVE → EXPIRED | REVOKED` (fail-closed transitions on the
  Etapa-1 sentinel mold; property-tested). `signature` stays RESERVED for
  E4 (receipts own signing). Resources in v1 are COARSE (operation-level:
  tool names or `*`) — fine-grained resources arrive with the Effect
  Engine in E3, documented as reserved.
- **FR-INT-2** **The root intent**: at boot, the kernel materializes the
  OPERATOR ROOT INTENT — permanent, ACTIVE, owned by the operator
  principal, purpose "operate this Korvun instance", operations `*`,
  no expiry — the explicit form of the standing authority the single
  operator already exercises. Its digest is deterministic (same config,
  same digest), it is idempotent across boots, and it is the parent under
  which today's behavior lives unchanged.
- **FR-INT-3** Limited intents (bounded purpose/operations/budget/window)
  are creatable through an admin path in this stage (see
  `[NEEDS CLARIFICATION]` 3 for WHICH path) and revocable; revocation and
  expiry fail CLOSED (§7.5): an action under a non-ACTIVE intent is
  DENIED with a stable rule (`intent_inactive`, `intent_expired`).

### FR-AUTH — Authority Grants + attenuation (§10.4, §14.3)

- **FR-AUTH-1** `AuthorityGrant{GrantID, IntentID, IssuerPrincipalID,
  SubjectPrincipalID, ParentGrantID, Operations, ResourceScope, Budgets,
  ValidFrom, ExpiresAt, DelegationDepthRemaining, Status, GrantDigest}`.
  `effect_ceiling` and `data_scope` are documented RESERVED (E3 owns
  effect semantics; storing an uncomparable ceiling would fake a check).
- **FR-AUTH-2** **Attenuation is the law (§14.3)**: a delegation is valid
  ONLY if child ⊆ parent in EVERY present dimension — operations subset,
  resources subset, budget ≤ remaining parent budget, expiry ≤ parent
  expiry, depth < parent depth. The validator is a PURE deterministic
  function in the leaf domain, property-tested (never accepts a widening
  on any dimension) AND fuzzed from birth (house standard since E1):
  `FuzzAttenuation` throws arbitrary parent/child pairs and asserts no
  panic and no accepted widening.
- **FR-AUTH-3** Expired or revoked authority fails CLOSED (mandatory
  test): the decision is DENIED with `authority_expired` /
  `authority_revoked` — never a silent pass, never a fallback to the root.

### FR-BUD — Budgets (§10.3 basics)

- **FR-BUD-1** v1 budgets per intent: `max_actions` (count) and
  `max_actions_per_operation` (optional map). Consumption lives in the
  kernel store and is enforced with a serialized conditional consume
  (`UPDATE ... SET consumed = consumed + 1 WHERE consumed < limit` on the
  single-writer connection): the MANDATORY test proves N concurrent
  consumers under `-race` never push consumption past the limit, and the
  loser attempts are DENIED `budget_exhausted` (fail-closed).
- **FR-BUD-2** The root intent carries NO budget (unlimited standing
  authority — today's truth); budgets bite only on limited intents, so
  existing flows cannot hit them.

### FR-MIG — the legacy bridge (blueprint E2: "migración explícita")

- **FR-MIG-1** Today's governed tools BECOME authority under the root
  intent with ZERO experience change: at boot the kernel DERIVES from
  config (not duplicates — see Decisions folded in) one stable grant per
  brain — subject `agent_brain:<name>`, operations = the brain's
  configured tools, channels restriction carried through — with
  deterministic derived ids (`grant_cfg_<digest>`). `SelectTools` and the
  two-point gate remain THE decision engine, byte-for-byte; the derived
  grants are the recorded EXPLANATION (`authority_refs`), not a second
  judge — the wrap-not-replace law of Etapa 1, inherited.
- **FR-MIG-2** Config remains the single source of truth for the
  operator's standing authority: editing tools in the Builder keeps
  working exactly as today, and the derived grants follow on reload.

### FR-ENV — the envelope wakes (E1 reserved fields)

- **FR-ENV-1** `action.Envelope` gains `Principal{PrincipalID, EvidenceID,
  ResponsibleHumanID}`, `IntentID`, `AuthorityRefs []string`; the kernel
  adapter (lote-3 runTool path) fills them from the resolver + the active
  intent/grants. The store's actions table migrates (v1→v2, ADDITIVE
  columns, the store's OWN migration lifecycle exercising its first real
  upgrade; existing rows keep NULL identity columns and remain readable).
- **FR-ENV-2** **Receipts compatibility**: `parameters_digest` algorithm
  and inputs are UNTOUCHED (identity fields are row columns, not digest
  inputs) — every Etapa-1 digest remains valid and comparable.
- **FR-ENV-3** Decision rules grammar grows only FINITE new labels
  (`intent_inactive`, `intent_expired`, `authority_expired`,
  `authority_revoked`, `budget_exhausted`, `principal_disabled`) — none
  reachable by today's flows under the root intent.
  - **Addendum (adjudicated by Chano, 2026-08-30, batch 2):**
    `authority_inactive` joins the finite set — a grant that is not yet
    in force (DRAFT, unknown status, or before its validity window) is
    neither expired nor revoked, and labeling it either would lie to the
    audit trail. Vocabulary rigor only; no scope change: the label is as
    unreachable by today's root-intent flows as the other six.
  - **Addendum (adjudicated by Chano, 2026-08-30, batch 5):** two more
    finite labels for the operator's CLI acts. `operator` — the operator
    wields the root's standing authority directly; calling that act
    `granted` or `ungoverned` would lie to the trail. And
    `attenuation_violated` — a CLI delegation refused by the §14.3 wall;
    the widened dimension stays in the human-facing error, keeping the
    trail's grammar finite. Vocabulary rigor only; no scope change:
    neither label is reachable from the hot path.

## Acceptance scenarios (Given / When / Then)

- **AS-1 (forged Sender — mandatory)** Given a webhook envelope whose body
  claims `sender_id` equal to the operator's principal id, When resolved,
  Then the principal is `channel_peer` under webhook evidence
  (`inbound_bearer`), and the recorded action's principal NEVER matches
  the operator's — asserted at resolver level AND end-to-end on the
  recorded row.
- **AS-2 (expired/revoked fails closed — mandatory)** Given a limited
  intent that expires (or is revoked) between two attempts, When the
  second attempt arrives, Then it records DENIED with
  `intent_expired`/`intent_revoked` semantics and the tool NEVER executes;
  same for an expired/revoked grant (`authority_*`).
- **AS-3 (no delegation widens — mandatory)** Given any parent grant,
  When a child requests one extra operation, a longer expiry, a larger
  budget, or equal depth, Then attenuation rejects it with the named
  dimension — table-driven for each dimension, property-tested across
  random pairs, and fuzzed.
- **AS-4 (concurrent budget — mandatory, -race)** Given a limited intent
  with `max_actions = N`, When 4×N concurrent attempts race, Then exactly
  N record AUTHORIZED and the rest DENIED `budget_exhausted`; consumption
  never exceeds N.
- **AS-5 (subset property model — mandatory)** Property tests over the
  subset lattice: attenuation is reflexive-safe (child == parent minus one
  element passes), antisymmetric where required, and NEVER accepts any
  pair the naive set-model rejects.
- **AS-6 (the outside does not move)** Given the entire pre-stage suite,
  When the whole gates run, Then everything passes with ZERO edits to
  existing tests; every today-flow action records under the root intent
  with the SAME outcome, observation and audit surface as v0.11.0.
- **AS-7 (migration equivalence)** Given a config with governed tools,
  When booted before and after this stage, Then `SelectTools` outcomes are
  identical for every (brain, tool, channel) triple, and the recorded
  `authority_refs` name the derived grant deterministically across boots.
- **AS-8 (store migration)** Given a v1 actions database with rows, When
  the store opens at v2, Then old rows read back intact (identity columns
  NULL), new rows carry identity, and `action_schema` reports 2 — with a
  crash-during-migration test proving boot-fatal-not-corrupt.

## Success criteria

- Coverage ≥90% for everything new in `internal/action` (authorization
  tier); ≥85% for the store's new surface; `make quality` green with
  `-race` over the WHOLE suite (fuzz smoke included — the new
  `FuzzAttenuation` and any new parser fuzz join it).
- The five mandatory blueprint tests present and green; the §24 16-point
  checklist re-run via the existing suites; `go.mod` untouched.
- Chano sees (master plan, Etapa 2): a limited intent created live
  («solo lecturas, hasta el viernes» in spirit: bounded ops + window) and
  an out-of-scope action dying DENIED with its rule — via the admin path
  of `[NEEDS CLARIFICATION]` 3 — plus the aduana query now answering WHO
  and UNDER WHAT INTENT for every row.

## Decisions folded in

1. **Resolver lives kernel-side, adapters untouched** — the provenance
   registry (channel name → type/credential kind) comes from config the
   app already owns; pushing evidence into 4 adapters would widen the
   blast radius for zero truth gained today (their transports are
   config-known). Revisited in E9 when external protocols arrive.
2. **Root intent auto-materialized at boot** — the single operator's
   standing authority made explicit without ceremony; deterministic and
   idempotent. (Fork 1 below offers the alternative for veto.)
3. **Derived (not persisted) config grants** — config stays the ONE
   source of truth for standing authority; persisted copies would drift.
   Only NEW limited intents/delegations persist as rows.
4. **Reserved-not-faked dimensions** — `effect_ceiling`/`data_scope`
   stay reserved until E3 gives them comparable semantics; storing an
   unenforceable ceiling would be governance theater.
5. **New decision rules are finite and unreachable by today's flows** —
   the grammar grows six labels, all gated behind limited intents or
   disabled principals, keeping AS-6 honest.

## `[NEEDS CLARIFICATION]` — bifurcaciones reales, con voto

1. **Nacimiento de la intención raíz**: (a) auto-creada ACTIVE en el boot
   desde config, idempotente — cero ceremonia, la autoridad de siempre
   hecha explícita (**mi voto**); o (b) paso explícito de creación única
   (ceremonia sin valor hoy; defendible solo si Chano quiere el acto
   fundacional visible).
2. **Los grants de config**: (a) DERIVADOS en memoria en cada boot con
   ids deterministas — config como única verdad (**mi voto**); o (b)
   materializados una vez como filas migradas (histórico más literal,
   pero dos verdades que pueden divergir).
3. **El camino de creación de intenciones limitadas en esta etapa**:
   (a) comando CLI admin (`korvun intent create|revoke|list`) + la
   demo-consulta de la aduana — motor puro, cero superficie, y la
   tarjeta de Consola llega DESPUÉS como pieza propia con maqueta y
   Sexta Ley (**mi voto**, alineado con el plan maestro que reserva la
   experiencia de creación como decisión de Chano); o (b) tarjeta mínima
   en la Consola YA en esta etapa — exige maqueta + su sí ANTES del RED
   de ese lote (Sexta Ley), alargando la etapa.
4. **Identidad del peer de canal**: (a) un principal POR CANAL con el
   subject del Sender como atributo de la evidencia — cardinalidad
   acotada, suficiente para E2 (**mi voto**); o (b) un principal por
   remitente (por chat_id) — más granular, pero abre cardinalidad sin
   consumidor real hasta que existan intents por-remitente.

## Troceo en lotes (etapa XL — cinco lotes)

- **Lote 1 — dominio de identidad (M):** Principal + IdentityEvidence +
  resolver puro con registro de procedencia; digests con el canonicalizador
  de E1; fuzz de cuna del resolver/claims. AS-1 a nivel dominio.
- **Lote 2 — intención y autoridad (L):** IntentContract v1 con su
  máquina fail-closed, AuthorityGrant, el validador de atenuación
  (property + fuzz), digests deterministas. AS-3/AS-5.
- **Lote 3 — persistencia v2 (M/L):** la PRIMERA migración real del
  store del kernel (tablas nuevas + columnas aditivas en actions, molde
  boot-fatal con test de crash a mitad), presupuestos con consumo
  concurrente acotado. AS-4/AS-8.
- **Lote 4 — el cuello despierta (L):** resolver + intención raíz +
  grants derivados cableados en el adaptador de runTool; el envelope
  rellena principal/intent/authority; reglas nuevas; equivalencia total
  con v0.11.0 (AS-6/AS-7); benchmark del peaje actualizado (el techo de
  5 ms p95 sigue mandando).
- **Lote 5 — el acto de Chano (M):** el camino de creación elegido en
  NC-3, la revocación/expiración vivas (AS-2 end-to-end), la
  aduana-consulta enriquecida (quién y bajo qué intención), cierre de
  etapa con su mini-bash.

Total honesto de la etapa: **XL** (cinco lotes, cada uno con su ciclo
RED→GREEN→quality completo y su review).
