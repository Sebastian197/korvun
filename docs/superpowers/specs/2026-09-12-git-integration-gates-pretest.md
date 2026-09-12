# Pre-test adversarial review: first implementation

Status: approved for RED by the originating copilot on 2026-09-12, before tests.
The approval requires trusted-base code, governance acknowledgment, complete
pagination, final rereads of head/base/reviews/attempts, accessible report bytes,
conservative applicability and no production activation without server evidence.
The copilot later authorized correcting the COMMENT event / COMMENTED stored-state
contract after a real API capture. This is a reduced implementation cut of the
design spec. The dedicated App and hosted evaluator are deferred, not prerequisites
for writing the local reducer. The director has authorized related CI/hook edits;
commit/push of the first bootstrap are authorized; merge and activation remain reserved.

## Literal implemented guarantees

1. The reducer returns success only when every applicable existing CI job and
   mandatory step has succeeded for the selected PR/head and current attempt.
2. Evidence validation requires an authenticated authorized custodian's submitted
   GitHub review COMMENT, whose body binds code parent, head, base, scope and
   report bytes. A marker or a PR body alone cannot satisfy it.
3. Changed inputs fail validation. This is an evaluation-time guarantee. The
   first implementation does not claim atomic revocation at GitHub's merge action.

The custodian is not silently treated as the code reviewer. A single maintainer
may record an external review, with reviewer identity and origin explicitly
declared. This authenticates custody, not the external engine or actual quality
of the review. No mandatory second GitHub account is invented.

## Source path and seams

PR event -> read-only integration workflow -> trusted-base Python stdlib script
-> GitHub current PR, complete files, worker runs/jobs and submitted reviews ->
pure reducer -> named terminal result. Existing worker commands and marker
checker remain intact. PR filtering is removed from frontend/site so dependency
changes outside their former paths cannot silently prevent applicable execution.
Unrelated worker failures do not affect the reducer. Worker source stays subject
to the existing human review and protected CI editing process.

The workflow uses the protected base implementation. On the initial bootstrap PR
the script does not yet exist on that base: report `BOOTSTRAP_REQUIRED` and install
in two reviewed publication steps before making the new check required. No
candidate-code fallback, privileged artifact execution or fake success.

## Attack matrix and planned mutations

| Attack | Exact failure | Mutation that must turn test red |
| --- | --- | --- |
| Green Go plus failed website | CHECK_FAILED | Ignore applicable website result |
| Missing or duplicate expected job | CHECK_MISSING / CHECK_DUPLICATE | Accept empty/duplicate job list |
| Neutral/skipped/cancelled mandatory job or step | CHECK_NOT_SUCCESS / STEP_NOT_SUCCESS | Treat skipped/neutral as success |
| Old head, old base, previous run attempt | CANDIDATE_STALE / EVIDENCE_STALE / RUN_STALE | Remove corresponding binding check |
| Same job names, wrong workflow/event/repository | RUN_SOURCE_MISMATCH | Select run by name alone |
| Rename from website to docs | Website remains applicable | Ignore previous_filename |
| Pagination fails on later page | INPUT_UNAVAILABLE | Return the collected prefix |
| Untrusted recorder or PR-body forgery | EVIDENCE_UNAUTHORIZED / EVIDENCE_MISSING | Accept caller-provided identity |
| Report bytes or SHA changed | EVIDENCE_DIGEST | Skip report digest comparison |
| P1/P2 or unadjudicated P3 | EVIDENCE_FINDINGS | Ignore finding counts/dispositions |
| Approval at H with changed B/scope | EVIDENCE_STALE | Ignore B/scope binding |
| Synthetic merge commit mistaken for H | CANDIDATE_STALE | Compare the wrong SHA field |
| API 403/429/timeout/malformed response | INPUT_UNAVAILABLE / INPUT_INVALID | Swallow the error |
| New PR head while polling | CANDIDATE_STALE | Omit final current-PR read |

## Persistence and concurrency

The reducer has no database and does not write remote reviews or checks through
the API. GitHub stores the submitted review and workflow result. Each run checks
the current head/base both before collection and before returning success. This
does not make API reads an atomic snapshot. A base update or review revocation
after return remains a rollout concern; strict protection and authenticated
re-evaluation are required before claiming enforcement at integration time.

## Evidence levels

Pure unit tests exercise reducer taxonomy and injected HTTP response failures.
A separate Python process exercises CLI input rejection and exit status.
No live mock HTTP server was used; no end-to-end API claim is made.
Executed mutations must make the corresponding tests fail. Existing
`make hook-probe` and `make quality` check regressions. GitHub publication and
merge-time behavior remain unverified until separately authorized publication.

## Review checklist

- [x] Literal claims scoped to evaluation time.
- [x] No credential or independent reviewer invented.
- [x] Failure taxonomy and mutation requirements specified.
- [x] Existing marker source and CI dependencies read.
- [x] Copilot approval before RED, with the conditions recorded here.
