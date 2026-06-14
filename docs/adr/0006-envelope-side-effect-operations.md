# ADR-0006: Side-effect operations — message edit and delete

> **Status:** accepted
> **Date:** 2026-06-14
> **Deciders:** Sebastián Moreno Saavedra

## Context

Phase 2E.6 of Stage 2-EXT adds support for **editing** and **deleting**
messages already sent by the bot, plus **observing** edits the user
makes to their own messages. Concretely the Telegram surface is:

- **Outbound mutations** (verified at `v1.21.0` via `go doc`):
  - `bot.EditMessageTextParams { ChatID, MessageID, InlineMessageID, Text, ReplyMarkup, ... }`
  - `bot.EditMessageCaptionParams { ChatID, MessageID, InlineMessageID, Caption, ReplyMarkup, ... }`
  - `bot.EditMessageReplyMarkupParams { ChatID, MessageID, InlineMessageID, ReplyMarkup }`
  - `bot.DeleteMessageParams { ChatID, MessageID }`
  - `bot.DeleteMessagesParams { ChatID, MessageIDs []int }`

  Target identification is consistently `(ChatID + MessageID)` XOR
  `InlineMessageID`. The edit-text and edit-caption primitives also
  carry an optional `ReplyMarkup` so the keyboard can be replaced in
  the same call as the body.

- **Inbound edits**: `Update.EditedMessage *Message` (and the variants
  `EditedChannelPost`, `EditedBusinessMessage`). The struct shape is
  the same as a regular `Message`; the discriminator is the
  `Message.EditDate int` field (Unix timestamp, non-zero iff the
  message has been edited).

ADR-0005 introduced — without naming it formally — a **side-effect
taxonomy** to justify why `AnswerCallbackQuery` belonged in 2E.4 and
edit/delete did not. This ADR makes that taxonomy explicit and shows
where each kind lives architecturally:

| Category | Examples | Conceptual shape | Architectural home |
|---|---|---|---|
| **Notifications to the user** | `AnswerCallbackQuery`, typing indicators | "tell the user something" | Envelope outbound; the existing `OutboundParams` pipeline maps it to a typed `Send*Params`. |
| **Mutations of existing state** | `EditMessageText`, `EditMessageCaption`, `DeleteMessage` | "change a thing identified by ID" | This ADR — also Envelope outbound, same pipeline. The Envelope describes the new state; Meta carries the target identifier. |
| **Reads from the channel** | `GetFile` / resolve file URL, `GetChat`, `GetMe`, `GetMyCommands` | "ask the channel a question and get a value back" | **Not** Envelope outbound — there is no payload to send, only an answer to receive. Will be modelled as a separate read primitive on the Channel port in a future ADR. |

The notification and mutation rows share the **Envelope-as-outbound-
instruction** pattern that 2E.2 (media), 2E.3 (location), and 2E.4
(keyboard + ack) all built on: one Envelope translates to one
`Send*Params` / `Edit*Params` / `Delete*Params` via the channel
adapter, with no new method on the Channel port. The read row breaks
that symmetry — it needs a request/response shape — so it gets its own
future ADR.

The structural question this ADR answers is:

1. **Outbound operations**: how does an Envelope signal "do X to an
   existing message" — where X is edit, delete, or the notification
   ack that ADR-0005 already shipped? An operation is a *verb*, not
   message content. It does not compose what a message says; it acts
   on a message. So it does not belong in `Parts`. Where it does
   belong is the structural question — a new top-level Envelope field
   (mirroring how `Keyboard` left `Parts` in ADR-0005) is the cleanest
   answer that survived scrutiny, and the reasoning is preserved in
   the *Alternatives* section so the smell trail of the rejected
   "operations as PartTypes" design is auditable.
2. **Inbound edits**: how does `Update.EditedMessage` differ from
   `Update.Message` in the resulting Envelope? New Direction? New
   PartType? A Meta marker?
3. **Scope**: does `DeleteMessage` ship in this ADR (it belongs to the
   same category) or does it wait for its own follow-up?
