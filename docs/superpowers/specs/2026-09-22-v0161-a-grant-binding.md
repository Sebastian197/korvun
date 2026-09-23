# v0.16.1 A — the CLI door that ties a signed grant to a binding

Date: 2026-09-22. Status: **DESIGN — RED is not open.** Base: master `32701e0`.

## Goal

Make delegation reachable. Attenuated child grants, shared ancestor budgets and
depth limits are implemented and tested; an operator has no way to put one in
the path of an execution. This is the missing write door, and only that.

## What already exists — the reason this is small

The store side is **done**, and the spec that describes it is already accepted:
`docs/superpowers/specs/2026-09-21-p3-f3-authority.md:152-158` (FR-AUTH-07a)
states that schema 15 extends a binding with an optional exact
`grant_id`/`grant_version`/`grant_digest`, that a non-null triple selects that
one persisted leaf, and that a null triple selects one config clause.

| Piece | Where | State |
|---|---|---|
| The three columns | `internal/action/sqlite/store.go:731-733` (v15 copy) | present, nullable, **no FK, no index** |
| The all-or-nothing guard | `internal/action/sqlite/authority_v2.go:2283-2287` | present, tested |
| The deciding branch | `internal/action/sqlite/authority_v2.go:2060` `if binding.GrantID == ""` | present, both arms tested |
| The chain walk | `authorityChainTx` `:2291`, leaf-digest check `:2089`, subject check `:2128` | present, tested |
| The INSERT that carries the triple | `internal/action/sqlite/intent_v2.go:262` | present; **only ever handed zero values** |
| **The write door** | — | **MISSING. This spec.** |

## FR

- **FR-A-1.** `korvun intent bind` accepts `--grant <grant_id>`. With it, the
  binding is written carrying the exact `grant_id`, `grant_version` and
  `grant_digest` of that grant's current active version. Without it, behaviour
  is byte-identical to today.
- **FR-A-2.** The door reads the grant through the same verification the start
  path uses, and refuses to write a binding the start path would then refuse.
- **FR-A-3.** Re-binding an actor/channel/conversation that already has an
  ACTIVE binding REVOKES the old one and inserts the new one at
  `revision+1`, in ONE transaction. Historical evidence is preserved, never
  overwritten.
- **FR-A-4.** The act is recorded as an authority act, not a plain operator act.

## The design decision, and why

`execution_bindings` carries a partial unique index
(`internal/action/sqlite/store.go:435-437`) over
`(actor_principal_id, channel, ifnull(conversation_id,''))` `WHERE status='ACTIVE'`,
and `PutExecutionBinding` (`internal/action/sqlite/intent_v2.go:262`) is a plain
`INSERT` with no `ON CONFLICT`. So a second bind for the same selector **fails
on the constraint today**. An operator who binds without `--grant` and then
wants the grant has no way forward: there is no `intent unbind`, no
`intent rebind`, and **`BindingRevoked` is declared at
`internal/action/intent_v2.go:406` and set by nothing outside tests**.

Three candidates:

| Option | Verdict |
|---|---|
| Require a prior revoke | **Rejected**: the revoke door does not exist, so this is «no way out» by construction |
| `UPSERT` over the existing row | **Rejected**: it overwrites evidence. A binding is what an execution's authority resolved through; the row that authorised yesterday's action must survive |
| **Revoke the ACTIVE row and insert at `revision+1`, one transaction** | **Chosen.** Preserves history, makes `BindingRevoked` reachable for the first time, and uses `Revision` for the purpose its `CHECK(revision > 0)` was written for |

This needs a new store method. `PutExecutionBinding` is left exactly as it is —
its callers and its plain INSERT are not touched.

## Plan de fallos — the norm of 2026-09-22

### 1 · Consumers of the result

| Consumer | Reads | On a refusal |
|---|---|---|
| The operator at the CLI | stdout line + exit code | nothing is written; the message names the grant and the reason |
| `resolveAuthorityTx` at every later start | the binding row | never sees a half-written triple — the door writes three or none, in one tx |
| `bindingAuthorityTx:2283` | the triple | its all-or-nothing guard is the contract this door must satisfy; the door is the first writer it was ever written for |
| **The human at the approval screen** | `preview.go:90` `grant_id`, via `approvalContext.GrantID = resolved.refs[last]` (`authority_v2.go:1606`) | **CHANGED, and visibly.** Today that field carries a `cfg_…` clause id; after a grant-bound binding it carries the grant id. Declared as a behaviour change, named in §4 |
| `korvun receipt verify` / `ledger check` | historical rows | unaffected: nothing is overwritten |

