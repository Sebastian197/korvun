# Stage 01 — Domain: Canonical Envelope

> **Status:** closed
> **Started:** 2026-06-13
> **Closed:** 2026-06-13

## Objective

Define the universal message type (Envelope) — multimodal, channel-agnostic —
that all Korvun components use to communicate.

## Phases Completed

| Phase | Description                  | Status |
|-------|------------------------------|--------|
| 1.1   | Base types                   | done   |
| 1.2   | Identity and builder         | done   |
| 1.3   | Validation and serialization | done   |

## Deliverables

### Phase 1.1 — Base Types
- `Direction` (Inbound/Outbound) with `String()`
- `PartType` (Text/Image/Audio/Video/File) with `String()`
- `Participant` (ID, Name)
- `Part` (Type, Content, Source, MIMEType)
- `Envelope` (ID, Channel, Direction, Sender, Parts, Timestamp, Meta)

### Phase 1.2 — Identity and Builder
- `NewID()` — unique, time-sortable, no external dependencies
  (format: `<unix-ms-hex>-<counter-hex>-<random-hex>`)
- `New()` — constructor with auto-generated ID, timestamp, initialized Meta
- `AddText()` / `AddMedia()` — chainable builder methods

### Phase 1.3 — Validation and Serialization
- `Validate()` — checks: empty ID, empty channel, invalid direction,
  empty sender ID, no parts, empty text content, media without source,
  zero timestamp
- JSON round-trip verified for all part types and metadata

## Quality Gate

- `make quality`: pass
- Coverage: 97.8% (threshold: ≥ 90%)
- All tests run with `-race`

## Branch Strategy

- `stage-1/envelope-canonical` — stage branch
- `stage-1/phase-1.1-base-types` — Phase 1.1
- `stage-1/phase-1.2-identity-builder` — Phase 1.2
- `stage-1/phase-1.3-validation-serialization` — Phase 1.3

## Key Decisions

- **No external dependencies for ID generation.** Uses timestamp + atomic
  counter + crypto/rand. Avoids adding UUID libraries for a single use case.
- **Meta as `map[string]string`.** Simple, serializable, sufficient for
  channel-specific metadata. Can be promoted to structured types later if
  needed without breaking the Envelope contract.

## Notes

- Context7 not needed — this stage uses only Go stdlib.
- TDD followed strictly: all tests written and confirmed red before
  implementation in each phase.