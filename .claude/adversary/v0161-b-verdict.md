# VETO MANTENIDO

Hostile review of the v0.16.1 B diff against base `32701e0`, worktree
`/Users/sebastianmorenosaavedra/Desktop/korvun-v0161b.nosync`.
Date: 2026-09-23.

Every claim below that carries a COMMAND block was executed. Mutations were run
on a COPY of the tree at
`/private/tmp/claude-501/-Users-sebastianmorenosaavedra-Desktop-korvun-nosync/dbc7d3ca-b129-478c-93e3-27f6702efb3f/scratchpad/v0161b-adv/{tree,base}`.
The audited worktree was NOT modified: `git status --short` at the end is
byte-identical to the one at the start (nine `M`, three `??`). The ONLY write I
made inside the audited worktree is this verdict file, which the mandate
designated.

Baseline, both green before any mutation:

```
$ cd /Users/sebastianmorenosaavedra/Desktop/korvun-v0161b.nosync && go vet ./internal/...
VET EXIT: 0
$ go test ./internal/...
ok  github.com/Sebastian197/korvun/internal/action/sqlite  71.611s
ok  github.com/Sebastian197/korvun/internal/app            84.352s
ok  github.com/Sebastian197/korvun/internal/cli            21.028s
   (39 packages, all ok)
$ cd cmd/korvun-desktop/frontend && npx vitest run
 Test Files  44 passed (44)
      Tests  505 passed (505)
$ gofmt -l internal/ cmd/korvun-desktop/   ->  (empty)
$ golangci-lint run ./internal/... ./cmd/...   ->  (no output, clean)
```

Nothing reddens. That is the problem with four of the six cures, not the proof
of them.

---

## P1-1 · `approvals list` accuses a HEALTHY store of corruption and exits non-zero

**Claim.** Cure 4 compares five separate queries against a COUNT taken
afterwards, with no transaction and no snapshot. A row parked by the live
server while the operator's read-only CLI is walking the statuses is counted
but not listed, and the command prints a sentence asserting a specific fact
that is false — "carry a status outside the five this command knows" — and
returns 1.

**Wire.** `/Users/sebastianmorenosaavedra/Desktop/korvun-v0161b.nosync/internal/cli/approvals.go:82-131`.
Five `store.ListApprovals` calls in a loop (lines 84-107), then
`store.CountApprovals(ctx)` at line 120, then `if stored > total` at line 125.
Six independent statements. No `BeginTx` anywhere in `approvalsList`. The store
is opened by `openOperatorStore` → `actionsqlite.OpenReadOnly`
(`internal/cli/intent.go:90-96`), which pins `db.SetMaxOpenConns(1)`
(`internal/action/sqlite/store.go:1914`) — one connection, six autocommit
reads, no shared snapshot. This is the repository's own named pattern: "a
snapshot broken between the reads of one command".

**REPRODUCTION (executed).**

1. Park one approval through the production door.
2. From a SECOND real connection to the same file, insert valid `PENDING`
   approval rows in a loop (200 µs cadence) — the row shape a live server's
   park leaves.
3. Run `korvun approvals list --config <cfg>` repeatedly.
4. Assert afterwards that ZERO rows carry a status outside the five.

```
$ go test ./internal/cli/ -run 'TestADV_approvalsList_falseAlarm' -count=1 -v
  runs=44  false alarms=1  rows=4  rows with a status outside the five=0
  the command said: korvun approvals list: 1 approval row(s) carry a status
  outside the five this command knows and are NOT listed above; the store holds
  3 in total
--- PASS
```

One false accusation in 44 runs, on a store with four rows and zero corruption.
On a busy server the rate is higher, not lower: the window is not the single
statement between the loop and the count — it is the WHOLE command after the
first status query, because a row parked after the `PENDING` query is missed by
that query and counted by `CountApprovals`.

**Outcome it should have.** Either the six reads share ONE transaction (a real
snapshot), or the difference is re-established before it is named — re-query
the specific ids, or compare `SELECT status, COUNT(*) GROUP BY status` in the
same read that lists. A letrero that asserts "these rows carry a status outside
the five" must be derived from a read that saw those rows, not from subtracting
two numbers taken at different instants. Exit code 1 on a correct store turns
every monitor wrapping this command into a false corruption alert.

**Aggravating.** The canto §2 names this exact risk — "`approvals list` | A
false alarm on a healthy store" — and answers it with "The control lists two
rows and exits 0 before anything is rewritten". That control exercises a
QUIESCENT store. It does not touch the failure mode it is offered against. The
risk was identified and then closed with evidence that does not address it.

**Also silent in the other direction.** A row deleted by retention during the
window gives `total > stored`, which fires nothing — the command has listed a
row that no longer exists and says so to no one. Not introduced here, but the
new comparison declares itself the accounting of the table and only accounts in
one direction.

---

## P1-2 · A full disk is published to the operator as permanent evidence corruption

**Claim.** Cure 2 (`purgeWriteFailure`) inverts the default for the claim's
purge: everything that is not busy and not a dead context becomes
`ErrApprovalEvidenceCorrupt`. Two ordinary operational failures —
`SQLITE_FULL` (13) and `SQLITE_READONLY` (8) — now reach the desktop screen as
"what no longer verifies is the stored evidence, and that is permanent".

