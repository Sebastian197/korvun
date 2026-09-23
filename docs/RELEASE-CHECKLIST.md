# Release checklist — permanent law

Every Korvun release walks these eight boxes IN ORDER. A release that skips
a box is not a release; a box is checked only with evidence in hand (the
work-state semantics apply: IMPLEMENTED ≠ VERIFIED ≠ ACCEPTED). Instituted
by the director's consolidated mandate on 2026-08-29, first executed for
v0.10.0.

## The eight boxes

1. **Software finished and approved.** Every piece of the release arc is on
   `master` through the green path, and the manual bug bash over the PACKAGED
   build has APPROVED it — zero use-breakers open. Who runs that bash is settled
   in box 6, and box 6 also records where it stands against the Sixth Law.
2. **Identity faithful to the sealed design.** Any brand or user-visible
   visual work matches the mockup the director approved with his eyes —
   side-by-side verification by the implementer BEFORE asking for his; his
   corrections outrank the mockup.
3. **The packaged artifacts carry the identity.** App icon
   (macOS/Windows/Linux via the Wails mold), README masthead, avatar and
   social preview all regenerate from the governed sources; the CLI banner
   tells the same story.
4. **Docs and ADRs closed.** ADRs updated for every identity or
   architecture decision; release notes written; `assets/brand/README.md`
   and the master documents current.
5. **Every public surface tells the release's truth.** Release notes,
   README, website landing facts EN+ES, website DOCUMENTATION EN+ES in
   parity (every new user-facing feature explained; no stale UI
   descriptions or captures), repo user guides. Every claim verified
   against the real product — the public-content law.
6. **The visual curtain, and who holds the packaged build.** The director sees
   the REAL thing (local build in HIS browser, artifacts to open) and says yes.
   Nothing publishes without that yes; retouches get another curtain.

   **The pass over the PACKAGED build is run by the EXECUTOR under the
   director's delegation** — recorded here by his instruction of 2026-09-23. His
   reason: that it is what had been happening for v0.15.0, v0.15.1 and v0.16.0
   while box 1 and the release notes said the act was his. That reason is HIS
   WORD, not a tree record — no packaged pass before v0.16.1 left a file, so
   nothing here can confirm or deny it, and it is written as what it is.

   **Against the Sixth Law, stated plainly and UNRESOLVED.** `CLAUDE.md` still
   reads «ninguna release se etiqueta sin la pasada manual de Chano sobre la
   build empaquetada», and marks it CRITICAL. This box does not amend that law
   and cannot: the law is the director's, and only he moves its text.

   So the gap is written, not closed. The law names two things — a manual pass
   BY THE DIRECTOR, over the PACKAGED build. The bash of box 1, which is the act
   the law names, is delegated here. What the curtain of this box gives him is
   «local build in HIS browser, artifacts to open», and **this checklist does not
   define what those artifacts are**: nowhere in the tree does anything say
   whether they are the packaged ones. So whether the
   curtain already puts a packaged artifact in his hands before the tag is
   **undecided here**, and this box does not decide it in its own favour. What is
   certain is box 8, where «his own hands confirm» an installed build — and that
   happens AFTER the tag, while the law speaks of tagging.

   **Nothing in this box claims the Sixth Law is satisfied.** Whether the law's
   wording changes, whether the practice moves back to it, and what «artifacts to
   open» means, are open decisions of the director; until he settles them this
   stays an open gap on every release that passes through here.

   What the delegation does NOT move: the **yes** is still the director's, and
   the executor's pass is evidence for that yes, never a substitute. The pass
   runs on a DISPOSABLE profile — never the director's own — exercises the
   surfaces the release changed THAT THE PACKAGED APP EXPOSES, names by hand any
   surface it therefore cannot reach (an operator CLI door, for one), and leaves
   its verbatim record in
   `docs/superpowers/specs/evidence/<tag>/packaged-pass.txt`. Where a release
   touches a screen, a CAPTURE of that screen lands in
   `docs/assets/captures/<tag>/`, which is where this repository keeps images,
   and the record names it by path.
7. **Green path push.** Whole-suite gates green locally (unit, e2e,
   typecheck, quality, brand/motion/contrast/parity/dist/docs; lockfiles
   untouched), `govulncheck` clean with the pinned version, rehearsal push
   full green with ZERO reruns, then master, three refs verified, and the
   public site checked from outside.
8. **Tag draft-until-complete + install.** The tag's release stays draft
   until every signed asset is present and API-verified (the v0.9.2 mold:
   21 assets, cosign manifests, SBOMs, the app with the current icon), the
   body is the release notes verbatim; then publish, install on the
   director's machine by the install mold, and his own hands confirm.
