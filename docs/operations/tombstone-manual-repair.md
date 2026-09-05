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

## Procedure

1. **Stop every korvun process and verify it.** There is no external
   lock command — the truth is a process check: `pgrep -f korvun`
   must print NOTHING (this covers the server AND any live CLI
   writer: an in-flight `approvals approve`, a `receipt rotate-key`).
   DECLARED WINDOW: nothing prevents another process from starting
   while your sqlite3 session is open — YOU guarantee exclusivity for
   the whole session; the document cannot. Run the pgrep check BEFORE
   opening your sqlite3 session: once it is open, `pgrep -f korvun`
   will match your own session too (the database path contains
   "korvun"), so a re-check during the session only yields false
   positives. (A sustained lock command is filed to v0.15.1.)

2. **Take a CONSISTENT backup first** (never `cp` on a live WAL set):

   ```
   sqlite3 "<profile>/korvun.db" ".backup '<safe-dir>/korvun-pre-repair.db'"
   ```

3. **Inspect the named row.** Do NOT paste an `approval_id` read from
   a possibly-corrupt database into interpolated SQL — use a bind
   parameter, or retype it after visual inspection:

   ```
   sqlite3 "<profile>/korvun.db"
   sqlite> .param set @apr 'apr_...'
   sqlite> SELECT * FROM approval_tombstones WHERE approval_id = @apr;
   ```

4. **Quarantine.** The FAITHFUL quarantine is the consistent
   `.backup` you already took in step 2 — it preserves every byte,
   type and NULL by construction. The UNIVERSAL way to record the
   exact bytes of the named field, whatever your sqlite3 version, is
   the hex recipe (bind the id as in step 3; the column is the one
   the boot error names):

   ```
   sqlite3 "<profile>/korvun.db"
   sqlite> .param set @apr 'apr_...'
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

6. **Verify after repairing.** Re-run the boot (the migration must
   converge) and verify the surviving evidence:

   ```
   korvun ledger check --config <config>
   korvun receipt verify --config <config> <action-or-receipt-id>
   ```

   Only a green boot plus a clean check closes the incident.