**Wire.**
- `internal/action/sqlite/approvals_v15.go:414-440` — `purgeWriteFailure`.
- `internal/action/sqlite/approvals_v15.go:862-868` — the purge `ExecContext`
  and its wrap.
- `internal/action/sqlite/store.go:1558-1561` — `isBusyClass`, which matches
  ONLY `"SQLITE_BUSY"` and `"database is locked"`.
- `internal/action/sqlite/approvals_v15.go:61-64` — the sentinel's own godoc:
  "ErrApprovalEvidenceCorrupt is any belt or evidence parse refusing …
  **Permanent: no profile repairs it.**"
- `internal/app/approvals_adapter.go:670-676` — `nameClaim` short-circuits the
  corrupt sentinel to `nameTouch(..., sealed=true)` WITHOUT the `ReReadParams`
  re-read.
- `internal/app/approvals_adapter.go:675` → `controlapi.ErrApprovalDecidedEvidenceBad`
  (`internal/controlapi/approvals.go:129`), wire text at
  `internal/controlapi/approvals.go:220-221`.
- `cmd/korvun-desktop/frontend/src/views/approvalsText.ts:65-66` — the screen
  literal.

**REPRODUCTION (executed, production door).**

1. Park and approve a request on a real store.
2. Force a REAL driver write failure on the purge's own UPDATE
   (`PRAGMA query_only = ON` for `SQLITE_READONLY`; `PRAGMA max_page_count = 1`
   for `SQLITE_FULL`; the store pins `SetMaxOpenConns(1)`, so the pragma binds
   the connection the claim uses).
3. Call `ClaimApprovalParamsUnderDigest` and read the class.

```
$ go test ./internal/action/sqlite/ -run 'TestADV_purge_operational' -count=1 -v
  [SQLITE_READONLY (a read-only mount / query_only)]
    err=action/sqlite: purge claim "apr_95b2…": action/sqlite: the approval's
        stored evidence no longer verifies: attempt to write a readonly database (8)
    is EvidenceCorrupt=true  is Unreadable=false
  [SQLITE_FULL (a full disk)]
    err=action/sqlite: purge claim "apr_b7b4…": action/sqlite: the approval's
        stored evidence no longer verifies: database or disk is full (13)
    is EvidenceCorrupt=true  is Unreadable=false
```

4. Follow the class to the operator surface:

```
$ go test ./internal/app/ -run 'TestADV_purgeCorruptClass' -count=1 -v
  AFTER  the cure: nameClaim -> approvals: the sealed decision's evidence no
                   longer verifies (decided_evidence_bad=true)
  BEFORE the cure: nameClaim over the transient class re-reads the row
                   (panic on the nil store: nil pointer dereference)
```

The second line is the oracle: with the OLD class `nameClaim` reaches
`a.store.ReReadParams` (it nil-panics on the zero-value adapter); with the NEW
class it returns without touching the store. The cure does not only rename the
failure — it DELETES the re-read that distinguished `params_held` from
`params_gone`.

**What the operator is told, on a full disk**
(`approvalsText.ts:66`): *"La decisión quedó registrada y sellada, con su
recibo, y esta ejecución no llegó a salir. Lo que ya no verifica es la evidencia
guardada. **Esto es permanente.** … Mira el libro antes de repetir nada."*

A full disk is repaired by deleting a file. A read-only remount is repaired by
`mount -o remount,rw`. Neither is permanent and neither is evidence corruption.
The project targets "a Raspberry Pi to the cloud" (CLAUDE.md), where a failing
SD card remounting read-only is a routine event, not an attack.

This is the exact harm the sibling this cure claims to copy already forbids, in
writing, at `internal/action/sqlite/authority_v2.go:697-702`: *"Calling an
intact book corrupt because a deadline landed one statement after the write
lock sends the operator after tampering that is not there."*

**Outcome it should have.** Name the DETERMINISTIC classes and fall to the
transient one for the residue — a constraint failure / `RAISE(ABORT)` is
identifiable (`constraint failed`, extended code 1811), a write error is not.
Or keep the inversion and stop routing it into a sentinel whose own godoc says
"Permanent: no profile repairs it" and whose screen literal says "Esto es
permanente".

**Two further properties of the classifier, captured:**

```
$ go test ./internal/action/sqlite/ -run 'TestADV_purgeWriteFailure_deadContext' -count=1 -v
  live ctx + RAISE(ABORT) -> corrupt=true  unreadable=false
  dead ctx + RAISE(ABORT) -> corrupt=false unreadable=true
  database table is locked (6) (SQLITE_LOCKED)   purgeWriteFailure=CORRUPT | authorityReadFailure busy=true
  interrupted (9)                                purgeWriteFailure=CORRUPT | authorityReadFailure busy=true
  disk I/O error (10)                            purgeWriteFailure=CORRUPT | authorityReadFailure busy=false
  locking protocol (15) (SQLITE_PROTOCOL)        purgeWriteFailure=CORRUPT | authorityReadFailure busy=false
```

