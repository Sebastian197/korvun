# Piece 3, phase 2: signed intent contracts

> **Status:** approved for TDD.
> Governing source: `design-drafts/2026-09-19-pieza-3-identidad-intencion-autoridad-codex.md`,
> phase 2, with the five director decisions resolved on 2026-09-19.
> External-docs note: this phase uses only the Go standard library and existing
> internal packages. It adds no dependency.

## Goal

Add bounded version 2 intent contracts, strict canonical bytes, Ed25519 terms
and lifecycle evidence, explicit execution bindings, legacy reads, explicit
root adoption, and operator CLI commands. Enforcement in the execution start
door remains pending phase 0. This phase does not change `internal/app`,
`internal/brain`, or the executor.

## Functional requirements

- **FR-INT-01** New contracts are DRAFT. The only general lifecycle edges are
  DRAFT to ACTIVE and ACTIVE to EXPIRED or REVOKED.
- **FR-INT-02** The schema rejects duplicate and unknown keys, unsupported
  versions, negative or floating budgets, oversized input, excessive depth,
  duplicate set members, and invalid registered values.
- **FR-INT-03** Canonical bytes use integers, sorted sets, and UTC timestamps.
  A digest always derives from the bytes read.
- **FR-INT-04** Activation signs exact immutable terms with the active ledger
  key under `korvun.intent.v2`. Lifecycle events use
  `korvun.intent-event.v1` and monotonic revisions.
- **FR-INT-05** A signer that changes the signed object is rejected before any
  row commits. Retired keys verify historical acts and cannot sign new acts.
- **FR-INT-06** Intent use checks terms, signature, head, event chain, and the
  half-open validity window. A valid terms signature cannot override a later
  revocation.
- **FR-INT-07** Execution bindings select an exact intent version and digest.
  Equal specificity is ambiguous. An explicit broken binding never falls back
  to the root.
- **FR-INT-08** Schema 12 migrates transactionally to schema 13 with
  `intent_versions`, `intent_events`, `intent_heads`, `execution_bindings`,
  and `authorization_snapshots`.
- **FR-INT-09** V1 rows remain readable as `legacy_unsigned`. Migration never
  signs or rewrites their historical bytes.
- **FR-INT-10** Root adoption is an explicit signed act. A missing, revoked,
  corrupt, or incompatible existing root is not silently recreated.
- **FR-INT-11** CLI commands create, activate, expire, revoke, inspect, bind,
  adopt, import, and verify version 2 intents.

## Acceptance scenarios

The executable molds are AS-INT-01 through AS-INT-10 from the governing paper.
Each mold has one exact mutation recorded in the phase canto. The tests are
table-driven where multiple cases share a contract.

## Success criteria

- Validation and verification coverage is at least 90 percent.
- New persistence coverage is at least 85 percent.
- Every mold is observed red before production code and green afterwards.
- Every listed mutation is executed and observed red after implementation.
- `go test -race`, gofmt, goimports, golangci-lint, gosec, govulncheck, and
  `make quality` pass over their required scopes.
- `internal/app`, `internal/brain`, and the executor have no diff.

## Decisions folded in

- The migration is 12 to 13 because this master has schema 12. The paper's
  13 to 14 numbering assumed phase 1 had already landed.
- Profile, owner, and actor identifiers are textual references in this phase.
  Phase 1 tables do not exist on this master, so this phase cannot add honest
  foreign keys to them.
- Legacy remains unsigned and needs explicit import. Root adoption is explicit.
- Budget terms use null for unlimited and zero for zero. Budget enforcement is
  deferred to phase 3.
- Revocation governs starts whose commit follows the revocation commit.
- Execution enforcement remains pending phase 0 and is stated as such in the
  canto and CLI help.

## `[NEEDS CLARIFICATION]`

None. The director resolved storage B, identity A, legacy B, budget A, and
revocation A in the governing paper.
