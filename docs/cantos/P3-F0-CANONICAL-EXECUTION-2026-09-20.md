# P3-F0: canonical execution

Date: 2026-09-20

Status: implemented, verified, reviewed and committed in this worktree. The
delta review that accompanied the implementation returned APTO. The internal
adversary then ran ONE pass over the COMPLETE diff of the commit and held the
veto with two P2 findings; both are cured in this same tree and declared in
"The pass over the complete diff" below, each with its executed red. Director
acceptance is pending.

Base: `adb44231a6635f8d1abd48196ef892e74809b935`

Branch: `p3/fase-0-canonical-execution`

Worktree: `/Users/sebastianmorenosaavedra/Desktop/korvun-p3f0.nosync`

The source worktree at
`/Users/sebastianmorenosaavedra/Desktop/korvun.nosync` was not written.

## Result

Every attempted tool call now starts with one canonical action, operation and
parameters digest. The action receives a finite decision before the registry
can invoke a tool. Immediate execution and approved resumption converge on the
private `invokeAndClose` routine in `internal/action/executor`. Only its private
`dispatch` method calls `Tool.Execute` or `ScopedTool.ExecuteScoped`.

`brain.(*AgentBrain).runTool` is now a presentation adapter over `Prepare` and
`Submit`. `app.ExecuteApprovedAction` retains its public signature and
`ApprovedExecution` result, and delegates the claim, invocation and close to
`ResumeApproved`.

The compatibility surface defined by the Phase 0 paper is green before and
after the refactor. The protocol-sensitive tests retain their exact bytes. The
action schema remains 12. No dependency file changed.

## Adaptation to the current tree

The 2026-09-19 paper described the tree before the v0.15.1 hardening landed.
The base used here already contained these stricter contracts, so the
implementation preserves them:

- `ClaimApprovalParamsUnderDigest` judges the complete snapshot inside the
  claim and returns the operation and parameter bytes that may run.
- `tool.CloseStateAfterRun` remains the outcome classifier for immediate and
  approved execution.
- terminal close still uses `context.WithoutCancel`.
- the existing claim sentinels retain their identities and precedence.
- crash recovery still closes stranded `AUTHORIZED` and `APPROVED` actions as
  `OUTCOME_UNKNOWN`.
- approved close time remains separate from executor latency time.
- immediate execution and denial events retain their established ordering
  relative to the durable record.

These facts narrowed the implementation. They did not contradict the Phase 0
goal.

## Guarantee, mould, mutation and evidence

Every mutation named below was applied to the green source, run, observed red,
removed, and followed by a restored green run. The captures are under the
session-local directory
`/tmp/korvun-p3f0-evidence-20260920/`; they are not part of the staged change.

