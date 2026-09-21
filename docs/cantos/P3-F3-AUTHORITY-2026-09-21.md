# P3-F3 Authority Canto — 2026-09-21

Status: written by the external executor, which stopped before delivery; taken
over, completed, corrected and cured by the delivery session. **IMPLEMENTED.**
VERIFIED is the gate's word and ACCEPTED is the external review's; this canto
claims neither. The section "What is NOT proven" says what no mould reaches.

Branch `p3/fase-3-authority`, base `f5762b8` (master with Piece 3 phases 0, 1
and 2). Action schema 14 → 15. `go.mod` and `go.sum` have no diff.

## Cut and adaptation

The implementation was adapted to the current master tree, not copied from the
2026-09-19 draft. Master already had the phase-0 coordinator, phase-2
`IntentContractV2` and execution bindings, and phase-1 signed ingress evidence.
The coordinator remains the owner of allow, deny, shadow and pending;
`SelectTools` still owns announcement. Authority only narrows what may start.

## Director decisions embodied

- Strict mode requires verifiable persistent storage.
- Siblings spend the intent and every applicable ancestor account.
- A committed start spends once even if it later becomes `FAILED` or
  `OUTCOME_UNKNOWN`.
- The approval document says `máximo` for the pre-attempt remainder.
- Revocation affects starts whose commit is ordered after the revocation.
- The issuer comes from the authenticated actor; an administrative act keeps
  the human operator separate from the grant issuer.
- 2026-09-21: the documents are corrected to what the code does; the five
  supporting moulds that had no equivalent are written in-process and the rest
  are mapped to their real names and levels; AS-AUTH-16 is cured by a generic
  rule and its hostile corpus is FILED, not written.

## What the delivery session found in the delivered tree

The external executor's own verdict named three gaps. Closing them uncovered
more, and each is recorded here rather than smoothed.

**The three named gaps.**

1. *AS-AUTH-05 had no window.* The mould revoked and THEN delegated, in
   sequence, so a door that read the parent before taking write ownership
   would have passed it. Proved, not argued: under the spec's own mutation —
   the parent read moved before the transaction — the delivered sequential
   mould stays GREEN. The new mould parks the delegation in flight at the last
   instant before it asks for write ownership, reads the parent ACTIVE on the
   delegating pool, revokes from a second real pool, confirms the commit from a
   third connection, and only then lets the delegation continue.
2. *Three moulds were incomplete.* AS-AUTH-14 accepted "some error" and attacked
   `VerifyAuthorityChain`, a function PRODUCTION NEVER CALLED; the function and
   the two tests over it are gone, and the mould now enters through the real
   coordinator and the real store with a dispatch counter. AS-AUTH-01 had one
   mutation for twenty dimensions and no row for `parent`. AS-AUTH-02's oracle
   called production's own `Rank()`, and three of its mutations SURVIVED.
3. *Two document claims did not match the code.* The pending snapshot's
   signature does not cover the strict marker — the signed approval-birth event
   does, and only under an activated profile. And the "generation read inside
   the transaction" was a function returning the constant 1 with an error that
   could not happen; it is now the constant `authorityEvidenceEpoch`.

**What closing them uncovered — [PRODUCT], each cured in this tree.**

| # | Defect | Cure | Mould |
|---|---|---|---|
| PF-1 | The approved resume re-resolved authority under the EMPTY conversation, while the pending birth had resolved under the request's own. A request parked under a conversation-scoped binding could NEVER start: it was refused as «authority revoked» when nothing had been revoked. Fail-closed, and a lie in the taxonomy. | The conversation key rides inside the SIGNED pending snapshot and the resume resolves under it. | `TestAuthority_ApprovedResumeKeepsConversationScope` |
| PF-2 | The URL resource matcher normalized the path while it was still percent-encoded, so an encoded segment was never recognized for what a server decodes it into. **Five of eight deceptive URLs probed passed as included before the cure.** | A RULE, not a list: the matcher judges the decoded path cleaned with `path.Clean` (not `filepath`, whose semantics follow the host OS), and REFUSES any URL whose encoded form is not the canonical encoding of its decoded path. | `TestAuthority_URLMatcherRefusesNonCanonicalEncoding` |
| PF-3 | The "canonical" encoding of a grant had TWO forms for the same terms: a nil collection encoded as `null`, an empty one as `[]`/`{}`. The doors normalize an absent per-operation map to an empty one, so a root issued with a nil map was stored and SIGNED under a digest its authenticated act had never covered, and a binding made with the digest of what the operator wrote failed every start as «evidence corrupt» with nothing corrupt. | Absent is written as empty, for every collection. | `TestAuthorityGrantV2_AbsentAndEmptyShareOneDigest`, `TestAuthority_IssuedDigestIsTheAuthorizedDigest` |
| PF-4 | A signed leaf naming an ancestor version that does not exist was reported as «authority missing» — sending an operator to issue authority when the evidence was what was wrong. | An absent ANCESTOR is a broken chain: `ErrAuthorityEvidenceCorrupt`. An absent FIRST link stays «missing». | row of `TestAuthority_SignedInvalidChainStillFails` |
| PF-5 | `authorityGenerationTx(ctx, tx)`: a transactional read in costume. | The constant `authorityEvidenceEpoch`, with the mutable generation FILED by name. | — (the dead error branches are gone) |
| PF-6 | `VerifyAuthorityChain`: exported, documented, tested, and called by nothing. | Removed with the two tests over it. | — |

