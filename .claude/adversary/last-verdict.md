VETO LEVANTADO ee43f6b45c6a378cb516f7e0aea23992b24b0640

Round: the hygiene batch of 2026-09-08 — a supply-chain train with NO
production Go code (sixteen files: two Go module lines, two npm
manifests, five workflows, the operating rules, SECURITY.md, three
historical documents, the handoff and this train's canto).

READ THE NEXT PARAGRAPH BEFORE READING THE FIRST LINE AS AN AUDIT
RESULT. The wire that reads this file reads only its first line, and its
own check script says what that line means: "a marker whose FIRST LINE
names the direct sole parent was committed", never "audited". Here is
what it does not say.

**The veto was lifted by the DIRECTOR, on 2026-09-08, not by the
adversary.** The seventh adversarial pass over this train returned VETO
MANTENIDO with two P1, four P2 and four P3. Both P1 were cured before
this commit — one of them was a section this canto claimed to have
rewritten and had not, because a script of mine failed halfway and I did
not verify it landed; the other was a line citation that two successive
"repairs" left pointing at the wrong pair of lines in the wire. An
eighth pass was NOT run: the director ordered delivery today and ruled
that whatever it would have returned is FILED WITHOUT DETAIL. That
filing is this paragraph, and it is the honest name of what happened.

What the train does carry, and what a reader of this marker can check:

- SEVEN adversarial passes ran over the batch. Every one returned VETO
  MANTENIDO, and each declared what it found that the pass before it did
  not see: the canto records all seven and names its own defects rather
  than only the tree's.
- The class the passes kept finding was mine, not the wire's: sentences
  wider than their evidence, closed enumerations short by one, and
  documentary counts that rotted under the very cures that corrected the
  previous count. The wire's own changes — the version bumps, the pins,
  the labels — were verified early and stayed stable.
- The four gates ran green over the FINAL tree: `make quality`,
  `make website-check` (24 e2e), `govulncheck` over the pruned package
  list (41 packages, the form this batch writes into the rules) and
  `actionlint` v1.7.12. All exit 0.
- Two dry-runs — `release.yml` and `release-desktop.yml`, both dispatched
  with `dry_run=true` — are preconditions of the MERGE, not of this
  push, and had not run when this marker was written.
- One weakness this batch found and could not close is published rather
  than buried: the `goreleaser-action`'s checksum and cosign
  verification skips with a warning when its fetch fails, so a blocked
  fetch yields an unverified binary and a green job. It is written into
  SECURITY.md's supply-chain section, and the cure is filed with
  declared priority for v0.15.1.

STATE: IMPLEMENTED. Not VERIFIED — that word belongs to the gate, and
the two dry-runs and the `ensayo` have not run. Not ACCEPTED — that word
belongs to the director, and the merge is his act.
