VETO LEVANTADO 3765f71ddd2cab27fee12273a2cb493d3c346218

Round: v0.15.1 public truth (2026-09-20). No production code: the tag was
cut on `261ab3f`, the two release lanes published 23 assets, and this train
moves the public claims to the release that now exists, plus the director's
new rule for short prose. Two commits: `c3c335b` (the public surfaces) and
`3765f71` (the rule, in `docs/HANDOFF.md`).

WHO HELD THE VETO, and how it was lifted. The internal adversary ran TWO
passes, both VETO MANTENIDO, and both are declared here rather than hidden:

- First pass, over the whole diff: P2-1 — the new releases-table row, EN and
  ES, enumerated two Known issues behind a colon and dropped the «including»
  hedge the row it demotes carries, on a surface whose own contract is
  "unsoftened". Cured in both locales, with the pointer to the notes restored.
  Five P3 came with it and all five were cured in the same tree: the row said
  «a digest mismatch» where only `params_digest_mismatch` is meant (the sister
  outcome leaves the request PENDING, so the sentence was false of it); the
  Known issue said «with a capture» of an artifact that is not in the tree;
  «Copy only» was silent about the state the scenario leaves; three published
  surfaces still spoke from before the publication (`SECURITY.md`, the
  operator-CLI reference page in both locales); and the ceremony record still
  opened claiming every command was executed while P2-11 says nothing in the
  tree binds it to a real run.

- Second pass, scoped to those cures: P2-1 cured in both locales, every P3
  cured, and a NEW P2-2 created by the cures themselves — the commit message
  claimed `docs/releases/v0.15.1.md` was synced with the body published for
  the tag, which the cures had made false by 453 bytes. Its lift condition was
  mechanical and it was EXECUTED: the cured file was published with
  `gh release edit --notes-file`, the byte comparison was re-run through the
  API (published 14288, file 14288, `IDENTICAL: True`), the message was
  reworded to say what happened, and its arithmetic was corrected (five issues
  filed to v0.15.2, a sixth bullet declaring itself unscheduled).

WHAT WAS NOT RE-PASSED, and under whose order. By the director's order of
2026-09-20 there is NO third pass over that last correction, and none over the
`docs/HANDOFF.md` paragraph: for this class the sole verification is the sixth
question — every corrected line was re-read ON DISK after being written — and
the marker says so instead of implying a pass that did not happen. The same
order becomes the standing rule this train registers: any change that is only
text and under ten lines gets at most ONE adversary pass, and what it returns
is cured in the same tree and declared.

WHAT RAN on this tree: `make quality` green (exit 0, «Quality gate passed»,
3m21s over the final tree); the three website guards green before and after
the cures — `check-current-release` («the five current-release claims name
v0.15.1»), `check-release-facts` («every release version flows from
releaseFacts.ts» and the online comparison «latest-release comparison OK
(v0.15.1)») and `check-parity` (13 page pairs); the adversary's own five
probing mutations of `check-current-release` on a copy, each red with its
named reason and restored. The pre-commit gate ran on every commit of the
train and announced its one skip by name («staged tree is identical to
f52b72b — its green stands»).

EVIDENCE LEVEL, honest: this is prose over published surfaces. Nothing here
is proved by a new test, because nothing here changes behaviour — the only
executable bytes that moved are the two literals of `website/src/releaseFacts.ts`.
The release itself was verified after publication, captured: cosign
`Verified OK` over `checksums.txt` and `checksums-desktop.txt`, both build
provenance attestations bound to the exact local digests and to the workflows
at `refs/tags/v0.15.1`, `shasum -a 256 -c` clean over both manifests, six
SPDX-2.3 SBOMs, and the published `.dmg` printing `korvun-desktop v0.15.1`.

STATE: IMPLEMENTED. Not ACCEPTED: the merge is the director's.
