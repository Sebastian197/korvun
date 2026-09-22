# VETO MANTENIDO

Adversarial audit of branch v0.16.1 C, diff against base `32701e0`, executed
2026-09-22 by the internal adversary with a clean context.

Every finding below that could be run WAS run. Commands and their raw output are
quoted. Two claims are labelled PREDICTION and stay predictions.

All mutations were applied to a COPY of the tree at
`/private/tmp/claude-501/.../scratchpad/mut1` (and a pristine base copy at
`.../scratchpad/base`). The audited tree
`/Users/sebastianmorenosaavedra/Desktop/korvun-v0161c.nosync` was NOT modified;
`git status --short` at the end of the audit is byte-identical to the opening
snapshot. The ONLY write I made inside it is this verdict file, at the path the
mandate named.

---

## P1-1 — ONE `UPDATE` of an UNSEALED column disables the verifier's sabotage arms. This is a REGRESSION against `32701e0`.

**Claim.** `actionRowIsALaterLife` (`internal/cli/receipt.go:554-571`) decides
"is this row my life?" by comparing the SEALED `receipt.StartedAt` against the
UNSEALED, freely writable column `actions.requested_at`
(`internal/action/sqlite/store.go:1836-1849`). An attacker with the same write
access the R15 sabotage arms assume needs ONE statement to make the verifier
declare a tampered row "a later life", skip every custody comparison, print
`OK`, and exit 0.

### Reproduction (EXECUTED)

1. Seed a healthy sealed action + receipt through the production door
   (`operatorReceipt`, the fixture `receipt_r15_join_test.go` itself uses).
2. Run `korvun ledger check --config <cfg>` — baseline.
3. Through a second real connection with foreign keys ON, run exactly:

```sql
UPDATE actions
   SET parameters_digest = 'sha256:0000000000000000000000000000000000000000000000000000000000000000',
       requested_at      = '2099-01-01T00:00:00Z'
 WHERE action_id = ?;
```

4. Run `korvun ledger check` and `korvun receipt verify <receipt-id>` again.

### Captured output — WITH the diff

```
BEFORE  ledger check: exit 0 "ledger main: 1 receipts, chain intact\n"
AFTER   ledger check: exit 0 "ledger main: NOTE 1 degraded check(s) across the walk — each one named on its own receipt; the digest-sealed receipts stand\nledger main: 1 receipts, chain intact\n"
AFTER   receipt verify: exit 0 "receipt rcpt_e248cb9033b1b149d1548ef12102f591 (seq 0): OK\nreceipt rcpt_e248cb9033b1b149d1548ef12102f591 (seq 0): NOTE action_id_reused: the action row for act_dcf09f00e5591b47e376e0d51883110e was requested after this receipt began, so it belongs to a later life; this receipt's custody is judged by its own seal, not by that row"
```

### Captured output — the SAME test against the base `32701e0` (tracked files reverted, the three new test files removed)

```
BEFORE  ledger check: exit 0 "ledger main: 1 receipts, chain intact\n"
AFTER   ledger check: exit 1 "ledger main: FAIL at receipt rcpt_b24834dd61415f3d119a28be54e9c5ea (seq 0): custody_mismatch: receipt action digest sha256:e2f2... but the action row carries sha256:0000...\n"
AFTER   receipt verify: exit 1 "receipt ... FAIL custody_mismatch: ..."
```

So: **exit 1 before the diff, exit 0 after it.** The diff converts a detected
tamper into `OK`.

### The same attack against the arm `receipt_r15_join_test.go` exists to guard (EXECUTED, separately)

Step 1 verbatim from `TestReceiptVerify_joinFailOpen_twoChildDeletesNoLongerBuyAPass`
(`DELETE FROM approvals WHERE action_id = ?`), then one bump:

```
BEFORE the bump: exit 1 "... FAIL approval_mismatch: receipt seals approval digest sha256:4c96... but no approval row exists for act_inbox1 while its action row remains — a cascade cannot do that"
AFTER  the bump: exit 0 "... OK
  NOTE action_id_reused: the action row for act_inbox1 was requested after this receipt began, so it belongs to a later life; this receipt's own row is gone and its approval went with it
  NOTE action_id_reused: ... this receipt's custody is judged by its own seal, not by that row"
```

### A second, independent one-UPDATE silencing of the same arm (EXECUTED)

`UPDATE actions SET requested_at = '0001-01-01T00:00:00Z' WHERE action_id = ?`
reaches the DEGRADE branch (`receipt.go:567`, `requested.IsZero()`):

```
[zero instant] after the UPDATE: exit 0 "... OK
  NOTE approval_row_absent_undecidable: the approval row for act_inbox1 is gone and this verifier cannot establish whether the action row present is this receipt's life (the action row carries no request instant)"
```

