# Piece — Governed tools + skills: Design Spec

> **Status: approved for TDD** (2026-08-09 — the six clarifications resolved by
> the director, plus three director extensions folded in: shadow mode, network
> shield, `/tools`). Review checklist green below.
> Governing ADRs: ADR-0021 (Agents — the `AgentBrain` loop, the `Tool` seam §4,
> the safety invariants §2, the PURE-toolset security decision §8, and its
> explicit deferral: "dangerous / side-effecting tools are deferred behind a much
> higher bar (their own ADR, with sandboxing / allow-listing / consent designed
> first)" — **that ADR is this piece's SP0, ADR-0041**), ADR-0012/ADR-0015
> (policy engine: declared-not-inferred sensitivity, pure pre-dispatch selection,
> `Decision.Provenance` as the audit surface), ADR-0023 (event bus), ADR-0024
> (live view / Activity feed — **§1 metadata-only law intact byte-for-byte**),
> ADR-0018 §6 via ADR-0021 §6 (a tool-use trace is observability, NOT
> conversation memory).
> External-docs note: the **AgentSkills `SKILL.md` format was verified at
> source on 2026-08-09** (<https://agentskills.io/specification>) — the exact
> field set and constraints are recorded in FR-SKILL-1. Context7 does not index
> the spec (only third-party skill collections), so the verify-at-source half of
> the External-docs rule applies. Everything in this piece uses stdlib +
> existing `internal/` packages only — **zero new dependencies** (R-3 resolved
> the YAML question to an in-house flat-subset parser; director-provided parity
> note: OpenClaw uses a single-line frontmatter parser, verified in its docs
> 2026-08-09).

## On-disk verification record (2026-08-09, graph-first then source)

The facts this spec builds on, verified in this session:

1. **Tool seam (`internal/tool`)** — exists exactly as ADR-0021 §4 specifies:
   a leaf package (stdlib-only) with `Tool{Name, Description, Execute}`, a
   load-bearing concurrency contract (N router workers share one instance),
   and `Registry map[string]Tool` read-only after construction.
   `tool.Builtin(name)` is the **single** safe-toolset boundary: only
   `time`/`echo`/`calc` resolve; anything else returns `ok=false`. All three
   built-ins are PURE by deliberate security decision (§8), not omission.
2. **`AgentBrain` (`internal/brain/agent.go`)** — the bounded single-model
   prompt-protocol loop is live: `TOOL: name(args)` / `OBSERVATION:` grammar
   (`protocol.go`), `DefaultAgentMaxIterations = 5`, per-tool timeout, tool
   failure = observation (never fatal), model failure = fallback, stateless
   across N workers, final-pair-only persistence — the trace is discarded
   (§6: if ever wanted, it is observability). The system prompt is built
   **once per Handle** from the full injected registry.
3. **Policy engine (`internal/policy`)** — `SelectModels(catalog, sensitivity)`
   is the whole pre-dispatch surface: a PURE function filtering MODELS by
   per-brain declared `Sensitivity` against per-model declared `Locality`,
   run ONCE at brain construction. **There is NO point today where TOOLS are
   governed — zero references to tools anywhere in `internal/policy`. The
   honest finding is confirmed: the governance surface is purely additive.**
   Sensitivity has exactly two tiers — `public|private` (the builder carries a
   tripwire test forbidding invented tiers) — so "network shield" semantics
   attach to `Private`, and every non-Private brain gets allow-list-only.
4. **Config + wiring** — `BrainConfig.Agent *AgentConfig{Tools []string,
   MaxIterations int, SystemPrompt string}`; structural validation in
   `config.validateAgent`; name resolution in `app.buildAgentBrain` through
   `tool.Builtin` (unknown name → `ErrUnknownTool`, fail-loud at wiring).
   The agent brain requires exactly one model (`ErrAgentModelCount`). The
   builder (`web/builder/src/config/schema.ts`) already **types**
   `AgentConfig` so the block round-trips faithfully, but `BrainPanel`
   (`canvas/CanvasView.tsx`) does **not** expose it: today it edits only
   name / sensitivity / dispatch / persona. The visual seat of this piece is
   an additive section in that panel.
5. **Routing-decision audit today (the bar "audited like routing decisions"
   must meet)** — `policy.Decision{Provenance{Considered[]}, Accounting[]}`
   consumed by the Orchestrator's structured logs (`logNoAnswer` logs the
   provenance; the happy path logs through metrics), Prometheus metrics
   (`metrics.Metrics`), and the ADR-0023 bus → ADR-0024 SSE Activity feed,
   which today carries exactly four lifecycle `EventType`s:
   `message_received`, `reply_sent`, `message_dropped`, `handle_failed`.
   **There is no tool-use event type today.**
6. **AgentSkills format (verified at source)** — a skill is a directory whose
   name matches the required frontmatter `name`; `SKILL.md` = YAML frontmatter
   + free markdown body. Required: `name` (1–64 chars, lowercase alnum +
   single hyphens, no leading/trailing/consecutive hyphen, must equal the
   parent directory name), `description` (1–1024 chars, non-empty). Optional:
   `license`, `compatibility` (≤500), `metadata` (string→string map),
   `allowed-tools` (space-separated, experimental). Progressive disclosure:
   metadata always loaded, body loaded on activation, bundled files
   (`scripts/`, `references/`, `assets/`) on demand.
7. **First-token command seam** — `/new` / `/reset` are handled in
   `internal/router/session.go` (`SessionPolicy.Triggers`): exact-match first
   token, a FIXED acknowledgement (`SessionResetAck`) through the normal
   outbound funnel, **zero model involvement**. `/tools` (FR-CHAT-1) rides the
   same pattern.

## Clarification resolutions (director, 2026-08-09)

- **R-1 (was NC-1) — v1 catalog:** `time`/`echo`/`calc` (kept) +
  **`http_fetch`** (GET-only, operator host allow-list, response-size cap, no
  redirects off-list) + **`read_file`** (read-only, jailed to one
  operator-configured root, size cap) + **`webhook_call`** (POST JSON to
  allow-listed hosts, response cap, hard timeout — the user's no-code tool
  factory and the n8n door; the full n8n bridge stays post-beta). Shell stays
  excluded (§8).
- **R-2 (was NC-2) — sensitivity:** house default + declared operator
  override. `read_file` = sensitive by default; `http_fetch` and
  `webhook_call` = not sensitive by default.
- **R-3 (was NC-3) — frontmatter parsing:** option (a) — a minimal in-house
  parser for the FLAT subset (`name`/`description`/`license`/`compatibility`;
  nested `metadata` skipped with tolerance). Zero dependencies. Parity note:
  OpenClaw uses a single-line parser (director-verified in its docs
  2026-08-09). The subset is documented in the skills guide (FR-DOC-1).
- **R-4 (was NC-4) — skill body injection:** minimal cut —
  `name`+`description` always; bodies of GRANTED skills under a configurable
  total budget with a sane default; past the budget, bodies are omitted with
  a structured warning, never a boot failure.
- **R-5 (was NC-5) — governance block location:** per-brain, inside `agent`
  (like persona/sensitivity; the builder panel edits one brain at a time).
- **R-6 (was NC-6) — audit privacy LAW (ADR-0024 §1 intact byte-for-byte):**
  bus/Activity tool events carry **METADATA ONLY** (brain, tool, channel,
  outcome, rule, latency) — **NEVER args nor prefixes** (args derive from the
  message = content). The bounded debugging prefix (80 runes) lives ONLY in
  local slog. Metrics: tool name + outcome only in labels. The chat's SSE
  no-leak audit must stay green untouched.

## Goal

After this piece, an agent brain has a small set of genuinely useful tools —
including, for the first time, tools with effects (I/O) — and the policy
engine acts as the **gatekeeper**: which tool each brain may use, on which
channel, in which of THREE modes (`allow` / `shadow` / `deny`), with
sensitive tools restricted to locally-running models and, for Private
brains, network tools confined to the private network (the shield); every
tool use, shadowed rehearsal, and denial is audited with the same rigor as
routing decisions (structured log + metric + Activity-feed event —
metadata-only by law). Alongside, brains can be taught **when** to use their
tools through markdown skills: `SKILL.md` files compatible with the
AgentSkills format, loaded read-only from disk — no code execution, no
plugins. The operator can interrogate the gatekeeper from the chat
(`/tools`). What explicitly stays out: plugins with code (post-beta),
provider-native function-calling (ADR-0021 §3.4, its own future ADR),
multi-model agents, shell execution (§8 stands), and the post-beta
progressive-governance ladder (recorded below, no commitment).

## Functional requirements

### Governance (the gatekeeper)

- **FR-GOV-1** — A new PURE selection function in `internal/policy` (working
  name `SelectTools`), the tool-shaped sibling of `SelectModels`: it filters
  a brain's configured tool set down to the tools permitted for
  **(brain, channel, model locality)**. Because the channel arrives
  per-message in the Envelope, this filter runs **per Handle** (unlike
  `SelectModels`, which runs once at construction) — it must therefore stay
  pure, allocation-light, and I/O-free. Additive: no existing `policy`
  symbol changes.
- **FR-GOV-2** — Tool **sensitivity is DECLARED, never inferred** (the
  ADR-0015 discipline): each catalog tool carries a house-default sensitivity
  class with a declared operator override (R-2); a sensitive tool is filtered
  out when the brain's single model has `Locality == Cloud`. The model's
  locality is known at wiring time in `internal/app` (the catalog carries
  it), so this half of the filter can be applied at construction; the channel
  half is per-message.
- **FR-GOV-3** — The gate holds at **two points** (defense in depth):
  (a) *advertisement* — `buildSystemPrompt` receives the already-filtered
  registry, so a disallowed tool is never announced to the model; and
  (b) *execution* — `runTool` resolves against the same filtered decisions,
  so even a hallucinated call to a known-but-denied tool does not execute.
  A denied execution attempt returns a tool-failure-class observation AND is
  audited as a denial (FR-AUD-2). A misconfigured governance input at Handle
  time **fails closed** (deny-all, logged), never open.
- **FR-GOV-4** — Governance config is declarative in the config file,
  per-brain inside `agent` (R-5): per-tool grants with a mode and an optional
  per-channel restriction. Absence of any governance block reproduces today's
  behavior byte-for-byte (the brain's configured tools, all channels, all
  allowed) — the piece is reversible by config, like the agent itself.
- **FR-GOV-5** — **Shadow mode (director extension A).** Every grant carries
  one of THREE modes: `allow` | `shadow` | `deny`. In `shadow`, the tool IS
  advertised to the model (the point is observing the model's real judgment)
  but execution does NOT happen: the full intention is recorded (bounded
  detail in slog only, R-6) and the model receives an honest simulation
  observation — exact text fixed in ADR-0041 — that makes clear no real
  effect occurred without breaking the loop. Promotion `shadow`→`allow` is a
  config change that takes effect on hot-apply, no restart.
- **FR-GOV-6** — **Network shield (director extension B).** For a brain with
  `Sensitivity == Private`, network tools (`http_fetch`, `webhook_call`) may
  only reach the PRIVATE network — loopback, RFC1918, link-local, IPv6
  ULA/loopback — AND must still be inside the allow-list (the shield
  RESTRICTS, never widens). **Verified at the dial:** the connection's
  effective IP is validated at connect time — mandatory ADR technical note:
  redirects and DNS-rebinding are controlled by validating the RESOLVED IP,
  not just the hostname. An attempt outside → nothing contacted, honest
  observation, `tool_denied` with rule `private_network_shield`. Non-Private
  brains (`public` — the only other tier): allow-list only. At the SELECTION
  level the shield is surfaced as a flag on the decision so the execution
  layer arms the dial guard.

### Audit (the same bar as routing decisions — metadata-only by law)

- **FR-AUD-1** — Every tool execution emits: a structured `slog` line
  (brain, tool, channel, outcome, latency, plus an 80-rune bounded args
  prefix — **slog only**, R-6), a Prometheus metric (counter by
  tool/outcome + a latency histogram — tool name and outcome ONLY in
  labels), and a new bus `EventType` (`tool_used`) that the ADR-0024
  Activity feed renders — **metadata only: brain, tool, channel, outcome,
  rule, latency; never args**. Additive on `metrics.Metrics` and `bus` —
  existing event consumers are untouched (unknown-type tolerance verified in
  the SSE consumer before the cut). The chat's SSE no-leak audit stays green
  untouched.
- **FR-AUD-2** — Every **denial** (a call to a tool filtered out by the
  gate, or stopped by the shield or a tool cage) is audited the same three
  ways (`tool_denied`), with the denying rule dimension (channel / locality /
  not-granted / private_network_shield / cage) recorded — metadata only.
- **FR-AUD-3** — The audit trail is observability, NOT conversation memory:
  nothing new is persisted to the conversation store (ADR-0021 §6 stands).
- **FR-AUD-4** — **Shadowed uses (extension A):** a `tool_shadowed` event
  with the same discipline (metadata-only on bus/Activity; bounded detail in
  slog only) + its own metric, so a shadow rehearsal is observable end to
  end before promotion.

### Tools with effects (the higher bar of ADR-0021 §8, now designed)

- **FR-TOOL-1** — The piece ships the R-1 catalog: **`http_fetch`**
  (GET-only, operator host allow-list, response-size cap, no redirects
  off-list), **`read_file`** (read-only, jailed to one operator-configured
  root, size cap), **`webhook_call`** (POST JSON to allow-listed hosts,
  response cap, hard timeout). Each is caged by construction: hard timeout
  (the existing per-tool ctx), bounded response size, allow-list where it
  reaches out, and the FR-GOV-6 dial-time shield when the brain is Private.
  Shell execution remains excluded. Each new tool honors the `Tool`
  concurrency contract.
- **FR-TOOL-2** — `tool.Builtin` remains the SINGLE safe-toolset boundary:
  new tools resolve there and nowhere else; a tool that needs config (an
  allow-list, a root directory) gets it at wiring time in `internal/app`,
  keeping `internal/tool` a leaf.
- **FR-TOOL-3** — ADR-0041 (SP0) precedes TDD (the §8 deferral names that
  bar explicitly): cages, governance seam (tri-state), network shield with
  the dial-time validation note, the exact simulation-observation text, and
  the skills loader with the in-house parser.

### Skills (markdown, AgentSkills-compatible)

- **FR-SKILL-1** — A skills loader (new leaf package, working name
  `internal/skill`) reads skill directories from a configured path: each
  contains `SKILL.md` with YAML frontmatter validated to the AgentSkills
  spec as verified at source (name 1–64 lowercase-kebab equal to the
  directory name; description 1–1024 non-empty; tolerant of the optional
  fields). Parsing per R-3: in-house FLAT-subset parser
  (`name`/`description`/`license`/`compatibility`; nested `metadata` skipped
  with tolerance), zero dependencies. Read-only: the loader never executes
  anything, follows references at most one level, and caps file size (bound
  in ADR-0041).
- **FR-SKILL-2** — Skills teach WHEN to use tools (R-4): `name`+`description`
  always join the agent's system prompt alongside the tool catalog; bodies of
  granted skills are included under a configurable total budget with a sane
  default — past it, bodies are omitted with a structured warning, never a
  boot failure.
- **FR-SKILL-3** — A skill that references tools the brain is not granted is
  not an error, but the skill's tool mentions never widen the gate: the
  policy engine's decision is final (skills are documentation, not
  authorization — `allowed-tools` in the frontmatter is recorded but NOT
  honored as a grant in this cut).
- **FR-SKILL-4** — A malformed skill (bad frontmatter, name/directory
  mismatch, oversize) is skipped with a structured warning at wiring time,
  never a boot failure — mirrors the degrade-gracefully house pattern.

### Chat, config, builder, docs

- **FR-CHAT-1** — **`/tools` (director extension C):** a first-token command
  on the `console` channel (the `/new` pattern — `internal/router/session.go`
  seam: exact match, fixed system response through the outbound funnel, zero
  model involvement). It answers with the gatekeeper report of that
  conversation's brain: effective grants (allow/shadow/deny + shield if
  applicable) and the last N uses/denials — **metadata-only, the R-6 law**.
  The recent-uses half is served from a bounded in-memory ring fed by the
  bus (no persistence — FR-AUD-3 stands).