### 2 · The failure table

| Category | Behaviour | NAMED error | What the operator sees | Mould + mutation | Repair path |
|---|---|---|---|---|---|
| invalid input — `--grant` empty string | refuse before opening the store | usage, exit 2 | the usage line | `TestIntentBind_emptyGrantFlagIsUsage` / drop the guard | retype |
| invalid input — grant id that does not exist | refuse at write time, no row | `ErrAuthorityMissing` | «no such grant» with the id | `TestIntentBind_unknownGrantIsRefusedAndWritesNothing` / skip the read | issue the grant first |
| impossible state — grant REVOKED | refuse | `ErrAuthorityRevoked` | the grant's state | `TestIntentBind_revokedGrantIsRefused` / accept any status | issue a new grant |
| impossible state — grant EXPIRED or not yet valid | refuse | `ErrAuthorityExpired` | the window | `TestIntentBind_expiredGrantIsRefused` / drop the window check | re-issue |
| impossible state — grant's subject ≠ `--actor` | refuse | `ErrIssuerMismatch` | both principals | `TestIntentBind_grantSubjectMustBeTheActor` / drop the comparison | bind the right actor, or delegate to this one |
| impossible state — grant's intent ≠ the bound intent | refuse | `ErrAttenuationViolated` | both intent ids | `TestIntentBind_grantMustBelongToTheBoundIntent` / drop the check | bind the intent the grant was issued against |
| impossible state — an ACTIVE binding already holds the selector | **revoke it and insert at revision+1** | — (success) | «revoked bind_… (rev 1), wrote bind_… (rev 2)» | `TestIntentBind_rebindRevokesAndBumpsTheRevision` / make it a plain insert → UNIQUE violation | — |
| race — two `bind --grant` for one selector | one wins; the loser meets the partial unique index inside its own tx and rolls back whole | `ErrBindingRaceLost`, distinct from a busy store | «another bind took this selector» | `TestIntentBind_concurrentRebindLosesWithoutPartialWrite`, two REAL connections, the loser observed | retry |
| race — the grant is revoked between the read and the commit | the write tx re-reads the grant under the authority write lock; a revocation committed first loses the row | `ErrAuthorityRevoked` | the grant's state | same mould, the revoker as the second connection | issue a new grant |
| crash BEFORE the effect | nothing written; the old ACTIVE binding stands | — | nothing | `TestIntentBind_crashBeforeCommitLeavesTheOldBindingActive`, probe inside the tx | re-run |
| crash AFTER the effect | the new binding is ACTIVE and the old REVOKED, both durable | — | the next command shows it | `TestIntentBind_crashAfterCommitKeepsBothRows`, probe after commit | none needed |
| corruption — the grant row fails its signature or its event replay | refuse; **never** write a binding to an unverifiable grant | `ErrAuthorityEvidenceCorrupt` | «this grant does not verify» | `TestIntentBind_unverifiableGrantIsRefused`, bytes mutated with the seal left stale / skip `readGrantTx` | repair or re-issue |
| dependency that does not answer — store busy | refuse, distinct from a lost race | `ErrAuthorityStoreBusy` (the existing busy class) | «the store is busy» | `TestIntentBind_busyIsNotARaceLoss` / collapse the two classes | retry |
| **an operator with no way out** | **the pre-existing one this door CLOSES**: a binding written without a grant could never be changed. **The one it does NOT close**: a binding whose grant is later revoked keeps pointing at it, and every start refuses `ErrAuthorityRevoked` until the operator re-binds. That is a way out — `bind --grant` again, or `bind` with no flag to fall back to the config clause — and the refusal must SAY so | `ErrAuthorityRevoked` | the refusal names `intent bind` as the repair | `TestStart_revokedGrantRefusalNamesTheRepair` / strip the sentence | re-bind |

### 3 · Neighbours and class siblings

- **`cmd/korvun-desktop/e2e-harness/main.go:1049`** already writes the triple.
  It is a test harness, not a door. It stays; the new door must produce the
  same shape, and the harness becomes a second witness rather than the only one.
