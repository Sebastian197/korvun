# ADR-0004: Modelling geographic locations in the Envelope

> **Status:** accepted
> **Date:** 2026-06-14
> **Deciders:** Sebastián Moreno Saavedra

## Context

Phase 2E.3 of Stage 2-EXT adds first-class support for Telegram **location**
messages (`message.location` inbound and `SendLocationParams` outbound). A
location is fundamentally a pair of geographic coordinates
(`latitude`, `longitude`), both `float64` in the Telegram Bot API and in
every other messaging platform that supports the concept (WhatsApp, Signal,
LINE, RCS, generic webhooks).

Unlike the voice-vs-audio distinction resolved in Phase 2E.1/2 (which was
purely a Telegram presentation detail and lived in
`Meta["telegram.audio_kind"]`), a geographic location is a **canonical**
concept: any channel can carry one, and the domain layer — router,
brains, policy — may legitimately want to read it without knowing which
channel produced it. It therefore belongs in `internal/envelope`, not in
the Telegram adapter's `Meta`.

The domain today (after Phases 2.1, 2.2, 2.3, 2E.1, 2E.2) is:

```go
type PartType int
const (Text PartType = iota; Image; Audio; Video; File)

type Part struct {
    Type     PartType `json:"type"`
    Content  string   `json:"content,omitempty"`
    Source   string   `json:"source,omitempty"`
    MIMEType string   `json:"mime_type,omitempty"`
}
```

`Validate()` splits by `PartType`: `Text` requires `Content`; the four
media types require `Source`. Two adapters consume this contract:

- `internal/channels/telegram` (inbound + outbound, switches on `Part.Type`).
- `internal/channel/webhook` (inbound + outbound, switches on `Part.Type`).

Both end their switches with a `default:` that either returns
`ErrUnsupportedContent` (Telegram outbound) or silently skips the part
(webhook outbound). That property matters for the choice below: adding a
new `PartType` constant is **non-breaking** for adapters that haven't been
extended yet — they will refuse the new kind cleanly rather than
misbehave.

The question is *how* to attach the two `float64` coordinates to the
Envelope without destabilising the existing canonical type.

## Decision

Adopt **Option B**: introduce a new `PartType` constant `Location` and
serialise the coordinates **inside the existing `Content` string field**
as a **JSON object**, with no new struct fields on `Part`.

The wire form is a JSON object with two mandatory keys, `lat` and `lon`,
both `float64`:

```json
{"lat":<lat>,"lon":<lon>}
```

The object is produced by `encoding/json` (`json.Marshal` on a typed
helper struct), so numeric precision follows Go's `encoding/json`
contract for `float64`. Examples:
`{"lat":41.40338,"lon":2.17403}`,
`{"lat":-33.8688,"lon":151.2093}`,
`{"lat":0,"lon":0}`.

JSON over CSV was chosen specifically for **forward-compatible
extensibility**. Telegram alone already carries optional companions for a
location — `horizontal_accuracy`, `live_period`, `heading`,
`proximity_alert_radius` — and other channels will add their own. A JSON
object lets future ADRs introduce additional optional keys
(`accuracy`, `heading`, `live_until`, …) without breaking the parser or
the wire format: today's parsers ignore unknown keys, tomorrow's parsers
read them. A CSV `"lat,lon"` cannot be extended without inventing a new
delimiter scheme or re-doing the format.

Parsing rules:

- Unknown keys are **tolerated** (not rejected) so older binaries can
  consume Envelopes produced by newer ones.
- `lat` and `lon` are **required**; absence is a validation error.
- This ADR fixes the *current* set of keys at `{lat, lon}`. Every
  additional key will require its own ADR amendment so the canonical
  schema stays auditable.

A typed builder and a typed accessor hide the encoding from callers:

```go
// In internal/envelope/builder.go
func (e *Envelope) AddLocation(lat, lon float64) *Envelope

// In internal/envelope/envelope.go (or a new location.go)
func (p Part) Location() (lat, lon float64, ok bool)
```

The accessor signature is intentionally narrow today. When a future ADR
adds optional fields (e.g. accuracy), the signature may grow to
`(lat, lon, accuracy float64, ok bool)` or be replaced by a typed
struct return — that is a future API decision, not blocked by this one.

`Validate()` gains one new case: for `PartType == Location`, `Content` is
required and must parse as a JSON object containing valid `lat` and
`lon` floats; `Source` and `MIMEType` must be empty (they have no
meaning for a coordinate pair, so allowing them would invite misuse).