(For completeness: `requested_at='not-a-time'` and `requested_at=''` still exit 1,
but under a DIFFERENT name — `custody_mismatch: action row ... is unreadable` —
so the approval arm is silenced there too, just not the exit code.)

### Why the design argument does not hold

The proof that `receipt.StartedAt` is sealed and `actions.requested_at` is not
was executed. Mutating the receipt side fails closed:

```
after started_at=NULL: exit 1 "... FAIL hash_mismatch: stored sha256:48a6..., recomputed sha256:91c9...
  FAIL signature_invalid: the signature does not verify against the registered public key \"ed25519:b5fc...\""
```

So the gate compares one value the chain protects against one value nothing
protects, and lets the UNPROTECTED one win. This is the catalog's "valid
signatures over invalid or stale surrounding data" and rule 2(b) inverted: a
STORED mutable value overrides a SEALED one.

`docs/cantos/V0161-C-2026-09-22.md:24` states the mitigation as
"The gate sits behind the presence switch and fires only when a row is present
and provably later." *Provably* is the load-bearing word and it is false: the
proof is an attacker-writable cell.

**Outcome it should have.** A receipt whose action row's `parameters_digest` no
longer matches its seal must FAIL `custody_mismatch`, exit non-zero, regardless
of `requested_at`. A discriminator for id reuse must rest on something the
chain covers (or must not be able to SUPPRESS a failure — at most it may
re-label one).

**Files read and verified:** `internal/cli/receipt.go:323-346`, `:397-415`,
`:546-571`; `internal/action/sqlite/store.go:1827-1849`;
`internal/action/receipt.go:34-86` (the sealed field list — `requested_at` of
the action row is not among them); `internal/action/sqlite/ledger.go:427-448`
(`receipt.StartedAt` is `actions.requested_at` read at seal time).

---

## P2-1 — The FOURTH writer of the `:279` class has NO mould. It can be left entirely uncured and the whole suite stays green.

**Claim.** `internal/action/sqlite/retention_after_commit_test.go:15` says
«The P1 of ficha docs/HANDOFF.md:279, **attacked at every door of its class**».
The table has THREE rows: the park, `RecordAttempt`, the identified writer.
`RecordAttemptAuthenticated` (`internal/action/sqlite/identity_v2.go:219`) — the
fourth door, and the one three operator CLI verbs actually use
(`internal/cli/intent.go:429`, `internal/cli/authority.go:48`,
`internal/cli/grant.go:60`) — has no row.

The spec is more explicit and equally wrong:
`docs/superpowers/specs/2026-09-22-v0161-c-park-commit.md:98` — "The **four**
sibling writers each carry a row proving their committed write survives a
failing cadence"; `:77` — "**Nothing of the class is cured in silence.**"

### Reproduction (EXECUTED — probing mutation)

1. In a copy of the tree, add a private `noteWriteUncured` that is the PRE-cure
   body (returns the error AND notifies the observer, so the observer half is
   untouched).
2. Change ONLY `identity_v2.go:219` back to `return s.noteWriteUncured(ctx)`.
   The other three doors stay cured.
3. `go build ./... && go test ./internal/action/... ./internal/app/... ./internal/cli/... -count=1`

```
mutation C applied: only RecordAttemptAuthenticated is uncured
BUILD OK
ok  	github.com/Sebastian197/korvun/internal/action	0.569s
ok  	github.com/Sebastian197/korvun/internal/action/executor	21.247s
ok  	github.com/Sebastian197/korvun/internal/action/sqlite	50.932s
ok  	github.com/Sebastian197/korvun/internal/app	41.623s
ok  	github.com/Sebastian197/korvun/internal/cli	14.594s
```

The P1 survives, whole, at one of its four doors, and nothing turns red.

**Doctrine points violated:** (4) the test of the test is universal —
no probing mutation for this door; (1) the exact outcome is not demanded at
every named door. Rule 2(d): a claim whose dangerous branch can be removed with
the suite identical. Rule 2(e): the godoc and the spec promise four, the wire
carries three.

**Outcome it should have.** Either a fourth table row, or the sentence
"attacked at every door of its class" and the spec's success criterion scoped
down to the three doors actually attacked, in the same commit that notices it.

---

## P2-2 — The sweep branch of `noteWrite` has NO mould. Deleting its observer notification leaves the entire suite green.

**Claim.** `internal/action/sqlite/store.go:1777-1779` is a NEW branch of this
diff:

```go
if _, _, err := s.SweepExpiredApprovals(ctx, time.Now().UTC()); err != nil {
    s.noteRetentionFailure(fmt.Errorf("action/sqlite: periodic expiry sweep: %w", err))
}
```

