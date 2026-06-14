# ADR-0005: Modelling inline keyboards and callback interactions in the Envelope

> **Status:** accepted
> **Date:** 2026-06-14
> **Deciders:** Sebastián Moreno Saavedra

## Context

Phase 2E.4 of Stage 2-EXT adds first-class support for **inline keyboards**
in Telegram, plus the **callback queries** they produce when a user taps a
button. Concretely the surface is:

- **Outbound:** attach a keyboard to a message. Telegram's
  `models.InlineKeyboardMarkup` is a 2D array (`[][]InlineKeyboardButton`)
  and rides as `SendMessageParams.ReplyMarkup` (and on every other
  `Send*Params`). Each button is a tagged union of 11 possible flavours
  (callback button, URL button, WebApp button, LoginURL, SwitchInlineQuery,
  CopyText, CallbackGame, Pay, plus icon/style modifiers).
- **Inbound:** when a user taps a callback button, Telegram delivers
  `update.callback_query` — **not** a `Message`. The update kind is
  fundamentally different, and the callback **must be acknowledged** with
  `AnswerCallbackQuery` or Telegram retries every few seconds. The query
  carries `ID`, `From User`, `Message` (the original message, possibly
  inaccessible), `Data` (the callback payload string), `ChatInstance`.

ADR-0004 explicitly deferred this decision and asked it to be reconsidered
*from scratch*: "Phase 2E.4 — inline keyboards / reply markup — must be
decided in its own ADR, because a keyboard is a structured tree of buttons
with callbacks, not a leaf payload, and may legitimately force a
reconsideration of whether Part should evolve from flat to structured."

The domain today (after 2.1–2E.3) is:

```go
type PartType int
const (Text PartType = iota; Image; Audio; Video; File; Location)

type Part struct {
    Type     PartType
    Content  string
    Source   string
    MIMEType string
}

type Envelope struct {
    ID, Channel string
    Direction   Direction
    Sender      Participant
    Parts       []Part
    Timestamp   time.Time
    Meta        map[string]string
}
```

Two adapters consume this contract: `internal/channels/telegram` and
`internal/channel/webhook`.

The structural question this ADR must answer has three parts:

1. **Outbound modelling.** A keyboard is a *tree* of buttons (rows of
   tagged-union buttons), and it is *decoration* travelling alongside a
   message — not the message itself. Where in the Envelope does it go?
2. **Inbound modelling.** A callback query is a *different update kind*
   from a Message. Does it become a different `Direction`, a new
   `PartType`, or a separate event type?
3. **Acknowledgement.** `AnswerCallbackQuery` is a *required* reply:
   Telegram retries the callback every few seconds until it is
   acknowledged. Shipping inline keyboards without an ack primitive is
   not "deferred work"; it is a feature that fails in production from
   the first tap. Is the ack in scope for this phase, and if so, at
   what minimal shape?

## Decision

Three coupled decisions, each minimum-invasive in its own axis.

### 1. Outbound keyboards → `Envelope.Keyboard *Keyboard` (new top-level field)

Add an optional, top-level `Keyboard` field on `Envelope`, distinct from
`Parts`:

```go
type Envelope struct {
    // ... existing fields ...
    Keyboard *Keyboard `json:"keyboard,omitempty"`
}

type Keyboard struct {
    Rows [][]Button `json:"rows"`
}

type Button struct {
    Text         string `json:"text"`
    CallbackData string `json:"callback_data,omitempty"`
    URL          string `json:"url,omitempty"`
}
```

Phase 2E.4 supports exactly two button flavours: **callback** (text +
`CallbackData`) and **URL** (text + `URL`). A button MUST have `Text` and
MUST set exactly one of `CallbackData` / `URL`. Every other Telegram button
kind (WebApp, LoginURL, SwitchInlineQuery*, CopyText, CallbackGame, Pay)
is **out of scope** for this phase and requires an amending ADR.

This is **not** a `PartType.Buttons`. The reasoning is structural:

- **A keyboard is decoration, not content.** `Parts` enumerates *what the
  message contains*. A button row is not part of the message body; it is
  an interactive overlay. Putting it inside `Parts` would conflate two
  unrelated categories (content vs. interaction surface) under one slice
  and force every consumer of `Parts` to filter the keyboard out.
