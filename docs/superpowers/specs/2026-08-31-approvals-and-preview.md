# Trust Layer Etapa 5 — Preview, Agent Diff and Approvals: Design Spec

> **Status:** APPROVED FOR TDD — **SEALED BY CHANO 2026-08-31**, the
> three bifurcations decided with the house votes:
> **NC-1 = (b)** — the CLI closes the stage; the Console card ships as
> the LAST batch behind the ENTIRE Sixth Law (mockup + Chano's yes
> BEFORE any RED), movable to E8 without reopening this spec.
> **NC-2 = (i)** — approving EXECUTES the stored envelope, deferred, by
> IDENTITY: "the version shown" is guaranteed by being THE SAME THING,
> never by an equivalence argument.
> **NC-3 = (α)** — receipt canonical v2 with approval_digest INSIDE the
> seal and version-aware verification; mixed eras in the chain, like
> the signing keys.
> Governing blueprint: `docs/blueprints/2026-08-15-execution-trust-layer.md`
> (§9.8 preview/approval, §10.8 Approval fields, §12 state machine, §13
> two-level policy, §15 ENTIRE — shadow/agent diff/approval workflow and
> its invalidation law, §23 approval-reuse threat, §24 untouchables,
> Etapa 5 with its five mandatory tests, §10.10 the receipt's RESERVED
> approval field).
> Governing ADRs: ADR-0024 (metadata-only shared surfaces), ADR-0017
> (boot-fatal posture), ADR-0041 (governed tools).
> External-docs note: ONLY stdlib + existing `internal/` packages + the
> already-adopted `modernc.org/sqlite`. No new dependency, no Context7
> need — signing reuses E4's Ed25519 keystore verbatim.
>
> Seam audit performed on real code (2026-08-31, graphify + source):
> - `effectGateRule` (`internal/brain/effects.go:57`) already emits the
>   honest no `approval_unavailable` under bounded authority for
>   `write_irreversible`/`critical` — its own godoc names E5 as the
>   stage that turns that no into an approval card. THIS is the seam;
>   precedence (`effect_ceiling` first) stays byte-identical.
> - `StatePendingApproval` / `StateRejected` / `StateApproved` exist
>   RESERVED in `internal/action/state.go` since E1; the transitions
>   table has no edges for them (fail-closed by construction) — E5 adds
>   exactly three edges and nothing else wakes.
> - The action digest (E1 `Envelope.Digest`, canonical + fuzzed from
>   birth) already covers operation, resource, parameters and effect
>   class — the approval↔digest binding is structural, not new code.
> - The operator IS a principal with signed receipts (E2 identity + E4
>   ledger): `recordOperatorAct` seals every CLI mutation with the
>   profile's Ed25519 key. The approver's "signature or proof of
>   decision" (§10.8) is therefore ALREADY BUILT: the decision act's own
>   ledger receipt.
> - Surfaces today: a mature CLI mold (`intent`/`grant`/`receipt`/
>   `ledger`) vs a desktop console with chat/activity views but NO
>   actions surface. Evidence drives the inbox proposal below.
> - The migration runner (anti-zombie, at v6) takes v7; the temporal
>   window mold (intents/grants `ValidFrom`/`ExpiresAt`, clock-injected
>   validity rules) is reused verbatim for approval expiry.

## Goal

After this stage, an action whose effect class demands human approval
under bounded authority no longer dies with `approval_unavailable`: it
parks as `PENDING_APPROVAL` with a persisted, structured PREVIEW (the
agent diff), waits for the operator's explicit decision — approve,
reject, cancel, or expire — and executes ONLY the exact digest the
human saw, leaving a receipt that references its approval. The
blueprint's exit criterion verbatim: an irreversible action cannot
execute until the human approves exactly the version shown. Out of
scope (declared): multi-action plans/transactions (E6 — plan approval
stays RESERVED with its stage), credential broker (E7), the full
console (E8), API/MCP/A2A (E9), multitenant (E10).

## Functional requirements

### FR-PRV — the structured preview (agent diff, §15.2)

