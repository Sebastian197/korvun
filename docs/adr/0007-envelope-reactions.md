# ADR-0007: Envelope reactions — outbound `OpSetReaction` + inbound `PartType.Reaction`

> **Status:** accepted
> **Date:** 2026-06-14
> **Deciders:** Sebastián Moreno Saavedra
> **Amends:** [ADR-0006](0006-envelope-side-effect-operations.md)

## Context

Phase 2E.7 of Stage 2-EXT adds support for **emoji reactions**: the bot
puts a reaction on a previously-sent message (outbound), and the bot
learns when a user reacts to one of its messages (inbound). Verified
against the pinned library version (`github.com/go-telegram/bot
v1.21.0`) via Context7 + `go doc`:

- **Outbound primitive**: `bot.SetMessageReactionParams{ChatID,
  MessageID, Reaction []models.ReactionType, IsBig *bool}` →
  `bot.Bot.SetMessageReaction(ctx, *) (bool, error)`. Passing
  `Reaction: []ReactionType{}` clears all the bot's reactions on the
  target message (the same endpoint is used for both set and clear).
  There is no standard `deleteMessageReaction` Bot API method;
  removal is "set to empty".
- **Inbound update kinds**: `Update.MessageReaction
  *MessageReactionUpdated` (a specific user's reaction change) and
  `Update.MessageReactionCount *MessageReactionCountUpdated`
  (anonymised aggregate for large chats).
  `MessageReactionUpdated{Chat, MessageID, User, ActorChat, Date,
  OldReaction []ReactionType, NewReaction []ReactionType}`.
- **`models.ReactionType`** is a **discriminated union** with three
  variants and a custom `MarshalJSON`/`UnmarshalJSON` pair:

  ```go
  type ReactionType struct {
      Type                    ReactionTypeType
      ReactionTypeEmoji       *ReactionTypeEmoji      // standard emoji
      ReactionTypeCustomEmoji *ReactionTypeCustomEmoji
      ReactionTypePaid        *ReactionTypePaid
  }
  ```

ADR-0006 set the architectural framework for side-effect operations.
This ADR is a small amendment that places **outbound reaction setting**
into that framework (one more `OperationKind`) and adds the
**inbound reaction event** alongside the existing `Callback`
PartType. The architectural reasoning behind both moves was already
done in ADR-0005 / ADR-0006; this ADR only justifies why **reactions
fit the existing pattern** rather than introducing anything new.

The structural questions this ADR answers are:

1. **Outbound modelling**: setting reactions is a mutation of an
   existing message's state. Does it fit `Envelope.Operation` (yes,
   per the ADR-0006 taxonomy), and if so, what does the per-kind
   contract look like?
2. **Inbound modelling**: a reaction event is reported by Telegram as
   a distinct `Update` kind. Does it fit a new `PartType` (parallel to
   the `Callback` precedent), or does it deserve a new top-level
   field on Envelope? The asymmetry "outbound is `Operation`, inbound
   is `PartType`" needs to be argued explicitly, not assumed.
3. **Inbound diff semantics**: Telegram delivers an `OldReaction` /
   `NewReaction` pair. What does the resulting Envelope's `Parts`
   slice contain — the new state, the old state, the delta? How does
   a downstream consumer tell "added" from "removed" from "changed"?
4. **ReactionType discriminated union**: how does the adapter handle
   inbound `custom_emoji` and `paid` reactions when the dominio only
   models the `emoji` variant in this phase?

## Decision

### 1. Outbound reactions → new `OperationKind.OpSetReaction`

Setting reactions is a state mutation on an existing message,
addressed by `(chat_id, message_id)` — exactly the side-effect
mutation shape that ADR-0006 modelled for `OpEditText`,
`OpEditCaption`, and `OpDelete`. Add one more kind to the existing
`OperationKind` enum:

```go
const (
    OpEditText OperationKind = iota
    OpEditCaption
    OpDelete
    OpCallbackAck
    OpSetReaction
)
```

Per-kind contract (extends the ADR-0006 Decision §1 table):