**What measuring coverage uncovered.** `internal/cli/authority.go` — the whole
operator door of authority — had 0 of 160 statements covered. In `internal/app`
only 51.9 % of the statements this diff adds were covered, and what was not
was the entire strict wiring: no test executed `actionRecorder.StartAuthorization`,
`approvalRecorder.RequestAuthorizationApproval` or
`approvedExecutionStore.StartApprovedAuthorization`. `internal/action/executor`
had dropped to 76.6 % because the strict branches of `Submit`,
`recordAuthorized` and `ResumeApproved` were exercised by nobody. Real tests
were written for all three; see the table below.

## Guarantee → mould → mutation → evidence level

**Ninety-six probing mutations were executed by the delivery session**, each
ALONE on a copy of this tree: the mould run while the mutation was present, its
output captured, the production file restored from the tree, and the copies
compared with the tree after the last one — no mutation survived in them. The
captures are in `docs/superpowers/specs/evidence/p3-f3-authority/delivery-mutations.txt`.
That includes every mutation the external executor had only declared:
**nothing in this table is inherited.**

Four things the mutations found by themselves, recorded rather than smoothed:

- **Three of AS-AUTH-02's mutations SURVIVED the delivered mould** — an absent
  total read as unlimited, an absent effect ceiling read as unlimited, the
  intent's own denials dropped from the inherited set. The mould was extended.
- **Two rows of the refusal taxonomy SURVIVED their first mutation.** The
  intent-approval row was shadowed by the grant's own guard, which refuses first
  with the same sentinel; the no-activation row was shadowed by a clause lookup
  that finds nothing and answers the same. Both rows were re-aimed at the one
  situation where their guard is the ONLY belt, and then reddened.
- **Several reds are by SENTINEL, not by the forbidden state**, because a second
  belt still refuses under another class. They are marked below; a red by
  sentinel proves the guard is reached and named, not that it is the last line.
- **Compile failures are not reds.** Four mutations first "failed" because the
  edit left a variable unused; each was re-aimed until the MOULD reddened.

### Core moulds