4. **Migration**: the 2E.4 `CallbackAck` shipped as `PartType.CallbackAck`
   because at the time it was the only operation kind. With this ADR
   introducing a clean operation home, that PartType is moved to
   `OperationKind.OpCallbackAck` to keep the model coherent — a small
   but real wire-format change for that one envelope shape (§1.b).

## Decision

Three coupled decisions, each minimum-invasive within its own axis.

### 1. Outbound operations → `Envelope.Operation *Operation` top-level field

Side-effect operations (notifications and mutations alike) are
**verbs**, not content. They get a top-level Envelope field, mirroring
ADR-0005's treatment of `Keyboard` for decoration. `Parts` stays as
the canonical container of message **content**.

```go
type Envelope struct {
    // ... existing fields ...
    Keyboard  *Keyboard  `json:"keyboard,omitempty"`
    Operation *Operation `json:"operation,omitempty"`
}

type Operation struct {
    Kind OperationKind `json:"kind"`
}

type OperationKind int
const (
    OpEditText OperationKind = iota
    OpEditCaption
    OpDelete
    OpCallbackAck
)
```

The reparto is uniform across every kind:

- **`Operation.Kind`** carries the verb.
- **`Parts`** carries the new content, when the operation has any.
- **`Meta`** carries the channel-specific target identifier(s).
- **`Keyboard`** rides along when the operation supports markup.

Per-kind contract:

| Kind | Parts | Keyboard | Target in Meta | Telegram primitive |
|---|---|---|---|---|
| `OpEditText` | one `Text` with non-empty Content (the new body) | optional | `telegram.chat_id` + `telegram.message_id` | `EditMessageTextParams` |
| `OpEditCaption` | one `Text` (Content may be empty to clear the caption) | optional | `telegram.chat_id` + `telegram.message_id` | `EditMessageCaptionParams` |
| `OpDelete` | empty | forbidden | `telegram.chat_id` + `telegram.message_id` | `DeleteMessageParams` |
| `OpCallbackAck` | empty (silent ack) OR one `Text` with non-empty Content (the toast) | forbidden | `telegram.callback_query_id` | `AnswerCallbackQueryParams` |

Four new `OutboundKind`s + four new pointer fields on the `Outbound`
tagged union route each operation to its native primitive:

```go
const (
    // ... existing kinds (Message, Photo, Document, Voice, Audio, Video, Location) ...
    OutboundKindEditText
    OutboundKindEditCaption
    OutboundKindDelete
    OutboundKindAnswerCallback   // unchanged name; routing now via Operation, not PartType
)

type Outbound struct {
    // ... existing fields ...
    EditText       *bot.EditMessageTextParams
    EditCaption    *bot.EditMessageCaptionParams
    Delete         *bot.DeleteMessageParams
    AnswerCallback *bot.AnswerCallbackQueryParams   // unchanged field; route changes upstream
}
```

Validate rules:

- When `Operation == nil`: existing rules apply unchanged.
  `len(Parts) ≥ 1`, exclusivity rules on `Callback` parts, the
  keyboard/parts mixing rules from 2.1–2E.5.
- When `Operation != nil`: per-kind rules above. Parts is permitted to
  be empty **only** when `Operation.Kind ∈ {OpDelete, OpCallbackAck}`
  — a scoped exception to the 2.1 invariant that an Envelope has at
  least one Part. Both exceptions are operations with no body.
- `Keyboard` is forbidden when `Operation.Kind ∈ {OpDelete,
  OpCallbackAck}` (Telegram's DeleteMessage / AnswerCallbackQuery
  primitives have no `ReplyMarkup` field).

Sentinel errors in the Telegram adapter:

- `ErrMissingTargetMessageID` — `OpEditText` / `OpEditCaption` /
  `OpDelete` without `Meta[telegram.message_id]`.
- The existing `ErrMissingChatID` continues to fire when
  `telegram.chat_id` is absent on edit/delete.
