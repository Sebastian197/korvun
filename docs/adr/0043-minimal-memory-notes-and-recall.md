# ADR-0043: Minimal memory — governed notes + deliberate recall

- Status: Accepted (2026-08-16)
- Deciders: Chano (product: D1-D4, P1-P4), copilot (technical adjudication)
- Spec: `docs/superpowers/specs/2026-08-16-minimal-memory.md` (v1.3, approved
  for TDD). Review: /plan-eng-review + Codex cross-review over v1.2 —
  1 blocker + 13 majors + 9 minors (H1-H24), all adjudicated into v1.3.
- Related: ADR-0018/0019 (conversation store), ADR-0021 (AgentBrain),
  ADR-0041/0042 (governed tools, native lane), ADR-0015 (declared
  sensitivity), ADR-0024 §1 (metadata-only observability).

## Context

The beta's last piece: a brain must conserve context beyond the live
session. Three shipped contracts constrain the design: the hard session cut
(FR-SESS-2 — `LoadRecent` reads only the active session, pinned by
`TestSessionCut_endToEnd_agentSeesNoPreResetTurns`); structural privacy
(`Sensitivity` is declared per brain, never inferred, and `SelectModels`
drops cloud before fan-out); and the governed-tools gate as the single
execution path (`runTool` → `SelectTools`/`decideTool`). Everything is
already persisted (SQLite v2, sessions/turns, serialized single writer).
Product decisions: D1 the model writes notes via a governed tool, the
operator views/clears; D2 scope = brain+conversation by default,
brain-global opt-in; D3 recovery is deliberate, never automatic; D4 no new
UI in beta. P1-P4 from adjudication: two sequential sub-phases; a
governance grant is required for the tool; recall imports ONE quoted block
into an empty session only; coherent note defaults (10×200=2000) with a
fail-loud coherence validation.

## Decision

1. **Two sequential sub-phases of one piece (P1):** SP-A recall
   (router+config only) → SP-B notes (store+tool+brain+app+config). The
   complexity check fires at ≥10 files; the features share no central
   implementation file.
2. **Recall = one quoted block, empty session only (P3).** `/recall [k]` is
   a channel-agnostic first-token command beside the session triggers. It
   renders the last k non-system turns (user, assistant, operator) of the
   newest archived session WITH dialogue into ONE `RoleUser` delimited
   block (header names source + truncation; cap `recallBlockRunes`=4000),
   appended to an ACTIVE session that has no non-system turn. Source scan
   is bounded (S=5 sessions) via a new `SessionStore.LoadSessionTail(key,
   session, n)`; constants C=32/S=5/4000 are named in code. No row copies.
3. **Notes domain in `internal/conversation`.** Explicit `NoteScope` enum
   (`ScopeConversation`/`ScopeBrainGlobal`) + a `NoteStore` seam whose
   `AppendNote` enforces the count cap ATOMICALLY in one transaction
   (typed `ErrNotesFull`) and REJECTS incoherent scope/key pairs — the
   empty-`Key` sentinel can never become an accidental global write. The
   single derivation is one pure exported function
   (`EffectiveNoteScope`), consumed only by internal/app, which composes
   every memory closure (write/load/list/clear) over one store. The
   `notes` table rides `createTableStmt` (idempotent, composite-PK
   WITHOUT ROWID); no versioned migration. `DeleteConversation` cascades
   the key's notes (FR-DEL-1 stays true); brain-global notes are not the
   conversation's and survive, stated.
4. **`memory_note` is a governed builtin with zero house attrs.**
   `Attrs{}` deliberately (a Sensitive attr would deny it on
   cloud-locality brains via `ToolRuleSensitiveLocality`, contradicting
   conversation-scope validity); `ParamTool` with ONE required field
   `note`; single-line normalization; refuse-when-full. **A governance
   grant is REQUIRED at boot when the tool is listed (P2, the E-11
   molde)** — D1 is never vacuously ungoverned.