Nothing anywhere asserts it. `grep -rn "periodic expiry sweep" --include=*.go .`
returns exactly one hit — the production line itself. The only assertions on the
observer's payload are `strings.Contains(heard[0].Error(), "periodic prune")`
(`errors_test.go:235`) and `... "periodic"` (`retention_after_commit_test.go:127`),
both reached through the PRUNE path.

### Reproduction (EXECUTED — probing mutation)

1. In a copy, replace the notification with `_ = err`.
2. `go build ./... && go test ./internal/action/... ./internal/app/... ./internal/cli/... -count=1`

```
mutation A applied
ok  	github.com/Sebastian197/korvun/internal/action	0.833s
ok  	github.com/Sebastian197/korvun/internal/action/executor	22.286s
ok  	github.com/Sebastian197/korvun/internal/action/sqlite	61.856s
ok  	github.com/Sebastian197/korvun/internal/app	58.631s
ok  	github.com/Sebastian197/korvun/internal/cli	23.208s
```

The "blindness" the spec explicitly REJECTED as an option
(`spec:52`, "Swallow the housekeeping error silently — **Rejected**") is
reachable for the sweep half and no test notices.

Worse, the spec PROMISES the missing mould by name.
`docs/superpowers/specs/2026-09-22-v0161-c-park-commit.md:62`:

> | corruption — the sweep meets an unreadable row | ... | `ErrApprovalUnreadable`, to the OBSERVER | ... | **the repair mould**: the observer receives the exact error / mutation: drop the observer call | ... |

That mould does not exist, and I executed exactly the mutation the spec names
("drop the observer call") with a green suite as the result.

And `docs/cantos/V0161-C-2026-09-22.md:88` answers question 5 — *"Did any cure
enter without its mould and its captured red?"* — with **"No."** That answer is
false for this branch.

