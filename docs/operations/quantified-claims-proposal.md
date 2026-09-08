# Proposal — the discipline of quantified claims

**Status: PROPOSED.** Written 2026-09-08 at the director's instruction, for
his adjudication. No code. Nothing in this document is in force.

**Scope of application, if accepted: the NEXT train.** Not retroactive to
R15 or to the hygiene batch; both finish under the process they started
under. That is the director's ruling of 2026-09-08, recorded here so the
proposal cannot be read as a rule already binding.

The seed is the copilot's, in three clauses. What follows keeps one of
them, refines two, refutes an escape hatch inside the first, and adds one
the seed did not have. It closes with the honest limit and with the count
that will decide whether the rule was right.

---

## 1. The evidence

Five adversarial passes across two trains between 2026-09-06 and
2026-09-08 returned at least one finding each whose root was a sentence
wider than the wire it described. Four are addressable today:

- `docs/superpowers/specs/2026-09-08-r15-pre-review.md:13-14` — R15's third
  pass: three P1, eight P2, seven P3. **Two of the three P1 were cures of
  earlier P1s** that fixed the form of a false sentence and re-imported
  the disease into its content.
- `docs/superpowers/specs/2026-09-08-r15-pre-review.md:18-26` — the paper's
  own diagnosis of the class: every one of its letreros stated a CLOSED
  enumeration more confidently than the wire supported.
- `docs/cantos/HYGIENE-2026-09-08.md:43-51` (hygiene worktree) — the second
  pass's P1-1: a false absolute (`x/crypto/ssh` and `openpgp` "have no
  provider in the graph at all") cured with a second false absolute. The
  repository had held the true sentence since July:
  `docs/notes/dependabot-triage-2026-07-19.md:17` — "wails CLI tooling
  (go-git → ssh); no Korvun binary links it".
- `docs/cantos/HYGIENE-2026-09-08.md:111-112` — the second pass's P1-2: a
  sentence labelled as read from a file, listing five options
  `body-parser` passes to `qs`, three of which are never passed. The label
  said "by reading"; the sentence was written from memory.

Evidence level: `SOURCE`. Each item above was read at the address it
carries, in this session. The count "five passes" comes from the two
trains' running reports and is the one number here with no address in the
tree; it is stated as a count of sessions, not as a measurement.

## 2. The diagnosis — the quantifier is the tell, not the disease

Every instance above shares an order of operations, not a vocabulary. The
sentence is written first; the sweep that would support it runs afterwards,
if it runs at all. A sweep that runs after the sentence is a CONFIRMING
sweep: it stops at the first supporting hit and never reaches the
counterexample, because it was never looking for one.

That order also explains the shape the failures take when they are cured.
A cure written in the same order produces another sentence of the same
class, which is exactly what R15's third pass found twice and what the
hygiene batch's P1-1 is. The quantifier — every, none, never, only,
exactly N, the four, the whole — is where the damage becomes visible,
because a quantifier is the one part of a sentence a reviewer can falsify
with a single counterexample. Removing quantifiers without changing the
order would remove the visibility and keep the disease.

So the rule below is not "avoid absolutes". It is "do not write the
sentence before the sweep, and make the sweep's artifact travel with it".

## 3. The rule

### R1 — the address, not the adjective

A quantified claim carries, inline and in the same edit, the ADDRESS of
what makes it true: `file:line`, a command with its exit code, a run id, a
SHA. Not a class label.

This is the refinement the seed's clause (a) needs. An evidence LABEL is
itself a claim, and it can be false in the exact way the sentence it
labels is false — the hygiene P1-2 is a sentence labelled "by reading"
that was written from memory, so the label survived the review that the
sentence failed. A label a reader cannot follow is decoration. An address
is falsifiable in one step by anyone, including its author a day later.

Corollary, and the operative half: the address is written BEFORE the
sentence. If there is no address to write, there is no sentence yet.

### R2 — the closed form blocks; the open form is not a universal escape

