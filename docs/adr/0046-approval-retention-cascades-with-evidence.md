# ADR-0046: Approval rows cascade with retention; the receipt is the evidence

Date: 2026-09-02
Status: Accepted (supersedes the second-pass exemption of 2026-09-01)

## Context

The E5 consolidation declared approval rows retention-exempt ("they
are the 8th check's evidence, never pruned"). The third audit's P2:
that exemption bounded nothing — approvals grew one row per parked
action FOREVER, and the claimed evidentiary role was already served
better by the receipt: since NC-3α the CONSUMED approval's decision
digest is sealed INSIDE the signed v2 receipt (preview digest and law
pin included since C2), which survives retention by design.

## Decision

- `approvals.action_id` becomes a real FK with ON DELETE CASCADE:
  when the retention prune removes a terminal action, its approval row
  goes with it. Live rows are safe by construction — the prune only
  ever deletes TERMINAL actions, and a PENDING_APPROVAL action is not
  terminal.
- The surviving evidence of an approval is its SIGNED receipt: the
  sealed approval_digest (decision terms + preview digest + law pin).
  The verifier distinguishes honestly: action row present + approval
  row absent = approval_mismatch (sabotage — a cascade cannot remove
  the approval while its action remains); both absent =
  approval_row_absent, a named NOTE (retention took the rows, the
  digest-sealed receipt is the evidence that remains) — the exact
  action_row_absent mold.
- v8→v9 by transactional table reconstruction (SQLite cannot add a
  constraint in place), one transaction with the version bump — the
  AS-8 anti-zombie mold, crash-rehearsed. Orphan approval rows
  predating v9 (action pruned under the old exemption) are RETIRED by
  the copy filter: their receipt is their evidence, exactly as if the
  cascade had run when their action was pruned.

## Consequences

- The approvals table is bounded by the same retention cap as
  actions (at most one approval per surviving action row).
- Historical stores migrate with a one-time retirement of orphans —
  declared here, never silent.
- The second-pass exemption text in spec/comments is superseded and
  purged in-phase.