| Field | Rule for `OpSetReaction` |
|---|---|
| `Parts` | 0 or more `Text` Parts. Each non-empty Content is interpreted as exactly one emoji character (or grapheme cluster). Empty `Parts` means "clear all of the bot's reactions on the target". |
| `Keyboard` | forbidden. `SetMessageReactionParams` has no `ReplyMarkup` field. |
| `Meta` target | `telegram.chat_id` + `telegram.message_id`, validated by the adapter (`ErrMissingChatID` / `ErrMissingTargetMessageID`). |
| Telegram primitive | `*bot.SetMessageReactionParams{ChatID, MessageID, Reaction}`. `IsBig` is not modelled by this ADR (see scope cut). |
| Validate rule | `Source` and `MIMEType` must be empty on every Part; `Type` must be `Text`; `Content` must be non-empty on every Part (an "empty emoji" slot is rejected to keep the Parts slice meaningful). |

A new builder helper:

```go
func (e *Envelope) SetReactions(emojis ...string) *Envelope
```

Variadic for ergonomics: `e.SetReactions("👍")`, `e.SetReactions("👍",
"🎉")`, `e.SetReactions()` (clear all). Replaces any pre-existing Parts
and Operation, mirroring the existing `Set*` builders.

Adapter dispatch: `outboundOperation` gets one new branch routing
`envelope.OpSetReaction` to a new `outboundSetReaction` helper that
parses `chat_id` + `message_id` (reusing `parseChatID` /
`parseTargetMessageID`), translates each Text Part to a
`models.ReactionType` of variant `emoji`, and returns a new
`OutboundKindSetReaction` populating
`*bot.SetMessageReactionParams`. A new `SetReaction
*bot.SetMessageReactionParams` field joins the `Outbound` tagged
union.

### 2. Inbound reactions → new `PartType.Reaction`

When Telegram delivers `Update.MessageReaction`, the adapter produces
an `Envelope` with `Direction = Inbound`, the reacting user as
`Sender`, and one or more **`Reaction` Parts**, each containing one
emoji as Content. The `MessageReactionUpdated.Chat.ID` and
`.MessageID` populate the existing `telegram.chat_id` and
`telegram.message_id` Meta keys, so downstream code can correlate the
reaction with the original message that received it.

A new `PartType` constant:

```go
const (
    Text PartType = iota
    Image
    Audio
    Video
    File
    Location
    Callback
    Reaction   // NEW
)
```

Builder:

```go
func (e *Envelope) AddReaction(emoji string) *Envelope
```

Appends a single `Reaction` Part with `Content: emoji`. Multiple
calls add multiple Parts, mirroring how `AddText` / `AddMedia` work
for content.

Validate rule for `Reaction` Parts: non-empty `Content`, no `Source`,
no `MIMEType`. **No exclusivity rule** — multiple `Reaction` Parts can
coexist within the same Envelope (the user can have multiple
reactions at once on a Premium account). Reaction Parts must not be
mixed with non-Reaction Parts (a reaction event has no text body,
media, location, callback, etc. alongside it); `validateExclusivePartTypes`
gains `Reaction` to its list of types that must be the sole PartType
in the Envelope when present. The existing rule "the part must be the
only Part" relaxes to "all parts must be the same Reaction type" when
the type is `Reaction`.

### 3. Why the asymmetry (outbound `Operation`, inbound `PartType`) is the right call

This same asymmetry already exists for callbacks: ADR-0005 + ADR-0006
ship `OpCallbackAck` (outbound) and `PartType.Callback` (inbound),
and the precedent is correct for reactions for the same reason — but
the ADR must argue it rather than assume it, because the alternative
("symmetry: both go in the same slot") is a tempting default that
would be wrong.

The argument:

- **Outbound is an instruction the bot issues** (a verb). "Set my
  reactions on message X to [emojis]." It mutates external state. It
  has no message body in the conversational sense — the emojis are
  parameters of the action, not content the user sees as a message.
  This is the side-effect mutation shape that ADR-0006 designed
  `Operation` for, and reactions fit it identically to edit/delete.