| Guarantee | Mould | Executed mutations | Evidence level |
| --- | --- | --- | --- |
| AS-EXEC-01. The only physical tool invocation sites are the exact registry sites in `executor.dispatch`; `runTool` and `ExecuteApprovedAction` each enter the coordinator once. | `TestExecutor_ASTRejectsEveryExternalDispatch` | direct dispatch; dispatch method value; equivalent local interface; type-aliased interface; coordinator method value; equivalent coordinator interface; type-aliased coordinator interface; hidden helper; extra `Prepare`; legacy approved-store read | AST with Go type resolution over production packages for darwin, linux and windows, each with default and desktop tags |
| AS-EXEC-02. A request is bound to its action, operation, digest, owner, channel and one shared consumption cell. Invalid input has no downstream effect. | `TestExecutor_RejectsUnboundRequest`; `TestSubmit_RejectedBindingDoesNotConsumeTheRequest` | remove digest binding; omit the use-cell check; consume before complete validation; wrap the replay sentinel | table-driven in-process test under `-race`, with tool, recorder, approval and closer counters |
| AS-EXEC-03. Every decision branch uses one action birth. Exactly one concurrent submission wins. Historical classifier reads keep their order and meaning. | `TestSubmit_AllBranchesHaveOneCanonicalAction`; the three stateful-classifier moulds | discarded birth; rebuild at parking; durable extra birth; replay durable write; remove the single-use claim; reuse the first class after a later read; let a second miss deny; skip the durable read after a first miss; skip the identity-plus-recorder read; skip the failed-approval terminal read | table-driven text and native lanes, stateless and durable rows, synchronized goroutines and `-race`; the removed claim also produced a data race |
| AS-EXEC-04. Immediate invocation receives the original argument bytes. Canonical bytes are used only for the digest. | `TestCompatibility_ImmediateArgumentsRemainByteExact` | pass canonical parameters to dispatch | table of spaced JSON, reordered JSON, empty input and non-JSON with byte equality |
| AS-EXEC-05. Decision precedence, observations, audit ordering and fixed governance remain compatible. | both `TestCompatibility_DecisionPrecedenceAndObservations` moulds and the brain ordering moulds | generic approval refusal; replace fixed governance; audit denial after record; log identity fallback after record; duplicate denial-record log; audit execution after close | table-driven executor and brain tests with exact branch, rule, observation, log, audit and zero-call assertions |
| AS-EXEC-06. Approved execution uses the snapshot read before the claim and the operation and bytes returned by the claim. | `TestResume_UsesClaimedOperationAndSnapshot`; `TestResumeApproved_UsesTheSnapshotReadBeforeTheClaim` | pass a nil snapshot; recompute it inside the adapter | fake contract test plus a real SQLite file, two connections and a claim-time row movement |
| AS-EXEC-07. Close classification, cancellation isolation and recovery remain compatible. | `TestExecution_CloseAndRecoveryRemainCompatible` | close with the cancelled caller context; classify uncertainty as `FAILED`; recover `AUTHORIZED` as `FAILED` | real SQLite file; delivered error, deadline, cancellation and crash recovery under `-race` |
| AS-EXEC-08. Nil-recorder mode still uses canonical admission without durable I/O or a close-clock read. | `TestCompatibility_NoRecorderStillUsesCanonicalExecutor` | require a recorder; read the close clock without a closer | text and native table rows, isolated profile root and invocation, id, filesystem and clock counters under `-race` |
| AS-EXEC-09 (added by the pass over the complete diff). The audit bus keeps receiving the LIVE inbound envelope on every branch, as it did before this phase. | `TestRunTool_AuditCarriesTheLiveInboundEnvelope` | restore the snapshot in `Prepare` | in-process, executed and denied rows, pointer identity against the envelope handed to `runTool` |

The exact mutation capture names are:

- AS-EXEC-01:
  `mutation-as-exec-01-direct.txt`,
  `mutation-as-exec-01-method-value.txt`,
  `mutation-as-exec-01-equivalent-interface.txt`,
  `mutation-as-exec-01-runtool-double-prepare.txt`,
  `mutation-as-exec-01-approved-legacy-get.txt`,
  `mutation-as-exec-01-coordinator-method-value.txt`,
  `mutation-as-exec-01-coordinator-equivalent-interface.txt`,
  `mutation-as-exec-01-hidden-helper.txt`,
  `mutation-as-exec-01-type-alias-dispatch.txt` and
  `mutation-as-exec-01-type-alias-coordinator.txt`.
- AS-EXEC-02:
  `mutation-as-exec-02-digest-binding.txt`,
  `mutation-as-exec-02-missing-use-cell.txt`,
  `mutation-as-exec-02-early-consume.txt` and
  `mutation-as-exec-02-wrapped-replay-sentinel.txt`.
- AS-EXEC-03:
  `mutation-as-exec-03-discarded-birth.txt`,
  `mutation-as-exec-03-rebuild-at-park.txt`,
  `mutation-as-exec-03-durable-extra-birth.txt`,
  `mutation-as-exec-03-replay-writes-durable-record.txt`,
  `mutation-as-exec-03-remove-single-use-claim.txt`,
  `mutation-effect-stateful-widening.txt`,
  `mutation-effect-second-miss-denies.txt`,
  `mutation-effect-first-miss-skips-durable-read.txt`,
  `mutation-effect-identity-recorder-skips-third-read.txt` and
  `mutation-effect-failed-approval-skips-fourth-read.txt`.
