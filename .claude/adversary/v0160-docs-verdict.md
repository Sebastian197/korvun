# Internal adversary — the ONE pass over the v0.16.0 public text

Object: the release notes, the README section, the SECURITY.md section and the
operator-CLI reference in EN and ES of the v0.16.0 train, over `be97cb1`.
Date: 2026-09-22. The text under the rule is the auditor's verdict VERBATIM, as
it was returned, persisted to disk BEFORE any cure began. The auditor writes no
files by charter; the delivery session wrote this one and changed nothing in it.

------------------------------------------------------------------------

VETO MANTENIDO

Object: the public text of the v0.16.0 train in `/Users/sebastianmorenosaavedra/Desktop/korvun-v0160.nosync` (branch `v0.16.0/docs-and-captures`, HEAD `be97cb1`). Nothing in that worktree was written, staged or committed by me; `git status --short` at the end is byte-identical to the start (4 modified files, 3 untracked paths). All mutations and executions ran on a copy at `/private/tmp/claude-501/-Users-sebastianmorenosaavedra-Desktop-korvun-nosync/dbc7d3ca-b129-478c-93e3-27f6702efb3f/scratchpad/copy`.

Three P1 and seven P2 stand. Sixteen of the factual claims I could check came back TRUE and are listed in the scope declaration, including the load-bearing one about grants and bindings, which I reproduced end to end with a compiled binary.

---

## P1-1 [FALSE] — "no production door creates that state today" is false, and the release's own recipe walks the operator into the state

**The sentence (`docs/releases/v0.16.0.md:148-149`):**
> "**A non-strict approval born after activation has no birth event**, so the profile then reads as corrupt. It fails closed — nothing starts — and no production door creates that state today, but none prevents it either."

**Why it is false.** The production door is a boot whose config carries no `authority` block. `internal/app/app.go:384` calls `PrepareStrictAuthority` only `if cfg.StrictAuthority()`; `internal/app/identity.go:402` is the only caller of `RequireAuthorityActivation`, which is the only writer of `s.authorityActivationDigest` (`internal/action/sqlite/authority_v2.go:405`). A non-strict boot therefore leaves that field empty, and `appendApprovalBirthTx` returns nil at `internal/action/sqlite/authority_v2.go:554-555`. Nothing refuses a non-strict boot over an activated store: `internal/config/config.go:687-689` returns nil when `c.Authority == nil`.

**REPRODUCTION (executed).** Attack file `internal/action/sqlite/auditor_attack_test.go` on my copy, two REAL `*Store` instances over one real SQLite file:

1. Build the phase-3 fixture and activate authority (`activateAuthorityFixture`), then `RequireAuthorityActivation(ctx, "", root)` — passes.
2. `Open(f.store.path)` a SECOND Store and wire it exactly as `app.Build` wires it WITHOUT `cfg.StrictAuthority()` — i.e. never call `RequireAuthorityActivation`. Assert `nonStrict.authorityActivationDigest == ""`.
3. Park one approval through the production door `CreateApprovalRequestAuthenticated`.
4. Close it. `Open` the file again — the operator has put the `authority` block back — and call `RequireAuthorityActivation(ctx, "", root)`.

```
$ go test ./internal/action/sqlite/ -run TestAuditor_NonStrictBootAfterActivationPoisonsTheProfile -v
=== RUN   TestAuditor_NonStrictBootAfterActivationPoisonsTheProfile
    auditor_attack_test.go:79: birth events for the approval born on the non-strict boot: 0
    auditor_attack_test.go:91: RequireAuthorityActivation on the re-armed boot = action/sqlite: authorization snapshot corrupt
--- PASS (0.06s)
```

**It is worse than the sentence admits, in two ways the text does not say.**
- The notes' OWN recipe opens the window. Step 5 is `korvun authority activate` (`docs/releases/v0.16.0.md:79`); the digest then goes in the config and the profile "is strict from the next boot". Between the activate and that restart the running server is still non-strict. Any request that parks in that window poisons the profile.
- "It fails closed — nothing starts" understates the outcome. `internal/app/app.go:384-389` makes `PrepareStrictAuthority`'s error boot-fatal: `app.Build` returns and the server does not start at all. And there is no remedy — `ActivateAuthority` refuses a second activation for the same profile (`internal/action/sqlite/authority_v2.go:295-301`, `exists != 0 → ErrAuthorizationSnapshotCorrupt`). The profile is permanently unable to boot strict.

The phase-3 canto files this ("a non-strict approval born after activation has no birth event, so the profile then reads as corrupt — fail-closed, declared, and worth a door", `docs/cantos/P3-F3-AUTHORITY-2026-09-21.md`). The canto does NOT claim "no production door creates that state". The notes added the safety assertion.

---

## P1-2 [FALSE] — the four refusal "names" quoted in three public surfaces do not exist anywhere in the product