- **A keyboard is naturally tree-shaped.** Locations stringify to two
  numbers; a keyboard stringifies to a nested tagged-union tree. Encoding
  that tree as JSON-inside-`Content` (the Location pattern) would force
  every adapter and every log to decode a string-inside-a-string. The
  tooling cost is real and the safety cost is real (no typed builder
  catches a malformed button at compile time).
- **The ADR-0004 escape clause applies here.** That ADR explicitly said
  the next non-text concept gets its own choice between Content,
  extending `Part`, or "introducing a sibling type alongside `Part`, or
  something else". This decision exercises the "something else" branch:
  the sibling field lives on `Envelope`, not on `Part`, because keyboards
  attach to *messages*, not to individual parts.

### 2. Inbound callbacks → new `PartType.Callback`

When Telegram delivers `update.callback_query`, the adapter converts it to
an Envelope with `Direction = Inbound` and a single
`Part{Type: Callback, Content: <callback data string>}`. The callback's
identifier — needed later to acknowledge it — rides in `Meta`:

- `telegram.callback_query_id` — the `CallbackQuery.ID` value, required by
  `AnswerCallbackQuery`.

The existing `telegram.chat_id` and `telegram.message_id` meta keys are
reused for the *original* message that carried the keyboard, when present
and accessible.

This is **not** a new `Direction`. The reasoning is structural:

- A callback IS a message in the sender's intent: the user "said
  `<callback_data>`" by tapping instead of typing. The Envelope's
  Sender/Direction/Parts shape fits unchanged.
- Adding a third `Direction` (e.g. `Direction.Callback`) would break the
  in/out duality every consumer relies on and would force pattern matches
  in router, brains, and policy that today are exhaustive switches.
- A new `PartType` is the minimum extension, parallel to how `Location`
  was added in 2E.3, and lets the same `InboundFromUpdate` function
  handle both `update.Message` and `update.CallbackQuery` paths.

A callback Envelope has exactly one Callback Part. Mixing Callback with
any other Part is rejected by Validate — a tap event has no media,
location, or text body beyond its data string.

### 3. `AnswerCallbackQuery` → minimal ack included as `PartType.CallbackAck`

A keyboard without an ack is broken on contact: Telegram retries every
unacknowledged callback every few seconds, so shipping 2E.4 without an
ack would mean shipping a feature that DDoSes the bot's own endpoint on
the first tap. That is not "deferred work"; it is active debt. The
extra surface needed to close this gap is small and structurally free,
so it lands in this phase.

Model: an outbound Envelope with a single
`Part{Type: CallbackAck, Content: <toast or "">}` and
`Meta["telegram.callback_query_id"] = <id>` translates to a
`*bot.AnswerCallbackQueryParams{CallbackQueryID, Text}` returned via a
new `OutboundKindAnswerCallback`.

```go
const (... ; Callback; CallbackAck)   // new PartType

// Idiomatic construction:
env := envelope.New("telegram", envelope.Outbound, bot)
env.Meta["telegram.callback_query_id"] = inbound.Meta["telegram.callback_query_id"]
env.AddCallbackAck("")            // silent ack
env.AddCallbackAck("Saved!")      // ack with a toast
```

Semantics:

- `Content` carries an optional toast text. Empty = silent ack (the
  call still goes out; Telegram just shows no toast). This is the
  default and the case that matters for "make the bot stop retrying".
- `Meta["telegram.callback_query_id"]` is **required** and validated by
  the Telegram adapter — not by the domain Validate, because the key
  itself is channel-specific (ADR-0001). Domain Validate enforces only
  the Part shape (no Source, no MIMEType, exclusive with other Parts).
- A CallbackAck Envelope reuses the existing `OutboundParams` pipeline
  — it returns an `Outbound{Kind: OutboundKindAnswerCallback,
  AnswerCallback: &bot.AnswerCallbackQueryParams{...}}`. No new method
  on the Channel port, no new orchestrator-side dispatch path.

This is structurally free for the same reason media outbound was
structurally free in Phase 2E.2: the Envelope-as-outbound-instruction
pattern (`one Envelope → one Send*Params produced by the adapter`)
already carries this shape. The CallbackAck Part is just another
flavour the adapter knows how to translate.

This decision is **deliberately narrower** than a "side-effect
operations" ADR. Three flavours of side-effect exist, and only one of
them belongs here:

