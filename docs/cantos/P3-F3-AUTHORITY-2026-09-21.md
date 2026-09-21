# P3-F3 Authority Canto — 2026-09-21

Status: written by the external executor, which stopped before delivery; taken
over, completed, corrected and cured by the delivery session; then attacked by
the internal adversary in ONE pass — **VETO MANTENIDO, one P1 and five P2** —
and cured again in this same tree. The adversary has not read those cures.
**IMPLEMENTED.**
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

**Ninety-seven probing mutations were executed by the delivery session**, each
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
| AS-AUTH-UI-04 the stored snapshot is what Chromium sees | `approvals-mockup.spec.ts` | first delivery: «máximo» removed — red in the browser. The scenario was REBUILT after the adversary's pass (production park door, one start spent after the park) and the mutation executed against the rebuilt scenario is the report's M4: red, «máximo 1 inicios». «máximo» removed was NOT re-executed against the rebuilt scenario | REAL CHROMIUM over the Go harness; for M4, the harness compiled from the mutated tree |

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

## The adversary's pass over the complete diff — VETO MANTENIDO, cured here

The internal adversary ran its ONE pass over `f5762b8..12e9d76`. Its verdict —
**VETO MANTENIDO: one P1 and five P2** — was written VERBATIM to
`.claude/adversary/p3-f3-diff-verdict-12e9d76.md` BEFORE any cure began, and that
file has not been edited since. There is no second internal pass, by the
director's rule: what was found is cured in this tree and declared here, and the
cures are judged by the external review at the tag gate. **The adversary has NOT
read these cures.** Every finding was first reproduced in this session's own
hands (the PRE-CURE captures); every cure has its mould and its executed
mutation, 38 mutation captures and 6 pre-cure reproductions in
`docs/superpowers/specs/evidence/p3-f3-authority/adversary-pass-mutations.txt`.