(a) `ctx.Err() != nil` OUTRANKS determinism: a `RAISE(ABORT)` that arrives
while the caller's context died for an unrelated reason is published as the
transient class — the precise inversion the cure exists to remove, restored by
a cancelled HTTP request. Reachability of that interleaving is not executed
here and is declared UNVERIFIED; the function-level property is executed.

(b) The godoc at `approvals_v15.go:436-437` — *"Same shape as this package's
authorityReadFailure, and the same declared limit in the opposite direction"* —
is FALSE for `SQLITE_LOCKED` and `interrupted`: `mapAuthorityStoreError`
(`authority_v2.go:690`) matches the substrings `busy`, `locked`, `interrupted`;
`isBusyClass` does not. Two classes that one twin calls transient the other
calls corruption. A letrero wider than its wire.

---

## P2-1 · The pair is named at ONE reader; the decide door and the plain claim door still consume it

**Claim.** Cure 1 puts the id/marker comparison inside
`approvalAuthoritySnapshotTx` only. The same incoherent pair is still approved
and still has its parked parameters handed over by the non-strict execution
door, which never consults the comparison.

**Wire.** The check: `internal/action/sqlite/approvals_v15.go:451-467`. Its two
call sites: `approvals_v15.go:357` (`approvalDetail`) and
`authority_v2.go:1748` (`startApprovedAuthorization`). The door that does NOT
consult it: `ClaimApprovalParamsUnderDigest`, `approvals_v15.go:789-798`, which
reads the SAME marker and refuses only `strictBorn != 0` — never comparing it
against the id prefix.

**REPRODUCTION (executed, production doors).**

1. Park a strict request through `ParkAuthorization` (via the real coordinator).
2. Through a SECOND real connection:
   `UPDATE approvals SET authority_snapshot_required=0 WHERE approval_id=?`
   and `DELETE FROM authorization_snapshots WHERE approval_id=?`
   — the exact state the new mould builds.
3. Call, in order: `GetApproval`, `ListPendingApprovals`, `ApprovalDetail`,
   `DecideApprovalUnderLaw(approved)`, `ClaimApprovalParamsUnderDigest`.

```
$ go test ./internal/app/ -run 'TestADV_whichDoors' -count=1 -v
  GetApproval           -> err=<nil>
  ListPendingApprovals  -> err=<nil> listed=true skipped=0
  ApprovalDetail        -> err=action/sqlite: approval "apr3_3806…" carries a
                           strict id against authority marker 0: authorization
                           snapshot corrupt
  APPROVE (production)  -> rule="" err=<nil>
  Claim (plain door)    -> err=<nil> params="{}"
```

The last line is the finding. For an id born STRICT — a request that by
construction requires a verified authorization snapshot, a budget debit and a
durable `authorization_starts` row — the plain claim door purges the parameters
and hands them back with no error, ready to dispatch, with none of that. No
authority verification, no debit, no start ledger, no revocation re-check.

**Exposure.** `executor.ResumeApproved` picks the strict door by
`e.config.StrictAuthority` (`internal/action/executor/executor.go:1005-1011`),
a CONFIG flag, not a property of the row. With strict authority ON the strict
door refuses. With it OFF — a profile that once ran strict and was turned off,
or the CLI path at `internal/cli/approvals.go:316,333` which passes
`cfg.StrictAuthority()` through — the plain claim above is the door that runs.

**Outcome it should have.** The comparison belongs where the marker is READ,
every time it is read. `ClaimApprovalParamsUnderDigest` already reads the
marker at `approvals_v15.go:792`; it should refuse the same pair there. Curing
one member of a class whose other members are named is the canto's own §6
discipline, and §6 files this class as "**cured**".

**Collateral, captured above:** the mould's own godoc at
`internal/app/strict_without_authority_test.go:51-52` says *"once the reader
names the pair, no production door can serve it, and a state with no emitter
does not enter a closed registry"*. Two production doors serve it —
`GetApproval` with no error, and `ListPendingApprovals`, which LISTS it as a
normal pending row with zero skips. The screen's list emits the state; the
detail then refuses to open it. The justification for adding no screen state
rests on a sentence my capture disproves, and the same test file contradicts it
at line 148, where it calls `GetApproval` on that very row and reads it back
successfully.

---

## P2-2 · The cure makes an existing APPROVED mould tautological, silently

**Claim.** `TestApprovalDetail_ActivatedLedgerPreventsLegacyDowngrade`
(`internal/action/sqlite/authority_snapshot_test.go:130-159`) builds exactly
the state the new pair check refuses: a strict row with the marker dropped to 0
and the snapshot deleted. After this diff the activation-ledger guard it exists
to watch can be deleted entirely and the test still passes.

**Wire.** The guard: `internal/action/sqlite/approvals_v15.go:442-447`
(`if s.authorityActivationDigest != "" { verifyAuthorityActivationTx … }`).
The new check that now pre-empts it: `approvals_v15.go:462-467`.