- The existing `ErrMissingCallbackQueryID` is reused unchanged for
  `OpCallbackAck` envelopes that lack `Meta[telegram.callback_query_id]`.

### 1.b. Migration of `CallbackAck` from PartType to OperationKind

`CallbackAck` shipped in 2E.4 as `PartType.CallbackAck` — at the time
the only side-effect intent the codebase carried. As soon as a second
side-effect category (mutations) joined it in this ADR, the
"intent-as-PartType" pattern broke down (see the rejection of M3 in
*Alternatives*). The smallest coherent model is to migrate `CallbackAck`
to `OpCallbackAck` now, rather than ship a third inconsistent model.

Wire format change:

- **Before (2E.4):** an ack Envelope marshals as
  `{ "parts": [{"type": 7, "content": "<toast>"}], "meta": {"telegram.callback_query_id": "..."} }`
  where `7` is the `CallbackAck` PartType integer.
- **After (this ADR):** the same Envelope marshals as
  `{ "operation": {"kind": 3}, "parts": [{"type": 0, "content": "<toast>"}], "meta": {"telegram.callback_query_id": "..."} }`
  (silent ack: `"parts": []`). `3` is `OpCallbackAck`; `0` is `Text`.

This is a non-backwards-compatible JSON change for `CallbackAck`
specifically. It is safe to do now because `CallbackAck` was
introduced in 2E.4 and nothing outside the codebase persists or
exchanges these envelopes yet — there is no installed base to
migrate. Stage 3+ persistence work, which has not begun, will start
from the post-migration shape.

Other PartType values (`Text`, `Image`, `Audio`, `Video`, `File`,
`Location`, `Callback`) are **unaffected**: their integer values and
field shapes stay identical. Every 2.1–2E.3 envelope and every
inbound 2E.4 callback envelope round-trips byte-stable before and
after this ADR.

The code surface of the migration is contained:

- `internal/envelope`: remove `PartType.CallbackAck`, its `String()`
  case, its `Validate` case, its entry in
  `validateExclusivePartTypes`, and the `Envelope.AddCallbackAck`
  builder. Add `Operation`, `OperationKind`, `Envelope.Operation`,
  and builder helpers for each kind (`SetEditText`, `SetEditCaption`,
  `SetDelete`, `SetCallbackAck`).
- `internal/channels/telegram`: remove `findCallbackAckPart` and
  `outboundAnswerCallback`. Add a single Operation-dispatch helper
  that handles all four kinds. The behavioural contract is preserved
  end-to-end (silent ack still produces an empty `Text`, toast ack
  still produces the toast text, missing `callback_query_id` still
  yields `ErrMissingCallbackQueryID`).
- Tests from 2E.4 are rewritten to construct ack envelopes through
  the new API; behaviour-level assertions (silent vs toast, missing
  ID, sibling tagged-union fields nil) carry over.

The migration is performed as a single atomic commit so the quality
gate stays green between "remove PartType.CallbackAck" and
"add OpCallbackAck routing" (rather than a transient broken state
on the master branch).

### 2. Inbound edits → reuse the Message path with a Meta marker

`InboundFromUpdate` gains an `EditedMessage` branch that delegates to
the existing `inboundFromMessage` helper, with one structural tweak:
when the source `Message.EditDate > 0`, the resulting Envelope sets

- `Meta[telegram.edited_at] = strconv.Itoa(EditDate)`

and uses `EditDate` (not `Date`) as `Envelope.Timestamp`, because for
an edit the "moment of the event" is when the user edited, not when
they originally sent.

The Envelope's content (Parts) reflects the **current** (edited)
state, exactly as Telegram delivers it. The `telegram.message_id`
Meta entry is the **same** ID as the original message (Telegram
preserves message IDs across edits), so downstream code can correlate
the edit with the original send by looking up its prior Envelope by
that ID. That correlation is the orchestrator's responsibility, not
the adapter's.