- **FR-CFG-1** — `AgentConfig` grows additively (per-tool grants with mode +
  channels + sensitivity override, per-tool cage config, skills path + body
  budget); every existing config file remains valid and byte-equivalent in
  behavior. `config.validateAgent` validates structure; semantic resolution
  stays in `internal/app` (the established split).
- **FR-UI-1** — The builder's `BrainPanel` gains an agent section (the
  block already round-trips via `schema.ts`): tool grants with their
  mode/channel/sensitivity annotations, max_iterations, system_prompt,
  skills path. **Per the house design law, the panel work does not start
  until rendered mockups are approved** (the aurora lesson: design cuts are
  decided by looking at mockups, never by description).
- **FR-DOC-1** — Operator docs: a TOOLS-and-skills guide (what each tool
  can reach, how the gate composes, shadow mode and the shield, how to write
  a skill + the R-3 frontmatter subset), the config reference block, and the
  ROAD-TO-BETA checklist update at close.

## Acceptance scenarios (Given / When / Then)

- **AS-1** Given a brain granted `calc` on channel `telegram` only, When a
  message arrives via `discord` and the model replies `TOOL: calc(2+2)`,
  Then calc is absent from the advertised catalog, the call does not
  execute, the model receives the tool-failure-class observation, and a
  `tool_denied` event (rule: channel) reaches log + metric + Activity feed.