**REPRODUCTION (executed, both trees).** Mutate
`if s.authorityActivationDigest != "" {` → `if false && s.authorityActivationDigest != "" {`
and run the mould.

```
########## BASE (32701e0) + mutation ##########
--- FAIL: TestApprovalDetail_ActivatedLedgerPreventsLegacyDowngrade (0.07s)
    authority_snapshot_test.go:159: downgraded detail error = <nil>
FAIL

########## CURED TREE + same mutation ##########
ok  github.com/Sebastian197/korvun/internal/action/sqlite  0.722s
```

**Outcome it should have.** Doctrine point 4 and Rule 2(d): a guarantee whose
probing mutation no longer reddens IS the finding. The diff destroyed the
probing mutation of an approved mould and declared nothing. Either the mould is
re-pointed at a state the new check does NOT catch (a downgrade with the id
left non-strict, or with the marker left at 1), or the loss is declared in the
canto beside the re-pointed vitest mould. The canto §2 "How each cure could
itself break something" does not contain this row.

---

## P2-3 · The mirror half of the new mould never reddens, and the canto declares a red for it

**Claim.** Step 2 of `TestStrictWithoutAuthority_theReaderRefusesTheIncoherentPair`
(`internal/app/strict_without_authority_test.go:100-117`) passes identically
with the pair check neutralised. The canto §3 declares a red for it.

**Wire.** The assert is an either/or by construction: `err == nil` is a
`t.Fatalf`; a wrong class is only a `t.Logf`
(`strict_without_authority_test.go:111-117`), and the test says so out loud:
*"a foreign-key cascade may reach it first — declared, not asserted"*. That is
doctrine point 2 (`PROHIBIDO el either/or en asserts de fallo`) and Rule 2(i),
written into the mould as an escape hatch.

**REPRODUCTION (executed).**

1. Neutralise the pair check
   (`… ; false && strict != (required == 1) {`).
2. Downgrade step 1's two `t.Fatalf` to `t.Logf` so execution reaches step 2.
3. Run.

```
$ go test ./internal/app/ -run 'TestStrictWithoutAuthority…' -count=1 -v
    strict_without_authority_test.go:94: DIRECTION 1 would have failed here
--- PASS: TestStrictWithoutAuthority_theReaderRefusesTheIncoherentPair (0.15s)
```

The mirror half PASSED with the cure gone, and its own `t.Logf` at line 115 did
NOT fire — so the error it saw was still `ErrAuthorizationSnapshotCorrupt`,
produced by a guard that predates this diff (the signed-columns comparison at
`approvals_v15.go:527-534`, or the missing-row arm at line 519, both of which
catch an approval id that no longer matches the signed snapshot).

**Outcome it should have.** The canto §3 row *"…and so is its mirror | same,
mirror half | same | **yes**"* is false: there is no executed red for the
mirror half, and there cannot be one, because the mirror direction was already
guarded twice before this diff. Rule 3 and doctrine point 4: without a red
mutation of its own the mirror mould does not exist. The code comment at
`approvals_v15.go:455-456` — *"in BOTH directions, because the pair is
symmetric and only one half had ever been thought about"* — asserts the mirror
half was unguarded; the capture says it was guarded.

---

## P2-4 · Seven line-number citations rotted inside the commit that wrote them

**Claim.** Every `docs/HANDOFF.md:NNN` citation this diff introduces points at
the WRONG ficha in the tree this same commit produces, because this same commit
edits `docs/HANDOFF.md` above those lines.

**REPRODUCTION (executed).**

```
$ grep -rn "HANDOFF.md:[0-9]" --include=*.go .
internal/cli/approvals.go:110:              // Ficha docs/HANDOFF.md:339.
internal/cli/approvals_test.go:612:         // Ficha docs/HANDOFF.md:339 —
internal/action/sqlite/approvals.go:1026:   // (ficha docs/HANDOFF.md:339)
internal/action/sqlite/approvals.go:1045:   // Ficha docs/HANDOFF.md:329.
internal/action/sqlite/approvals_v15.go:256:// Ficha docs/HANDOFF.md:356.
internal/action/sqlite/approval_read_class_test.go:13:  // Ficha docs/HANDOFF.md:329 —
internal/action/sqlite/approval_read_class_test.go:68:  // Ficha docs/HANDOFF.md:356 —

$ for n in 329 339 356; do sed -n "${n}p" docs/HANDOFF.md; done
                                              <- 329 is BLANK
> **Curado.** La puerta responde por `classifyApprovalRead` como sus hermanas.
> lista —esta puerta no puede leer esas filas— pero se **nombra** y cambia el

$ grep -n "^### " docs/HANDOFF.md | sed -n '5,7p'
337:### ~~`GetApprovalByAction` devuelve su error de lectura sin clase~~ …
352:### ~~`korvun approvals list` oculta una fila …~~ …
374:### ~~La lista de pendientes nombra con id vacío la fila que salta~~ …

$ git show 32701e0:docs/HANDOFF.md | sed -n '329p;339p;356p'
### `GetApprovalByAction` devuelve su error de lectura sin clase
### `korvun approvals list` oculta una fila con un estado fuera de los cinco conocidos
### La lista de pendientes nombra con id vacío la fila que salta
```

