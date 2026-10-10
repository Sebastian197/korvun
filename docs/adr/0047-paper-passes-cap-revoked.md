# ADR-0047: The two-pass cap on pre-test papers, and its revocation

> **Status:** accepted
> **Date:** 2026-09-08
> **Deciders:** the director

## Context

Dates: the cap entered CLAUDE.md on 2026-09-06 (ed4f3c6), the director
revoked it on 2026-09-08, and its text left CLAUDE.md for this record on
2026-10-10.

R14 was a documentary train whose pre-test paper took FIVE adversarial
passes while the train wrote no production code at all. The paper cost
more than the code that did not exist.

## Decision

The director first capped paper passes. A train with NO production code
admitted at most TWO paper passes: the 9-bis over the design, and ONE
scoped re-pass over its cures. Beyond that the rule was fixed and was not a
judgement call:

- what folds in a LINE is folded, in place, and the text ships;
- what would demand a REDESIGN of a sentence is FILED to the next train
  WITH its capture, and the text ships with what it has — EXCEPT a
  sentence known to be FALSE, which is NEVER shipped: it is scoped down
  or deleted in the same commit that notices it, and only its full
  redesign is deferred. The Tone law admits no exception and this
  ceiling grants none.

What was capped was the number of rounds spent perfecting prose. Nothing
else moved: the destructive doctrine's evidence requirements stood
untouched — a capture is still a capture, a probing mutation is still a
probing mutation, and a letrero wider than its wire is still a
public-truth violation the moment it is noticed. A CI step is the same
discipline in another surface: every new step is born pinned OR with its
exception written in the same commit, and a Scorecard alert opened by our
own commit is debt of the same train.

On 2026-09-08 the director REVOKED the cap. A pre-test paper now iterates
until the adversary lifts its veto, with no round limit, and every pass
declares what it found that the pass before it did not see. Everything
the cap said about EVIDENCE stands and is not weakened by the revocation.

## Consequences

The governing rule lives in CLAUDE.md under "Paper passes"; this record
keeps the reasoning of the revoked cap out of the rule an agent reads
every session. A paper may now take more rounds than its train's code
justifies; the cost is accepted in exchange for never shipping a paper
the adversary still vetoes.

## Alternatives Considered

Keeping the revoked text inside CLAUDE.md "for the reasoning it records"
(the state until this ADR): rejected because an agent reads a dead rule
next to the live one on every session.