- **`ResolveExecutionBinding`** (`internal/action/sqlite/intent_v2.go:280`) is a
  store method with **no production caller**, and its SELECT does not read the
  triple at all — a drifted shadow of `bindingAuthorityTx`. **Named, not cured
  here**: curing it is a separate question (delete it, or make it the one
  reader) and folding it in would widen this diff. Filed.
- **`korvun grant` is the LEGACY v1 noun** (`internal/cli/grant.go`), writing the
  unsigned `grants` table that `resolveAuthorityTx` never reads. An operator who
  runs `grant issue` produces something this door must refuse by name rather
  than accept and strand. A row of the failure table covers it.
- **Grant version 2 is unreachable**: `insertGrantTx` never advances
  `grant_heads.active_version` and `readGrantTx` requires equality
  (`authority_v2.go:1416`). So `grant_version` will be 1 in practice. The door
  writes what it reads rather than a literal 1, so it is correct the day that
  changes. Named.
- **`BindingRevoked` becomes reachable for the first time.** Everything that
  reads bindings filters `status='ACTIVE'`; that must be re-verified, not
  assumed, before RED.

### 4 · Declared and NOT covered

- **The approval screen's `grant_id` changes meaning**, from a `cfg_…` clause id
  to a real grant id, for any actor bound with `--grant`. No screen code
  changes and no layout moves; what changes is the value a human reads. It goes
  in the release notes as a behaviour change. **It is not a new visible piece,
  so the sixth law's mockup gate is not triggered** — but the director sees this
  line before RED opens.
- **No `intent unbind`.** Revocation happens only as the first half of a
  re-bind. An operator who wants «no binding at all» still has no verb. Named,
  not built.
- **`grant_id` gets no foreign key.** Adding one would rewrite a `WITHOUT ROWID`
  table under a migration, for a check the door already performs at write time
  and `authorityChainTx` performs again at read time. The gap is that a row
  written by something other than this door can still name a missing grant —
  which is exactly what `internal/action/sqlite/authority_supporting_test.go:618`
  exploits on purpose.
- **`resolvedAuthority.configGeneration` stays 0 on the grant branch**
  (`authority_v2.go:149`). What `insertAuthorizationStartTx` does with a zero
  generation must be established before RED, and if it is wrong today it is a
  defect this door would EXPOSE rather than cause. Named as an open item.

## The three open items — CLOSED 2026-09-23, adjudicated by the director

| Question | Answer, from the tree | Decision |
|---|---|---|
| What a zero `configGeneration` means on the grant branch | It travels into the SIGNED `AuthorizationSnapshotV1` as `config_generation`. No production reader judges it beyond the snapshot's own signature. And **0 is never a real generation**: `config_authority.go` starts at 0 and increments before writing, so the first is 1 | **0 means «no config clause»**, unambiguously. A sentence in the field's godoc says so, and nothing else moves |
| Whether every reader of `execution_bindings` filters ACTIVE | Exactly TWO production readers, and both do: `ResolveExecutionBinding` and `bindingAuthorityTx` | **Revoke-and-insert is safe**: REVOKED rows are invisible to both from the first day |
| Whether the door belongs under `intent` or `authority` | `recordAuthorityAct` fixes the namespace itself — `operatorAuthenticatedEnvelope(store, "authority", verb, …)` — so the act is recorded as `authority/<verb>` whatever noun the operator types | **`intent bind --grant`**, wrapped in `recordAuthorityAct`. One command binds; the flag says whether it carries a grant. The ledger already records what the act IS |

**A trap this closes by name:** `korvun grant` is the LEGACY v1 noun, writing the
unsigned `grants` table that `resolveAuthorityTx` never reads. A verb added
there would land on the wrong noun and produce something the start path cannot
find. It is not touched.

## Success criteria

- `make quality` green with `-race` over the whole suite, once, at the end.
- Every row of the failure table has its mould and its executed probing
  mutation, red captured.
- The two-connection race row is a real race with real connections, observed —
  not «an outcome is permitted».
- `TestIntentV2CLI_CreateActivateVerifyBind` (`internal/cli/intent_v2_test.go:28`),
  today the only test that runs the real `intent bind` and which asserts nothing
  about `grant_id`, is extended rather than replaced.
- The v0.16.0 Known issue at `docs/releases/v0.16.0.md:170-181`, the operator
  reference in both locales and `SECURITY.md:245` are retired in the same train.