The three numbers were correct against the BASE and are wrong against the
COMMIT, shifted by 8, 13 and 18 lines by this diff's own insertions. Two of
them now land inside a DIFFERENT ficha's cure note, which is worse than landing
on blank: a reader following `docs/HANDOFF.md:339` from `internal/cli/approvals.go`
arrives at the `GetApprovalByAction` ficha and reads a real, wrong answer.

**Outcome it should have.** CLAUDE.md, "Comments carry no relative positions"
(CRITICAL): *"A line NUMBER may be cited only when it is re-derived against the
final commit, and a quoted phrase is preferred because it survives its own
cure."* None of the seven was re-derived. Cite the heading, which the diff
already quotes elsewhere.

**Aggravating.** Canto §7, question 2, answers: *"Does any comment cite a
symbol that does not exist? Every ficha is cited by its heading, never by a
line number into a file this commit edits."* That sentence is false about seven
citations in its own diff, and it is the answer to the question designed to
catch it.

---

## P2-5 · `GetApprovalByAction` lost its godoc; `CountApprovals` inherited two other doors' text

**Claim.** `CountApprovals` was spliced into the middle of an existing comment
block, so its public documentation opens with a paragraph about a DELETED door
and a sentence describing `GetApprovalByAction`, and `GetApprovalByAction` — an
exported method — is left with no godoc at all.

**Wire.** `internal/action/sqlite/approvals.go:1012-1037`. The block at
1012-1019 documents the deleted `ClaimApprovalParams` and then
`GetApprovalByAction`; the diff inserted `// CountApprovals returns how many…`
at line 1020 and the function at 1029, leaving `func (s *Store) GetApprovalByAction`
at line 1037 bare.

**REPRODUCTION (executed).**

```
$ go doc ./internal/action/sqlite Store.CountApprovals
func (s *Store) CountApprovals(ctx context.Context) (int, error)
    The old ClaimApprovalParams door was DELETED on 2026-09-13. …
    GetApprovalByAction returns the approval bound to one action (the verifier's
    approval-coherence lookup). CountApprovals returns how many approval rows
    exist, whatever their status.
    …

$ go doc ./internal/action/sqlite Store.GetApprovalByAction
func (s *Store) GetApprovalByAction(ctx context.Context, actionID string) (action.Approval, action.ActionPreview, error)
                                                 <- no documentation
```

**Outcome it should have.** CLAUDE.md Go standards: *"Every exported symbol has
a godoc comment."* `golangci-lint` cannot catch this — the config
(`.golangci.yml`) enables only `govet`, `staticcheck`, `errcheck`, `gosec`, so
`make quality` stays green over it. "Gate green" does not cover this class, and
the canto's "gate green" is therefore not evidence about it.

---

## P2-6 · "the one place this repository spells it" is false in the same file

**Claim.** The new const's godoc asserts a uniqueness that the same file
contradicts 22 lines earlier.

**Wire and REPRODUCTION (executed).**

```
$ sed -n '131,155p' internal/action/approval.go
131 // approvalIDShape accepts the compatibility and strict store-minted shapes.
132 var approvalIDShape = regexp.MustCompile(`^(?:apr|apr3)_[0-9a-f]{32}$`)
…
151 // strictApprovalPrefix is the one place this repository spells it. The minter
152 // below and IsStrictApprovalID are the only readers, so a caller can never ask
153 // the question with a literal of its own that drifts from the answer.
154 const strictApprovalPrefix = "apr3_"

$ grep -rn "apr3" --include=*.go . | grep -v _test
internal/action/approval.go:132:var approvalIDShape = regexp.MustCompile(`^(?:apr|apr3)_[0-9a-f]{32}$`)
internal/action/approval.go:154:const strictApprovalPrefix = "apr3_"
(+ two comment mentions in approvals_v15.go)

$ grep -n "apr3" cmd/korvun-desktop/frontend/src/views/Approvals.tsx
181:const APPROVAL_ID_RE = /^(?:apr|apr3)_[0-9a-f]{32}$/
```

**Outcome it should have.** Scope the sentence down to what it is — one home
for the PREFIX PREDICATE, beside a regexp that still spells the same token and
a TypeScript regexp that spells it a third time — or fold `approvalIDShape` to
be built from the const. Canto §6 repeats the wider claim: *"One question
spelled in several places | the `apr3_` prefix | **cured**: one home"*. The
Tone law admits no exception and this is a comment stating a guarantee stronger
than its wire. Note the pre-existing HANDOFF ficha "La forma del recibo vive
solo en TypeScript" (line 400) already records that this token lives in two
languages.

---

## P3-1 · The cured error message calls an ACTION id an approval id

`internal/action/sqlite/approvals.go:1045-1054`. The comment justifies keeping
the not-found arm out of the classifier because *"classifyApprovalRead would
reword it around the APPROVAL id, which this door does not have"* — and then
passes `actionID` into `classifyApprovalRead` on the OTHER arm, where the same
rewording happens.

