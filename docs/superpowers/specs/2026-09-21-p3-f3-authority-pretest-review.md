# Piece 3, phase 3: pre-test adversarial review

> **Status:** veto lifted on pass 9. RED may begin. This verdict accredits the
> pre-test design only; it is not implementation, GREEN, or mutation evidence.

## Pass 1, 2026-09-21

The independent adversary read the governing paper, current schema, executor,
identity, intents/bindings, approvals, API, config, console, and UI. It returned
`VETO MANTENIDO` with eight findings.

| Finding | Attack | Required correction now in the design |
|---|---|---|
| P1-1 | Caller swaps `Envelope.AuthorityRefs` to a broader valid leaf. | Exact leaf comes from execution binding or the signed current config head; envelope refs are overwritten evidence. Added `TestAuthority_StoreOwnsApplicableLeaf`. |
| P1-2 | Writer rewrites pending snapshot bytes and digest, then start overwrites the original. | Pending snapshot is signed, verified by API, and never overwritten; start has a distinct signed snapshot. Added signature and survival molds. |
| P1-3 | Strict storage, missing identity/binding/authority, console, resume, import, prune, and commit-order mutations survive the core 16. | Added twelve named supporting molds with exact sentinels, zero-write/effect outcomes, mutation, and evidence level requirement. AS-AUTH-04 now has one exact normalization result. |
| P1-4 | Caller narrates safe data/destination while actual arguments use another class/target. | Registered operation-use analyzer derives resources, data tags, and destinations from canonical arguments. Added actual-use mold. |
| P1-5 | Removing a config clause has no verifiable current head. | Added signed per-profile/per-brain config heads advanced atomically on boot/reload; starts require membership. Added reload/pending mold. |
| P1-6 | The proposed lock table did not exist; `action_schema` is not a singleton. | Added constrained, seeded `authority_write_lock`; exact first statement and all-door ordering mold are specified. |
| P1-7 | Revocation A lacked both commit orders; crash probes were not exact; `committed-at` overclaimed. | Added two-order real-pool mold, named child-process probes and probe-removal mutations. Renamed time to authorization time. |
| P2-8 | Full-history recount makes start work and raw evidence growth unbounded. | Signed tail debits make start O(chain). Fixed-size raw window plus signed checkpoint compaction bounds raw rows independently of action prune. Added prune/compaction mold. |

## Novel finding rule

The next pass must either identify a new attack not reduced by these changes or
state, with source evidence, why it lifts the veto. Repeating a pass-1 finding
without testing its correction is not a new pass.

## Pass 2, 2026-09-21

The independent adversary returned `VETO MANTENIDO` with five new findings.

| Finding | Attack | Required correction now in the design |
|---|---|---|
| P1-9 | Compaction removes exact membership for a replayed old action id. | Strict ids carry a store generation. Checkpoint rotation closes a generation wholesale; every old id is conservatively `ErrActionAlreadyStarted`, with only exact pending rows exempted for their one-shot claim. Added compacted-replay mold. |
| P1-10 | An authenticated non-owner issues root authority under another owner's intent. | Ordinary issue requires actor equals signed intent owner. Admin issue is the only exception and records the human. Added non-owner mold. |
| P1-11 | Account id includes version/generation and restores spent to zero. | Stable account digest excludes version, terms digest, config generation, and clause digest. Added v1-to-v2 and reload mold. |
| P2-12 | Deleting a strict pending snapshot looks like legitimate legacy absence. | Added an approval-row strict snapshot marker written atomically at birth. Missing required row is corruption. Added DELETE mutation. |
| P2-13 | A protected reader uses `s.db` inside a one-connection transaction and deadlocks despite correct lock order. | Added real one-connection completion mold and `tx.Query*` to `s.db.Query*` mutations for protected readers. |

## Pass 3, 2026-09-21

The independent adversary returned `VETO MANTENIDO` with three new findings.

| Finding | Attack | Required correction now in the design |
|---|---|---|
| P1-14 | An attacker changes the mutable strict marker to zero and deletes the pending snapshot, making the UI treat a strict request as legacy. | Strict approvals now use the executor-only `apr3_` namespace. That namespace independently requires marker `1` plus its signed snapshot; the signature also seals the marker. Added the joint downgrade-and-delete mold. |
| P1-15 | Narrowing a config clause from every channel to `telegram` changes an account id that contains the channel selector and restores spent budget. | Config account identity is now the stable `(brain principal, tool name)` tuple. Every mutable term is excluded, and the reload mold changes only the channel selector after eight spends. |
| P2-16 | Compaction closes the generation of an id minted before `Submit`, rejecting a fresh action that never started. | Strict ids now mint inside `Submit` only for effectful allow/pending and hold an opaque generation lease through commit. Rotation takes the matching close mutex and requires zero live leases. Added the mint/rotation barrier mold. |

## Pass 4, 2026-09-21