| Guarantee | Mould | Mutation executed here | Evidence level |
|---|---|---|---|
| AS-AUTH-01 every dimension rejects widening, by name | `TestAuthority_EveryDimensionRejectsWidening` | 21 mutations, one per dimension, 21 reds | unit |
| AS-AUTH-02 agreement with an independent finite model | `TestAuthority_PropertySubsetAgainstFiniteModel` | 6 of 6 red after the extension | unit; oracle with its own effect ladder |
| AS-AUTH-03 delegation uses the remaining balance | `TestAuthority_DelegateUsesRemainingBudget` | the contract maximum instead of the balance — red, `error = <nil>` | in-process, real store |
| AS-AUTH-04 a per-operation ceiling cannot vanish | `TestAuthority_PerOperationBudgetCannotDisappear` | the inherited limit not materialized — red | unit |
| AS-AUTH-05 delegate and revoke serialize | `TestAuthority_DelegateAndRevokeSerialize` | the parent read before the transaction — red, the child observed born in five row probes | MULTIPLE REAL CONNECTIONS + barrier |
| AS-AUTH-06 every ancestor stays live | `TestAuthority_AncestorRevocationStopsLeaf` | only the leaf's lifecycle judged — red | in-process, real store |
| AS-AUTH-07 siblings share ancestor ceilings | `TestAuthority_ConcurrentStartsShareAncestorBudget` | only the leaf account debited — red, 24 commits against a ceiling of 12 | two real pools |
| AS-AUTH-08 busy is not exhaustion | `TestAuthority_BusyIsNotBudgetExhaustion` | busy mapped to exhaustion — red | real external writer |
| AS-AUTH-09 debit, proof and claim are atomic | `TestAuthority_StartDebitAndApprovalClaimAreAtomic` (rewritten: the probe's OWN error, zero starts, action still APPROVED, the approval still startable) | the debits committed before the probe — red, 4 durable debits | in-process, in-transaction probe |
| AS-AUTH-10 one action id spends once | `TestAuthority_RepeatedActionIDCannotSpendOrStartTwice` (rewritten: eight callers over two real pools) | an existing start answered with success — red on both rows, `started=8` | multiple real connections |
| AS-AUTH-11 the crash boundary preserves truth | `TestAuthority_CrashAfterStartKeepsDebitAndUnknownOutcome` (rewritten: debits counted across TWO recoveries) | 3 red: each probe removed; recovery refunding | COMPILED TEST BINARY IN A SEPARATE OS PROCESS, ended by `os.Exit` at the named probe; NOT a signal from the parent |
| AS-AUTH-12 config keeps tool–channel clauses | `TestAuthority_ConfigMigrationPreservesToolChannelRelation` (rewritten: every tool on every channel against the live `policy.SelectTools`) | the channel union — red, naming the invented pairs | in-process, pure functions |
| AS-AUTH-13 the authenticated actor supplies the issuer | `TestAuthority_IssuerComesFromAuthenticatedActor` | the issuer copied from the parent — red | in-process, real store |
| AS-AUTH-14 signatures do not bless bad chains | `TestAuthority_SignedInvalidChainStillFails` (MOVED to the real door) | 3 red. Signature-only acceptance REACHES THE DISPATCHER; the cycle row reddens by sentinel | in-process, real coordinator + real store |
| AS-AUTH-15 a counter rewrite cannot restore spend | `TestAuthority_CounterTamperingDoesNotRestoreBudget` | the mutable counter trusted — red | in-process, real store |
| AS-AUTH-16 actual arguments bind resources | `TestAuthority_ResourceMatcherBindsActualArguments`, `TestAuthority_URLMatcherRefusesNonCanonicalEncoding` | 3 red | unit |
| AS-AUTH-UI-01 to -03, strict id | `Approvals.test.tsx` | 4 red: block removed; one field un-escaped; a negative remainder accepted; the legacy-only id validator | jsdom |
| AS-AUTH-UI-04 the stored snapshot is what Chromium sees | `approvals-mockup.spec.ts` | «máximo» removed — red in the browser | REAL CHROMIUM over the Go harness, the mutated frontend rebuilt |

### Guards from the external executor's hostile review

All twelve re-executed and red: SIGNED-DEBIT, START-PROOF, APPROVAL-LINK,
APPROVAL-STATE, ADMIN-HUMAN, DETAIL-DOWNGRADE, STRICT-CONSOLE, STRICT-PREP,
DELEGATE-COUNTER-SEAL (by sentinel: «exhausted» where «corrupt» is due),
DELEGATE-ATTENUATION-DOOR, START-CHAIN-DOOR (its mould raised from "some error"
to the named dimension first), START-ACTUAL-USE-DOOR.

### Moulds written by the delivery session

| Guarantee | Mould | Mutation executed here | Evidence level |
|---|---|---|---|
| The issue door reads its intent inside the writer | `TestAuthority_IssueReadsIntentInsideWriter` | red, the grant observed born | multiple real connections + barrier |
| Revocation A in BOTH commit orders | `TestAuthority_StartAndRevokeFollowCommitOrder` | 2 red | multiple real connections + barriers |
| The store owns the applicable leaf | `TestAuthority_StoreOwnsApplicableLeaf` | red | in-process |
| Write ownership precedes protected reads | `TestAuthority_WriteOwnershipPrecedesProtectedReads` | 2 red | SOURCE-LEVEL scan, perimeter stated |
| Protected readers use their transaction | `TestAuthority_ProtectedReadersUseTransactionReceiver` | 5 red | in-process, the real one-connection store |
| A budget account survives a config reload | `TestAuthority_BudgetAccountSurvivesVersionAndReload` | red | in-process |
| Activation is an authenticated one-shot act | `TestAuthority_ActivationRequiresAuthenticatedOneShotAct` | red on every refusal row | in-process |
| An approval does not resurrect authority | `TestAuthority_ApprovedResumeUsesStartAuthorization` | 2 red, both by sentinel | in-process |
| The resume keeps its conversation scope (PF-1) | `TestAuthority_ApprovedResumeKeepsConversationScope` | red with the false «authority revoked» | in-process |
| One digest for one set of terms (PF-3) | the two PF-3 moulds | red, the forbidden state reproduced | unit; in-process |
| The named refusals of the strict start | `TestAuthority_StartRefusalTaxonomy` | 6 red: 4 with a start observed, 2 by sentinel; 2 rows re-aimed | in-process; the unpinned-process row uses a SECOND real store |
| The coordinator's three strict doors | `TestAuthority_StrictImmediateStartDoor`, `…PendingBirthDoor`, `…ApprovedResumeDoor` | 3 red; the resume one RESUMES withdrawn authority | in-process, real coordinator over a FAKE strict store |
| The strict seam production wires | `TestAuthority_StrictAppDoorsEndToEnd` | red: a dispatch AFTER the revocation | in-process; real coordinator, real production adapters, real store. NOT a booted App |
| The operator's CLI door | `TestAuthorityCLI_OperatorDoor`, `TestAuthorityCLI_UsageIsExitTwo` | red on both refusal rows | in-process CLI over a real SQLite file; NOT the compiled binary |
| The administrative CLI door names the human operator | `TestAuthorityCLI_AdministrativeDoorNamesTheOperator` | `admin-issue` routed to the ordinary door — red by a belt worth naming: the one-shot act was recorded over the ADMINISTRATIVE parameters, so the other door refuses it | in-process CLI |
| The ordinary CLI delegation refuses a forged issuer | `TestAuthorityCLI_OrdinaryDelegationRefusesAForgedIssuer` | `delegate` routed to the administrative door — red by sentinel | in-process CLI |
| The bytes a human approved are the bytes that run | `TestAuthority_ApprovedResumeRefusesRewrittenParameters` | the digest comparison dropped — red, the rewritten bytes HANDED OUT and a durable start | multiple real connections |
| The strict boot preparation pins the root and syncs the clauses | `TestPrepareStrictAuthority_PinsTheRootAndSyncsTheClauses` | the activation check skipped — red, by a second belt: the clause sync refuses an unpinned store | in-process, real store |

## What is NOT proven

Every mould above has its captured red. What follows is what NO mould reaches,
by construction:

- `app.Build` under a strict config is exercised by NO mould. FILED: "strict
  boot end to end".
- `ErrAuthorityAmbiguous` is reached by no mould; the only door that writes
  config clauses refuses a second clause per tool first.
- A grant moving from version 1 to version 2: this phase has no door that
  writes a second version.
- Symbolic links: path inclusion is lexical.
- "Never retries dispatch" after a crash: the store has no dispatcher; what is
  proved is that recovery leaves the action terminal.

## Coverage, MEASURED ON THE DIFF

From the profile the final gate wrote (`make quality`, `-race`), statement by
statement, over the NEW files — not over the packages that dilute them:

| What | Covered | Floor |
|---|---|---|
| Authorization domain: `internal/action/authority_v2.go` + `authorization_snapshot.go` | 319 of 349 = **91.4 %** | 90 |
| New schema-15 persistence: `internal/action/sqlite/authority_v2.go` + `config_authority.go` | 1241 of 1460 = **85.0 %** | 85 |
| The operator's door: `internal/cli/authority.go` | 130 of 160 = 81.2 % (was 0 of 160) | — |

**The persistence figure sits ON its floor with no margin**: one statement less
and it is 84.9. It got there with real moulds — the refusal taxonomy, the
rewritten parameters, the signed-snapshot disagreement — and what is left
uncovered is mostly infrastructure-failure branches of the big doors.

Per package, same run, against the project floors (85; 90 for router, envelope,
policy and brain): action 88.5, executor 90.1 (had dropped to 76.6), sqlite
85.6, app 85.0 (was 83.6), **cli 84.9 — one tenth UNDER the floor, declared**,
config 97.3, controlapi 91.7, brain 92.5, envelope 96.8, identity 95.2, router
92.3, policy 100.0. Total internal coverage 88.7 %; the three gate runs of this
session gave 88.6, 88.6 and 88.7 — the total is not deterministic in this
repository, because several moulds exercise real races and real timers.

## FILED by name, for the next phase

- "the mutable authority generation read inside the writer transaction"
- "bounded verification of the approval-birth ledger" — under an activated
  profile every start re-verifies the whole ledger and every `approvals` row,
  LINEAR in lifetime approvals
- "the hostile URL and path corpus for AS-AUTH-16"
- "strict boot end to end"
- every evidence-level upgrade listed in the spec's delivery correction: the
  child OS processes and the restart between processes the commission promised
- a non-strict approval born after activation has no birth event, so the
  profile then reads as corrupt — fail-closed, declared, and worth a door
- `DelegateAuthority` over a parent that does not exist returns a raw
  `sql.ErrNoRows`, an unnamed class
- `internal/action/sqlite` now takes about 300 s locally under `-race`; the
  windows runner has run that package between 2.5 and 4 times slower, against
  a 30-minute ceiling

## Existing approved tests changed

| Test contract | Before | After |
|---|---|---|
| Current schema version | 14 | 15 |
| Webhook phase-1 evidence clock | Fixed at 2026-09-21 14:00 UTC | Current test instant, truncated to one second |

The webhook assertion and production behaviour are unchanged; the fixed clock
had made a one-minute credential expire during the run.

## The six questions, answered for this delivery

1. *Does any sentence claim more than the code and the captures show?* Not
   knowingly, and the two that did were corrected: five sites of the spec, the
   pre-test review and `postreview-red.txt`. Four godocs written by this session
   said «mutation executed» of mutations that had not yet run; they were scoped
   down the moment it was noticed, and carry the executed mutation and its red
   now. Three more godocs placed a red where it did not land, and say today what
   the capture shows.
2. *Does any comment cite a symbol that does not exist?* The spec named
   seventeen moulds that did not exist under those names. It now carries a
   delivery correction mapping every one to what is in the tree.
3. *Does any mould enter through a private function instead of the production
   door?* AS-AUTH-14 entered through a function production never called; it was
   moved. `TestAuthority_RootValidationRejectsEveryWideningDimension` still
   enters through the private `normalizeRootAuthority`, inherited and declared.
4. *Did I cure ONE door of a class that has several?* The stale-read class had
   three doors — delegate, issue, start — and all three now have a barrier
   mould. The "absent means empty" class was cured in the encoder for every
   collection, not only the map that exposed it.
5. *Did any cure enter without its mould and its captured red?* PF-1, PF-2, PF-3
   and PF-4 each have both. PF-5 and PF-6 are removals. No mould in the tree is
   marked as not executed: all 96 mutations ran, one at a time, and each red is
   in `delivery-mutations.txt`. Two of them SURVIVED on the first attempt (rows
   1 and 4 of the refusal taxonomy); those rows were re-aimed and then went red.
   For row 4 the surviving first attempt is kept in the captures under its own
   name; for row 1 only the capture against the re-aimed row was kept.
6. *Have I re-read on disk every file I say I edited?* Each edit was read back
   after the write, by grep against the written text.

## Verification record

Everything below ran in THIS worktree over the final tree.

| What | Result |
|---|---|
| `make quality` (guard, gofmt, goimports, vet, golangci-lint with gosec, tests under `-race`, coverage, fuzz smoke, hook probe, integration probe) | **exit 0**, «Quality gate passed», total 88.7 % |
| `-race` over every touched package | green |
| `govulncheck` over the PRUNED package list (42 packages, never `./...`) | «No vulnerabilities found» |
| `go.mod`, `go.sum` | no diff |
| `make desktop-frontend-check` (typecheck, lint, format, jsdom coverage) | exit 0 — 44 files, 503 tests |
| AS-AUTH-UI-04 in REAL CHROMIUM over the Go harness | 1 passed; and red under its mutation with the frontend rebuilt |
| Probing mutations | 96 executed, 96 captured; the copies identical to the tree afterwards |

Two gates of this session did NOT pass at first, and both are recorded: `gofmt`
refused a file this session had written, and the first run of the batch that
re-executed mutations was stopped by the session's own safety tooling — after
which every mutation was run one at a time, each in its own command, and none
was stopped.
