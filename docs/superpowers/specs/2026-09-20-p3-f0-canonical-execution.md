# Piece 3, Phase 0: Canonical Execution Design Spec

> **Status:** implemented and verified on 2026-09-20; hostile delta review
> APTO; director acceptance pending.
> Governing material: `CLAUDE.md`, ADR-0021, ADR-0041, ADR-0043, and
> `design-drafts/2026-09-19-pieza-3-identidad-intencion-autoridad-codex.md`.
> This phase uses only the standard library and existing `internal/`
> packages. It adds no dependency and keeps the action store at schema 12.

## Goal

Every attempted tool call is represented once by an `action.Envelope` and its
parameters digest before a decision can lead to dispatch. Immediate and
approved calls converge on one coordinator in `internal/action/executor` for
admission or claim, private invocation, and terminal close. `runTool` remains
the brain-facing presentation adapter. `ExecuteApprovedAction` keeps its
signature and `ApprovedExecution` contract while delegating the execution
protocol.

This is a structural refactor. Observable behavior, protocol bytes, decision
precedence, persistence transactions, error taxonomy, and schema stay as they
are on `master` at `adb44231a6635f8d1abd48196ef892e74809b935`.

## Current-tree adaptation

The design draft uses `v0.15.0` as its compatibility baseline. Three hardening
changes landed by `v0.15.1`; the implementation treats them as law:

- `ClaimApprovalParamsUnderDigest` returns the operation judged inside the
  claim and compares the full caller snapshot before consuming parameters.
- `tool.CloseStateAfterRun` is the only outcome classifier for both paths.
- terminal close and receipt read-back use `context.WithoutCancel`; recovery
  closes stranded `AUTHORIZED` and `APPROVED` rows as `OUTCOME_UNKNOWN`.
- approved close timestamps keep their wall clock independently from the
  executor latency clock; immediate close keeps the injected brain clock.
- successful immediate execution publishes its existing audit event after
  dispatch and before the terminal ledger close.
- non-executing immediate decisions publish their existing log and audit event
  before the current best-effort terminal record.

These changes narrow the implementation choices without conflicting with the
Phase 0 goal. No older shape from the draft may replace them.

## Functional requirements

- **FR-EXEC-01.** `brain.(*AgentBrain).runTool` asks the coordinator to prepare
  an opaque request from the inbound envelope, lane, tool name, original
  arguments, and decision plan, then submits that request. Preparation
  constructs one canonical action and derives scope before decision.
  `runTool` translates the coordinator result into the existing logs, events,
  observations, and return string.
- **FR-EXEC-02.** The public executor surface accepts an action-bound request.
  There is no public production method that dispatches by tool name and raw
  arguments alone. A request whose operation or parameters do not re-derive
  its envelope digest fails with `ErrExecutionBindingMismatch` before any
  recorder, approval requester, closer, or tool is called.
- **FR-EXEC-03.** `internal/action/executor` owns decision precedence,
  parking, authorization recording, approved claim coordination, the private
  invocation capability, and terminal close. It imports neither `brain` nor
  `app`; adapters satisfy narrow executor-owned interfaces.
- **FR-EXEC-04.** The coordinator calls `policy.SelectTools` once and wraps a
  successful result in an opaque plan bound to that executor instance and
  inbound channel. The same call returns the advertised registry. If selection
  fails, the coordinator returns the current empty advertisement plus a valid,
  bound deny-all plan and the selector error separately. The brain adapter logs
  that error at Error level with the existing message and the exact
  `envelope_id`, `channel`, and `cause` attributes. This is distinct from a nil
  or zero plan and preserves the current fail-closed behavior for all five
  selector error families: unknown sensitivity, unknown locality, unknown tool
  mode, duplicate grant, and empty grant name.
  Preparation and `Submit` reject nil, zero, foreign, or channel-mismatched
  plans.
  Precedence remains:
  unknown tool, undeclared effect, shadow or capability denial, effect
  ceiling, approval, authorized start. Ungoverned mode also uses an opaque
  plan; nil never means allow.
- **FR-EXEC-05.** Approved resumption invokes only the operation and bytes
  returned by `ClaimApprovalParamsUnderDigest`, after the claim compares the
  complete pre-read `action.Approval` snapshot. It never reconstructs the
  operation from the earlier read.
