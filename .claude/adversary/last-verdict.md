VETO LEVANTADO 5b4479ab046cc3f0aca370ef2589b78e90eadbfc
Round: governance train, the prompt audit re-based on master (2026-10-10).
Legacy marker by the director's order: the versioned procedure of
docs/INTEGRATION.md cannot be followed without force for a new train (P2
of the procedure, adjudicated 2026-10-10; the checker cure goes in its own
PR). After rebase this marker names a rewritten SHA; the director repairs
master's marker by hand, as after #69 and #74.

WHAT THE TRAIN IS. Two commits, docs and agent instructions only, no
production code. e88bf3a records the pending text already decided ("Merges
are NEVER squashed", "Dependency alerts", the adversary's "YOU EXECUTE"
section) verbatim; the second commit applies the audit as adjudicated
(findings 1-8 and 11; 9 and 10 rejected), adds ADR-0047, tracks AGENTS.md
aligned with master, and records in HANDOFF that the v0.16.2 tag waits for
this PR and a green master CI.

THE ADVERSARY'S SHORT PASS (one pass, delta only): VETO MANTENIDO on one
P2 — AGENTS.md entered tracked with "41 Go packages" under "verified
2026-09-12" while master has 43 (make guard-gopkgs on 1e22640: 43; on
265dc1a: 41), and the ADR row became 47 only through ADR-0047. CURED and
declared: the stack table re-verified on 2026-10-10 against the final tree
(43 packages, 47 ADRs, npm 12.0.2 locally).
P3 dispositions: (a) the adversary catalog wording is the rewrite the
director accepted for finding 8; (b) commit 1 carries the old merges text
verbatim by the director's order, corrected by commit 2 in the same PR;
(c) "below" in AGENTS.md -> "in this file", cured; (d) the letrero clause
restored in "Paper passes", cured; (e) the frontend standards name their
scope (builder and desktop; the website has its own gate), cured; (f)
ADR-0047 carries the three dates (2026-09-06, 2026-09-08, 2026-10-10),
cured.
Verified by the adversary: repo settings (squash and merge commit
disabled, rebase allowed); the checker accepts d20dea6 (versioned) and
rejects 1e22640 (legacy, rewritten); #69/#74 SHAs and trees; the reflow
changes no word; the quality target list equals the Makefile; no heading
keeps a pressure suffix except the destructive testing doctrine.
Not verified: GitHub's server-side behaviour; /memory display of the file.