- **Inbound is an event the bot observes** (a report). "User U just
  changed their reactions on message X to [emojis]." From the
  Envelope's vantage point, the emoji set IS the content of the
  event — there's nothing else the event carries beyond who, when,
  and which emojis. That makes the emojis ordinary content, which is
  what `Parts` is for. Treating it as an `Operation` instead would
  mean the inbound Envelope has an `Operation` field set, which by
  the ADR-0006 contract signals "an outbound instruction" — wrong
  direction.

The "both go in the same slot" alternative is rejected for the same
reason ADR-0005 rejected it for keyboards: forcing symmetry between
two semantically different categories costs more than the cognitive
gain of having one shape. The `Callback` / `CallbackAck` precedent
proves this works in practice — every consumer of the codebase has
been able to keep the distinction clean since 2E.4 / 2E.6.

### 4. Inbound diff semantics — what `Parts` contains and how `action` is derived

Telegram's `MessageReactionUpdated` delivers two slices: `OldReaction`
(before) and `NewReaction` (after). The adapter compares the
**emoji-filtered** versions of those slices (see §5) and produces one
of three actions plus the Parts contents:

| Telegram delivers (emoji-filtered) | Envelope `Meta[telegram.reaction_action]` | Envelope `Parts` |
|---|---|---|
| `Old=[]`, `New=["👍"]` | `"added"` | one Reaction Part per emoji in `New` |
| `Old=["👍"]`, `New=[]` | `"removed"` | one Reaction Part per emoji in `Old` |
| `Old=["👍"]`, `New=["❤️"]` (any change, both sides non-empty) | `"changed"` | one Reaction Part per emoji in `New`; the previous emojis ride in `Meta[telegram.reaction_previous]` as a comma-separated string (`"👍"`) |
| `Old==New` after filtering (no effective change) | event dropped — adapter returns `ErrUnsupportedContent` | n/a |

The rule **"`Parts` always contains the emojis that the action
references"** keeps a consistent semantic: for `"added"` and
`"changed"`, that's the new set; for `"removed"`, that's the set that
just disappeared. A downstream consumer that wants "what changed in
this event" always reads `Parts` and gets useful data.

For the `"changed"` case, `Meta[telegram.reaction_previous]` is
present and contains the old emoji set (comma-separated, no spaces:
`"👍,❤️"` for a two-emoji previous state). For `"added"` and
`"removed"`, this key is absent (the information is implicit in the
action and Parts).

**The "Parts may be empty" invariant exception** introduced in
ADR-0006 (for `OpDelete` and silent `OpCallbackAck`) does NOT apply
to inbound Reaction Envelopes. Every Reaction event delivered to
downstream has at least one Reaction Part — the "no-op" case is
dropped at the adapter seam with `ErrUnsupportedContent`, never
delivered upward.

### 5. Handling the `ReactionType` discriminated union — emoji-only modelling

`ReactionType` carries three variants (`emoji`, `custom_emoji`,
`paid`). This ADR models **only the `emoji` variant** in both
directions. The handling of the other two is:

- **Outbound**: the `Envelope.SetReactions` builder only accepts
  emoji strings (one emoji per variadic arg). There is no API for
  emitting `custom_emoji` or `paid` reactions. Adding them is
  future amending-ADR territory.
