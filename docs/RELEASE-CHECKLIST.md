# Release checklist — permanent law

Every Korvun release walks these eight boxes IN ORDER. A release that skips
a box is not a release; a box is checked only with evidence in hand (the
work-state semantics apply: IMPLEMENTED ≠ VERIFIED ≠ ACCEPTED). Instituted
by the director's consolidated mandate on 2026-08-29, first executed for
v0.10.0.

## The eight boxes

1. **Software finished and approved.** Every piece of the release arc is on
   `master` through the green path, and the director's manual bug bash over
   the packaged build (the Sixth Law) has APPROVED it — zero use-breakers
   open.
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
6. **The visual curtain.** The director sees the REAL thing (local build in
   HIS browser, artifacts to open) and says yes. Nothing publishes without
   that yes; retouches get another curtain.
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
