# Ledger restore — the D2 procedure

**Status: manual procedure, for `ledger_unreadable` only.** When
`korvun ledger check` prints `ledger standing: ledger_unreadable (…)`, or the
app's «¿Qué pasa hoy?» screen says «El libro no se puede leer: …», the ledger
holds a defect Korvun does not repair and does not recreate: every new act is
refused while the defect is there. This page is how to replace the file.

Confirm the verdict first, with every process stopped (see «Before
anything»). A `korvun` command that judges the ledger while the app founds or
adopts it can name `ledger_unreadable` for an instant with no defect at all:
the judgement reads the identity row and the profile mark separately (a known
limit of v0.16.2).

It is NOT the remedy for the other failures `ledger check` can name, which are
not verdicts on the ledger: `ledger_environment` (the disk or the permissions
around the file), `ledger_busy` (SQLite answered busy: usually another writer
held the ledger past the busy timeout, though two opens born together on a
file that does not exist yet can meet it at once) and `unavailable` (the
ledger could not be checked now). Fix the environment, or wait and check
again; restoring a copy over a healthy ledger throws away everything recorded
since the copy.

## What the file holds

The ledger lives in the profile's storage file: `storage.path`, or by default
`korvun/korvun.db` under the user's configuration directory. That file
also holds this profile's conversations: replacing it replaces both, and
setting it aside sets both aside.

Beside it, SQLite may keep two companion files: `<file>-wal` (committed changes
not yet folded into the main file) and `<file>-shm` (the index of that log). The
same folder holds `korvun.lock`, the profile lock, and `keys/`, the key material
that signs the receipts.

## Before anything

1. **Stop every process that uses the profile**: the app, the desktop window,
   and any `korvun` command. This procedure is a cold restore; a file replaced
   under a running process is not covered here.
2. **Check again, with everything stopped.** Run
   `korvun ledger check --config <profile>`. If it no longer names
   `ledger_unreadable`, stop here: the ledger is not damaged, and nothing on
   this page applies.
3. **Keep the damaged files.** Move the main file, its `-wal` and its `-shm`
   together, unchanged, into a folder of their own. Do not delete them: they are
   the evidence of what failed.
4. **Leave the key material alone.** Nothing in `keys/` is part of the restore.

## Restoring a copy

What you restore depends on what the copy is.

- **A single-file copy** — only the main file, taken with every process stopped
  and nothing left in its `-wal` (TE51 takes it right after
  `PRAGMA wal_checkpoint(TRUNCATE)` with every process stopped). Put it at the
  ledger's path and make sure no `-wal` or `-shm` remains beside it. A stale
  `-wal` left beside a restored copy is read on the next open and brings the
  damage back: TE51 restores a copy beside the damaged ledger's `-wal` and
  reads `ledger_unreadable` again.
- **A main/-wal set** — the main file and its `-wal`, taken together with every
  process stopped: restore the main file and its -wal together, at the ledger's
  path, with no `-shm` beside them.

Never mix files from different moments: a main file from one copy with a `-wal`
from another copy, or from the damaged ledger, is a copy of nothing.

Then check the result before starting Korvun:

- `korvun ledger check --config <profile>` prints the standing the copy had —
  for a founded copy of this profile's own ledger, `ledger standing: ok` with
  this profile as the owner — and says whether the chain is intact;
- `korvun receipt verify --config <profile> <receipt id>` verifies a receipt you
  know was in the copy.

## Without a copy

Set the damaged files aside as in «Before anything» and start Korvun: it starts
a new, empty ledger, without this profile's history and without its
conversations. That ledger is not founded: `korvun ledger check` names it
`legacy_unfounded`, with no receipts. The files set aside stay exactly as they
were.

## What this procedure does not claim

- It is not a hot replacement: every process is stopped before and restarted
  after.
- It makes no claim about durability across a power loss, or about the
  filesystem's own guarantees.
- Its evidence is TE51 in `internal/cli/ledger_e4_test.go`
  (`TestE4_TE51_theRestoreGivesTheCopysHistory`): the compiled `korvun` binary in
  separate OS processes, crashed child processes that leave their commits in the
  `-wal`, and cold restarts. In that test the ledger without a copy is started
  through the store's own opener, not through a full app boot.