- AS-EXEC-04: `mutation-as-exec-04-canonical-args.txt`.
- AS-EXEC-05:
  `mutation-as-exec-05-generic-refusal.txt`,
  `mutation-fixed-governance-replaced.txt`,
  `mutation-denial-audit-after-record.txt`,
  `mutation-identity-fallback-after-record.txt`,
  `mutation-denial-duplicate-record-log.txt` and
  `mutation-execution-audit-after-close.txt`.
- AS-EXEC-06:
  `mutation-as-exec-06-nil-snapshot.txt` and
  `mutation-as-exec-06-recomputed-snapshot.txt`.
- AS-EXEC-07:
  `mutation-as-exec-07-cancelled-close.txt`,
  `mutation-as-exec-07-failed-classification.txt` and
  `mutation-as-exec-07-recovery-failed.txt`.
- AS-EXEC-08:
  `mutation-as-exec-08-recorder-required.txt` and
  `mutation-as-exec-08-stateless-close-clock.txt`.

Three supplementary mutations cover compatibility facts adjacent to the eight
moulds:

- `mutation-approved-empty-id-binding.txt` keeps the empty approval-id store
  sentinel.
- `mutation-approved-close-latency-clock.txt` keeps the approved close clock
  separate from latency measurement.
- `mutation-live-text-skip-submit.txt` proves the real text protocol reaches
  `Submit` and one tool execution.

Representative killed outputs, copied from the captures (line numbers re-derived against this commit; see P3-2 below), include:

```text
direct_tool_dispatch .../internal/tool/memorynote.go:73:9: Tool.Execute and ExecuteScoped are confined to executor.dispatch
canonical_execution_test.go:258: Submit error = <nil>, want ErrExecutionBindingMismatch
canonical_execution_test.go:530: wins=2 replays=0 ids=1 dispatches=2 records=0 approvals=0 finishes=0
canonical_execution_test.go:551: tool args = "{\"a\":1,\"b\":2}", want exact "  {\"b\":2,\"a\":1}  "
canonical_execution_test.go:587: branch/rule = "denied"/"denied", want "denied"/"approval_unavailable"
canonical_execution_test.go:835: claim snapshot = <nil>
canonical_execution_test.go:868: close inherited caller cancellation: context canceled
canonical_execution_test.go:936: stateless close clock calls = 1, want 0
```

## RED captures

The first structural mould ran directly against the pre-cure tree:

```text
$ go test -race ./internal/action/executor -run '^TestExecutor_ASTRejectsEveryExternalDispatch$' -count=1 -v
canonical execution structure violated:
canonical_coordinator_bypass .../internal/app/approvals.go:158:1: ExecuteApprovedAction calls ResumeApproved 0 times, want 1
canonical_coordinator_bypass .../internal/brain/agent.go:678:1: runTool calls Prepare 0 times, want 1
canonical_coordinator_bypass .../internal/brain/agent.go:678:1: runTool calls Submit 0 times, want 1
FAIL
```

AS-EXEC-02 through AS-EXEC-08 used the compile-only RED scaffold through a Go
overlay. These are the exact commands recorded in their captures:

