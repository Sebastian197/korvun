# Trust Layer Etapa 4 — Ledger and verifiable receipts: Design Spec

> **Status:** APROBADO POR CHANO — sealed for TDD, 2026-08-30. The three
> forks resolved with the house votes: **NC-1 YES** — a signed receipt
> for ALL outcomes, denials included («los noes son la mitad valiosa de
> la evidencia»; the toll will measure the microseconds); **NC-2 YES** —
> SHADOWED leaves its receipt at record time like any terminal (the
> rehearsal's observation made provable; one rule, zero special cases);
> **NC-3 YES** — result_digest computed on the fly over the observation,
> NEVER persisting raw content (privacy and verifiability married).
> Governing frame: `design-drafts/trust-layer-master-plan.md` (sealed)
> and the blueprint `docs/blueprints/2026-08-15-execution-trust-layer.md`
> (§7.8, §10.10, §19 ENTERO, §23, §24, Etapa 4, §30).
> Governing ADRs: ADR-0024 (metadata-only surfaces), ADR-0010 (secrets
> env-only), ADR-0017 (boot-fatal posture).
> External-docs note: ONLY stdlib (`crypto/ed25519`, `crypto/sha256`,
> `crypto/rand`, `encoding/json`, `os`) + existing `internal/` packages +
> the already-adopted `modernc.org/sqlite`. No new dependency, no
> Context7 need — the §30 signature decision lands on stdlib (below).
> Seam audit performed on real code (2026-08-30): the aduana rows (store
> v4) already carry action_id, correlation, source, operation,
> parameters_digest (the E1 `Digest` = op + canonical params — this IS
> §10.10's action_digest), effect_class, state, recovery_marker,
> requested/finished_at, principal_id, intent_id, authority_refs,
> decision (outcome, rule, policy_version, policy_digest) and per-attempt
> evidence. RAW PARAMETERS ARE NEVER PERSISTED — only digests (E1 law,
> re-verified): this truth decides the §30 encryption question honestly
> (below). The E1 canonicalizer (`CanonicalParams`/`HashCanonical`,
> fuzzed from birth) is the canonicalization base; `Finish` is the
> terminal close seam; the operator-CLI mold (E2, inherited decision 3)
> carries the verifier; the profile dir (`os.UserConfigDir()/korvun`,
> 0700 observed) hosts the signing key; the migration runner (v4, AS-8
> anti-zombie) takes the v5 step.

## Goal

Operational audit becomes durable, verifiable EVIDENCE without exposing
sensitive content. After this stage the aduana's rows are reified into
canonicalized, signed Execution Receipts on an append-only, hash-chained
ledger: a third party (today: the operator, offline, via CLI) can verify
WHAT was requested, WHICH policy decided and WHAT result was recorded —
and any modification, gap or reordering of the ledger is DETECTED and
NAMED. §7.8 elevated: proof was already part of execution
(record_failed, E1); now the proof itself is verifiable. The engine
stays invisible: transcript, feeds and SSE receive nothing (§19.1); the
experience does not move one pixel.

**The honesty sentence (spec-level requirement, §19.3):** the hash chain
provides TAMPER EVIDENCE. It must NEVER be described — in this spec, in
README, web, release notes or any future teaser — as absolute
immutability, because the operator controls storage and keys. Allowed
public language: "tamper-evident, verifiable receipts". Forbidden:
"immutable", "unforgeable", "blockchain-grade". This is a standing rule
for every public surface from now on.

Out of scope (deferred, declared): approvals/preview (E5), transactions
(E6), credential broker (E7), Console receipt surfaces (E8 — this
stage's verifier is CLI), public verification API (E9 — the receipt
format is designed to be verifiable outside the process, which prepares
that interface without opening any door), multitenancy (E10 — the
partition field exists with ONE partition, below). The Windows
first-run-deadline hardening filed on 2026-08-30 may enter as its own
small piece inside this stage when ordering allows.

## §30 decisions RESOLVED with evidence

1. **Signature algorithm/format: Ed25519 (stdlib `crypto/ed25519`).**
   Alternatives weighed: ECDSA P-256 (stdlib, but signature
   malleability/encoding pitfalls and nonce sensitivity), RSA-PSS
   (stdlib, large keys/signatures, slow), Sigstore/cosign keyless (a
   heavy dependency + network trust root — right for RELEASE artifacts,
   wrong for a local, offline, per-profile ledger). Ed25519: pure
   stdlib, deterministic signatures (no nonce failure mode), 32-byte
   keys / 64-byte signatures, fast (~tens of µs — the toll measurement
   will pin it), widely verifiable outside Go (prepares E9). Format:
   `signature = ed25519.Sign(priv, sha256(canonical_receipt))` carried
   hex-encoded; `signing_key_id = "ed25519:" + hex(sha256(pub))[:16]`.
   The algorithm name lives INSIDE the key id — a future algorithm
   coexists without ambiguity (the E1 digest-prefix precedent).
2. **Protected-parameters encryption provider: DEFERRED WITH EVIDENCE —
   there is NOTHING to encrypt.** Raw parameters are never persisted
   (E1 law, re-verified at the seam); receipts carry digests only.
   Adopting an encryption provider today would be governance theater
   over an empty set. The schema reserves `protected_params_ref`
   (NULL), and the decision re-opens WHEN a stage actually persists
   protected content (E6 transactions / E7 broker), with the house
   preference noted then: stdlib AES-256-GCM under a profile key before
   any external dependency. Written reason, zero theater — the E2
   RESERVED discipline.

## Functional requirements

### FR-LED — the append-only ledger (§19.2)

- **FR-LED-1** New store surface (migration v4→v5 on the anti-zombie
  runner): `receipts` table — one row per TERMINAL outcome (DENIED and
  SHADOWED at record time; SUCCEEDED/FAILED at `Finish`) — and
  `signing_keys` (key_id, public_key, created_at, retired_at; retired
  keys are KEPT forever, never deleted). Receipts are appended in the
  SAME transaction as the row they reify (the FR-EVID-2 mold: together
  or nothing) through the domain API only — no UPDATE/DELETE paths
  exist on receipts (append-only by construction, and the verifier
  detects out-of-band edits).
- **FR-LED-2** Hash chain per partition: `previous_receipt_hash` +
  `receipt_hash` with a stable order (chain sequence per partition).
  v1 ships ONE partition (`"main"`) — the local single-operator truth —
  with the partition field real, so E10 shards without a schema break.
  The genesis receipt links to the empty hash (documented constant).
- **FR-LED-3** Gap and tamper detection is a FIRST-CLASS operation
  (`ledger check`, FR-VER-2): re-canonicalize → re-hash → walk the
  chain → verify signatures and key validity windows; the FIRST broken
  link is NAMED (receipt id, position, which check failed: hash
  mismatch / chain break / missing sequence / bad signature / key
  invalid at signing time).
- **FR-LED-4** Retention: receipts are EXEMPT from the E1 actions cap
  (they are the evidence; the actions cap keeps pruning operational
  rows). Bounded-growth reason written: receipts are one bounded row
  per action outcome, dominated by the same cap dynamics upstream —
  and `ledger check` reports counts so growth is observable. (Per-
  tenant retention: E10.)

### FR-REC — the canonicalized receipt (§10.10 subset v1)

- **FR-REC-1** v1 fields: `receipt_id`, `action_id`, `intent_digest`
  (the stored contract's term digest at decision time), `principal_id`,
  `authority_digest` (the explaining grant's digest; "" under the
  root's standing authority), `decision_digest` (canonical digest over
  outcome+rule+policy pin), `action_digest` (THE E1 parameters digest,
  unchanged), `effect_class`, `attempt` (1 in v1 — retries are E6),
  `outcome` (the terminal state), `result_digest` (see NC-3),
  `started_at`, `finished_at`, `partition`, `chain_seq`,
  `previous_receipt_hash`, `receipt_hash`, `signing_key_id`,
  `signature`. RESERVED with written stages: `transaction_id` (E6),
  `approval_digest` (E5), `executor_id`/`target_system` (E7 — today
  there is exactly one in-process executor; writing a constant would be
  decoration), `external_reference` (E7), tenant (E10),
  `protected_params_ref` (§30-2 above).
- **FR-REC-2** Canonicalization is DETERMINISTIC on the E1 fuzzed
  canonicalizer: the receipt's signable form is the canonical JSON of
  its fields (sorted keys, RFC3339Nano UTC times — the contract-digest
  mold), so the same action produces the same receipt bytes, byte for
  byte — pinned by test, and the canonical form is FUZZED from birth
  (parser side: `receipt verify` reads untrusted bytes).
- **FR-REC-3** `receipt_hash = "sha256:" + hex(sha256(canonical form
  WITHOUT receipt_hash/signature))`; the signature covers the same
  bytes. Verification never trusts stored hashes: it recomputes.

### FR-KEY — signing and rotation (§19.3)

- **FR-KEY-1** Key generation at boot, idempotent (the root-intent
  mold): missing key → generate Ed25519 pair, private at
  `<profile>/keys/receipt-signing.key` (0600, dir 0700), public row
  into `signing_keys`; present → verified no-op; unreadable →
  boot-fatal. Stateless deployments (no storage) keep the whole ledger
  off — recording off means receipts off, the E1 posture.
- **FR-KEY-2** Every receipt carries `signing_key_id`. Rotation
  (`korvun receipt rotate-key`, operator CLI): retire the current key
  (retired_at stamped, public key KEPT), generate the new pair, new
  receipts sign with the new id — and EVERY historical receipt keeps
  verifying against its retired public key (blueprint mandatory test
  2). Key validity window checked by the verifier: a receipt signed
  outside its key's [created_at, retired_at] window FAILS verification.
- **FR-KEY-3** Threat honesty (§23): the private key lives on the disk
  the operator controls — the ledger is tamper-EVIDENT against
  accidental/after-the-fact modification and third-party doubt, not
  against the key-holding operator. Written in the spec, in SECURITY.md
  when this ships, and governing all public copy (Goal above).

### FR-VER — the operator's verifier (CLI, E2 mold)

- **FR-VER-1** `korvun receipt verify --config <path> <receipt-id>`:
  OFFLINE, against the file — re-canonicalize, recompute digests,
  verify signature, verify previous-hash link, check the key's validity
  window, and check COHERENCE with the underlying decision row
  (outcome/rule/policy pin match). Human output, stable exit codes
  (0 verified / 1 failed naming the check / 2 usage).
- **FR-VER-2** `korvun ledger check --config <path>`: the whole chain —
  every receipt re-verified, sequence continuity, gap/tamper NAMED at
  first break; summary line with counts (receipts, partitions, keys,
  oldest/newest). Brief WAL-safe access, the E2 CLI discipline.
- **FR-VER-3** Backup/restore verifiable (blueprint mandatory test 3):
  `ledger check` over a file-level backup copy verifies identically —
  the chain carries its own evidence; the test restores a backup and
  proves chain + states survive.

### FR-COMPAT — the outside does not move (master criterion)

- **FR-COMPAT-1** Motor invisible: zero experience change; ZERO
  existing tests modified; transcript/feeds/SSE receive NOTHING from
  the ledger (§19.1 — pinned by the explicit negative test: no receipt
  material, no key material, no signature bytes on any shared surface).
- **FR-COMPAT-2** The toll is re-measured WITH signing in the hot path
  (Ed25519 sign per terminal outcome): the 5 ms p95 ceiling stands;
  the measured cost is DECLARED in the batch report (expected: tens of
  µs over the ~0.9 ms identified+classified path).
- **FR-COMPAT-3** `record_failed` elevated (blueprint mandatory test
  5): every denial and every execution produces its receipt in the
  same transaction, or the attempt fails CLOSED with the visible error
  — an unreceipted effect cannot exist.

## Acceptance scenarios (Given / When / Then)

- **AS-1 (tampering detected — mandatory)** Given a populated ledger,
  When any receipt row is modified out of band (a byte of the outcome,
  a timestamp, a signature), Then `ledger check` FAILS naming that
  receipt and the failed check; same for a deleted row (sequence gap)
  and a reordered chain.
- **AS-2 (rotation preserves history — mandatory)** Given receipts
  signed under key A, When the operator rotates to key B and records
  more, Then `ledger check` verifies the WHOLE chain — historical
  receipts against retired A, new ones against B — and a receipt
  forged under A with a timestamp after A's retirement FAILS.
- **AS-3 (backup/restore — mandatory)** Given a backup of the store
  file, When restored on a clean profile, Then `ledger check` verifies
  the full chain and every state matches the original.
- **AS-4 (no secrets on shared surfaces — mandatory, negative)** Given
  full ledger activity, When logs, SSE, metrics and the transcript are
  captured, Then NO parameter content, key material, signature bytes or
  receipt payloads appear — digests and finite labels only (ADR-0024).
- **AS-5 (every outcome receipts or dies — mandatory)** Given a
  recorder whose receipt append fails, When an attempt reaches the
  gate, Then it fails CLOSED (record_failed — no effect without proof);
  and Given normal operation, every DENIED/SHADOWED/SUCCEEDED/FAILED
  row has exactly one receipt.
- **AS-6 (determinism)** Given one logical action, When its receipt is
  canonicalized twice (or on two machines), Then the bytes are
  identical; changing ANY v1 field changes receipt_hash.
- **AS-7 (verifier honesty)** Given a receipt whose underlying decision
  row disagrees (outcome swapped out of band), Then `receipt verify`
  FAILS on coherence naming the mismatch.
- **AS-8 (migration v5)** The AS-8 crash mold re-armed against a
  hand-built v4 file: aborted v5 leaves clean v4; next boot completes;
  fresh files land on v5.

## Success criteria

- Coverage ≥90% for everything new in `internal/action` (+ ledger
  package if split); ≥85% store surface; `make quality` green `-race`
  whole-suite, fuzz smoke extended (receipt canonical form parser).
- The five blueprint-mandatory tests present and green; §24 re-run;
  `go.mod` untouched; toll declared under ceiling.
- Exit criterion (blueprint): a third party can verify what was
  requested, which policy decided, and what result was recorded —
  offline, from the file, with the operator's CLI.

## Decisions folded in

1. **Ed25519 stdlib** (§30-1 above, with alternatives and threat
   model). 2. **Encryption deferred with evidence** (§30-2 above).
3. **One partition ("main") with a real partition field** — local
   single-operator truth now, E10 shards later without schema break.
4. **Receipts exempt from the actions retention cap** — evidence
   outlives operational pruning; growth observable via `ledger check`.
5. **Private key as a 0600 profile file, NOT the OS keychain** — the
   backup/restore mandatory test and headless Linux demand portable
   files; the keychain would break both. Threat model unchanged: the
   operator already controls the disk (§19.3 honesty). Trade-off
   declared; revisit if E9 introduces remote verification roots.
6. **Public language rule**: "tamper-evident", never "immutable" — a
   standing requirement for every future public surface.

## `[NEEDS CLARIFICATION]`

1. **Receipt emission for non-terminal-executed outcomes**: DENIED and
   SHADOWED receipts are emitted synchronously in the record
   transaction — signing on every denial costs one Ed25519 sign (~µs).
   (a) synchronous for ALL outcomes (one law, simplest chain);
   (b) receipts only for executed outcomes, denials stay rows-only.
   **My vote: (a)** — §7.8 says proof is part of execution, and a
   denial IS an outcome a third party may need to verify (the aduana's
   whole point); the toll measurement will prove the µs claim.
2. **Where the SHADOWED receipt closes**: shadow records terminally at
   the gate (never executes). (a) receipt at record time like DENIED;
   (b) treat as non-terminal. **My vote: (a)** — SHADOWED is terminal
   in the E1 machine; one rule, no special case.
3. **result_digest v1**: the tool result (observation) is NEVER
   persisted raw. (a) compute `result_digest` over the canonical
   observation bytes at Finish (cheap truth, raw never stored);
   (b) RESERVED until E6. **My vote: (a)** — it makes AS-7 coherence
   real for executed outcomes at zero content cost.

## Batching (honest troceo — it smells like L, so five batches)

1. **Lote 1 — the receipt domain (M):** canonical receipt v1 subset
   (+RESERVED reasons), deterministic canonicalization on the E1
   canonicalizer, receipt_hash + chain-link computation, birth fuzzing
   of the canonical-form parser, property tests (determinism,
   field-sensitivity).
2. **Lote 2 — keys (M):** Ed25519 keystore (boot-idempotent generation,
   0600 file + signing_keys table), signing_key_id, sign/verify
   helpers, rotation with retired keys kept and validity windows.
3. **Lote 3 — the ledger persisted (L):** migration v5 (AS-8 re-armed),
   append in the same transaction as record/finish, chain per
   partition, retention exemption, record_failed elevated, toll
   re-measured and declared.
4. **Lote 4 — the verifier (M-L):** `korvun receipt verify` +
   `korvun ledger check` + `korvun receipt rotate-key` (CLI mold,
   receipts for the operator's own acts included), backup/restore
   verification, gap/tamper naming.
5. **Lote 5 — stage close (M):** the five mandatory tests end to end,
   the AS-4 negative sweep, §24, public-language rule landed in
   SECURITY.md, the mini-act for Chano (tamper a byte, watch the check
   name it) — plus the Windows first-run-deadline hardening as its own
   small piece if ordering allows.
