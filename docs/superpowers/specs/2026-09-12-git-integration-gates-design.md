# Git integration controls, Phase A: Design spec

> Status: approved for TDD by the originating copilot before the first RED.
> The approval's conditions are recorded in the pre-test review. The first
> publication includes verifier/tests/workers and excludes integration.yml;
> activation remains a separate, unverified step.
> Baseline: c6ccd242b8539890593dfeced652a6d322fe2531.
> Governing decisions: CLAUDE.md delivery/review/testing rules; ADR-0025,
> ADR-0026, ADR-0029, ADR-0040. No accepted ADR is rewritten.
> GitHub primary sources and API responses: see the verification report. Only Python
> standard library and existing Git/Bash tooling are used; no new library or App.

## Goal

Add a PR check that evaluates applicable existing CI results and exact-code
review evidence. Keep the director's integration decision and the local marker
contract. Evidence authenticates the GitHub custodian's declaration, not the
external reviewer's identity or quality of work. Deliver local implementation,
attack tests and operating instructions; publish and activate in later authorized
steps, without installing an external service or inventing a reviewer.

## Functional requirements

- FR-1: `scripts/integration_gate.py` requires the current open PR's head H and
  base B, requires B to be an ancestor of H, and checks H with the existing
  marker script. H is a pure marker commit on code parent C. Its tree and the
  complete changed-file scope are bound to the evidence.
- FR-2: The reducer uses all pages of changed files, old and new rename paths,
  deletions and unknown path classes. Missing pages/count mismatches block.
  Go and CodeQL always apply. Frontend applies to Go/frontend shared code.
  Website applies to public docs/assets/site. Shared tooling, module manifests
  and unknown paths run all lanes. Internal specs/ADRs/stage notes alone do not
  require frontend/site. The reference implementation defines exact path rules.
- FR-3: Collect actual Actions runs using repository ID, workflow ID/path, PR,
  head SHA and event. Select the newest run and its current attempt. Require
  each expected job and all returned execution steps to be completed/success,
  except the explicitly documented Windows Unix-hook probe skip. Missing,
  duplicate, failed, cancelled, neutral or unexpectedly skipped jobs block.
- FR-4: Collect a submitted GitHub COMMENT review (returned state COMMENTED)
  whose authenticated custodian is configured in trusted-base policy. Its JSON
  binds repository, PR, H, B, C, tree, scope, declared reviewer and origin,
  accessible inline report bytes and SHA256, zero P1/P2 and each P3 disposition.
  Reviewer/origin configuration starts empty; without a real choice it blocks.
  Pending/dismissed evidence and outstanding requests for changes block.
- FR-5: Changes of workflows, hooks, scripts, Makefile or governing instructions
  require explicit governance-scope acknowledgment in the authenticated record.
  The CI verifier and policy execute from B. Candidate-controlled commands do not
  run with privileged credentials. The check is not resistant to an administrator
  deliberately changing the verification workflow or forging review declarations.
- FR-6: Read head, base, review bodies/states and current run attempts again before
  returning success. Any change during collection fails. Bound response size,
  pagination, each network call and total wait; API failures never mean absence
  or inapplicability. Changed state after return is outside this guarantee.
- FR-7 (second delivery): `integration.yml` will run for PR lifecycle and review events without path
  filters. Existing worker commands stay separate. Frontend/site PR filters are
  removed so applicable shared changes can produce evidence; irrelevant worker
  failures do not affect the reducer. Push/release behavior stays unchanged.
- FR-8: `make integration-probe` and CI run the attack suite. `make quality`
  includes it. Mutation probes run in temporary copies and leave the checkout
  intact. No fake verdict or synthetic review is submitted remotely.

## Acceptance scenarios (Given / When / Then)

- AS-1: Given green Go and failed applicable website, when evaluated, then
  CHECK_FAILED/CHECK_NOT_SUCCESS names the failed job/run. Missing/duplicate/
  stale job evidence yields its distinct named failure.
- AS-2: Given an internal-spec-only change and a failed unrelated frontend job,
  when classifying, then only Go/CodeQL are required. Public docs and module
  changes have their conservative website/shared applicability.
