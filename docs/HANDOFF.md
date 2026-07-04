# HANDOFF — Korvun

> **Read this at the start of every session.** Restores the project
> context, the current state, and the next thing to do without having
> to re-derive it from `git log`. CLAUDE.md is the operating rules;
> HANDOFF is the running state of the work.

---

## Objectives

### Project (one line)

Korvun is a single Go binary acting as messaging gateway + multi-model
router + multi-brain orchestrator, with a configurable dispatch
policy engine (privacy / cost / consensus) driven by a no-code
visual builder. Self-hosted, cross-platform, same binary from a
Raspberry Pi to the cloud.

### Stage 4 (closed)

Pin the abstraction every reasoning component in Korvun talks
through (`model.Model`) and ship the mechanism every multi-provider
component will eventually use (`fanout.Coordinator`). Validate the
abstraction against two providers of materially different shape
(local-no-auth Ollama + cloud-bearer-token-quota Groq) so a single
contract carries both. Keep the policy of "what to do with the
outcomes" strictly out of the mechanism layer — that's Stages 5–6.

---

## Current state (as of session close, 2026-06-28)

> **master is at `a8075f9`** (Stage 15 packaging machinery, direct to master),
> `make quality` green with `-race` + cross-compile ×6 `CGO_ENABLED=0`. Stages
> closed: **0–9, 11, 12, 13, 14 Phase 1 (foundation), and 15 (packaging
> machinery)**. `go.mod` stays at **3 direct deps** (`go-telegram/bot` +
> `modernc.org/sqlite` + `prometheus/client_golang`) — Stage 15 added none
> (GoReleaser is build-time). The binary boots, serves Telegram live, remembers
> across restarts, is observable (`/metrics` + `/healthz`), uses tools
> (`AgentBrain`), is introspectable read-only (`/api/brains` + `/api/channels`),
> **streams its message-pipeline lifecycle live** (`/api/events` SSE + an embedded
> `/ui`), reports `--version`, and has a **validated GoReleaser release pipeline**
> (tag → ×6 binaries + checksums + archives + SBOM; no real tag pushed yet).
>
> **Stage 14 Phase 1 (the builder's FOUNDATION, not the builder) is CLOSED**
> (`docs/stages/STAGE-14.md`, ADR-0023 + ADR-0024), split by blast radius:
> - **Phase 1a (event bus + router hook, ADR-0023, `464f8c2`)** — `internal/bus`,
>   an in-process best-effort non-blocking pub/sub (slow-subscriber drop+counter,
>   panic-safe, `-race`-validated under `brainWorkers>1`) + one additive nil-safe
>   `WithEventPublisher` router hook (MessageReceived on enqueue, ReplySent after
>   Send). Concurrency `/review` APPROVED.
> - **Phase 1b (SSE live-view + UI, ADR-0024, `4f36447`)** — `internal/liveview`:
>   `GET /api/events` (stdlib `http.Flusher` SSE, the bus's first real subscriber,
>   validating it end-to-end) + a `go:embed` vanilla read-only `/ui`. The bus is
>   WOKEN in `app`: real `InMemoryBus` built only when observability is on (its
>   only consumer rides the admin server), `WithEventPublisher` wakes the hook,
>   `onRouterError` publishes `MessageDropped`/`HandleFailed`, `bus.DroppedCount`
>   + `liveview.DroppedCount` exposed as pull metrics. **F2 teardown resolved at
>   the root by DECOUPLING** (the bus Handler writes only to an in-process
>   per-connection buffer, never the ResponseWriter, which only the serve loop
>   touches — so a Handler firing after unsubscribe cannot write a torn-down conn).
>   **Frames SECRET-FREE by construction** (the `frame` type has no field that can
>   carry content/Meta/Err), test-asserted. Copilot review APPROVED.
>
> The Stage 10 bus deferral is now closed correctly (built when, and only when, a
> consumer arrived to validate it).
>
> **Stage 14 Phase 2 (the builder proper) DEFERRED.** The builder's mutation wants
> a real consumer (a non-author operator, which only exists once Korvun is
> installable); packaging unlocked the value already built first. Phase 2, when it
> comes, is **mutation** (add-only or reload-and-rebuild, **NEVER granular live
> editing** — the router registry is boot-time and has **no per-brain cancel**, so
> live granular mutation is a router concurrency/lifecycle change, not a handler) +
> **AUTH** (the trigger of mutation; read-only is what keeps loopback-no-auth valid
> today) + the edit UI + the visual canvas.
>
> **Stage 15 (packaging) is CLOSED** (`docs/stages/STAGE-15.md`, ADR-0025,
> `a8075f9`) — the release **machinery**, validated with `--snapshot` but with **NO
> real release tag pushed yet** (a conscious pending decision). Approach A
> (GoReleaser, build-time, never in `go.mod`). What landed: a `.goreleaser.yaml`
> (×6 `CGO_ENABLED=0` binaries + SHA256 `checksums.txt` + `.tar.gz`/`.zip` archives
> with binary+LICENSE+README + git Conventional-Commits changelog + per-release SBOM
> via Syft + ldflags `-X main.version=v{{.Version}}`); a tag-triggered
> `release.yml` (pinned SHAs: goreleaser `ec59f47`/v7.0.0, syft `e22c389`/v0.24.0;
> tags pushed by hand); `--version` via a TDD'd `internal/buildinfo.Format` helper
> (100%) — the ONLY production-code touch, short-circuiting before any config load;
> example `configs/edge.json` (Pi, local, private, storage on) + `configs/cloud.json`
> (groq+ollama fan-out) — files, NOT a runtime profile system; `docs/packaging/`
> (INSTALL.md + a basic un-hardened `korvun.service`). `/review`: 0 P1, 1 P2 (the
> `{{.Version}}` v-prefix strip) found and fixed. SBOM = describe (15); signing/
> provenance/SLSA = prove (16).
>
> **Honest scope (do not oversell):** Stage 15 does NOT make Korvun installable by
> anyone — that is Stage 16's public flip. It delivers the author installing
> versioned artifacts cross-machine (`gh release download`) + the proven machinery
> so the flip is one line.
>
> **Stage 16 (hardening + public release) is IN PROGRESS — Phase A + Phase B DONE**
> (`/office-hours` + `/plan-eng-review`, copilot-approved), pinned by **ADR-0026
> (status: accepted, `a68a0b8`)**. Phasing is **Order 1**, ordered by reversibility.
>
> **Phase A (pre-flip machinery + gate) — DONE** (`master` through `23e6b3c`,
> checklist report `1bdd8db`): keyless cosign signing of `checksums.txt`
> (GoReleaser `signs:`; installer SHA-pinned `cosign-installer@v4.1.2`, cosign
> binary pinned `v2.6.3` — cosign v3's `--new-bundle-format` drops the classic
> `--output-signature`/`--output-certificate` GoReleaser relies on) + SLSA
> provenance (`attest-build-provenance@v4.1.1`, SHA-pinned); hardened systemd unit
> (static `User=korvun` + `StateDirectory`, NOT DynamicUser); consolidated
> developer docs (`QUICKSTART.md`, `CONFIGURATION.md`, README index); 9 stale
> remote branches deleted (open Dependabot PR #4 left, then closed by decision);
> pre-flip checklist run (`docs/notes/stage-16-preflight-checklist.md`) — gitleaks
> + trufflehog clean over all history, no secrets in Actions logs, `.gitignore`
> clean. The three human gate items were resolved by Sebastián: the parked
> operating-rules file COMMITTED (`1bdd8db`), the author email ACCEPTED, panel
> settings done. **A `workflow_dispatch` `--snapshot` dry-run in CI proved the
> keyless cosign OIDC signing GREEN** (real Fulcio/Rekor, `tlog` entries) before
> any tag; provenance is gated to real tag pushes (GitHub cannot persist
> attestations for user-owned PRIVATE repos).
>
> **Phase B (THE FLIP) — DONE 2026-07-04 (Sebastián's act).**
> `github.com/Sebastian197/korvun` is now **PUBLIC**. Private vulnerability
> reporting + Dependabot alerts ON; Dependabot security updates deliberately OFF.
> `scorecard.yml` automatic triggers re-enabled (`8a7becf`, `ci: re-enable
> scorecard on public repo`). All six README badges resolve (HTTP 200); Quality
> Gate GREEN on the public repo. First Scorecard run: aggregate 5.1/10.
>
> **Scorecard findings — DECIDED (copilot-reviewed). TWO fixed, the rest are
> conscious decisions, NOT pending work:**
> - **FIXED — Token-Permissions (0/High):** `release.yml` top-level dropped to
>   `contents: read`; `contents`/`id-token`/`attestations: write` moved to the one
>   `goreleaser` job that needs them (`cdebe0b`). Least privilege, behavior
>   unchanged.
> - **FIXED — SAST / CodeQL (0):** re-added `.github/workflows/codeql.yml` for Go
>   (push/PR/weekly; `github/codeql-action@v4.36.3` SHA-pinned `54f647b7…`,
>   verified at source) now that code scanning is free on the public repo
>   (`c893638`).
> - **Branch-Protection (0/High) — CONSCIOUS SKIP:** single-maintainer repo;
>   revisit if collaborators join. Not a defect.
> - **Code-Review / Contributors (0) — STRUCTURAL:** single maintainer, direct to
>   master; no PR-review chain to score.
> - **Maintained (0) — TIME:** repo <90 days old; improves with age/activity.
> - **Signed-Releases / CI-Tests (-1) — AUTO-POPULATE:** Signed-Releases fills at
>   `v0.1.0` (cosign keyless already proven); CI-Tests fills as PRs appear.
> - **Pinned-Dependencies (6/Medium) — CONSCIOUS:** `actions/checkout` +
>   `actions/setup-go` pinned by `@v6` tag (GitHub-owned, repo convention). Left
>   as-is; not prioritized.
> - **PASSED (10):** Security-Policy, License, Vulnerabilities, Dangerous-Workflow,
>   Binary-Artifacts, Packaging, Dependency-Update-Tool.
>
> **Phase C (first public release) — PENDING, Sebastián's act.** Push `v0.1.0`
> (not before the Scorecard-findings decision). The tag fires the signed
> `release.yml` (now with real provenance, repo public) → first public, signed
> release + SBOM. Do NOT push the tag autonomously.
>
> Phasing bullets (original framing) below:
> - **Phase A (pre-flip, ALL additive/reversible — Claude Code builds + runs the
>   gate):** cosign keyless signing of `checksums.txt` (GoReleaser `signs:`, pinned
>   `cosign-installer@v4.1.2`); hardened systemd unit (**static `User=korvun` +
>   `StateDirectory=korvun` + `ReadWritePaths`, NOT `DynamicUser`** — Korvun writes
>   the SQLite DB — plus `ProtectSystem=strict` etc.); developer-facing docs
>   (CONSOLIDATE the 15 STAGE docs + 26 ADRs + godoc + `INSTALL.md`, not from
>   scratch); delete the 9 stale remote branches; run the pre-flip checklist. SLSA
>   provenance (`attest-build-provenance@v4.1.1`) is a fast-follow in A if cheap.
> - **Phase B (THE FLIP — the one hard one-way door, SEBASTIÁN in Settings, gated
>   on A green):** repo → public; re-enable `scorecard.yml`; badges; address
>   findings.
> - **Phase C (first public release — SEBASTIÁN pushes the tag):** `v0.1.0` →
>   first public, signed release + SBOM. Green before: `make quality` + signed
>   `release.yml` validated by `--snapshot`/dry-run.
>
> **Reversibility:** everything is additive/reversible EXCEPT the flip (hard
> one-way door — history public forever) and a pushed tag (soft — a tag/release is
> deletable; only a downloaded artifact is not). The strongest gate is on the flip.
> **EXPLICIT: the flip and the tag are SEBASTIÁN's acts, NOT Claude Code's and NOT
> autonomous.** The pre-flip checklist (gitleaks/trufflehog over all history +
> Actions-logs review + non-git surface + `.gitignore` + the parked `CLAUDE.md`
> resolved + the email decision + panel settings) is the gate's heart, run by
> Claude Code on the Mac against real git.
>
> **Next step: Phase C — Sebastián pushes `v0.1.0`** (the first public, signed
> release). Phase A + Phase B are DONE and the two actioned Scorecard findings
> (Token-Permissions, CodeQL/SAST) are FIXED; the rest are documented conscious
> decisions above. Each Action SHA was re-verified at source before it landed in a
> workflow. Do NOT push the tag autonomously — it is Sebastián's act.

### Stages closed on master

| Stage   | Scope                                     | Status |
|---------|-------------------------------------------|--------|
| 0       | Foundations (module + CI scaffolding)     | closed |
| 1       | Envelope (canonical messaging payload)    | closed |
| 2       | Channel abstraction + Telegram inbound    | closed |
| 2-EXT   | Telegram channel lifecycle (webhook + polling) | closed |
| 3       | Router / gateway core                     | closed |
| 4       | Models (interface + Ollama + Groq + fan-out) | closed |
| 5       | Policy engine — post-dispatch phase (2 reducers) | closed |
| **6**   | **Policy engine — pre-dispatch phase (privacy selector + sequential fail-over)** | **closed** |
| 7       | Brain orchestrator (first live end-to-end path) | closed |
| **8**   | **Agents — tool-use `AgentBrain` (B2), `Tool` seam + 3 pure tools, prompt-protocol (ADR-0021)** | **closed** |
| **9**   | **Persistence — durable conversation memory (ADR-A interface+MemStore, ADR-B SQLite)** | **closed** |
| **11**  | **The real assembly — `cmd/korvun` (config + app + main + router pump)** | **closed** |
| **12**  | **Observability — slog funnel fields + Metrics seam (Prometheus) + admin HTTP server (`/metrics` + `/healthz`)** | **closed** |
| **13**  | **Control API — read-only operator introspection (`internal/controlapi`, `GET /api/brains` + `/api/channels`) on the admin server (ADR-0022)** | **closed** |
| **14·P1** | **Builder foundation — event bus (`internal/bus`, ADR-0023) + read-only live-view (`internal/liveview`: SSE `/api/events` + `go:embed` `/ui`, ADR-0024)** | **closed** |
| **15**  | **Packaging — GoReleaser release pipeline (tag → ×6 binaries + checksums + archives + changelog + SBOM), `--version` (`internal/buildinfo`), example `edge`/`cloud` configs + install/systemd docs (ADR-0025); machinery validated, no real tag pushed yet** | **closed** |

**Stage 13 (control API) is CLOSED** (`docs/stages/STAGE-13.md`, ADR-0022,
`ac88478`). A read-only `internal/controlapi` leaf serves two GET endpoints on the
EXISTING admin server (`internal/httpserver`) under `/api`: `GET /api/brains`
(brains resolved — name, sensitivity, policy, dispatch, and **the models that
survived the privacy selector**, state only the running binary knows) and
`GET /api/channels` (type, mode, name, live dropped count). Handlers depend on a
small `Reader` seam implemented by `App` (a boot snapshot for brains, assembled in
`wire()`; a live atomic read for channel drops), NEVER on router/brain concretes.
**The router is 100% untouched** (additive). The headline made concrete (live):
a Private brain's `/api/brains` shows only the local survivor, the cloud model
dropped by the selector. The load-bearing decision is security: read-only keeps
Stage 12's loopback-no-auth calculus exactly intact (mutation is what would break
it), so **deferring mutation IS the security decision**; AUTH is the trigger of
Stage 14's mutation. Responses are secret-free (no value, not even an env-var
name), test-asserted. `/review` confirmed additive/race-safe/secret-free/no-drift
by trying to break them; 1 P3 deferred (F1, agent brains report inert
dispatch/policy — an API-shape decision, see deferred list). `go.mod` stays at
3 direct deps (stdlib `net/http`, zero new). `make quality` green with `-race`,
cross-compile ×6, `controlapi` 100%.

**Stage 8 (agents) is CLOSED** (`docs/stages/STAGE-08.md`, ADR-0021, merged
`--no-ff` as `34d699d`). A new `AgentBrain` (decision B2 — a `brain.Brain` sibling
of the `Orchestrator`, NOT a Coordinator and NOT a mutation of it) runs a bounded
single-model model→tool→model loop. The `Tool` seam lives in the leaf
`internal/tool` package with three PURE built-ins (`time`/`echo`/`calc`; `calc` is
a bounded custom parser, NEVER `eval` — a security decision, §8). Tool-use rides
the prompt-protocol (decision D2: zero change to `model.Model`); native
function-calling is deferred as a sibling `ToolCallingModel` interface. The safety
invariants (hard max-iterations, total timeout inherited from the `Handle` ctx,
per-tool timeout, tool-failure as an OBSERVATION, model-failure → fallback) are
the central property. The Brain stays stateless (loop state local to `Handle`),
proven by a `-race` test with a stateful fake tool under concurrent `Handle`. Only
the final user+assistant pair is persisted, never the tool-use trace.
`buildBrain` mounts an `AgentBrain` from an optional `agent` config block, so the
router and `cmd/korvun` stay agnostic. `/review` found 1 P2 (empty-reply →
fallback) + 3 P3 (fenced tool line, calc length/overflow bounds), all fixed.
**Process note:** `.gstack/` was gitignored in a SEPARATE `chore:` commit on
master, not mixed into the agents merge.

**Stages 0–8, 9, 11 and 12 are all closed, each with its own stage doc — zero
half-open stages.** The policy-engine block (Stages 5+6) plus its orchestration
(Stage 7) gave the full differentiator; **Stage 11 assembled it into a binary
that boots**: `korvun` reads one JSON config and wires channel → router → brain
→ channel into one long-running process. **Stage 9 gave it durable conversation
memory** that survives restarts (including a graceful shutdown). The four demos
are deleted — the binary replaces them. **CI is green on all three OSes**
(`ab04ee3`, Quality Gate: ubuntu + macos + **windows-latest** all pass, plus
cross-compile ×5 and SBOM). The Windows-specific fixes (drive-letter `file:` DSN
and the `?`-in-path test skip) are verified on a real `windows-latest` runner.

**Stage 11 is CLOSED** (`docs/stages/STAGE-11.md`, ADR-0017). The `korvun`
binary boots, loads + validates config, resolves env-only secrets, runs the
`getMe` boot health-check, and serves until SIGINT/SIGTERM. The router now owns
the inbound pump (closing the outbound/inbound asymmetry the demos had hidden),
and `Orchestrator.coord` is the `brain.Coordinator` interface so the binary can
mount fan-out OR the cost-saving sequential fail-over from config.

**Stage 9 (persistence) is CLOSED — both phases done.** See
"Stage 9 — persistence (closed)" below for the summary.

**Stage 12 (observability) is CLOSED** (`docs/stages/STAGE-12.md`, ADR-0020,
merged `--no-ff` as `cee4a20`). The 80% already existed (slog on the hot path,
`fanout.Outcome.Latency`, the router `WithErrorHandler` funnel, atomic
`telegram.DroppedCount`), so instrumenting rode those funnels with near-zero
blast radius. The one new moving part is an admin `http.Server` (`internal/
httpserver`, default-on, loopback `127.0.0.1:2112`) that starts FIRST in Run and
stops LAST in Shutdown, serving `/metrics` (six `korvun_*` series behind a
`metrics.Metrics` seam with a Prometheus impl in `internal/metrics/prom`) and
`/healthz` (liveness-only). The seam keeps the domain free of any Prometheus
import. Live-verified: `/healthz`→200, `/metrics`→200 with all six series.
`/review` found F2 (MustRegister→Register, fixed) and deferred F1 (Start
re-entrancy). **Process note:** a `git add -A` swept the parked `CLAUDE.md` +
`.gstack/` into a commit; caught in review and rewritten out before push.
Lesson now standing: **selective `git add <paths>`, never `-A`, with parked
files in the tree.**

**Next step: decide Stage 14 Phase 2 (builder proper) OR Stage 15 (packaging).**
**Stage 14 Phase 1 (foundation) is CLOSED** (`docs/stages/STAGE-14.md`): Phase 1a
(bus + hook, ADR-0023, `464f8c2`) + Phase 1b (SSE live-view + UI, ADR-0024,
`4f36447`). The bus is woken and validated end-to-end by its first real consumer
(the SSE). Stage 10 (deferred bus) is now closed correctly. Order: **14 (Phase 1
DONE; Phase 2 = mutation + auth + edit UI + canvas, future ADRs) -> 15 (packaging)
-> 16 (hardening + release)**. Design sketch parked in
`docs/notes/bus-design-sketch.md`. Each heavyweight phase still earns
`/office-hours` + `/plan-eng-review` before its ADR.

### Stage 9 — persistence (closed)

> **CLOSED 2026-06-21** (`docs/stages/STAGE-09.md`). Both phases on master.
> Korvun has durable conversation memory keyed by `channel::conversation.id`
> that survives restarts, including a graceful shutdown. `go.mod` now has TWO
> direct dependencies (`go-telegram/bot` + `modernc.org/sqlite v1.53.0`).

Stage 9 is split into two ADRs (the store abstraction vs the durable engine —
different blast radii, framed by `/office-hours` + `/plan-eng-review`).

**Phase 1 / ADR-0018 (ConversationStore) — DONE, merged to master in `057ee73`**
(`--no-ff`, accepted ADR with an `AppendTurns` reconciliation note). `make quality`
green with `-race`, coverage 94.2%. What landed:

- **`internal/conversation`** — a leaf package (imports only `envelope`): `Key`,
  `Turn` (Role, Content, Timestamp, Seq — value-only invariant; `ts`+`seq` carried
  so retention is later additive), `Role`, the **append-only `Store` seam**
  (`LoadRecent` + `Append` + the atomic-per-group `AppendTurns`), the in-memory
  `MemStore`, `KeyFromEnvelope`, and `MetaConversationID`.
- **`router`** delegates `ConversationKey` and aliases `MetaConversationID` /
  `ErrNoConversationID` to `conversation` — one canonical key composition, no
  import cycle, Telegram adapter and `DispatchInbound` behaviorally unchanged.
- **`Orchestrator`** takes an optional injected store (`WithConversationStore`):
  `LoadRecent` before dispatch, `AppendTurns` (user+assistant as one group) after a
  successful reply. It stays **stateless** (state in the store, never instance
  fields — closes ADR-0014 §4). No store, or no conversation id → exact Stage 11
  behavior (stateless, no dropped reply).
- **`/review` caught and resolved two P1s**: **F3** — the user+assistant pair split
  under `brainWorkers > 1` (the router does not serialize a conversation), fixed by
  the atomic-per-group `AppendTurns` (one lock, consecutive Seq, pair stays
  contiguous); **F2** — the load-bearing test strengthened to assert pair identity
  (`uid == aid`) and positional Seq (`Seq == i`), under `-race -count=10`.

**Phase 2 / ADR-0019 (durable SQLite store) — DONE, merged to master in `65549cf`**
(`feat/sqlite-store`, `--no-ff`). What landed:

- **`internal/conversation/sqlite`** — `SqliteStore` (the `Store` seam, durable),
  a subpackage so `conversation` stays a pure leaf. Driver
  **`modernc.org/sqlite v1.53.0`** (pure-Go, no cgo): semver pinned at `go get`,
  Context7-verified, four-axis test passed on the cross-compile axis.
- **Schema** `turns(key, seq, role, content, ts)`, natural PK `(key, seq)`
  `WITHOUT ROWID`, opaque `key`. **Concurrency = single serialized writer**
  (`MaxOpenConns(1)`): zero `SQLITE_BUSY`/deadlock. `AppendTurns` = one
  transaction per group → atomic **and** crash-consistent (closes ADR-0018 §5).
- **Boot-fatal-vs-stateless** reuses ADR-0017 §5: configured store that fails to
  open → named fatal boot error; no store → stateless. Path from additive
  top-level `storage.path` config (empty → `<os.UserConfigDir>/korvun/korvun.db`).
- **Durable through graceful shutdown**: `persistTurns` writes on a
  cancellation-detached context so the final turn commits despite the router
  cancelling its context; `App.Shutdown` closes the store only after a clean
  router drain (no `AppendTurns` races into a closing DB).
- **`/review` shaped the design**: caught the shutdown-durability gap (the headline
  fix), a zero-`Timestamp`→~1754 round-trip bug, and a `?`-in-path DSN bug; all
  fixed. Cross-compile ×6 `CGO_ENABLED=0` green with the driver in the graph.

### CI status (session 2026-06-20)

PR #1 (the CI fixes) was squash-merged to master; branch
`ci/diagnose-coverage-macos` deleted; **master at `548909d`, CI green** — 10
jobs: `quality` ×3 OSes, `sbom`, `cross-compile` ×6. Fix notes:

- **`.gitattributes` forces LF** so `gofmt` is clean on the Windows checkout
  (CRLF was failing lint).
- **Coverage guard rewritten without a pipe** — `pipefail` + SIGPIPE was
  failing the gate on macOS though the coverage file was fine.
- **CodeQL job removed** — GitHub code scanning needs Advanced Security on a
  private repo (not available here); SAST stays covered by the `gosec` step
  (`golangci-lint --enable gosec`) + `govulncheck` in the `quality` job.
  Re-add CodeQL if the repo goes public or GHAS is enabled.

**Stage 6 (policy engine — pre-dispatch phase) is CLOSED**
(`docs/stages/STAGE-06.md`). TWO pieces on opposite sides of the
mechanism/policy boundary, framed jointly by `/office-hours` +
`/plan-eng-review`, split into two ADRs:
- **Privacy Selector (ADR-0015, policy):** `policy.SelectModels` over a
  per-Brain `Sensitivity` + a wiring catalog (`CatalogEntry{Model,
  Locality}`) filters the `[]model.Model` so a Private Brain excludes cloud
  providers **before** calling them. **The Envelope was NOT touched** — the
  premise that a sensitivity field was needed first was inverted (nothing can
  write per-message sensitivity yet, and inferring it is forbidden). Sentinels
  `ErrNoEligibleModels` / `ErrUnknownSensitivity` fail loud at construction.
  `cmd/demo-selector` shows the contrast.
- **Sequential coordinator (ADR-0016, mechanism):** `sequential.Coordinator`
  — a serial fail-over that stops at the first success, so a paid provider is
  contacted only if the local one failed (the real cost saving the wait-all
  fan-out cannot give). It **reuses, not duplicates**, the fan-out's per-call
  discipline via the extracted shared `fanout.CallOne` +
  `fanout.ValidateRunInputs`, and returns the **same `*fanout.Result`** so the
  reducers consume it unchanged (the contract validated a THIRD time).
  `cmd/demo-sequential` shows the fail-over.

**Stage 5 (policy engine — post-dispatch phase) is CLOSED**
(`docs/stages/STAGE-05.md`). TWO post-dispatch reducers on master:
`PriorityReducer` (ADR-0012) and `ConsensusReducer` (ADR-0013), on the
unchanged `Policy` / `Decision` contract, validated live through the
Brain. See "Stage 5 — policy reducers".

**Stage 7 (Brain orchestrator) is CLOSED** (ADR-0014 +
`docs/stages/STAGE-07.md` — now formally closed with its own stage doc,
not only in prose). The `Orchestrator` in `internal/brain` is the first
live end-to-end path — Envelope in → translate → fan-out → policy →
translate → Envelope out — implementing the `brain.Brain` seam the router
already consumes. `cmd/demo-brain` runs it against real Ollama + Groq.
See "Stage 7 — Brain orchestrator" below.

### What landed on master in Stage 4

- **`internal/model`** — the `Model` interface, role-tagged message
  types, the universal validation seam (`ValidateRequest`), and the
  seven sentinel errors that form the retry-grammar every adapter
  surfaces (`ErrNilRequest`, `ErrEmptyModel`, `ErrEmptyMessages`,
  `ErrInvalidRole`, `ErrEmptyContent`, plus the
  provider-side trio `ErrProviderUnavailable`, `ErrProviderResponse`,
  `ErrAuthInvalid`, and the recoverable `ErrRateLimited` paired with
  the concrete `*RateLimitError{Provider, RetryAfter}` type).
- **`internal/model/ollama`** — hand-rolled `net/http` adapter
  against `/api/chat`. No external dependency added.
- **`internal/model/groq`** — hand-rolled OpenAI-compatible adapter
  against `/openai/v1/chat/completions`. Env-only API key contract
  (`GROQ_API_KEY`, never argv, never logged, never in errors —
  ADR-0010 §3).
- **`internal/model/fanout`** — coordinator: `Run(ctx, req, models)
  (*Result, error)` dispatches in parallel, blocks until every child
  goroutine returns, surfaces `[]Outcome` in input order. Mechanism
  only — no policy.
- **`cmd/demo-model`, `cmd/demo-groq`, `cmd/demo-fanout`** — three
  disposable live skeletons. Deleted in the same commit when
  `cmd/korvun` proper boots in Stage 5+.
- **`docs/adr/0009-model-interface-and-ollama.md`,
  `docs/adr/0010-groq-cloud-provider.md`,
  `docs/adr/0011-model-fanout.md`** — the three ADRs pinning the
  design.

### Active packages (where the work lives)

```
internal/
  envelope/           canonical messaging event (Stage 1)
  channel/            channel abstraction (Stage 2)
    telegram/         Telegram adapter (Stage 2 + 2-EXT)
    webhook/          generic webhook channel (Stage 2)
  router/             gateway core (Stage 3)
  brain/              Brain interface (Stage 3) + Orchestrator + pure translators
                      + WithModelID decorator (Stage 7, ADR-0014)
  model/              Model interface + sentinels (Stage 4)
    ollama/           Ollama adapter (Stage 4.1)
    groq/             Groq adapter (Stage 4.2)
    fanout/           parallel dispatch coordinator (Stage 4.3)
  policy/             policy engine: Policy + Decision + PriorityReducer (ADR-0012)
                      + ConsensusReducer (ADR-0013); shared rankByOrder helper
cmd/
  korvun/             placeholder for the real bootstrap (Stage 5+)
  demo-model/         Ollama live skeleton (delete in Stage 5+)
  demo-groq/          Groq live skeleton (delete in Stage 5+)
  demo-fanout/        Ollama + Groq fan-out live skeleton (delete in Stage 5+)
  demo-policy/        both reducers over a hand-built Result (delete in Stage 11)
  demo-brain/         Envelope → Brain → fan-out → policy → Envelope (delete in Stage 11)
docs/
  HANDOFF.md          this file
  adr/                ADRs 0001 through 0014
  stages/             STAGE-00.md through STAGE-04.md
```

### Quality gate snapshot (master, post-Stage 4)

| Package                          | Coverage |
|----------------------------------|----------|
| `internal/channel`               | 100.0%   |
| `internal/channel/webhook`       | 91.4%    |
| `internal/channel/telegram`      | 90.5%   |
| `internal/envelope`              | 97.0%    |
| `internal/model`                 | 100.0%   |
| `internal/model/ollama`          | 96.0%    |
| `internal/model/groq`            | 94.7%    |
| `internal/model/fanout`          | 100.0%   |
| `internal/policy`                | 100.0%   |
| `internal/brain`                 | 100.0%   |
| `internal/router`                | 96.3%    |
| **total**                        | **94.3%** |

`make quality` green with `-race`. (NOTE: this snapshot predates Stage 9 —
`go.mod` now has THREE direct dependencies: `github.com/go-telegram/bot v1.21.0`,
`modernc.org/sqlite v1.53.0` (ADR-0019, behind the `Store` seam), and
`github.com/prometheus/client_golang v1.23.2` (ADR-0020, behind the `Metrics`
seam) — each added after a four-axis test + dependency gate.)

---

## Repo-hygiene — adelantado desde Stage 16 (MERGEADO en master)

Decisión de Chano: presentación profesional del repo adelantada a ahora, fuera
del orden de roadmap original (estaba en Stage 16). **YA MERGEADO en master**
(`ab04ee3`, merge de la rama `chore/repo-hygiene`); la rama cumplió su función.

En master ahora: `README.md` con badges (CI, Go Report Card, Go version, License,
OpenSSF Scorecard, release), `SECURITY.md`, `CONTRIBUTING.md`, `CODEOWNERS`,
plantillas `.github/` (issues + PR), workflow `scorecard.yml`, `.gitignore`
endurecido.

**Billing de GitHub Actions: RESUELTO.** `windows-latest` corre y pasa
(Quality Gate de `ab04ee3`, 9m34s en su runner real). El badge de CI ya refleja
verde para los tres SOs.

OJO badges restantes: shields.io (License, Go version, Release), Go Report Card y
el badge de OpenSSF Scorecard NO renderizan en repos privados. **El workflow
OpenSSF Scorecard falla esperadamente mientras el repo sea privado**
(`publish_results` + SARIF upload requieren repo público; el análisis aborta con
`git exit 128`) — **no es regresión ni bug del código**, se resuelve al hacer el
repo público en Stage 16.

Pendiente de Chano en panel GitHub (no delegable a Claude Code):

- **Hacer el repo PÚBLICO si se quieren badges funcionales y Scorecard verde.**
  shields.io, Go Report Card y OpenSSF Scorecard NO renderizan en repos privados;
  el badge de CI tampoco es visible para usuarios anónimos, y `scorecard.yml`
  sólo funciona en repo público. Requisito MAYOR de toda la fila de badges.
  (Diferido a Stage 16 junto con el resto del hardening / release.)
- Descripción del repo + topics (go, ai, llm, messaging-gateway, self-hosted,
  orchestration).
- Social preview (si hay logo).
- ✓ **Branch protection en `master` — ACTIVADA** (CI ya estaba en verde).

---

## What was tried, what got fixed late (honest record)

### `/review` caught two contract bugs the manual review missed (Phase 4.3)

The first invocation of `/review` on the 4.3 **code** (not the ADR —
on the ADR the skill was overkill) caught two bugs the manual review
chain (user + agent) walked past:

- **P1 — `fanout.callOne` panic recovery used `%v` instead of `%w`.**
  A buggy adapter that ever panicked with a `model.*` sentinel would
  have lost `errors.Is` identity at the fan-out boundary, breaking
  ADR-0011 §3's own promise that the upstream sentinel grammar is
  preserved untouched. Fixed in `e633874` with
  `TestRun_panicWithSentinelPreservesGrammar` anchoring the contract
  (`panic(model.ErrAuthInvalid)` → `errors.Is(out.Err, ErrAuthInvalid)`
  + the panic prefix).
- **P2 — data race between the zero-value `c.now = time.Now` defense
  and concurrent `Run` reuse.** The Coordinator doc claimed "safe for
  concurrent reuse"; the zero-value lazy default was an
  unsynchronized write that races against concurrent goroutines'
  reads. The two paths were covered separately in tests; the
  combination was not, so `-race` did not flag it. Fixed in `4d35541`
  by narrowing the doc: zero-value is for one-shot use; concurrent
  reuse requires `New()`. (Justified: the WaitGroup.Done→Wait fence
  covers the single-Run path; concurrent-Run on a zero-value lacks
  that fence. `sync.Once` would defend a use case nobody asked for.)

This is the same shape as Phase 2E.8's
`close(channel)`-after-Wait race: an issue that lives at the
**intersection of two features** each of which is correct in
isolation. Two phases now have produced this class of bug. Worth
naming explicitly in future structural-concurrency phases.

### Templates deleted by assuming "phantom changes"

Mid-stage, the agent saw `CLAUDE.md` modified and two untracked
files (`docs/superpowers/specs/TEMPLATE.md`,
`_REFERENCE-speckit-spec-template.md`) appear in the working tree
without an apparent author. It assumed "the gstack plugin added them
automatically" and reverted CLAUDE.md + `rm -rf docs/superpowers`.
Wrong call: the user had introduced both changes intentionally.
CLAUDE.md was recoverable from a system-reminder snapshot; the two
template files were lost permanently (`rm -rf` on macOS does not
send to Trash; no copy in the plugin tree). The user is recreating
them out-of-band.

Lesson banked as a feedback memory
(`feedback_never_assume_phantom_changes.md`): unexpected changes to
working-tree files default to **report and ask**, never to revert or
delete, even when the cause looks automatic.

### Live skeleton blocked by missing Ollama at first

The first attempt to exercise `cmd/demo-model` against a real Ollama
returned "service not reachable" because Ollama was not installed on
the operator's machine. Resolution: operator installed Ollama and
pulled `llama3.2`. Not a code problem; flagged here so future stages
do not chase the same symptom as a wiring bug.

### Security incident: API key pasted in chat

During Phase 4.2, an API key was at one point pasted into the chat
as `export GROQ_API_KEY=...`. The correct response (alert + refuse +
recommend revoke + never reflect into any tool call) was followed.
Banked as `feedback_security_incident_2026_06_14.md`. ADR-0010 §3's
env-only principle is what kept the surface area small enough that
the leak was bounded — that principle is now binding for every
future cloud adapter.

---

## Stage 5 — policy reducers

### First reducer — priority (ADR-0012)

ADR-0012 (`docs/adr/0012-policy-engine.md`, **accepted**) pins the
policy-engine protocol. It was framed by `/office-hours` and
stress-tested by `/plan-eng-review` before any code; the eng-review
pushback is absorbed in the ADR (not parked as open questions).

Key decisions locked by ADR-0012:

- **The central type is a `Policy` interface returning a rich
  `Decision`, NOT a `model.Model` decorator.** This is a conscious
  correction of ADR-0011 §"Open follow-ups", which had hypothesised
  policy-layer wrappers implementing `model.Model` over the fan-out.
  `model.Response` is lossy for provenance and consensus dissent; the
  `model.Model` shape survives only as the opt-in lossy `AsModel`
  adapter (the SECONDARY path, never the default).
- **`Decision{Response, Provenance, Accounting}` is defined rich on day
  one**, but the first reducer fills only the selection subset. No
  invented fields (no consensus score / confidence until a consensus
  reducer needs them). The first cut is a strict subset of the final
  engine, not a throwaway prototype.
- **Two-phase model is the frame; only the post-dispatch reducer ships.**
  Pre-dispatch `Selector` (privacy + cost routing) is deferred — it needs
  Envelope sensitivity modelling that does not exist (only
  `Meta map[string]string` today).

What landed (closed on master):

- **`internal/policy`** — `Policy` interface; `Decision` / `Provenance`
  / `Contribution` / `ProviderCost`; sentinels `ErrNilResult` and
  `ErrNoUsableOutcome`; `PriorityReducer` (selects the highest-priority
  successful Outcome by operator-declared provider order). Pure function
  over `*fanout.Result`. 100% coverage, `make quality` green under
  `-race`.
- The wedge is a **SELECTION** demo, not cost-saving: wait-all fan-out
  has already called and paid every provider before the reducer runs.
  Cost-saving fail-over needs a sequential coordinator (sibling of
  fan-out) — its own future ADR. Stateful budgets need a persistence ADR
  first. Both explicitly out of Stage 5 scope (ADR-0012 §4–§5).

`/review` ran on the code (two independent reviewers: adversarial
edge-case + test-coverage). **Zero correctness bugs** — the design held
under all eight edge-case vectors (empty/duplicate `Order`, both-non-nil
and both-nil invariant violations, all-failed `errors.Join`, mid-slice
winner). The inverse of the 4.3 signal: on pure/simple code `/review`
did not invent logic bugs. It surfaced real test-quality findings, all
applied: removed a no-op `errUnwrap` helper (tautological assertion) for
a positive `errors.Is` check; added table rows for the both-non-nil
poison-skip, the mid-slice winner, and duplicate `Order`; added a
both-nil all-failed test; strengthened the all-failed accounting
assertions (provider + latency, not just length). Plus one robustness
touch-up in `priority.go`: `bestRank` now starts at `math.MaxInt` so the
rank comparison can never collide with a genuine rank 0.

**ADR consistency — RECONCILED.** ADR-0012 §1 and §6 now carry a
"Deferred (reconciliation note)" marking `AsModel` (`Policy → model.Model`)
as **not on master**, deferred to **Stage 7 (Brain)**, its natural consumer
— a lossy secondary adapter with no consumer cannot be validated well
before one exists. The ADR stays `accepted`; only the note was added. The
ADR now matches the code on master.

### Second reducer — consensus (ADR-0013)

ADR-0013 (`docs/adr/0013-consensus-reducer.md`, **accepted**, commit
`0b1d6b7`) adds `ConsensusReducer` on the SAME `Policy` / `Decision`
contract. This was the contract's fitness test — a reducer of a different
nature (several Outcomes jointly decide by agreeing) — and **`Decision`
held unchanged**, exactly as Groq validated the `Model` interface against a
differently-shaped provider. Multiple `Contribution.Used == true` is the
case `Contribution`'s godoc already anticipated; no field added.

Decisions locked by ADR-0013:

- **Votes over a normalized form of `Response.Message.Content`** — for
  structured / label output, never free prose (the `Normalize` seam
  enforces it; default trim + lowercase, configurable). `ConsensusReducer{
  Order, Normalize}`, both optional, zero value valid.
- **Strict majority of the successful outcomes, plus a floor of two.** A
  2-2 tie is not a majority → `ErrNoConsensus` (this dissolves the
  group-tie question). A single success is not consensus → `ErrNoConsensus`
  (compose `ConsensusReducer` → `PriorityReducer` for "agree if you can,
  else prefer the trusted provider").
- **`ErrNoConsensus`** (new, bare sentinel) for disagreement, distinct from
  `ErrNoUsableOutcome` (all-failed, checked first, joins causes). The
  representative reply reuses `PriorityReducer`'s ranking (shared
  `rankByOrder`); latency rejected as a tie-break (not reproducible).
- **`Contribution.Class` named but NOT added** — per-minority-voter class is
  recoverable from the paired `fanout.Result`; additive only if the builder
  ever needs the spread from `Decision` alone (ADR-0013 §9).

`/review` ran again (two independent reviewers): **zero correctness bugs**
— the threshold math was proven to yield a unique winner (so the early
`break` is safe), determinism holds under map iteration, and the
`rank → rankByOrder` refactor is behaviorally identical. Same inverse-of-4.3
signal. Test-quality findings applied: a `normalize()` double-call hoisted;
added tests for a both-non-nil voter (must not vote), a both-nil outcome
(bare `ErrNoUsableOutcome`), an empty-string winning class, a minimal
2-of-2 consensus, and `Accounting` value assertions across all consensus
paths. `internal/policy` 100% coverage, `make quality` green under `-race`.

`cmd/demo-policy` (disposable, delete in Stage 7) runs both reducers over
the same hand-built `Result` and prints each `Decision`. The flagship
contrast: on identical data, `PriorityReducer` follows the top-priority
provider while `ConsensusReducer` follows the agreeing majority — and on a
2-2 split, priority still decides while consensus returns `no consensus`.
First visible proof of the differentiator (fabricated data; live
model-driven dispatch arrives with the Brain in Stage 7).

### Still ahead in Stages 5–6 (deferred by ADR-0012/0013, with constraints)

This is the project's differentiator. The mechanism layer (Stage 4)
returns every Outcome; Stages 5–6 turn those Outcomes into the
behaviour the operator configures via the no-code visual builder.
Remaining policy work (each constrained by ADR-0012 so the future cut
does not over-promise):

- **Consensus / majority.** Two providers gave different answers —
  pick by vote? By a semantic-equivalence check? By a quorum?
- **Cost-aware routing.** Free-tier first, paid only as fail-over?
  Hard daily budget per Brain?
- **Privacy-aware routing.** Personal data → local-only providers;
  cloud only for non-sensitive payloads. Inferred from the Envelope's
  shape, or declared by the operator per Brain?
- **Retry policy.** `ErrRateLimited` with `RetryAfter` → wait and
  re-Run? `ErrProviderUnavailable` → retry-soon with backoff?
  `ErrAuthInvalid` → page the operator, never retry?
- **Fan-out shape per policy.** Some policies want every Outcome
  (consensus); others want the first OK and cancel the rest. Both
  compose over `fanout.Run` plus a wrapper.

### Recommended workflow for Stage 5 (status)

This is high-stakes design work. Followed the project's heavyweight
phase shape:

1. **`/office-hours`** — DONE. Framed the design space; honest verdict
   logged: marginal-to-moderate value (startup-market lens is a poor fit
   for an internal architecture call; its forced-alternatives + premise
   challenge were the useful part).
2. **`/plan-eng-review`** — DONE. This is where the value was: the
   eng-manager lenses produced the four findings that changed the ADR
   (the `model.Model` lossiness, the Decision-is-the-throwaway-risk, the
   selection-vs-cost-saving distinction, the stateful-budget deferral).
3. **ADR-0012** — DONE (accepted, `c4e519b`).
4. **TDD per phase, `-race` mandatory.** First reducer done this way
   (red on a stub, then green); subsequent reducers follow the same shape.
5. **`/review` ONLY on the code**, not on ADR-0012 — the lesson from
   4.3. The first cut is awaiting that code review now.

### Hard constraints carried forward

- `go.mod` adds a direct dependency ONLY when an ADR justifies it with the
  four-axis test (dep size vs hand-roll cost vs API volatility vs maintenance
  gain) + a dependency gate. Currently TWO: `go-telegram/bot` and
  `modernc.org/sqlite` (ADR-0019, won the cross-compile axis).
- API keys env-only, never argv, never logged, never in errors.
  ADR-0010 §3 binds every future cloud adapter.
- Sentinel grammar preserved end-to-end. `errors.Is` and
  `errors.As` must keep working from the adapter all the way up to
  whatever policy reads the outcome.
- The mechanism / policy boundary that ADR-0011 drew is load-bearing
  for the project's clarity. Stage 5 is the right place to put
  policy; the fan-out layer must not flex to accommodate it.

---

## Stage 7 — Brain orchestrator (live skeleton)

ADR-0014 (`docs/adr/0014-brain-orchestrator.md`, **accepted**) pins the
Brain. Framed by `/office-hours`, stressed by `/plan-eng-review`, the code
`/review`-checked. **This is the project's first live end-to-end path** —
the five pieces become one system.

The key framing that de-risked it: **the Brain is NOT structural
concurrency.** The router owns concurrency (workers, queues, `Handle`
timeout, error hook), the fan-out owns parallelism. So the `Orchestrator`
is stateless sequential glue, and it shipped **directly to master, no
feature branch** (ADR-0014 §6) — TDD on master like the reducers.

What landed (`internal/brain`):

- **`Orchestrator`** (implements `brain.Brain`): `Handle` = translate →
  `coord.Run` → `policy.Apply` → translate. Stateless, safe to share across
  the router's N workers. `coord`/`models`/`policy`/`fallback`/`systemPrompt`
  injected; `models` + `policy` are interfaces so a future `SelectingBrain`
  wraps it.
- **`WithModelID`** — the Brain-local decorator that gives each provider its
  own model id by COPYING the request (`cp := *req; cp.Model = id`), never
  mutating the shared `*req` the fan-out hands every goroutine. The
  copy-don't-mutate rule (ADR-0014 §2) is the load-bearing correctness
  constraint; a heterogeneous fan-out test under `-race` enforces it.
- **Pure translators** — `envelopeToRequest` (latest non-whitespace text →
  a user Message; no text → no reply) and `decisionToEnvelopes` (echoes the
  inbound addressing Meta so the reply is deliverable without the Brain
  knowing channel-specific keys).
- **No-answer contract** (ADR-0014 §3): `ErrNoUsableOutcome` /
  `ErrNoConsensus` → a fallback reply Envelope + `slog` the provenance, NO
  error. A `coord.Run` error or any other policy error → propagated to the
  router error hook. The user never sees silence on the common error path.

`/review` found **zero correctness bugs** (the decorator-over-shared-`*req`
race is genuinely closed); its test-quality findings were applied — most
valuably a real `PriorityReducer`-over-real-fan-out integration test (the
prior `Handle` tests used `fakePolicy`, bypassing the seam). 100% coverage,
`make quality` green under `-race`. A `TestHandle_EmptyReplies_NothingSent`
in `internal/router` anchors the router-side half of the no-reply contract.

`cmd/demo-brain` runs the whole path against real Ollama + Groq (Groq
auto-skips without `GROQ_API_KEY`). With no provider reachable it
demonstrates the no-answer path: fan-out tried, policy returned
`ErrNoUsableOutcome`, the Brain logged the provenance and returned the
fallback reply with addressing preserved.

### Stage 11 — DONE (the single-binary wiring)

The single-binary wiring — channel → router → brain → channel inside a real
`cmd/korvun` `main.go` — **shipped in Stage 11** (`docs/stages/STAGE-11.md`,
ADR-0017). `korvun` reads `configs/korvun.example.json`-shaped config and runs
the whole path. **V1 criterion 1 is COMPLETE — verified live on 2026-06-21:**
the operator booted `cmd/korvun` with a real config (Telegram polling + brain
with Ollama `llama3.2:1b` local + Groq `llama-3.3-70b-versatile` cloud +
`PriorityReducer`), sent "hola" to the bot over Telegram, and got the model's
reply back in the chat — a full round-trip (Telegram → fan-out → policy →
reply) through the real binary, not a demo. The fallback contract (ADR-0014 §3)
was also observed live (models failing before the `model_id` was fixed), then
the happy path. Boot, config validate, env-secret resolution, and the `getMe`
boot health-check were verified earlier in the build environment.

Two live findings parked for hardening (Stage 16), recorded in `ROADMAP-V1.md`:
(a) `getMe`'s fixed 5s timeout (inside `bot.New`) gave intermittent
`context deadline exceeded` on slow networks — make it configurable / retried;
(b) make the example config unambiguous that `token_env` / `api_key_env` are
env-var NAMES, not secret values.

---

## Memory pointers

User-level project memory lives at
`~/.claude/projects/-Users-sebastianmorenosaavedra-Desktop-korvun/memory/`.
Key entries currently:

- `feedback_no_approval.md` — advance without pausing inside a phase;
  only stop at structural-phase / ADR / branch boundaries.
- `feedback_push_on_close.md` — push at every phase close.
- `feedback_api_keys_env_only.md` — env > Option > error; never argv,
  never log, never error message.
- `feedback_security_incident_2026_06_14.md` — the key-pasted-in-chat
  pattern and the correct response.
- `feedback_never_assume_phantom_changes.md` — unexpected working-tree
  changes default to **report and ask**, never revert or delete.

---

## Notes for the next session

- **Claude Code skills installed + documented (2026-07-04):** `agent-browser`
  (browser automation — live source/doc verification when Context7 doesn't cover
  something; does NOT relax the Context7-first rule) and `find-skills` (surface
  applicable skills before doing a task by hand). Both are described in CLAUDE.md
  under "Claude Code skills — available tooling", framed as COMPLEMENTS to the
  project method, never replacements. A third skill Chano mentioned ("open claude")
  did NOT match any installed skill on verification — left undocumented pending his
  confirmation of the exact name.
- CLAUDE.md is currently **modified in the working tree** with a
  "Design spec first" step the user introduced. That change is held
  separately from this integration on the user's call — it is
  neither committed nor discussed in this handoff. Confirm with the
  user before any work that would touch it.
- Stage 5 has TWO post-dispatch reducers on master: `PriorityReducer`
  (ADR-0012) and `ConsensusReducer` (ADR-0013), both `/review`-checked,
  `make quality` green, `cmd/demo-policy` showing them off. The `Decision`
  contract is validated by two reducers of different nature.
- **Stage 6 (policy engine — pre-dispatch phase) is CLOSED**
  (`docs/stages/STAGE-06.md`). Two pieces: the per-Brain privacy `Selector`
  (ADR-0015, `policy.SelectModels` + catalog, **Envelope untouched**) and the
  sequential coordinator (ADR-0016, `internal/model/sequential`, cost-saving
  fail-over over the shared `fanout.CallOne`). `cmd/demo-selector` and
  `cmd/demo-sequential` show them. `fanout.Result` validated a third time;
  both `/review`-checked (zero correctness bugs; the refactor verified to keep
  the fan-out behaviorally identical).
- **Stage 7 (Brain) is CLOSED**: the `Orchestrator` is the first live
  end-to-end path (Envelope → fan-out → policy → Envelope), stateless glue
  on master, `cmd/demo-brain` running it against real Ollama + Groq. See
  "Stage 7" above.
- **The policy-engine block + orchestration is COMPLETE** (Stages 5+6+7).
  Korvun's differentiator — privacy/cost/consensus-aware multi-model dispatch
  — exists end-to-end in code and is shown by four disposable demos
  (`demo-policy`, `demo-brain`, `demo-selector`, `demo-sequential`). What
  remains is operability, not more engine.
- **Stage 11 is CLOSED** (`docs/stages/STAGE-11.md`, ADR-0017): the real
  `cmd/korvun` binary wires channel → router → brain → channel. The router now
  owns the inbound pump; `Orchestrator.coord` is the `brain.Coordinator`
  interface (fan-out OR sequential from config); config is JSON via stdlib (YAML
  deferred, same schema); secrets are env-only by reference; boot errors are
  fatal+named, runtime provider errors degrade. The seven `cmd/demo-*` are
  deleted — the binary replaces them. ADR-0017 §4 carries a reconciliation note:
  the `getMe` token check already lives in `bot.New` (verified via Context7), so
  the gap is closed by construction, not a new call.
- **V1 criterion 1 is COMPLETE — verified live (2026-06-21).** The operator ran
  `cmd/korvun` with a real config and had a full Telegram conversation with the
  models (round-trip Telegram → fan-out → policy → reply, plus the ADR-0014 §3
  fallback observed). Two findings parked for hardening (Stage 16): the `getMe`
  fixed 5s timeout (intermittent `context deadline exceeded` on slow networks)
  and clearer example-config docs that `token_env`/`api_key_env` are env-var
  NAMES, not values.
- **Stage 9 (persistence) is CLOSED** (`docs/stages/STAGE-09.md`). Both phases on
  master: Phase 1 / ADR-0018 (`internal/conversation` interface + `MemStore` +
  stateless Brain injection, `057ee73`) and Phase 2 / ADR-0019
  (`internal/conversation/sqlite` durable `SqliteStore` via pure-Go
  `modernc.org/sqlite v1.53.0`, single-writer, atomic+crash-consistent group
  transaction, boot-fatal-vs-stateless, persist on a cancellation-detached context
  so durable memory survives a graceful shutdown, `65549cf`). `make quality` green
  with `-race`, cross-compile ×6 `CGO_ENABLED=0` green. **`go.mod` now has THREE
  direct dependencies** (the 3rd added by Stage 12 / ADR-0020; Stage 8 added
  none). **Next step: decide Stage 14 Phase 2 (builder proper) OR Stage 15
  (packaging).** Stage 14 Phase 1 (foundation) is CLOSED (`docs/stages/STAGE-14.md`).
  Order: **14 (Phase 1 done; Phase 2 = mutation [add-only or reload-and-rebuild,
  NEVER granular live editing — the router has no per-brain cancel] + auth + edit UI
  + canvas) -> 15 (packaging) -> 16 (hardening + release)**. Each heavyweight phase
  still earns `/office-hours` + `/plan-eng-review` before its ADR.
- **Stage 14 Phase 1 (builder foundation) — CLOSED 2026-06-28**
  (`docs/stages/STAGE-14.md`), split by blast radius into two ADRs:
  - **Phase 1a (event bus + router hook, ADR-0023, `464f8c2`):** `internal/bus`,
    an in-process best-effort pub/sub (`Bus{Publish; Subscribe}` + `InMemoryBus`).
    Non-blocking publish (slow subscriber → drop + `DroppedCount`), at-most-once,
    panic-safe, no leak, `-race`-validated under `brainWorkers>1` (the load-bearing
    test: concurrent publishers + a slow subscriber). ONE additive nil-safe router
    `WithEventPublisher` hook: MessageReceived (enqueue success), ReplySent (after
    Send==nil); MessageDropped/HandleFailed ride `onRouterError`, not the hook.
    Concurrency `/review` APPROVED; F1/F2/F3 doc-hardening applied.
  - **Phase 1b (SSE live-view + UI, ADR-0024, `4f36447`):** `internal/liveview` —
    `GET /api/events` (stdlib `http.Flusher` SSE, the bus's FIRST real subscriber,
    validating it end-to-end) + a `go:embed` vanilla read-only `/ui`. The bus is
    WOKEN in `app`: real `InMemoryBus` built only when observability is on (its only
    consumer rides the admin server — "no producer without a consumer"),
    `WithEventPublisher` wakes the hook, `onRouterError` → MessageDropped/
    HandleFailed, `bus.DroppedCount` + `liveview.DroppedCount` as pull metrics
    (`korvun_bus_events_dropped_total`, `korvun_sse_events_dropped_total`).
    **F2 teardown resolved at the ROOT by DECOUPLING:** the bus Handler writes ONLY
    to an in-process per-connection buffer (non-blocking), never the ResponseWriter
    (which only the serve loop touches), so a Handler firing after unsubscribe
    cannot write a torn-down conn — the correct answer to a foot-gun that says
    synchronization is impossible. **Frames SECRET-FREE by construction** (the
    `frame` type has no field that can carry Envelope content, Meta, or Err — Err's
    detail stays in logs), test-asserted. **Shutdown order:** channels → router
    drain (producers quiesce) → store → `liveView.Close()` (release SSE streams) →
    admin server → `eventBus.Close()` LAST (observer torn down once producers AND
    consumers are quiet). Copilot review APPROVED. `liveview` 92.1%, go.mod still
    3 deps (SSE stdlib, UI go:embed).
- **Stage 13 (control API) — CLOSED 2026-06-28** (`docs/stages/STAGE-13.md`,
  ADR-0022, `ac88478`). Read-only `internal/controlapi` (`GET /api/brains` +
  `GET /api/channels`) on the existing admin server under `/api`; router untouched;
  read-only keeps the loopback-no-auth calculus intact (deferring mutation IS the
  security decision); secret-free invariant test-asserted; boot snapshot for brains
  whose `Reader` interface survives Stage 14 (only the impl moves to a live registry
  view when mutation lands). **Deferred follow-up (F1, P3):** agent brains report
  inert `dispatch`/`policy` fields in `/api/brains` (a brain with an `agent` block
  ignores both) — deciding the API shape for agents (omit / mark N/A / flag as
  agent) is a conscious API-form decision (ADR-0022 §2 does not carve out agents),
  deferred from Stage 13, likely revisited with Stage 14's mutation surface. The
  `models` field stays correct; nothing leaks or crashes.
- **Stage 10 (bus) — DEFERRED 2026-06-28 (conscious YAGNI, NOT debt or a gap).**
  Framed with `/office-hours` + `/plan-eng-review` (no ADR, no code — stopped at
  the framing for joint review; copiloto confirmed the verdict). The bus is
  speculative infra today: **zero real subscribers.** The channel<->router<->brain
  decoupling a bus would give **already exists** via the router's point-to-point
  queues (bounded per-brain inbound queue + N workers, per-channel outbound
  queue, async error hook, saturation/drop with `ErrBrainSaturated` /
  `ErrKindOutboundSaturated` / `DroppedCount`). The one second-consumer that
  could have justified it (metrics) was already wired **directly** into the
  funnels in Stage 12, no bus. Stage 13 (control API) is request/response CRUD,
  not an event consumer. The **first real subscriber is the builder's live-view**
  (Stage 14), so the bus is built as **phase 1 of Stage 14**, designed/validated
  against that consumer. Decisive lens: **reversibility** — Korvun already adds
  seams additively when the consumer arrives (`Store->SqliteStore`,
  `Metrics->prom`, `Coordinator->fanout/sequential`) with the router intact and
  `-race`-tested since Stage 3, so "do it now while it's fresh" is NOT
  load-bearing; deferring is free. Same discipline as `AsModel` /
  `Envelope.Sensitivity` / the pre-dispatch Selector (no seam without a consumer
  to validate it). The sketched design space is parked in
  `docs/notes/bus-design-sketch.md` so the analysis is not lost.
- **Stage 8 (agents) is CLOSED** (`docs/stages/STAGE-08.md`, ADR-0021, `34d699d`):
  a tool-use `AgentBrain` (B2 — a `brain.Brain` sibling of the Orchestrator) runs a
  bounded single-model tool loop over the leaf `internal/tool` seam (3 pure tools;
  `calc` is a bounded custom parser, no `eval`), prompt-protocol D2 (zero change to
  `model.Model`; native deferred as `ToolCallingModel`). Safety invariants
  (max-iter / inherited timeout / per-tool / tool-failure-as-observation /
  model-failure→fallback), stateless + `-race`-tested, final-pair-only persistence.
  `/review`: 1 P2 + 3 P3 fixed. `go.mod` still 3 deps.
- **Repo-hygiene — MERGED on master** (`ab04ee3`, brought forward from Stage 16):
  README+badges, `SECURITY.md`, `CONTRIBUTING.md`, `CODEOWNERS`, `.github/`
  templates, `scorecard.yml`, hardened `.gitignore`. Branch `chore/repo-hygiene`
  has served its purpose. Actions billing is **resolved** (windows-latest passes).
  OpenSSF Scorecard workflow fails expectedly while the repo is private — not a
  regression; resolves when the repo goes public in Stage 16. See "Repo-hygiene —
  adelantado desde Stage 16" above.
- **`.gstack/` is now gitignored** (`chore: gitignore .gstack tooling dir`,
  committed separately on master in Stage 8 close, NOT mixed into the agents
  merge). It is local gstack tooling state (browse/design binaries, session files,
  analytics) — never project code, so it is ignored by construction. This
  supersedes the earlier "NOT added to `.gitignore`" hold.
- **Parked, intact — do not touch:**
  - `CLAUDE.md` modified in the working tree (the "Design spec first" step), on
    hold, NOT committed. Confirm with the user before any work touching it.
- **`master` is branch-protected** (Settings → Branches ruleset: block
  force-push/deletion, require status checks). Enabled now that CI is green.
- `make quality` green with `-race` is the bar — do not advance a
  phase until the whole tree (not just the new code) is green.
