# Contributing to Korvun

Thanks for your interest in Korvun. This project follows a strict, deliberate
workflow — the rules below are non-negotiable because they keep a cross-platform,
security-sensitive binary correct. Please read this in full before opening a PR.

The authoritative, always-current version of these rules lives in
[`CLAUDE.md`](CLAUDE.md) at the repo root.

## Language

- Code, identifiers, comments, commit messages and in-repo docs: **English**.
- (Project planning conversation happens in Spanish, but the repository is
  English-only.)

## The workflow — per phase, not just per stage

Work proceeds in strict granularity: **Stage → Phase → Task**. For every unit of
work:

1. **Verify external docs first.** Before writing any code against a library,
   framework, SDK, or external API, verify it — never from memory.
   - **Code libraries/frameworks** (Go packages, SDKs, drivers): consult
     version-specific docs via **Context7** first. If it can't be verified, stop
     — do not guess signatures or versions.
   - **GitHub Actions / CI tooling:** verify the current tag at the Action's own
     repo/releases or the Marketplace (Context7 does not cover Action tags).
     Prefer pinning Actions to a full commit SHA, or at least a fixed major tag.
2. **Design spec first.** For non-trivial work, draft the design spec from
   `docs/superpowers/specs/TEMPLATE.md` (Goal, FR-IDs, acceptance scenarios,
   success criteria) before code.
3. **Tests first (TDD).** Write the test that defines the contract, confirm it
   fails (red), then write the minimum code to make it pass (green).
4. **Implementation.** Only the code needed to pass the tests.
5. **Quality gate.** `make quality` must be green over the *whole* suite.
6. **Documentation.** Update stage docs, ADRs, and the master document.

## Before every commit

```sh
make quality      # gofmt + goimports + go vet + golangci-lint + tests + coverage
go test -race ./... # tests must pass with the race detector
```

`make quality` runs lint (`gofmt`, `goimports`, `go vet`, `golangci-lint` with
govet/staticcheck/errcheck/gosec), the full test suite with `-race`, and the
coverage gate.

## Go standards

- `gofmt` + `goimports` are mandatory; `golangci-lint` must pass.
- Wrap errors with `%w`; pass `context.Context` on every cancellable operation.
- No mutable global state; no `panic` on normal paths.
- Tests are table-driven and run with `-race`.
- **Coverage:** ≥ 85% in core packages; ≥ 90% in `policy`, `router`, `envelope`,
  `brain`.
- Every exported symbol has a godoc comment.
- Every source file starts with:
  ```go
  // Copyright 2026 Sebastián Moreno Saavedra
  // SPDX-License-Identifier: Apache-2.0
  ```

## Dependencies

- Prefer the Go standard library whenever reasonable.
- **Every new external dependency requires a justifying ADR** in `docs/adr/`
  *and* Context7 verification before adoption. New deps must pass the four-axis
  test (dependency size vs hand-roll cost vs API volatility vs maintenance gain).

## Branching and integration

- Work on a feature branch (`feat/...`, `fix/...`, `chore/...`, `docs/...`).
- **ADRs land on `master` first**, before the code they govern.
- Integrate with a `--no-ff` merge so the branch history is preserved.
- Keep `master` green: do not advance a phase until the whole tree passes
  `make quality` with `-race`.

## Commit messages — critical

- Use **Conventional Commits**: `feat:`, `fix:`, `test:`, `docs:`, `refactor:`,
  `chore:`, etc. **SemVer** for versioning.
- **Commit messages and PR descriptions MUST NOT contain any AI attribution** —
  no "Generated with / by", no co-author trailers, no reference to any assistant.

## Pull requests

Open a PR against `master` and fill in the
[PR template](.github/PULL_REQUEST_TEMPLATE.md) checklist. A PR is reviewable
when `make quality` is green, tests were written before the implementation, an
ADR exists for any new external dependency, and exported symbols are documented.

## Security

Never commit secrets. Report vulnerabilities privately — see
[SECURITY.md](SECURITY.md), not a public issue.

## Code of conduct

Be respectful and constructive. Assume good faith and keep discussion technical.