- **FR-EXEC-06.** The only production selectors that dispatch
  `tool.Tool.Execute` or `tool.ScopedTool.ExecuteScoped` are the registry
  dispatch sites inside `internal/action/executor`. `MemoryNote.Execute` and
  `ExecuteScoped` share a private implementation helper instead of one public
  method calling the other.
- **FR-EXEC-07.** Both execution paths close through
  `tool.CloseStateAfterRun` on a `context.WithoutCancel` context. Existing
  claim sentinels, receipt read-back, close errors, and crash recovery to
  `OUTCOME_UNKNOWN` remain intact.
- **FR-EXEC-08.** Digest canonicalization never replaces immediate raw
  arguments. Approved execution uses the exact stored bytes returned by the
  claim.
- **FR-EXEC-09.** A nil recorder remains a supported in-memory mode. It still
  constructs and decides a canonical request, opens no SQLite store, and
  produces the same tool result and observation. It does not consult the
  terminal close clock because no close exists.
- **FR-EXEC-10.** Durable denial remains best effort. Durable authorization
  failure remains fail closed. Approval birth failure falls back to the same
  `approval_unavailable` denial. A terminal write failure after dispatch
  remains evidence loss and never triggers a retry.
- **FR-EXEC-11.** Immediate scoped execution accepts no caller-supplied
  `tool.Scope`. The coordinator snapshots the inbound envelope, derives the
  conversation with `conversation.KeyFromEnvelope`, and combines it with the
  brain name fixed when that executor was built. Approved execution retains
  its current empty scope.
- **FR-EXEC-12.** Known tool names enter the action in full. Unknown names keep
  the existing `boundedArgs` representation before the one action is built,
  while the raw name remains available only for the exact not-found
  observation and bounded local log. The digest follows the stored operation
  name exactly.
- **FR-EXEC-13.** The effect classifier's historical declaration check and any
  later descriptor read keep their existing order and count. The first read
  continues to decide the undeclared wall; the identity gate uses its existing
  later read; and the durable envelope uses its own existing read, including
  after a failed approval birth. A later absent descriptor keeps the current
  gate behavior and records an `unclassified` effect when that read feeds a
  durable action. Stateful classifiers may therefore expose different gate and
  durable values, as they do on the baseline. The coordinator creates one
  action id, operation, and digest; a later historical durable read may update
  only that action's effect evidence before its sink and never rebuilds it.
- **FR-EXEC-14.** A missing `conversation.id` is compatible input. `Prepare`
  preserves the current behavior by deriving an empty conversation scope and
  continuing execution; it does not propagate the
  `conversation.KeyFromEnvelope` error. A present id is copied exactly.
- **FR-EXEC-15.** `Prepare` copies every inbound fact used after preparation,
  including channel, conversation, and sender identity. Mutating the caller's
  envelope after `Prepare` cannot change scope or recorded identity.
- **FR-EXEC-16.** Every prepared request is a single-use capability. All value
  or pointer copies share one private atomic consumption cell. Exactly one
  concurrent `Submit` can consume it; every later submission returns
  `ErrExecutionAlreadySubmitted` before recorder, approval requester, closer,
  or tool. Before attempting that atomic claim, `Submit` validates nil and
  zero requests, cell presence, owner, plan, operation, and digest. Any such
  malformed input returns `ErrExecutionBindingMismatch` without panic and
  without consuming a cell. This rule also applies when the recorder is nil.

## Coordinator shape

`executor.Submission` carries an inbound-envelope snapshot, lane, raw tool
name, original argument bytes, and an opaque `DecisionPlan`. Its fields do not
include scope or a policy map. The plan has unexported owner, channel,
ungoverned, and decision fields. Only that executor's `SelectTools` method can
mint a valid plan. `Prepare` validates the plan, derives scope, creates the
action, and returns an opaque `Request` whose action, original bytes, scope,
and plan fields are unexported. `Submit` revalidates owner, channel, operation,
and digest before it consults any adapter. The request also holds a pointer to
an unexported atomic consumption cell, so copying the exported opaque value
cannot copy or reset its right to submit.