This is **not** a new `Direction` and **not** a new `PartType`. The
edit IS a message in intent — the user updated what they said. The
content shape is identical to a fresh message. Adding a new
`PartType.EditedMessage` would force every downstream consumer of
`Parts` to handle a duplicate variant of every content type
(EditedText, EditedImage, ...), which is a tagged-union explosion for
no semantic gain. The single Meta marker carries everything a
consumer needs to know.

Scope for inbound edits in this phase:

- `Update.EditedMessage` is in scope.
- `Update.EditedChannelPost` and `Update.EditedBusinessMessage` are
  **out** of scope and will be added in amending phases when the
  Korvun feature set actually exercises channels and business
  accounts. They share the same `*Message` shape, so the eventual
  addition is a one-line dispatcher branch.

### 3. Scope cut: what does **not** ship in 2E.6

In line with the discipline of ADR-0004 (Location companions),
ADR-0005 (button kinds, ack fields) and 2E.5 (command extras):

- **`EditMessageReplyMarkup`** (markup-only edits). Telegram's edit
  primitives all accept a `ReplyMarkup` field, so a caller that wants
  to replace only the keyboard while keeping the text can simply send
  an EditText envelope with the same text content. The dedicated
  markup-only primitive saves a network round-trip in that narrow
  case; it does not unlock any new capability. **Deferred** to an
  amending ADR when a real perf-sensitive caller asks for it.
- **`DeleteMessages` (bulk)**. The single-message `DeleteMessage`
  covers the canonical case; bulk delete is a 100-at-a-time batch
  optimisation that has no caller demand yet. **Deferred**.
- **`InlineMessageID` target**. The XOR alternative to
  `(ChatID, MessageID)` only makes sense for messages sent via
  Telegram's inline-query mode (when the bot is invoked as
  `@botname query`), which Korvun does not yet support on either
  inbound or outbound. **Deferred** to whatever ADR adds inline mode
  support.
- **`EditMessageMedia`** (replacing the media itself, not just the
  caption). Conceptually different from EditCaption: needs the same
  `InputFile` plumbing as fresh media outbound, plus an `InputMedia*`
  wrapper. **Deferred** to a follow-up ADR specifically about media
  edits.