```
$ go test ./internal/action/sqlite/ -run 'TestADV_getApprovalByActionMessage' -count=1 -v
  cancelled read  -> action/sqlite: approval for action "act_msg": action/sqlite:
                     approval "act_msg": action/sqlite: the approval could not be
                     read: context canceled
  absent row      -> action/sqlite: approval for action "act_never": action/sqlite:
                     approval row absent: action/sqlite: action not found
```

`approval "act_msg"` is an action id in an approval's slot, printed by
`korvun receipt verify` as part of an `approval_mismatch` failure line
(`internal/cli/receipt.go:324-328`).

## P3-2 · The cure's stated reason is false about its only production caller

The godoc at `internal/action/sqlite/approvals.go:1045-1051` and the canto say
*"a caller branching on «retry or not» — and the receipt verifier at
internal/cli/receipt.go is one — got nothing to branch on."* The verifier does
not branch on it:

```
$ sed -n '289,330p' internal/cli/receipt.go
289  switch consumed, _, err := store.GetApprovalByAction(ctx, r.ActionID); {
290  case errors.Is(err, actionsqlite.ErrNotFound):
…
324  case err != nil:
328      fail("approval_mismatch", "approval row for %s is unreadable: %v", r.ActionID, err)
```

One `err != nil` arm, one named failure, the raw error interpolated. Adding the
class is not wrong; the sentence that justifies it is false about the tree, and
no caller in the tree consumes the new class. Mandatory question "which test
would turn red if the guarantee were false" is answered only by the test the
diff wrote for itself.

## P3-3 · Documentary arithmetic: "the twelve returns"

Canto §1 and `internal/app/strict_without_authority_test.go:43` both say *"Of
the twelve returns in `approvalAuthoritySnapshotTx`, exactly one left without
an error."*

```
$ (return statements counted between the func line and its closing brace)
CURED tree: lines 441-537, 15 return statements
BASE  tree: lines 403-483, 14 return statements
```

Neither is twelve. Rule 2(h): documentary arithmetic not verified by execution.
Secondarily, two returns leave with a nil error, not one (`return nil, nil` and
`return &snapshot, nil`); the intended reading is defensible but the literal
count is not.

## P3-4 · Documentary arithmetic: "6 rows" for a mutation that reddens 2

Canto §3, last row: *"A non-minted receipt is not a sealed receipt |
`Approvals.v0151b.test.tsx`, three rows | **reject takes its receipt as the
string it is** | 6 rows"*.

```
MUTATION AS DECLARED (move `if (!isReceiptID(receipt))` back below the reject branch):
 Test Files  1 failed | 43 passed (44)
      Tests  2 failed | 503 passed (505)
   × P2-1 · hermana: un recibo no acuñado en un rechazo no se pinta como recibo sellado
   × P2-1 · hermana: un `receipt_id` vacío en un rechazo tampoco

VARIANT (delete the minted-shape demand for BOTH verbs):
      Tests  6 failed | 499 passed (505)
   × P2-10 · receipt_id vacío …            }
   × P2-10 · receipt_id mal formado …      }  four PRE-EXISTING approve-side rows
   × P2-10 · failed con receipt_id vacío … }  guarding a check this diff did not add
   × P2-10 · failed con receipt_id mal formado … }
   × P2-1 · hermana … (the two new ones)
```

The declared mutation yields 2. The 6 comes from a wider mutation that also
neutralises the approve-side guard that predates this diff, so four of the six
reds are not evidence about this cure. Rule 3 requires the mutation to
neutralise the branch the mould claims to watch.

## P3-5 · The re-pointed mould's coverage loss, named exactly

The mandate asks what the old mould caught that nothing now catches. Executed,
both directions:

```
BASE code + BASE mould, escape at the receipt render sites DELETED:
   × P2-1 · hermana: el recibo tras una decisión se imprime por el alfabeto de escape
 Test Files  1 failed (1)   Tests  1 failed | 51 passed (52)

CURED tree, the SAME deletion (all three sites, lines 1518, 1535, 1547):
 Test Files  44 passed (44)
      Tests  505 passed (505)
```

Answer: nothing now reddens if `escapeUntrusted` is removed from
`Approvals.tsx:1518`, `:1535` and `:1547` — the `executed`, `failed` and
`rejected` receipt render sites. Today that is harmless, because all three are
shape-gated by `isReceiptID`, so it is defence in depth with zero coverage
rather than a live defect. But the canto §4 answers "Why that is not a
downgrade" with a statement about the GUARANTEE and is silent about the
COVERAGE, and HANDOFF line 424 says *"No es rebaja del escape"* without
qualification. The honest sentence is: the guarantee got stronger and the
escape at three sites lost its only mould.

## P3-6 · Relative positions in comments this diff writes

CLAUDE.md, "Comments carry no relative positions" (CRITICAL): *"no line
distances, no 'above'/'below'."*

- `internal/action/approval.go:151-152` — "The minter **below** and
  IsStrictApprovalID are the only readers".
- `internal/action/sqlite/approvals_v15.go:256-259` — "The godoc **above**
  promises the id …, and the three time arms **below** keep it." The first is
  rescued by the quoted phrase that follows it; "the three time arms below" is
  a pure relative locator.