**The sentences:**
- `docs/releases/v0.16.0.md:92` — "the tools above will refuse every start — by name, `resource_out_of_scope` or `authority_use_unresolved`, with nothing consumed."
- `docs/releases/v0.16.0.md:141-142` — "a non-strict approval decided later than that refuses with `identity_evidence_expired`."
- `SECURITY.md:226-227` — "a store that did not answer is `authority_store_busy` and keeps its cause"
- `website/docs/reference/operator-cli.md:266-267` — "the start refuses by name — `resource_out_of_scope`, or `authority_use_unresolved` when the arguments cannot be resolved at all."
- `website/i18n/es/.../operator-cli.md:279-280` — "el inicio se rechaza por su nombre: `resource_out_of_scope`, o `authority_use_unresolved` …"

**What the tree says.** Zero occurrences in any code file:

```
$ for n in resource_out_of_scope authority_use_unresolved authority_store_busy identity_evidence_expired; do
    echo "--- $n ---"; grep -rn "$n" --include='*.go' --include='*.ts' --include='*.tsx' . | grep -v node_modules
  done
--- resource_out_of_scope ---      (count: 0 files)
--- authority_use_unresolved ---   (count: 0 files)
--- authority_store_busy ---       (count: 0 files)
--- identity_evidence_expired ---  (count: 0 files)
```

The whole-tree search for `authority_use_unresolved` returns only five files: the two reference pages, the release notes, the design spec `docs/superpowers/specs/2026-09-21-p3-f3-authority.md` and the adversary verdict `.claude/adversary/p3-f3-diff-verdict-12e9d76.md`. No wire format, no log line, no UI string, no CLI output.

**What the product actually spells:**
- `internal/action/authority_v2.go:41` — `ErrResourceOutOfScope = errors.New("action: resource out of authority scope")`
- `internal/action/authority_v2.go:39` — `ErrAuthorityUseUnresolved = errors.New("action: authority use unresolved")`
- `internal/action/sqlite/authority_v2.go:36` — `ErrAuthorityStoreBusy = errors.New("action/sqlite: authority store busy")`
- `internal/identity/identity.go:24` — `ErrIdentityEvidenceExpired = errors.New("identity: authenticated ingress expired")`

Captured from a real start, my copy:
```
auditor_attack_test.go:112: memory_note under an intent scoped for read_file = action: authority use unresolved
auditor_attack_test.go:165: strict start on a profile with no v2 binding = action/sqlite: authority missing
```

**Why this is P1 and not cosmetic.** This repository already has a real snake_case refusal taxonomy on its public surfaces (`approval_unavailable`, `preview_args_mismatch`, `tombstone_action_mismatch`), so a snake_case token in the operator's MANUAL reads as a wire code. An operator who follows `website/docs/reference/operator-cli.md` will grep their logs for `resource_out_of_scope` and find nothing, in two languages. And the SAME release-notes file quotes the real spelling fifty lines later at `docs/releases/v0.16.0.md:171-172` — "refused by name — `action: resource out of authority scope`" — so the document contradicts itself on the spelling of one refusal. These four tokens are Go identifiers transliterated, i.e. derived from memory, not from captured output: a direct Letreros-law violation.

---

## P1-3 [FALSE] — "The Execution Trust Layer closes." is false and is contradicted by the same README twelve lines above

**The sentence (`README.md:204`):**
> "The Execution Trust Layer closes. Every tool call a model asks for now carries, in one transaction with the action itself:"

**What the tree says.**
- `README.md:192`, same file: "The first release of the **Execution Trust Layer** era (stage 1 of 10)."
- `docs/blueprints/2026-08-15-execution-trust-layer.md` defines Etapa 0 through Etapa 10 (`:1031`, `:1056`, `:1081`, `:1106`, `:1131`, `:1155`, `:1179`, `:1204`, `:1227`, `:1252`, `:1276`).
- Shipped so far, by this README's own sections: Etapa 1 = v0.11.0, Etapa 2 = v0.12.0, Etapa 3 = v0.13.0, Etapa 4 = v0.14.0, Etapa 5 = v0.15.0. The published release table says the same: `website/docs/releases/index.md:17` calls v0.15.0 "the fifth stage of the Execution Trust Layer".
- Unshipped: Etapa 6 "transacciones, idempotencia y compensation" (Transaction Coordinator, dependency graph, prepare/commit/abort, compensation registry, durable idempotency store, `OUTCOME_UNKNOWN` reconciliation), Etapa 7 Credential Broker, Etapa 8 "Consola y Builder completos", Etapa 9 "Gateway API, MCP y A2A", Etapa 10 multitenancy. Five of ten stages, and not small ones.

Nothing in the phase-3 spec (`docs/superpowers/specs/2026-09-21-p3-f3-authority.md`) or the canto claims the layer closes. This is an invention of the README section, in its first sentence, on the project's most-read public surface.

---

## P2-1 [FALSE] — "the seventeen required checks": eleven are required

