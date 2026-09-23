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

   **Against the Sixth Law — SETTLED by the director on 2026-09-23.** `CLAUDE.md`
   reads «ninguna release se etiqueta sin la pasada manual de Chano sobre la
   build empaquetada», marks it CRITICAL, and **keeps that wording**. His ruling:
   the law does not change; what happens is a **DELEGATION DECLARED RELEASE BY
   RELEASE**.

   Said exactly, because the two halves are easy to blur: a declared delegation
   does not make the executor into the director, and **this box does not claim
   the law's letter is satisfied**. What the declaration does is put on the
   record, in each release, that the act the law assigns to him was carried out
   under his instruction and that he saw its evidence before saying yes. Whether
   that is compliance or a standing exception to a CRITICAL law is his to say,
   and he has said the wording stays — so the record is the whole of what this
   box offers, and it offers it release by release, never once and for all.

   So the rule from here: the law stands as written; the executor runs the pass
   under a delegation the release's own notes record by name; and the director's
   **yes** is still the act that publishes. A release whose notes do NOT declare
   the delegation has not been delegated — silence is not consent, and this box
   grants no standing exemption.

   **What is still undefined, filed and not blocking:** the curtain gives the
   director «local build in HIS browser, artifacts to open», and nothing in this
   tree says what those artifacts are. Filed in `docs/HANDOFF.md` to be defined
   in this kit; it blocks no release meanwhile.

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
   body is the release notes verbatim; then publish and install by the install
   mold.

   **When something is learned AFTER the tag** — an install that found
   something, a ruling the director made that day — it goes in a DATED addendum
   at the end of the notes, never by rewriting what was published, and the
   release body is re-pushed so the two stay identical. «Verbatim» binds the body
   to the notes file, not to the notes file as it stood at tagging time.

   **The install-and-confirm is run by the EXECUTOR under the director's
   delegation**, by his ruling of 2026-09-23, on the same terms as box 6: from
   the PUBLISHED artifact, never a local build, with its output recorded in that
   release's evidence directory and DECLARED in its notes. What the executor
   gives is evidence; the director's own hands remain the higher evidence
   whenever he uses them.
