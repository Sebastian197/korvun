# Tombstone manual repair — expert procedure

**Status: manual expert procedure.** This is NOT a Korvun-supported
repair feature: Korvun never rewrites evidence. When the v12
re-validation (or the v10→v11 copy) halts the boot naming a tombstone
row and field, a HUMAN adjudicates with the steps below.

**Prerequisite:** the `sqlite3` command-line shell. Commands below are
verified on macOS (`sqlite3` ships with the OS); on Linux install the
`sqlite3` package; on Windows the commands are UNVERIFIED — use the
sqlite.org shell and adapt paths.

The boot error names the row (`approval_id`) and the field. Deleting
or altering evidence is an adjudication decision, not maintenance —
whatever you change, you own.

**Not only the boot points here.** A decided close can also refuse
with the same `tombstone_corrupt` fault when its tombstone's
re-insert collides with a stored row that fails the contract. Since
R13 every read of a tombstone row judges the STORAGE CLASS of every
column next to its value, so such a row is named by its column and
its class — for example `approval_id (storage class blob)`: the key
column no longer holds TEXT. That row will NOT answer `WHERE
approval_id = @apr` in steps 3 and 4 — the engine's `=` and the
primary key index do not equate a BLOB to its own text. Find it by the
value's bytes instead, and read its storage class:

```
sqlite> SELECT typeof(approval_id), hex(approval_id) FROM approval_tombstones
   ...>  WHERE CAST(approval_id AS TEXT) = @apr;
```

Declared limit of this era — the LOOKUP column: the column a reader
looks a row up BY (`approval_digest` for `receipt verify`, `action_id`
for the by-action reader, `approval_id` for the decided close's own
re-insert) is not judged by that reader, because a row whose lookup
column changed class is never selected — it is INDISTINGUISHABLE FROM
ABSENCE for that reader (`receipt verify` prints
`approval_row_absent`). The lookups stay indexed on purpose; a
full-table scan verifier that judges every row is filed to v0.15.1.

## Procedure

1. **Stop every korvun process and verify it.** There is no external
   lock command — the truth is a process check, and `pgrep -f korvun`
   is a PARTIAL one, declared: it matches the WORD in any command
   line, so it names the server AND any live CLI writer (an in-flight
   `approvals approve`, a `receipt rotate-key`) — but also any
   unrelated process whose command line carries the word (a shell
   whose argument is a path under a `korvun/` directory: a FALSE
   POSITIVE), and it MISSES a korvun binary renamed or run from a
   path without the word (a FALSE NEGATIVE). Read its output by
   process, never by count: an empty list is necessary, not proof.
   DECLARED WINDOW: nothing prevents another process from starting
   while your sqlite3 session is open — YOU guarantee exclusivity for
   the whole session; the document cannot (the race is not closable
   by a check). Run the pgrep check BEFORE opening your sqlite3
   session: once it is open, `pgrep -f korvun` will match your own
   session too (the database path contains "korvun"), so a re-check
   during the session only yields false positives. (A sustained lock
   command is filed to v0.15.1.)

2. **Take a CONSISTENT backup first** (never `cp` on a live WAL set):

   ```
   sqlite3 "<profile>/korvun.db" ".backup '<safe-dir>/korvun-pre-repair.db'"
   ```

3. **Inspect the named row.** Do NOT paste an `approval_id` read from
   a possibly-corrupt database into interpolated SQL — bind it with
   `.param`, in DOUBLE quotes, and retype it after visual inspection
   (the `.param` line is an interpolation point too: a hostile id is
   typed there by YOUR hand, never pasted):

   ```
   sqlite3 "<profile>/korvun.db"
   sqlite> .param set @apr "apr_..."
   sqlite> SELECT * FROM approval_tombstones WHERE approval_id = @apr;
   ```

   Why double quotes — CAPTURED on sqlite3 3.39.5: the dot-command
   tokenizer does not accept the SQL spelling of a single quote inside
   a single-quoted value (`.param set @apr 'it''s'` prints the
   `.parameter` usage, exits 0 and leaves `@apr` UNBOUND, so the
   following SELECT silently matches nothing); `.param set @apr
   "it's"` binds the exact bytes. An id carrying a double quote is
   escaped as `\"` inside the double quotes.

4. **Quarantine.** The FAITHFUL quarantine is the consistent
   `.backup` you already took in step 2 — it preserves every byte,
   type and NULL by construction. The UNIVERSAL way to record the
   exact bytes of the named field, whatever your sqlite3 version, is
   the hex recipe (bind the id as in step 3; the column is the one
   the boot error names):

   ```
   sqlite3 "<profile>/korvun.db"
   sqlite> .param set @apr "apr_..."
   sqlite> SELECT hex(decision_at) FROM approval_tombstones WHERE approval_id = @apr;
   ```

   A `.dump` is a readable CONVENIENCE, not the faithful record, and
   its rendering of TEXT is version-dependent. Observed with sqlite3
   3.39.5 (macOS): `.dump` TRUNCATES a TEXT value after an embedded
   NUL byte. From sqlite3 3.50.0 the shell encodes special characters
   through `unistr()` (its changelog, 2025-05-29); what it does with an
   embedded NUL there is NOT verified by this project. Do not rely on
   a dump for exact bytes on any version:

   ```
   sqlite3 "<profile>/korvun.db" ".dump approval_tombstones" > quarantine-tombstones.sql
   ```

5. **Adjudicate.** A digest does not let you recover an original
   value. A correction is acceptable ONLY with the exact preimage
   obtained from independent evidence — era logs, previous backups,
   prior exports. (A v2 receipt is NOT such a source: it seals only
   the approval digest, not the fields themselves.) "Fixing
   the field so the digest matches" is forbidden — that is
   fabrication. If no independent evidence exists, the alternatives
   are: keep the profile on its current version (do not upgrade), or
   remove the row AFTER quarantining it, accepting in writing that
   the evidence is lost.

6. **Boot FIRST, then verify.** The order matters: `korvun ledger
   check` and `korvun receipt verify` open the store READ-ONLY and
   REFUSE a profile whose schema is behind the binary's, by name ("is
   at schema v11, this binary reads v12 — a read-only consult never
   migrates; run the server boot to lift the schema" — captured from
   the binary). So:

   1. re-run the boot (`korvun serve --config "<config>"`; the migration
      must converge — a boot that halts again names the next row);
   2. then verify the surviving evidence:

   ```
   korvun ledger check --config "<config>"
   korvun receipt verify --config "<config>" "<action-or-receipt-id>"
   ```

   Only a green boot plus a clean check closes the incident. (An empty
   partition reports `0 receipts, chain intact` — a reading of the
   binary, see SECURITY.md on what the verifier cannot detect.)