- **Read operations** (`GetFile`, `GetChat`, `GetMe`,
  `GetMyCommands`). These break the Envelope-outbound pattern and
  need a separate read primitive on the Channel port. **Deferred** to
  a future ADR (working title "ADR-0007: read operations on the
  Channel port").
- **Inbound `EditedChannelPost` / `EditedBusinessMessage`**. Same
  shape as `EditedMessage`, deferred until a real caller exercises
  those contexts (see §2 above).

These exclusions are not gaps; they are scope discipline. Each item
gets its own ADR when a real use case provides the evidence for the
right design.

## Consequences

### What this changes

**`internal/envelope`:**

- New top-level field `Envelope.Operation *Operation` (optional,
  `omitempty`).
- New `Operation` struct + `OperationKind` enum
  (`OpEditText`, `OpEditCaption`, `OpDelete`, `OpCallbackAck`).
- New builder helpers, one per kind:
  `Envelope.SetEditText(text string) *Envelope`,
  `Envelope.SetEditCaption(caption string) *Envelope`,
  `Envelope.SetDelete() *Envelope`,
  `Envelope.SetCallbackAck(toast string) *Envelope`.
  Naming uses `Set*` rather than `Add*` because Operation is
  at-most-one per Envelope (mirrors `WithKeyboard`).
- New `Validate` rules for `Operation != nil`, branching per kind
  per the Decision §1 contract table.
- Existing `validateExclusivePartTypes` is **simplified**, not
  extended: the `CallbackAck` entry is removed (now handled by
  Operation), and no edit/delete PartType ever enters it. The rule
  list shrinks to `{Callback}` — interaction events that are still
  legitimately PartTypes.
- Removal of `PartType.CallbackAck`, its `String()` case, its
  `Validate` case, and `Envelope.AddCallbackAck`. See §1.b for the
  migration rationale.

**`internal/channels/telegram`:**

- `InboundFromUpdate` dispatches `u.EditedMessage` to
  `inboundFromMessage`. The helper learns the `EditDate > 0` branch:
  set `Meta[telegram.edited_at]` and use `EditDate` for
  `Envelope.Timestamp`.
- `OutboundParams` learns a single Operation-dispatch branch
  (checked before the existing classify-parts pipeline, the same way
  the CallbackAck PartType was checked in 2E.4):
  - `OpEditText` → `*bot.EditMessageTextParams{ChatID, MessageID,
    Text, ReplyMarkup}`. `ReplyMarkup` from `Envelope.Keyboard`.
  - `OpEditCaption` → `*bot.EditMessageCaptionParams{ChatID,
    MessageID, Caption, ReplyMarkup}`.
  - `OpDelete` → `*bot.DeleteMessageParams{ChatID, MessageID}`. No
    keyboard.
  - `OpCallbackAck` → `*bot.AnswerCallbackQueryParams{CallbackQueryID,
    Text}`. The pre-2E.6 `findCallbackAckPart` / `outboundAnswerCallback`
    helpers are removed; the new dispatch handles this case alongside
    the three mutations.
- Edit/delete routings require `telegram.message_id` and
  `telegram.chat_id` in Meta; `ErrMissingTargetMessageID` on absence
  of the former, the existing `ErrMissingChatID` on absence of the
  latter. CallbackAck routing reuses the existing
  `ErrMissingCallbackQueryID`.
- New `OutboundKind` constants: `OutboundKindEditText`,
  `OutboundKindEditCaption`, `OutboundKindDelete`.
  `OutboundKindAnswerCallback` (introduced in 2E.4) keeps its name
  and integer position; only its dispatch trigger changes from
  PartType to Operation.
- New `Outbound` fields: `EditText *bot.EditMessageTextParams`,
  `EditCaption *bot.EditMessageCaptionParams`,
  `Delete *bot.DeleteMessageParams`. The existing `AnswerCallback`
  field is preserved verbatim.
- New Meta constant `MetaEditedAt = "telegram.edited_at"`.

### What this does NOT change

- **No new `Direction`.** Edits remain inbound; mutations remain
  outbound. The duality is preserved.
- **No change to `Part` struct layout.** Same `{Type, Content, Source,
  MIMEType}` fields used since 2.1.
- **No change to `Parts` semantics for non-operation envelopes.** An
  Envelope with `Operation == nil` behaves exactly as it did through
  2E.5 — `Parts` enumerates the message content, full stop.
- **No new `PartType` values for operations.** The pre-2E.6
  `PartType.CallbackAck` migrates out (§1.b); no new operation
  PartTypes are added. `PartType` is now exclusively a
  content-classification enum, restoring the discipline ADR-0005
  promised but did not fully achieve.
- **No mandatory adapter rewrite for non-operation channels.** The
  webhook adapter (Phase 2.2) keeps its `default:` fallback. Envelopes
  whose `Operation` is non-nil simply don't reach its outbound path —
  the webhook adapter doesn't subscribe to operation semantics, and
  any consumer that hands it an operation envelope is making a
  routing error the adapter is allowed to refuse.

### JSON wire format impact

Three categories of change:

1. **New optional top-level key `"operation"`** on Envelope. Envelopes
   that don't carry one (every Envelope produced by 2.1–2E.5 except
   the 2E.4 CallbackAck path) marshal byte-identically: `*Operation`
   nil + `omitempty` = absent key.
2. **No changes to existing PartType integers** except the removal
   described in §1.b. `Text` (0), `Image` (1), `Audio` (2), `Video`
   (3), `File` (4), `Location` (5), `Callback` (6) keep their iota
   positions. The pre-2E.6 `CallbackAck = 7` slot is freed; no new
   PartType reuses it.
3. **Non-backwards-compatible change for 2E.4 CallbackAck envelopes
   only**, fully described in §1.b. Mitigated by the absence of an
   installed base — Stage 3+ persistence starts from the post-migration
   shape.

Every 2.1–2E.3 envelope and every inbound 2E.4 Callback envelope
(both production-relevant categories) round-trips byte-stable.

