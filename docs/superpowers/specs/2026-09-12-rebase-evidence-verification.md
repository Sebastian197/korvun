# Rebase evidence verification

Scope: extend PR #32 with versioned markers and explicit source/integrated
history verification. No protection change, force push, release or new required
check is included. The director has authorized integration after validation.

The coordinating reviewer approved the concrete pre-test contract before RED.
The initial 15-test run produced 26 failing assertions against the legacy
checker. The implementation passed the initial 20-test suite; three additional
tests verify the history limit, raw commit limit and output-reader failure.
Those targeted tests passed, followed by the complete final 23-test suite.
The hook and integration targets both passed after the adjudicated expectation
change. The [compact record](evidence/git-integration-2026-09-12/rebase-verification.json)
binds local evidence to source hashes. Remote CI, final review and integrated
identities are recorded in the PR as they are observed.

All 18 destructive rebase mutants were detected against the recorded helper
hash. The unchanged integration reducer still detects all 27 of its mutants.
The mutation record distinguishes removed error walls from variants that would
otherwise accept changed content or provenance; detection is not a completeness
claim. Original and rewritten fixtures use real Git objects. The gh boundary
is controlled test data, not a real GitHub review or merge.

The complete local quality invocation passed Go package discovery, formatting,
vet/lint, race tests, coverage and fuzz smoke, then stopped at the legacy
BIGBLOB fixture, which expected a 200 KiB marker to pass. The new 64 KiB cap
correctly blocked it. The reviewer explicitly approved changing only that
expectation to require the exact size rejection. A separate run of the hook
and integration targets validates the affected checks without repeating
unchanged Go work. Fresh govulncheck v1.7.0 reported no vulnerabilities.

Windows runs the Python helper directly, including the gh.cmd boundary. The
two legacy Bash/pre-push tests are explicitly skipped there, preserving their
existing Linux/macOS scope. Linux/macOS run the Bash entry point and a real
pre-push hook. After actual GitHub integration, the retained original source
history and API PR record must pass the same comparison on the integrated SHA.

The precheck/merge/postcheck sequence has no atomic expected-base operation.
Changed base or missing provenance blocks validation and is never repaired by
rewriting evidence. Source branches must remain available; API unavailability
or missing source objects blocks validation of rewritten histories.