After plan validation, `Prepare` checks registry existence to preserve the
current known-name versus bounded-unknown representation. It performs the
historical declaration read and calls its single action factory. The factory
owns id, clock, source, operation, initial effect, and digest construction. Its
id generator is injectable for the AS-EXEC-03 count oracle. `Submit` performs
the later gate and durable reads only where the baseline path reaches them. A
failed approval birth performs the baseline's additional terminal durable read
by updating only the existing action's effect evidence. The coordinator
derives scope from copied inbound facts and its construction-time brain name.
A missing conversation id yields the current empty conversation scope rather
than a preparation error. Its exported result reports the finite branch, rule,
action, result, latency, execution error, approval id, and recording
diagnostics needed by adapters. It does not contain presentation text.

`executor.Executor.Submit` accepts only the opaque action-bound request. It
validates the request and its private cell before it atomically consumes that
cell or consults an adapter. It then performs the immediate decision and, only
after a successful authorized record, creates an unexported invocation
capability. A losing concurrent or repeated submission returns
`ErrExecutionAlreadySubmitted` with no downstream work. A nil or zero request,
including `&executor.Request{}`, returns `ErrExecutionBindingMismatch` without
panic and leaves all counters at zero.
`executor.Executor.ResumeApproved` obtains the same unexported capability only
after the approval adapter's current prechecks and atomic claim succeed. Both
paths call one private invoke-and-close routine. A plain caller-created allow
value is not an invocation API: no exported type lets it populate the private
plan fields, and a plan from another executor or channel fails before effects.

The executor owns narrow interfaces for attempt recording, identified
recording, approval birth, identity binding, approved reads and claim, close,
and receipt read-back. Existing brain and app adapters implement or alias
those contracts. SQLite remains the transaction owner; the coordinator does
not duplicate store logic.

## Pre-test adversarial review

### Literal guarantees

1. No production code outside the exact private registry dispatch sites in
   `internal/action/executor` can call, capture, or transfer a method with the
   `Tool.Execute` or `ScopedTool.ExecuteScoped` execution signature.
2. No tool dispatch occurs without one previously constructed action whose
   operation and parameters digest bind the exact invocation.
3. Unknown, undeclared, deny, shadow, pending, and allow outcomes use that one
   action. Only allow or a successful approved claim can mint the private
   invocation capability.
4. Immediate and approved execution call the same private invocation and
   close routine without changing externally observed behavior.
5. The current SQLite claim, close, receipt, and recovery guarantees remain
   unchanged, including their error classes and transaction ownership.
6. A caller cannot replace the `SelectTools` result or derived scope with
   request fields. Both are bound inside the executor before admission.

### Attack matrix

