# Piece 3, phase 3: enforced authority and durable starts

> **Status:** approved for TDD by the director's implementation commission of
> 2026-09-21, subject to the independent pre-test adversarial verdict recorded
> beside this spec.
> Governing source:
> `design-drafts/2026-09-19-pieza-3-identidad-intencion-autoridad-codex.md`,
> phase 3, with the director's storage B, shared-budget A, and revocation A
> decisions. The source file is not present on this worktree; it was read from
> the director's source checkout and is not copied or edited here.
> Current-tree anchors are phase 0's canonical executor, phase 2's signed
> intents and bindings, and phase 1's authenticated ingress at schema 14.
> External-docs note: this phase uses only the Go standard library, the
> existing `database/sql` surface, existing internal packages, and the already
> adopted modernc SQLite driver. It adds no dependency and no new driver API.

## Goal

Make every effectful start in an explicitly strict profile depend on a
verified authenticated principal, one active signed intent, and a complete
active signed authority chain. `StartAuthorization` becomes the durable
commit boundary: it verifies current evidence and scope, consumes the intent
and every applicable grant budget, records the authorization proof, records
the allow decision, and, for an approved request, claims its parameters in one
transaction. Only a committed start hands the executor an invocation
capability. Existing non-strict profiles retain their current behavior.

The phase also makes the approval document show only facts persisted in its
authorization snapshot. Monetary budgets, guaranteed child reservations,
automatic refunds, distributed coordination, external exactly-once, and
compensation remain out of scope.

## Current-tree adaptation

The 2026-09-19 paper assumed an earlier tree. This implementation starts at
`f5762b8` and adapts as follows:

- The schema is 14, not 12 or 13. Phase 3 migrates 14 to 15.
- `executor.Executor` already owns the only physical dispatch and both the
  immediate and approved routes converge at `invokeAndClose`. Authority joins
  that coordinator; it does not create another dispatcher.
- Phase 1 already derives signed identity evidence at every ingress and
  re-verifies it inside the approval claim. Phase 3 consumes that exact
  evidence. The console's optional issuer is the filed sibling: strict mode
  refuses a message when the issuer is absent.
- Phase 2 already has strict signed intent terms, lifecycle evidence, and
  exact execution bindings. Phase 3 adds transaction-local readers rather
  than calling `ResolveExecutionBinding` before the start transaction.
- The existing approval claim already reads the complete approval snapshot,
  verifies identity, and purges parameters atomically. Its transaction grows
  to include authority verification, debits, the start proof, and the durable
  start decision. The old claim is retained for non-strict profiles.
- The existing `authorization_snapshots` table is unused. Schema 15 turns it
  into the append-only, signed pending-display record. A pending request
  persists a non-consuming snapshot in the same transaction as approval birth.
  A confirmed start writes a distinct signed snapshot inside
  `authorization_starts`; it never overwrites what the operator read.
- V1 intents, grants, and `budget_spent` remain readable. They do not become
  authority by being present. Explicit import produces signed v2 authority
  and a signed legacy-baseline debit.

## Functional requirements

- **FR-AUTH-01** `action.AuthorityGrantV2` is a bounded, strict, signed terms
  object. It carries profile, exact intent id/version/digest, issuer, subject,
  optional parent id/version, operations, channels, allowed and denied
  resources, allowed and denied data tags, output destinations, effect
  classes and ceiling, total and per-operation budgets, half-open validity,
  remaining delegation depth, and approval requirement.
- **FR-AUTH-02** `ValidateAuthorityAttenuation` checks every dimension in the
  governing table against both parent and intent. Equal scope is permitted;
  only delegation depth must strictly decrease. It returns
  `*AttenuationError`, whose `Dimension` is stable and which wraps
  `ErrAttenuationViolated`.
- **FR-AUTH-03** Resource inclusion is delegated to a registered matcher by
  resource kind. A missing matcher, an unknown effect class, ambiguous
  normalization, or arguments that cannot be proved inside the grant fail
  closed. One registered operation-use analyzer receives the actual canonical
  arguments and derives all three runtime dimensions: resources, data tags,
  and output destinations. None can be supplied by the caller or copied from
  the envelope. An operation without an analyzer may start only when its
  verified intent and authority terms contain no runtime resource, data, or
  destination restriction; otherwise it returns `ErrAuthorityUseUnresolved`.
- **FR-AUTH-04** `Store.IssueAuthority` starts a write transaction, reads and
  verifies the active intent and actor evidence in that transaction, validates
  the root grant against the intent, and persists signed terms, lifecycle
  event, head, and budget account before committing. The ordinary issue door
  additionally requires the verified actor to equal the signed intent's
  `OwnerPrincipalID`; otherwise it returns `ErrIssuerMismatch`. Only the
  separate administrative door may issue under another owner's intent, and it
  records the human operator and reason.
- **FR-AUTH-05** `Store.DelegateAuthority` acquires write ownership before it
  reads the parent. It reads and verifies the full parent chain, intent,
  lifecycle heads, revisions, and remaining total/per-operation balances in
  the same transaction that persists the child. The ordinary door derives the
  issuer from authenticated actor evidence and requires it to equal the
  parent's subject.
- **FR-AUTH-06** `Store.AdminDelegateAuthority` and administrative issue,
  revoke, and import calls are separate doors. They preserve the requested
  authority subject but record the authenticated operator as the event actor
  and the administrative reason. They never fabricate an agent issuer.
