VETO LEVANTADO b22468728102ce945fc75f8d185e08cf240feecb

Round: R14, the truth of the ledger — a train with NO production code
(diff 49b7974..b224687, twenty-eight commits).

The R13 verdict this file carried until now is preserved verbatim in
git at `49b7974:.claude/adversary/last-verdict.md`; read it with
`git show 49b7974:.claude/adversary/last-verdict.md`. AMENDMENT to it,
dated 2026-09-07: its sentence "P3-4, P3-6, P3-7 ... each with its
probing mutation executed and recorded" is wrong. The ledger
(`docs/cantos/R13.md`, the eighteenth pass's own paragraph) records TWO
mutations, m-a21c and m-a21d, for P3-4 and P3-7, and says so in its own
words — "Both elevations carry their probing mutation, executed". P3-6
was a godoc scope-down with NO mutation; its other half is FILED in
R13's §8, under R13's own label, the entry beginning "FILED, not cured
(the eighteenth pass, P3-6's other half)". The marker said "each" over
three findings where the ledger records two mutations over two. Before
this sentence was written, `git show` was run against that object and
its md5 compared with the file on disk: c91709589933bbc610f808839448ad1b
both times.

HOW THIS VERDICT WAS REACHED, because the wire reads only the first
line and a reader deserves the rest:

- NINETEEN adversary passes ran over the COMPLETE diff. Eighteen ended
  VETO MANTENIDO. The nineteenth found no P1 and no P2 — nothing false
  in the code, the godocs, the specs or the public copy — and its four
  P3s were folded in the commit this marker names.
- R14 discovered by EXECUTION that Korvun's central public claim was
  false in three ways: a tail cut leaves no hole; an attacker who can
  write the store registers a key of his own, re-signs every receipt
  and re-links the chain, and both verifiers exit 0; a redirected
  config judges another book. Five captures by the BUILT BINARY in a
  separate OS process, and the truth was corrected across the public
  surfaces before it was propagated.
- THREE more absolutes died by execution during the cure loop, each
  one a sentence this train had shipped: `OpenReadOnly` creates a WAL
  store's sidecars; on a store not already in WAL it rewrites the
  journal header (sha256 before and after: equal for WAL, DIFFERENT for
  `journal_mode=delete`); and the same DSN over an absent path creates
  the file, which survives Close. Every one is scoped now, in the
  godoc, in both website locales and in the release notes.
- THE PIN: `TestLedgerCheck_chainReSignedWithASelfRegisteredKeyIsNOTDetected`
  fixes the forgery blind spot so the capture cannot decay. Its
  conforming mutation is M6 (the ladder's key-lookup arm inverted → the
  pin's own assertion RED with `key_unknown`); M7 (the forger's
  registration deleted) proves its green depends on the sabotage
  landing; M4 separates it from the sequence walk. FIVE inherited
  asserts were ELEVATED — three by the director's D7, two because this
  train's own cures reached inside their bodies — each with its own
  executed mutation: M8, M9, M10, M11+M12, M13. M10 and M11 are
  declared NON-discriminating, with the reason, rather than sold as
  proofs they are not.
- WHAT THE CURES THEMSELVES BROKE, and the argument for D4: a letrero
  minted by a cure appeared in almost every round — a cardinal on the
  website, a quantifier in the release notes, a count in §8, a false
  claim about which failures the verifier prints, a false consequence
  about partitions, an absolute about the store's CONTENT, another
  about "every statement" — each caught by the pass AFTER the cure that
  made it. Nine line citations rotted, every one of them moved by an
  edit this train made. A sweep by hand does not converge; the CI guard
  for the letrero class is filed.
- Standing residuals, all in `docs/cantos/R14.md` §5 and §8: the
  external anchor family filed to v0.15.1 (the last sealed hash, a
  verification key outside the profile, "at most one active key" in the
  TABLE, the verifier asking WHICH key is authoritative, an anchor of
  STORE IDENTITY, and a read-only DSN); the TAIL pin with its whole
  M-analysis; L5's rule, that every cited execution leave a versioned
  artifact — this train's own binary captures are unreproducible from
  git and say so; the rename of `TestReceiptVerify_namesEveryFailure`;
  `--partition`, a public flag documented nowhere; and one SQLITE_BUSY
  seen once under a mutated run, not reproduced, filed OPEN rather than
  narrated away.
- The PUBLISHED v0.14.0 release body on GitHub still carries the old
  wording and must receive this one — Chano's act, scheduled for the
  day of the v0.15.0 tag.
