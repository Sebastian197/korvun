# Integration checks and review custody

This first bootstrap delivery installs the verifier, tests, policy and worker
changes. It deliberately excludes `.github/workflows/integration.yml`. The
`korvun/integration` check is not emitted or required by this delivery. A second
reviewed delivery can wire the verifier after it exists on the protected base.
The verifier evaluates CI evidence and authenticated custody of an external
report; it does not itself merge code or modify GitHub protection.

## Local checks

Use Python 3.9 or newer, Git and Bash. No Python packages are required.

```sh
make integration-probe
python3 scripts/integration_gate_mutations.py > /tmp/integration-mutations.json
make quality
```

The mutation command uses temporary copies. Its JSON records the verifier hash,
named attacks, exit codes and captured failures. The Go quality gate retains its
package discovery, coverage floors, fuzz tests and hook probes.

## Record a real external review

The code producer, declared external reviewer, authenticated GitHub custodian
and final integrator are separate roles. The custodian attests that the supplied
report is the actual external review. The script authenticates that custodian
through GitHub, not the external engine or its intellectual work.

`scripts/integration-policy.json` lists custodian numeric GitHub IDs and maps
permitted reviewer names to permitted origin identifiers. The initial custodian
is the repository's verified owner, 38566816. `reviewers` is deliberately empty.
The director must name a real reviewer and origin; an empty configuration blocks
with `EVIDENCE_REVIEWER`. The second-stage workflow must load policy from the protected base. The candidate
cannot configure its own reviewer through the report.

Only configured custodians can supersede a custody declaration. Requests for
changes count only from numeric IDs in `blocking_reviewer_ids`; when omitted,
that list defaults to `custodian_ids`. Unknown commenters cannot veto the reducer.
GitHub may separately impose its own review protections.

After actual review of code commit C, retain the existing pure marker commit H
whose first line names C. Use the versioned format described in the rebase
procedure for a train that will be integrated through GitHub rebase. Do not
fabricate a marker to obtain a descriptor. With
H available locally, an open PR and its current base B, inspect the descriptor:

```sh
python3 scripts/integration_gate.py --repo Sebastian197/korvun \
  --pr "$PR_NUMBER" --head "$HEAD_SHA" --base "$BASE_SHA" --describe
```

Supply a read-only `GH_TOKEN` through the existing environment/credential manager;
do not paste it into a file or command. The descriptor contains candidate fields,
applicable lanes and whether governance review is needed. It does not approve
anything. H must include B and pass the existing marker checker.

The authorized custodian submits a PR review with event `COMMENT` and
`commit_id=H`. GitHub stores its state as `COMMENTED`. Its body starts with the
exact line `korvun-review-v1`, followed by one JSON object containing:

| Field | Meaning |
| --- | --- |
| `schema` | integer 1 |
| `candidate` | exact descriptor object: repository_id, pr, head, base, code, tree, scope |
| `reviewer`, `origin` | real configured reviewer and report origin |
| `report` | actual report text, inline and accessible, at most 4 MiB |
| `report_sha256` | SHA256 of the UTF-8 report string, before JSON escaping |
| `p1`, `p2` | integer zero after all in-scope findings are resolved |
| `p3` | array of objects with nonempty `id` and `disposition` for each P3 |
| `governance_scope` | exact candidate scope digest when workflows/scripts/hooks/policy change |

No example here constitutes review evidence. Do not generate a green report from
these field names. The latest authorized custody record supersedes previous ones;
pending/dismissed records block. A later ordinary comment does not resolve a
request for changes. A digest authenticates bytes only, not the report's claims.

Run the command without `--describe` to evaluate. It waits up to 3600 seconds for
missing/pending workers, reads complete paginated collections and rechecks current
head, base, reviews and run attempts before returning. Failures have named codes
such as `CHECK_MISSING`, `RUN_SOURCE_MISMATCH`, `EVIDENCE_STALE` and
`CAPTURE_CHANGED`. It makes no remote writes.

## CI scope

The planned second-stage workflow must execute verifier and policy from B with
read-only permissions, fetch H's objects, and avoid executing H. That workflow is
not included in this delivery. Existing Go/CodeQL/
frontend/site workers run candidate code under their existing permissions. The
reducer requires applicable jobs and returned execution steps to succeed, with
the existing Windows Unix-hook probe skip explicitly allowed. Worker logs are
not evidence that every dangerous test branch was exercised.

Go/CodeQL apply to every PR. Shared Go/frontend code activates frontend; public
docs/assets/site activate website; shared tooling/module manifests/unknown paths
activate all lanes. Internal specs, ADRs and stage notes alone use Go/CodeQL.
Frontend/site workers now run on all master PRs, so required evidence can exist
for changes outside their old path filters. Irrelevant worker failures do not
block the reducer. This increases CI use; push and tag triggers are unchanged.