- `internal/action/sqlite/approvals_v15.go:459` — "fell through to the
  `return nil, nil` **below**".

## P3-7 · What the `Scan` cure rests on — VERIFIED, with one dead claim

The mandate asked for execution. `database/sql` does populate earlier
destinations before failing, with THIS repository's driver
(`modernc.org/sqlite v1.59.0`):

```
$ go test ./internal/action/sqlite/ -run 'TestADV_scanAssignsLeftToRight' -count=1 -v
  later-column failure: err=sql: Scan error on column index 1, name "n":
      converting driver.Value type string ("siete") to a int: invalid syntax
      | id="apr_first" n=0 tail=""
  first-column failure: err=<nil> | id="\x00\xff" n=7 tail="zzz"
```

The cure's premise HOLDS: the id is the first destination
(`approvals_v15.go:249`), and it is set whenever a later column fails. No
finding against the mechanism.

The second line kills the comment's closing claim. *"An id that is still empty
here means the id column itself failed, and then empty is the truth rather than
a loss."* The first destination is a `string`; SQLite hands a BLOB straight
through to a string without error, and `approval_id` is `TEXT NOT NULL PRIMARY
KEY` so it cannot be NULL. The branch describes a state that cannot occur with
this query. Harmless, but it is a sentence about behaviour that was not
verified.

Related, undeclared: the arm now returns the PARTIALLY populated `action.Approval`
as its first value where it used to return the zero value
(`approvals_v15.go:267`). The only caller discards it
(`approvals_v15.go:207-211`), so no live defect — but handing a half-scanned
struct out of an error path is Rule 2(f) territory and the canto does not name
it.

---

## Attacks run that found NOTHING — declared, because the mandate named them

- **Can a legitimate row make the pair check fire?** No. Exactly two writers of
  `approvals` rows exist in production: `internal/action/sqlite/approvals.go:164`
  (non-strict id from `NewApprovalID`, marker defaults to 0) and
  `internal/action/sqlite/authority_v2.go:1674` (strict id from
  `NewStrictApprovalID`, marker literal `1`), both inside one transaction with
  the id they mint. `grep -rn "INSERT INTO approvals\|UPDATE approvals SET approval_id"`
  finds no third. The claim "they cannot disagree in a row this store wrote" is
  TRUE.
- **Migrations from schema < 15.** `addAuthorityV15Columns`
  (`store.go:725-758`) adds the column with `DEFAULT 0`. Strict ids are minted
  only by `ParkAuthorization`, which requires the v15 tables, so a pre-v15
  store cannot hold an `apr3_` row. Legacy `apr_` rows migrate to marker 0 —
  coherent.
- **A marker that is neither 0 nor 1.** The `CHECK(... IN (0,1))` survives
  `ALTER TABLE ADD COLUMN` and is enforced on a migrated table:
  ```
  marker := 2        REFUSED  -> CHECK constraint failed
  marker := -1       REFUSED  -> CHECK constraint failed
  marker := '1'      ACCEPTED -> stored (1, 'integer')   (INTEGER affinity)
  marker := x'01'    REFUSED  -> CHECK constraint failed
  ```
  A second connection cannot write an out-of-domain marker without disabling
  the constraint or rebuilding the table.
- **Does the pair check change `ParkAuthorization` / `startApprovedAuthorization`?**
  `ParkAuthorization` never calls the reader. `startApprovedAuthorization`
  (`authority_v2.go:1748`) does, and for an incoherent pair it returned
  `ErrAuthorizationSnapshotCorrupt` before (via `pending == nil` at line
  1752) and returns the same sentinel now, with a different message. No class
  change. Full suite green confirms no existing mould that builds such a row
  deliberately broke — including `TestApprovalDetail_ActivatedLedgerPreventsLegacyDowngrade`,
  which now passes for the wrong reason (P2-2).
- **`ApprovalsAdapter.Reject` and the receipt shape.**
  `internal/app/approvals_adapter.go:421-455` returns
  `ReceiptID: after.DecisionReceiptID`, re-read after a committed decide. That
  cell is written only from `recordDecisionActTx`'s `proofID`
  (`approvals.go:320,338`), which is `action.NewReceiptID()` =
  `"rcpt_" + 32 hex` (`internal/action/wire.go:67-71`) — the exact shape
  `RECEIPT_ID_RE` demands. The expiry close writes an EMPTY receipt id
  (`closeApprovalTx`, `approvals.go:404-410`) but leaves the row `EXPIRED`, and
  `Reject` refuses that in band before it can emit a DTO. I found NO production
  path emitting a `rejected` outcome with a receipt the new check refuses.
  When it does happen the screen falls to `UNRECOGNISED_SUCCESS`
  (`approvalsText.ts:38-39`), whose literal asserts nothing — honest. The cure
  itself is sound on the wire; only its coverage claim (P3-5) is not.
- **Canto §5's effect-class reasoning.** Verified against
  `internal/action/effect.go:36-66`: `EffectCritical` ranks 5,
  `unknownEffectRank = 6`, strictly above. The adjudication note's arithmetic
  holds.