```text
$ go test -race -overlay=/tmp/korvun-p3f0-evidence-20260920/red-overlay.json ./internal/action/executor -run '^TestExecutor_RejectsUnboundRequest$' -count=1 -v
FAIL
EXIT: 1

$ go test -race -overlay=/tmp/korvun-p3f0-evidence-20260920/red-overlay.json ./internal/action/executor -run '^TestSubmit_AllBranchesHaveOneCanonicalAction$' -count=1 -v
FAIL
EXIT: 1

$ go test -race -overlay=/tmp/korvun-p3f0-evidence-20260920/red-overlay.json ./internal/action/executor -run '^TestCompatibility_ImmediateArgumentsRemainByteExact$' -count=1 -v
FAIL
EXIT: 1

$ go test -race -overlay=/tmp/korvun-p3f0-evidence-20260920/red-overlay.json ./internal/action/executor -run '^TestCompatibility_DecisionPrecedenceAndObservations$' -count=1 -v
FAIL
EXIT: 1

$ go test -race -overlay=/tmp/korvun-p3f0-evidence-20260920/red-overlay.json ./internal/action/executor -run '^TestResume_UsesClaimedOperationAndSnapshot$' -count=1 -v
FAIL
EXIT: 1

$ go test -race -overlay=/tmp/korvun-p3f0-evidence-20260920/red-overlay.json ./internal/action/executor -run '^TestExecution_CloseAndRecoveryRemainCompatible$' -count=1 -v
FAIL
EXIT: 1

$ go test -race -overlay=/tmp/korvun-p3f0-evidence-20260920/red-overlay.json ./internal/action/executor -run '^TestCompatibility_NoRecorderStillUsesCanonicalExecutor$' -count=1 -v
FAIL
EXIT: 1
```

The overlay files were temporary test scaffolding. They are not in the
worktree or the staged delivery.

## GREEN verification

### Baseline and final quality gate

The same command ran on base and after the hostile review was cleared:

```text
$ make quality
```

The baseline output contained these exact result lines:

```text
GO_PKGS guard: 41 packages, node_modules-free.
Coverage: 90.3%
adversary-gate-probe: all probes hold.
Ran 18 tests in 0.104s
OK
Ran 23 tests in 26.846s
OK
Quality gate passed.
```

The final post-APTO output contained:

```text
GO_PKGS guard: 41 packages, node_modules-free.
Checking gofmt...
Checking goimports...
Coverage: 90.1%
adversary-gate-probe: all probes hold.
Ran 18 tests in 0.091s
OK
Ran 23 tests in 24.030s
OK
Quality gate passed.
```

That command ran `go vet`, `golangci-lint`, the complete Go suite under
`-race`, the full internal coverage gate, all eight fuzz targets at 25000
executions each, the hook probes, integration-gate tests and rebase-evidence
tests.

### Eight moulds after APTO

```text
$ go test -race ./internal/action/executor ./internal/brain ./internal/app -run '^(TestExecutor_ASTRejectsEveryExternalDispatch|TestExecutor_RejectsUnboundRequest|TestSubmit_RejectedBindingDoesNotConsumeTheRequest|TestSubmit_AllBranchesHaveOneCanonicalAction|TestCompatibility_ImmediateArgumentsRemainByteExact|TestCompatibility_DecisionPrecedenceAndObservations|TestResume_UsesClaimedOperationAndSnapshot|TestResumeApproved_UsesTheSnapshotReadBeforeTheClaim|TestExecution_CloseAndRecoveryRemainCompatible|TestCompatibility_NoRecorderStillUsesCanonicalExecutor)$' -count=1
ok  	github.com/Sebastian197/korvun/internal/action/executor	22.115s
ok  	github.com/Sebastian197/korvun/internal/brain	2.400s
ok  	github.com/Sebastian197/korvun/internal/app	2.517s
```

### Touched-package coverage

```text
$ go test -race -coverprofile=/tmp/korvun-p3f0-evidence-20260920/final-approved-cover-executor.out ./internal/action/executor
ok  	github.com/Sebastian197/korvun/internal/action/executor	22.056s	coverage: 88.6% of statements

$ go test -race -coverprofile=/tmp/korvun-p3f0-evidence-20260920/final-approved-cover-brain.out ./internal/brain
ok  	github.com/Sebastian197/korvun/internal/brain	3.132s	coverage: 92.9% of statements

$ go test -race -coverprofile=/tmp/korvun-p3f0-evidence-20260920/final-approved-cover-app.out ./internal/app
ok  	github.com/Sebastian197/korvun/internal/app	25.795s	coverage: 85.6% of statements

$ go test -race -coverprofile=/tmp/korvun-p3f0-evidence-20260920/final-approved-cover-tool.out ./internal/tool
ok  	github.com/Sebastian197/korvun/internal/tool	4.563s	coverage: 92.3% of statements
```

