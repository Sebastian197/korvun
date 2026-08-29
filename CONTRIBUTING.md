# Contributing to Korvun

Korvun is built under a strict, deliberate engineering method. This document
describes that method as it IS — not as an aspiration. The authoritative,
always-current rules live in [`CLAUDE.md`](CLAUDE.md) at the repo root; when
the two disagree, `CLAUDE.md` wins.

## Language

- Code, identifiers, comments, commit messages and in-repo docs: **English**.
- Project planning conversation happens in Spanish; the repository is
  English-only.

## The method, per phase

Work proceeds in strict granularity: **Stage → Phase → Task**. Every phase
walks the same cycle:

1. **Verify external docs first.** Never program against a library, SDK, API,
   or CI tool from memory. Code libraries are verified through Context7
   (version-specific docs) before any code; GitHub Actions and CI tooling are
   verified at their own releases page or the Marketplace, pinned to a full
   commit SHA or at least a fixed major tag. If verification is impossible,
   the work stops and says so.
2. **Design spec first.** Non-trivial work starts from
   `docs/superpowers/specs/TEMPLATE.md` — goal, functional requirements,
   Given/When/Then acceptance scenarios, success criteria. Anything undefined
   is marked `[NEEDS CLARIFICATION]` and blocks the next step until resolved.
   Substantial specs receive **adversarial review** (an independent
   cross-review pass) before they are accepted for TDD; the findings are
   triaged with evidence, not adopted wholesale.
3. **Tests first — and the approved RED is a contract.** The test suite that
   defines the contract is written BEFORE the implementation and confirmed
   failing (red). Once a red suite is reviewed and approved, it is not
   adjusted during the green step without asking first: diagnose, then change
   only after an explicit yes. Weakening an approved test to make an
   implementation pass is forbidden.
4. **Implementation.** Only the code needed to make the tests pass.
5. **Quality gate.** `make quality` must be green over the WHOLE suite —
   lint (`gofmt`, `goimports`, `go vet`, `golangci-lint` with
   govet/staticcheck/errcheck/gosec), the full test suite with `-race`, and
   the coverage gate (≥ 85% across `internal/`; ≥ 90% in `policy`, `router`,
   `envelope`, `brain`). When desktop chrome sources changed,
   `make desktop-frontend-check` mirrors the CI chrome lane locally
   (typecheck · lint · prettier · coverage).
6. **Documentation.** Stage docs, ADRs, and the master document are updated
   before the phase closes.

Two more standing laws shape the cycle:

- **Model-dependent behavior needs a real model.** Anything whose behavior
  depends on what a model actually emits (tool-use protocol, prompt
  contracts, output parsing) is exercised against a REAL model in its own
  sub-phase. A green suite over fakes proves our code, never the contract.
- **UX-design-first (the Sixth Law).** No user-visible piece opens its RED
  phase without an experience design (from
  `docs/superpowers/specs/UX-TEMPLATE.md`) approved by the project director
  over RENDERED mockups — prose alone is never approved. No release is
  tagged without the director's manual pass over the packaged build; the bug
  bash is a permanent customs gate, not an event.

## Integration: rehearsal before master

`master` is never red. The integration path is:

1. Run `govulncheck ./...` locally with the pinned toolchain BEFORE every
   rehearsal — a green rehearsal expires as advisories land. New reachable
   advisories are fixed at the source (toolchain or dependency bump), never
   by silencing the scanner.
2. Push the batch to the `ensayo` branch and wait for the FULL green
   (Quality Gate across the three OSes + the Frontend lanes).
3. Only with the rehearsal green does the same tree get pushed to `master` —
   as its own deliberate step, never bundled inside another task.

## Commits

- **Conventional Commits** (`feat:`, `fix:`, `test:`, `docs:`, `refactor:`,
  `chore:`, …) and **SemVer** for versions.
- Commit messages and PR descriptions carry **no AI attribution** of any
  kind — no "Generated with/by", no co-author trailers, no assistant names.

## Go standards

- `gofmt` + `goimports` mandatory; `golangci-lint` must pass.
- Errors wrapped with `%w`; `context.Context` on every cancellable operation.
- No mutable global state; no `panic` on normal paths.
- Tests are table-driven and run with `-race`.
- Every exported symbol has a godoc comment.
- Every source file starts with:
  ```go
  // Copyright 2026 Sebastián Moreno Saavedra
  // SPDX-License-Identifier: Apache-2.0
  ```

## Invariants over examples

Subsystems with security, privacy, routing, lifecycle, or resource
consequences are specified and tested as INVARIANTS, not happy-path samples:
private data flows only to explicitly trusted sinks; unauthenticated control
surfaces are loopback-only; cancelled work stops and releases what it owns;
no externally-driven path grows unbounded; fallback is driven by explicit
error semantics (a permanent failure never silently takes the transient
path).

## Dependencies

- Prefer the Go standard library whenever reasonable.
- Every new external dependency requires a justifying ADR in `docs/adr/`
  AND Context7 verification before adoption.
- Frontend lockfiles are regenerated with `npm install --include=optional`
  and `npm ci` verified clean twice in a row; floating transitives are
  pinned with exact `overrides`.

## Releases

Releases are **draft-until-complete**: the tag's release stays a draft until
every artifact of both families (headless + desktop) is uploaded and signed —
cosign keyless signatures and SPDX SBOMs per platform — and only then is it
published. The packaged build passes the director's manual bug bash before
any tag. See [SECURITY.md](SECURITY.md) for the verification commands.

## Pull requests

Open PRs against `master` and fill the
[PR template](.github/PULL_REQUEST_TEMPLATE.md). A PR is reviewable when
`make quality` is green, tests were written before the implementation, an ADR
exists for any new dependency, and exported symbols are documented.

## Security

Never commit secrets. Report vulnerabilities privately — see
[SECURITY.md](SECURITY.md), never a public issue.

## Code of conduct

Be respectful and constructive. Assume good faith and keep discussion
technical.