- **AS-2** Given a tool declared sensitive and a brain whose single model
  has `Locality == Cloud`, When the brain is wired, Then the sensitive tool
  is excluded from that brain's effective registry (never advertised, never
  executable) while a Local-model brain with the same grant keeps it.
- **AS-3** Given any successful tool execution, When it completes, Then
  exactly one `tool_used` event with brain/tool/channel/outcome/latency —
  and NO args (R-6) — is observable on the bus and the metric increments —
  asserted in a `-race` test running concurrent `Handle` calls over ONE
  shared `AgentBrain` (the ADR-0021 §5 mandatory shape, extended to the
  audit path).
- **AS-4** Given a config with no governance block, When the app wires an
  agent brain configured as today (`tools: ["time","echo","calc"]`), Then
  behavior is byte-for-byte today's: same system prompt, all tools on all
  channels, no denials possible, zero new log/metric noise beyond the
  `tool_used` audit events.
- **AS-5** Given a skills directory with one valid skill and one malformed
  skill (frontmatter name ≠ directory name), When the app wires the brain,
  Then the valid skill's name+description appear in the system prompt, the
  malformed one is skipped with a structured warning naming the violation,
  and boot succeeds.
- **AS-6** Given an effectful tool with an operator allow-list (e.g.
  `http_fetch` limited to `example.com`), When the model calls it against a
  host off the list, Then the tool returns an error observation (allow-list
  named), nothing is contacted, and the use is audited as `tool_denied` with
  the cage rule.