**The sentence (`docs/releases/v0.16.0.md:158-159`):**
> "and the seventeen required checks green on the pull request of each phase, Windows included."

**Seventeen is right for checks that REPORTED.** `gh pr checks 50 | wc -l` = 17, all `pass`, on each of PRs 50, 51, 52, 53, 54.

**Eleven is the number REQUIRED.** The active ruleset:
```
$ gh api repos/Sebastian197/korvun/rules/branches/master
… "type":"required_status_checks","parameters":{… "required_status_checks":[
  {"context":"quality (macos-latest)"},{"context":"quality (ubuntu-latest)"},
  {"context":"quality (windows-latest)"},{"context":"sbom"},{"context":"Analyze (Go)"},
  {"context":"cross-compile (darwin, amd64)"},{"context":"cross-compile (darwin, arm64)"},
  {"context":"cross-compile (linux, amd64)"},{"context":"cross-compile (linux, arm64)"},
  {"context":"cross-compile (windows, amd64)"},{"context":"cross-compile (windows, arm64)"}]}
  … ruleset_id 20886440, enforcement "active"
```
Classic protection requires TEN (the same list minus `Analyze (Go)`):
```
$ gh api repos/Sebastian197/korvun/branches/master/protection
"contexts":["quality (ubuntu-latest)","quality (macos-latest)","quality (windows-latest)",
 "cross-compile (linux, amd64)","cross-compile (linux, arm64)","cross-compile (darwin, amd64)",
 "cross-compile (darwin, arm64)","cross-compile (windows, amd64)","cross-compile (windows, arm64)","sbom"]
```
Union = 11. The six that are NOT required anywhere: `CodeQL`, `builder`, `builder e2e`, `chrome`, `chrome e2e`, `website`. The sentence inflates gate strength by 55%, in the Verification section, on a tag that has no external pass.

---

## P2-2 [FALSE] — "How NOT to turn it on" names the wrong refusal for the exact scenario it warns about, and that scenario cannot exist

**The sentence (`docs/releases/v0.16.0.md:89-94`):**
> "**How NOT to turn it on.** Do not add the `authority` block to a profile whose intents were written before this release: under strict mode an intent that lists no allowed resource grants none, so the tools above will refuse every start — by name, `resource_out_of_scope` or `authority_use_unresolved`, with nothing consumed."

Two defects.

**(a) The scenario is impossible for the audience.** A profile "whose intents were written before this release" has v1 intents, because `korvun intent create-v2` ships for the first time in v0.16.0:
```
$ git show v0.15.1:internal/cli/intent.go | grep -c "create-v2"
0
```
A v1 intent is not a signed v2 contract and carries no execution binding, so the strict start dies at the FIRST gate — `bindingAuthorityTx` (`internal/action/sqlite/authority_v2.go:2276-2281`, `sql.ErrNoRows → ErrAuthorityMissing`), which runs before `activeIntentTx` and long before `validateAuthorityActualUse` at `:2132`.

**(b) REPRODUCTION (executed)** — the refusal such a profile actually gets:
1. Build the strict fixture with a scoped intent for `tool/http_fetch@1`.
2. `DELETE FROM execution_bindings` — the shape of a profile that never ran `korvun intent bind` because the verb did not exist for it.
3. `StartAuthorization` with `Arguments: "https://api.example.com/orders"`.
```
$ go test ./internal/action/sqlite/ -run TestAuditor_ProfileWithoutABindingRefusesWithADifferentName -v
    auditor_attack_test.go:165: strict start on a profile with no v2 binding = action/sqlite: authority missing
--- PASS
```
Neither `ErrResourceOutOfScope` nor `ErrAuthorityUseUnresolved`. The advice sends an operator hunting for the wrong two names.

("with nothing consumed" is TRUE and I confirmed it: `resolveAuthorityTx` runs before the debit loop in `StartAuthorization` (`internal/action/sqlite/authority_v2.go:1890-1901`) under a deferred `tx.Rollback()`; my scoped-intent run recorded `budget_debits=0 authorization_starts=0`.)

---

## P2-3 [TOO STRONG] — the CLOSED WORLD is not limited to the three named tools; a scoped intent also kills `memory_note`

**The sentence (`docs/releases/v0.16.0.md:53-54`, mirrored at `website/docs/reference/operator-cli.md:262-264` and its ES twin at `:275-278`):**
> "With that cured, under a **strict** profile those three tools are CLOSED WORLD:"

**What the tree says.** `validateAuthorityActualUse` (`internal/action/sqlite/authority_v2.go:2198-2210`) computes `restricted` from the INTENT's `AllowedResources`/`DeniedResources`/`DataScope`/`OutputDestinations` plus every grant, not from the operation. If `restricted` and the operation has no registered analyzer, it returns `ErrAuthorityUseUnresolved`. Only three analyzers are registered (`internal/action/authority_v2.go:674-680`). `memory_note` is `write_reversible` (`internal/tool/effects.go:36-40`), so it is NOT `EffectPure` and goes through `StartAuthorization` (`internal/action/executor/executor.go:686`).