| Attack | Dangerous branch forced | Required oracle |
|---|---|---|
| Direct, parenthesized, captured, method-expression, and equivalent-interface dispatch | A caller attempts to escape the coordinator without changing selector spelling in an obvious way. | Type-resolved analyzer reports `direct_tool_dispatch` at the external site; the exact registry sites remain accepted. |
| Action A with operation or argument bytes B | A caller presents an allowable name with a digest for another request. | `ErrExecutionBindingMismatch`; tool, recorder, approval requester, and closer counters stay zero. |
| Nil, zero, foreign-owner, or wrong-channel decision plan | A caller fabricates allow, substitutes another result, or uses nil as ungoverned. | `ErrExecutionBindingMismatch`; preparation creates no action and every downstream counter stays zero. |
| Policy selection fails | Any of the five selector error families turns fail-closed governance into an invalid plan, preparation error, ungoverned allow, or silent failure. | Empty advertisement and a valid bound deny-all plan; the adapter emits the existing Error log with envelope, channel, and cause; the call receives the existing denial observation, records the finite denial, and never dispatches. |
| Conversation A with an attempted scope for B | A caller tries to redirect `memory_note` while preserving arguments. | Submission has no scope field; the executed scope equals the value derived from the copied inbound envelope and fixed brain name. |
| Missing conversation id | A stricter derivation rejects a tool call that currently runs with an empty conversation scope. | The scoped tool runs once with the fixed brain and an empty conversation; no preparation error is returned. |
| Envelope sender changes after preparation | A caller-owned alias changes identity evidence while the action and digest remain valid. | Recorded provenance retains the sender copied by `Prepare`; later envelope mutation has no effect. |
| Historical classifier reads collapse into one value | Stateful compatibility changes the undeclared wall, identity gate, or durable evidence. | The first read controls declaration, the gate read controls admission, and the applicable durable read supplies the same action's recorded effect; call counts and values match the baseline. |
| Unknown plus a permissive decision | Nonexistence competes with a fabricated allow. | Unknown wins; finite audit label and exact observation remain; zero dispatch. |
| Shadow plus irreversible effect and approval requester | A later effect gate could override the rehearsal decision. | Shadow wins; exact rehearsal observation; zero approval births and dispatches. |
| Above-ceiling action plus approval requester | Approval could be used to exceed authority. | `effect_ceiling` wins; zero approval births and dispatches. |
| Approval-required action whose birth fails | A storage failure could fall open or lose its stable rule. | Same `approval_unavailable` denial; durable denial remains best effort; zero dispatch. |
| Authorization record fails | Proof could be skipped before the external effect. | `record_failed`; zero dispatch and zero close. |
| Canonical JSON differs from raw bytes | Digest normalization could leak into invocation. | Tool receives the original bytes exactly; digest re-derives from canonical parameters. |
| Approval row moves after the outer read on a second connection | Stale prechecks could consume or execute moved evidence. | `ErrApprovalMovedUnderTheClaim`; an abort trigger and direct read prove no parameter consumption; zero dispatch. |
| Claim returns an operation different from the outer read | Adapter could execute the stale operation. | Only the claimed operation counter increments. |
| Effect returns delivered error, deadline, or cancellation | A refactor could convert uncertainty to definite failure. | `OUTCOME_UNKNOWN`; close lands through an uncancelled context; no retry. |
| Process lifecycle finds a stranded authorized or approved start | Recovery could rewrite uncertainty as failure or permit rerun. | Existing crash-recovery receipt and marker say `OUTCOME_UNKNOWN`; a second recovery is a no-op. |
| Nil recorder in text and native lanes | Coordinator could make SQLite or durable proof mandatory, or the adapter could bypass it. | Exact result and arguments, coordinator invocation counter one, no storage path created. |
| Unknown name longer than the log bound, then the same name registered | Uniform name handling could move historical operation and digest bytes. | Unknown record uses the exact bounded name and matching digest; known record and invocation use the full name. |
| Duplicate action construction whose first result is discarded | A second birth could be invisible in all records. | Injected id factory count is exactly one in every branch; a discarded extra construction makes it two. |
| A valid prepared request is copied or submitted concurrently | Stateless execution repeats one external effect under the same action id and digest. | Two synchronized submissions of value copies have one winner; the loser returns `ErrExecutionAlreadySubmitted`; id and dispatch counters both remain one. |
| Nil or zero request submitted directly | The public zero value panics on a missing consumption cell or reaches an adapter without preparation. | Nil, `Request{}`, and a request with an absent cell return `ErrExecutionBindingMismatch` before the CAS; every downstream counter stays zero. |
| Invalid operation or arguments consume the request | A rejected request cannot be repaired and submitted under its original binding. | The exact binding sentinel leaves the shared cell unclaimed; restoring the sealed value permits one successful submission. |
| Legacy coordination restored in either production adapter | New coordinator methods exist but are not the production route. | Type-resolved callsite guard requires exactly one `Prepare` and `Submit` in `runTool`, exactly one `ResumeApproved` in `ExecuteApprovedAction`, rejects their former coordination calls, and rejects exact or equivalent coordinator calls in every helper. |
| Identity fallback warning moves after the legacy recorder | A recorder abort suppresses provenance diagnostics that the baseline already published. | A panicking fallback recorder still leaves the warning captured before the panic. |

### Failure taxonomy

- A malformed or mismatched execution request returns
  `ErrExecutionBindingMismatch` and performs no downstream call.
- A second or concurrent submission of any copy of a valid request returns
  `ErrExecutionAlreadySubmitted`. Exactly one submission can perform recorder,
  approval, close, or tool work.