The independent adversary returned `VETO MANTENIDO` with two new findings.

| Finding | Attack | Required correction now in the design |
|---|---|---|
| P1-17 | Rename `apr3_` to `apr_`, clear the marker, and delete the snapshot; every remaining classifier says legacy. | Strict activation now signs a birth-ledger genesis and imports every existing approval as non-strict. Every later birth has a signed cross-linked event. Once activated, a row without its exact event is corruption. The joint mutation now renames the id too. |
| P2-18 | Process B compacts while process A holds only an in-memory generation lease, closing A's fresh id. | The lease design was removed. The store now mints the final id after taking the same SQLite writer ownership used by compaction and persists it before releasing that transaction. The mold uses two live child processes. |

## Pass 5, 2026-09-21

The independent adversary returned `VETO MANTENIDO` with two new findings.

| Finding | Attack | Required correction now in the design |
|---|---|---|
| P1-19 | Delete the entire birth ledger, rename/downgrade the strict row, restart, and let boot sign the manipulated rows as a new legacy import. | Boot no longer activates. A separate authenticated administrative command creates a signed root whose digest is pinned in strict config. Every boot requires that exact root and its ancestry; missing history is corruption. Added a delete-all/restart mold. |
| P1-20 | Approved resume follows the immediate-start recipe, mints a new id, and cannot resolve vanished ingress after restart. | Approved start is now an explicit branch: it reuses the parked action id and verifies stored phase-1 evidence and the pending snapshot. Added a successful cross-process restart/resume mold. |

## Pass 6, 2026-09-21

The independent adversary returned `VETO MANTENIDO` with one new finding.

| Finding | Attack | Required correction now in the design |
|---|---|---|
| P1-21 | `authority activate` signs the trust root without consuming an authenticated one-shot administrative act in the same transaction. | Activation is now an exact `authority.activate` operation under FR-AUTH-06a. The signed root seals its unique actor action id, enabled human operator, reason, profile, and legacy manifest digest. Added a child-process restart mold and omission mutation. |

## Pass 7, 2026-09-21

The independent adversary returned `VETO MANTENIDO` with one new finding.

| Finding | Attack | Required correction now in the design |
|---|---|---|
| P1-22 | Delegate or another mutation accepts a signed act for the wrong operation/parameters because only activation has the full destructive matrix. | Added one table-driven mold over every ordinary and administrative issue/delegate/revoke/import door. It executes missing/invalid identity, wrong operation, wrong parameters, reuse, and non-human mutations at each changed call site and requires zero authority writes. |

## Pass 8, 2026-09-21

The independent adversary returned `VETO MANTENIDO` with one new finding.

| Finding | Attack | Required correction now in the design |
|---|---|---|
| P2-23 | Closed approval birth events and pending snapshots grow forever, while boot must either scan lifetime history or miss a deleted link. | The authority compactor now folds contiguous closed birth/snapshot prefixes into signed checkpoints, retains every pending row and a 65,536 raw suffix, and verifies checkpoint plus suffix at boot. Added a 65,537-row prune/compact/restart mold, gap probe, and three omission mutations. |

## Pass 9, 2026-09-21

The independent adversary returned `VETO LEVANTADO`.

It re-read the complete design, UX contract, governing paper, and current
prune/approval/snapshot anchors. It found no new P1 or P2 defect preventing RED.
It specifically validated the separate signed birth and snapshot checkpoints,
the contiguous closed-prefix rule, retention of every live pending row, bounded
boot verification, the 65,537-row restart mold, and all four P2-23 mutations.

Residuals remain evidence work, not design exemptions: measure the real mold,
execute every declared mutation, and keep coherent whole-database restoration
outside the stated tamper claim.

## Implementation-scope correction

The implementation review returned to the director's 2026-09-19 paper. That
paper requires exact debit and start evidence to survive ordinary pruning; it
does not require archival, checkpoint compaction, or bounded disk growth.
Passes that proposed those additions remain a historical record of the hostile
design exchange, not shipped guarantees. The implemented schema therefore
keeps exact signed rows and excludes compaction from this phase.

The same holds for the STORE GENERATION that passes 2 to 5 lean on (P1-9,
P2-16, P2-18: ids that "carry a store generation", rotation that "closes a
generation"). None of that shipped. In the code the generation is the constant
`authorityEvidenceEpoch = 1`: it is not read inside the writer transaction, it
does not rotate, and no id is ever refused for belonging to a closed one. Exact
replay protection in this phase is the `authorization_starts` primary key and
nothing else. "The mutable authority generation read inside the writer
transaction" is FILED for the next phase, by that name.

Likewise the strict marker: pass 2's P2-12 and pass 3's P1-14 say the pending
snapshot's signature "seals the marker". It does not — the snapshot's canonical
bytes do not carry it. The signed approval-birth EVENT does, and only under an
activated profile (FR-AUTH-12b, corrected).