**REPRODUCTION (executed), with its control:**
```
$ go test ./internal/action/sqlite/ -run 'TestAuditor_(ScopedIntentAlsoClosesMemoryNote|UnscopedIntentLetsMemoryNoteStart)' -v
=== RUN   TestAuditor_ScopedIntentAlsoClosesMemoryNote
    memory_note under an intent scoped for read_file = action: authority use unresolved
    budget_debits=0 authorization_starts=0
--- PASS
=== RUN   TestAuditor_UnscopedIntentLetsMemoryNoteStart
    memory_note under an UNSCOPED intent = action "act3_1_cc62930f4602acda6ce7a3d4cb7110ae" err <nil>
--- PASS
```
The control is the point: the refusal comes from the resource scope the same document tells the operator to write ("Write the intent's scope first", `:93-94`), not from `memory_note`. An operator who follows the advice to enable `read_file` silently loses `memory_note`, and no public sentence warns them. This is the section the notes call the one "an operator must read before turning strict mode on" (`:7-8`).

---

## P2-4 [MISSING] — the published five-command recipe is not executable as written

**The block (`docs/releases/v0.16.0.md:73-80`, `website/docs/reference/operator-cli.md:240-248`, ES `:253-261`):**
```
korvun intent create-v2   --config korvun.json --file intent.json
korvun intent activate-v2 --config korvun.json <intent_id> 1
korvun authority admin-issue --config korvun.json --file grant.json --reason "…"
korvun intent bind        --config korvun.json --actor principal_brain_<name> --channel <channel> <intent_id> 1
korvun authority activate --config korvun.json --profile <profile_id> --reason "…"
```
The verbs and flags all EXIST and the order works — I verified each against `internal/cli/intent.go:38-78` and `internal/cli/authority.go:17-40`, and ran the whole sequence with a binary built from the copy. It still cannot be followed, for three reasons, and I captured each.

