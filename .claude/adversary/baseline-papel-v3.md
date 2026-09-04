# Fire-test baseline — the internal adversary vs the v3 paper (2026-09-04)

The freshly built adversary audited the R11 v3 design paper BLIND
(without the external auditor's verdict). Its verdict: VETO MANTENIDO,
1 P1 + 3 P2 + 4 P3. Full report preserved in the session log of this
date. Baseline notes:

- Of the third external veto's 8 points, only two are reconstructable
  from the director's mandates by number: point 6 (the R1 oracle must
  survive a DDL-swap mutant and verify post-state) — CAUGHT, including
  the exact DDL-swap reproduction; point 8 (repair support labeling /
  platform gate) — PARTIALLY CAUGHT (the sqlite3 host dependency and
  the unverified-Windows gate; the "supported" label had already been
  fixed in HEAD). The remaining six points of the external veto were
  never shown textually to this session; the director holds the full
  cross-count.
- The adversary also found issues NOT known to be in the external
  veto, notably: a same-life action-row rewrite escaping with exit 0
  under the paper's custody redesign (its P1, with the undeclared
  loss and the renounced receipts-are-never-pruned oracle); the 7b
  redesign leaving live-approval read errors without a named class;
  the AST guard covering the type instead of the invariant, with no
  mutation; R6/R7 born green against HEAD with no possible red; the
  Error()-vs-Class rendering contradiction; the empty-digest gate.
- This file is the honest baseline, not an exam pass.