**Outcome it should have.** A row forcing `SweepExpiredApprovals` to fail on the
cadence write (e.g. blocking the sweep's own UPDATE) and asserting the observer
hears exactly one error naming `periodic expiry sweep`; or the spec row deleted
and the canto's question 5 answered honestly.

---

## P2-3 — The canto states TWICE that an undecidable/equal instant DEGRADES. It ACCUSES. Executed.

**Claim.** Two public sentences:

- `docs/cantos/V0161-C-2026-09-22.md:25` — "| `:414` | Two lives that share an instant | Strictly-after only. **An equal or unknown instant is «cannot tell», which degrades** |"
- `docs/cantos/V0161-C-2026-09-22.md:72` — "**Two lives sharing a request instant** are indistinguishable to `:414`'s discriminator; **the check degrades rather than guess.**"

The wire says otherwise. `internal/cli/receipt.go:570` returns
`(requested.After(r.StartedAt), "")` — for an EQUAL instant that is
`(false, "")`, the guard at `:341` (`later || why != ""`) is false, and control
falls straight through to `fail("approval_mismatch", ...)`. It does not degrade.
It accuses, and `ledger check` stops at that link — the exact defect the ficha
was opened for.

### Reproduction (EXECUTED)

1. Park + approve `act_reused_life` through the production door; read the
   sealed receipt (its `StartedAt`).
2. `DELETE FROM actions WHERE action_id='act_reused_life'` through a second
   real connection (frees the id — see "verified premise" below).
3. Record a SECOND life on the same id through `Store.RecordAttempt`, with
   `RequestedAt` set to **exactly the earlier receipt's `StartedAt`** (row 1),
   and to **one second earlier** (row 2, a clock stepped back).
4. `korvun receipt verify <earlier receipt>` and `korvun ledger check`.

```
[same instant] receipt verify of the EARLIER life: exit 1
  "FAIL approval_mismatch: receipt seals approval digest sha256:4584... but no approval row exists for act_reused_life while its action row remains — a cascade cannot do that
   FAIL custody_mismatch: receipt attests \"SUCCEEDED\" but the action row says \"AUTHORIZED\""
[same instant] ledger check: exit 1
  "ledger main: FAIL at receipt rcpt_1382... (seq 1): approval_mismatch: ..."

[clock stepped back one second] receipt verify of the EARLIER life: exit 1
  "FAIL approval_mismatch: ...  FAIL custody_mismatch: ..."
[clock stepped back one second] ledger check: exit 1
  "ledger main: FAIL at receipt rcpt_6d1b... (seq 1): approval_mismatch: ..."
```

Both canto sentences are false as literally written, and the cure is partial:
`:414` is fixed only when the clock moves forward between the two lives. A
backwards step (NTP correction, VM/snapshot restore, a restored backup, a
misconfigured clock corrected) reinstates the original false accusation, with
the walk still stopping at the first link.

`docs/HANDOFF.md:425-431`'s strikethrough — "Donde no se puede decidir, la
comprobación se degrada con su razón en vez de acusar" — carries the same
overreach: for the SECOND arm (`receipt.go:410`) the undecidable case is not
degraded at all; it is fed to `store.Get`, which then fails
`custody_mismatch: ... is unreadable`.

**Outcome it should have.** Either the discriminator handles `<=` and
undecidable as "cannot tell" (which is what the docs sell), or both sentences
are scoped to what the wire does: "a row requested strictly AFTER the receipt's
seal is treated as a later life; equal, earlier or unknown instants are still
accused."

---

## P3-1 — Four code comments cite `docs/HANDOFF.md` line numbers that THIS SAME COMMIT invalidated.

The diff turns three ficha headings into strikethroughs and adds 6 quoted lines
above each. Re-derived against the post-diff file:

```
279: ### ~~Un aparcamiento confirmado puede devolver error~~ — CURADO 2026-09-22 (v0.16.1 C)     [correct]
376: pendiente, escribir texto no numérico en `policy_version` y llamar a                        [WRONG — a different ficha]
414: sujeto un molde aprobado: el que prueba que la pantalla ESCAPA un recibo hostil             [WRONG — a different ficha]

actual heading lines now:  279, 382, 425
```

Stale citation sites, all written or moved by this diff:
`internal/action/wire.go:76`, `internal/action/receipt_shape_test.go:14`,
`internal/cli/receipt.go:323`, `internal/cli/receipt.go:397`,
`internal/cli/ledger_reused_id_test.go:18`.

This is the literal violation of CLAUDE.md "Comments carry no relative positions
(2026-09-08) — CRITICAL": *"A line NUMBER may be cited only when it is
re-derived against the final commit."* The law was born from exactly this —
a citation invalidated by another cure of the SAME commit.

---

## P3-2 — `action.ValidReceiptID` has ZERO production callers, and the weaker check the diff's own godoc denounces is still there.

```
$ grep -rn "ValidReceiptID" --include=*.go .
internal/action/wire.go:82,84,85         (definition)
internal/action/receipt_shape_test.go:36,45,65,66   (its own test)
```

Nothing else. The exported predicate is a door only its test reaches — the
repository's own recorded lesson ("enumerar métodos exportados sin llamador de
producción antes del canto").

`internal/action/wire.go:71-74` says the only Go check anywhere "was a prefix in
`internal/cli/receipt.go` — weaker than the screen's, so the two were already
inconsistent with each other", and then presents the new constant as "the Go
half of that seam". `internal/cli/receipt.go:122` still reads
`if strings.HasPrefix(id, "rcpt_") {`. The inconsistency the godoc describes in
the past tense is alive in the present tense, in a file this same diff edits.
The canto's §6 "Declared and not covered" does not mention either fact.

**What is true and worth keeping:** the ficha's own reproduction now reddens.
EXECUTED — minter `make([]byte, 16)` → `make([]byte, 20)`:

```
--- FAIL: TestReceiptID_theMintedShapeIsTheOneTheScreenDemands (0.00s)
    receipt_shape_test.go:37: NewReceiptID minted "rcpt_9d78a22bb7a265c253448f90624f485281589e69", which the screen refuses
```

And the Go literal drifting alone reddens two moulds (EXECUTED,
`{32}` → `{32}` with `A-F` added):

```
--- FAIL: TestReceiptID_theTwoLanguagesHoldOneShape
    the screen requires "^rcpt_[0-9a-f]{32}$" and Go mints against "^rcpt_[0-9a-fA-F]{32}$" — the two descriptions of one receipt have drifted
--- FAIL: TestReceiptID_ValidReceiptIDRefusesWhatTheScreenRefuses/uppercase_hex
```

So the seam is PINNED against a Go-side drift and against the minter. It is
NOT ENFORCED anywhere in production, and the CLI's weaker gate is untouched.
Say it plainly in the canto.

---

## P3-3 — The cross-language mould is a TEXT SCRAPE. It does not watch the predicate the screen applies, and a comment satisfies it.

`internal/action/receipt_shape_test.go:88-92` scrapes
`RECEIPT_ID_RE\s*=\s*/([^/\n]+)/` out of `Approvals.tsx` with `FindSubmatch`
(first match wins, no `const` anchor, no uniqueness check). Its godoc at `:74`
claims: *"Either side moving alone reddens here instead of shipping a receipt
the other cannot read."*

### (a) The screen's APPLIED predicate drifts — the mould stays green (EXECUTED)

`cmd/korvun-desktop/frontend/src/views/Approvals.tsx:190`,
`return RECEIPT_ID_RE.test(id)` → `return /^rcpt_[0-9a-f]{40}$/.test(id)`:

```
$ go test ./internal/action/ -count=1 -run TestReceiptID
ok  	github.com/Sebastian197/korvun/internal/action	0.454s
```

### (b) A COMMENT satisfies the scraper while the real declaration is widened (EXECUTED)

```tsx
/** historical: RECEIPT_ID_RE = /^rcpt_[0-9a-f]{32}$/ before the widening */
const RECEIPT_ID_RE = /^rcpt_[0-9a-f]{40}$/
```

```
$ go test ./internal/action/ -count=1 -run TestReceiptID
ok  	github.com/Sebastian197/korvun/internal/action	0.435s
```

This is rule 2(g) — a guard by NAME/TEXT where the guarantee is about a SITE.
Case (a) would be caught by the frontend's own vitest suite (its fixtures are
32-hex: `Approvals.test.tsx:105,110,1071,1583`) — READ, **NOT EXECUTED**: there
is no `node_modules` in either the audited tree or my copy, so I could not run
vitest. Case (b) is caught by nothing I can find.

