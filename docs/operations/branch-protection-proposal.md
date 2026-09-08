# Branch protection for `master` — APPLIED 2026-09-08

> **STATUS: APPLIED.** This is no longer a proposal. Verified against the
> GitHub API on 2026-09-08 from this session: the ACTIVE ruleset is
> `proteger-master` (id 20886440, enforcement `active`, target
> `~DEFAULT_BRANCH`); the older `protect-master` (id 18081696) is
> `disabled`. It requires ELEVEN status checks — `quality (ubuntu-latest)`,
> `quality (macos-latest)`, `quality (windows-latest)`, `sbom`,
> `Analyze (Go)`, and `cross-compile` for the six GOOS/GOARCH pairs — plus
> the `deletion` and `non_fast_forward` rules. `strict_required_status_checks_policy`
> is false, so a PR need not be rebased onto the tip to merge.
>
> Note on reading it: `GET /repos/.../branches/master` reports only TEN of
> the eleven (it omits `Analyze (Go)`), because that endpoint shows classic
> branch protection and the eleventh lives in the ruleset. The ruleset
> endpoint is the authoritative one.
>
> **Every check reports on a PR against master.** Verified in the tree:
> `quality.yml` and `codeql.yml` both carry `pull_request: branches:
> [master]` with NO `paths:` filter, `Analyze (Go)` is the literal `name:`
> of codeql.yml's `analyze` job, and `cross-compile`/`sbom` are jobs of
> `quality.yml` with `needs: quality`. Nothing blocks a PR forever by
> never reporting; a `quality` failure SKIPS its two dependents, which
> holds the PR until `quality` is green — the gate working, not a defect.
>
> The text below is preserved as the proposal that was applied.


Written 2026-09-08 for the PR flow (CLAUDE.md, "The PR flow"). The
copilot PREPARES this; only Chano applies it, in Settings → Branches →
Add branch ruleset (or classic branch protection) for `master`.

Apply it BEFORE the merge of R15, after R15's PR is open — the order
matters only in that a ruleset applied mid-review does not invalidate
an open PR.

## What to turn on

- **Require a pull request before merging: YES.**
- **Required approvals: 0.** Single author. GitHub does not allow
  self-approval, so any number above zero deadlocks the repository.
  This is a deliberate, declared limitation, not an oversight — it is
  also why OpenSSF Scorecard's Code-Review check cannot be satisfied
  and stays declared as structural.
- **Dismiss stale approvals:** irrelevant at zero approvals; leave off.
- **Require status checks to pass before merging: YES**, with "Require
  branches to be up to date before merging" ON.
- **Require conversation resolution before merging: YES.**
- **Do not allow bypassing the above settings:** ON for everyone,
  including admins — the point of the flow is public evidence, and an
  admin bypass erases it. (If a genuine emergency needs it, the
  setting is toggled off, used, and toggled back on, and the fact is
  recorded in the train's canto.)
- **Allow force pushes / deletions: OFF.**
- **Linear history:** optional; today's history is already linear.

## Which checks to require — and which NOT to

Require ONLY checks that run on EVERY pull request against `master`.
A path-filtered workflow does not report a check on a PR that misses
its paths, and a required check that never reports blocks the merge
forever. Verified against `.github/workflows/` on 2026-09-08:

REQUIRE (they run on every PR to master):

- `quality (ubuntu-latest)`
- `quality (macos-latest)`
- `quality (windows-latest)`
- `cross-compile (…)` — the six matrix legs (linux/windows/darwin ×
  amd64/arm64), which run `needs: quality`
- `sbom`
- `Analyze (Go)` — CodeQL

DO NOT REQUIRE (path-filtered; they simply do not report when the PR
does not touch their paths):

- `builder (lint · typecheck · test · build)`, `builder e2e (…)`,
  `chrome (…)`, `chrome e2e (…)` — `frontend.yml`, filtered to
  `web/builder/**`, `cmd/korvun-desktop/frontend/**`,
  `cmd/korvun-desktop/e2e-harness/**` and its own file.
- `website (full harness via make website-check)` — `website-pr.yml`,
  filtered to `website/**` and its own file.

The exact context strings must be copied from a real run's check list
(Settings shows the ones GitHub has seen recently); a typo in a
required context is the same deadlock as a path-filtered one.

## What this does NOT change

The `ensayo` rehearsal stays exactly where it is: the full gate runs
there BEFORE master advances, and the PR is opened from the train's
branch. The PR is public evidence of the discipline, not a
replacement for the rehearsal.