- **FR-PRV-1** A pure domain constructor builds an `ActionPreview` from
  the envelope + its identity + the gate context: intent purpose, actor
  and delegation chain (principal, grant id, depth), the resource(s)
  the operation touches, WHAT DATA LEAVES the system (the tool's
  declared egress from its E3 Effect Descriptor + the bounded args
  digest — never raw parameters on shared surfaces; the FULL parameters
  are shown ONLY on the operator's decision surface, which is loopback
  and his by right), cost/budget consumed (the E2 budget ledger view),
  reversibility (the E3 class, spelled), credentials/systems involved
  (the tool name and its cage today — honest: no broker until E7),
  relevant policies (the E3 policy pin: version + digest + the rule
  that demanded approval). "Difference against a previously approved
  plan" is E6's row and stays RESERVED, stated in the preview struct as
  an absent-by-design field.
- **FR-PRV-2** The preview is PERSISTED with the approval request (the
  human must be able to see later exactly what was approved) and its
  canonical digest rides inside the approval binding — a preview that
  no longer matches its action is an invalidation, not a display bug.

### FR-APR — the persistent Approval (§10.8)

- **FR-APR-1** `Approval` domain type, all §10.8 fields honest today:
  `approval_request_id` ("apr_" mold), `action_digest` (E1 digest — the
  EXACT anchor; plan digest RESERVED→E6), `requested_from` (the
  operator principal), `reason` (the gate rule that demanded it),
  `risk_summary` (class + reversibility line from the preview),
  `expires_at` (clock-injected, the E2 window mold; configurable TTL,
  default 1h), `status` (PENDING/APPROVED/REJECTED/EXPIRED/CANCELLED —
  finite, fail-closed), `decision_principal_id`, `decision`,
  `decision_at`, `comment`, and **proof of decision** = the receipt id
  of the operator's own decision act in the E4 ledger (its Ed25519
  signature IS §10.8's `signature`; no second signing path invented).
- **FR-APR-2** THE INVALIDATION LAW (§15.3), enforced structurally and
  by comparison at consume time: the approval binds the action digest
  (operation, resource, protected parameters, amount, recipient and
  effect class all live UNDER that digest by E1/E3 construction — any
  change is a different digest and the approval simply does not match)
  PLUS the policy pin captured at request time (a policy change between
  request and decision/execution invalidates: `approval_invalidated`
  naming "policy"). Plan/dependency changes: RESERVED→E6.
- **FR-APR-3** ONE-SHOT consumption: an approval is consumed exactly
  once, atomically, in the same transaction that moves the action out
  of PENDING_APPROVAL — two concurrent operators cannot double-approve
  (UNIQUE + state machine; the loser gets `approval_already_decided`).
  Reuse against any other action is impossible by digest binding (§23).
- **FR-APR-4** Expiry/rejection/cancellation: an expired approval NEVER
  executes (clock rule at consume time, not a background sweeper — the
  E2 validity mold); reject and cancel are operator acts with their own
  receipts; every terminal approval outcome closes the ACTION too
  (REJECTED action state; expiry surfaces as rejection with reason
  `approval_expired` at the next touch — no zombie PENDING rows:
  `korvun approvals list` shows expired ones honestly).

### FR-GATE — the gate wakes (§13, the E3 seam)

- **FR-GATE-1** `effectGateRule` precedence is UNTOUCHED; the
  `approval_unavailable` arm becomes `require_approval` ONLY when the
  approval workflow is enabled in config (`approvals.enabled`, default
  ON with the stage; OFF = today's honest no byte-for-byte — the
  fail-closed fallback stays forever as the disabled path).
- **FR-GATE-2** On `require_approval` the attempt is recorded
  PENDING_APPROVAL (new edge NORMALIZED→PENDING_APPROVAL... see
  FR-STATE-1) with its preview + approval request persisted in the SAME
  transaction (the E4 law: no unrecorded intermediate), and the MODEL
  receives an honest observation naming the pending request id — the
  conversation never blocks waiting for a human (the tool-loop timeout
  reality). PENDING_APPROVAL is NOT terminal: no ledger receipt yet.