5. **Scope reaches the tool via an optional interface.**
   `tool.ScopedTool{ExecuteScoped(ctx, Scope{Brain, Conversation},
   args)}`, envelope facts only; `runTool` type-asserts (the
   `ToolCallingModel`/`ParamTool` precedent). The brain's name arrives via
   `WithAgentName` — decoupled from the audit option (with observability
   off, names would collide on ""). Composition via
   `WithAgentMemory(load, budgetRunes)` + pure `ComposeNotes`: an inert
   delimited block AFTER `skillsBlock`, both lanes, fail-open like
   `loadHistory`.
6. **Config, fail-loud with field paths:** `brains[i].agent.memory`
   (requires `storage`; `budget_runes ≥ max_notes × max_note_runes` — P4,
   calcification impossible; defaults 10/200/2000). `memory.scope:
   "brain"` requires the SELECTED model to be Local (the
   `localityOf(selected[0])` precedent — not the raw catalog).
   `session.recall_max` 0..50 (0 = disabled, falls through). Reserved
   trigger tokens `/recall`, `/notes` rejected in `session.triggers`.
7. **Audit split stated plainly (H18's resolution):** the metadata-only
   law (ADR-0024 §1) governs the OBSERVABILITY surfaces — tool audit,
   metrics labels, SSE frames, Activity — which carry no note content,
   ever (verified pre-RED: liveview frames are content-free by
   construction AND test-asserted). The `/notes` report is CONVERSATION
   content the user asked for (`AckNotesReport`, the `/tools` molde);
   on self-persisting channels it persists as a SYSTEM turn; on network
   channels the durable trace is the structured slog line.

## Alternatives considered and rejected

- **Row-copy recall** (append the archived turns as real rows): destroys
  provenance, permits duplication/amplification, fabricates invalid role
  sequences, contaminates search/counts (H6, H19, H23; Codex concurring).
  The quoted block kills all four by construction.
- **Automatic carry / model summaries on session expiry:** contradicts D3,
  hollows out FR-SESS-2, and adds a model call (~77 s/round on the target
  hardware) to a path that is free today.
- **`recall_history` as a model tool:** deferred — recovery is the
  human's deliberate act, not the model's (larger privacy surface).
- **Context values for tool scope:** rejected for an explicit optional
  interface (the house precedent).
- **Eviction when the note box is full:** rejected for refuse-when-full —
  predictable, the operator clears; per-note update/delete is post-beta.
- **`Key("")` as the brain-global sentinel:** collides with the
  no-identity sentinel (`KeyFromEnvelope`); replaced by the explicit enum
  + store-level pair validation.
- **Catalog-wide cloud guard for brain scope:** rejected for the
  selected-model check (the shipped precedent); catalog-wide would reject
  Private brains whose selection already dropped cloud.
- **A new bus event type for memory:** deferred; slog + persisted acks
  suffice in beta (named as a future Activity candidate).

## Consequences

Positive: context survives the session cut only through deliberate,
policy-governed acts; both new capabilities reuse the shipped gate, store,
command molde and composition seams — zero new dependencies, zero new gate
code; every privacy property is enforced at boot or by construction, not
by runtime vigilance.

Negative / accepted residuals (named):
- **Read-side governance is structural, not per-channel:** the gate
  governs the WRITE; the READ is governed by scope + construction (a
  conversation Key embeds channel+conversation; brain-global is all-local
  only). A per-channel injection policy is future work if evidence
  demands it.
- **Persistent-injection residual:** notes are model text elevated into
  the system prompt. Mitigated by the inert delimited block, single-line
  normalization, an adversarial-format AS and the mandatory real-model
  sub-phase; the remaining model-obedience risk is accepted for beta.
- The `/notes` report persists as a SYSTEM turn on self-persisting
  channels; clearing notes does not rewrite history.
- Recall provenance lives in the block header and the ack; network
  channels do not persist acks (slog is the durable trace there).
- Dialogue older than the bounded scan (S=5 archived sessions) is
  honestly "nothing to recall".

Named, not committed: reserving `/tools` in `session.triggers` (same
shadowing exists today); `bus.MemoryEvent` for the Activity feed;
`recall_history`; per-note management.