- **Notifications to the user (`AnswerCallbackQuery` here, conceivably
  also typing indicators)** — semantically an outbound message that
  the user perceives. Encodes as an Envelope; lands in this phase.
- **Mutations of existing state (edit message, delete message)** —
  identifies a target by ID and changes it. May warrant a separate
  primitive on the Channel port; its own future ADR.
- **Reads from the channel (resolve file URL, get chat info)** — pure
  reads with no Envelope analogue. Its own future ADR.

Including the ack here does **not** prejudice the design of the other
two: if a later ADR refactors side-effects under a common umbrella, the
ack is one of the things refactored, not a special case.

Out of 2E.4 by deliberate scope cut: `ShowAlert`, `URL`, `CacheTime`
fields of `AnswerCallbackQueryParams`. The narrow goal of the ack here
is to stop the retry loop; modal alerts, redirects, and cache hints are
UX enhancements that do not change the retry-storm fix. Each of those
can be added by amending ADR when a real use case asks for it — same
discipline applied to button kinds in this ADR and to Location
companions in ADR-0004.

## Consequences

### What this changes

**`internal/envelope`:**

- New `Keyboard` and `Button` types (likely a new file `keyboard.go`).
- New `PartType.Callback` constant + `PartType.String()` case.
- New `PartType.CallbackAck` constant + `PartType.String()` case.
- New `Envelope.Keyboard *Keyboard` field (top-level, optional).
- New `Validate` cases:
  - Keyboard (when non-nil): every row non-empty, every button has Text
    and exactly one of `CallbackData` / `URL` (and not both).
  - Callback Part: non-empty Content, no Source/MIMEType, no coexistence
    with non-Callback parts.
  - CallbackAck Part: Content is optional (empty = silent ack), no
    Source/MIMEType, no coexistence with other Parts. The required
    `Meta["telegram.callback_query_id"]` is **not** enforced here —
    the key is channel-specific and is validated by the adapter, in
    line with ADR-0001.
- New builder helpers: `Envelope.WithKeyboard(rows ...) *Envelope`,
  `Envelope.AddCallback(data string) *Envelope`,
  `Envelope.AddCallbackAck(toast string) *Envelope`, and tiny
  constructors `CallbackButton(text, data)` / `URLButton(text, url)` for
  ergonomics.

**`internal/channels/telegram`:**

- `InboundFromUpdate` gains a `u.CallbackQuery != nil` branch: produces a
  Callback Envelope with `Meta["telegram.callback_query_id"]` set,
  preserving the original `chat_id` / `message_id` when the original
  message is still accessible.
- `OutboundParams` learns two new dispatches:
  - **Keyboard attachment:** when `Envelope.Keyboard != nil`, the
    translated `models.InlineKeyboardMarkup` is attached as the
    `ReplyMarkup` of every Send*Params it already produces (Message,
    Photo, Document, Voice, Audio, Video, Location). The text-only /
    media-only / Location-only dispatch logic does not change otherwise.
  - **CallbackAck routing:** an Envelope whose single non-text part is
    a CallbackAck is dispatched as a new `OutboundKindAnswerCallback`
    populating `*bot.AnswerCallbackQueryParams{CallbackQueryID, Text}`.
    `CallbackQueryID` is read from `Meta["telegram.callback_query_id"]`;
    its absence is rejected with `ErrMissingCallbackQueryID`. `Text` is
    the CallbackAck Part's Content (empty = silent ack).
- New tagged-union field on `Outbound`:
  `AnswerCallback *bot.AnswerCallbackQueryParams`.

**Telegram adapter sentinel errors:**

- `ErrCallbackQueryMissing` (or similar) — defensive case where the
  inbound callback query lacks `From` or `ID`.
- `ErrMissingCallbackQueryID` — outbound CallbackAck without
  `Meta["telegram.callback_query_id"]`.
- Reuses `ErrUnsupportedContent` for callback updates that carry no
  `Data` string.

### What this does NOT change

- **No PartType for keyboards.** Buttons never appear in `Parts`. Every
  existing consumer of `Parts` (router, brains, policy, webhook adapter,
  Telegram media dispatch) keeps its current contract: `Parts` enumerates
  the message content, full stop. The two new PartTypes added by this
  ADR (`Callback`, `CallbackAck`) are *interaction events*, not
  decoration: a tap is a kind of message in intent, and an ack is a
  toast-shaped notification — both fit the existing Parts shape, the
  keyboard does not.