- **FR-GATE-3** Shadow stays SACRED: a shadowed tool never executes and
  never creates approval requests (§15.1 semantics untouched, pinned).

### FR-STATE — three edges, nothing more (§12)

- **FR-STATE-1** The transitions table gains EXACTLY:
  `AUTHORIZED→PENDING_APPROVAL` is NOT drawn (blueprint §12 draws it
  from the authorization fork): the gate decides approval BEFORE
  authorization completes, so the edge added is
  `NORMALIZED→PENDING_APPROVAL`, plus
  `PENDING_APPROVAL→REJECTED` and `PENDING_APPROVAL→APPROVED`, plus
  `APPROVED→SUCCEEDED|FAILED` (E5 executes directly; PREPARING waits
  for E6). Terminal() gains REJECTED. Everything else stays fail-closed.
- **FR-STATE-2** Ledger integration: REJECTED (and the expired-close)
  births its signed receipt at the terminal transition; APPROVED is not
  terminal (no receipt); the executed outcome's receipt carries the
  approval reference (FR-RCPT).

### FR-EXEC — approve executes the shown version

- **FR-EXEC-1** Approving EXECUTES, deferred: the decision act (CLI,
  loopback) consumes the approval and runs the STORED envelope — the
  exact bytes whose digest the human saw — through the one executor
  seam, then closes SUCCEEDED/FAILED with the result digest (E4 NC-3
  unchanged). No agent retry loop, no second execution path: the model
  already moved on; the effect happens under the operator's hand.
  [Bifurcation NC-2 below records the alternative and the vote.]

### FR-RCPT — the receipt's approval field wakes (§10.10)

- **FR-RCPT-1** Canonical receipt v2: `approval_digest` (the canonical
  digest of the CONSUMED approval — id + action digest + decider +
  decision + decision_at) joins the sealed fields. Version-aware
  verification: v1 receipts keep verifying under the v1 form (their
  stored bytes are law), v2 under v2 — `receipt verify` and
  `ledger check` dispatch on the receipt's schema version; the chain
  mixes eras exactly like signing keys do. A v2 receipt with an
  approval whose digest does not match its stored approval row fails
  `custody_mismatch` naming approval.
- **FR-RCPT-2** Non-approved outcomes carry `approval_digest` = "" —
  honest empties, the E4 mold.

### FR-INBOX — where the human decides

- **FR-INBOX-1** CLI first (evidence: the mature E2/E4 operator mold):
  `korvun approvals list` (pending + recently decided, expiry visible),
  `korvun approvals show <apr_…>` (THE AGENT DIFF rendered: every
  §15.2 row the preview persisted, full parameters included — loopback
  operator surface), `korvun approvals approve <apr_…>` (consume +
  execute + receipt), `korvun approvals reject <apr_…> [--comment]`.
  All acts leave signed operator receipts (E4). List/show read-only.
- **FR-INBOX-2** The Console card ([NEEDS CLARIFICATION NC-1]): a
  minimal desktop inbox (pending list + diff + approve/reject) over a
  loopback control-API extension — SIXTH LAW: no RED phase before
  Chano approves a UX design from `UX-TEMPLATE.md` + rendered mockup.
  Batched LAST so the CLI (which satisfies every mandatory test)
  closes the stage even if the card slips to E8.

### FR-AUTH — approver authentication, beta-honest

- **FR-AUTH-1** Today's truth, stated without theater: Korvun is a
  single-operator binary; the approver is THE operator, authenticated
  by possession of the machine, the profile and the loopback CLI
  (provenance `cli` / `CredentialLoopbackInProcess` — the E2 identity
  chain), and every decision is signed by the profile's Ed25519 key via
  its own ledger receipt. "Reinforced configurable authentication"
  (per-approval re-auth, second factor, remote approvers) is RESERVED
  with reason — meaningful only when a second principal or a remote
  surface exists (E9/E10); faking a password prompt in front of the
  key's owner would be exactly the theater the house forbids. The
  config knob ships as the enabled/disabled flow only.

