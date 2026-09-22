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

## Plan de fallos — MANDATORY, and RED does not open without it

The director's permanent norm of 2026-09-22. Fixed shape, four parts.

**1 · Consumers of the result.** Who reads what this produces — screen, CLI,
API, another module — and what each of them does with a refusal.

**2 · The failure table.** One row per category, and no category is dropped in
silence: invalid input; impossible state; race; crash before the effect; crash
after the effect; corruption; a dependency that does not answer; **an operator
with no way out**. Per row:

| Category | Behaviour | NAMED error | What the user sees | Mould + its mutation | Repair path |
|---|---|---|---|---|---|
| invalid input | … | `ErrX` | … | `TestX_…` / mutation that reddens it | … |

A row whose repair path is «none» is a row that leaves someone stuck: say so
here, in the design, not after the adversary finds it.

**3 · Neighbours and class siblings.** What else does the same thing by another
door, what states can arrive here, who else would have to change. If the fault
this design prevents has siblings, they are cured together or named here.

**4 · Declared and NOT covered.** By name, with the reason. A gap named in the
design is a decision; a gap found later is a defect.

## Failure plan for a CURE (short form)

A cure carries the same norm in three parts, written BEFORE the cure:
**how the cure itself could break something** (which good path stops working,
which operator is left with no way out, which datum someone needs and the cure
does not give), the **control** that proves the good case still behaves exactly
as before, and the **repair** that names the way out for whoever trips. Three
moulds minimum, red before the cure: the fault, the control, the repair. If one
of the three cannot exist, say which and why before delivering.

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
