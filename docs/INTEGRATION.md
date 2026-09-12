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
whose first line names C. Do not fabricate a marker to obtain a descriptor. With
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
4. Resolve the merge policy before changing it: the captured server permits
   squash and requires linear history, while GitHub rebase rewrites SHAs. The
   local checker rejects a merge commit. A possible merge-only approach preserves
   H and tags H after integration only if ancestry and tree equality to the merge
   are proven; this is a proposal requiring a packaging rehearsal, not the active
   procedure. Do not tag an unreviewed merge or auto-write a repair marker.
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