- AS-3: Given skipped/neutral/cancelled job or mandatory step, when evaluated,
  then CHECK_NOT_SUCCESS/STEP_NOT_SUCCESS, not success.
- AS-4: Given a rename from website into docs or a later failed API page, when
  collecting, then website remains applicable or INPUT_INCOMPLETE/INPUT_UNAVAILABLE
  blocks. No partial file list may pass.
- AS-5: Given multiple runs or a new cancelled attempt, when collecting, then
  the newest current attempt controls; stale successful evidence cannot replace it.
- AS-6: Given wrong workflow/event/repository/PR/head, when selected, then
  RUN_SOURCE_MISMATCH. Check names alone do not establish origin.
- AS-7: Given a forged recorder, unknown reviewer/origin or changed report bytes,
  when validating, then EVIDENCE_UNAUTHORIZED/EVIDENCE_REVIEWER/EVIDENCE_DIGEST.
- AS-8: Given changed H/B/C/tree/scope/repository/PR, when validating, then
  EVIDENCE_STALE. Missing governance acknowledgment yields
  GOVERNANCE_REVIEW_REQUIRED. P1/P2 or unadjudicated P3 yield EVIDENCE_FINDINGS.
- AS-9: Given a later COMMENTED note after a request for changes, when evaluating,
  then REVIEW_CHANGES_REQUESTED persists. COMMENT is rejected as a stored state;
  the COMMENTED API contract is verified with an actual response capture.
- AS-10: Given base/review/current attempt changes during reads, when returning,
  then CAPTURE_CHANGED. Invalid input or an expired deadline cannot reach a
  credential-bearing network request.
- AS-11: Given the initial deployment PR and no verifier on its base, when the
  workflow starts, then BOOTSTRAP_REQUIRED. Never fall back to candidate code.

## Cross-scenarios

AS-5/10 cross independent API reads and run lifecycle. AS-7/8 cross the GitHub
custodian identity, Git objects and report content. AS-11 crosses protected-base
execution and initial publication. Unit tests and a separate Python CLI process
prove only their exercised boundaries. Real GitHub scheduling, invalidation and
merge behavior require an authorized rehearsal; they are not proven locally.

## Success criteria

- Attack suite passes and every test has a detected executed mutation.
- Existing `make quality` passes in this isolated worktree, with -race and current
  coverage floors. Do not claim this certifies the dirty principal checkout.
- actionlint passes touched workflows; existing checker/hooks remain byte-identical.
- No runtime Go dependency change or product-code change; no new package coverage
  floor is substituted for the existing core thresholds.
- Hostile copilot review has zero unresolved P1/P2 within the delivered scope.
- Operational docs distinguish local implementation, custody and remote activation.

## Decisions folded in

The broader App/hosted-evaluator proposal was not selected. Reuse existing worker
runs and a trusted-base reducer. Full frontend/site workers run on every PR for
now; the reducer ignores irrelevant failures. This spends more CI minutes but
avoids false absence from incomplete path filters. No claim of immutable external
attestation, independent human approval or atomic merge authorization is made.

Keep the marker unchanged. GitHub rebase changes SHAs and merge commits fail the
local checker. The server's squash/linear-history discrepancy remains an explicit
activation dependency. Do not change merge settings until a candidate/merge/tag
rehearsal proves the chosen policy; never repair a marker automatically.

## Review checklist

- [x] Source and remote controls captured.
- [x] Pre-test review approved by originating copilot, with conditions.
- [x] Exact failure taxonomy and mutation cases defined.
- [x] COMMENTED correction authorized by copilot and verified against API output.
- [x] Final code review and full-quality evidence recorded in the verification report.
- [ ] Server bootstrap/invalidation/merge rehearsal complete (activation only).

## [NEEDS CLARIFICATION]

Local implementation has no remaining design blocker. Before producing a real
green evidence result, the director must name the permitted external reviewer
and report origin in `scripts/integration-policy.json`. No fictitious identity
is supplied. Before activation, publication follows the approved two-step bootstrap,
and server tests must resolve post-success review revocation/base change behavior
and merge/marker compatibility. Keep the eleven existing required checks intact.