The mould's own honesty paragraph ("it compares two SOURCE literals, not two
running implementations") is good and correct. The sentence at `:74` is the one
that is wider than its wire.

---

## P3-4 — `ActionRequestedAt` conflates a MISSING row with an UNREADABLE store, and the caller degrades OPEN on a TOCTOU window the surrounding code fails CLOSED on.

Captured taxonomy (EXECUTED, `internal/action/sqlite/store.go:1836-1849`):

```
MISSING ROW  -> action/sqlite: requested_at of "act_missing": sql: no rows in result set
  errors.Is(err, sql.ErrNoRows)  = true
  errors.Is(err, ErrNotFound)    = false        <-- the package's own sentinel is NOT used
HEALTHY ROW  -> 2026-08-30 10:00:00 +0000 UTC <nil>
CORRUPT CELL -> action/sqlite: requested_at of "act_ok" is unparseable: parsing time "zzz" ...
NULL cell refused by the schema: NOT NULL constraint failed: actions.requested_at
CLOSED STORE -> action/sqlite: requested_at of "act_ok": sql: database is closed
```

`actionRowIsALaterLife` (`internal/cli/receipt.go:564-566`) collapses all of
these into one string: *"the action row's request instant is **unreadable**"*,
and the first arm (`:341`) turns that into a DEGRADED note, exit 0.

The window is real and the file already knows about it. `receipt.go:341` runs
after `ActionRowsPresent` said the row is PRESENT, on a separate statement, on
no shared transaction. If a concurrent prune removes the row in between,
`ActionRequestedAt` returns `sql: no rows`, the helper calls it "unreadable",
and the approval-sabotage accusation is dropped. Forty lines below, the very
same window is handled the opposite way:

```go
case errors.Is(err, actionsqlite.ErrNotFound):
    // Both rows were present a moment ago; a not-found here is the
    // TOCTOU window, not an absence this command can attest.
    fail("custody_mismatch", "the action row for %q vanished between the presence check and the read", r.ActionID)
```

One window, two policies: the old one fails closed, the new one degrades open.
This is the repository's own named pattern — "corruption masquerading as absence
on ANY surface" — with the arrow reversed.

**The race itself is a PREDICTION: NOT EXECUTED.** There is no injectable
seam between `ActionRowsPresent` and `ActionRequestedAt`, and I declined to ship
a flaky timing loop as a capture. The taxonomy above IS executed, and the two
call sites were read.

---

## P3-5 — `SetRetentionFailureObserver` is an unsynchronized exported mutator, under the Store's own "safe from concurrent brain workers" godoc. Data race captured.

`internal/action/sqlite/store.go:1263-1264`:

> "All access flows through one serialized connection (the house single-writer
> discipline), so **method calls are safe from concurrent brain workers**."

`SetRetentionFailureObserver` (`:1796-1798`) is a method and writes a plain
field; `noteRetentionFailure` (`:1786-1789`) reads it from the writer's
goroutine. `writesMu` sits four lines above, added for exactly this reason
("mutex-guarded because callers are concurrent brain workers").

### EXECUTED

```
$ go test ./internal/action/sqlite/ -count=1 -race -run TestADV_observerSetterRace
==================
WARNING: DATA RACE
Read at 0x00c0001282b8 by goroutine 35:
  (*Store).noteRetentionFailure()  store.go:1787
Previous write at 0x00c0001282b8 by goroutine 36:
  (*Store).SetRetentionFailureObserver()  store.go:1797
```