- **No new Direction.** `Inbound`/`Outbound` stays the in/out duality.
- **No layout change to `Part`.** The `{Type, Content, Source, MIMEType}`
  layout from 2.1 is preserved. The Location-in-Content trick of ADR-0004
  is *not* generalised: ADR-0004's scoping note is honoured — Location
  was its own decision, keyboards get their own.
- **No mandatory adapter rewrite for non-keyboard channels.** The webhook
  adapter (Phase 2.2) can stay untouched: it never produces or consumes
  `Envelope.Keyboard`, and its existing Part switch already terminates in
  a `default` that silently ignores unknown types — a Callback Part would
  fall there and be dropped, which is the correct fail-safe behaviour
  for a channel that does not speak callbacks. Whether the webhook
  adapter eventually grows callback support is a future, channel-specific
  decision.

### JSON wire format impact

Two changes to the marshalled Envelope:

1. New optional top-level key `"keyboard"`. Envelopes that don't use it
   (every Envelope produced by 2.1–2E.3) marshal identically: `omitempty`
   on a `*Keyboard` pointer means absent-when-nil.
2. New `PartType` value `Callback` in the `"parts"` array. Existing
   parsers tolerate unknown PartType integers by falling through their
   switches; Callback specifically is the marker for "this is a tap, not
   a typed message".

The Phase 2.3 JSON round-trip tests pass byte-for-byte because no
existing field changes shape.

### Coverage impact (`internal/envelope` ≥ 90 %)

New code paths to be covered by red tests first:

1. `PartType.String()` for `Callback` and `CallbackAck` — two rows
   added to the existing table.
2. `Validate` rules for Callback Parts — happy path, empty content,
   non-empty Source, non-empty MIMEType, mixed with other parts.
3. `Validate` rules for CallbackAck Parts — happy path with toast,
   happy path with empty toast (silent ack), non-empty Source,
   non-empty MIMEType, mixed with other parts. The
   `Meta["telegram.callback_query_id"]` requirement is **not** tested
   here; it is the adapter's responsibility.
4. `Validate` rules for Keyboard — non-nil with no rows, row with no
   buttons, button without Text, button with both `CallbackData` and
   `URL`, button with neither.
5. Builder helpers — `WithKeyboard`, `AddCallback`, `AddCallbackAck`,
   `CallbackButton`, `URLButton`, chaining, and JSON round-trip of the
   new top-level field.
6. Adapter outbound for CallbackAck — happy path (silent ack and
   ack-with-toast), missing `Meta["telegram.callback_query_id"]`,
   coexistence with other Parts rejected, sibling fields nil so the
   Outbound stays a clean tagged union.

Estimate: ~10 new domain tests + ~5 adapter tests for the ack path, no
decrease in coverage below 95 %.

### Trade-offs accepted

- **The Envelope grows a top-level optional field.** This is a deliberate
  step beyond the "everything in Parts" pattern from 2.1. The trade-off
  is bounded: `Keyboard` is the only top-level non-Part canonical concept
  we add in this ADR, and the scoping note carries over — future
  decoration-style concepts (reply context, message effects, ...) each
  get their own ADR and their own placement decision.
- **No compile-time mutual exclusion on Button action fields.** Go has no
  sum types, so `Button{Text, CallbackData, URL}` is technically capable
  of carrying both action fields at once. `Validate` catches this at the
  Envelope boundary (every adapter calls `Validate` before dispatch).
  An alternative would be sealing the action via an interface
  (`type ButtonAction interface{ buttonAction() }`), but the
  ergonomic and JSON cost is high for the gain, and Validate is the
  established pattern.
- **Three distinct modelling choices in the same phase** — `Keyboard`
  at Envelope level, `Callback` as a PartType for the tap event, and
  `CallbackAck` as a PartType for the acknowledgement. Resist the
  temptation to unify them out of false symmetry: they answer different
  questions (decoration vs. interaction event vs. side-effect
  notification) and deserve different homes.