| Finding | What was wrong | Cure | Mould → executed mutation | Evidence level |
|---|---|---|---|---|
| **F1 · P1 · [PRODUCT]** | The operator CLI re-used the phase-1 id of its operator role (`principal_local_operator_role`) with a new KIND. A principal's kind is identity and its row sits under a signed event, so `RegisterIdentity` refused it and EVERY sealed CLI verb died on any profile the base's CLI had touched — non-strict ones included, and `authority activate` with them. | The human operator is a principal of its own, `principal_local_operator`; the earlier row is history and is not rewritten. | `TestOperatorCLI_OpensAProfileTheBaseCLITouched` → the id re-used again: red on every verb with the report's exact message | in-process CLI over a real file; the report's two-binary reproduction was ALSO run by hand in separate OS processes, see the verification record |
| **F2 · P2 · [PRODUCT]** | The operation-use analyzers parsed a JSON object NO shipped tool accepts. Against the real tools a scoped grant refused every start and an unscoped one bound nothing; and a registered analyzer's failure was swallowed into «no analyzer». Every AS-AUTH-16 mould fed the fiction. | The analyzers speak their tools' grammar; an analyzer's failure is its own class and always refuses; a URL path not already clean is refused, not cleaned; a resource scoped by its query includes only that query; a relative `read_file` path is unresolved. | `TestAuthorityUse_AnalyzersSpeakTheRealToolsGrammar` (the REAL tools and the analyzers over the same strings) → JSON grammar again: red on all three tools. `TestAuthority_StartDoorBindsActualResourceArguments` → use check skipped; analyzer failure swallowed. `TestAuthority_URLMatcherJudgesWhatTravels` → path cleaned instead of refused; query comparison dropped | in-process; real tools through their exported constructors, a real file in a real jail, no network; the start door over a real store |
| **F3 · P2 · [TEST]** | «Approved starts atomically … purge» had no mould that could go red: AS-AUTH-09 interrupts BEFORE the purge. The adversary's M1 survived five packages. | — (the product was right) | `TestAuthority_ApprovedStartPurgeIsInsideTheStart` → **M1 verbatim**: red, «budget debits = 4, want 0»; commit before the before-commit probe: red | in-process; an abort TRIGGER as the oracle by impossibility, and an in-transaction probe |
| **F4 · P2 · [TEST]** | The detail moulds and the Chromium scenario were born through `Store.CreateAuthorizedApprovalRequest`, a door production never called, fed a snapshot the TEST wrote; one call to it bricked an activated profile; and the spec's own UI-04 mutation («current live data») survived everything, Chromium included. | The door is GONE (`approvals.go` is back to its base). The harness parks through `ParkAuthorization` over an authority built with exported doors, and spends one start AFTER the park. | `TestApprovalDetail_ShowsTheParkedSnapshotNotTheLiveBudget` → **M4 verbatim**: red, «remaining = 3, want the parked 5». AS-AUTH-UI-04 → **M4 in real Chromium**: red, the browser painted «máximo 1 inicios» where the parked 2 was wanted. The two other detail moulds → signature unverified; absent snapshot read as none | in-process over a real store; REAL CHROMIUM over the Go harness compiled from the mutated tree |
| **F5 · P2 · [PRODUCT]** | The evidence verifiers answered CORRUPT for an intact store that did not answer — a context that was over — and swallowed the cause. The same deadline was «store busy» at a door's edge and «corrupt» one statement later. | `authorityReadFailure`: a failed READ has one class — busy with its cause kept, or the corruption sentinel. Applied to the three verifiers, the grant origin, the config head, the activation reader and the detail's key read. | `TestAuthority_AStoreThatDidNotAnswerIsNotCorruptEvidence` (the report's probe P-G, five readers) → classifier always corrupt: red on all five | in-process; the package's own verifiers inside one live transaction |
| **F6 · P2 · [PRODUCT]** | The strict resume refuses once the INGRESS evidence has expired: five minutes in production, against a one-hour approval window. | **NOT changed, and DECLARED — the director's decision.** Phase 1's accepted spec names this very threat («evidence expires … while approval waits → old authentication starts a new effect») and demands the refusal; overturning an accepted guarantee is not an executor's call. The spec now says what the resume does, with the two production numbers, and the name is FILED. | The row «identity expired» names `ErrIdentityEvidenceExpired` and proves it consumes nothing → expiry judged at the park instant: red | in-process, one real store |
| **F7 · P3 · [DOC]** | Eight sentences false or wider than their wire. | All eight corrected IN PLACE in the spec, each saying what it used to say. F7c was also a product defect: a repeated approved start answered «parameters column empty … action not found»; the repeat check is now the first judgement. | F7c: the repeated start names `ErrActionAlreadyStarted` → check neutralized: red | in-process |
| **F8 · P3 · [TEST]** | Twenty «some error» asserts; AS-AUTH-08 blind to the driver's own busy; AS-AUTH-11 recovering through `Store.Recover`, a door production never called. | Every site names its error. Three doors that answered a bare `sql.ErrNoRows` (revoke, delegate, legacy import) answer `ErrAuthorityMissing`; the signer refusal and a trailing JSON value have a class. `Store.Recover` is GONE: the crash mould recovers through the strict boot's own doors. | `TestAuthority_TheDriversOwnBusyIsClassifiedBusy` (the driver's REAL error) → **M2 verbatim**: red. Absent-row doors; unverified seal; unnamed signer; unclassed trailing JSON ×2; activation check skipped; grant history and grant signature unverified; AS-AUTH-11 refund re-executed through the boot doors — all red | multiple real connections for the busy mould; the rest in-process |
| **F9 · P3 · [PRODUCT]** | `ParseAuthorizationSnapshotV1` returned the ZERO snapshot under a NIL error; the legacy claim handed a strict-born approval its parameters with no debit; `BuildApprovalExecutor` took the config and ignored its strict mode. | The parser refuses by name; the legacy claim reads the ROW's strict marker and refuses (`ErrApprovalRequiresAuthority`) — the four-door enforcement no longer rests on an executor flag alone; the builder honours the config. | one mould each → refusal neutralized / fence neutralized / config ignored: red | unit; in-process over a real store; in-process over the real coordinator |
| **F9, a PREDICTION of the report, EXECUTED here · [PRODUCT]** | The report predicted, without running it, that the ledger verification re-judges the human who activated the profile as ENABLED on every start. Executed: disabling that human afterwards made every start AND the strict boot answer «authorization snapshot corrupt» about a ledger nobody had touched — a profile bricked under a false name. Latent: no production door disables a principal today. | The actor is judged AT THE ACTIVATION INSTANT, which is what it signed. A disable dated at or before the activation still refuses the ledger. | `TestAuthority_TheActivatingOperatorIsJudgedAtTheActivation` → enabled-now demanded again: red, «snapshot corrupt»; the disable ignored: red, «error = <nil>» | in-process; both disables through the store's signed `DisablePrincipal` door |
| **Coverage, by real attacks** | The F5 cures added read-failure branches no mould reaches, and new persistence fell to 1265 of 1489 = 84.96 %, under its floor of 85. | No branch was removed and no threshold moved. Four SIGNED ledgers that no test had made the signature's alone to defend are attacked: the last debit, the counter head, a start proof, a birth event — columns coherent, signature not the key's. And the exported `IdentityRuntime` the harness now uses is pinned against the boot's own registry. Looking at what the app package still missed showed something that mattered more than its figure: the production adapter's copy of the authority snapshot into the API document (FR-UI-01), and the name it gives a snapshot that no longer verifies, were executed by NO Go test — only the browser reached them. | `TestAuthority_EverySignedLedgerRowIsHeldByItsSignature` → each signature check skipped, one at a time: four reds. `TestIdentityRuntime_MintsEvidenceTheBootsRegistryAccepts` → channels dropped: red. `TestApprovalsAdapter_DetailCarriesTheStoredAuthority` (parked by the real coordinator, one start spent after the park, read through `ApprovalsAdapter.Detail`) → authority object dropped: red; corrupt snapshot no longer named: red, «store unavailable» | in-process over a real store; the production adapter, NOT the HTTP handler |

**What this pass did NOT cure, said plainly.**

- The mutation M3 of the report — the write-lock statement removed from
  `beginAuthorityWrite` — still SURVIVES the three barrier moulds (AS-AUTH-05,
  `StartAndRevokeFollowCommitOrder`, `IssueReadsIntentInsideWriter`). They prove
  «the read happens inside a transaction begun after the hook», not «write
  ownership precedes the read»; that sentence rests on the SOURCE-LEVEL scan
  alone. FILED: "a behavioural mould for write ownership before protected reads".
- AS-AUTH-07's race is reached, not forced: no barrier makes two transactions
  overlap. Its captured mutation is deterministic. FILED: "a forced overlap for
  the sibling race".
- The approved path has no crash mould of its own. FILED.
- Rows whose refusal is the DRIVER's and not ours (a closed database) assert the
  driver's text and have no branch of ours to mutate. The «grant terms» tamper row
  is held by two belts (the parser and the signature); no single mutation reddens
  it.
- Two predictions of the report were not executed by anyone and are FILED as
  UNVERIFIED: an imported legacy child under a parent with per-operation limits;
  the CLI syncing config clauses from whatever `--config` it is given. The third
  was executed and cured, see the table.

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
- A REAL MODEL has driven none of this. The argument grammar the analyzers now
  speak is the one the shipped tools parse, pinned against the real tools; what
  a model actually emits for those tools under a strict profile has not been
  observed in this phase.

**A behaviour change of the cures, for whoever turns strict mode on.** Once the
analyzers speak the real tools' grammar, FR-AUTH-03 is closed-world for the
three built-in effectful tools: under a strict profile `read_file`, `http_fetch`
and `webhook_call` start only under an intent — and every grant of the chain —
that LISTS the resources, the data tag and the destinations they may touch. In
the first delivery they started with nothing bound, because nothing their
analyzers were given ever parsed. And a `read_file` path relative to the jail is
refused as unresolved under strict authority: the authority layer does not know
the jail root. Non-strict profiles run no analyzer and are untouched.

## Coverage, MEASURED ON THE DIFF

From the profile the final gate wrote over the CURED tree (`make quality`,
`-race`), statement by statement, over the new files — not over the packages
that dilute them:

| What | First delivery (`12e9d76`) | After the cures | Floor |
|---|---|---|---|
| Authorization domain: `internal/action/authority_v2.go` + `authorization_snapshot.go` | 319 of 349 = 91.4 % | 327 of 355 = **92.1 %** | 90 |
| New schema-15 persistence: `internal/action/sqlite/authority_v2.go` + `config_authority.go` | 1241 of 1460 = 85.0 % | 1272 of 1490 = **85.4 %** | 85 |
| The operator's door: `internal/cli/authority.go` | 130 of 160 = 81.2 % (was 0 of 160) | unchanged | — |

**The persistence figure went UNDER its floor on the way, and that is recorded.**
The F5 cures added read-failure branches that no mould reaches, and the first
gate over the cures measured 1265 of 1489 = 84.96 %. No branch was removed and
no threshold moved: the four signed ledgers and the API door in the table of the
adversary's pass are what brought it back, and each of them is an attack that
was missing, not a line that was missing.

Per package, same run, against the project floors (85; 90 for router, envelope,
policy and brain): action 88.8, executor 90.1, sqlite 85.7, app 85.1, **cli 84.9
— one tenth UNDER the floor, declared, as in the first delivery**, config 97.3,
controlapi 91.7, brain 92.5, envelope 96.8, identity 95.2, router 92.3, policy
100.0, tool 92.9. Total internal coverage 88.8 %. The total is not deterministic
in this repository — several moulds exercise real races and real timers — and
the gates of this session gave 88.6, 88.6, 88.7, 88.7, 88.8 and 88.8; the app
package alone read 84.8, 84.9, 85.0 and 85.1 across them.

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
- FOR THE DIRECTOR'S ADJUDICATION: "the ingress TTL against the approval
  window" — a strict approval can be resumed only while its five-minute ingress
  evidence lives, against a one-hour approval window; phase 1's accepted spec
  demands that refusal (F6)
- FOR A UX DECISION: "the screen's answer to a strict id with no authority
  object" (F7f)
- "authority scope across http_fetch redirects"; symbolic links stay the jail's
- "typed driver-error classification"
- "the config clause carried by an opaque executor plan" and "the cage digest
  recomputed at start" — two wires the spec described and the tree never had
- "a behavioural mould for write ownership before protected reads" (the
  report's M3 survives the three barrier moulds)
- "a forced overlap for the sibling race" and "a crash mould for the approved
  path"
- two UNVERIFIED predictions of the adversary's report, executed by nobody:
  the imported legacy child under per-operation limits; the CLI syncing clauses
  from whatever `--config` it is given. (The third — the activating operator
  re-judged as enabled on every start — was executed, confirmed and cured.)
- `internal/action/sqlite` now takes about 300 s locally under `-race`; the
  windows runner has run that package between 2.5 and 4 times slower, against
  a 30-minute ceiling

## Existing approved tests changed

| Test contract | Before | After |
|---|---|---|
| Current schema version | 14 | 15 |
| Webhook phase-1 evidence clock | Fixed at 2026-09-21 14:00 UTC | Current test instant, truncated to one second |
| `approvalSentinels`, the closed set of `approvals_sentinels_test.go` | 12 sentinels | 13: `ErrApprovalRequiresAuthority` enrolled. The closed-set mould refused the tree until it was, which is what it is for; the earlier 11 → 12 carried the director's authorisation, and this one is DECLARED here for the same eye |

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
   **This answer was incomplete, and the adversary proved it**: two more store
   doors with no production caller were carrying moulds —
   `CreateAuthorizedApprovalRequest` under the detail moulds and the Chromium
   scenario, `Store.Recover` under the crash mould. Both doors are gone and the
   moulds stand on production doors. How the class was searched this time: every
   exported method of the store's new files against its non-test callers.
4. *Did I cure ONE door of a class that has several?* The stale-read class had
   three doors — delegate, issue, start — and all three now have a barrier
   mould. The "absent means empty" class was cured in the encoder for every
   collection, not only the map that exposed it. After the adversary's pass:
   the «did not answer ≠ corrupt» class was cured at EVERY reader that judges
   authority evidence — three verifiers, the grant origin, the config head, the
   activation reader, the detail's key read — not at the three the report
   probed; and the bare `sql.ErrNoRows` at all three doors that returned it.
5. *Did any cure enter without its mould and its captured red?* PF-1, PF-2, PF-3
   and PF-4 each have both. PF-5 and PF-6 are removals. No mould in the tree is
   marked as not executed: all 97 mutations of the first delivery ran, one at a
   time, and each red is in `delivery-mutations.txt` (the count was 96 in the
   first canto; the adversary recounted it by script and it is 97: 69 blocks,
   100 entries, three of which are not a red under mutation and say so in their
   names). The adversary's pass added 38 more, in
   `adversary-pass-mutations.txt`. What has NO mutation is listed by name under
   «What this pass did NOT cure». Two of them SURVIVED on the first attempt (rows
   1 and 4 of the refusal taxonomy); those rows were re-aimed and then went red.
   For row 4 the surviving first attempt is kept in the captures under its own
   name; for row 1 only the capture against the re-aimed row was kept.
6. *Have I re-read on disk every file I say I edited?* Each edit was read back
   after the write, by grep against the written text.

## The known-classes checklist, run over the diff of the cures

(a) empty treated as absent: the legacy-claim fence refuses ANY marker that is
not an explicit 0, and says so; an empty or blank tool argument is unresolved,
never «no restriction». (b) recomputed where a stored value exists: the
activating operator is judged against the activation instant the ledger STORES,
not against now. (c) swallowed errors: none introduced; the one swallowed error
of this pass lived in a probing mutation, made it come back green, and is
recorded. (d) a mould that would pass without its branch: every mould added or
rebuilt has its executed mutation; what has none is listed under «What this
pass did NOT cure». (e) a promise wider than its wire: eight corrected; three
classes derived from text are now SAID to be derived from text. (f) struct
comparison against never-persisted fields: none. (g) a guard by name where it
must be by site: the strict fence moved from an executor flag to the ROW; the
screen's answer to a strict id stays a guard nobody wrote, FILED. (h)
documentary arithmetic: the mutation counts and every coverage figure are
script output, and the one count that was not — 96 — was wrong. (i) either/or
asserts: twenty removed, none added.

## Verification record

Everything below ran in THIS worktree. The rows of the first delivery are kept;
the rows «after the cures» ran over the tree this canto ships with.

| What | Result |
|---|---|
| `make quality` over `12e9d76` (guard, gofmt, goimports, vet, golangci-lint with gosec, tests under `-race`, coverage, fuzz smoke, hook probe, integration probe) | exit 0, «Quality gate passed», total 88.7 %; and again, silently, inside git's own pre-commit hook when `12e9d76` was created |
| `make quality` AFTER THE CURES, over the final code tree | **exit 0**, «Quality gate passed», total 88.8 % |
| `-race` over every touched package, after the cures | one red, and it was a guard doing its job: the closed set of approval sentinels refused a sentinel that had not been enrolled. Enrolled; green |
| `govulncheck` v1.7.0 over the PRUNED package list (42 packages, never `./...`) | «No vulnerabilities found», before the first commit and again before the push |
| `go.mod`, `go.sum` | no diff |
| `make desktop-frontend-check` (typecheck, lint, format, jsdom coverage), after the cures | exit 0 — 44 files, 503 tests |
| AS-AUTH-UI-04 in REAL CHROMIUM over the Go harness, parked through the production door | 1 passed; and RED under the report's M4 over the harness compiled from the mutated tree: the browser painted «máximo 1 inicios» |
| F1, the report's reproduction verbatim | two compiled CLI binaries in separate OS processes, `f5762b8` and the cured tree: the cured CLI opens the profile the base's CLI touched; both principals stored, each as it was signed |
| Probing mutations | first delivery: 97 reds under mutation in 100 entries (counted by script); the adversary's pass: 38 more, plus 6 pre-cure reproductions and the two-binary reproduction of F1; the copies identical to the tree afterwards |

Gates of this session that did NOT pass at first, all recorded: `gofmt` refused
a file this session had written; the first run of the batch that re-executed
mutations was stopped by the session's own safety tooling, after which every
mutation was run one at a time and none was stopped; the closed-set sentinel
guard above; and one probing mutation of the pass (M4) that came back green
because it was aimed at a column that does not exist — re-aimed, then red.