- **Inbound**: when the adapter receives `OldReaction` / `NewReaction`
  slices, it **filters out** every `ReactionType` whose `Type` is not
  `ReactionTypeTypeEmoji`. Custom-emoji and paid reactions in the
  incoming payload are silently dropped before the diff is computed.
  This means:
  - A user whose only reaction is `custom_emoji` is invisible to the
    adapter — no Envelope produced (the filtered slices are empty
    on both sides).
  - A user with mixed reactions (`emoji` + `custom_emoji`) yields an
    Envelope that only mentions the `emoji` ones.
  - A `paid` reaction (Telegram's "stars" feature) is invisible.

The drop is **silent and deliberate**: the adapter doesn't refuse
the update, it just doesn't surface the parts of it that the dominio
doesn't model. A future ADR that adds custom_emoji or paid support
will turn the drop into a surface and may introduce new PartType
constants (e.g. `CustomReaction`, `PaidReaction`) or extend
`Content` with a typed prefix — that decision can wait for evidence.

The adapter does not panic on the union: the library's
`ReactionType.MarshalJSON` / `UnmarshalJSON` handle the on-the-wire
shape; the adapter reads the `Type` field of each entry and routes
accordingly.

## Consequences

### What this changes

**`internal/envelope`:**

- One new `OperationKind` constant: `OpSetReaction`. Add a `String()`
  case (`"set_reaction"`).
- One new `PartType` constant: `Reaction`. Add a `String()` case
  (`"reaction"`).
- One new outbound builder: `Envelope.SetReactions(emojis ...string)
  *Envelope`. Empty variadic means "clear all"; replaces any prior
  Parts and Operation.
- One new inbound builder: `Envelope.AddReaction(emoji string)
  *Envelope`. Appends a single Reaction Part.
- `validateOperation` gains a case for `OpSetReaction`: every Part is
  Text with non-empty Content, no Source/MIMEType, Keyboard
  forbidden. The Parts-may-be-empty exception from ADR-0006 extends
  to `OpSetReaction` (empty = clear all).
- `validatePart` gains a case for `Reaction`: non-empty Content, no
  Source/MIMEType.
- `validateExclusivePartTypes` adds `Reaction` to the exclusive list:
  when any Reaction Part appears, every Part in the Envelope must be
  a Reaction Part. (This is a refinement of the existing
  Callback-exclusivity rule, not a replacement: Callback parts are
  still solo, Reaction parts can be multiple-of-the-same-kind.)

**`internal/channels/telegram`:**

- `outboundOperation` gains the `envelope.OpSetReaction` branch
  dispatching to a new `outboundSetReaction` helper.
- `outboundSetReaction` parses `chat_id` + `message_id` (reuses
  `parseChatID` / `parseTargetMessageID`), maps each Text Part's
  Content to a `models.ReactionType{Type: ReactionTypeTypeEmoji,
  ReactionTypeEmoji: &ReactionTypeEmoji{Type:
  ReactionTypeTypeEmoji, Emoji: <content>}}`, and returns
  `*bot.SetMessageReactionParams`.
- `InboundFromUpdate` gains a `u.MessageReaction != nil` branch
  routing to a new `inboundFromMessageReaction` helper.
- `inboundFromMessageReaction` builds the Sender from
  `mr.User`, populates `telegram.chat_id` /
  `telegram.message_id` from `mr.Chat.ID` / `mr.MessageID`, uses
  `mr.Date` (`time.Unix`) for `Envelope.Timestamp`, applies the
  emoji filter to `OldReaction` and `NewReaction`, computes the
  action and Parts per §4, and stamps
  `telegram.reaction_action` (and `telegram.reaction_previous` for
  `"changed"`).
- New `OutboundKindSetReaction` constant + `SetReaction
  *bot.SetMessageReactionParams` field on `Outbound`.
- New Meta constants: `MetaReactionAction = "telegram.reaction_action"`,
  `MetaReactionPrevious = "telegram.reaction_previous"`.
- Adapter sentinel errors: reuses `ErrMissingChatID` /
  `ErrMissingTargetMessageID` (outbound) and `ErrUnsupportedContent`
  (inbound: no emoji on either side after filtering, or no actual
  change).

### What this does NOT change

- **No new `Direction`.** Inbound reactions are inbound; outbound
  reaction setting is outbound. The duality is preserved (same
  argument that excluded `Direction.Callback` in ADR-0005 and
  `Direction.Edited` in ADR-0006).
- **No new top-level Envelope field.** `Operation` (ADR-0006) and
  `Keyboard` (ADR-0005) remain the only two non-Parts top-level
  optional fields.
- **No change to the `Part` struct layout.** Same `{Type, Content,
  Source, MIMEType}` from 2.1.
- **No change to the existing `OpDelete` / silent `OpCallbackAck`
  "Parts may be empty" exception** — `OpSetReaction` extends the same
  rule (the only `Operation` kind that didn't before).
- **No change to the existing `Update.MessageReactionCount` path** —
  it is explicitly out of scope (§Scope), so the inbound dispatcher
  simply ignores `u.MessageReactionCount`.

### JSON wire format impact

- `PartType.Reaction` adds a new integer value at the next available
  position (8, after `Callback = 6` since the `CallbackAck = 7` slot
  was retired by ADR-0006). Older parsers tolerate unknown values
  exhaustively at the receiving end; this is the same forward-compat
  promise as every prior PartType addition.
- `OperationKind.OpSetReaction` adds a new integer value at position
  4 (after `OpCallbackAck = 3`). Same forward-compat.
- **No new top-level keys** on `Envelope`. Every envelope produced by
  2.1–2E.6 marshals byte-identically before and after this ADR.

### Coverage impact

New code paths to be covered by red tests first:

1. `PartType.String()` for `Reaction` — one row added to the existing
   table.
2. `OperationKind.String()` for `OpSetReaction` — one row added.
3. `Validate` rules for `OpSetReaction`:
   - Happy paths: 0 Parts (clear), 1 emoji Part, multiple emoji
     Parts, optional Keyboard rejected.
   - Errors: non-Text Part type, empty-Content Text Part, non-empty
     Source/MIMEType, Keyboard present.
4. `Validate` rules for `Reaction` PartType:
   - Happy paths: single Reaction Part, multiple Reaction Parts,
     valid envelope shape.
   - Errors: empty Content, non-empty Source/MIMEType, mixed with
     non-Reaction Parts.
5. JSON round-trip of envelopes carrying `Reaction` Parts and
   `OpSetReaction` Operations, plus the negative round-trip
   (operation/PartType absent when Operation/Reaction not used).
6. Builders `Envelope.SetReactions` and `Envelope.AddReaction` —
   chaining, replace-on-set semantics for the outbound builder.
7. Telegram inbound: `MessageReaction` fixture covering the three
   actions plus the no-op drop. Plus the emoji-filter cases:
   `custom_emoji` ignored, `paid` ignored, mixed types reduced to
   the emoji subset.
8. Telegram outbound: set, clear (variadic empty), multiple emojis,
   missing `chat_id` / `message_id` errors, sibling tagged-union
   fields nil.

Estimate: ~12 new domain tests + ~10 adapter tests, no decrease in
coverage below 95 %. Aligns with the budget held through every prior
2E phase.

### Trade-offs accepted

- **The asymmetry "outbound `Operation`, inbound `PartType`" is now
  applied to two semantic categories** (interaction events:
  callbacks; reactions). The asymmetry was justified for callbacks
  in ADR-0005 / ADR-0006 and reapplied here with the same argument.
  If a third inbound user-event kind appears that does NOT fit this
  pattern (e.g. an interaction whose payload is structured rather
  than text-leaf), it will warrant its own ADR rather than being
  forced into `PartType`.
- **`validateExclusivePartTypes` gains a per-PartType nuance.**
  `Callback` is "must be the only Part"; `Reaction` is "all Parts
  must be `Reaction`". The exclusivity helper now distinguishes
  "exactly one" from "all the same kind", which is a small extra
  rule but bounded — both are exclusivity, just at different
  granularities.
- **Silent drop of `custom_emoji` and `paid` reactions** on inbound.
  Documented and bounded; the alternative (refusing the entire
  Update) would suppress legitimate emoji reactions that ride
  alongside non-emoji ones. The drop is deliberate scope discipline,
  not accidental data loss.
- **`Meta[telegram.reaction_previous]` is a comma-separated emoji
  string**, not a structured array. The string shape is sufficient
  for the action-detection use case and consistent with Meta's
  scalar-string contract from ADR-0001. A future ADR may promote
  this to a richer encoding if needed.
- **`MessageReactionCount` (aggregated, anonymised reactions on
  groups) is out of scope.** It carries a different shape
  (`Reactions []ReactionCount` with totals, no per-user data) and
  serves a different downstream use case (group analytics, not
  per-user interaction). Modelling it would need its own
  abstractions (probably a separate `PartType.ReactionCount` or a
  Meta-only summary) that don't yet have a real use case.

### Scope of this ADR

This ADR decides **only** how standard-emoji reactions are modelled on
Telegram. It explicitly does **not** decide:

- **`custom_emoji` reactions**: silently filtered out on inbound, not
  emittable on outbound. Future amending ADR when a real caller asks.
- **`paid` reactions** (Telegram Stars): same handling, same future
  ADR territory.
- **`IsBig` flag** on `SetMessageReactionParams`. Pure UX enhancement
  (animated emoji); doesn't change the retry-or-correctness story.
  Future amending ADR if requested.
- **`actor_chat`** on `MessageReactionUpdated`. For reactions from
  anonymous group admins, the User pointer can be nil and ActorChat
  points to the chat instead. This phase treats nil-User as "no
  sender" and drops the event with `ErrUnsupportedContent`.
- **`MessageReactionCount`** (anonymised group aggregate). Different
  shape, different use case; future ADR.
- **The `OldReaction` / `NewReaction` containing multi-user history**
  beyond a single user's change. Telegram delivers one
  `MessageReactionUpdated` per user-action; this ADR does not attempt
  to reconstruct any cross-user history.
- **Promotion of `telegram.reaction_action` to a canonical Meta key**
  (without the `telegram.` prefix). Same discipline as ADR-0006
  applied to commands: stays channel-prefixed until a second channel
  with reactions provides evidence for the right canonical form.

## Alternatives Considered

### Outbound, Option R1 (CHOSEN) — new `OperationKind.OpSetReaction`

See *Decision §1*. Wins because:

- Reactions setting is a state mutation on an existing message,
  identical in shape to `OpEditText` / `OpDelete` (target by chat_id
  + message_id, no user-perceptible message body in the dominio's
  conversational sense, one Envelope → one `*Params`).
- Reuses the existing dispatch infrastructure: same
  `outboundOperation` switch, same `parseChatID` /
  `parseTargetMessageID`. The new code is small and concentrated.
- Symmetric with the precedent ADR-0006 established for mutations.

### Outbound, Option R2 — new top-level `Envelope.Reaction *Reaction` field

Modelled like `Envelope.Keyboard`: a top-level optional struct that
carries the reaction set.

**Rejected.** Two reasons:

1. **Different category than `Keyboard`.** A keyboard is decoration
   that attaches to a message-shaped Envelope (one with content
   Parts). A reaction setting is an instruction with no message body
   — it should live in `Operation` (where instructions live), not
   alongside content.
2. **Worse pipeline fit.** Every consumer that today checks
   `Envelope.Operation` to decide "this is an instruction" would
   need to ALSO check `Envelope.Reaction`. Two top-level optional
   fields representing the same category (instruction) is the smell
   ADR-0006 explicitly avoided.

### Outbound, Option R3 — pure Meta-driven (no domain change)

Use channel-prefixed Meta keys to signal "this is a reaction set
request" without touching `OperationKind`. Same shape as the
rejected "edit via Meta hint" option in ADR-0006 §M1.

**Rejected** for the same reason ADR-0006 rejected its M1: hidden-
state coupling between Meta and dispatch logic, and asymmetry with
every other mutation in the codebase. The dominio change is small;
the architectural coherence gain is large.

### Inbound, Option R4 (CHOSEN) — new `PartType.Reaction`

See *Decision §2*. Wins because:

- The emoji set IS the content of a reaction event from the dominio's
  vantage point: who reacted, with which emojis. No other content.
- `Parts` is the canonical container for "the data carried by this
  Envelope", and a Reaction Part fits that role identically to a
  Callback Part.
- Mirrors the `Callback` precedent (inbound user-initiated event).

### Inbound, Option R5 — new `Operation` on inbound envelopes

Set `Envelope.Operation` to a new `OpUserReacted` (or similar) on
inbound, to make outbound and inbound symmetric.

**Rejected.** Three reasons:

1. **Direction signaling is meaningful.** Per ADR-0006, `Operation`
   on an outbound Envelope means "the bot wants to perform an
   action". Putting `Operation` on inbound would conflate two
   distinct semantics: "outbound instruction" and "inbound event
   classification". Every downstream consumer that today reads
   `Operation` as "this is an outbound instruction" would have to
   start handling an inbound case too.
2. **Forced symmetry breaks the established categories.** ADR-0005
   chose asymmetry between `Keyboard` (top-level) and `Callback`
   (Part); ADR-0006 chose asymmetry between `CallbackAck`
   (Operation) and `Callback` (Part). Asymmetry is the established
   answer to "these two things are in the same conceptual region
   but different semantic categories". Forcing symmetry now would
   undo two phases of intentional design.
3. **`Parts` is the right home for inbound content anyway.** The
   emojis are content; nothing else about the reaction event needs
   structured representation.

### Inbound, Option R6 — new top-level `Envelope.Reaction *Reaction` (mirror of R2 inbound)

**Rejected** for the same content-vs-decoration argument as R2. A
reaction event's payload is content (the emojis), not decoration.

### Inbound diff semantics, Option D1 — `Parts = NewReaction` always

`Parts` always holds the new state, regardless of action. For the
`"removed"` case (`NewReaction = []`), `Parts` is empty.

**Rejected.** Three reasons:

1. **Forces a new exception to the "≥1 Part" invariant** beyond what
   `OpDelete` / `OpCallbackAck` already needed. Inbound Reaction
   envelopes would join the exception list, and the exception
   wouldn't be guarded by an `Operation` marker (since inbound has
   no Operation) — it would have to be guarded by inspecting Parts'
   shape, which is recursive and fiddly.
2. **Loses information.** For `"removed"` events, a consumer that
   reads `Parts` gets nothing. The most actionable downstream
   question ("which emoji did the user just remove") becomes
   impossible to answer.
3. **Inconsistent semantics across actions** if we tried to fix this
   by putting `OldReaction` in Meta — Parts means "current state"
   for `added`/`changed` but is empty for `removed`, and the actual
   removed data lives elsewhere. Hidden coupling.

### Inbound diff semantics, Option D2 (CHOSEN) — `Parts = the emojis the action references`

See *Decision §4*. Wins because:

- A single semantic rule covers every case: `Parts` is what the
  action is about. Action `"added"` → added emojis. Action
  `"removed"` → removed emojis. Action `"changed"` → new emojis
  (with previous in Meta for downstream that cares).
- Every Reaction event delivered upward has ≥1 Part — no new
  invariant exception.
- The "no-op" case (filtered Old == filtered New) is dropped at the
  adapter seam, not delivered as a degenerate Envelope.

### Inbound diff semantics, Option D3 — emit two Envelopes per "changed" event (one removed, one added)

`InboundFromUpdate` returns `[]*Envelope` instead of `*Envelope`.

**Rejected.** Breaking signature change for every existing caller.
The single-Envelope-with-action model (D2) carries the same
information without the API churn. The atomicity of the change is
expressed by `"changed"` + `Meta[telegram.reaction_previous]`, not
by emitting paired envelopes.

### Discriminated union handling, Option U1 (CHOSEN) — silent emoji filter

See *Decision §5*. Wins because:

- Allows mixed-reaction events (a user reacting with an emoji AND a
  custom_emoji at the same time) to surface the emoji portion
  without blocking on the rest.
- Future amending ADR can promote `custom_emoji` / `paid` to first-
  class PartTypes without rewriting this filter — they'd just stop
  being filtered out.

### Discriminated union handling, Option U2 — reject any Update containing non-emoji reactions

**Rejected.** Suppresses legitimate emoji data based on the presence
of unrelated reaction types. The downstream pipeline would see
fewer reaction events than the user actually performed, which is the
exact information loss we are trying to avoid.

### Discriminated union handling, Option U3 — model all three variants in this ADR

**Rejected** as out of scope. `custom_emoji` requires modelling the
Telegram custom-emoji ID space (referencing pre-uploaded animated
emojis), and `paid` is the Stars-economy feature. Each is its own
amending-ADR-sized decision; bundling them into 2E.7 expands the
phase beyond the "minimal reactions" promise and locks in modelling
choices without evidence of a real use case.