---

## Scope declaration

**What I read** (whole files or the named regions, on disk, in this worktree):
`internal/action/approval.go`, `internal/action/effect.go`,
`internal/action/wire.go:67-71`, `internal/action/sqlite/approvals.go`
(160-180, 260-440, 780-1060), `internal/action/sqlite/approvals_v15.go`
(55-75, 190-280, 340-540, 680-900), `internal/action/sqlite/authority_v2.go`
(380-420, 500-560, 690-740, 1590-1800),
`internal/action/sqlite/store.go` (255-330, 700-780, 1400-1600, 1860-1920),
`internal/action/sqlite/approval_rows.go:160-192`,
`internal/action/sqlite/authority_snapshot_test.go:110-160`,
`internal/action/sqlite/approvals_sentinels_test.go:95-140`,
`internal/action/executor/executor.go:975-1045`,
`internal/app/approvals.go:180-210`, `internal/app/approvals_adapter.go`
(400-545, 655-700), `internal/app/authority_strict_doors_test.go:56-350`,
`internal/app/strict_without_authority_test.go` (whole),
`internal/action/sqlite/approval_read_class_test.go` (whole),
`internal/cli/approvals.go:55-135, 285-350`, `internal/cli/intent.go:88-135`,
`internal/cli/receipt.go:275-400`, `internal/controlapi/approvals.go`
(115-225, 310-320), `cmd/korvun-desktop/frontend/src/views/Approvals.tsx`
(100-240, 1460-1560), `.../approvalsText.ts`, `.../Approvals.v0151b.test.tsx`
(diff hunks), `docs/cantos/V0161-B-2026-09-23.md` (whole),
`docs/HANDOFF.md` (the diff hunks and lines 240-450), `.golangci.yml`,
`Makefile:250-252`, `go.mod`.

**What I executed** (all commands and outputs are quoted above):
`go vet ./internal/...`; `go test ./internal/...` (39 packages, green);
`npx vitest run` (44 files / 505 tests, green); `gofmt -l`;
`golangci-lint run ./internal/... ./cmd/...` (clean);
`go doc` on two symbols; nine probing mutations, each applied ALONE to a COPY
and reverted (five reddened as the canto claims: pair check direction 1, purge
class, scan arm, `GetApprovalByAction` class, the CLI comparison; two produced
the P2-2 and P2-3 findings; two are the vitest receipt mutations of P3-4/P3-5);
six adversary probes (real driver error classification, real `SQLITE_BUSY` /
`SQLITE_READONLY` / `SQLITE_FULL` / closed pool, `Scan` ordering, the purge
through the production door under a forced write failure, the operator-surface
chain, the `approvals list` race, the door survey over the incoherent pair,
the CHECK-constraint survey after `ALTER TABLE`).

**What I could NOT verify.**
- The canto's "gate green": I ran `lint`, `test` and the frontend suite, not
  `cover`, `fuzz-smoke`, `hook-probe`, `integration-probe`, `wails-pin-probe`,
  nor `govulncheck`. "Gate green on this tree" is UNVERIFIED by me.
- The interleaving in which a cancelled context and a deterministic
  `RAISE(ABORT)` land together (P1-2, point (a)): the function-level inversion
  is executed, the interleaving is a PREDICTION, labelled as such.
- Whether `SQLITE_LOCKED` or `interrupted` can land on the purge's UPDATE with
  this DSN (no shared cache): the classifier divergence is executed, the
  reachability is a PREDICTION, labelled as such.
- The real-model, OS-process-binary and crash-restart evidence levels: nothing
  here was run as a compiled binary in a separate OS process. The new CLI
  mould's own label ("the compiled CLI inside the test process") is accurate
  and I did not raise it.
- `cmd/korvun-desktop/e2e-harness` and Playwright were not run.

**What remains unexamined.** Everything outside the nine changed files and the
three new ones: the website, the workflows, the integration gate, the other
`internal/*` packages beyond the call graphs traced above, and the rest of
`docs/HANDOFF.md`.

---

# VETO MANTENIDO

Two P1 and six P2 stand. The blocking set, in order:

1. **P1-1** — `approvals list` accuses a healthy store and exits 1 (captured).
2. **P1-2** — a full disk is published as permanent evidence corruption, and
   the corrupt class deletes the `ReReadParams` re-read (captured).
3. **P2-1** — the pair is named at one reader; approve and the plain claim
   still consume it and hand over the parked parameters (captured).
4. **P2-2** — an approved mould became tautological, undeclared (captured
   both ways).
5. **P2-3** — the mirror mould never reddens and the canto declares its red
   (captured).
6. **P2-4** — seven line citations rotted inside their own commit; the canto's
   §7 Q2 denies it (captured).
7. **P2-5** — an exported door lost its godoc; a new one inherited two others'
   (captured with `go doc`).
8. **P2-6** — "the one place this repository spells it", contradicted 22 lines
   earlier in the same file (captured).

Cure 5 (the `Scan` arm) is the only one whose mechanism I could not break: its
premise is verified by execution against this repository's own driver.