- A missing, zero, foreign, or channel-mismatched decision plan returns the
  same binding sentinel before action construction. Nil is never ungoverned.
- A `policy.SelectTools` error is not a binding error. It produces a valid
  executor-owned deny-all plan, the current empty advertisement, and the
  adapter's current Error-level misconfiguration log, including envelope id,
  channel, and cause, before normal denial processing. The rule applies to all
  five named selector error families.
- An absent registry entry remains `ErrUnknownTool` at the executor seam and
  the exact `tool %q not found` observation at the brain seam.
- Undeclared effect, capability denial, shadow, ceiling, approval unavailable,
  and record failure keep their current finite rules and event types.
- A failed approval claim preserves the exact `actionsqlite` sentinel through
  the existing `app: claim execution of ...` wrapper. It is not converted to a
  race loss.
- An unreadable parked action remains `ErrApprovalRecordUnreadable`; pending
  remains `ErrApprovalNotDecided`; closed remains
  `ErrApprovalAlreadyClosed`; a failed close or receipt read remains
  `ErrApprovalCloseFailed`.
- Tool errors remain execution results. `tool.CloseStateAfterRun` alone
  distinguishes `FAILED` from `OUTCOME_UNKNOWN`; neither class becomes an API
  error that invites retry.

### Transaction and persistence boundaries

- `RecordAttempt` and `RecordAttemptIdentified` retain their existing single
  store transactions. No schema, constraint, or migration changes.
- `CreateApprovalRequest` remains the born-whole transaction for action,
  request, preview, protected parameters, and their cross-links.
- `ClaimApprovalParamsUnderDigest` remains the only transaction that compares
  the complete approval snapshot, law, stored digests, authority, and purge.
  The coordinator consumes only the operation and bytes returned after its
  commit.
- Tool invocation occurs after authorization record or approved claim commit;
  it is never inside a SQLite transaction.
- `FinishWithResult` remains the terminal state and receipt transaction. It
  receives `context.WithoutCancel(ctx)` after an effect may have occurred.
- `RecoverPreviousLife` is unchanged. Its existing transaction closes
  stranded starts and writes the recovery receipt once.

### Planned reproductions and evidence levels

- AS-EXEC-01: in-process source analyzer over the active darwin, linux, and
  windows file sets, with default and desktop production tags, plus separately
  applied valid production mutations. It proves buildable repository
  structure, not runtime loading.
- AS-EXEC-02 through AS-EXEC-05 and AS-EXEC-08: in-process Go tests with
  counting or aborting fakes. They prove coordinator and adapter behavior.
- AS-EXEC-06: multiple real SQLite connections to one file, with the attack
  commit confirmed before claim. It proves rollback and preservation in the
  real store, not cross-process locking.
- AS-EXEC-07 close cases: real SQLite file in process. Recovery case: close,
  reopen, and run the real recovery pass; it is crash-restart simulation, not
  an OS-process crash.
- Section 24 protocol cases: in-process suites plus local Ollama processes for
  native-tool, text-tool, and no-tool-degradation tests. The command asserts
  every named test passed and none skipped. It does not prove remote-provider
  availability.

### Unresolved risk

Another connection can restore protected approval parameters after a
successful claim commit while the action is still approved. This is the known
v0.15.2 filing in current source. Phase 0 neither widens nor claims to close
that window.

## Functions that change

| Surface | Declared change |
|---|---|
| `executor.New` and `executor.Executor` | Compose the coordinator and private registry dispatcher. |
| `executor.(*Executor).Run` | Removed as a name-and-args door; replaced by action-bound submit and approved resume. |
| `brain.NewAgentBrain` | Builds the coordinator after all options are applied, including fixed brain name, governance inputs, identity, recorder, classifier, and clock. |
| `brain.(*AgentBrain).runTool` | Becomes a submit/result presentation adapter. |
| `brain.(*AgentBrain).effectiveTools` | Delegates the unchanged `SelectTools` inputs to the executor and carries its opaque plan beside the advertised registry. |
| `brain.(*AgentBrain).buildActionEnvelope` | Removed; its one construction moves behind the executor's counted action factory. |
| `brain.ActionRecorder`, `brain.IdentifiedRecorder` | Preserve their exported contracts through executor-owned aliases or adapters. |
| `brain.identify` | Preserves current provenance rules through the executor identity seam. |
| `app.ExecuteApprovedAction` | Keeps signature and result; adapts the SQLite store and translates coordinator stages to current errors. |
| `app.BuildApprovalExecutorFromCage` | Builds the same coordinator over the already-resolved cage. |
| `tool.MemoryNote.Execute`, `ExecuteScoped` | Delegate to one private implementation helper. |

