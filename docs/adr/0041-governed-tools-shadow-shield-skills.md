# ADR-0041: Governed tools — tri-state policy gate, shadow mode, network shield, caged effectful tools, and the markdown skills loader

> **Status:** accepted
> **Date:** 2026-08-09
> **Deciders:** Sebastián Moreno Saavedra
> **Builds on:** [ADR-0021](0021-agents.md) (the `AgentBrain` loop, the `Tool`
> seam §4, the safety invariants §2, and §8's explicit deferral: dangerous /
> side-effecting tools require "their own ADR, with sandboxing / allow-listing /
> consent designed first" — **this is that ADR**), [ADR-0012](0012-policy-engine.md)
> / [ADR-0015](0015-pre-dispatch-selector.md) (declared-not-inferred sensitivity,
> pure selection, fail-loud on unconfigured values), [ADR-0023](0023-event-bus-and-router-hook.md)
> (the best-effort bus), [ADR-0024](0024-builder-live-view-sse-and-ui.md)
> (**§1 metadata-only law — intact byte-for-byte**), [ADR-0018](0018-conversation-store-interface.md)
> §6 via ADR-0021 §6 (a tool-use trace is observability, never conversation
> memory). Governing spec:
> `docs/superpowers/specs/2026-08-09-governed-tools-skills.md` (approved
> 2026-08-09; director resolutions R-1..R-6 + extensions A/B/C).

## Context

ADR-0021 shipped the agent loop with three PURE tools and deliberately parked
every tool with effects behind a higher bar. The beta criterion "policy-governed
tools + markdown skills" (ROAD-TO-BETA) now calls that bar: agents need useful
tools (network and disk I/O), and Korvun's differentiator is that the **policy
engine is the gatekeeper** — per-brain, per-channel, sensitivity-aware,
rehearsable in shadow, shielded on private brains, and audited like routing
decisions. This ADR fixes the security design BEFORE any test is written
(the §8 contract), covering: the governance seam, the two-point gate, shadow
mode's exact semantics and observation text, the network shield's dial-time
validation, the three caged tools, the audit event grammar, and the skills
loader with its in-house frontmatter parser.

### External-docs verification (per CLAUDE.md non-negotiable)

Everything here is stdlib + existing `internal/` packages — **zero new
dependencies**, so Context7 does not apply. The two external contracts touched
were verified at source on 2026-08-09: the AgentSkills `SKILL.md` format
(<https://agentskills.io/specification>; field set and constraints recorded in
the spec) and — director-provided — OpenClaw's single-line frontmatter parser
parity note. The effectful tools use `net/http`, `net`, `net/netip`, `os`, and
`path/filepath` from the standard library.

## Decision

### 1. The governance seam — `policy.SelectTools`, pure, tri-state, per-message

A new PURE function in `internal/policy` (additive; no existing symbol
changes), the tool-shaped sibling of `SelectModels`:

```go
// ToolMode is the tri-state grant mode (spec FR-GOV-5). The zero value is
// invalid so an unconfigured grant fails loud (ErrUnknownToolMode), the
// ADR-0015 discipline.
type ToolMode int
const (
    ToolAllow  ToolMode = iota + 1 // advertised and executed
    ToolShadow                     // advertised, NEVER executed (rehearsal)
    ToolDeny                       // neither advertised nor executed
)

// ToolAttrs are the DECLARED attributes of a catalog tool (house default +
// operator override, R-2). Never inferred.
type ToolAttrs struct {
    Sensitive bool // local-models-only when true
    Network   bool // subject to the Private-brain network shield
}

// ToolGrant is one per-brain grant (config: per-brain inside `agent`, R-5).
type ToolGrant struct {
    Name     string
    Mode     ToolMode
    Channels []string // nil/empty = every channel
}

// ToolRule names the dimension that decided a non-allow outcome, for audit.
type ToolRule string
const (
    ToolRuleNone              ToolRule = ""                       // allowed / shadowed
    ToolRuleDenyGrant         ToolRule = "deny"                   // explicit deny grant
    ToolRuleNotGranted        ToolRule = "not_granted"            // no grant for the tool
    ToolRuleChannel           ToolRule = "channel"                // channel restriction
    ToolRuleSensitiveLocality ToolRule = "sensitive_locality"     // sensitive tool × Cloud model
    // Dial-time / execution-layer rules (same grammar, emitted by the cages):
    ToolRulePrivateShield     ToolRule = "private_network_shield" // FR-GOV-6
    ToolRuleCage              ToolRule = "cage"                   // allow-list / cap / timeout
)

// ToolDecision is the per-tool outcome for one message.
type ToolDecision struct {
    Mode   ToolMode // effective mode after restrictions
    Rule   ToolRule // why, when Mode == ToolDeny (or shield/cage at execution)
    Shield bool     // true = network tool on a Private brain: arm the dial guard
}

// ToolQuery is the per-message context.
type ToolQuery struct {
    Channel     string
    Sensitivity Sensitivity // the brain's declared tier (public|private)
    Locality    Locality    // the agent's single model
}

func SelectTools(grants []ToolGrant, attrs map[string]ToolAttrs, q ToolQuery) (map[string]ToolDecision, error)
```

**Rule precedence (restrictions apply BEFORE the mode; the gate restricts,
never widens):**

1. Explicit `ToolDeny` grant → `ToolDeny` / `deny`.
2. Channel restriction unmatched → `ToolDeny` / `channel` (even for shadow).
3. `attrs.Sensitive && q.Locality == Cloud` → `ToolDeny` /
   `sensitive_locality` (even for shadow — a rehearsal must not advertise a
   tool the privacy rule forbids outright).
4. Otherwise the granted mode stands; `Shield = attrs.Network &&
   q.Sensitivity == Private`.

The result map covers the union of grant names and attrs keys; a tool present
in attrs but not granted maps to `ToolDeny` / `not_granted`. A granted tool
absent from attrs gets zero attrs (not sensitive, not network) — attributes
are declared in the catalog; absence means the house default.

**Fail-loud inputs (construction-class errors, mirrored from
`ErrUnknownSensitivity`):** unknown `q.Sensitivity` → `ErrUnknownSensitivity`;
unknown `q.Locality` → `ErrUnknownLocality` (new); a grant with zero/unknown
`Mode` → `ErrUnknownToolMode` (new); a duplicate grant name →
`ErrDuplicateToolGrant` (new); an empty grant name → `ErrInvalidToolGrant`
(new). All wrapped with `%w`.

**Purity contract:** no I/O, no allocation beyond the result map, inputs never
mutated, deterministic — it runs on the hot path once per `Handle`.

### 2. The two-point gate in `AgentBrain` — and fail-closed

`AgentBrain` accepts optional governance at construction
(`WithAgentGovernance`; nil = today's behavior byte-for-byte, the AS-4
tripwire). When present, each `Handle`:

- calls `SelectTools` once with the envelope's channel;
- **advertisement:** `buildSystemPrompt` receives the registry filtered to
  `ToolAllow ∪ ToolShadow` (shadow IS advertised — extension A's point is
  observing the model's real judgment);
- **execution:** `runTool` consults the same decisions —
  `ToolAllow` executes; `ToolShadow` returns the simulation observation
  WITHOUT executing; `ToolDeny` (or a name missing from the map) returns the
  denial observation WITHOUT executing. A name absent from the registry keeps
  today's `tool %q not found` observation (checked first — honesty about
  nonexistence beats governance theater).
- **fail-closed:** if `SelectTools` returns an error at `Handle` time
  (misconfigured governance that slipped past wiring validation), the gate
  DENIES ALL — empty advertisement, every call denied, the error logged. A
  gatekeeper that fails open is not a gatekeeper (spec D-6).

**Observation texts (exact, fixed here per extension A; constants in
`internal/brain`):**

- Simulation (shadow):

  ```
  shadow mode: tool <name> was NOT executed (governance rehearsal). No real
  action or effect occurred. Do not invent or assume a result; if the user's
  request depended on this action, say it was not performed.
  ```

  One line in the code (`shadowObservation(name)`); it must make the absence
  of real effect unmistakable to the model without killing the loop — the
  loop continues exactly as with any tool-failure-class observation, bounded
  by the iteration cap.

- Denial: `tool <name> is not permitted here` — deliberately rule-silent
  toward the MODEL (the rule dimension goes to the audit surfaces, not into
  the conversation; telling the model which rule blocked it invites
  rule-shopping).

### 3. The network shield — validated at the DIAL, not the hostname

For a Private brain, network tools (`http_fetch`, `webhook_call`) may only
reach the private network, AND still only allow-listed hosts (the shield
restricts, never widens — spec AS-10/AS-11).

**Technical note (mandatory, extension B): the check runs on the RESOLVED
IP at connect time**, via the stdlib dialer's `Control` hook
(`net.Dialer.Control`), which fires for every connection attempt AFTER DNS
resolution with the concrete `address` being dialed. Validating only the
hostname is rejected because it is bypassable two ways:

- **redirects** — a listed host answering 3xx toward an unlisted/public one
  (also closed at the HTTP layer: `http_fetch` follows redirects only within
  the allow-list, and under the shield only to private addresses; the dial
  guard still backstops every hop because each hop dials);
- **DNS-rebinding** — a listed hostname resolving to a public IP on the
  second lookup; the `Control` hook sees the actual IP of the actual
  connection, so a rebound resolution is denied at the socket, before any
  byte leaves.

**Private means (parsed with `net/netip`):** IPv4 loopback `127.0.0.0/8`,
RFC1918 (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`), link-local
`169.254.0.0/16`; IPv6 loopback `::1`, ULA `fc00::/7`, link-local
`fe80::/10`; plus their IPv4-mapped-in-IPv6 forms (`::ffff:a.b.c.d`
normalized via `netip.Addr.Unmap` before classification). Everything else is
public. A shield denial contacts nothing, feeds the model the honest denial
observation, and audits `tool_denied` with rule `private_network_shield`.

Non-Private brains (`public` — the only other tier; no tier is invented,
spec D-8): allow-list only, no shield.

### 4. The three caged effectful tools (R-1 catalog)

All resolve exclusively through `tool.Builtin` (the single safe-toolset
boundary stands); per-tool config (allow-lists, root, caps) is injected at
wiring time in `internal/app`, keeping `internal/tool` a leaf. Every cage
violation is a `tool_denied` with rule `cage` (or `private_network_shield`
when the shield fired). Shell remains excluded (ADR-0021 §8 A3 stands).

- **`http_fetch`** — GET only; operator host allow-list (exact host match,
  case-insensitive, optional port); response-size hard cap (default fixed in
  the SP3 spec table); redirects followed ONLY to allow-listed hosts (and
  under the shield, only to private addresses) up to a small hop cap; the
  per-tool ctx timeout stands (ADR-0021 §2); the shield's dial guard armed
  per §3 when `ToolDecision.Shield`.
- **`read_file`** — read-only; every path resolved (`filepath.Abs` +
  `filepath.EvalSymlinks`) and REQUIRED to stay under the operator-configured
  root (jail; symlink escapes die at the resolved-path check); size cap;
  **sensitive by house default** (R-2) so a Cloud-model brain never sees it
  unless the operator explicitly overrides the class.
- **`webhook_call`** — POST with a JSON body to allow-listed hosts (same
  allow-list + shield semantics as `http_fetch`); response cap; hard timeout.
  This is the user's no-code tool factory and the n8n door; the full n8n
  bridge stays post-beta.

### 5. Audit — three surfaces, metadata-only by law (R-6)

Three new bus `EventType`s: `tool_used`, `tool_denied`, `tool_shadowed`.
On the bus and the ADR-0024 Activity feed they carry **metadata only**:
brain, tool, channel, outcome, rule, latency — **never args, never
prefixes** (args derive from the message = content; ADR-0024 §1 stays intact
byte-for-byte and its no-leak audit tests must keep passing untouched).
Prometheus: counters + latency histogram with tool name and outcome as the
only labels (bounded cardinality). The 80-rune bounded args prefix for
debugging lives ONLY in local slog. Nothing is persisted (ADR-0021 §6 —
observability, not memory). `/tools` (spec FR-CHAT-1) serves its
recent-uses lines from a bounded in-memory ring fed by the bus, same
metadata-only grammar.

### 6. The skills loader — read-only markdown, in-house flat parser (R-3)

A new leaf package `internal/skill` (stdlib only): loads skill directories
from a configured path; each `SKILL.md` = frontmatter + body. **Parser:
in-house, FLAT subset** — first-level `key: value` lines only, for `name`,
`description`, `license`, `compatibility`; nested blocks (`metadata:`, any
indented continuation) are skipped with tolerance; `allowed-tools` is
recorded but NEVER honored as a grant (skills are documentation, not
authorization — spec D-4). Validation per the source-verified AgentSkills
constraints (name 1–64 lowercase-kebab, equal to the directory name;
description 1–1024 non-empty). Caps: `SKILL.md` ≤ 64 KiB read cap;
references followed at most one level. A malformed skill is skipped with a
structured warning at wiring, never a boot failure. Injection per R-4:
name+description always; granted-skill bodies under a configurable total
budget (default fixed in the SP4 spec table); past it, bodies omitted with
a structured warning.

## Consequences

### What this enables
- Tools with real effects under an auditable gatekeeper, rehearsable in
  shadow before promotion (hot-apply, no restart), with Private brains
  physically unable to reach the public network through them.
- The post-beta ladder (ask-mode, double-sign, full config rehearsal) builds
  on this tri-state seam and event grammar without new seams.

### What this asks / costs
- `SelectTools` runs per `Handle` (the channel is per-message): a map build
  per message on agent brains. Accepted — it is allocation-light and pure.
- The shield adds a `Control` hook on the network tools' dialers; a few
  nanoseconds per connect, only on governed network tools.
- Redirect handling in `http_fetch` must re-check every hop (list + shield).

### Trade-offs accepted
- **Rule-silent denials toward the model** (the rule goes to audit only):
  less model self-correction, no rule-shopping.
- **In-house flat frontmatter parser over yaml.v3:** zero deps and a
  documented subset, at the cost of ignoring exotic-but-valid YAML; the two
  REQUIRED AgentSkills fields are flat strings, so the subset is sufficient.
- **Exact-host allow-lists (no wildcards) in v1:** simpler to reason about
  and to audit; wildcard patterns are an additive follow-up if operators ask.

## Alternatives considered

- **Widening `model.Model` / native function-calling now** — still rejected;
  unchanged from ADR-0021 §3.4 (a sibling `ToolCallingModel`, its own
  Context7-verified ADR).
- **Hostname-only allow-list validation** — rejected (§3): bypassable via
  redirects and DNS-rebinding; the resolved-IP dial check is the guarantee.
- **`gopkg.in/yaml.v3` for frontmatter** — rejected (R-3): +1 direct dep on
  the single-binary promise for two flat required fields.
- **Advertising denied tools with a "you may not use X" note** — rejected:
  it wastes prompt budget and invites the model to argue with the gate; a
  tool that cannot run is simply not in the catalog (shadow is the
  deliberate, observable exception).
- **A global `tool_policy` config block** — rejected (R-5): grants are
  per-brain inside `agent`, matching how sensitivity/persona are declared
  and how the builder panel edits one brain at a time.
- **Persisting the audit trail** — rejected; ADR-0021 §6 stands (the ring
  behind `/tools` is bounded, in-memory, metadata-only).

## Out of scope (recorded, not silently dropped)
- The progressive-governance ladder: ask-mode, double-sign, full-config
  shadow rehearsal (post-beta, spec extension D).
- The full n8n bridge (post-beta; `webhook_call` is the door).
- Plugins with code; shell execution (ADR-0021 §8 A3).
- Wildcard/CIDR allow-list patterns (additive follow-up).