- **CallbackAck uses a channel-specific Meta key for its identifier.**
  `Meta["telegram.callback_query_id"]` is enforced by the Telegram
  adapter rather than by the domain Validate, because the key itself is
  Telegram-specific (ADR-0001). The trade-off is a small one — the
  domain cannot reject a malformed ack envelope on its own — but the
  consistency with the rest of the channel-specific Meta surface
  (`telegram.chat_id`, `telegram.message_id`, …) is worth more than the
  marginal validation tightening.

### Scope of this ADR

This ADR decides **only** how inline keyboards (outbound), callback
queries (inbound), and the minimal `AnswerCallbackQuery` acknowledgement
are modelled in the canonical Envelope. It explicitly does **not**
decide:

- **Extended ack fields** (`ShowAlert`, `URL`, `CacheTime` of
  `AnswerCallbackQueryParams`). The narrow purpose of the ack in this
  phase is to stop Telegram's retry loop; modal alerts, redirects, and
  cache hints are UX enhancements that do not change the retry-storm
  fix. Each gets an amending ADR when a real use case asks for it.
- **Other side-effect operations** (edit message, delete message,
  resolve file URL, get chat info). These are categorically different
  from a notification-to-user ack — mutations of existing state or
  pure reads — and warrant their own ADR designing them as a category.
  The ack's modelling here does not prejudice that future decision: if
  the side-effects ADR refactors everything, the ack is one of the
  things refactored, not a special case.
- How custom reply keyboards (`ReplyKeyboardMarkup`),
  `ReplyKeyboardRemove`, or `ForceReply` are modelled — those are
  different UX patterns; they may reuse `Envelope.Keyboard` with new
  Button kinds, or get their own field, but the decision is deferred
  until a real use case appears.
- How non-callback, non-URL button kinds (WebApp, LoginURL,
  SwitchInlineQuery*, CopyText, CallbackGame, Pay) are modelled. The
  same precedent as ADR-0004 applies: adding optional fields to
  `Button` is forward-compatible (older parsers ignore unknown JSON
  keys); renaming or removing `Text`/`CallbackData`/`URL` is not.

## Alternatives Considered

### Outbound, Option K1 — `PartType.Buttons` with JSON-in-Content (Location-style)

```go
const (... ; Location; Buttons)
// Buttons part: Content = JSON-stringified keyboard tree
```

**Rejected.** Four reasons:

1. **A tree-as-string is genuinely ugly.** Lat/lon was two numbers in a
   3-key JSON object — stringifying it kept `jq .parts[].content` readable
   and tooling-friendly. A keyboard tree stringified to `Content`
   produces nested escaped JSON that is painful to inspect, painful to
   diff in logs, and painful to assert against in tests.
2. **No compile-time typing.** Every adapter would have to re-parse the
   string at the seam, every test would have to compare escaped strings
   or re-parse, and Validate would need a near-replica of the typed-tree
   validation logic against an opaque string. The cost compounds.
3. **Conceptual conflation.** `Parts` would now contain "actual content
   of the message" plus "an interactive overlay attached to the
   message". Every iteration over `Parts` in router/policy/brains would
   need a filter step. ADR-0004 was content-in-content; this would be
   decoration-in-content.
4. **Symmetry argument is weak.** ADR-0004 explicitly warned against
   "everything goes in Content" becoming dogma. This is the exact
   instance the warning was written for.

### Outbound, Option K2 — extend `Part` with a typed `Buttons [][]Button` field

```go
type Part struct {
    Type     PartType
    Content  string
    Source   string
    MIMEType string
    Buttons  [][]Button `json:"buttons,omitempty"`
}
```

**Rejected, but the second-closest runner-up.** Cleaner than K1: gets
typed validation back. Still wrong, for two reasons:

1. **`Buttons` is meaningful for at most one PartType.** Every Text,
   Image, Audio, Video, File, Location part would carry `Buttons: nil`.
   `omitempty` on a nil slice keeps the wire format clean, but the
   struct surface grows in a way that "tagged union pretending to be a
   record" worsens. ADR-0004 rejected the same shape for Lat/Lon.
2. **`Part` is still not the right home.** Even with a typed field, the
   semantic mismatch ("a Buttons part is not really content") remains.
   This is the structural question, not the typing question.

### Outbound, Option K3 (CHOSEN) — `Envelope.Keyboard *Keyboard` top-level field

See *Decision*. Wins because:

- Decoration lives next to message, not inside it.
- Tree shape is preserved as a typed tree, not a string.
- `Parts` invariant ("enumerates content") survives.
- Every existing consumer of `Parts` keeps its contract unchanged.

### Outbound, Option K4 — `Meta` carries a serialised keyboard

```go
env.Meta["telegram.reply_markup"] = `{"inline_keyboard":[...]}`
```

**Rejected.** Identical to ADR-0001's rule for `Meta` (channel-specific
detail the domain doesn't need to understand) but pointed in the wrong
direction: a keyboard *is* a canonical concept (every multi-channel
adapter from WhatsApp button replies to Slack interactive messages has
it). Hiding it in channel-specific `Meta` keys would force every other
adapter to invent its own key and would make the router blind to the
presence of an interactive surface on the outgoing message.

### Inbound, Option C1 — new `Direction` value for callbacks

```go
const (Inbound Direction = iota; Outbound; Callback)
```

**Rejected.** Breaks the in/out duality every consumer relies on. A
callback *is* inbound (from the user, to the bot); calling it a third
direction conflates "originating side" with "interaction modality" and
forces non-exhaustive switches in router, brains, policy. Also doubles
the size of every `Direction.String()` table.

### Inbound, Option C2 (CHOSEN) — new `PartType.Callback`

See *Decision*. Wins because:

- A callback IS a message in intent: "the user said `<data>` by tapping".
- Reuses every existing Envelope mechanism (Sender, Timestamp, Meta).
- Minimum extension: one new constant, one new Validate case.
- Symmetric with how 2E.3 added `Location`.

### Inbound, Option C3 — separate `Callback` event type, distinct from `Envelope`

```go
type Callback struct {
    ID       string
    Sender   Participant
    Data     string
    Timestamp time.Time
    Meta      map[string]string
}
```

**Rejected.** Doubles the surface of the Channel port: every adapter
exposes two methods (`InboundFromUpdate` AND `InboundFromCallback`),
every downstream component (router, brains, policy, persistence) has to
handle two event types where one would do. Whatever ergonomic gain a
typed `Callback` would bring is dwarfed by the cross-cutting cost.

### Side-effect, Option A1 (CHOSEN, with explicit scope cut) — minimal `AnswerCallbackQuery` modelled in this phase

The initial draft of this ADR deferred the ack to a future
"side-effect operations" ADR, on the principle that ack/edit/delete/
resolve-URL deserve a common design rather than a per-feature add-on.
That principle is partly right and partly wrong:

- **Partly right:** edit-message, delete-message and resolve-file-URL
  ARE categorically distinct (state mutations and reads, respectively),
  and DO deserve their own ADR. Including them here would conflate
  unrelated concerns.
- **Partly wrong:** the ack is not categorically the same as those. It
  is a *notification to the user* whose dispatch shape
  (`one Envelope → one AnswerCallbackQueryParams produced by the
  adapter`) is identical to the existing media outbound flow. Forcing
  it into a future ADR while shipping keyboards in this one would
  ship a feature that retries in a loop on every tap — broken on
  contact, not deferred.

Decision: include the ack at **minimal shape only** (`CallbackQueryID`
from Meta + optional `Text` toast from CallbackAck Part Content).
`ShowAlert`, `URL`, `CacheTime` stay out by the same discipline that
kept Location companions out of ADR-0004 and exotic button kinds out of
this ADR. State-mutation and read side-effects stay out for their own
ADR.

This decision is structurally neutral with respect to that future
side-effect ADR: if it later refactors ack/edit/delete under a common
primitive, the ack is one of the things refactored — not a special case.

### Side-effect, Option A2 — separate side-effect primitive on the Channel port

```go
type Channel interface {
    InboundFromUpdate(...) (*Envelope, error)
    OutboundParams(...) (*Outbound, error)
    Dispatch(...) (...)  // new
}
```

**Rejected for the ack specifically.** Adding a third method to the
Channel port just for the ack would (a) double the surface every
downstream component checks, (b) duplicate plumbing that the existing
`OutboundParams` already provides, and (c) prejudice the design of the
broader side-effects ADR by pre-committing to a shape. Reusing the
existing `OutboundParams` flow with a new `OutboundKind` is the
minimum-surface, maximum-symmetric choice for the ack.