### Coverage impact

New code paths to be covered by red tests first:

1. `OperationKind.String()` for the four kinds — a new four-row table.
2. `Validate` per `OperationKind` (the §1 contract table, one
   sub-table per kind):
   - `OpEditText`: requires exactly one Text Part with non-empty
     Content; rejects empty Parts, non-Text Parts, multiple Parts.
   - `OpEditCaption`: requires exactly one Text Part (Content may be
     empty); rejects empty Parts, non-Text Parts, multiple Parts.
   - `OpDelete`: requires empty Parts and nil Keyboard; rejects either
     being non-empty.
   - `OpCallbackAck`: allows empty Parts (silent) OR one Text Part
     with non-empty Content (toast); rejects multiple Parts, non-Text
     Parts, empty-Content Text Part (use empty Parts for silent
     instead), nil Keyboard required.
3. Builder round-trip for `SetEditText`, `SetEditCaption`, `SetDelete`,
   `SetCallbackAck`, including chaining and JSON preservation through
   `Marshal/Unmarshal`. Plus a "Operation absent ⇒ omitempty hides
   the key" round-trip assertion (analogous to the Keyboard
   round-trip).
4. Telegram inbound: `EditedMessage` produces an Envelope whose
   `Timestamp` is `EditDate` and whose `Meta[telegram.edited_at]` is
   the stringified `EditDate`. Existing Message path coverage stays
   untouched.
5. Telegram outbound: each of the four operations — happy path (with
   and without Keyboard where applicable), missing
   `telegram.message_id` for edit/delete, missing `telegram.chat_id`,
   sibling tagged-union fields nil, behavioural parity with the 2E.4
   ack contract (silent ack still empty `Text`, toast ack still
   carries the toast).
6. Migration regression suite: every 2E.4 CallbackAck behavioural
   assertion (silent, toast, missing ID, no chat ID required) is
   re-expressed against the new API. The behavioural contract is
   what survives — the wire-shape assertions are updated to the new
   shape.

Estimate: ~16 new/migrated domain tests + ~10 adapter tests
(including migrated 2E.4 ack tests), no decrease in coverage below
95 %. The 95 % budget has held through every prior 2E phase.

### Trade-offs accepted

- **The 2.1 invariant "every Envelope has ≥1 Part" gains a scoped
  exception.** When `Operation.Kind ∈ {OpDelete, OpCallbackAck}`,
  Parts is allowed to be empty. The exception is well-fenced: only
  these two kinds, only because they semantically have no body.
  Every other Envelope, operation or not, retains the original
  invariant. Validate enforces this precisely.
- **The Envelope grows a second top-level optional field (Operation),
  after Keyboard in 2E.5.** This is a deliberate step — both fields
  carry information that is not message content. The trade-off is
  bounded: top-level fields are reserved for "things that are not
  Parts but attach to a message" (decoration, operation intent).
  Future concepts get evaluated on this criterion before adding a
  third top-level field.
- **Target identification stays channel-prefixed in Meta.** A future
  second channel that supports edits (Slack, Discord) will likely
  have its own ID format and its own Meta keys. Promotion to a
  canonical `message.target_id` would be a future ADR with evidence
  from a second channel — same discipline as commands in 2E.5.
- **CallbackAck wire format changes non-backwards-compatibly for
  envelopes already produced in 2E.4.** Bounded by the absence of an
  installed base; see §1.b. The migration is intentional architectural
  cleanup, not accidental drift.
- **No tagged-union sub-structs inside Operation.** A more elaborate
  design would expose `Operation { Kind, EditText *EditTextOp,
  EditCaption *EditCaptionOp, ... }` so each kind carries typed
  payload structs. The current design defers that: the per-kind data
  lives in `Parts` (content) or `Meta` (target identifier), both of
  which already exist. Adding sub-structs duplicates plumbing
  without unlocking new capability. If a future operation kind
  arrives whose payload genuinely doesn't fit into Parts/Meta (e.g.
  an EditMedia carrying an `InputMedia*` wrapper), that ADR can
  introduce per-kind sub-structs without breaking this one.