## Functions whose behavior does not change

`brain.Handle`, `runLoopNative`, `callNative`, `nativeCallArgs`, `nativeArgs`,
`rescueTextToolCall`, `toToolSpecs`, `shadowObservation`,
`deniedObservation`, `pendingApprovalObservation`, `auditTool`, `cageRule`,
`loadHistory`, `persistPair`, Orchestrator behavior, model adapters, tool
cages, shields, retries, preflight, reload, policy selection, approval
factories, SQLite transactions, retention, receipts, and recovery retain their
current contracts.

## Acceptance scenarios and destructive mutations

All case matrices are table-driven and run with `-race`.

| ID and mold | Required observation | Production mutation that must redden it |
|---|---|---|
| **AS-EXEC-01** `TestExecutor_ASTRejectsEveryExternalDispatch` | AST plus type resolution over darwin, linux, and windows, with default and desktop production tags, detects direct calls, parenthesized calls, method values, method expressions, type aliases, and equivalent local interfaces outside the exact registry sites. It requires direct calls to `Prepare` and `Submit` exactly once in `runTool`, a direct call to `ResumeApproved` exactly once in `ExecuteApprovedAction`, rejects captures or transfers, scans every production helper for exact or equivalent coordinator calls, and rejects both adapters' former coordination calls. Violations are `direct_tool_dispatch` or `canonical_coordinator_bypass`. | Executed separately: direct dispatch, dispatch method value, equivalent dispatch interface, type-aliased dispatch interface, coordinator method value, equivalent coordinator interface with dead compliant calls, type-aliased coordinator interface, hidden helper coordination, extra `Prepare`, and legacy approved-store read. |
| **AS-EXEC-02** `TestExecutor_RejectsUnboundRequest` plus `TestSubmit_RejectedBindingDoesNotConsumeTheRequest` | Table cases for nil, zero, foreign, and wrong-channel plans fail before action construction; nil, zero, and otherwise-valid requests with a missing use cell fail before that cell is touched; operation and argument mismatches fail before tool, record, approval, close, or consumption. Every case returns the exact `ErrExecutionBindingMismatch`. Repairing a rejected operation or argument binding permits one submission. Concurrent replay returns the exact `ErrExecutionAlreadySubmitted`. Scoped execution derives the fixed brain and conversation from copied inbound facts. A missing conversation id executes with an empty conversation. A caller mutation of sender after `Prepare` cannot change recorded identity. | Executed separately: remove digest binding, remove the missing-use-cell validation, consume before full validation, and wrap the replay sentinel. |
| **AS-EXEC-03** `TestSubmit_AllBranchesHaveOneCanonicalAction` plus the three classifier compatibility molds | Unknown, undeclared, deny, shadow, pending, and allow branches, with every applicable recorder mode and both lanes, call the injected id factory exactly once and retain that action id; prohibited branches never invoke. Long-name cases pin bounded unknown and full known operation and digest bytes. The classifier's declaration, gate, durable, and failed-approval terminal reads retain baseline count and meaning, including first-read absence and differing stateful values. One request copied into two synchronized goroutines is exercised stateless, with durable execution, and with durable approval. Every row has one winner and one exact `ErrExecutionAlreadySubmitted`; recorder, approval requester, closer, id, and dispatch counters show work only from the winner. | Executed separately: add a discarded construction, rebuild at parking, add a construction in durable `recordAttempt`, remove the single-use claim, let a replay loser write a durable row, use the first class after a later critical read, let a later absent descriptor trigger the undeclared wall, skip the durable read after a first miss, skip the third identity-plus-recorder read, and skip the fourth failed-approval read. |
| **AS-EXEC-04** `TestCompatibility_ImmediateArgumentsRemainByteExact` | Leading/trailing spaces, reordered JSON, empty input, and non-JSON reach the tool byte for byte while their digest stays canonical. | Pass `action.CanonicalParams(args)` to the registry dispatcher. |
| **AS-EXEC-05** `TestCompatibility_DecisionPrecedenceAndObservations` plus fixed-governance and decision-order compatibility cases | Simultaneous unknown, shadow, ceiling, and approval conditions retain the baseline rule, event, observation, and zero-call properties. A table drives all five selector errors. Each returns an empty advertisement and a valid deny-all plan, emits the existing Error log with exact message plus `envelope_id`, `channel`, and matching cause, records the existing finite denial, and never dispatches. Missing context cannot replace fixed governance with caller decisions. A non-executing decision audits before its best-effort record. An unresolved-identity warning is published before the legacy recorder is entered, even if that recorder panics. | Executed separately: replace `approval_unavailable` with a generic refusal, replace fixed governance with caller `allow`, move denial audit after its record, and delay the identity fallback warning until after the legacy record. |
| **AS-EXEC-06** `TestResume_UsesClaimedOperationAndSnapshot` plus `TestResumeApproved_UsesTheSnapshotReadBeforeTheClaim` | The executor fake proves that the snapshot reaches `Claim`; the app mold uses the real adapter, store, and a second connection. Moving a pre-read approval column yields the existing sentinel, preserves stored parameters, and invokes nothing; a valid claim uses its returned operation and bytes. | Executed separately: pass a nil snapshot and recompute the snapshot inside `approvedExecutionStore.Claim`. |
| **AS-EXEC-07** `TestExecution_CloseAndRecoveryRemainCompatible` | Delivered error, bare deadline, and cancellation close as `OUTCOME_UNKNOWN`; a stranded start recovers once as `OUTCOME_UNKNOWN` and is never rerun. | Use the cancelled context for close, classify uncertainty as `FAILED`, and recover `AUTHORIZED` as `FAILED`, one mutation at a time. |
| **AS-EXEC-08** `TestCompatibility_NoRecorderStillUsesCanonicalExecutor` | Stateless text/native submissions keep exact result and arguments, pass through the coordinator invocation counter, construct one in-memory action, create no file under the isolated profile root, and never consult the terminal close clock. | Executed separately: require a recorder for admission and consult `CloseClock` without a closer. |