No production path calls it concurrently today (`internal/app/app.go:364`, once,
inside `Build`, before the store is shared), and `SetReceiptSealer`
(`ledger.go:47`) is the same house pattern, so this is P3 and not P2. It is
still a new exported method that breaks its own struct's literal promise and
carries no concurrency contract in its godoc.

---

## P3-6 — The observer runs synchronously, in the writer's goroutine, after the commit, with no `recover`. A panicking or slow observer kills or hangs the committed writer. Undeclared.

`noteRetentionFailure`'s godoc (`store.go:1782-1785`) says *"It **never panics**
on a nil observer"* — precisely true, and precisely the wrong reassurance.

### EXECUTED

```
=== RUN   TestADV_panickingObserverKillsTheCommittedWriter
    ADV F CONFIRMED: RecordAttempt panicked after its commit: the log sink is gone
    and the row IS durable — the caller got a panic for a committed write
```

That is the P1's own shape in a new skin: a durable write whose caller is told
something other than "it worked". A blocking observer blocks the writer for as
long as it blocks (PREDICTION by reading — NOT EXECUTED; the call is a plain
synchronous invocation at `store.go:1788` with no timeout and no goroutine).

In this tree the only observer is `b.logger.Error`, and `WithLogger(nil)` is
ignored (`internal/app/app.go:198-209`, default `slog.Default()` at `:266`), so
there is no live crash. The seam is new, exported, and the canto's §6 "Declared
and not covered" does not name this cost. Re-entrancy was checked and is safe:
no lock is held across the call (`writesMu` is released at `store.go:1767`).

---

## P3-7 — `noteWrite` carries TWO stacked lead godocs, and one of them is false.

`internal/action/sqlite/store.go:1751-1762`:

```
// noteWrite is the periodic half of the retention invariant: every
// pruneEvery-th committed attempt pays the (cheap, bounded) prune, so the
// file stays capped without any scheduler or config.
// noteWrite pays the retention cadence AFTER its caller's transaction has
// committed, and returns NOTHING. ...
// A failure here is real and is NOT swallowed: it goes to retentionFailure. ...
```

The old paragraph was never merged or removed; two sentences both open with the
symbol name.

More than cosmetic: `:1761`, "**A failure here is real and is NOT swallowed**",
is FALSE whenever no observer is wired — which is the default, and which the
struct field's own comment at `:1309-1314` states correctly
("nil means nobody is listening, and then the failure is DROPPED"). One commit,
two contradictory statements about the same branch; the function-level one is
the wider letrero. Tone law.

The train's own mould proves the drop is real —
`TestRetention_noObserverIsADroppedFailureAndNotAPanic` asserts exactly that.

---

## P3-8 — Three of the spec's cited `file:line` references resolve to unrelated lines against the final tree.

`docs/superpowers/specs/2026-09-22-v0161-c-park-commit.md` cites
`store.go:1735` (as `RecordAttempt`'s tail), `store.go:1748` and `store.go:1753`
(as the prune/sweep error returns). Re-derived:

```
1735: 	if state != action.StateAuthorized {
1748: 	return nil
1753: // file stays capped without any scheduler or config.
```

The real sites are `store.go:1747` (the call), `:1771-1773` and `:1777-1779`.
`approvals.go:176`/`:181`, `evidence.go:115`, `identity_v2.go:219`,
`app/approvals.go:67-69` and the two executor ranges DO check out.

The spec also names a mould `TestPark_aFailedCommitIsStillTheCallersError`
(`spec:60`) that does not exist; the shipped control is
`TestRetention_aFailedCommitIsStillTheCallersError`, and it drives
`RecordAttempt`, not the park.

---

## P3-9 — `ledger_reused_id_test.go:43` calls a hand-written `DELETE` "pruned the way retention prunes it". The mould never runs `Store.Prune`.

The test executes `DELETE FROM actions WHERE action_id = 'act_reused_life'`
through a raw connection. That is an attack, which is legitimate; calling it
"the way retention prunes it" is a claim about `Store.Prune` that the mould
never exercises. The canto's §7 question 3 ("Does any mould enter through a
private function instead of the production door? **No**") reads more confidently
than the fixture supports.

**I executed the missing check, and the ficha's premise HOLDS** (EXECUTED,
`internal/action/sqlite`):

```
Store.Prune removed 1 rows, err=<nil>
after the prune, Get -> action/sqlite: action not found: "act_life"
re-using the id through the production writer -> <nil>
```

So `:414`'s premise is real. It is verified by me, not by the train.

---

## P3-10 — The `:279` "declared cost" is narrower than its footprint.

`docs/cantos/V0161-C-2026-09-22.md:73-74`: *"The observer is nil until the app
wires it, and the app wires it in this same train. **A store used outside the
app** drops those failures."*