### Scope of this ADR

This ADR decides **only** the modelling of message edit (text and
caption) and message delete on Telegram, and the side-effect
taxonomy that justifies why those mutations fit the Envelope
outbound pipeline. It explicitly does **not** decide:

- **Read operations** (`GetFile`, `GetChat`, `GetMe`,
  `GetMyCommands`). A future ADR will design the read primitive on
  the Channel port. The taxonomy in this ADR's Context section is
  the conceptual groundwork for that decision.
- **Bulk delete**, **InlineMessageID**, **EditMessageReplyMarkup**,
  **EditMessageMedia**, **inbound `EditedChannelPost` /
  `EditedBusinessMessage`** — see the scope cut in Decision §3.
- **The orchestrator-level correlation** between an edit Envelope and
  the original send Envelope. This ADR ships the data
  (`telegram.message_id` is identical on both); building a lookup
  table or correlation index is an orchestrator concern decided in
  the appropriate stage (Stage 3+).

## Alternatives Considered

### Outbound mutations, Option M1 — single `PartType.Edit` with a Meta intent hint

```go
env.AddEdit("new text or caption")
env.Meta[MetaEditField] = "text"     // or "caption"
```

**Rejected.** Three reasons:

1. **Hidden-state coupling.** The Meta hint becomes a second
   discriminator that must be kept in sync with the Content. A caller
   that forgets to set it gets silent miscompilation into the wrong
   Edit* primitive.
2. **Worse symmetry.** Other intents in the codebase (Callback,
   CallbackAck, Location) all wear their meaning on the PartType.
   Edit would become the odd one whose meaning is partly in PartType
   and partly in Meta.
3. **Future expansion is harder.** Adding an EditMessageMedia path
   later would mean a third Meta value (`"media"`) and a string-typed
   enum hidden in the Meta map. A dedicated `EditMedia` PartType in a
   future ADR is the cleaner extension point.

### Outbound mutations, Option M2 (CHOSEN) — new top-level `Envelope.Operation *Operation`

```go
type Envelope struct {
    // ... existing fields ...
    Operation *Operation `json:"operation,omitempty"`
}
type Operation struct {
    Kind OperationKind `json:"kind"`
}
```

See *Decision §1*. Wins because:

- **Operations are verbs, not content.** `Parts` enumerates *what the
  message contains*. An operation does not compose a message; it acts
  on one. Putting an operation inside `Parts` was the structural smell
  ADR-0005 caught for `Keyboard` and the same argument applies here
  with more force — Delete has no content at all. Top-level placement
  honours the same discipline.
- **`Parts` invariant survives across operation envelopes too.** When
  `Operation` carries content (edit text, edit caption, ack toast),
  that content rides as a normal Text Part — the natural home for
  text. When the operation has no body (`OpDelete`, silent
  `OpCallbackAck`), Parts is empty by a scoped exception that does
  not pollute the meaning of `Parts` itself.
- **Exclusivity logic simplifies.** The pre-2E.6
  `validateExclusivePartTypes` rule list shrinks (it loses the
  `CallbackAck` entry and never grows for edit/delete) because
  operation envelopes are recognised by `Operation != nil`, not by
  scanning Parts for operation-flavoured types.