## Publication and activation

1. Obtain explicit commit/push authorization. Publish verifier, tests, policy and
   worker changes first, keeping `integration.yml` for the second publication
   step. The existing rehearsal/review/PR rules apply to both steps.
2. Merge the reviewed verifier onto master through the director's normal PR flow.
   Then publish the workflow. If published together, it reports
   `BOOTSTRAP_REQUIRED` on the first PR because the verifier is absent from B.
   Never bypass this by running the candidate verifier as trusted code.
3. Configure the real reviewer/origin through review. Exercise real positive and
   negative cases in GitHub: paths, failed/skipped/missing jobs, forks, new head,
   base advancement, edited/dismissed review, rerun/cancel and API outage.
4. Integrate through the rebase procedure with versioned evidence. Keep the
   existing linear-history and PR protections. Squash and merge commits do not
   satisfy this procedure. Never auto-write a repair marker or claim review of
   a rewritten SHA from a review of the original SHA.
5. Only after demonstrated server behavior may the director make the new check
   required and align strict-base/review/merge settings. Preserve all eleven
   existing required checks, administrator enforcement and no-bypass protection.
   Read classic protection and rulesets back after any authorized mutation.

The planned workflow uses review events for re-evaluation; base advancement by another PR does not
necessarily create a new run for this PR. Reads and success publication are not
atomic with GitHub's merge action. Post-success revocation and simultaneous PRs
sharing H require server validation and further design before enforcement. The
shared GitHub Actions App identity does not prevent a malicious administrator or
candidate workflow from publishing a same-name result. Do not present this check
as a tamper-proof security boundary or independent human approval.

If the control becomes unhealthy, stop integration and report the exact failing
context. Restore previously validated code through the reviewed flow; never
manufacture success, accept unknown state, disable protections or bypass hooks.
Until activation prerequisites are met, existing protections continue to govern.

## Rebase procedure and retained evidence

The director authorizes integration; the integrator may execute that authorized
decision after review and checks. The marker records the actual review of C.
Its second line is exactly `KORVUN-REBASE-EVIDENCE v1`. The remaining bytes are
one JSON object with exactly `schema` (integer 1), `repository`
(`Sebastian197/korvun`), `pr` (the actual positive PR number), `base` (the exact
reviewed base B as a full lowercase SHA), and `review` (the real review text).
The first line remains `VETO LEVANTADO <C>`. All markers, including legacy ones,
are capped at 64 KiB before the Bash checker loads their content.

Before publication, H must change only the marker and have C as its direct
parent. The checker validates the complete original B..H sequence, allowing at
most 128 commits. Obtain the PR number from the existing PR; for a new train,
open the initial reviewed PR under the existing workflow before adding its
versioned marker. Never guess a future number or fabricate a review to obtain it.

Run both destructive mutation suites, the complete quality gate and fresh
govulncheck. Publish H to ensayo and the PR branch with hooks enabled and no
force. Wait for the full rehearsal and the PR's own required checks. Reread H,
B, review state and protections. Stop if any input changed. Integrate with
GitHub rebase using the expected-head constraint; do not delete the source
branch. The merge API has no expected-base constraint: prechecks are not an
atomic exclusion of another integrator. Detecting a base change means stopping
and obtaining fresh review/CI, not accepting a different base silently.

After integration, fetch master and run the reviewed checker on GitHub's
reported merge result J. Its recorded C is unchanged. For rewritten histories,
the checker reads the merged PR through `gh api --hostname github.com`, requires
the fixed repository identity in both head and base, base branch master, source
head H and merge result J. It requires the original source objects locally and
checks the marker bytes, pure marker commit and each commit in B..H versus B..J.
Trees, author, message and encoding must match in order. Only committer metadata,
Git signatures and remapped parent links may differ. This proves explicit
equivalence and GitHub provenance; it does not authenticate the reviewer's work
or assert review of a rewritten commit identity.

Python 3.9+ is needed for versioned evidence. After rebase, gh, API access and
the original source history are needed. Keep the PR source branch as evidence.
If a clone lacks source objects, fetch the preserved branch explicitly, for
example `git fetch origin codex/git-integration-controls` for PR #32. In a
shallow clone, obtain complete history with `git fetch --unshallow origin`
first. Verify that origin is the intended repository. The checker itself never
fetches objects and rejects missing provenance, missing objects, malformed API
data and deadline/size violations. API downtime blocks rewritten histories.

A failed postmerge check is an integration incident, never an accepted result
or a reason to weaken the anchor or repair the marker automatically. Record the
original H, integrated J, exact B and validation output in the PR evidence. This
procedure does not authorize tags/releases or activate `korvun/integration`.
