# Internal adversary — the SHORT pass over the boot-refusal cure

Object: the cure of the non-strict boot over an activated profile, over the tree
of `2d69e98` plus the uncommitted cure. Date: 2026-09-22. The text below is the
auditor's verdict VERBATIM, persisted before any cure of the cure began.

------------------------------------------------------------------------

VETO MANTENIDO

P1 — The cure converts "runs, at risk" into "does not run, unrecoverable": a
profile whose activation digest the operator no longer has can no longer boot AT
ALL, and the refusal's own remedy is unreachable through every shipped door.
The refusal tells the operator to add "activation_digest", but it does not print
the digest although it reads the very row that holds it; no CLI verb prints it
after the one line `authority activate` emits; and re-activation is refused.
Reproduced with the shipped binary built from this tree: activate, lose the
line, boot -> fatal; `authority --help`, `ledger check`, `approvals list`,
`intent show` -> the digest appears nowhere; re-activate -> «authorization
snapshot corrupt». The digest is on disk the whole time, in the row the new
reader already SELECTs (config_authority.go:53 selects profile_id ONLY).
The director's ruling quoted in the cure's own release text is "a path which
bricks a profile is not published with a warning". This cure introduces one.

P2 — Two legitimate activations on one store are reported as CORRUPTION, and the
named sentinel is lost on that path. `authority activate` accepts any --profile
and only blocks re-activating the SAME id, so two activations leave two rows;
the new reader calls that corruption while the strict boot over the identical
store accepts it and starts. errors.Is(err, ErrAuthorityActivatedProfileNeedsStrict)
is false there. Reproduced with the binary: two activations both exit 0; the
non-strict boot dies with «authorization snapshot corrupt»; the strict boot
pinning the first digest passes the whole authority gate.

P2 — The mould's declared evidence contains a capture the mould cannot produce:
«and the control row showed the poisoned state that follows». No row parks
anything; under the executed mutation both control rows PASS.

P2 — Two of the three arms of the new exported reader have no test and no red
mutation. M3 (multi-row arm -> return profiles[0]) and M2a (fail OPEN on every
read error -> return "", nil) both leave the whole suite green.

P3 — `actions == nil` fails OPEN where its sibling `PrepareStrictAuthority`
fails CLOSED for the same input.
P3 — The new godoc locates a symbol by a relative position ("the armed accessor
above"), against the 2026-09-08 law.
P3 — The CLI still runs unarmed over an activated profile; it cannot park, so
the cure's claim survives, but the consequence of deciding/executing unarmed is
a prediction the auditor did not execute. Pre-existing.
P3 — Preflight cannot see the new refusal class, so a reload that drops the
block costs a failed cutover instead of a cheap rejection. Bounded by rollback.
P3 — Activation is irreversible and nothing says so.

WHAT THE CURE DOES ACHIEVE: the targeted path is closed; M1 (delete the
refusal) reddens rows 1 and 2; no existing test regresses; no deadlock on the
one-connection pool; every error arm is fail-closed as written; the fixture is
honest that the app registry has no human principal and only the CLI shape can
activate; the evidence label is accurate.

Scope, executed and unexamined items: as recorded in the session transcript of
this pass. The worktree was never written to by the auditor.