All four touched packages meet the 85 percent floor.

### Security and vulnerability checks

The security linter used the same package discovery rule as the repository:

```text
$ GO_SCAN_PKGS=$(go list -e -f '{{if not .Error}}{{.ImportPath}}{{end}}' $(find . \( -name node_modules -o -name .git \) -prune -o -type f -name '*.go' -print | sed 's|/[^/]*$||' | sort -u)
$ GO_SCAN_DIRS=$(echo "$GO_SCAN_PKGS" | sed 's|github.com/Sebastian197/korvun|.|')
$ $(go env GOPATH)/bin/golangci-lint run --enable gosec $GO_SCAN_DIRS
```

Output: empty. Exit status: 0.

```text
$ GO_SCAN_PKGS=$(go list -e -f '{{if not .Error}}{{.ImportPath}}{{end}}' $(find . \( -name node_modules -o -name .git \) -prune -o -type f -name '*.go' -print | sed 's|/[^/]*$||' | sort -u)
$ $(go env GOPATH)/bin/govulncheck $GO_SCAN_PKGS
No vulnerabilities found.
```

### Real local model compatibility

This command ran both before implementation and after APTO. Neither run
skipped a test:

```text
$ KORVUN_LIVE_OLLAMA=1 KORVUN_LIVE_OLLAMA_NOTOOLS=1 go test -race ./internal/brain -run '^TestLive_(textLane_echoThroughPromptProtocol|nativeLane_readFileThroughTheJail|nativeLane_shadowNeverExecutesWithARealModel|nativeLane_degradesOnNoToolModel)$' -count=1 -v
```

Baseline output:

```text
--- PASS: TestLive_textLane_echoThroughPromptProtocol (6.40s)
--- PASS: TestLive_nativeLane_readFileThroughTheJail (7.49s)
--- PASS: TestLive_nativeLane_shadowNeverExecutesWithARealModel (15.18s)
--- PASS: TestLive_nativeLane_degradesOnNoToolModel (0.56s)
PASS
ok  	github.com/Sebastian197/korvun/internal/brain	31.359s
```

Final output:

```text
--- PASS: TestLive_textLane_echoThroughPromptProtocol (9.86s)
--- PASS: TestLive_nativeLane_readFileThroughTheJail (8.70s)
--- PASS: TestLive_nativeLane_shadowNeverExecutesWithARealModel (10.34s)
--- PASS: TestLive_nativeLane_degradesOnNoToolModel (2.58s)
PASS
ok  	github.com/Sebastian197/korvun/internal/brain	33.134s
```

The text-lane live test was added in this phase because the previous live file
covered only the native lane. Its killed mutation bypassed `Submit`; the test
then failed with no audit event.

### Documentation, schema and dependencies

```text
$ go doc github.com/Sebastian197/korvun/internal/tool.ScopedTool
ScopedTool is the OPTIONAL conversation-identity capability of a Tool ...

$ go doc github.com/Sebastian197/korvun/internal/tool.MemoryNote.Execute
Execute implements Tool through the same private implementation as ExecuteScoped ...

$ go doc github.com/Sebastian197/korvun/internal/action/executor.Executor.ResumeApproved
ResumeApproved performs the current prechecks and claim through ApprovalStore ...

$ go doc github.com/Sebastian197/korvun/internal/app.ExecuteApprovedAction
ExecuteApprovedAction runs the EXACT stored envelope of an APPROVED request ...

$ rg -n '^const schemaVersionCurrent = 12$' internal/action/sqlite/store.go
126:const schemaVersionCurrent = 12

$ git diff --exit-code -- go.mod go.sum
```