- **AS-7** Given the builder with a brain node selected, When the operator
  edits the agent section and applies, Then the emitted JSON round-trips
  through `config.Validate` unchanged and hot-applies (the ADR-0024/0030
  apply path), with the same 400-field-path inline error behavior persona
  has today. *(Cut gated on approved mockups, FR-UI-1.)*
- **AS-8 (shadow, extension A)** Given a grant in `shadow` mode on a tool
  with effects, When the model calls it, Then the tool IS present in the
  advertised catalog, NOTHING external is contacted (asserted — the spy/fake
  records zero executions), the model receives the exact simulation
  observation of ADR-0041, and a `tool_shadowed` event (metadata-only)
  reaches log + metric + Activity feed.
- **AS-9 (shadow promotion, extension A)** Given a running app with a
  `shadow` grant, When the operator hot-applies a config changing it to
  `allow`, Then the next call executes for real — no restart — and audits
  `tool_used`.
- **AS-10 (shield beats allow-list, extension B)** Given a Private brain
  whose `http_fetch` allow-list contains a PUBLIC host, When the model
  fetches it, Then the shield wins: nothing is contacted, the model gets the
  honest observation, and `tool_denied` (rule `private_network_shield`)
  reaches the three surfaces.
- **AS-11 (shield inverse, extension B)** Given the same Private brain and
  an RFC1918 host on the allow-list, When the model fetches it, Then the
  call executes and audits `tool_used` — the shield restricts to private
  networks, it does not block them.