The seed offers, as the alternative to a labelled quantifier, rewriting
the claim as an OPEN enumeration ("includes X and Y" instead of "the four
are"). That is the right default and it is FORBIDDEN in a specific place.

CLAUDE.md's own review law requires that fail-closed claims ENUMERATE
their error classes, and the "Claim-class sweep" section requires that a
class be swept entirely rather than sampled. Where the closed form is
required, softening it does not reduce risk — it destroys the sentence's
purpose and hides the gap from every future reader. A taxonomy that says
"includes `tombstone_corrupt` and `tombstone_read_failed`" promises
nothing and cannot be refuted.

So: when the claim must be closed, it BLOCKS. It is not written until the
sweep that closes it has been executed and its command recorded. The open
form is for claims that never needed to be closed. Without this half, the
rule would trade false precision for unfalsifiable vagueness, and no
adversarial pass can catch a sentence that asserts nothing.

### R3 — search by identifier, and record the search

Before a structural claim, search the prior record — `docs/notes/`,
`docs/cantos/`, `docs/adr/`, `docs/superpowers/specs/` — by the SUBJECT'S
IDENTIFIERS: module path, symbol, file name, workflow name, error name.
Never by the claim's own wording.

This is where the seed's clause (b) needs sharpening rather than agreement.
Prior findings are prose, phrased differently every time. Grepping the
July note for "no provider in the graph" returns nothing; grepping it for
`x/crypto` returns the row that already held the truth. Searching by
wording produces a false negative that reads exactly like a clean sweep.

And the search is RECORDED with its command, so a negative result becomes
evidence ("swept for `x/crypto` across docs/, nothing beyond the July
note") rather than an assumption ("nothing found", when nothing was
looked for).

### R4 — an absolute is cured by scoping, never by substitution

The seed's clause (c) is kept, with its shape made explicit so the form
does the work instead of the author's vigilance. The cure of a false
absolute is: delete the quantifier, name the MEASUREMENT, name the SCOPE,
and state what is no longer claimed.

A cure written in that shape cannot be a new absolute, and it is rejected
at writing time by its form alone, without anyone having to check whether
it is true. The hygiene P1-1 is the test case: "measured by `go mod why`
over the main module's package graph under `imports.AnyTags()`, no path
reaches `x/crypto/ssh` from a published binary" has no grammatical room
for "no provider in the graph at all". The false absolute could not have
been written in the required form.

### R5 — the rewrite inherits the review (added; not in the seed)

A cure is a NEW claim of the same class and carries the same obligations
as the sentence it replaces. Nothing in the seed says so, and it is the
half the evidence most insists on: two of R15's three P1 in the third pass
were cures, and the hygiene batch's P1-1 was a cure.

The current process does catch these, by re-reviewing the complete diff at
every pass. It catches them at the cost of a full round each time. R5 is
what moves the check from the round to the keystroke; it is the clause with
the largest expected saving and the one most likely to be skipped, because
a cure feels like the end of the work rather than the start of a claim.

## 4. What this cannot do

**No gate enforces it.** A textual lint that flags quantifier tokens
lacking an adjacent address is buildable, and it is a structural proxy at
best. CLAUDE.md warns specifically against a textual gate that stays green
while the property is false, and this is that gate: it cannot tell an
address that supports the sentence from an address that does not. If it is
built, its green must be documented as meaning nothing on its own.
Recommendation: do not build it in the same train that adopts the rule.
Measure the rule first, with §5's count. A proxy adopted alongside an
unmeasured rule will be credited with the rule's effect.

**It does not catch a claim that is wrong at its address** — a misread
line, a stale citation, a command whose output was skimmed. It moves the
error from unfalsifiable to one-step falsifiable. That is the whole gain,
and it is smaller than "the class is eliminated".

**It costs writing time**, on every quantified sentence, including the ones
that would have been true. The count in §5 exists so that cost can be
weighed against a measured benefit instead of an assumed one.

## 5. How we will know it worked

The next train's canto records, per adversarial pass: total findings by
severity, and how many belong to THIS class — a quantified or absolute
claim wider than its wire — split by where they landed (paper and canto
prose, versus code, godoc and public copy).

Baseline to beat, from the two trains in flight:

| Train | Pass | P1 of this class | Note |
|---|---|---|---|
| R15 | third | 3 of 3 | two of them were cures of earlier P1s |
| Hygiene | second | 2 of 2 | one was a cure; one was a false evidence label |

The rule is judged at the next train's `VETO LEVANTADO`, against those
numbers. If the class does not shrink, the diagnosis in §2 is wrong about
the cause and the rule is WITHDRAWN rather than reinforced with more
clauses. A method rule that survives its own refutation by being made
stricter is the same disease at the level of process.

## 6. Where it would live

If accepted: a section of `CLAUDE.md` adjacent to "Claim-class sweep",
which it extends. That section acts at REVIEW time, over a finished diff;
this one acts at WRITING time, over a single sentence. They are the same
property enforced at two moments, and naming the relation keeps the second
from being read as a duplicate of the first.

Not accepted, and this document is deleted with the train that proposed
it. It carries no behaviour and leaves nothing to roll back.