Three shipped operator verbs open a store outside the app and drive a
cadence-paying writer: `internal/cli/intent.go:429`, `internal/cli/authority.go:48`,
`internal/cli/grant.go:60` — all `RecordAttemptAuthenticated`, all through
`openOperatorStoreSealed` → `OpenOperator` → `open()`, which sets
`capRows: defaultCapRows, pruneEvery: defaultPruneEvery`
(`store.go:1450`, `:1322-1323`). No observer is wired on that path
(`grep -rn SetRetentionFailureObserver` → `internal/app/app.go:364` only).

Practical exposure is small — `writes` is per-process and resets at open, so a
short CLI run rarely reaches the 512th write — and I say so rather than inflate
it. But "a store used outside the app" reads as a hypothetical embedder when it
is three commands in this repository, and the fourth-door gap (P2-1) is
precisely that door.

Out of scope but recorded, since it collides with the above:
`internal/cli/intent.go:105-107` states the operator door is "writing, but no
recovery, **no prune**". `OpenOperator` inherits the default cadence, so the
prune is reachable there. Pre-existing, not touched by this diff, not a finding
against it.

---

# What I attacked and found SOLID

Stated so this verdict is not read as uniform condemnation.

| Claim | How I checked | Result |
|---|---|---|
| Restoring `return s.noteWrite(ctx)` reddens the `:279` moulds | mutation applied, observer kept intact | RED: 3 subtests + `TestRetention_noObserverIsADroppedFailureAndNotAPanic` + the inverted `TestRecordAttempt_periodicPruneFailureNeverSpeaksForACommittedWrite`. Matches the canto |
| `noteWrite`'s counter after the early return | read `store.go:1764-1769` | correct — the increment precedes the `due` test exactly as before; `!due → return` is semantically identical to the old `if due {}` |
| Does any caller still depend on `noteWrite`'s error? | `grep -rn "noteWrite"`; all `tx.Commit()` tails in `internal/action/sqlite/*.go` read | No. Four call sites, all `s.noteWrite(ctx); return nil`. No other post-commit tail in the package returns a post-commit error |
| Is the observer called with a lock held / inside a tx / re-entrantly? | read `store.go:1763-1789` | No. `writesMu` released before; `Prune`/`Sweep` own their transactions and have returned |
| Does `Prune`'s failure leave the store past its cap forever? | read `noteWrite` + `Prune` | No worse than before: the counter still advances and the next 512th write retries. What changed is WHO is told, and P2-2/P3-7 cover that |
| The canto's §4 finding about the terminal seed | removed the seed, ran the mould | RED for the park and the identified writer ("the observer heard 0 failures"), green for `RecordAttempt`. The canto's "two of three would have passed for the wrong reason" is exactly right, and the observer assertion is load-bearing |
| `:414` mutation "the discriminator always answers «same life»" | applied | RED — `TestLedgerCheck_aReusedIdIsNotASabotage` |
| `:414` SECOND arm removed alone | applied | RED — same test, `custody_mismatch: receipt attests "SUCCEEDED" but the action row says "AUTHORIZED"`. Both arms are probed |
| Does the cure disturb an ordinary healthy row? | ran `receipt verify` over a healthy sealed receipt | `exit 0 "OK"`, no `action_id_reused` note. Unchanged |
| Is `runIntentCLI` honestly labelled? | read `internal/cli/intent_test.go:52-57` | Yes — `Run(args, &stdout, &stderr)` in-process; the test says "Not a separate OS process". Honest |
| The inverted test: strictly stronger? | compared old vs new | **Yes, strictly stronger.** The old asserted only `err != nil`. The new asserts `err == nil`, durability via `Get`, an exact-count oracle (`len(heard) != 1`) and the error text. Nothing the old one covered was lost — "the failure is not swallowed" survives on the observer channel |
| `go vet` / full Go suite | `go vet ./internal/action/... ./internal/cli/... ./internal/app/...`; `go test ./... -count=1` | both clean (exit 0) |

---

# The mandatory questions, answered

**What LITERAL guarantee does each piece promise, and where is the exact WIRE?**

- `:279` — "a committed write never reports the cadence's failure, and the
  failure reaches an observer." Wire: `store.go:1763-1789` plus the four
  `s.noteWrite(ctx); return nil` sites (`store.go:1747`, `approvals.go:181`,
  `evidence.go:115`, `identity_v2.go:219`) and `app/app.go:364`. Half one holds
  at all four sites. Half two is unwatched at one of its two branches (P2-2) and
  unwatched at one of the four doors (P2-1).