AS-EXEC-01's analyzer loads production Go files and resolves selector and
function signatures. It has no package-wide exception for `internal/tool`.
Fixtures prove import aliases, type aliases, delayed calls, dead compliant
calls, and coordination moved into a helper. Its callsite half prevents a
second coordinator from surviving behind a compliant physical dispatch.

## Existing tests allowed to change

Only these approved edits change an existing test body:

| Test | Before | After |
|---|---|---|
| `TestTripwire_theOnlyPathToExecuteIsThisPackage` | Regex sweep with broad executor/tool directory exclusions. | Replaced by `TestExecutor_ASTRejectsEveryExternalDispatch` using AST and type information with exact allowed dispatch positions. |
| `TestRun_plainAndScopedRouting` | Calls `Run(ctx, name, scope, args)` and asserts the caller-supplied scope `{Brain: "b", Conversation: "c"}`. | Mints a plan, prepares, and submits the opaque request. Result and latency assertions stay unchanged; the scope assertion becomes the fixed brain plus the canonical conversation derived from channel `console` and id `c`: `{Brain: "b", Conversation: "console::c"}`. |
| `TestRun_unknownToolSentinel` | Calls the name-and-args `Run` door. | Mints a plan, prepares, and submits an absent operation; sentinel and `Has` assertions stay unchanged. |
| `TestRun_perToolTimeoutBoundsExecution` | Calls the name-and-args `Run` door. | Mints a plan, prepares, and submits the opaque request; timeout assertion stays unchanged. |
| `TestRun_toolErrorPassesThroughUnclassified` | Calls the name-and-args `Run` door. | Mints a plan, prepares, and submits the opaque request; partial output and original error assertions stay unchanged. |

New molds may reuse existing fixtures. Existing brain, app, tool, store, and
protocol expectations are not weakened or rewritten.