The final command produced no output and exited 0.

## Production functions and surfaces changed

`internal/action/executor` now owns the protocol. The changed or new functions
are `New`, `NewCoordinator`, `SelectTools`, `Decisions`, `Prepare`, `Submit`,
`observeDecision`, `validateRequest`, `recordAttempt`,
`refreshDurableEffect`, `recordAuthorized`, `bindIdentity`, `effectGateRule`,
`invokeAndClose`, `cloneInbound`, `immediateCloser`, `dispatch`,
`(*ResumeError).Error`, `(*ResumeError).Unwrap` and `ResumeApproved`. The old
public raw-name `Run` entry point was removed.

In `internal/brain`, `NewAgentBrain`, `Handle`, `runLoop`, `runTool` and
`effectiveTools` now carry the opaque plan and delegate execution. The new
presentation functions are `presentToolResult`, `observeImmediateDecision`,
`logPreDenialDiagnostics`, `observeImmediateExecution`,
`observeIdentityFallback`, `governanceFromDecisions`,
`logAttemptDiagnostics` and `logCloseDiagnostic`. The former action-building,
recording and finishing helpers were removed. The recorder, identity and
effect seams are aliases or adapters for executor-owned contracts.

In `internal/app`, `ExecuteApprovedAction` now delegates to `ResumeApproved`.
The `approvedExecutionStore` methods adapt the existing SQLite operations:
`ReadApproval`, `ReadActionState`, `Claim`, `Close` and `ReadReceipt`.

In `internal/tool`, `MemoryNote.Execute` and `ExecuteScoped` share the private
`execute` helper. The `ScopedTool` and `MemoryNote.Execute` godoc now describes
the actual dispatch relation.

## Contracts and functions not changed

The following production functions were not edited:

- `runLoopNative`, `callNative`, `toToolSpecs`, `nativeCallArgs`,
  `rescueTextToolCall` and `nativeArgs`;
- `cageRule`, `auditTool`, `loadHistory` and `persistPair`;
- `tool.CloseStateAfterRun`;
- `actionsqlite.Store.ClaimApprovalParamsUnderDigest`,
  `actionsqlite.Store.FinishWithResult` and
  `actionsqlite.Store.RecoverPreviousLife`.

The public `AgentBrain.Handle` body changed only to carry the coordinator plan;
its response, fallback, persistence and error contracts remain under the
existing compatibility suite. Cage, shield, retry, model, orchestrator,
preflight, reload and policy-selection implementations were not changed.

The SQLite transaction bodies, schema, retention, receipts and recovery source
were not edited. `ExecuteApprovedAction` keeps the same signature and
`ApprovedExecution` result. The existing `WithoutCancel`, claim-sentinel and
`OUTCOME_UNKNOWN` behavior is exercised by the Phase 0 moulds and the complete
suite.

## Existing tests changed, before and after

Only five existing test bodies changed:

| Test | Before | After |
| --- | --- | --- |
| `TestTripwire_theOnlyPathToExecuteIsThisPackage` | Regex search with broad directory exclusions. | Replaced by `TestExecutor_ASTRejectsEveryExternalDispatch`, which parses and type-checks every production package for six platform/tag configurations and admits only the exact dispatch sites. |
| `TestRun_plainAndScopedRouting` | Called `Run` with a caller-supplied scope; expected `b` and `c`. | Uses `Prepare` and `Submit`; scope is derived from the fixed brain and inbound envelope, so it expects `b` and `console::c`. |
| `TestRun_unknownToolSentinel` | Called `Run` and checked `ErrUnknownTool`. | Submits a canonical request and checks the same sentinel. |
| `TestRun_perToolTimeoutBoundsExecution` | Timed a raw `Run` call. | Times the same tool through a canonical submission. |
| `TestRun_toolErrorPassesThroughUnclassified` | Read partial output and the tool error from `Run`. | Reads the same partial output from `Result` and the same unclassified error from `Submit`. |