The serialised form is **part of the canonical contract**: the `{lat,
lon}` key set is fixed by this ADR. Additions are allowed (forward
compatibility), removals or renames are not.

### Scope of this ADR (important)

This ADR decides **only** how a geographic location is modelled. It is
**not** a general endorsement of "encode every non-trivial type as JSON
inside `Content`". Every future non-text canonical concept gets its own
ADR and its own choice between:

- staying in `Content` (the route this ADR takes for location),
- extending the `Part` struct,
- introducing a sibling type alongside `Part`,
- or something else.

In particular, **Phase 2E.4 — inline keyboards / reply markup — must be
decided in its own ADR**, because a keyboard is a structured tree of
buttons with callbacks, not a leaf payload, and may legitimately force a
reconsideration of whether `Part` should evolve from flat to
structured. That re-evaluation is a feature, not a regression of this
decision: the precedent set here is "minimum invasive change per
canonical concept", not "always serialise into `Content`".

## Consequences

### What this changes

- `internal/envelope/envelope.go`: one new constant `Location` in the
  `PartType` iota block, one new case in `PartType.String()`, one new
  helper method `Part.Location()` (or free function — implementation
  detail).
- `internal/envelope/validate.go`: one new case in `validatePart`.
- `internal/envelope/builder.go`: one new builder `AddLocation`.
- Telegram adapter (Phase 2E.3 work): adds Location cases on inbound
  and outbound paths. The existing `default: return ErrUnsupportedContent`
  in `OutboundParams` continues to be the correct behaviour for any
  future unmodelled `PartType`.

### What this does NOT change

- **`Part` struct layout is untouched.** No new fields are added, so the
  existing JSON wire format of every Text/Image/Audio/Video/File
  Envelope produced or consumed by Phases 2.1–2E.2 is **byte-identical**
  before and after this ADR. The `TestJSON_roundtrip` and
  `TestJSON_roundtrip_preserves_all_part_types` suites do not change in
  any way that asserts a different layout for existing types; they only
  gain coverage for the new `Location` type.
- **No existing adapter is forced to recompile its switch.** Phase 2E.2
  Telegram outbound's `default` case already returns
  `ErrUnsupportedContent`, and the webhook adapter's switch silently
  skips unknown types. Both behaviours are documented and intentional;
  neither hides a bug if a Location envelope is fed into a channel that
  doesn't yet handle it.
- **No mandatory adapter rewrite.** Phase 2E.3 will add Location handling
  to the Telegram adapter only. The webhook adapter can stay unchanged
  until a real demand arises (and when it does, it can be added in a
  one-line case without a domain change).

### Coverage impact (`internal/envelope` ≥ 90 %)

Three new code paths are introduced:

1. `Location` case in `PartType.String()` — tested by adding
   `{"location", Location, "location"}` to `TestPartType_String`.
2. `Location` case in `validatePart` — tested by adding rows to
   `TestValidate_errors` covering: empty `Content`, non-JSON
   `Content`, JSON object missing `lat`, JSON object missing `lon`,
   `lat`/`lon` of the wrong JSON type, and `Source`/`MIMEType`
   non-empty for a Location part. Plus a happy-path assertion in
   `TestValidate_valid_envelope` for a location envelope, including
   one with an unknown extra key to lock in the
   forward-compatibility rule.
3. `AddLocation` and `Part.Location()` round-trip — tested by a
   dedicated `TestBuilder_AddLocation` (covering positive, negative and
   `(0, 0)` coordinates so Null Island survives the round-trip) and a
   JSON round-trip case extending
   `TestJSON_roundtrip_preserves_all_part_types`.

Every new line is reached by at least one new test, so coverage stays
≥ 90 %.

### Trade-offs accepted

- **No compile-time type safety on coordinates.** A caller could in
  principle build a Part with `Type: Location, Content: "garbage"`
  bypassing the builder. `Validate()` catches this at the boundary
  (every adapter calls `Validate()` before handing an Envelope to the
  rest of the system), so the practical risk is the same as for any
  other malformed Envelope — caught at the seam, not silently
  propagated.
- **Coordinates round-trip through JSON.** `encoding/json` writes
  `float64` in a round-trippable shortest form and parses it back
  losslessly, so `Marshal`→`Unmarshal` of `(lat, lon)` returns the
  same `float64` bits. The `Part.Location()` accessor is what callers
  use; nothing else inside the Envelope manipulates the textual form.
- **Format is locked but extensible.** Once Phase 2E.3 ships, the
  `{lat, lon}` key set is part of Korvun's wire contract and cannot be
  changed without a new ADR superseding this one. Adding optional keys
  is allowed by amendment ADRs; removing or renaming `lat` / `lon` is
  not. That asymmetry is intentional: it preserves backward
  compatibility while leaving room for live locations, accuracy, and
  future companions.