- **AS-12 (/tools, extension C)** Given a console conversation routed to a
  governed brain, When the operator sends `/tools`, Then the reply is a
  system response (zero model calls) listing the effective grants
  (allow/shadow/deny, shield flag) and the last N uses/denials as
  metadata-only lines — and the SSE no-leak audit stays green.

## Success criteria

- Coverage: ≥90% in `internal/policy` and `internal/brain` (house floor for
  those packages); ≥85% in `internal/tool`, `internal/skill`, and touched
  `internal/app` surface. The new selection function and the gate paths are
  table-driven and `-race`-tested in the concurrent-Handle shape.
- `make quality` green with `-race` over the WHOLE suite, not just new code.
- **Zero new dependencies across the whole piece** (`go.mod` untouched) —
  R-1/R-3 close the only two candidates (stdlib `net/http` + in-house
  parser).
- Every existing config file validates and behaves identically (AS-4 is the
  tripwire).
- Auditability demonstrated end-to-end: a live smoke shows a use, a shadow,
  and a denial appearing in the desktop Activity feed, metadata-only.
- ADR-0024 §1 (SSE metadata-only) provably intact: the existing no-leak
  audit tests pass untouched.

## Decisions folded in (vetoable at review, no archaeology needed)

- **D-1** The gate is enforced at advertisement AND execution (FR-GOV-3) —
  a single point would either leak the catalog to the model or rely on the
  model's honesty; two points cost one map lookup. Shadow is the deliberate
  exception on the advertisement side only (announced, never executed).