`TestLive_textLane_echoThroughPromptProtocol` is new, not a changed contract.
All other existing tests are unchanged.

## Hostile review

The independent pre-test review ran before the first RED test. Delta review
then repeated after each cure. It found and forced fixes for hidden-helper and
method-value bypasses, type aliases, stateful classifier read drift, identity
fallback ordering, concurrent replay, fixed-governance replacement and a
nil-recorder close-clock read.

The final verdict was APTO. It reported no open P1 or P2 finding and accepted
the evidence level for concurrency, aliases, nil-recorder mode, common close,
stateful classifiers, snapshots, sentinels, precedence, raw arguments and
recovery.

## Outside this phase

This phase does not change schema, migrations, dependencies, accepted ADRs,
approval UI, CLI protocol, model adapters, cage or shield policy, receipt
format, recovery policy, release metadata or generated bundles. It does not
commit, push, merge or publish the branch.

## The pass over the complete diff (2026-09-20)

The internal adversary audited the committed diff — one pass, by the
director's order of today for this train. Verdict: `VETO MANTENIDO`, no P1, two
P2 and three P3. Both P2 are cured in this same tree; each cure carries the
auditor's own reproduction as its acceptance criterion.

### P2-1, cured — the structural wall was blind to a whole syntactic category

The claim under attack is AS-EXEC-01: "the only physical tool invocation sites
are the exact registry sites in `executor.dispatch`". The walk in
`dispatchViolations` and `coordinatorViolations` started at `fn.Body` after a
`decl.(*ast.FuncDecl)` filter, so every `*ast.GenDecl` was skipped unvisited. A
package-level `var` whose initializer is a function literal — a shape this
repository already writes in production (`internal/action/sqlite/store.go`,
`var migrationCopies = map[int]func(*sql.Tx) error{`) — therefore never
reached the guard. The auditor wrote a reachable EXPORTED bypass
(`internal/brain/zz_backdoor.go`: a package-level closure calling
`t.Execute`, a map of closures calling `t.ExecuteScoped`, and an exported
`(*AgentBrain).RunUngoverned` that invokes any registered tool with no
canonical action, no decision, no record, no close and no audit) and it passed
`go build`, `go vet`, the guard and all four packages' suites.

Cure: both walks now cover EVERY declaration of every file. Inside a function
body the judgement is unchanged; outside one, a `Tool.Execute` /
`ScopedTool.ExecuteScoped` selector is a violation by construction (it has no
adapter contract and can never be the allowed dispatch), and a coordinator
method referenced from a package-level declaration is a
`canonical_coordinator_bypass`.

Acceptance criterion, the auditor's reproduction verbatim, EXECUTED here: the
same `zz_backdoor.go`, byte for byte, dropped into a copy of this tree.

```text
$ go build ./internal/brain && go vet ./internal/brain
(clean)
$ go test ./internal/action/executor -run '^TestExecutor_ASTRejectsEveryExternalDispatch$' -count=1
--- FAIL: TestExecutor_ASTRejectsEveryExternalDispatch (10.95s)
    tripwire_test.go:117: canonical execution structure violated:
        direct_tool_dispatch …/internal/brain/zz_backdoor.go:10:9: Tool.Execute and ExecuteScoped are confined to executor.dispatch
        …(x6, one per GOOS/tag configuration)
        direct_tool_dispatch …/internal/brain/zz_backdoor.go:23:10: Tool.Execute and ExecuteScoped are confined to executor.dispatch
        …(x6)
```

Both shapes are caught, in all six scanned configurations. With the file
removed the same command returns `ok … 9.610s`.

Declared and NOT cured, from the same finding: the scan still pins
`GOARCH=amd64`, scans only the tags `""` and `desktop`, and only the roots
`./internal/...`, `./cmd/...` and `./web/builder`. No arch-constrained or
out-of-root production file exists today, so the holes are latent; a
`reflect`-built call is invisible to any AST guard and is an accepted limit,
stated here rather than implied.