- **Symmetric with `Keyboard`.** Both are top-level optional fields
  attached to messages, both honour the "not content" criterion, and
  both let consumers decide quickly ("is there a keyboard? is there
  an operation?") without scanning Parts.

The initial framing of this Option in the first draft of this ADR
("duplicates Meta, worse pipeline fit") was wrong on both counts: the
target identifier lives in Meta in *every* option, so there is no
differential duplication; and a single `if env.Operation != nil`
branch at the top of `OutboundParams` is no more complex than a Parts
scan. That framing reflected attachment to the M3 design below; it
did not reflect the actual cost.

### Outbound mutations, Option M3 (REJECTED, was CHOSEN in first draft) — three new PartTypes

```go
const (
    // ... existing PartTypes ...
    EditText
    EditCaption
    Delete
)
```

**Rejected on reconsideration.** Three reasons that together are
decisive:

1. **Conflates content with operation in the same enum.** `PartType`
   started as a content-classification enum (Text, Image, ...). Adding
   intent kinds to it (and the 2E.4 precedent of `CallbackAck` is the
   same mistake) mixes two unrelated categories under a single
   discriminator. Every consumer of `Parts` then has to filter
   operation PartTypes out before treating Parts as content.
2. **`Delete` with empty Content is a smell, not a quirk.** The first
   draft's trade-off note ("Delete envelopes look slightly odd at
   first glance") was the diagnostic: a Part with no content is not
   a Part. The exclusivity rule list growing to keep Delete from
   mixing with real content is the second diagnostic.
3. **Breaks the discipline ADR-0005 introduced.** Keyboard came out
   of `Parts` because it is not content. Operations are *less*
   content than Keyboard (Keyboard at least is concrete data; an
   operation is a verb on a target identified elsewhere). Putting
   operations back into `Parts` would have meant the precedent was
   case-by-case rather than principled.

This option survives in the ADR as a documented reject precisely so a
future reader sees the smell trail. The decisive critique that
forced reconsideration is preserved verbatim in the
[ADR-0006 review thread] (it explicitly identified the conflation
of intent with content as a category error).

### Outbound mutations, Option M4 — separate "side-effect" primitive on the Channel port

```go
type Channel interface {
    InboundFromUpdate(...) (*Envelope, error)
    OutboundParams(...) (*Outbound, error)
    Dispatch(intent SideEffectIntent) (...)  // new
}
```

**Rejected.** Same reasoning as ADR-0005 rejecting Option A2 for the
ack: adding a third method just for mutations would (a) double the
port's surface for downstream components that today only need two
methods, (b) duplicate plumbing the existing `OutboundParams` already
provides, and (c) prejudice the design of the read primitive that
genuinely DOES need a separate method. Reusing the Envelope-outbound
pipeline keeps mutations within the existing pattern and reserves the
new-method surface for the read primitive when it lands.

### Inbound edits, Option E1 — new `PartType.EditedText` / `EditedImage` / ...

A duplicate PartType for every existing content type, marking it as
"this is an edit, not original".

**Rejected.** Tagged-union explosion: every existing content type
doubles. Every consumer of `Parts` either has to switch on a 2×N
PartType space or filter early. The Meta-marker approach gives the
same information with zero new PartTypes.

### Inbound edits, Option E2 (CHOSEN) — Meta marker + Timestamp shift

See *Decision §2*. Wins because:

- One scalar Meta entry replaces N new PartType variants.
- Consumers that care about edits read one Meta key; consumers that
  don't see a regular Envelope.
- `Envelope.Timestamp = EditDate` makes downstream "when did this
  event arrive" semantics correct without any new field.

### Inbound edits, Option E3 — new `Direction.Edited`

**Rejected.** Same reasoning as ADR-0005 rejecting `Direction.Callback`:
breaks the in/out duality, forces non-exhaustive switches in router /
brains / policy. An edit IS inbound; modelling it as a third Direction
conflates "where it came from" with "what kind of event it is".

### Scope, Option S1 — defer Delete to a follow-up ADR

**Rejected.** Delete shares the exact same architectural shape as edit
(target by `(chat_id, message_id)` in Meta, no body, one-Envelope-to-
one-`*Params`). Splitting the ADR would leave the side-effect
taxonomy half-explained and produce a thin ADR-0007 with no design
content beyond "same as ADR-0006 but with `DeleteMessageParams`".
Bundling them keeps the conceptual unit intact.

### Scope, Option S2 (CHOSEN) — Delete in this ADR, others deferred

See *Decision §3*. Bulk delete, markup-only edit, inline messages,
media edits, channel/business edited posts, and all read operations
each get their own future ADR or amending ADR when a real use case
arrives.