## Alternatives Considered

### Option A — add `Lat`/`Lon` fields to `Part`

```go
type Part struct {
    Type     PartType `json:"type"`
    Content  string   `json:"content,omitempty"`
    Source   string   `json:"source,omitempty"`
    MIMEType string   `json:"mime_type,omitempty"`
    Lat      float64  `json:"lat,omitempty"`
    Lon      float64  `json:"lon,omitempty"`
}
```

**Rejected.** Three reasons:

1. **Wire-format bloat for every other type.** With
   `omitempty`, `(lat,lon) = (0,0)` is still a valid coordinate
   (Null Island), so safely omitting them requires either pointer fields
   (`*float64`, nil-able) or a sentinel — both invasive. Without
   pointers, `omitempty` would silently drop the legitimate
   `(0, 0)` location.
2. **Conceptual leak.** A text part carrying a `Lat` field is a
   category error. The `Part` struct already mixes `Content`/`Source`
   by `PartType`; adding two more fields that are meaningful for
   exactly one type pushes the struct further toward "tagged union
   pretending to be a record" and increases the cost of every future
   review of every adapter.
3. **JSON round-trip risk.** The Phase 2.3 `TestJSON_roundtrip` suite
   asserts the *exact* shape of `Part` in marshalled bytes for every
   existing type. Adding fields — even `omitempty` ones — changes the
   marshalled output for any part whose zero value differs from
   omitted, and forces every consumer of the JSON form (logs, future
   persistence, replay harness) to be re-validated.

### Option C — introduce a `Location` struct in `Part`

```go
type Location struct {
    Lat float64 `json:"lat"`
    Lon float64 `json:"lon"`
}

type Part struct {
    // ... existing fields ...
    Location *Location `json:"location,omitempty"`
}
```

**Rejected, but the closest runner-up.** Strictly cleaner than Option A
because `*Location` is properly absent for non-location parts via
`omitempty`, sidesteps the Null Island ambiguity, and gives compile-time
typing. The reasons to still prefer Option B:

1. **New top-level JSON key.** Every existing JSON consumer (today only
   the round-trip tests, tomorrow possibly logs, replay, persistence)
   gains a new optional field whose absence must be tolerated. Option B
   adds **zero new keys**.
2. **Cross-cutting precedent.** If Location justifies a sub-struct, the
   next canonical concept (contact card, poll, sticker pack) will too,
   and `Part` ends up as a discriminated union with N pointer fields
   ("`*Location`, `*Contact`, `*Poll`, …") — exactly the smell Option A
   tries to avoid. Keeping the encoding inside `Content` and shielding
   it behind a typed accessor preserves the discipline that a `Part` is
   `{Type, Content, Source, MIMEType}` and nothing else, with `PartType`
   carrying the meaning of `Content`.
3. **Symmetry with existing types.** A `Text` part already stores its
   semantic payload in `Content`. A `Location` part doing the same —
   with a documented JSON form and a typed accessor — is the *minimum*
   deviation from the established pattern, with the added benefit that
   the JSON form is self-describing and forward-extensible.

Option C remains a reasonable future evolution if Korvun later needs
several non-string canonical payloads at once; at that point a new ADR
would supersede this one and migrate the wire format. For Phase 2E.3,
the smaller change is the right one.

### Option D — model Location *outside* `Parts`, e.g. as `Envelope.Location *Location`

**Rejected.** Treats Location as an envelope-level attribute rather than
a content part. Breaks the invariant that `Parts` enumerates *all*
content of the message, complicates multipart messages
("text + location" would need two distinct fields to inspect rather than
walking `Parts`), and forces every existing component that iterates
`Parts` to also check `Envelope.Location` to be complete. The
`Parts`-based model is what Korvun has committed to since Phase 2.1.

### Option E — keep Location entirely in `Meta`

**Rejected.** `Meta` is for **channel-specific** detail that the domain
doesn't need to understand (e.g. `telegram.chat_id`,
`telegram.audio_kind`). A geographic coordinate is the opposite:
channel-independent and useful to the domain layer. Hiding it in
`Meta["telegram.location"]` would force every other future channel
adapter to invent its own key, and would make the router and brains
guess at the location's presence. This is exactly the line that
ADR-0001 drew when it stated "every Telegram-specific field not modelled
by Envelope is preserved in `Envelope.Meta`" — Location *is* modelled,
so it belongs in the type.