- `:376` — "the minted receipt has the shape the screen demands, and the two
  descriptions cannot drift apart." Wire: `wire.go:78,84` + the three moulds.
  Holds for the Go side and the minter; does not cover the screen's applied
  predicate (P3-3) and has no production caller (P3-2).
- `:414` — "a reused id is not accused of a sabotage; where undecidable, the
  check degrades." Wire: `receipt.go:341-346`, `:410-415`, `:554-571`,
  `store.go:1836-1849`. It holds only for a forward-moving clock (P2-3), and it
  buys that by making a real sabotage silenceable with one UPDATE (P1-1).

**Which test would turn red if the guarantee were false?** Named above for each,
with the mutation and the captured red — except the sweep branch (none exists),
the fourth door (none exists), the screen's applied predicate (none in Go), and
the equal/backwards-instant case (none, and the docs assert the opposite).

**Which mutation is missing?** Three, all executed by me with the suite green:
`identity_v2.go:219` uncured; the sweep notification dropped; the TS applied
predicate widened.

**Is the evidence level honest?** Yes, where declared. `ledger_reused_id_test.go:37-40`
correctly says in-process CLI, not a separate OS process.
`receipt_shape_test.go:76-79` correctly says two source literals, not two
engines. `retention_after_commit_test.go:31-33` correctly says in-process with a
real trigger. The labels are the most honest part of this train.

**For every test: what would have to be FALSE in the code to go red, and was
that falsehood ever executed?** Executed for six of nine watched branches; the
three misses are P2-1, P2-2 and P3-3.

---

# Scope declaration

**Read in full:** the complete `git diff 32701e0`; the five new files;
`internal/cli/receipt.go:240-575`; `internal/action/sqlite/store.go:1240-1320`,
`:1620-1860`, `:1370-1460`; `internal/action/sqlite/approvals.go:50-200`,
`:290-390`, `:790-820`; `internal/action/sqlite/ledger.go:390-470`;
`internal/action/sqlite/evidence.go` and `identity_v2.go` tails;
`internal/action/receipt.go:34-95`; `internal/action/wire.go`;
`internal/app/app.go:195-270`, `:295-435`; `internal/app/approvals.go:45-85`;
`internal/action/executor/executor.go:548-595`;
`internal/cli/intent.go:88-120`; `internal/cli/receipt.go:95-135`;
`internal/cli/receipt_r15_join_test.go` (whole);
`cmd/korvun-desktop/frontend/src/views/Approvals.tsx:170-195`;
the canto and the spec, sentence by sentence; every `file:line` those two cite.

**Executed:** `go version` (go1.26.6 darwin/amd64); `go vet` on the three
touched package trees; `go test ./... -count=1` on the audited tree (green);
`go test -race` on targeted probes; twelve mutations and nine adversary tests,
all on copies. Every capture in this verdict is verbatim tool output.

**Could not verify:**
- the vitest suite (no `node_modules` in the audited tree or any copy) — so
  P3-3(a)'s mitigation is a READ, not a capture;
- `make quality` end to end — `golangci-lint` is not on PATH
  (`which golangci-lint` → not found), so the canto's "gate green" is verified
  only for `go build`, `go vet` and `go test`, not for lint or coverage;
- `govulncheck` — not run, out of scope for a diff review;
- the TOCTOU race of P3-4 — declared a PREDICTION, not executed;
- a blocking (as opposed to panicking) observer — PREDICTION, not executed;
- every canto sentence describing the AUTHORING process ("Each mutation applied
  ALONE, its red captured, the tree restored"; "`receipt_r15_join_test.go`,
  which went red and said so") — **UNVERIFIABLE** from the tree: the canto
  carries no captured output for any of its eight mutation rows. I re-ran five
  of them myself and all five behaved as claimed; the other three
  (the nil-observer row, the "same mutation" rows) I did not separate.

**Unexamined:** everything outside the diff — `internal/controlapi`, the
desktop harness, the website, CI workflows, the integration gate, migrations
before v15, and the `:279`/`:376`/`:414` fichas' siblings in `docs/HANDOFF.md`.

**Writes I made inside the audited tree:** this file only.

---

# VETO MANTENIDO

One P1 (`internal/cli/receipt.go:341`, `:410`, `:554-571` +
`internal/action/sqlite/store.go:1836-1849`): the `:414` cure makes a real
tamper pass, and it is a regression captured against `32701e0` on both
`ledger check` and `receipt verify`.

Three P2s: the fourth door of the `:279` class has no mould and survives
uncured with a green suite; the sweep branch has no mould and its removal is
invisible; the canto asserts twice a degradation the code does not perform.

Ten P3s, of which P3-1 (stale ficha line citations written by this commit) and
P3-7 ("A failure here is real and is NOT swallowed") are literal violations of
standing repository laws and fold in a line.