**(a) `grant.json` requires `intent_digest`, and no CLI verb prints it.** `authorityGrantWireV2` (`internal/action/authority_v2.go:144`) has `IntentDigest string json:"intent_digest"`, and `Validate` (`:167-171`) refuses an empty one. Executed:
```
$ korvun authority admin-issue --config korvun.json --file grant.json --reason "why this authority exists"
korvun authority admin-issue: action: intent evidence corrupt
EXIT=1
```
(that is with a placeholder digest — note also the error blames the STORE for the operator's own file). The three doors that could give the digest do not:
```
$ korvun intent show --config korvun.json int_pedidos
korvun intent show: action/sqlite: action not found: intent "int_pedidos"     EXIT=1
$ korvun intent list --config korvun.json
no intents stored                                                            EXIT=0
$ korvun intent verify-v2 --config korvun.json int_pedidos 1
intent int_pedidos version 1: OK                                             EXIT=0
```
The recipe only completes after reading the digest out of SQLite by hand:
```
$ sqlite3 korvun.db "SELECT intent_id,version,digest FROM intent_versions;"
int_pedidos|1|sha256:1a0ac3899691c1755ffbbda19d1516fd8b1986bff45e5615d79af8ffce2784c3
$ korvun authority admin-issue … ; korvun intent bind … ; korvun authority activate …
authority grant grant_pedidos_root issued
binding bind_act_ea09be47fe1a9fd7c98f1fc75a0ad420 -> int_pedidos version 1 ACTIVE
authority activated for profile_ops
activation_digest: sha256:bca68ddff139df18c72b35f0cb634f52d9bb91ae63d5305e6e30139ff6246554
```
No public page mentions that step.

**(b) The shape of `intent.json` and `grant.json` is documented nowhere.** `grep -rln "create-v2\|IntentContractV2" website/docs docs/CONFIGURATION.md docs/*.md` returns exactly one file: the new section itself. Both parsers use `DisallowUnknownFields` plus `rejectDuplicateJSONKeys` (`internal/action/intent_v2.go:130-134`, `internal/action/authority_v2.go:300-305`), so guessing fails. Every OTHER section of the same reference page gives complete, runnable flag lists (`korvun intent create --purpose … --operations …`, `website/docs/reference/operator-cli.md:24-29`). This section breaks that contract.

**(c) The `authority` config block is documented nowhere, and its precondition is not stated.** `grep -n "authority" docs/CONFIGURATION.md` returns nothing. `internal/config/config.go:694-696` refuses strict mode without the `storage` block — none of the three texts say so, and none says the block is top-level.

---

## P2-5 [TOO STRONG] — the README's opening claim is unconditional and its own paragraph contradicts it 28 lines later

**The sentences (`README.md:204-220`):**
> "Every tool call a model asks for now carries, in one transaction with the action itself: … **a signed purpose.** … **a signed authority.**"

vs `README.md:232-233`:
> "It is OFF by default: a profile without the `authority` block behaves exactly as before."

Both cannot be true. Under the default profile no signed v2 intent is consulted and no grant chain is verified: `resolveAuthorityTx` is reached only from `StartAuthorization`, which `internal/action/executor/executor.go:686` gates on `e.config.StrictAuthority`, wired from `cfg.StrictAuthority()` at `internal/app/app.go:1115-1116`.

Even the first bullet ("a verified name") is wider than its wire under non-strict: `internal/action/executor/executor.go:659-671` has an explicit identity FALLBACK — when `bindIdentity` does not resolve, `result.IdentityFallback = true` and the attempt is recorded through plain `RecordAttempt(ctx, canonical, outcome, rule, state)` with no evidence.

A reader who reads the bold bullets — which is what bullets are for — takes away a guarantee the default build does not give.

(I checked and did NOT find a defect in the adjacent absolute "Every ingress door mints an opaque authenticated capability…". The console mint is conditional in shape, `internal/controlapi/console.go:153-165`, but `phase1IdentityRuntime` creates the console issuer unconditionally and boot-fatally at `internal/app/identity.go:130-132`, so the conditional arm is unreachable through `app.Build`. Reported as verified-true, not as a finding.)

---

## P2-6 [MISSING] — the assurance paragraph counts five verdicts without saying one is an admitted reconstruction, and without saying no adversary read the cures that ship

**The sentence (`docs/releases/v0.16.0.md:10-15`), mirrored at `SECURITY.md:253-256`:**
> "**No external pass ran against this tag.** … What exists instead is the copilot's review of the critical diff, and five internal adversary verdicts recorded in the markers of this history — phase 0, the loopback test binds, phase 2, phase 1 and phase 3. Their scope is stated in each marker; none of them is an external review and none of them is acceptance."

Placement and the "No external pass" headline are GOOD — third paragraph, bold, unambiguous, and repeated in SECURITY.md. Two things the paragraph withholds.

**(a) The phase-1 marker is not a recorded verdict.** `git show f5762b8:.claude/adversary/last-verdict.md` says, in its own words:
> "THIS VERDICT IS A RECONSTRUCTION, and says so. The internal adversary ran ONE pass over the complete diff … and its verdict was NEVER WRITTEN TO DISK. What survives of it is what each cure carries in its own comments … This marker therefore rebuilds the verdict from the TREE, not from the auditor's text … P3-5 and P3-7 left no trace in the tree and nothing is guessed about them."

"five internal adversary verdicts recorded in the markers" counts a reconstruction as a recorded verdict. "Their scope is stated in each marker" delegates the disclosure to a file the public reader will not open.

**(b) No adversary pass has read the code that ships.** `git show be97cb1:.claude/adversary/last-verdict.md`:
> "THE ADVERSARY'S ONLY VERDICT OVER THIS TRAIN WAS «VETO MANTENIDO»: one P1 and five P2, over the delivery commit 12e9d76. The first line of this file, «VETO LEVANTADO», is the push gate's token … NOT because the adversary lifted anything. THE ADVERSARY HAS NOT READ THE CURES."

Every one of the five markers records a `VETO MANTENIDO`: phase 0 two P2 + three P3; loopback four P2 + seven P3; phase 2 two P1 + eleven P2; phase 1 twelve findings including two P1; phase 3 one P1 + five P2. All cured after the pass, none re-read. "none of them is acceptance" is true but does not say this. Since the external gate has not run either, the honest statement is that NO adversarial reading — internal or external — has covered the tree being tagged. That sentence is missing from both surfaces.

---

## P2-7 [MISSING] — the capture spec writes into a TRACKED repository path on every e2e run, CI included

`cmd/korvun-desktop/frontend/e2e/authority-captures.spec.ts:20` sets `const OUT = '../../../docs/assets/captures/v0.16.0'` and line 75 writes `authority-block-${theme}.png` there. `cmd/korvun-desktop/frontend/playwright.config.ts` sets `testDir: './e2e'` with no `testMatch`, so the spec is collected by every run — including the CI check "chrome e2e (Playwright · SP4 proxy + real no-network core)", which passed on all five phase PRs.

Every other screenshot-writing spec in that directory writes through the `SHOT()` helper into `design-drafts/` (`e2e/util.ts:23`), which is gitignored (`.gitignore:114`). `docs/assets` is not:
```
$ grep -n "docs/assets\|captures" .gitignore
(no output)
```
So a developer or a CI job running `npx playwright test` silently rewrites the two PNGs the README publishes, with that machine's fonts and rendering. Nothing in the spec header, the README or the notes says so. The spec's own header is otherwise exemplary — it declares "NOT a guarantee test", it declares that it writes files, and it explains the theme mechanism — which makes the omission of "and it writes them into the tracked tree" the gap.

**On the capture itself I found no defect.** The spec's provenance claim is TRUE: `cmd/korvun-desktop/e2e-harness/main.go:782-803` accepts only `intent_id`, `intent_purpose`, `budget` and `spend_after_park` from the test, comments that "Who requested, who acts and through which chain are the store's to establish, not the test's to dictate", and parks through the production door `store.ParkAuthorization` (`:911-1070`). The picture's `principal_channel_telegram` (QUIÉN PIDIÓ) and `principal_brain_asistente → principal_brain_operaciones` (CADENA) are store-established, and `máximo 3 inicios` matches the `total: 3, spend_after_park: 0` the spec parks. The screen binds `authority.requester_principal_id`, `authority.intent_id`, `authority.principal_chain` and `authority.budget.remaining` (`cmd/korvun-desktop/frontend/src/views/Approvals.tsx:1122-1144`), and the README's "budget that remained when the request was parked — a signed snapshot, not a live meter" agrees with the screen's own heading "PRESUPUESTO ANTES DE ESTE INTENTO". The README does not claim the packaged app. Two smaller notes: `authority-block-light.png` is shipped and referenced by nothing (`grep -rn "authority-block-light"` → no hits), and the new capture lives in a new `docs/assets/captures/` tree while the established family — including `approvals-detail-{dark,light}.png` — lives in `docs/assets/readme/`.

---

## P3-1 [MISSING] — the v0.15.2 filings: five of eleven are enumerated behind a colon

`docs/releases/v0.16.0.md:98-103` says "**The v0.15.2 filings stay open**, all of them, and the v0.15.1 Known issues with them:" and then lists five. "all of them" is TRUE — I verified `internal/action/sqlite/approvals.go`, `approvals_v15.go`, `internal/cli/approvals.go`, `receipt.go`, `ledger.go`, `escape.go` have no commit between `v0.15.1` and HEAD (`git log --oneline v0.15.1..HEAD -- …` returns only the four piece-3 feature commits), and `GetApprovalByAction` still returns the bare driver error at `internal/action/sqlite/approvals.go:1027-1029`. But `docs/HANDOFF.md:84-277` carries ELEVEN filings, and the colon reads as an enumeration. The six absent: `GetApprovalByAction` returning its read error with no class; `korvun approvals list` hiding a row whose status was rewritten outside the five known values (corruption masquerading as absence — this repository's own named failure class); the pending list naming the skipped row with an empty id; the receipt shape living only in TypeScript; a reject with an empty `receipt_id` printing a bare "Recibo "; a future effect class reading as corrupt. The one about the approvals SCREEN is the same screen this release showcases.

## P3-2 [MISSING] — the CLOSED WORLD bullet states only the restrictive half of the query rule

`docs/releases/v0.16.0.md:63` — "a resource scoped by its query includes only that query." TRUE (`internal/action/authority_v2.go:756-761`). The code comment on the very next line states the half the notes omit: "A resource with no query says nothing about it, and includes any." That is the half an operator plans a scope with. Executed, unit level, over the functions the strict door calls:
```
$ go test ./internal/action/ -run TestAuditor_ScopeWithoutQueryAdmitsAnyQuery -v
  ALLOWED: use={Resources:[{Kind:url ID:https://api.example.com/orders?tenant=999&drop=all}]
           Data:[] Destinations:[api.example.com]} under scope https://api.example.com/orders
  under the query-scoped resource ?tenant=1 the same use = action: resource out of authority scope
--- PASS
```
In a section headed CLOSED WORLD, publishing only the closing half is the wrong half to publish.

## P3-3 [FALSE] — the reference pages' `authority` verb list is incomplete and misattributes `--reason`

`website/docs/reference/operator-cli.md:254-257` and ES `:267-270`: "`authority` also has `issue` …, `delegate` and `admin-delegate`, `revoke`, and `import-v1`. The administrative forms require `--reason`…". `admin-revoke` exists and is missing from both lists (`internal/cli/authority.go:24`, `:32-33`). And ordinary `revoke` requires `--reason` too — `internal/cli/authority.go:179` refuses on `*reason == ""` for both forms — so "the administrative forms require `--reason`" implies a distinction the code does not make. Also: "`issue` (ordinary, where the intent's own owner is the actor)" is true as a rule (`internal/action/sqlite/authority_v2.go:755`) but unusable as written, because from the CLI the actor is always `principal_local_operator`, a principal id that appears nowhere in the public docs:
```
$ korvun authority issue --config korvun.json --file grant2.json
korvun authority issue: action/sqlite: authority issuer mismatch     EXIT=1
$ sqlite3 korvun.db "SELECT DISTINCT actor_principal_id FROM grant_events;"
principal_local_operator
```

## P3-4 [MISSING] — an upgrading operator cannot run step 1 until the server has booted once

The notes say "Action schema 12 → 15 across the four phases" (`:45`) — VERIFIED (`git show v0.15.1:internal/action/sqlite/store.go` → `schemaVersionCurrent = 12`; current `internal/action/sqlite/store.go:127` → `15`). But `OpenOperator` (`internal/action/sqlite/store.go:1364-1377`) refuses any existing store not already at 15, and the recipe starts with a CLI act. Executed against a store pinned at 12:
```
$ korvun intent create-v2 --config korvun.json --file intent.json
korvun intent create-v2: action/sqlite: store "…/korvun.db" is at schema v12, this binary writes v15 —
an operator act never migrates an existing store; run the server boot to lift the schema     EXIT=1
```
The error is self-explaining, so this is soft — but "How to turn it on, in this order" presents itself as complete and is not.

## P3-5 [MISSING] — the manual pass has no capture in the tree

`docs/releases/v0.16.0.md:128-129` — "Observed in this release's manual pass: the parked start ran under `cfg_25fc…` while `grant_pedidos_root` sat ACTIVE beside it" — and `:167-173` (Ollama `llama3.2`, three signed debits, an out-of-scope URL refused). `grep -rl "cfg_25fc"` and `grep -rl "grant_pedidos_root"` return `docs/releases/v0.16.0.md` and nothing else; `docs/superpowers/specs/evidence/` has no `v0.16.0` directory. I cannot verify these sentences from the tree; by the Letreros law the capture should be filed with them. I did independently confirm the FACT they report — see below.

## P3-6 [STYLE] — SECURITY.md "exactly one class" reads as self-contradictory; ES page keeps English example strings

`SECURITY.md:229-232` — "A failed READ of authority evidence has exactly one class: a store that did not answer is `authority_store_busy`…; only an absent or unconvertible row is corruption." The godoc it paraphrases (`internal/action/sqlite/authority_v2.go:695-705`) means "one class per failure"; the public sentence reads as "one class" followed by three. And the ES reference page reproduces the code block verbatim, leaving `--reason "why this authority exists"` and `--reason "why this profile goes strict"` in English (`website/i18n/es/.../operator-cli.md:255-261`).

---

## What came back TRUE — verified, not assumed

- **The load-bearing Known issue is exactly right.** "No public door ties a signed grant to an execution binding" (`docs/releases/v0.16.0.md:122-133`, README `:235-237`, both reference pages). `internal/cli/intent.go:355` builds the `ExecutionBinding` with no `GrantID`/`GrantVersion`/`GrantDigest`; `grep -rn "GrantID:" --include=*.go . | grep -v _test` shows the only non-test writer into a binding is the e2e harness (`cmd/korvun-desktop/e2e-harness/main.go:1049`). `internal/action/sqlite/authority_v2.go:2060-2079` then takes the config-clause path when `binding.GrantID == ""`. **Reproduced end to end with the compiled binary**, after running the published recipe in full:
  ```
  $ sqlite3 -header korvun.db "SELECT binding_id,actor_principal_id,channel,intent_id,grant_id,grant_version,grant_digest,status FROM execution_bindings;"
  bind_act_ea09…|principal_brain_ops|telegram|int_pedidos||||ACTIVE
  $ sqlite3 -header korvun.db "SELECT grant_id,active_version,status FROM grant_heads;"
  grant_pedidos_root|1|1|…|ACTIVE
  ```
  Empty `grant_id`, ACTIVE unused grant. Both the notes and the reference pages state this at the right strength, including "Nothing starts outside the intent's scope either way" — `validateAuthorityActualUse` (`:2216`) checks the intent's terms on both paths.
- **Schema 12 → 15**, five markers, their commit list and order (`d2a36fd`, `55537f3`, `551f977`, `f5762b8`, `be97cb1`, matching "phase 0, the loopback test binds, phase 2, phase 1 and phase 3").
- **Every number in Verification** against `docs/cantos/P3-F3-AUTHORITY-2026-09-21.md`: 97 + 41 mutations, 7 pre-cure reproductions and the two-binary reproduction of F1 (`:361-365`, `:408` and the phase-3 marker); 42 pruned packages with govulncheck v1.7.0 (`:403`); 44 files / 503 tests (`:405`); ~300 s locally under `-race` and 2.5–4× slower on Windows against a 30-minute ceiling (`:297-298`) — corroborated by PR 54's `quality (windows-latest)` at 28m30s.
- **The four cantos exist** with the cited prefixes (`docs/cantos/P3-F0-…`, `P3-F1-…`, `P3-F2-…`, `P3-F3-…`).
- **The config shape** `"authority": {"mode":"strict","activation_digest":"sha256:…"}` — top-level, `internal/config/config.go:77` and `:162-165`.
- **Closed-world mechanics.** Analyzers parse their tools' real grammar (`internal/action/authority_v2.go:644-681`); a relative `read_file` path is `ErrAuthorityUseUnresolved` (`:647`); an unclean or non-canonically-encoded URL path is refused, not normalized (`:704-715`); an intent with no `allowed_resources` grants none (`:768-770` with `resourcesIncluded` at `:561-579`); "nothing consumed" holds.
- **The AUTORIDAD block's signed-snapshot claims** in README and SECURITY.md: `approvalAuthoritySnapshotTx` verifies the signature and compares every stored column against the signed bytes (`internal/action/sqlite/approvals_v15.go:450-483`), so "a writer with the database and without the profile key cannot move it without the document refusing itself" holds.
- **Known issue 3** (linear re-verification): `verifyAuthorityActivationTx` walks the whole birth chain and every `approvals` row on each strict start (`internal/action/sqlite/authority_v2.go:408-546`, called at `:2036-2042`).
- **Known issue 2** (the two resume windows) matches the F6 cure commit `2d37273` verbatim, and the five-minute ingress TTL is real (`internal/app/identity.go:122`).
- **SECURITY.md's self-limiting claims**: `app.Build` under a strict config is exercised by no test (71 `Build(` calls in `internal/app` tests, none on a line carrying `Authority`; only `PrepareStrictAuthority` is covered, `internal/app/authority_prepare_test.go:26` and `authority_strict_doors_test.go:366`).
- **EN/ES parity is clean.** I compared the two sections clause by clause, including the "limit to know before you plan with it" / "Un límite que conviene saber antes de planear con esto" paragraph. The Spanish softens nothing; where the English is wrong the Spanish is wrong identically. The only delta is the untranslated example strings (P3-6).
- **"No external pass ran against this tag"** is stated plainly, in bold, in the third paragraph of the notes and again at `SECURITY.md:253-256`. Correct placement.

---

## Scope declaration

**READ, in full:** `docs/releases/v0.16.0.md` (176 lines); the four diffs (`git diff README.md SECURITY.md website/docs/reference/operator-cli.md website/i18n/es/.../operator-cli.md`); `cmd/korvun-desktop/frontend/e2e/authority-captures.spec.ts`; both PNGs, rendered; the five marker verdicts via `git show <sha>:.claude/adversary/last-verdict.md`; `docs/cantos/P3-F3-AUTHORITY-2026-09-21.md`; `docs/HANDOFF.md:1-330`; the blueprint stage list; and the production source cited above — `internal/action/authority_v2.go`, `internal/action/sqlite/authority_v2.go`, `approvals_v15.go`, `approvals.go`, `store.go`, `intent_v2.go`, `internal/action/executor/executor.go`, `internal/action/intent_v2.go`, `internal/cli/intent.go`, `internal/cli/authority.go`, `internal/config/config.go`, `internal/app/app.go`, `internal/app/identity.go`, `internal/controlapi/console.go`, `internal/tool/effects.go`, `cmd/korvun-desktop/e2e-harness/main.go`, `cmd/korvun-desktop/frontend/src/views/Approvals.tsx`, `playwright.config.ts`.

**EXECUTED:** five new attack tests on a copy (`internal/action/sqlite/auditor_attack_test.go`, `internal/action/auditor_query_test.go`), all passing; a `korvun` binary built from the copy and driven through the complete published recipe against a real profile and a real SQLite store, plus a schema-12 store; `sqlite3` inspection of the resulting `execution_bindings`, `grant_heads`, `grant_versions` and `intent_versions`; `gh pr checks 50 51 52 53 54`; `gh api repos/Sebastian197/korvun/branches/master/protection` and `.../rules/branches/master`; `git show v0.15.1:…` for the schema constant and the absence of `create-v2`; whole-tree greps for the four refusal names, the capture references and the manual-pass identifiers.

**COULD NOT VERIFY:**
- The manual pass with the real Ollama model (`docs/releases/v0.16.0.md:167-173`) and the `cfg_25fc…` observation (`:128-129`). No capture exists in the tree; I have no model endpoint. I did independently confirm the FACT the observation reports (the config-clause path and the unused ACTIVE grant) with the compiled binary — see "What came back TRUE".
- "`make quality` with `-race` green over the tagged tree" (`:156`). The tag does not exist, and the docs commit adds a TypeScript file (`authority-captures.spec.ts`) that the frontend lane lints. I did not run the eleven-minute gate. Flagged as a forward-looking assertion, not scored.
- The two UNVERIFIED predictions the notes declare (`:109-113`) — I did not attempt them; the notes label them UNVERIFIED, which is honest.
- Whether the 84.9% `cli` coverage the canto declares ("one tenth UNDER the floor") is compatible with `make quality` going green. The public texts make no coverage claim, so it is out of scope for this pass.

**UNEXAMINED:** the rest of README.md and SECURITY.md outside the two new sections; `website/docs/releases/index.md` (which will need a v0.16.0 row and does not have one); the v0.16.0 entry in any changelog; the release-checklist / `releaseFacts` guard for this version; the desktop frontend beyond `Approvals.tsx`'s AUTORIDAD block; the builder and website surfaces.