The compatibility-only addition
`TestLive_textLane_echoThroughPromptProtocol` wraps the real Ollama model as
`model.Model` so the textual protocol must parse and execute one distinctive
tool call. Its probing production mutation skips the parsed call instead of
submitting it; the tool counter remains zero and the test reddens. This test is
run before and after the refactor with its opt-in enabled; a skip is not
evidence.

## Compatibility dossier for blueprint section 24

The before and after dossier records each row separately, even though the full
`make quality` gate also executes them:

1. Orchestrator without tools: response, fallback, and persistence tests.
2. Text and native lanes: current protocol suites plus local Ollama smokes for
   a native tool call, a textual tool call through a model deliberately exposed
   only as `model.Model`, and degradation from a no-tool model.
3. Common kernel: coordinator counter and typed structural tripwire.
4. Shadow: zero dispatch and exact observation.
5. Unknown tool: exact not-found observation and finite audit label.
6. Strict config: existing unknown-key, type, and version corpus.
7. Private without cloud: current selected-model locality guards.
8. Cages and limits: complete `read_file`, `http_fetch`, and `webhook_call`
   packages and app wiring cases.
9. Post-DNS shield: resolved-address, redirect, and rebinding cases.
10. Model retries: attempt-count and both-lane tests.
11. Preflight: current zero-effect negative matrix.
12. Reload: failed candidate leaves the active app and recorded law intact.
13. Transcript: exact prompt, observation, role, and argument assertions.
14. Metrics, SSE, and feeds: current secret and argument absence canaries.
15. Skills: skill text cannot alter the capability decision.
16. Existing configs: valid and invalid config corpus produces the same result.

Baseline evidence is taken from the current `master` tip, not the older tag,
because that tip contains the v0.15.1 hardening listed above. The captured
baseline commands are `make quality` and the enabled local-Ollama live suite.
The live command sets both opt-in variables explicitly and names every expected
test. A skip or missing test is a failed evidence check even if `go test`
returns zero. The same commands run after implementation. Exact-output comparisons use
fixed clocks, ids, and fixtures where the protocol requires byte identity;
timings and randomized test temporary paths are not protocol data.

## Cross-scenarios

- CLI and desktop approval execution race on one request: only the successful
  claim obtains an invocation capability; every loser keeps its current named
  result.
- Reload between approval birth and execution: the app resolves the current
  cage and law once, and the claim judges that law inside its transaction.
- Caller cancellation after an external effect: terminal close and receipt
  read-back outlive the caller without re-executing the tool.
- Recovery and retention: Phase 0 changes neither recovery selection nor
  pruning, and historical schema-12 receipts remain verifiable.
- Stateless execution and configured storage share the same decision order;
  only the persistence adapters differ.

## Success criteria

- All eight Phase 0 molds first fail for the intended missing behavior.
- Every listed destructive mutation is applied separately and produces its
  intended red before being reversed.
- `go test -race` is green for every touched package and the full suite.
- Statement coverage is at least 85 percent in every touched package.
- `gofmt`, `goimports`, `golangci-lint`, `gosec`, and `govulncheck` are clean.
- `make quality` and the enabled real-model compatibility smokes are green.
- The action SQLite schema remains exactly 12 and no dependency changes.
- Every exported symbol added or changed has accurate godoc.
- The completed diff receives a separate hostile review before the canto is
  written and files are staged.

## Decisions folded in

- Phase 0 preserves stateless execution. Durable storage is not made a new
  prerequisite.
- No identity, intent, grant, budget, legacy-adoption, or revocation semantics
  move in this phase.
- The coordinator owns control flow; SQLite retains transaction ownership.
- The current v0.15.1 hardening is baseline behavior, not refactor latitude.

## Out of scope

New authentication, stricter identity evidence, intent enforcement, contract
signatures, budget consumption, revocation, approval UI changes, a schema
migration, retention changes, retry policy changes, new deadlines, and the
known post-commit approval-parameter restore gap remain outside Phase 0.

## `[NEEDS CLARIFICATION]`

None. The director resolved the five decisions in the source design, and only
the stateless-compatibility decision applies to this phase.