### FR-COMPAT — the outside does not move

- **FR-COMPAT-1** Byte-for-byte lanes wherever no approval intervenes:
  root authority and unceilinged grants NEVER see the new flow (the E3
  rule stands); with `approvals.enabled=false` the whole stage is
  invisible. ZERO existing tests modified.
- **FR-COMPAT-2** Shared surfaces stay metadata-only (AS-4 mold): the
  preview's raw parameters appear ONLY on the operator's loopback
  surfaces; transcript/SSE/metrics receive finite labels and digests.
- **FR-COMPAT-3** The toll re-measured with the approval branch in the
  hot path (the non-approval lane must not notice it; ceiling 5ms
  stands).
- **FR-COMPAT-4** Migration v7 (approvals table + preview storage) on
  the anti-zombie runner, AS-8 mold re-armed against a hand-built v6.

## Acceptance scenarios (the five blueprint-mandatory + the walls)

- **AS-1 (mandatory — changed parameter invalidates)** Given a
  PENDING_APPROVAL action and its approval request, When any parameter
  of the underlying request changes (a new attempt with different
  args), Then the new attempt gets its OWN digest and request — and an
  approve of the old request against a tampered stored envelope (digest
  mismatch at consume) fails `approval_invalidated` naming the digest;
  nothing executes.
- **AS-2 (mandatory — no double approval)** Given two concurrent
  operators deciding the same request (goroutine race, -race), When
  both approve, Then exactly ONE consumption wins atomically, one
  execution happens, and the loser receives
  `approval_already_decided`; the ledger shows ONE outcome receipt.
- **AS-3 (mandatory — expired never executes)** Given an approval past
  its `expires_at`, When approve is attempted, Then the consume fails
  `approval_expired`, the action closes REJECTED with that reason and
  its receipt — the effect never happens (clock injected, no sleeps).
- **AS-4 (mandatory — no effects before commit)** Given shadow mode and
  given a PENDING_APPROVAL action, When the suite sweeps the executor
  and the tool fakes, Then ZERO executions occurred for either —
  shadow never executes (E1 pin re-asserted) and pending-approval
  actions produce no effect until the approve act itself.
- **AS-5 (mandatory — the preview tells the truth)** Given an approval
  request for a `write_irreversible` tool with declared egress, When
  `korvun approvals show` renders it, Then the output contains: the
  data leaving the system (egress line), the cost/budget line, and the
  reversibility line (class spelled) — asserted verbatim on the CLI
  surface, plus purpose, actor chain, policy pin and expiry.
- **AS-6 (policy change invalidates)** Given a pending approval, When
  the brain's policy pin changes (config reload) before the decision,
  Then approve fails `approval_invalidated` naming "policy".
- **AS-7 (the receipt attests its approval)** Given an approved and
  executed action, When its receipt is verified, Then the receipt is
  canonical v2, its `approval_digest` recomputes from the stored
  approval row, and `receipt verify` passes — while a tampered approval
  row fails `custody_mismatch` naming approval. v1-era receipts in the
  same chain keep verifying under v1 (mixed-era `ledger check` green).
- **AS-8 (migration v7)** The crash mold re-armed against a hand-built
  v6 file: aborted v7 leaves clean v6; next boot completes; fresh files
  land on v7.
- **AS-9 (disabled = today, byte-for-byte)** Given
  `approvals.enabled=false`, When the bounded irreversible attempt
  arrives, Then the outcome is `approval_unavailable` exactly as E3
  shipped it — pinned against the E3 test expectations unchanged.

## Success criteria

- Coverage: ≥90% `internal/action` additions and `internal/brain`
  (house floor for brain), ≥90% sqlite tier, ≥85% cli/app.
- `make quality` green `-race` whole-suite; fuzz smoke extended with
  the approval canonical form (seventh fuzzer).
