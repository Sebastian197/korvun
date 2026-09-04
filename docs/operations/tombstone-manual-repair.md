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

1. **Stop the server and take the profile lock.** No korvun process
   may be running against the profile. Do not proceed while anything
   holds the database.

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

4. **Quarantine the row preserving bytes, types and NULLs** — use the
   dump form (a literal INSERT), never an ambiguous textual export:

   ```
   sqlite3 "<profile>/korvun.db" ".dump approval_tombstones" > quarantine-tombstones.sql
   ```

   (Keep only the INSERT line of the named row alongside the full dump.)

5. **Adjudicate.** A digest does not let you recover an original
   value. A correction is acceptable ONLY with the exact preimage
   obtained from independent evidence (receipts, era logs). "Fixing
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