- **D-2** Tool governance reuses the declared-not-inferred discipline of
  ADR-0015 verbatim (sensitivity classes: house default + declared operator
  override, R-2), so the privacy reasoning stays non-recursive.
- **D-3** The per-message half of the filter lives in `AgentBrain.Handle`
  (channel is per-envelope); the per-construction half (model locality)
  lives in `app.buildAgentBrain`. No new seam between them — the policy
  function is pure and called from both.
- **D-4** Skills are documentation, not authorization (FR-SKILL-3): the
  frontmatter's experimental `allowed-tools` never widens a grant.
- **D-5** The audit trail rides the EXISTING three surfaces (slog, metrics,
  bus→Activity) rather than a new store — "audited like routing decisions"
  means the same surfaces routing uses today, and ADR-0021 §6 already ruled
  a persisted trace out of conversation memory. The R-6 split (args-bearing
  detail in slog only; metadata everywhere else) is law.
- **D-6** Governance misconfiguration at Handle time fails CLOSED (deny-all,
  logged): a gatekeeper that fails open is not a gatekeeper.
- **D-7** `/tools` lands in SP5 (it needs SP2's events for the recent-uses
  ring and SP5's config wiring for real grants); the builder panel moves to
  its own mockup-gated SP6 so the visual gate never blocks config/docs.
- **D-8** There is no "Internal" sensitivity tier and this piece does not
  invent one (`public|private` only, builder tripwire enforced): the shield
  attaches to `Private`; `Public` brains are allow-list-only.

## Sub-phase plan (each gated by the full phase cycle)

- **SP0** — **ADR-0041**: effectful tools + cages (timeout, caps,
  allow-lists) + governance seam (`SelectTools` pure, two points, TRI-STATE)
  + network shield (dial-time IP validation; redirects/DNS-rebinding note) +
  the exact simulation-observation text + skills loader with the in-house
  flat parser. Accepted before any test.
- **SP1** — Governance seam, strict TDD: `policy.SelectTools` (tri-state +
  sensitivity + shield flag at selection level) + the two-point gate in
  `AgentBrain` (shadow = announced, never executed, simulation observation;
  fail-closed) — proven with the three existing PURE tools only (seam first,
  payload after; AS-1/AS-2/AS-4 + the shadow AS at unit level).
- **SP2** — Audit, TDD: `tool_used`/`tool_denied`/`tool_shadowed` bus events
  (metadata-only law) + metrics + slog (bounded prefix here only); Activity
  feed rendering; the concurrent `-race` audit test (AS-3); SSE no-leak
  audit stays green.
- **SP3** — Effectful tools v1 per the R-1 catalog, one tool per commit,
  each with its cage and the FR-GOV-6 dial-time shield (AS-6/AS-10/AS-11),
  and its audit wiring. **House-catalog attrs tripwire (copilot rider,
  2026-08-09): a test MUST pin that the app wiring declares
  `http_fetch`/`webhook_call` with `Network: true` and `read_file` with
  `Sensitive: true` — a forgotten declaration would silently bypass the
  shield or the locality rule (zero attrs = not sensitive, not network).**
- **SP4** — Skills: loader + flat-subset parser + validation +
  system-prompt injection under budget (AS-5).
- **SP5** — Config surface (`AgentConfig` grants/cages/skills, hot-apply
  promotion AS-9) + **`/tools`** (FR-CHAT-1, AS-12) + docs + ROAD-TO-BETA
  close.
- **SP6** — Builder panel (FR-UI-1, AS-7) — **gated on approved mockups**.

## Post-beta (recorded, no commitment — director extension D)

The progressive-governance ladder: `shadow` → **ask-mode** (per-use approval
from the console) → **double-sign** (two-model consensus for sensitive
actions) → **full rehearsal** (dry-run of an entire config in shadow from
the builder). Each rung builds on this piece's tri-state seam and audit
surfaces; none is in the beta scope.

## Review checklist (template gate — green before "approved for TDD")

- [x] Goal stated behaviorally, with explicit exclusions (plugins with code,
      native function-calling, multi-model agents, shell, the post-beta
      ladder).
- [x] Every FR traces to a governing decision (ADR-0021 §§2/4/6/8,
      ADR-0012/0015 discipline, ADR-0023/0024, director resolutions R-1..R-6
      and extensions A/B/C of 2026-08-09) and names its seam and blast
      radius.
- [x] Acceptance scenarios assertable; unhappy paths covered (denial by
      channel/locality/shield/cage, fail-closed misconfig, malformed skill,
      budget overflow, shadow non-execution, SSE no-leak).
- [x] Success criteria measurable (coverage floors, `-race`, zero new deps,
      AS-4 tripwire, metadata-only proofs).
- [x] External-docs verification stated: AgentSkills verified at source
      2026-08-09; everything else stdlib + in-repo; Context7 N/A (no new
      library).
- [x] `[NEEDS CLARIFICATION]` — none open: the six were resolved by the
      director on 2026-08-09 (R-1..R-6 above).
