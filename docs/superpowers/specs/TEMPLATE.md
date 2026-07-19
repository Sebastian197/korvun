# <Piece/Stage + sub-phase> — <subject>: Design Spec

> **Status:** draft | approved for TDD | superseded.
> Governing ADRs: <ADR-00XX §§...>. External-docs note: name the Context7 /
> primary-source verification that backs any external API this spec touches,
> or state explicitly that only stdlib + existing `internal/` packages are
> used. Record here anything the spec inherits as law (e.g. a package
> contract from an ADR or a doc.go).

## Goal

One paragraph: the capability this sub-phase delivers, in behavioral terms —
what exists afterwards that does not exist today, and what explicitly stays
out (deferred to which sub-phase).

## Functional requirements

- **FR-1** — One requirement per bullet, numbered (`FR-<AREA>-N` when several
  areas coexist). Each FR states WHAT, its seam (package/function surface),
  and the governing decision it traces to. Mark additive changes to shared
  packages explicitly, with their blast radius.
- **FR-2** — ...

## Acceptance scenarios (Given / When / Then)

- **AS-1** Given <precondition>, When <action>, Then <observable outcome —
  assertable in a test, including the error text/sentinel where relevant>.
- **AS-2** ... (Cover the unhappy paths and the guard rails, not only the
  happy path; name the tripwire tests that carry a structural decision.)

## Success criteria

- Coverage floor for the new package(s) (house: ≥85%; ≥90% for
  policy/router/envelope/brain).
- `make quality` green with `-race` over the WHOLE suite.
- What must remain untouched (headless binary, pipelines) and how it is
  proven (e.g. `go version -m` diff).

## Decisions folded in

Surface-level calls made inside this spec's mandate (with one-line
rationale each), so the review can veto them without archaeology.

## `[NEEDS CLARIFICATION]`

Numbered genuinely-open points that BLOCK TDD until resolved by the copilot
— or the explicit statement that none arose. Per CLAUDE.md, do not proceed
to tests while any of these is open.