- The five mandatory tests present and green, mapped in the close.
- §24's sixteen points re-run; toll declared; `go.mod` untouched.
- Exit criterion: an irreversible action cannot execute until the human
  approves exactly the version shown — demonstrated end to end on the
  operator's surface.

## Decisions folded in

1. **The proof of decision is a ledger receipt.** §10.8 asks for
   "signature or proof of decision"; E4 already signs every operator
   act. Inventing a second signature path would duplicate the ink; the
   approval row references the decision act's receipt id, and THAT
   receipt is Ed25519-signed and chain-linked. One signing system.
2. **Invalidation is digest-structural.** §15.3's list (operation,
   resource, protected params, amount, recipient, effect class) is
   already UNDER the E1 action digest by construction; the spec adds
   only the policy-pin comparison. No field-by-field diff machinery —
   the digest IS the law, which is why it was fuzzed from birth.
3. **Expiry is judged at consume time** (the E2 clock-rule mold), not
   by a background sweeper — no new goroutine lifecycle, no timer
   drift; the list surface shows expiry honestly.
4. **PENDING_APPROVAL births no receipt** (not terminal); the terminal
   close does. Matches E4's "every TERMINAL outcome" law exactly.

## [NEEDS CLARIFICATION] — real bifurcations, with votes

- **NC-1 — the inbox surface.** (a) CLI-only in E5; the Console inbox
  moves whole to E8. (b) CLI closes the stage AND a minimal Console
  card ships as the LAST batch behind the Sixth Law (UX-TEMPLATE +
  mockup + Chano's yes before any RED). **My vote: (b)** — the
  blueprint's E5 explicitly lists "Inbox de approvals en la Consola",
  and batching it last means the stage's mandatory tests never wait on
  design; if the mockup round slips, the batch moves to E8 without
  reopening the spec (the CLI already satisfies the exit criterion).
- **NC-2 — what approving does.** (i) Approve EXECUTES the stored
  envelope deferred (one executor path, the human's hand fires the
  effect, the exact-digest guarantee is trivial). (ii) Approve only
  unblocks; the agent must retry (keeps execution in the agent loop,
  but a retry is a NEW action/digest — the approval would need
  digest-of-parameters matching instead of envelope identity, and the
  conversation may be long gone). **My vote: (i)** — it is the only
  shape where "the human approves exactly the version shown" is
  enforced by identity rather than by equivalence argument.
- **NC-3 — the receipt canonical bump.** (α) Receipt canonical v2 with
  `approval_digest` sealed, version-aware verification (mixed-era
  chains like key eras). (β) Approval reference outside the canonical
  form (a plain column). **My vote: (α)** — §10.10 lists approval in
  the receipt for a reason: an unsealed reference could be re-pointed
  after the fact without any check failing, which would hollow AS-7.

## Batching (honest troceo)

1. **Lote 1 — the approval domain (M):** `Approval` + `ActionPreview`
   pure types, canonical forms + digests, the invalidation comparisons,
   expiry rules on the injected clock, birth fuzzing (seventh fuzzer),
   property tests (digest sensitivity per §15.3 dimension).
2. **Lote 2 — persistence v7 + the state machine wakes (L):** the
   three edges + Terminal(REJECTED), migration v7 (AS-8 re-armed),
   approvals/preview storage, one-shot atomic consumption under -race,
   REJECTED/expired receipts in the same transaction (E4 law).
3. **Lote 3 — the gate and the honest observation (M):** config knob,
   `require_approval` replacing the honest no when enabled (E3
   behavior pinned byte-for-byte when disabled), PENDING_APPROVAL
   recording with persisted preview, the model's observation naming
   the request, shadow sacredness pinned.
4. **Lote 4 — the operator decides (L):** `korvun approvals`
   list/show/approve/reject on the sealed-store mold, deferred
   execution through the executor seam, receipt canonical v2 +
   version-aware verify/check, AS-7 sabotage.
5. **Lote 5 — stage close (M):** the five mandatory tests mapped, the
   invalidation sweep (AS-1/6), §24 re-run, toll declared, docs — plus
   the two filed cures as their own piece (fuzz-smoke Makefile budget;
   LiveView SSE frame-order hardening).
6. **Lote 6 — the Console card: MOVED INTACT TO ETAPA 8** (Chano's
   NC-1b resolution, 2026-09-01, exactly as this spec provided): the
   CLI closed the stage and satisfies the exit criterion; the desktop
   inbox ships with E8 under the full Sixth Law (UX-TEMPLATE + mockup
   + the visual yes before any RED). This spec does not reopen.

---

## STAGE CLOSE — 2026-09-01 (lote 5)

Lotes 1-5: IMPLEMENTED + VERIFIED local (the director's bash pending).
The acceptance scenarios, green and mapped to executable tests:

| Scenario | Test(s) | Where |
|---|---|---|
| AS-1 changed parameter invalidates | `TestValidateApprovalBinding_theInvalidationLaw` + `_isAnchoredToTheRealEnvelopeDigest`; execution belt in `TestExecuteApproved_theDigestBelt` | `internal/action/approval_test.go`, `internal/app/approvals_exec_test.go` |
| AS-2 no double approval | `TestApproval_decideOneShotAtomicUnderTheHammer` (24 racers, one winner); flow-level `TestExecuteApproved_raceOverTheFullFlow` + `_neverTwice` | `internal/action/sqlite/approvals_test.go`, `internal/app/approvals_exec_test.go` |
| AS-3 expired never executes | `TestApproval_expiryJudgedAtTheTouch` + `TestExecuteApproved_aNoNeverExecutes` | sqlite + app |
| AS-4 no effects before commit | `TestGate_theArmWakes` (zero executions on park), `TestGate_shadowNeverTouchesTheApprovalPath`, `TestApprovalsReject_closesWithItsInk` (no execution after a no) | `internal/brain/agent_approval_test.go`, `internal/cli/approvals_test.go` |
| AS-5 the preview tells the truth | `TestApprovalsShow_theFullTruthForTheHuman` (egress, cost, reversibility, raw params, THE digest — verbatim on the CLI) | `internal/cli/approvals_test.go` |
| AS-6 policy change invalidates | RE-MAPPED by the C1 consolidation (2026-09-01): the domain unit alone was acceptance theater — nothing wired it. Now the PRODUCTION path: `TestApprovalsApprove_policyChangeInvalidates` + `TestApprovalsApprove_revokedToolNeverExecutes` + `TestApprovalsReject_worksWhateverTheLawDid` (e2e over the CLI), with the stable-law pin pinned by `TestPolicyPin_stableAcrossBoots` / `TestPolicyPin_coversWhatGovernsTheCage` and the executor depth check `TestBuildApprovalExecutor_revokedToolRefusesByName` | `internal/cli/approvals_c1_test.go`, `internal/app/policy_stable_test.go`, `internal/app/approval_executor_revoked_test.go` |
| AS-7 the receipt attests its approval | `TestReceiptsAreBornV2_withTheApprovalSealWhenApproved`, `TestReceiptVerify_approvalCoherence` (approval_mismatch by name), `TestMixedEraChain_verifiesWhole`, `TestReceiptV1_bytesAreFrozenForever` | sqlite + cli + action |
| AS-8 migrations v7/v8 | `TestMigrationV7_freshAndCrash`, `TestMigrationV8_receiptsGainTheApprovalSeal` (both against hand-built prior eras) | `internal/action/sqlite/approvals_test.go` |
| AS-9 disabled = today byte-for-byte | `TestGate_theSacredPin_withoutTheExtensionE3ByteForByte`, `TestApprovalsKnob_absentMeansOffAndNoExtension` + the whole untouched E3 suite | brain + app |

Stage discoveries fixed under the cross-check law, pinned: the E1
crash recovery killed legitimately parked actions (PENDING_APPROVAL
now always survives; APPROVED survives while unclaimed) and flattened
the new REJECTED terminal to FAILED — both found by the lote-4 RED.

THIRD stage discovery (2026-09-01, caught by the cross-check law
BEFORE the ceremony): the production brain identity carried NO effect
ceiling — config-derived authority is ceilingless by sealed E3 design
— so the chat path could never park an action; the gate's whole flow
was proven only through hand-mounted identities. The missing cable
landed as `agent.effect_ceiling` (strict decode; absent = today
byte-for-byte, pinned; unknown class boot-fatal naming the ladder)
with the chat-path parking test the stage lacked: the REAL wire, model
tool-call to PENDING_APPROVAL, end to end.

### E5 CONSOLIDATION — second external audit (2026-09-01, adjudicated)

Five P1 + one P2, each cured reproduction-first (born red from the
auditor's own scenario) in its own commit:

- **C1 — the law that actually invalidates.** The policy pin was
  versioned by the config-load instant (every reboot "a different
  law") and the domain validator was wired to nothing — AS-6 was
  acceptance theater, purged and re-mapped above. Stable identity:
  the pin digests the effective cage-governing content (whole agent
  block + sensitivity + effect registry); `DecideApprovalUnderLaw` is
  the only exported decision surface; execute re-checks at its own
  touch; a revoked tool never rebuilds (`BuildApprovalExecutor`
  membership check).
- **C2 — the preview sealed for real.** Read AND decision recompute
  the stored preview against its pinned digest
  (`preview_digest_mismatch`, named); preview_digest + policy pin
  are decision terms inside `Approval.Digest()` — the v2 receipt
  seals what the human read, end to end.
- **C3 — `korvun approvals execute`.** The resume act for a crash
  between decision and execution; same one-executor path, at most
  one executor start.
- **C4 — the third door.** `OpenOperator`: writing, but no recovery,
  no prune, never a migration of an existing store — those belong to
  the server boot. CLI-beside-a-live-server is a permanent test.
- **C5 — the E6 border, honest.** A crash past the params claim
  closes `OUTCOME_UNKNOWN` (woken from the E6 reserved set as a
  terminal) with its named marker — never a FAILED lie.
  Idempotency/reconciliation stay E6, declared in SECURITY.md.
- **C6 (P2).** Expiry reaches list/show (`EffectiveStatusAt`, the
  read-only door still never writes); the prune knows REJECTED and
  OUTCOME_UNKNOWN with the evidence exemption pinned; params capped
  at 64 KiB at birth.
- **C7.** Over-promising comments aligned to real guarantees (the
  claim hands params to exactly one executor START; effects past a
  crash are OUTCOME_UNKNOWN).
- **C8 — the auditor's cross battery, permanent members:** tool
  revocation (`TestApprovalsApprove_revokedToolNeverExecutes`,
  `TestBuildApprovalExecutor_revokedToolRefusesByName`), policy
  change (`TestApprovalsApprove_policyChangeInvalidates`), allowlist
  change (`TestApprovalsApprove_allowlistChangeInvalidates`, born
  green over the C1 cure — a regression pin), altered preview
  (`TestPreviewSwap_readRefusesByName`/`_decisionRefusesByName`),
  decide→execute crash + resume (`TestApprovalsExecute_*`),
  CLI beside a live server
  (`TestOpenOperator_besideALiveServerTouchesNothingItDoesNotOwn`).

Toll, final: the NEW path costs ~1.02 ms per parked request (preview
assembly + born-whole birth), paid ONLY when an irreversible action
parks; the normal sealed hot path re-measured at ~1.17 ms/op with the
knob code in the tree — the 5 ms ceiling stands with >4× margin, and
the disabled lane is the E3 lane byte-for-byte. §24's sixteen points
re-run green via the whole suite (37 packages, -race); nine fuzzers
in the smoke, FuzzReceiptCanonical walking BOTH wire eras.

Exit criterion, standing and test-proven: an irreversible action
cannot execute until the human approves EXACTLY the version shown —
the approval binds the E1 digest, the preview seals what was shown,
approve executes the stored object by identity after an atomic claim
and a digest belt, and every no (reject, cancel, expiry) closes the
action receipted without any execution path remaining. Pending only:
the director's bash (the Sixth Law).
