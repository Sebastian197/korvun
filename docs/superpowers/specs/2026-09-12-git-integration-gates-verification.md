# Integration verifier bootstrap: verification

This first delivery contains the verifier, attack tests, mutation runner, empty
reviewer policy, existing worker changes and operating instructions. It excludes
integration.yml. No new required check, protection change, merge or release is
part of this delivery. The source base is c6ccd242b8539890593dfeced652a6d322fe2531.

## Observed local evidence

- All 18 attack tests pass, including a separate Python CLI process.
- All 27 executed mutations are detected; none survives. The compact machine
  record binds the results to source hashes and names each attack.
- make quality passes in the isolated worktree: the 41-package guard, format,
  vet, lint, race tests, coverage, fuzz smoke, existing hook probes and integration
  attacks. The unchanged core keeps its existing coverage floors.
- actionlint passes the three modified worker workflows; git diff --check passes.
- Existing marker checker and hooks are unchanged from the source base.
- Replaying actual API jobs from quality run 34235161191 accepts its ten jobs and
  returned steps. This is not a complete PR/review/merge validation.

See [compact execution record](evidence/git-integration-2026-09-12/bootstrap-verification.json)
and [operating instructions](../../INTEGRATION.md). Full local logs and captures
are retained outside this publication; synthetic review fixtures were not sent
as real evidence to GitHub.

## Adversarial review

The pre-test contract was approved with trust, pagination, capture-consistency
and activation constraints. Review found and reproduced three defects:

1. The API stores COMMENTED; COMMENT is only the creation event. An actual GET
   response established the contract and the corrected fixture failed before
   the implementation cure.
2. An unauthorized newer record or request for changes could eclipse valid
   evidence. The exact attack failed before the cure. Custodian filtering now
   precedes latest selection; only configured blocking identities can veto.
3. A newer run from another PR sharing the head could displace this PR's result.
   The exact attack failed before the cure. Origin/PR filtering now precedes
   latest selection; duplicate run identities are refused.

The independent reviewer re-read both P2 cures, ran all 18 tests and repeated
all three P2 reproductions, accepting those cures for the in-process scope.
No open P1/P2 remained from that review. The final publication commit receives
its own scoped review before the existing marker is recorded.

A mutation also exposed a rename fixture whose current docs paths already
activated website. Using internal-spec paths restored the oracle: only the old
website path activates the lane, and removing that guard makes the test fail.

## Remaining activation work

The first delivery prepares the trusted base. It does not emit korvun/integration
or claim atomic merge authorization. The director must select a real external
reviewer/origin before the empty policy can accept a report. The second delivery
must test lifecycle/review events, changed base, reruns/cancellation and stale
success in GitHub before making any check required. Merge/marker/tag compatibility
must also be rehearsed before changing squash, rebase or linear-history policy.
Existing required contexts and the director's merge decision remain in force.