### P2-2, cured — "zero observable change" was false on the audit bus

`Prepare` stored `cloneInbound(submission.Inbound)` and handed that copy to
`BeforeClose`, `BeforeRecord` and `BeforeIdentityFallback`, so
`bus.Event.Envelope` stopped being the live inbound message on every immediate
branch except the parked one, which still carried the live pointer. The
auditor captured it with a 23-scenario behavioural differential run against
both `adb44231` and this commit; the same run found every other observable
byte-identical. The clone was also HALF-deep (`Keyboard` and `Operation`
stayed shared) and turned an empty-but-non-nil `Parts` into `nil`.

Cure: the request keeps the caller's pointer, and `cloneInbound` is gone. The
criterion of this phase is zero observable change, so the observable object is
the one it was before.

New mould, `TestRunTool_AuditCarriesTheLiveInboundEnvelope` (executed and
denied rows): the audited envelope must be pointer-identical to the inbound
one. Its probing mutation restores the snapshot in `Prepare`:

```text
--- FAIL: TestRunTool_AuditCarriesTheLiveInboundEnvelope (0.00s)
    --- FAIL: …/executed: agent_canonical_execution_test.go:280: audit envelope = 0x3d52da682510, want the live inbound 0x3d52da682480
    --- FAIL: …/denied:   agent_canonical_execution_test.go:280: audit envelope = 0x3d52da682750, want the live inbound 0x3d52da6826c0
```

Mutation reverted; the mould is green in this tree.

### P3-2, cured — six of seven citations named lines that hold other code

The "Representative killed outputs" citations were taken against an earlier
revision of the test file. Re-derived against this commit: 227→258, 423→551,
461→587, 541→835, 574→868, 895→936; 530 was already right. The repository's
own law admits a line number only when it is re-derived against the final
commit, which is what this paragraph records.

### Filed, not cured, with the director's adjudication pending

- **P3-1.** `governanceFromDecisions` (`internal/brain/agent.go`) is production
  code that production never reaches: it rebuilds a `*executor.Governance` out
  of a caller's decisions map so tests may keep calling `runTool` directly. The
  auditor instrumented it and ran the whole brain suite — eleven
  reconstructions, `Handle` in zero of the eleven stacks — and a `panic` probe
  named the moulds that travel it, three of them added by this commit. The
  round-trip is faithful for every rule `policy.SelectTools` can emit, so no
  wrong decision is produced today; what is filed is that those moulds assert
  over a reconstruction instead of the sealed plan. Curing it means moving
  those moulds onto the production door, which is a change of test doors and
  belongs to its own piece.
- **P3-3.** `presentToolResult`'s `default:` arm ("agent: canonical tool result
  has no branch") is structurally unreachable — every `Submit` return sets a
  branch — and has no test. Defensive dead code, recorded, not covered.
- The auditor's own largest declared gap: `app.ExecuteApprovedAction` was
  audited by reading both revisions line by line and by the executor-level
  moulds, not by a two-tree execution against a real SQLite file.

### What ran after the cures

```text
$ go test -race ./internal/action/executor ./internal/brain ./internal/app ./internal/tool -count=1
ok  	github.com/Sebastian197/korvun/internal/action/executor	35.574s
ok  	github.com/Sebastian197/korvun/internal/brain	4.773s
ok  	github.com/Sebastian197/korvun/internal/app	55.142s
ok  	github.com/Sebastian197/korvun/internal/tool	4.771s
```

`make quality` was run in this worktree before the cures (exit 0, "Quality gate
passed", with coverage 88.6 / 92.9 / 85.6 / 92.3 percent for executor, brain,
app and tool) and again by the pre-commit gate over the cured tree. The real
local-model compatibility suite ran here with both opt-in variables and no
skips: four PASS, `ok … 46.241s`.