- **FR-AUTH-06a** Authority mutation doors receive an `actor_action_id`, not a
  principal string. Inside their writer transaction they read and verify that
  action's phase-1 signed identity, exact `authority.issue|delegate|revoke|
  import|activate` operation, and parameters digest. The parameters of
  `authority.activate` include profile id and the ordered legacy approval
  manifest digest. `grant_events.actor_action_id` and
  `approval_birth_heads.actor_action_id` are unique, so one authenticated act
  cannot authorize two mutations. The ordinary door derives issuer from that
  verified actor. Every administrative door, including activation, additionally
  requires a currently enabled human operator principal and records
  `administrative=true`, the operator id, and reason. The activation root's
  signed canonical bytes cover that actor action id, operator, reason, profile,
  manifest digest, and activation time. The CLI creates and later closes this
  administrative action separately; the durable mutation points to it rather
  than attributing the act to a supplied principal.
- **FR-AUTH-07** `StartAuthorization` first takes SQLite write ownership, then
  reads and verifies principal, principal binding, execution binding, signed
  intent, the complete authority chain, config snapshot generation, and all
  applicable budget accounts. A revocation committed before this transaction's
  start commit refuses it; a later revocation cannot retract the committed
  invocation capability.
- **FR-AUTH-07a** The applicable leaf is store-owned. Schema 15 extends an
  execution binding with an optional exact `grant_id`, `grant_version`, and
  `grant_digest`. A non-null triple selects that one persisted leaf. A null
  triple selects exactly one active current-generation config clause for the
  bound actor, operation, and channel. Zero matches return
  `ErrAuthorityMissing`; more than one returns `ErrAuthorityAmbiguous`.
  `Envelope.AuthorityRefs` is never a selector: strict start ignores its input,
  derives the verified ordered refs, and overwrites the envelope field before
  persistence. An opaque executor decision plan carries only the current
  config clause identity produced by `SelectTools`; callers cannot construct
  or replace it.
- **FR-AUTH-08** The start transaction verifies operation, actual channel,
  actual resource arguments, data tags, destinations, effect class, approval
  requirement, validity windows, and current configuration policy. Authority
  can only reduce `SelectTools`; it can never turn a shadow or denial into an
  allow.
- **FR-AUTH-09** One committed start inserts one unique
  `authorization_starts` row and one debit per `(action, account,
  operation_key)` for the intent and every grant in the chain. It verifies
  counters against the append-only debit history and updates the total and
  operation counters with bounded predicates before recording the decision.
  A repeated action id returns `ErrActionAlreadyStarted` and adds no debit.
- **FR-AUTH-09a** Strict action ids are store-minted as
  `act3_1_<random>` after immediate `StartAuthorization` or strict pending
  birth has acquired SQLite write ownership. The `1` is the schema-15 evidence
  epoch, not a mutable database generation: in the code it is the constant
  `authorityEvidenceEpoch`, never read from the database, and it does not move.
  FILED for the next phase, by name: "the mutable authority generation read
  inside the writer transaction". Callers and `Prepare` never supply
  the final id. The immediate/pending writer passes the minted id to the
  configured phase-1 resolver, verifies and signs the returned identity
  evidence in the still-open transaction, and then persists the complete
  pending birth or immediate start. Rollback publishes neither id nor
  capability. Approved `StartAuthorization` is deliberately different: it
  reuses the exact parked action id and verifies the stored signed identity and
  pending snapshot; it neither mints an id nor needs the vanished ingress
  capability. The durable `authorization_starts` row survives action pruning,
  so a confirmed id remains exact replay evidence and is rejected as
  `ErrActionAlreadyStarted`.
- **FR-AUTH-10** Shared ancestor accounts make sibling ceilings maxima, not
  reservations. A child ceiling is checked against the parent's current
  remaining balance at delegation and the entire chain is checked again at
  start. A confirmed start is never refunded after `FAILED` or
  `OUTCOME_UNKNOWN`.
- **FR-AUTH-11** Immediate starts atomically persist identity evidence, action,
  decision, authorization snapshot, debits, and start proof. Approved starts
  atomically add the same authority material to the existing claim and purge.
  A failure before commit leaves no debit, no start, and, for approvals, the
  parameters intact. A crash after commit is recovered as
  `OUTCOME_UNKNOWN`, with no automatic invocation retry.
- **FR-AUTH-12** Deny, shadow, and pending decisions do not insert budget
  debits or start proofs. Strict pending birth does verify enough current
  authority to persist the display snapshot, but approval never replaces the
  start-time recheck.
- **FR-AUTH-12a** Pending and start snapshots are separately signed. Pending
  bytes cover snapshot kind, action/approval ids, the conversation key the
  authority was resolved under (so the approved resume re-resolves under the
  SAME scope), requester, actor, exact
  intent, purpose, the verified principal chain, remaining-before value and
  its finite/unlimited kind, and recorded-at time. The separate signed birth
  event covers the strict marker and snapshot digest. Grant versions/digests,
  config generation, debit-set digest, decision digest, and policy pin are
  covered by the signed start proof. The detail API verifies the pending
  signature, stored digest, and cross-links before returning the object. A
  writer who can rewrite bytes and recompute a digest without the profile key
  gets `ErrAuthorizationSnapshotCorrupt`.
- **FR-AUTH-12b** Strict approval ids use the distinct store-only shape
  `apr3_<random>`; legacy and non-strict ids keep the frozen
  `apr_<random>` shape. Schema 15 adds
  `approvals.authority_snapshot_required INTEGER NOT NULL DEFAULT 0 CHECK(...
  IN (0,1))`. Strict approval birth writes `1` in the same transaction as the
  signed snapshot. **The snapshot's signature does NOT cover that marker** — its
  canonical bytes are the fields FR-AUTH-12a lists and nothing else. What seals
  the marker is the signed approval-birth EVENT, whose canonical bytes carry the
  `strict_required` bit beside the snapshot digest; and that event exists only
  under an ACTIVATED profile, so a store with no activation has a marker that
  nothing signed protects. Detail and approved
  start do not trust that marker or namespace as the source of truth. Strict
  mode requires an explicit administrative `authority activate` act before the
  profile can boot. Its writer transaction creates the signed
  `approval_birth_heads` genesis and one signed non-strict
  `approval_birth_events` import for every approval row already present. The
  genesis seals the ordered legacy-manifest digest and produces an
  `activation_digest` that the operator pins in the root strict configuration.
  Strict boot never creates or recreates a genesis: it requires the configured
  digest, verifies that exact signed root and its ancestry to the current
  signed head, and refuses a missing or mismatched ledger. Every later STRICT
  pending birth appends a signed event covering exactly: profile, sequence,
  approval id, action id, action digest, strict-required bit, snapshot digest
  and previous event digest — then advances the signed head. The ACTIVATION
  DIGEST is covered by the signed HEAD, not by each event, and NO birth time is
  signed anywhere in this ledger. Non-strict events are written only by the
  activation import: this phase has no door that appends one afterwards, so an
  approval born later outside the strict door has no event and the profile
  reads as corrupt — fail-closed, and declared rather than discovered.
  After activation, every approval row must have exactly one valid
  matching event. A strict event requires marker `1`, `apr3_`, and its valid
  signed snapshot; a non-strict event requires marker `0`, `apr_`, and no
  snapshot. A missing head, event, snapshot, signature, cross-link, or pinned
  root is `ErrAuthorizationSnapshotCorrupt`. Therefore changing the id and
  marker and deleting the snapshot cannot manufacture a legacy birth, and
  deleting the whole ledger cannot trigger a trusted re-import on restart.
  Birth evidence survives action and approval pruning as an exact signed event.
- **FR-AUTH-13** SQLite schema 14 to 15 creates `grant_versions`,
  `grant_events`, `grant_heads`, `config_authority_snapshots`,
  `config_authority_heads`, `approval_birth_events`,
  `approval_birth_heads`, `budget_accounts`, `budget_counters`,
  `budget_debits`, `authorization_starts`, `authority_write_lock`, and
  `legacy_authority_imports`, and extends `execution_bindings` and
  `authorization_snapshots`, and adds the strict snapshot marker to
  `approvals`. Grant terms and lifecycle evidence reference exact versions.
  Debit, pending-snapshot, and start-proof rows have no
  cascading action foreign key and survive action pruning.
- **FR-AUTH-14** Config authority snapshots are derived per effective
  governance clause. Each clause retains its exact operation-channel relation
  and effective cage digest. Ungoverned tools receive one explicit
  compatibility clause each; shadow and deny clauses create no executable
  authority. IDs use the full terms digest in a new era.
- **FR-AUTH-14a** `config_authority_heads` holds one signed current generation
  per profile and brain principal. Boot and sequential reload persist all new
  immutable clauses and atomically advance that head before the new router is
  exposed. Removal is represented by absence from the head's signed ordered
  clause digest set. Starts, including approved resumes, require the exact
  current head and clause. `MAX(generation)` is never used.
- **FR-AUTH-15** V1 grants remain inspectable and import only through an
  explicit administrative act that validates every v2 dimension. Existing
  `budget_spent` becomes a signed baseline debit for the imported stable scope;
  absence of historical proof is never interpreted as zero consumption.
- **FR-AUTH-16** Root config gains an optional
  `authority: {"mode":"strict","activation_digest":"sha256:..."}` block.
  Absent means compatibility mode. Strict validation requires storage and a
  shaped activation digest. Strict boot fails when its exact signed activation
  root and current birth head, signed root intent, bindings, config snapshots,
  or signing material cannot be verified, and refuses all effectful starts
  without current principal, intent, and authority. Activation is a separate
  authenticated administrative CLI act; boot never signs missing history. Pure
  actions remain under the existing policy path.
- **FR-AUTH-17** The strict console handler refuses before dispatch when no
  phase-1 issuer is wired. It never falls back to an issuer-less envelope.
- **FR-UI-01** `GET /api/approvals/{id}` extends `ApprovalDetail` with one
  `authority` object read from the stored authorization snapshot: requester,
  intent id, intent purpose, ordered principal chain, and the minimum remaining
  budget before this attempt. The object is absent only for legacy/non-strict
  approvals.
- **FR-UI-02** `Approvals.tsx` renders an `AUTORIDAD` section immediately
  after `ORIGEN`. Its exact labels are `QUIÉN PIDIÓ`, `BAJO QUÉ CONTRATO`,
  `CADENA` and `PRESUPUESTO ANTES DE ESTE INTENTO`. Finite remaining budget is
  rendered as `máximo N inicios`; unlimited is `máximo sin límite declarado`.
  Every value passes `escapeUntrusted`.
- **FR-UI-03** An authority object that is present but incomplete, malformed,
  or internally inconsistent makes the detail unreadable. The screen does not
  infer a chain or budget from action fields and never displays current live
  state as if it were the parked snapshot.

## Applicable-authority selection

The authority chain never comes from action-row data.

1. Transaction-local binding resolution selects an exact signed intent.
2. If the binding contains a grant triple, the store reads that exact leaf and
   walks its exact parent versions to the intent. A different, missing, or
   ambiguous version is a refusal.
3. If the binding has no grant triple, the store reads the exact current signed
   config head for the actor's brain and requires one clause id carried by the
   opaque executor plan. It recomputes that clause from action operation,
   channel, effect, and current cage digest and requires membership in the
   head's signed clause set.
4. Strict calls made outside an executor plan can use a persisted exact grant
   binding only. They cannot nominate a config clause or an envelope ref.
5. After verification, the store writes the ordered grant/config refs into the
   action row. Those refs explain the decision; they never choose it.

## Attenuation table

| Dimension | Child requirement |
|---|---|
| Profile and intent | Same profile, intent id, intent version, and intent digest. |
| Issuer | Authenticated actor equals parent subject on the ordinary door. Administrative acts name the operator separately. |
| Operations | Subset of the parent and intent registered triples. |
| Channels | Subset; absence is normalized explicitly and cannot erase a finite parent set. |
| Allowed resources | Each child descriptor is included by the registered matcher under parent and intent descriptors. |
| Denied resources | Superset of every inherited prohibition. |
| Data | Allowed tags and destinations are subsets; denied tags are inherited or extended. |
| Effects | Allowed classes are a subset and the ceiling is no higher. Unknown is invalid. |
| Total budget | Finite child maximum is no greater than current parent balance; unlimited only below unlimited. |
| Per-operation budget | Effective child maximum is no greater than current parent balance. An omitted inherited limit is materialized at the parent's effective remaining maximum; an explicitly larger value is rejected. |
| Start | `child.valid_from >= parent.valid_from` and intent start. |
| End | `child.expires_at <= parent.expires_at` and intent end; unlimited only below unlimited. |
| Depth | `0 <= child.depth < parent.depth`, with the implementation maximum 16. |
| Approval | Required stays required; optional may become required. |

## Transaction and persistence boundaries

`IssueAuthority`, `DelegateAuthority`, revocation, config-head activation,
explicit approval-birth activation, strict pending birth, import, and
`StartAuthorization` each own one database transaction. Schema
15 creates and seeds exactly one row in
`authority_write_lock` with `singleton INTEGER PRIMARY KEY CHECK(singleton=1)`
and `revision INTEGER NOT NULL`. Every protected transaction's first statement
is `UPDATE authority_write_lock SET revision=revision+1 WHERE singleton=1
RETURNING revision`; zero or multiple rows are corruption. The statement is
executed before any protected read and is proved against the repository's
pinned modernc driver with separate real pools. Removing it is a mutation for
each protected door. Errors in the SQLite BUSY family, including WAL snapshot
busy, map to `ErrAuthorityStoreBusy` only. They never map to
`ErrBudgetExhausted`.

Immediate `StartAuthorization` follows this order inside its transaction:

1. acquire write ownership, mint the final action id under the FIXED evidence
   epoch `1` (a constant in this phase — nothing is read; see FR-AUTH-09a), and
   reject a caller-supplied final id;
2. resolve phase-1 identity for that id, then verify and sign the evidence and
   current principal state;
3. resolve the exact execution binding and verify intent terms and events;
4. read every grant link and verify terms, signatures, heads, events, ancestry,
   current config generation, scope, and time;
5. derive actual resources from canonical arguments through registered
   matchers and check policy/approval constraints;
6. verify each counter against its baseline or the indexed signed tail debit;
7. insert unique debits and bounded counter updates for intent and grants;
8. write the distinct start authorization snapshot and signed start proof;
9. write the allow decision and action state, or purge approved parameters and
   verify the approval/action authority again;
10. commit, then and only then return the invocation capability.

Strict pending birth follows the same first two steps before it mints the
store-only approval id. It appends the signed approval birth event, signed
pending snapshot, action, decision, and approval row before one commit. The
resolver callback is local and bounded: it consumes the already authenticated
opaque ingress and performs no I/O. Holding writer ownership across that call
is intentional; it removes the cross-process mint/rotation gap.

Approved `StartAuthorization` acquires the same writer ownership, reads the
exact pending row and signed birth event, uses its existing action id, and
verifies the stored phase-1 evidence, pending snapshot, intent, authority,
budget, and current heads before debit, start proof, claim, purge, and commit.
It never calls the ingress resolver. A successful resume after process restart
has the same action id in approval, identity evidence, pending snapshot,
debits, authorization start, decision, and start proof.

The proof's signed bytes cover action id, identity evidence digest, intent
version/digest, ordered grant version/digest chain, checked head revisions,
config generation, debit-set digest, decision digest, and authorization time.
This detects isolated counter or link tampering; it does not promise detection
after coherent restoration of the whole database and signing keys.

Every debit is itself signed and carries sequence, previous debit digest, and
cumulative spent. A start verifies only the indexed signed tail and counter for
each account, so its BUDGET work is bounded by the chain depth of 16 rather
than lifetime history. Schema indexes support direct `(account_id,
operation_key, sequence)` tail reads.

That bound is about budgets and about nothing else. Under an ACTIVATED profile
every start and every strict pending birth also re-verifies the complete
approval-birth ledger and every row of `approvals`
(`verifyAuthorityActivationTx`), which is LINEAR in the lifetime number of
approvals. This phase does not bound that work and does not claim to. FILED for
the next phase, by name: "bounded verification of the approval-birth ledger".

Raw debit, start, approval-birth, and pending-snapshot evidence survives
ordinary action and approval pruning as exact signed rows. The signed debit
tail and signed counter head keep start verification bounded by the authority
chain rather than lifetime history. This phase does not implement archival or
compaction and makes no bounded-disk-growth claim. It also does not claim to
detect a coherent rollback of the complete database and its signing material.

Budget account identity is stable across versions and reloads. It is the full
digest of `(profile_id, scope_kind, stable_scope_id)`, where `stable_scope_id`
is the intent id, grant id, or config tuple `(brain principal, tool name)`.
Current config already forbids duplicate governance grants for one tool, so
that pair identifies the continuing clause while operation, channel selector,
effect, cage, version, terms digest, config generation, and clause digest stay
mutable signed terms and are deliberately excluded. A renamed tool is a new
scope; narrowing `*` to `telegram` is not. The account identity would also
survive a new VERSION of a grant — but this phase has no door that writes one
(`insertGrantTx` inserts the head and never advances it), so that half is a
property of the identity formula, not something any mould exercises.

## Failure taxonomy and precedence

The start door uses typed errors. Corrupt or unreadable proof wins over a
business denial when the required fact cannot be trusted. After trusted facts
are read, precedence is: repeated action, disabled/missing principal, broken
binding, inactive/expired/revoked intent, broken/cyclic authority chain,
inactive/expired/revoked authority, scope/data/effect/approval mismatch,
budget evidence corrupt, budget exhausted, store busy/infrastructure error.

The stable classes are `authority_missing`, `authority_ambiguous`,
`authority_inactive`,
`authority_expired`, `authority_revoked`, `attenuation_violated`,
`issuer_mismatch`, `principal_missing`, `principal_disabled`,
`intent_binding_missing`, `intent_binding_broken`, `resource_out_of_scope`,
`data_out_of_scope`, `destination_out_of_scope`, `effect_out_of_scope`,
`approval_required`, `authority_use_unresolved`, `budget_exhausted`,
`budget_evidence_corrupt`, `authorization_snapshot_corrupt`,
`action_already_started`, `authority_store_busy`, and
`authority_evidence_corrupt`. No class is derived by matching error text.

## Acceptance scenarios and exact mutations

| ID / test | Observable guarantee | Executed mutation required after GREEN |
|---|---|---|
| **AS-AUTH-01** `TestAuthority_EveryDimensionRejectsWidening` | Every single widened dimension returns `ErrAttenuationViolated`, names that dimension, and leaves zero child rows. | Delete each dimension check separately. |
| **AS-AUTH-02** `TestAuthority_PropertySubsetAgainstFiniteModel` | Exhaustive small universes and chains never disagree with an independent oracle that does not call production normalization, ranking, or matching. | Replace subset with overlap; omit inherited denials; treat absent as unlimited. |
| **AS-AUTH-03** `TestAuthority_DelegateUsesRemainingBudget` | A parent at 8/10 refuses child maximum 3 with `budget_remaining` and no child row. | Compare with contract maximum instead of remaining balance. |
| **AS-AUTH-04** `TestAuthority_PerOperationBudgetCannotDisappear` | An omitted inherited per-operation maximum 2 is materialized as 2; an explicit 3 returns attenuation dimension `budget_operation_remaining`; neither path erases the limit. | Iterate only child-provided limit keys. |
| **AS-AUTH-05** `TestAuthority_DelegateAndRevokeSerialize` | A revocation committed on a second connection before delegation commit yields `ErrAuthorityRevoked` and no child. | Restore the pre-transaction parent read. |
| **AS-AUTH-06** `TestAuthority_AncestorRevocationStopsLeaf` | Revoking a three-link chain ancestor refuses the next leaf start with no debit or dispatch. | Verify only the leaf. |
| **AS-AUTH-07** `TestAuthority_ConcurrentStartsShareAncestorBudget` | At least two real pools race 4N distinct starts across siblings; exactly N commit and 3N return `ErrBudgetExhausted` when no infrastructure error is injected. | Debit only leaves or split check and update. |
| **AS-AUTH-08** `TestAuthority_BusyIsNotBudgetExhaustion` | A held external writer past the retry window yields `ErrAuthorityStoreBusy`, no debit, and no dispatch. | Map busy to budget exhaustion. |
| **AS-AUTH-09** `TestAuthority_StartDebitAndApprovalClaimAreAtomic` | A probe failure after debit but before commit preserves balance and approval parameters and creates no start. | Commit debit or purge separately. |
| **AS-AUTH-10** `TestAuthority_RepeatedActionIDCannotSpendOrStartTwice` | Two callers reusing a committed action id get one start/debit/dispatch and one `ErrActionAlreadyStarted`. | Remove start uniqueness or accept the conflict. |
| **AS-AUTH-11** `TestAuthority_CrashAfterStartKeepsDebitAndUnknownOutcome` | A child process reports reaching named probes `before_commit` and `after_commit_before_return`; the parent kills it at each probe. Before leaves no start; after retains debit, recovers `OUTCOME_UNKNOWN`, and never retries dispatch. | Delete each named probe separately; refund on recovery; enqueue the tool again. |
| **AS-AUTH-12** `TestAuthority_ConfigMigrationPreservesToolChannelRelation` | Clause derivation equals `SelectTools` for distinct-channel and unrestricted tools and creates no cartesian grants. | Reintroduce the global channel union. |
| **AS-AUTH-13** `TestAuthority_IssuerComesFromAuthenticatedActor` | A forged issuer fails ordinary delegation with `ErrIssuerMismatch`; the admin door records the operator as actor. | Copy parent subject into issuer. |
| **AS-AUTH-14** `TestAuthority_SignedInvalidChainStillFails` | Correctly signed cycles, foreign intents, or incompatible parents fail with exact chain errors and no dispatch. | Return allow immediately after signature verification. |
| **AS-AUTH-15** `TestAuthority_CounterTamperingDoesNotRestoreBudget` | Lowering only the mutable counter produces `ErrBudgetEvidenceCorrupt`. | Trust the mutable counter without history reconciliation. |
| **AS-AUTH-16** `TestAuthority_ResourceMatcherBindsActualArguments` | Resource A authority cannot start arguments for B, including path escape and deceptive URL corpus rows. | Use string-prefix matching or caller-declared resource ids. |

Additional UI molds use jsdom and the existing real Chromium approval
harness:

- `TestApprovalDetail_AuthoritySnapshotSignatureIsRequired`: coherent rewrite
  of bytes and digest without the key returns
  `ErrAuthorizationSnapshotCorrupt`; mutation accepts digest-only evidence.
- `TestApprovalDetail_PendingSnapshotSurvivesStartAndPrune`: the exact pending
  bytes remain after start and action prune; mutation overwrites or cascades.
- `TestApprovalDetail_MissingRequiredSnapshotIsCorrupt`: deleting a snapshot
  from a row marked strict returns `ErrAuthorizationSnapshotCorrupt`; mutation
  treats no row as a legacy omission.
- `TestApprovalDetail_StrictNamespaceCannotDowngrade`: changing a strict row's
  id to `apr_...`, changing its marker to zero, and deleting its snapshot still
  returns `ErrAuthorizationSnapshotCorrupt` because the signed birth event does
  not match; mutation trusts the mutable row without the birth ledger.
- `TestAuthority_DeletedBirthLedgerCannotReactivate`: after a child process
  activates strict mode and parks one request, another deletes the birth events
  and head and renames/downgrades the row. A fresh strict boot with the pinned
  activation digest fails with `ErrAuthorizationSnapshotCorrupt` and never
  signs a new genesis. Mutation auto-activates when the head is absent.
- `TestAuthority_ActivationRequiresAuthenticatedOneShotAct`: a child process
  creates and closes one exact `authority.activate` administrative action; a
  fresh process verifies the root names its enabled human operator and reason.
  Missing, forged, wrong-operation, wrong-manifest, reused, or non-human actor
  actions leave no root. Mutation calls activation without verifying and
  consuming `actor_action_id` inside its writer transaction.
- `TestAuthority_EveryMutationDoorConsumesExactActorAct`: table rows for
  ordinary issue/delegate/revoke and administrative issue/delegate/revoke/import
  each exercise missing evidence, invalid signature, wrong operation, wrong
  canonical parameters digest, reused action, and, for administrative rows,
  disabled or non-human actor. Every case returns its exact identity/issuer
  sentinel and leaves grant versions, events, heads, imports, and actor-action
  consumption unchanged. Mutations omit each check separately at every door;
  no shared-helper test substitutes for executing each changed call site.
- `Approvals AS-AUTH-UI-01`: jsdom places `AUTORIDAD` after `ORIGEN`, renders
  the four exact labels and `máximo`; mutation deletes or moves the section.
- `Approvals AS-AUTH-UI-02`: jsdom sends requester, purpose, every chain item,
  and budget strings through `escapeUntrusted`; mutation bypasses one field.
- `Approvals AS-AUTH-UI-03`: jsdom refuses each missing/wrong-typed authority
  field and unsafe integer; mutation falls back to action fields.
- `approvals-mockup AS-AUTH-UI-04`: real Chromium serves the signed snapshot,
  paints its exact escaped text, and makes no network request outside the
  loopback harness; mutation substitutes current live data.

## Supporting molds for requirements outside AS-AUTH-01..16

The governing paper fixes the sixteen destructive core molds; the complete
feature needs these additional named guards. Each is RED before production and
has its own executed mutation.

| Test | Exact result | Mutation |
|---|---|---|
| `TestAuthority_StoreOwnsApplicableLeaf` | Injected or changed `Envelope.AuthorityRefs` cannot select a grant; exact binding/config head wins, zero match is `ErrAuthorityMissing`, multiple is `ErrAuthorityAmbiguous`. | Trust the envelope ref or first matching grant. |
| `TestAuthority_IssueReadsIntentInsideWriter` | A second pool's intent revocation committed before writer acquisition yields `ErrIntentRevoked` and zero grant rows. | Read intent before the transaction or before write ownership. |
| `TestAuthority_OrdinaryRootIssueRequiresIntentOwner` | A valid authenticated non-owner `actor_action_id` returns `ErrIssuerMismatch` and zero grant rows; the administrative door records the enabled human operator. | Omit owner equality after authenticating the actor. |
| `TestAuthority_EveryMutationDoorConsumesExactActorAct` | Issue, delegate, revoke, and import accept only a signed one-shot action for the exact operation and canonical parameter digest; every administrative variant requires an enabled human. | Per door, omit identity, signature, operation, parameter, uniqueness, or human checks one at a time. |
| `TestAuthority_StrictModeRequiresStorage` | Config validation returns the exact `authority.mode` storage error. | Remove the validation. |
| `TestAuthority_StrictEffectRequiresPrincipalIntentAndAuthority` | Separate absent principal, binding, intent and authority rows return their exact sentinels, with zero start/debit/dispatch. | Permit each absence separately. |
| `TestConsole_StrictModeWithoutIssuerRejects` | Strict handler answers 503 `identity_unavailable` and dispatch count zero. | Restore issuer-less dispatch. |
| `TestAuthority_ApprovedResumeUsesStartAuthorization` | Revoked/exhausted authority leaves approved params intact, no debit, no dispatch. | Call the legacy claim from strict resume. |
| `TestAuthority_ApprovedResumeAfterRestartKeepsActionIdentity` | A strict pending request created in process A resumes successfully in process B without ingress; approval, evidence, both snapshots, debits, start, decision, and proof all cross-link the original action id. | Mint a new id or call the ingress resolver on approved resume. |
| `TestAuthority_ExplicitLegacyImportKeepsBaseline` | V1 stays inactive until import; imported account starts at exact `budget_spent`; duplicate import is refused. | Auto-import or baseline zero. |
| `TestAuthority_EvidenceSurvivesActionPrune` | Debit, start proof, and authorization snapshot remain verifiable after action prune. | Add a cascade or delete authority evidence during prune. |
| `TestAuthority_BudgetAccountSurvivesVersionAndReload` | Eight spends under grant v1 remain eight under v2 and under a config generation change that narrows only the channel selector from unrestricted to `telegram`; only two more starts fit a maximum ten. | Include version, terms digest, generation, clause digest, operation, channel selector, effect, or cage digest in account id. |
| `TestAuthority_StartAndRevokeFollowCommitOrder` | Real pools and barriers prove both orders: revoke commit first yields `ErrAuthorityRevoked`; start commit first yields one durable capability and later revocation. | Read revocation before writer ownership or after the start commit. |
| `TestAuthority_ActualUseBindsDataAndDestinations` | Actual argument analyzer derives secret/B and refuses contracts limited to public/A; missing or ambiguous analyzer is `ErrAuthorityUseUnresolved`. | Trust caller fields or skip data/destination checks. |
| `TestAuthority_ConfigHeadRemovalInvalidatesPending` | Reload generation 2 omits a generation-1 clause; approved resume returns `ErrAuthorityRevoked`, retains params, and never uses `MAX(generation)`. | Ignore the signed head or use maximum generation globally. |
| `TestAuthority_WriteOwnershipPrecedesProtectedReads` | A trace/probe for issue, delegate, revoke, import, config activation, pending birth, and start sees lock update before any protected SELECT. | Remove or move the lock statement after a read. |
| `TestAuthority_ProtectedReadersUseTransactionReceiver` | Every protected door completes through the real one-connection store and sees a concurrent committed change only according to writer order. | Change each protected `tx.Query*` call site to `s.db.Query*`; the focused row must time out/red. |

Every row is exercised under `-race`; the concurrency, busy, revocation-order,
and crash rows additionally use independent SQLite pools or child processes as
stated. The canto records an evidence level for every core and supporting mold.

### Delivery correction: what is in the tree, under which name, at which level

The two tables above are the COMMISSION. They name seventeen supporting moulds
that the delivered tree did not contain under those names, and several at an
evidence level — a child OS process, a restart between processes — that was
never built. A spec that names a mould which does not exist sells a guarantee
the tree cannot show, so this section says what is actually there. By the
director's decision of 2026-09-21 the moulds with NO equivalent were written
in-process, the rest are mapped to their real names and real levels, and every
upgrade the commission promised and the tree lacks is FILED by name.

| Commissioned name | In the tree as | Real evidence level | FILED |
|---|---|---|---|
| `TestAuthority_StoreOwnsApplicableLeaf` | same name (written at delivery) | in-process, one real store | `ErrAuthorityAmbiguous` is reached by NO mould: the only door that writes config clauses refuses a second clause per tool first |
| `TestAuthority_IssueReadsIntentInsideWriter` | same name (written at delivery) | multiple real connections + a real barrier | — |
| `TestAuthority_StartAndRevokeFollowCommitOrder` | same name (written at delivery) | multiple real connections + real barriers, BOTH orders | — |
| `TestAuthority_WriteOwnershipPrecedesProtectedReads` | same name (written at delivery) | SOURCE-LEVEL scan, perimeter stated in the mould | a statement trace from the driver; the scan does not follow calls into helpers |
| `TestAuthority_ProtectedReadersUseTransactionReceiver` | same name (written at delivery) | in-process, the real one-connection store, deadline plus watchdog | the per-call-site mutation matrix the commission asks for; five representative readers were mutated |
| `TestAuthority_BudgetAccountSurvivesVersionAndReload` | same name (written at delivery) | in-process, one real store | the GRANT half: this phase has NO door that writes a second version of a grant, so "eight spends under grant v1 remain eight under v2" cannot be exercised |
| `TestAuthority_ActivationRequiresAuthenticatedOneShotAct` | same name (written at delivery) | in-process | the child OS process that creates and closes the act, and the fresh process that verifies it |
| `TestAuthority_ApprovedResumeUsesStartAuthorization` | same name (written at delivery), plus `TestAuthority_StrictApprovedResumeDoor` (the coordinator, over a fake store, with a dispatch counter and the legacy claim counted at zero) and the resume stage of `TestAuthority_StrictAppDoorsEndToEnd` (real adapters, real store) | in-process | the same through a booted App |
| `TestAuthority_ApprovedResumeAfterRestartKeepsActionIdentity` | `TestAuthority_StrictPendingDoesNotSpendAndApprovedResumeReusesAction` | in-process, SAME process | the restart between two OS processes |
| `TestApprovalDetail_StrictNamespaceCannotDowngrade` | `TestApprovalDetail_ActivatedLedgerPreventsLegacyDowngrade` | in-process | — |
| `TestApprovalDetail_PendingSnapshotSurvivesStartAndPrune` | partly `TestAuthority_StrictPendingDoesNotSpendAndApprovedResumeReusesAction` (the pending row survives the start) and `TestAuthority_EvidenceSurvivesActionPrune` (a START snapshot survives the prune) | in-process | the exact pending BYTES compared before and after a prune |
| `TestAuthority_DeletedBirthLedgerCannotReactivate` | `TestAuthority_ActivationRootIsPinnedAndNotRecreated` | in-process | the child processes and the fresh strict boot |
| `TestAuthority_ConfigHeadRemovalInvalidatesPending` | `TestAuthority_ConfigHeadRemovalInvalidatesFutureStart` | in-process; a FUTURE immediate start, refused as `ErrAuthorityMissing` | the PENDING request resumed after the reload |
| `TestAuthority_ExplicitLegacyImportKeepsBaseline` | `TestAuthority_LegacyImportIsExplicitAndCarriesConsumedBaseline` | in-process | — |
| `TestAuthority_OrdinaryRootIssueRequiresIntentOwner` | rows of `TestAuthority_MutationDoorsRejectMalformedActorsAndBudgets` | in-process | a mould of its own with its own mutation |
| `TestAuthority_EveryMutationDoorConsumesExactActorAct` | rows of `TestAuthority_MutationDoorsRejectMalformedActorsAndBudgets` and `TestAuthority_MutationDoorRejectsLegacyIdentitySnapshot` | in-process | the full door × fault matrix with one mutation per changed call site |
| `TestAuthority_StrictEffectRequiresPrincipalIntentAndAuthority` | `TestAuthority_StartRefusalTaxonomy` (written at delivery: one named sentinel per refusal, nothing started, nothing spent), rows of `TestAuthority_BindingAndChainFailureStates` and `TestAuthority_StartAndParkRejectIncompleteInputsWithoutWrites`, and `TestAuthority_StrictImmediateStartDoor` for the coordinator's side | in-process | — |
| `TestAuthority_ActualUseBindsDataAndDestinations` | `TestAuthority_StartDoorBindsActualResourceArguments` (resources, at the door) and `TestAuthority_ActualUseFailsClosedOnlyWhenTermsNeedResolution` | in-process | data tags and destinations refused AT THE START DOOR |

Moulds exist that the commission never asked for, because the delivery session
found what they pin: `TestAuthority_ApprovedResumeKeepsConversationScope`,
`TestAuthority_URLMatcherRefusesNonCanonicalEncoding`,
`TestAuthorityGrantV2_AbsentAndEmptyShareOneDigest` and
`TestAuthority_IssuedDigestIsTheAuthorizedDigest` (four product defects, see
the canto); `TestAuthority_StrictAppDoorsEndToEnd`,
`TestAuthority_StrictImmediateStartDoor`, `TestAuthority_StrictPendingBirthDoor`
and `TestAuthorityCLI_OperatorDoor` (strict wiring that no test executed at
all). The canto says, mould by mould, which of these has had its probing
mutation executed and which has not. And AS-AUTH-14 was MOVED: the commissioned mould attacked
`VerifyAuthorityChain`, a function production never called; the function is gone
and the mould now enters through the real coordinator and the real store.

## Existing approved tests that may change

Only the tests named by the governing paper may change expectations:

- `TestAttenuation_everyWideningDimensionIsRejectedAndNamed`
- `TestAttenuation_propertyAgainstTheOracle`
- `TestAttenuation_unlimitedSemantics`
- `TestConsumeBudget_concurrentHammerNeverExceedsTheLimit`
- `TestDelegate_parentMustBeStoredAndDelegable`
- `TestGrant_issueRequiresExistingIntent`
- `TestDerivedConfigGrant_fromGovernance`
- `TestIdentity_ungovernedRecordsNoAuthorityRefs`
- `TestGrantDelegate_attenuatedChildPersists`

Claim, sentinel, close, phase-0 coordinator, phase-1 identity, and phase-2
intent tests retain their existing assertions. New authority checks extend
their transactions without weakening the old belts. The canto lists every
approved test actually edited with its before/after expectation; an empty list
is valid and preferred.

## Pre-test attack matrix

| Attack | Dangerous false claim | Forced observation |
|---|---|---|
| Forge sender/issuer fields | Caller chooses who authorized the action. | Phase-1 signed evidence and ordinary issuer mismatch; zero grant/start. |
| Revoke parent between caller read and store write | Stale authority delegates or starts. | Second real pool commits before protected transaction; no child/debit. |
| Present a validly signed invalid leaf | Signature alone is authorization. | Cycle, foreign-intent, wrong-parent corpus; chain-specific refusal. |
| Race siblings at last ancestor unit | Each leaf spends a private copy. | Multiple real pools and distinct action ids; exact N committed. |
| Hold SQLite write lock | Infrastructure failure is called policy denial. | Busy window exceeded; typed busy; zero debit/effect. |
| Fail after debit or purge | Partial commit loses budget or approval bytes. | In-transaction probe; rollback observed from another connection. |
| Crash around commit | Start is refunded or retried. | Child process killed before/after commit; durable proof and recovery state. |
| Reuse action id | One request spends or invokes twice. | Concurrent duplicate id; unique start and dispatch counter. |
| Lower counter | Mutable projection restores budget. | Debit-history mismatch; corruption sentinel. |
| Path/URL textual confusion | Prefix looks in-scope while actual target is not. | Independent canonical-target corpus; no invocation. |
| Flatten config clauses | One tool inherits another tool's channel. | Exhaustive operation-channel matrix against `SelectTools`. |
| Approve stale pending request | Human approval resurrects revoked authority. | Revocation before claim; params retained and no start. |
| Malformed UI snapshot | Screen invents provenance. | API/UI refuse incomplete object; no fallback to action fields. |
| Swap envelope authority ref | Caller selects a broader valid grant. | Exact binding/config head remains authoritative; stored refs are overwritten. |
| Rewrite coherent pending snapshot | Operator sees invented provenance. | Signature verification fails by name even when digest is recomputed. |
| Rename, downgrade, and delete strict pending snapshot | Corruption is presented as a legitimate legacy omission. | Signed approval-birth event and head no longer match the row; detail returns snapshot corruption. |
| Remove config clause on reload | Pending request retains withdrawn config authority. | Signed per-brain head moves; resume refuses and retains params. |
| Misdeclare data/destination use | Contract checks caller narration. | Registered analyzer derives actual use from canonical arguments. |

## Success criteria

- All sixteen molds are captured RED before production implementation and
  GREEN afterwards.
- Every exact mutation above, plus each UI mutation, is executed and captured
  red after GREEN.
- Authorization-domain coverage is at least 90 percent and new schema-15
  persistence coverage is at least 85 percent.
- Focused suites and the whole suite pass under `-race`.
- `gofmt`, `goimports`, `golangci-lint`, `gosec`, `govulncheck`, godoc checks,
  desktop typecheck/lint/format/coverage, jsdom, real Chromium, and
  `make quality` are green.
- `go.mod` and `go.sum` have no diff. No dependency is added.
- The canto maps every guarantee to its mold, executed mutation, evidence
  level, command output, exclusions, and exact claim width.

## Decisions folded in

1. Strictness is an explicit root config mode. Inferring it from the presence
   of storage would silently break existing stored profiles.
2. Only effectful actions require authority. This follows the director's exact
   boundary and does not widen the change to pure reads.
3. A pending snapshot is a non-consuming authorization preview, not a promised
   future start. Start-time authority writes a separate record after the
   current-chain recheck and preserves the pending snapshot unchanged.
4. Config snapshots are signed history while config and `SelectTools` remain
   the live source and first reducer.
5. Shared budget uses ancestor accounts and child ceiling accounts. Issuance
   does not reserve capacity.
6. Existing v1 contracts are history, not implicit authority. Import is an
   administrative signed act with a legacy consumption baseline.
7. Start proof and debit history survive action retention by construction,
   without a cascading action foreign key.
8. Full database-and-key rollback is outside the tamper guarantee. Isolated
   projection changes are detected.
9. Applicable authority is selected by exact execution binding or the current
   signed config head. Envelope authority refs are output evidence only.
10. Pending and start snapshots are separate signed records; neither rewrites
    the other.
11. `authorization time` is the instant checked inside the transaction. The
    commit order is established by SQLite serialization, not by pretending a
    pre-commit timestamp observed the future commit.
12. Ordinary root issuance belongs to the signed intent owner. Administrative
    issuance under another owner is a separate human act with provenance.
13. Budget accounts identify stable scopes, never versions or digests.
14. Exact replay protection is the durable `authorization_starts` primary key.
    ID mint and pending/start persistence share one SQLite writer, and start
    evidence is excluded from ordinary action pruning.
15. A signed approval-birth ledger classifies every approval after strict-mode
    activation. Renaming a row and deleting its mutable marker and snapshot
    cannot make a strict birth look legacy.
16. The activation root is an explicit administrative act pinned outside the
    database in strict config. Boot verifies it and never manufactures missing
    history, so total ledger deletion is a refusal rather than a new baseline.

## `[NEEDS CLARIFICATION]`

None. The director's commission fixes storage B, shared-budget A, revocation A,
the strict execution boundary, and the approval document content. The explicit
strict-mode config shape and pending-preview wording are implementation-level
adaptations recorded above and remain fail-closed.
