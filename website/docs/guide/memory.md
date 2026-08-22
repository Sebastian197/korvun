# Governed memory

Since v0.8.0 a brain can conserve context beyond the live session in two
**deliberate, policy-governed** ways: persistent **notes** written through
the governed `memory_note` tool, and **`/recall`** — a deliberate,
provenance-visible recovery of the previous session's tail. Nothing is
remembered automatically: no embeddings, no model-made summaries, no silent
carry-over.

The design is in the repo:
[ADR-0043](https://github.com/Sebastian197/korvun/blob/master/docs/adr/0043-minimal-memory-notes-and-recall.md)
and the
[minimal-memory spec](https://github.com/Sebastian197/korvun/blob/master/docs/superpowers/specs/2026-08-16-minimal-memory.md).

## Notes — bounded facts that survive resets

An agent brain with a `memory` block keeps **notes**: short facts the model
stores through `memory_note`, one required field, single-line normalized,
size- and count-capped. Notes are not history — they survive `/new` and
`/reset` on purpose, and ride the brain's system prompt on every later
message of the same scope as an inert, clearly-delimited data block.

`memory_note` passes through the same tri-state gate as every tool
([allow / shadow / deny](/guide/tools-and-skills)):

- **`shadow` first** — announce the tool in rehearsal, watch what the brain
  *would* store (audited as `tool_shadowed`, visible in `/tools` and the
  Activity feed), then hot-apply the grant to `allow`.
- **Boot refuses half-configured memory** — listing `memory_note` without
  its `memory` block, or without a governance grant covering it, is a loud
  boot error, never a default.
- **Full is full** — when the box is full the tool refuses; the operator
  clears with `/notes clear`. No silent eviction.

The operator always sees the box: `/notes` lists the scope's notes
numbered; `/notes clear` wipes the scope. Both are instant system commands
— no model call — and clearing notes never touches the transcript.

## Scopes — and the structural privacy guarantee

`memory.scope` declares how far a note reaches:

- **`conversation`** (default) — notes live and ride only within the
  conversation that stored them.
- **`brain`** — one shared box for the whole brain, an explicit opt-in that
  makes notes cross conversations. **This scope REQUIRES the brain's
  selected model to be local**: brain-global memory refuses to boot with a
  cloud-selected model. The guarantee is structural — enforced at boot,
  not by a runtime attribute — so a private brain's notes never leave the
  machine.

## `/recall` — deliberate recovery of a session cut

Sessions cut context hard (see [the operator console](/guide/chat)).
`/recall` makes that cut recoverable **on purpose, never by accident**:

- Enabled by `session.recall_max` (`0` ⇒ disabled; 1..50). Bare `/recall`
  imports up to the configured max; `/recall <k>` clamps `k` to it.
- **Only into an empty session** — on a non-empty active session it
  refuses and points you to `/new` first. After one import the session is
  non-empty, so duplication is impossible by construction.
- **One quoted block** — the last turns of the newest archived session
  arrive as ONE clearly-delimited quoted block whose header names the
  source session; the acknowledgement names how many turns came back.
  Provenance stays visible: quoted context, not new messages.
- Zero model involvement — it is a system command, handled
  channel-agnostically.

## Configuration

```json
{
  "session": { "recall_max": 10 },
  "brains": [{
    "name": "assistant",
    "sensitivity": "private",
    "policy": {"kind": "priority"},
    "models": [{"provider": "ollama", "model_id": "llama3.2", "locality": "local"}],
    "agent": {
      "tools": ["time", "memory_note"],
      "governance": [
        {"tool": "time", "mode": "allow"},
        {"tool": "memory_note", "mode": "shadow"}
      ],
      "memory": {"scope": "conversation", "max_notes": 10, "max_note_runes": 200}
    }
  }]
}
```

This brain rehearses `memory_note` in shadow — watch what it would store,
then promote the grant to `allow` with a hot apply. Field-by-field details:
the [configuration reference](/reference/configuration). The `memory` block
requires the `storage` block (notes live in the durable store).

## What is out of scope — on purpose

Semantic retrieval and embeddings, automatic carry or model-made summaries,
and per-note editing are deliberately not part of minimal memory. The
operator clears a scope; the model stores within its caps; everything else
stays deliberate and visible.