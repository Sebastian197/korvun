# Review provenance across GitHub rebase integration

Status: pre-test review approved by the coordinating reviewer on 2026-09-12,
before tests or implementation. This extends PR #32; no protection changes,
force push, tag, release or activation of `korvun/integration` is included.

## Contract

Keep legacy markers and their current exact-parent behavior, with a new 64 KiB
cap for every marker enforced before the shell reads its content. A versioned marker
still begins `VETO LEVANTADO <reviewed C>`, followed by the exact line
`KORVUN-REBASE-EVIDENCE v1`, then one strict JSON object with schema=1,
repository=`Sebastian197/korvun`, integer `pr`, full lowercase `base` SHA B and
nonempty `review` text supplied by the actual reviewer. This declares review of
C, never a fabricated review of GitHub's rewritten C'. Duplicate/unknown keys,
unsupported versions, malformed types and oversized data fail closed.

The marker commit must remain a regular file and change exactly the marker.
Both source and integrated histories must be linear sequences anchored at the
same exact B, with at most 128 commits including the final marker. Read raw
commit objects, validate their object hashes, and reject merges, duplicate
headers, cycles, missing objects and unknown headers. Compare every commit's
complete tree, author, message and encoding, in order. Parent links may only be
remapped to the corresponding predecessor. Committer metadata and `gpgsig`
may differ; no approval of a cryptographic signature is inferred. Every other
header is rejected. Compare the marker commit too, including its full tree,
message/author and marker bytes. Do not compare only the final tree.

## Two validation modes

Before integration, H's direct parent equals C. Validate the entire B..H sequence
and marker structure locally. This remains the existing discipline aid: a marker
declares a real review but does not authenticate the reviewer's work. No helper
may generate a verdict. The review must precede committing the marker.

After rebase, J's direct parent differs from recorded C. Local tree equality is
insufficient. Read GitHub's PR record through `gh api --hostname github.com` at
the fixed repository path. Require the same repository identity (1268356234),
same-repository source, PR number, base branch master, merged state, source head
H and merge result exactly J. Require H and its original history locally, and
validate H in original mode: its parent must be the declared C and its marker
bytes must exactly equal J's. Then compare B..H with B..J as specified. A fake
self-contained commit sequence cannot substitute for GitHub's source H.

Unavailable API, missing gh, missing source objects or an unmerged PR block with
specific reasons. Preserve the source branch after integration. A full clone
normally includes it; a shallow/single-branch clone must fetch the recorded
source branch before checking. The checker never fetches or writes remote data.
GitHub is trusted for PR provenance; the authenticated API does not prove an
external reviewer's identity, intellectual work or administrator honesty.

## Integration procedure

Obtain a real verdict for the updated C and commit a pure versioned marker H.
Rehearse H on ensayo and pass all PR checks, then reread PR head H, base B,
readiness and protections. Stop on any changed input. Use GitHub rebase merge
with an expected-head constraint; retain the branch. Read the merged PR record,
fetch the resulting master objects and validate J with the reviewed checker.
Record H, J, B, GitHub provenance, per-commit equivalence and test evidence.

GitHub's merge API offers an expected head, not an expected base. The precheck
and postcheck are not atomic. Do not claim an exclusion lock over other
integrators. Coordinate known work and stop on a detected base change; an
unexpected concurrent integration is a failed postvalidation, never accepted
by relaxing the anchor. This limitation remains explicit before server activation.

## Implementation and acceptance plan

1. Add Python stdlib helper `scripts/rebase_evidence.py`, reached by the existing
   Bash checker only for versioned markers. Legacy fixture behavior stays intact.
   Python 3.9+ is required for v1; gh and network only for rewritten histories.
   Bound marker/API responses to 64 KiB, raw commits to 64 KiB, history to 128,
   and each subprocess to 20 seconds, with a total 25-second validation deadline.
   Consume stdout in bounded chunks while the process runs; never capture an
   unbounded stream and check its size afterwards.
2. Add `scripts/rebase_evidence_test.py`: real Git fixture histories, explicit
   changed committers and rewritten parent links, source branch and a fake gh
   boundary. Exercise the actual Bash entry point and real pre-push hook on
   Linux/macOS. Windows exercises the Python helper directly; the two legacy
   Bash/pre-push tests are explicitly skipped, preserving the existing platform
   perimeter. Resolve gh.cmd through PATH before starting subprocess on Windows.
3. Add destructive mutations for each equivalence/provenance wall. Run existing
   hook probes, 18 reducer tests, both mutation suites and complete make quality.
   Add the new test command to Makefile and the three-OS quality workflow.
4. Review exact committed code, record a real v1 marker, fresh govulncheck,
   fast-forward ensayo and PR branch, wait for CI, then integrate as authorized.

| Attack or acceptance | Required result |
| --- | --- |
| Legacy valid / stale marker | Preserve existing success / exact failure |
| Original v1 marker and same-B rebase with committer-only rewrite | Success |
| Rebase on advanced base, including empty commit | REBASE_BASE |
| Changed intermediate tree then restored final tree | REBASE_EQUIVALENCE |
| Changed author, message, encoding, mode, rename, symlink or marker body | REBASE_EQUIVALENCE |
| Source head unrelated to PR or merge result not J | REBASE_PROVENANCE |
| Open/unmerged PR, wrong repository, number or base branch | REBASE_PROVENANCE |
| Missing original source object, omitted/extra/reordered commit, merge | REBASE_HISTORY |
| Marker carries another path | Existing pure-marker failure |
| Duplicate JSON keys, unknown schema/header, invalid SHA or bounds | REBASE_FORMAT |
| gh absent, timeout, API error, malformed response | REBASE_API |
| Original mode with poisoned proof or invalid base | Named rejection before push |
| Postmerge marker source and result histories identical in all allowed fields | Success through Bash checker and pre-push |

Tests distinguish omitted workflow steps from individual test skips. No test
claims real GitHub rebase execution until the authorized merge is observed.

The reviewer approved changing only the legacy BIGBLOB expectation from allowed
to blocked after its RED reproduction, because the new explicit size contract
rejects that 200 KiB marker before loading it. All other legacy expectations
remain unchanged.
