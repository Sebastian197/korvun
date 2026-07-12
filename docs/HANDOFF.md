# HANDOFF — Korvun

> **Read this at the start of every session.** Restores the project
> context, the current state, and the next thing to do without having
> to re-derive it from `git log`. CLAUDE.md is the operating rules;
> HANDOFF is the running state of the work.

---

## Tooling — project knowledge graph (graphify) — consult FIRST

> **Set up 2026-07-11.** Korvun is indexed as a queryable knowledge graph with
> **graphify** (installed globally; `graphifyy` on PyPI, CLI `graphify`; skill at
> `~/.claude/skills/graphify/`). Outputs live in `graphify-out/`:
> `graph.json`, `GRAPH_REPORT.md`, interactive `graph.html`.
>
> **RULE (in CLAUDE.md): consult the project THROUGH the graph first** —
> `graphify query "<question>"`, `graphify path "A" "B"`, `graphify explain "X"` —
> before grep/read. Grep/Read is the fallback for what the graph does not cover.
>
> **First build:** 2.390 nodes · 5.895 edges · 150 communities (191 Go files via
> AST + 81 docs/ADRs/STAGE via semantic extraction). God nodes: `OutboundParams()`
> (70), `New()` (59), `InboundFromUpdate()` (57), `shutdown()`, `Key`, `Build()`.
> Health note: ~812 semantic edges have a dangling endpoint (LLM-guessed target
> IDs that didn't match an AST node id) — those edges don't connect; the graph is
> still usable.
>
> **Auto-update:** a branch-guarded `post-commit` hook (`.git/hooks/post-commit`,
> `graphify hook install` + a `master`-only guard) rebuilds the graph on every
> commit **to master** (code-only AST re-extract, detached, no LLM). The default
> `post-checkout` hook was removed on purpose (it would rebuild off non-master
> branches, contradicting the master-tracks-the-graph rule). After large **doc**
> changes, refresh manually: `/graphify --update`.

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

## Current state (as of session close, 2026-07-12)

> **CURRENT (2026-07-12): Piece 3 (CLI) sub-phase 1 DONE and committed
> (`feat(cli): add subcommand dispatch with serve seam and -config shim`,
> `7c3742c`).** A new `internal/cli` package owns argv parsing + subcommand
> dispatch behind a single `Run(args, stdout, stderr) int`, so `cmd/korvun/main`
> is now a 3-line forwarder (ADR-0017). Landed: `serve` / `version` / `help`
> dispatch; the **retrocompat `-config` shim** (`korvun -config x.json` boots the
> SAME path as `korvun serve --config x.json`, **byte-identical** to before);
> a TTY-gated placeholder logo banner to stderr. `config`/`status` are announced
> in help but land in later sub-phases; ANSI styling (violet identity, ADR-0030)
> is integrated per command as its output is born. Serve seam: the pre-CLI main
> boot body moved verbatim to `internal/cli/serve.go` as `serveMain` (parses
> `-config` with a local FlagSet, returns an exit code, slog untouched).
> `make quality` green `-race`, total 93.0%.
>
> **Coverage decision (closed, do not reopen):** `internal/cli` ~70% is ACCEPTED
> for SP1 — the shortfall is entirely the relocated boot glue `serveMain` (still
> exempt as un-unit-tested entry-point glue, covered by `internal/app` e2e). The
> master doc's ≥85% bar is for the **core** packages (policy/router/envelope/
> brain); the dispatch/version/help surface is ~100%. Classified in an **ADR-0017
> addendum (2026-07-12)** + the design spec's Success Criteria. **SP2** makes
> `serve` unit-testable and clears `internal/cli` ≥85.
>
> **ldflags: PARKED (its own sub-phase).** `.goreleaser.yaml` still targets
> `-X main.version`; now that `version` lives in `internal/cli`, that must move to
> `-X …/internal/cli.version` — **not touched** here. Impact today: nil (no real
> release tag pushed; a local build reports `korvun dev (rev)` correctly). Noted
> in the `version` godoc.
>
> **NEXT STEP: Piece 3 sub-phase 2 (SP2) — `serve` gets its own flag surface**
> with injectable writers (so the serve path becomes unit-testable and
> `internal/cli` clears ≥85), plus its styling. See the design spec
> (`docs/superpowers/specs/2026-07-12-piece-3-cli-design.md`), the 5 sub-phases.
>
> **Brand assets added this session** (`chore(brand)`): see the "Brand assets"
> section below (`assets/brand/`).
>
> **LOCAL uncommitted changes pending Chano's decision (NOT the CLI piece, left
> OUT of both commits on purpose — reported, not reverted):** `.gitignore` (adds
> `graphify-out/`, `.ua/`, `.obsidian/` ignores) and `CLAUDE.md` (adds the
> graphify "consult FIRST" rule section). These are graphify / understand-anything
> tooling setup, unrelated to the CLI. Chano decides whether to commit them.
>
> **PREVIOUS (2026-07-11): master was at `1f7c18f`** — this session advanced **Piece 2
> (production error handling)**, the last open V1 criterion ("survives a down provider").
> Done today, **docs + one ADR draft only, NO code**: (1) `/plan-eng-review` on the
> framing → 10 findings (4 P1), in `docs/notes/piece-2-framing.md`; (2) **F6 RESOLVED on
> hardware** (Chano's Mac, Ollama 0.30.8, llama3.2:1b): a client disconnect DURING the
> model load makes Ollama **ABORT the load** (`aborting load`, 499) — so retry does NOT
> fix cold start; the fix is a generous per-attempt timeout + optional boot warmup;
> (3) **ADR-0031** (resilience: timeouts + retry + degradation) written as a **draft
> (`status: proposed`)**, copilot-validated; (4) **second voice** (Claude adversarial
> subagent — the project's documented fallback, Codex not installed) run on ADR-0031 →
> **3 findings copilot-approved, PENDING ABSORPTION** into the ADR; (5) the **4 pieces
> (2→3→4→5) recorded as Chano's declared beta goal** (commit `1f7c18f`). **Next session's
> LITERAL next step: ABSORB the 3 second-voice findings into ADR-0031** (spelled out in
> "Notes for the next session"), then final copilot review → ADR to `accepted` → TDD from
> sub-phase 1. `make quality` still green on the last merged master (nothing compiled this
> session). The `ollama serve` started for the F6 test was killed; nothing left running.
> The block below is retained as history.
>
> **PREVIOUS (2026-07-05): master was at `60e79df`** — the public master now includes
> **Phase 2a** (config mutation + auth, **PR #6**) + **Phase 2b** (the no-code builder
> UI, **PR #7**, `442f7ea`) + **Piece 1** (user documentation + installation validation,
> **PR #8**, merge commit `60e79df`, merged by Chano on GitHub). **Piece 1 is CLOSED and
> MERGED** — the macOS install path AND the full quickstart were **validated on real
> hardware** (iMac Intel, macOS 13, Ollama `llama3.2:1b`): a Telegram message received
> the **local model's** reply, **zero cloud**. **Next decided work: Piece 2 (production
> error handling), BEFORE the new CLI piece** — see the ROAD TO BETA block under "Notes
> for the next session". Piece 2 is **framing-pending** (Chano stopped before starting
> its framing); its motivation is already demonstrated (the cold-start Ollama timeout,
> recorded in `ROAD-TO-BETA.md`). `make quality` green `-race` on the merged master
> (total 92.6%), `go.mod` still 3 direct deps (the frontend toolchain is build-time only,
> never in the Go module graph). The two OPTIONAL structural follow-ups remain (nested
> `go.mod` under `web/builder`; the 3 `internal/app` e2e tests → ephemeral port). The
> Stage-15 pointer below is retained as history.
>
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

## Brand assets (2026-07-12)

Korvun's logo was decided on **2026-07-12**: a single **"K terminal"** mark — the
letter **K** knocked out of the rounded tile, a nod to a shell's `|<`. Sources live
in **`assets/brand/`**:

- `korvun-logo-hero.svg` — teal `#2BC8B7` → violet `#7A5AF5` gradient (hero
  signature only; ADR-0030 reserves the gradient for identity moments).
- `korvun-logo-mono-violeta.svg` — monochrome (delivered by Chano).
- `korvun-avatar-512.png` — 512×512 avatar.
- `README.md` — the full brand note.

**Open question (Chano's call, NOT to be resolved):** the mono was delivered with
`fill="#6E56CF"`, different from the identity violet `#7A5AF5` — intentional flat-
ink violet, or correct it? Kept as delivered.

**Pending:** derive the CLI header ASCII art (`internal/cli`, today a placeholder)
from this logo; GitHub social preview; upload the avatar (Chano, via web).

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

## Load-bearing principles (learned in Phase 2a — do not repeat)

> **Read this like the non-negotiable rules.** These three principles were paid
> for by `/review` findings during Phase 2a. They persist across sessions so the
> same class of mistake is not made twice. They are about ENGINEERING DISCIPLINE,
> not any one file.

**PRINCIPLE 1 — TRUTHFULNESS: the code, its tests, and its documentation must tell
the SAME story.** When they diverge, that divergence is a FINDING, not a detail. Two
faces of the one rule, both caught by `/review` this phase:

- **(a) A test of a load-bearing property MUST BITE when the property is violated.**
  Prove it by injecting the regression, seeing red, and reverting. A green test that
  does not bite is proof of nothing. (Caught 3 times in Phase 2a: Unit A's "no
  workers" guard asserted a property it did not actually guard; B2 asserted more than
  it proved; Unit B's crash-loop P1 passed green because `B3b` did not wire the
  persist recorder, so the test never saw the bad on-disk config.)
- **(b) A comment/godoc must NOT claim a behavior the code does NOT have.** (Caught in
  Unit C: `ErrShuttingDown`'s godoc claimed a "503 on shutdown" that did not exist —
  the real path returns a generic 500 and is never even reached. The real behavior was
  safe; the documentation lied.)

**PRINCIPLE 2 — WHEN A `/review` FINDS A DIVERGENCE, THE ORDER IS FIXED:** (1) FIRST
determine, WITH EVIDENCE (never for convenience), whether the REAL behavior is
correct/safe; (2) ONLY THEN choose the fix. If the behavior is correct but the doc or
the test lies, correct the doc/test so it tells the truth — and "correct" means leave
it honest AND COMPLETE (name the edge case as known-behavior / caveat, the way the ADRs
name their caveats), never quietly delete the inconvenient part. If the behavior is
INcorrect, fix the code. NEVER use "fix the comment" as a shortcut to avoid fixing a
behavior that is genuinely wrong. C12 was documentation-only PRECISELY BECAUSE the
review first PROVED the reload-in-shutdown rejection is safe (nothing is persisted,
nothing leaks, the handle is not observable) — that proof is what unlocked the
document-it option.

**PRINCIPLE 3 — DO NOT TOUCH DELICATE CODE WITHOUT A REAL BENEFIT:** guarantees by
construction beat fragile defenses layered over sensitive code. When a fix would
perturb stabilized concurrency (e.g. the supervisor's shutdown ordering / cutover swap,
whose `-race` we verified 20/20 twice) for a marginal benefit over an already-inocuous
case, prefer NOT touching the code and documenting the limit instead. (Applied in C12:
the shutdown ordering was not moved to manufacture a 503 for a safe edge case.)

---

## Ideas aparcadas (post-beta, SIN compromiso — proponer cuando toque)

> Candidatos evaluados pero **fuera de scope hoy**. No son trabajo pendiente ni
> parte de ROAD-TO-BETA; son semillas para proponer más adelante, cada una con su
> propio encuadre + `/plan-eng-review` + ADR si se activa.

- **Texto propio para el desacuerdo de consenso (fallback) — semilla, ADR-0031
  sub-fase 7.** Hoy un `ErrNoConsensus` (los proveedores respondieron pero sin mayoría)
  cae al `defaultFallback` genérico, porque "provider unavailable" sería falso. Un texto
  dedicado ("los modelos no llegaron a acuerdo…") sería más preciso, pero es **decisión de
  voz de Chano** y no cierra ningún criterio: se propone post-beta cuando toque.

- **Capa de voz / STT-TTS (candidato) — herramienta: Voicebox
  (`jamiepine/voicebox`, MIT, local-first, expone REST local `127.0.0.1:17493`
  + servidor MCP).** Evaluado 2026-07-12 a petición de Chano.
  - **Encaje conceptual, NO integrar ahora.** Korvun es hoy gateway de
    mensajería + router + orquestador; voz no está en la misión actual ni en
    ninguna pieza abierta (vamos por Pieza 2 / ADR-0031 sub-fase 4).
  - **Caso de uso más natural = transcripción de notas de voz entrantes.** Flujo
    propuesto: **voz de Telegram → Voicebox STT (Whisper local) → router → brain
    → (opcional) TTS con voz clonada → respuesta**. Encaja con la postura del
    proyecto: 100% local, sin claves de API de terceros, expuesto por REST/MCP
    (coherente con "secrets solo por env, nada a la nube", ADR-0010 §3).
  - **Qué haría falta si se activa:** ADR propio (nueva dependencia externa →
    justificación + verificación Context7/al-source por regla de CLAUDE.md);
    decidir si es proceso externo (REST/MCP) o embebido; contrato de canal para
    audio entrante/saliente en el Envelope (hoy solo texto); coste/latencia STT
    en hardware objetivo (desde Raspberry Pi a la nube — Voicebox necesita
    modelos y VRAM/CPU no triviales, verificar en el SO/HW más pobre del target).
  - **Cuándo proponerlo:** después de la beta (las 4 piezas), o antes si aparece
    demanda real de mensajes de voz. Recordatorio a Claude Code: **proponer este
    flujo cuando se considere conveniente**, no antes.

## Notes for the next session

- **ROAD TO BETA:** el plan de las piezas que faltan vive en
  [`docs/ROAD-TO-BETA.md`](./ROAD-TO-BETA.md). Estado a **2026-07-11**: **Pieza 1
  (docs+instalación) CERRADA/MERGEADA**. **Chano declaró las CUATRO piezas (2→3→4→5) como
  objetivo de beta** (commit `1f7c18f`), en **orden secuencial estricto, una a una**. No
  cambia los *criterios* V1 (solo la Pieza 2 cierra el 6º; 4 y 5 siguen siendo *más
  alcance*, con el aviso del doc maestro §9 de que WhatsApp es opcional/traicionera) — es
  compromiso de *ejecución*. Completar las 4 es un compromiso de **varias sesiones**; cada
  una es fase de peso (encuadre + `/plan-eng-review` + ADR + TDD). **Sin prisa.**

- **PIEZA 1 (documentación de usuario + validación de instalación) — CERRADA / MERGEADA
  a master vía PR #8** (merge commit `60e79df`, merged by Chano on GitHub 2026-07-05).
  Entregó: guía de instalación por SO (`INSTALL.md`), quickstart cero-a-mensaje
  (`QUICKSTART.md`), `BUILDER.md`, sección "Updating Korvun", y el bloque `admin`
  documentado en `CONFIGURATION.md`. **Validado en hardware real** (iMac Intel, macOS 13,
  Ollama `llama3.2:1b`): la ruta macOS + el quickstart completo end-to-end — un mensaje
  de Telegram recibió la respuesta del **modelo local, cero nube**. Solo docs, sin código.
  Linux/Windows escritas por analogía y **marcadas no-verificadas**.

- **PIEZA 2 (manejo de errores de producción) — ✅ CERRADA. ADR-0031 ACEPTADO +
  Closure (2026-07-12); las 7 sub-fases TDD HECHAS y comiteadas.** Cierra el 6º y
  último criterio V1 **"aguanta un proveedor caído sin caerse"** → **Korvun cumple los
  6 criterios V1.**
  - **CIERRE 2026-07-12 (última) — sub-fases 5, 6 y 7 HECHAS y comiteadas:**
    - **Sub-fase 5 (`db8d79b`) — invariantes cold-start de Chano (F6) como guards
      permanentes** (`internal/app/coldstart_test.go`): timeout generoso deja completar la
      carga fría (1 hit, sin retry); timeout corto NO se rescata con retry (deadline no
      reintentable, 1 hit — el "aborting load" de Ollama en test); rechazo rápido 503 SÍ se
      reintenta. El hallazgo de hardware convertido en contrato. Nacieron verdes (el
      decorador ya existía) — guards de invariante.
    - **Sub-fase 6 (`7ea1614`) — warmup opcional best-effort de modelos locales** vía el
      modelo DECORADO (`ModelConfig.Warmup`; cloud+warmup falla en Validate). Lanzado desde
      **Start** (no Run) para que el arranque vía supervisor (ADR-0027) también caliente;
      goroutines en paralelo, dedup por (provider,baseURL,modelID), `warmupDone` para que
      Shutdown espere el unwind. F6 gratis (el decorador no reintenta el deadline). Fallo de
      warmup jamás fatal (WARN, boot sigue).
    - **Sub-fase 7 (`cd32fb8`) — fallback diferenciado + métricas de retry + F8.** Fallback
      de **3 salidas** (retry-soon / unavailable / genérico-en-no-consenso): `ErrNoConsensus`
      usa el genérico porque "unavailable" sería falso (los proveedores respondieron) — la
      3ª salida la forzó `TestOrchestrator_optionGuards`. Clasificación en el Orchestrator
      con sentinels de `model` + `context` (SIN importar `retry` — frontera intacta).
      Métricas `korvun_provider_retries_total` / `_retry_budget_exhausted_total` (label
      provider) emitidas desde el decorador (`retry.WithMetrics`). F8: `ObserveProviderDuration`
      documentada como TOTAL incl. reintentos+backoff (por construcción, pinneado en `fanout`).
    - `make quality` verde `-race` tras cada sub-fase; cobertura core ≥90.
  - **RESOLVED (was a blocker):** govulncheck GO-2026-5856 (crypto/tls ECH advisory,
    stdlib) failed the Quality Gate on ubuntu+macos after the 0a9cee4 push. Fixed
    by bumping the go directive in go.mod 1.26.4→1.26.5 (commit 63d60f1); CI green
    on all 3 OSes. Not a code defect — stdlib advisory under our normal HTTPS
    calls. Local toolchain auto-upgraded to 1.26.5; Chano's Homebrew base still
    1.26.4 (optional `brew upgrade go` to align, non-blocking).
  - **ACTUALIZACIÓN 2026-07-12 (última) — sub-fase 4 HECHA y comiteada (`b6631d4`, SIN
    push — es de Chano):** el decorador de retry (LA GORDA de la Pieza 2).
    - **`internal/model/retry` (paquete nuevo):** `New(inner, Config{PerAttempt,
      MaxRetries}, WithClock/WithRand)`; per-attempt `context.WithTimeout` en CADA intento
      (incl. el 0º, retry on/off) → dueño ÚNICO del deadline para TODOS los shapes (SV3
      final). Clasificación en orden load-bearing R1(parent ctx→stop, F3)→R2(DeadlineExceeded
      →stop, F6)→R3(RateLimit, cap 30s + budget-guard sin dormir)→R4(Unavailable, full jitter
      200ms×2 cap 2s)→resto no-retryable. Reloj+rand inyectables (cero sleeps en tests);
      default `math/rand/v2` concurrent-safe. Cobertura 96.3%, `-race -count=20` limpio.
    - **Wiring (`buildCatalog`):** `WithModelID(retry.New(adapter, {PerAttempt:
      EffectiveRequestTimeout(m), MaxRetries: efectivo}), id)` para TODOS los shapes;
      `effectiveMaxRetries` fuerza 0 en sequential (SV2) y con `retry:false`; nil→on.
    - **Des-wire del agente (D-agent):** `buildAgentBrain` ya NO cablea
      `WithAgentPerModelTimeout` (opción conservada en `brain` para uso directo/test, godoc
      lo dice); el ceiling del agente deriva de `EffectiveRequestTimeout(bc.Models[0])`; el
      fan-out puebla `backoffBudget = maxRetries × 2s` (FR-A3).
    - **Bite-proofs demostrados (inyectar→rojo→revertir):** BP-orden (invertir R2/R4 → el
      load-bearing cae, calls=3) y BP-a (quitar el guard sequential → el guard SV2 de wiring
      cae, hits=3). Test de composición F3 (fan-out cancel + decorados) con fakes,
      determinista. `make quality` verde `-race`. Spec en
      `docs/superpowers/specs/2026-07-12-adr-0031-subphase4-retry-decorator-design.md`;
      nota intermediate-state del ADR-0031 cerrada a estado final en este mismo commit de docs.
  - **ACTUALIZACIÓN 2026-07-12 — sub-fase 3 HECHA y comiteada (`e1925a6`, SIN
    push — es de Chano):** refinamiento del mapeo de errores de Ollama (COMPLETITUD F9).
    - **`ollama.mapHTTPError` (aislada, table-tested):** `5xx→ErrProviderUnavailable`,
      `429→*RateLimitError{Provider:"ollama", RetryAfter=ParseRetryAfter(Retry-After)}`,
      `401/403→ErrAuthInvalid` (Ollama es sin auth; vendrían de un proxy delante, retry
      no ayuda), resto `4xx→ErrProviderResponse`; `Generate` sustituye su rama inline
      no-2xx por `mapHTTPError(resp)`; snippet del body capado a `maxErrorBodyBytes`.
    - **`model.ParseRetryAfter` compartida (D2):** extraída a `internal/model` (godoc
      honesto: solo segundos, NO HTTP-date, ni Ollama ni Groq lo usan hoy); **Groq
      migrado** a usarla (move MECÁNICO, sin cambios de comportamiento — ningún test de
      groq distinto del `TestParseRetryAfter` movido necesitó tocarse).
    - Cobertura `ollama` 96.5% / `groq` 94.1% / `model` 100%; `make quality` verde
      `-race` sobre todo el árbol. Spec de diseño versionado en
      `docs/superpowers/specs/2026-07-12-adr-0031-subphase3-ollama-error-mapping-design.md`.
  - **ACTUALIZACIÓN 2026-07-11:** **sub-fases 1 y 2 en verde y comiteadas en
    local (SIN push — es de Chano):**
    - **Sub-fase 1 (`b00926d`) — jerarquía de timeouts + config per-modelo + ceiling
      derivado (Decisiones 2 y 3).** `ModelConfig.RequestTimeout`/`MaxRetries`,
      `BrainConfig.Retry`, top-level `request_timeout`/`brain_handler_timeout`; el app
      DERIVA el ceiling por brain (fan-out `max_i` / sequential `Σ` / agent
      `maxIterations`, `defaultCeilingMargin = 500ms`) e instala
      `WithBrainHandlerTimeout(max)`, override `≥ derived` o falla ruidoso; retirada la
      doble aplicación del timeout (coordinator + adapter) — **SV3 verificado: ningún
      path sin deadline** (el AgentBrain conserva `WithAgentPerModelTimeout`; el resto
      hereda el ceiling del router).
    - **Sub-fase 2 (`03901a6`) — cancelación fan-out al primer éxito usable + carve-out
      de consenso (SV1, Opción A).** `fanout.WithCancelOnFirstUsableSuccess()` (opt-in;
      default wait-all); `buildCoordinator(dispatch, policyKind)` cablea priority→cancela,
      consensus→wait-all; el coordinator NO importa `policy`. Bite-test del carve-out
      DEMOSTRADO que muerde; `-race -count=20` limpio.
    - Registro en ADR-0031 (margin + nota SV3 del estado intermedio) va en este mismo
      commit de docs. `make quality` verde `-race` tras cada sub-fase.
  - **PIEZA 2 CERRADA — no queda trabajo aquí.** El PRÓXIMO PASO del proyecto es la
    **PIEZA 3 (CLI)**; ver su bloque abajo.
  - **ACTUALIZACIÓN previa 2026-07-11:** los **3 hallazgos de la 2ª voz (SV1–SV3)
    están ABSORBIDOS en el ADR-0031 y comiteados** (commit `docs: absorb second-voice
    findings SV1-SV3 into ADR-0031`); tras la **revisión final del copiloto**, el
    **ADR-0031 pasó a `accepted`**. El encuadre y los 3 hallazgos de abajo se conservan
    como HISTORIAL (ya no son trabajo pendiente).
  - **Motivación DEMOSTRADA en hardware + F6 RESUELTO:** el timeout Korvun→Ollama en frío
    (~5s < carga del modelo) hace fallar el primer mensaje; y la incógnita F6 quedó
    resuelta en el Mac de Chano — **al desconectar durante la carga, Ollama ABORTA la
    carga** (`aborting load`, 499). Consecuencia: **el retry NO salva el arranque en
    frío** (re-dispara y re-aborta la carga, desperdicia CPU); lo arreglan el **timeout
    generoso + precalentado**. Detalle en `docs/notes/piece-2-framing.md` (F6, 10/10).
  - **LOS 3 HALLAZGOS DE LA 2ª VOZ A ABSORBER (para no perderlos):**
    1. **[P1] Amplificación del ceiling en fan-out + contradicción interna del ADR:** el
       fan-out corta al primer éxito, pero el ceiling se derivaba asumiendo que *todos*
       agotan sus reintentos → ceiling de hasta **~20 min**. Además el AgentBrain (3er
       shape) hace N llamadas por `Handle` y el ADR no lo modela. **RESOLUCIÓN:** el
       fan-out **CANCELA a los restantes al primer éxito usable** (`context`), y el ceiling
       se deriva del **PEOR MODELO INDIVIDUAL, no de la suma** (baja a ~2 min) — cierra F2
       **por construcción**. *Test:* fan-out rápido-OK + lento-que-reintenta → `Handle`
       vuelve con el rápido y el lento se cancela.
    2. **[P2] Doble reintento en sequential:** retry-por-modelo × avance-al-siguiente se
       multiplican. **RESOLUCIÓN:** retry-por-modelo **DESACTIVADO en sequential** (el
       sequential YA es el fail-over). *Test.*
    3. **[P3] Verificado:** quitar `WithPerModelTimeout` + `WithRequestTimeout` **no deja
       ningún path sin deadline** (el per-intento del decorador lo cubre — aplicado en
       TODO intento, incluido el 0º, con retry on/off). **Dejar constancia explícita en el
       ADR** (y cubrir el path del AgentBrain, que llama `fanout.CallOne` directo).
  - **DECISIONES DEL ADR-0031 YA VALIDADAS (NO cambian al absorber):** arranque en frío =
    **timeout generoso obligatorio + precalentado opcional** (`keep_alive` desaloja el
    modelo a los 5 min → precalentar solo no basta); **ceiling del router DERIVADO**
    (dispatch + per-modelo + reintentos), **NO expuesto como knob** (evita reproducir el
    bug de hoy — garantía por construcción), override manual solo si **≥ derivado**, falla
    ruidoso si no; **timeout de config PER-MODELO** (`ModelConfig`, candidato
    `request_timeout`) con default top-level; **jerarquía colapsada a 2 capas** (per-intento
    en el decorador + ceiling en el router), eliminando el `WithPerModelTimeout` del
    coordinator y el `WithRequestTimeout` doble del adapter; **retry SOLO transitorios
    post-carga** con **guardia de `ctx.Err()`** (F3) y la distinción **"error antes del
    deadline" (reintenta) vs "deadline expiró" (no** — mantiene F6 fuera del retry);
    **circuit breaker DIFERIDO post-beta** (reconociendo que **F2/F7 son su coste real**,
    no YAGNI puro); **métricas de retry por proveedor**; **cero dependencias** (stdlib).
  - **Nota de alcance:** la **re-clasificación del breaker en el checklist de la Pieza 2**
    de `ROAD-TO-BETA.md` se hará **al CERRAR la Pieza 2** (no un commit suelto ahora).
  - Artefactos de la pieza: **ADR-0031** (`docs/adr/0031-resilience-timeouts-retry-and-degradation.md`,
    proposed) + **notas** (`docs/notes/piece-2-framing.md`: encuadre + F1–F10 + F6
    verificado + los 3 hallazgos de la 2ª voz).

- **PIEZA 3 = CLI (subcomandos estilo git/docker) — ENCUADRADA y APROBADA por el copiloto;
  ES EL PRÓXIMO PASO ahora que la Pieza 2 está cerrada.**
  - **PRÓXIMO PASO LITERAL de la próxima sesión:** arrancar la Pieza 3 por su **design
    spec** (el encuadre ya está aprobado; el resumen de abajo lo conserva).
  - **REQUISITO DE CHANO (2026-07-12) — acabado profesional y elegante:** la CLI debe tener
    un pulido visual de primera; **referencia visual = la CLI de OpenClaw** (verificar su
    look REAL **en la fuente** al arrancar la pieza, no de memoria — regla External-docs de
    CLAUDE.md). La tensión **color/estilo vs disciplina zero-deps** (¿stdlib ANSI a mano vs
    una lib de estilo?) **se decide en el design spec** — posible **ADR de dependencia** si
    una lib cruza el listón; por defecto zero-deps salvo justificación.
  - Resumen del encuadre aprobado (para no perderlo):
  - **Subcomandos** `korvun serve` / `config check` / `status` / `version` / `help`, en
    **stdlib (`flag.NewFlagSet`), NO Cobra** — set pequeño y estable, disciplina zero-deps
    (3 deps hoy), mandato del maestro "stdlib si es razonable"; Cobra no cruza el listón
    (YAGNI). Estructura en un paquete `internal/cli` con `Run(args, stdout, stderr) int`
    (testable; `main.go` queda de 3 líneas).
  - **ADR corto de CONTRATO de interfaz** (no de dependencia): fija el set de subcomandos,
    la retrocompat, y las convenciones de exit code (0 ok / 1 fallo / 2 uso).
  - **Shim de retrocompat (~5 líneas):** `korvun -config x.json` sigue funcionando
    (= `serve` implícito) para **no invalidar la doc/systemd recién validados en
    hardware**; forma canónica nueva `korvun serve --config` (stdlib `flag` acepta `-` y
    `--` igual, gratis).
  - **`config check`:** split **offline `config.Validate()` por defecto** + **`--preflight`
    online** (reusa `app.Preflight`: getMe + secretos + selector de privacidad).
  - **`status` = cliente HTTP fino de la read-only control API YA existente**
    (`GET /api/brains` + `/api/channels` + `/healthz` en `127.0.0.1:2112`; flag `--addr`;
    **sin token**; **cero código de servidor nuevo**; fallo honesto si el admin está off).
  - **Logo ASCII** `[placeholder]` a **STDERR nunca stdout** (no contaminar salida
    machine-readable); el arte concreto se decide aparte.
  - **5 sub-fases TDD** (una a una, cada una con su `/review` + `make quality -race`):
    (1) scaffold+dispatch+`version`+logo · (2) `serve`+shim · (3) `config check` ·
    (4) `status` · (5) **docs-update + re-validación macOS**. **La sub-fase 5 DEBE
    actualizar `INSTALL.md`/`QUICKSTART.md`/`BUILDER.md`/`korvun.service` de
    `./korvun -config` a `korvun serve …` y re-validar en el Mac de Chano.**
  - **Prioridad:** la CLI **NO cierra ningún criterio V1** (es DX/pulido). La Pieza 2
    mantiene mayor prioridad de *criterio* (cierra "aguanta proveedor caído"); la CLI
    tiene mayor prioridad de *timing* que las piezas 4-5 porque **reescribe la doc de la
    Pieza 1** (el shim evita que se rompa, solo deja de ser canónica).

- **FOLLOW-UPS abiertos (recordar):**
  - **(a)** `korvun.example.json` en el **paquete de release** (GoReleaser `archives.files`)
    — el release no trae config de ejemplo → el usuario adivina el formato y falla (Chano
    lo vivió). Cambio de empaquetado, **no docs**; candidato junto a la Pieza 2 o cuando se
    toque empaquetado. *(La sub-fase 5 de la CLI también añade un `korvun.example.json` al
    repo como referencia — coordinar.)*
  - **(b)** **7 TODO-VERIFY** en la doc: **5 en la sección Windows de `INSTALL.md`**
    (`curl.exe`; `Get-FileHash` + compare; cosign en Windows; wording de SmartScreen;
    sintaxis `$env:` de PowerShell) + **2 en `BUILDER.md`** (`/builder` sin barra final
    ¿redirige?; el cleartext gate ¿avisa o bloquea?). Se cierran con acceso a esos
    SO/navegador.
  - **(c)** ✓ **HECHO esta sesión:** Mac al día (`master` en `60e79df`) + rama local
    `feat/user-docs` borrada (fully-merged). La rama **remota** `origin/feat/user-docs`
    sigue en GitHub (prune = decisión de Chano).
  - **(d)** Los 2 follow-ups estructurales viejos: `go.mod` anidado en `web/builder`
    (excluir `node_modules` por construcción); los 3 tests e2e de `internal/app`
    (`TestControlAPI_endToEnd`, `TestLiveView_endToEnd`, `TestRunShutdown_lifecycle`) a
    puerto efímero `127.0.0.1:0`.

- **PHASE 2b (the no-code builder UI — React/TS/Vite) — COMPLETE / MERGED to master
  via PR #7** (merge commit `442f7ea`, merged by Chano on GitHub 2026-07-05; master
  now includes Phase 2a via PR #6 + Phase 2b via PR #7). **The builder feature is CLOSED
  — no feature work pending; only the two OPTIONAL structural follow-ups below remain.**
  The visual builder that Phase 2a's mutation surface was built for. Chano
  approved the violet look and the edit flow after seeing it live, so 2b.3 was
  CONSERVATIVE polish, not a redesign.
  - **ADRs (accepted directly on master):** `ADR-0029` (frontend toolchain) + `ADR-0030`
    (visual identity + UI architecture) went to **accepted** after a `/plan-eng-review`
    that caught **3 spec-level P1s** (all resolved in the ADR text BEFORE accepted):
    **(a)** `go:embed` build-ordering + a placeholder `dist/.gitkeep`+stub so
    `go build` / `make quality` / cross-compile ×6 / release never break in a clean
    clone; **(b)** no-CDN enforced by the NETWORK LAYER (CSP `default-src 'self'` from the
    Go `/builder` handler + a Playwright no-same-origin assertion), the grep left
    advisory; **(c)** `GET /api/config` gated (same bearer) for the edit round-trip —
    exposes env-var NAMES, never values (`os.Getenv` is not called).
  - **2b.0 Go hardening (`f0366f7`):** empty-token guard in `bearerAuth` (closes the
    `sha256("")` bypass footgun) + `http.MaxBytesReader` 1 MiB on `POST /api/config`.
    Both TDD red-first, **proven to bite** (red with 202, fix, green 401/413;
    `callCount==0` after the fix).
  - **2b.1 scaffold + design system (`32dc879`) + reconciliations (`32b8857`, ADR-0029
    to npm):** React 19.2.7 / Vite 8.1.3 / Tailwind 4.3.2 / TS 5.9.3 — all versions
    pinned EXACT via Context7 (patch, not `^`; Tailwind v4 the single innovation token
    with a pre-agreed fallback). Design system in `src/design/tokens.ts` (functional
    violet accent OUTSIDE the event palette; `received/sent/dropped/failed` semantics
    verbatim from `/ui`; iridescent teal→violet identity-only). OFL fonts
    (Archivo / IBM Plex Sans / IBM Plex Mono) EMBEDDED, no CDN, licenses in-repo.
    `go:embed` of the dist with placeholder + build-ordering; CSP `default-src 'self'`
    (the real no-CDN gate); `/builder` conditional on the bearer; `GET /api/config`
    gated (seam `Reloader.CurrentConfig()`); WCAG AA by Vitest over the token table
    (proven to bite). Light + dark from day one.
  - **2b.2 edit surface (`bd439da` 2b.2a · `f6569a6` 2b.2b · `421045c` 2b.2c + `f01c5ab`
    remove-brain):** read → edit → POST round-trip; the reload state machine verbatim
    from `supervisor.State` (pending → cutover-in-progress → succeeded/rolled-back/failed,
    ECONNREFUSED-during-cutover = RETRY not failed, form-lock total during cutover);
    error/edge states (400 field-mapped inline, the two 409 distinguished, empty/first-run,
    dirty+Discard confirm, 401→re-auth clears the in-memory token); Playwright e2e with a
    mock control API (happy flow + rejections + ECONNREFUSED-in-cutover + axe-core +
    no-same-origin assertion that BITES). remove-brain closed the add/edit/remove symmetry
    gap Chano hit live (empty-state on removing the last brain).
  - **2b.3 polish (`02698c3` header · `c24bc4e` save-bar · `4d6fd3f` chevron · `24866da`
    microinteractions):** Chano tested the UI and APPROVED the violet look + the flow →
    CONSERVATIVE polish, NOT a redesign. **3 functional fixes** (header reads "builder"
    not "read-only" — the UI does not lie; sticky save-bar no longer occludes the last
    content; ▾ chevron on selects via data-URI, no external request). **4 microinteractions**
    CSS + View Transitions API, **NO Motion** (the reload state transition the highest
    value; enter/exit; hover; focus stays instant). **reduced-motion test that BITES**;
    the global `prefers-reduced-motion` guard covers the new transitions.
  - **Final verification:** **60/60 vitest + 6/6 e2e + `make quality` green `-race`, total
    94.0%, `controlapi` ≥90%.** `go.mod` STAYS at **3 direct deps** (the frontend is
    build-time; `builderui` is stdlib-only). The polish is ADDITIVE: it does NOT touch the
    `fieldset disabled` form-lock or the reload logic. The 2b.2 tests stay intact.
  - **MILESTONE:** Chano SAW the builder working end-to-end locally (`make build` + the real
    binary, not a mock): edited a brain's dispatch fanout→sequential, Save and reload, watched
    the banner go pending→cutover (form locked)→succeeded, and the binary rewrote
    `configs/dev.local.json` on disk (`dispatch: sequential`) — Phase 2a's hot config mutation,
    TRIGGERED FROM THE PHASE 2b UI, with zero external requests. The two halves of the builder
    working together.
  - **Branch `feat/builder-ui` — MERGED to master via PR #7** (merge commit `442f7ea`).
    The branch carried **16 commits** = 2 docs (ADR-0029+0030 accepted `84ec176` + HANDOFF
    `c1c3d7f`, the Phase 2b prerequisite, self-contained in the PR) + 14 builder commits:
    `f0366f7` 2b.0 · `32dc879` 2b.1 · `32b8857` reconciliations · `bd439da` 2b.2a ·
    `f6569a6` 2b.2b · `421045c` 2b.2c · `ff1a566` dev proxy · `02f4b30` trim token ·
    `f01c5ab` remove-brain · `02698c3` header · `c24bc4e` save-bar · `4d6fd3f` chevron ·
    `24866da` microinteractions · `bdc9f7f` HANDOFF. The local branch was deleted after
    the merge (fully-merged); `origin/feat/builder-ui` still exists on GitHub (the merge
    did not auto-delete it — Chano's call to prune the remote).
  - **DONE (this session): PR of `feat/builder-ui` to master.** Claude Code ran
    `make quality` (green `-race`), pushed the branch, and opened PR #7 "Phase 2b: no-code
    builder UI"; Chano reviewed and **merged it on GitHub** (merge commit `442f7ea`,
    preserving all 16 commits). The Mac was then synced to `442f7ea` and `make quality`
    re-run green on the merged master (total 92.6%) as a merge sanity check.
  - **OPTIONAL structural FOLLOW-UPS (NOT urgent, NOT feature — the only work left after
    2b, each its own change + verification):** (1) nested `go.mod` in `web/builder` (move
    `embed.go` → `internal/builderui`) to exclude `node_modules` from Go tooling BY
    CONSTRUCTION, dropping the manual Makefile filter (Principle 3); (2) the 3
    `internal/app` e2e tests (`TestControlAPI_endToEnd`, `TestLiveView_endToEnd`,
    `TestRunShutdown_lifecycle`) to `observability.addr="127.0.0.1:0"` (ephemeral) — today
    they bind the fixed 2112 and collide with a local Korvun on that port (it hit Chano a
    prior session; port was free this session so no collision).

- **PHASE 2a (the builder — config mutation + auth) — CLOSED, MERGED to master via PR #6.**
  (Historical record below; branch was `feat/config-mutation`, since merged. Master is now
  at `3f9d34a`.) All 3 TDD units GREEN and Unit C `/review` (with outside voice) DONE.
  Built in 3 TDD red-first units, each with its own `/review`. `ADR-0027`
  (reload-and-rebuild) + `ADR-0028` (bearer auth) are **accepted** (3 rounds of
  cross-model `/plan-eng-review`) and were **NOT touched** during implementation —
  the code was aligned to the text, never the reverse.
  - **Unit A — effect-free `Preflight` in `internal/app` (`8398a2c`). CLOSED.**
    Validates a config (throwaway `getMe`, secret resolution, privacy selector)
    WITHOUT opening the store or starting workers. Regression guard
    `TestPreflight_neverRegistersOnRouter` **proven to bite** if `wire()`/
    `RegisterChannel` is reintroduced. Duplicate-channel dedupe flagged as a
    follow-up (`c9e3328`, see the deferred-follow-up bullet below).
  - **Unit B — supervisor in `internal/supervisor` (`ae7bf42`+`3661ac7`, redesign
    `c11d118`+`2fc0f88`). CLOSED.** The most dangerous concurrency of the phase. Its
    `/review` caught a **P1 crash-loop** (the self-heal reintroduced a bad config on
    disk on the rollback-fatal path; the test did not see it). **Resolved via
    Option A:** `app.Run` split into `Start(ctx)` (admin bind + channel starts,
    fallible) + `Serve(ctx)` (block until ctx.Done) + `Run = Start+Serve`; the
    supervisor **persists ONLY after a confirmed `Start`**. The invariant "no
    crash-loop; `-config` is never overwritten with a config that cannot come up" is
    now guaranteed **by construction**, no self-heal — guarded by `B3b`
    (`persistCount==0` on the fatal path, **proven to bite**). Load-bearing `B1`:
    `-race` quiesce→rebuild→swap, swap under mutex + concurrent reader,
    `-count=20` → 20/20.
  - **Unit C — endpoint + auth in `internal/controlapi` (`3d9f43f` C-auth +
    `ab08424` C-supervisor). CLOSED (`/review` DONE; contract fix `dc264d7`).** C-auth: `RegisterMutation`
    mounts `POST /api/config` (gated) + `GET /api/reload/{handle}` (status); the gate
    compares a **fixed-length SHA-256** with `subtle.ConstantTimeCompare` (F12, never
    raw tokens); **"no token ⇒ mutation NOT mounted"** (conditional mount in
    `app.Build` on `os.Getenv(cfg.Admin.TokenEnv)`); read-only (`/api/brains`,
    `/api/channels`, `/api/events`, `/ui`) intact without a token; CSRF defended by
    Authorization-header-never-cookie. C-supervisor: late-binding of the supervisor
    in `cmd/korvun`, `WithPreflight(app.Preflight)` wired (Unit B's dead seam, ADR
    step 5: Preflight while the old app STILL serves, failure → status `failed`
    without touching the old app, F7); `B1 -race -count=20` → 20/20 **confirmed after
    restructuring `serve()`** (the `reasonReload` case is byte-identical, the swap
    stays under mutex).
  - **The two Unit-B P3s closed in C:** `PreflightFunc` wired; reload-during-shutdown
    resolved as an `ErrShuttingDown` rejection.
  - **Unit C `/review` VERDICT (cross-model; Codex not installed → Claude adversarial
    subagent as the documented fallback, converged independently):** GATING is COMPLETE
    and hole-free — only `POST /api/config` mutates and it is ALWAYS bearer-wrapped
    (`RegisterMutation`), no CORS anywhere, token read from `Authorization` never a
    cookie. The C12 shutdown race is INOCUOUS: a reload that wins the race against the
    drain gets a 202 but is silently dropped, leaving NO observable handle (persist
    never runs → `-config` intact, no app built → zero leak, `statusHandler` dies in the
    same `shutdownApp` → client cannot poll it). The two P2s were CONTRACT-HONESTY, not
    security: the godoc claimed `ErrShuttingDown` maps to HTTP 503, but it maps to a
    generic 500 AND the HTTP path never even reaches it (admin server is already down
    when `shuttingDown` flips true). **Copilot chose option (a):** correct the
    contract/comment so it stops lying and name the shutdown race as accepted
    known-behavior; do NOT touch the race-verified shutdown ordering (moving the flag +
    adding a 503 branch would perturb the concurrency stabilized after the Unit-B P1,
    `-race` 20/20 twice, for a benign near-unreachable case). Applied doc-only in
    `dc264d7`; `make quality` green (`-race`), coverage 93.2%.
  - `make quality` green, `-race`, **93.3% total**; `internal/controlapi` 96.9%,
    `internal/supervisor` 93.8%; **3 direct deps, none new** (all stdlib + internal).
  - Full trail on `feat/config-mutation`: `8398a2c` (A) · `c9e3328` (dedupe note) ·
    `ae7bf42`+`3661ac7` (B B0-B8) · `c11d118`+`2fc0f88` (B Option-A redesign / P1) ·
    `3d9f43f`+`ab08424` (C).
  - **RECURRING RULE OF THE PHASE (banked):** three times a `/review` caught a test
    that **asserted more than it proved** (Unit A P2, B2 precision, Unit B P1). Every
    test of a load-bearing property MUST BITE when the property is violated — prove
    it by injecting the regression, seeing red, and reverting.
- **NEXT STEP — SUPERSEDED (all items DONE).** Phase 2a was merged to master via PR #6;
  Phase 2b was merged to master via **PR #7** (merge commit `442f7ea`). Both the builder's
  mutation surface and the builder UI are now on the public master — see the **PHASE 2b —
  COMPLETE / MERGED** block at the top of this section. No feature work is pending; only
  the two OPTIONAL structural follow-ups (nested `go.mod`; ephemeral e2e port) remain.
  (Historical: item 1 was "decide the push of `feat/config-mutation`" → done via PR #6;
  item 2 was "Phase 2b framed, implementation started" → now complete, all four sub-phases
  green.)
- **HARDENING deferred to Phase 2b (reported by Unit C's `/review`, one P3 at a time,
  each with its own micro-decision — deliberately NOT folded into the 2a contract fix):**
  1. **Empty-token guard in `bearerAuth` (LATENT FOOTGUN — close FIRST in 2b).**
     `bearerAuth` has no internal guard against `token==""`: if it were ever called with
     an empty token, `want=sha256("")` and an `Authorization: Bearer ` (empty presented
     token) would hash-match → full bypass. Safe TODAY only by invariant
     (`RegisterMutation` is called only when `token != ""`, `app.go:284`, plus F11
     `wouldSelfLock` refusing a config that resolves the token empty). A future SECOND
     caller of `RegisterMutation` without that check would reopen the bypass. Fix: reject
     an empty token at the top of `bearerAuth` (or `RegisterMutation`) so the guarantee
     is by-construction, not by-invariant. Cheap; kills the footgun at the root.
  2. **Request-body size limit on `POST /api/config`.** `mutation.go` decodes `r.Body`
     with no `http.MaxBytesReader`; an authenticated admin could send an unbounded body.
     Low severity (behind bearer auth); wrap the body with `N = 1 MiB`.
  3. **Server-side cleartext gate for `POST /api/config` (candidate hardening of
     ADR-0028, surfaced by the 2b eng-review).** Optionally refuse mutation when bound
     non-loopback without TLS / `X-Forwarded-Proto: https`, so a bearer in the clear is
     rejected server-side, not just warned about. NOT blocking: ADR-0028 §2 F10 already
     delegates cleartext to the operator and ADR-0030's advisory banner covers the UX. If
     taken, it gets its own mini-decision + a test-that-bites in 2b, and lands in
     ADR-0028, not 0030.
  > **NOTE (1 + 2 are 2b.0, done red-first before any frontend line):** items 1 and 2
  > are the Go-only hardening of phase 2b.0; item 3 is a later candidate.
- **Deferred follow-up (own change, NOT 2b.1/2b.2) — node_modules Go-tooling hygiene
  BY CONSTRUCTION.** `web/builder/node_modules` vendors a stray Go package (`flatted`)
  that `./...` picks up, so 2b.1 filters it out of `go test`/`go vet` in the Makefile
  (`GO_PKGS := go list ./... | grep -v /web/builder/node_modules/`). That filter is
  functional but fragile (Principle 3: a manual filter, not a guarantee). The
  by-construction fix: give `web/builder` its own **nested `go.mod`** (moving
  `web/builder/embed.go` → `internal/builderui/` with the dist output relocated to
  `internal/builderui/dist`), so root `./...` skips `web/builder` automatically and
  the Makefile filter can be removed. It is a STRUCTURAL refactor (move the package +
  nested module + re-verify `//go:embed`/build on the 3 OSes + cross-compile ×6), so it
  ships as its own change with its own verification, NOT folded into scaffolding. The
  Makefile filter holds in the meantime.
- **Deferred follow-up (own change, NOT the builder work) — app end-to-end tests bind
  the FIXED default admin port (2112), not an ephemeral one.** `TestControlAPI_endToEnd`,
  `TestLiveView_endToEnd`, and `TestRunShutdown_lifecycle` in `internal/app` build an app
  WITHOUT setting `observability.addr`, so it defaults to `127.0.0.1:2112`
  (`DefaultObservabilityAddr`). If a real Korvun (e.g. a local demo) is already listening
  on 2112, the test's admin bind fails and the tests fail ("admin server never became
  healthy" / "did not start the channel") — surfaced 2026-07-05 while dogfooding the
  builder against a live binary. Not a regression, a pre-existing fragility. By-construction
  fix: set `cfg.Observability = &config.ObservabilityConfig{Addr: "127.0.0.1:0"}` in those
  three tests (ephemeral, as `mutation_mount_test` already does) → immune to a local Korvun.
  Its own change with its own verification, NOT folded into feature work.
- **Deferred follow-up (own fix, NOT Phase 2a) — duplicate channel dedupe.**
  `config.Validate` (`config.go:217`, `validateChannels`) dedupes channels by NAME but
  not by TYPE, and `router.RegisterChannel`/`RegisterBrain` (`router.go:146,189`)
  **silently overwrite** on a duplicate registry key rather than erroring. So a config
  with two channels of the same type/name could pass `Validate` and leak the first
  adapter's worker goroutines inside `wire`. Pre-existing, unrelated to the Phase 2a
  Preflight (surfaced by the Unit A `/review`, 2026-07-04). Fix it in its own change
  (a `Validate` dedupe + a `Register*` duplicate-name error), never folded into the
  builder units.
- **Claude Code skills installed + documented (2026-07-04):** `agent-browser`
  (browser automation — live source/doc verification when Context7 doesn't cover
  something; does NOT relax the Context7-first rule) and `find-skills` (surface
  applicable skills before doing a task by hand). Both are described in CLAUDE.md
  under "Claude Code skills — available tooling", framed as COMPLEMENTS to the
  project method, never replacements. These two are the complete set (confirmed by
  Chano 2026-07-04); no third skill exists to document.
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
