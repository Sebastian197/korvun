# ADR-0026: Stage 16 — Hardening + public release

> **Status:** proposed
> **Date:** 2026-06-28
> **Deciders:** Sebastián Moreno Saavedra (+ copilot review)

## Context

Stage 16 is the last stage of the V1 plan and the one with the most mixed nature:
it bundles an **irreversible visibility flip** (the repo goes public, and the
entire history is visible forever), a **cryptographic trust layer** (signing),
the OpenSSF Scorecard reviving, a hardened systemd unit, developer-facing docs,
and the **first real release tag**. The framing (`/office-hours` +
`/plan-eng-review`, copilot-approved) found these must be **split into phases by
blast radius** (the Stage 14 move), ordered by **reversibility**, with a
**pre-flip verification gate** as the first thing.

The decisive lens is reversibility. **Everything in this stage is additive and
reversible EXCEPT two acts:**

- **The public flip is the one hard one-way door.** Once the repo is public, its
  history can be cloned, forked, and archived (Wayback, GitHub Archive) by anyone;
  you cannot un-publish what was seen. The strongest gate goes before it.
- **A pushed tag / release is a SOFT one-way door** — a tag and a GitHub Release
  can be deleted; only an artifact someone already downloaded cannot be
  un-distributed. So the first tag can safely come AFTER the flip.

### Who does what — EXPLICIT

**The flip and the first tag are Sebastián's acts, NOT Claude Code's and NOT
autonomous.** Claude Code builds the Phase A machinery (signing, hardened unit,
docs) and RUNS the pre-flip checklist on the Mac against the real git history.
**Sebastián** performs the flip in GitHub Settings and pushes the `v0.1.0` tag,
with the pre-flip checklist green in front of him. No tool flips the repo; no tool
pushes the tag.

### What the repo gives (verified by reading, not memory)

- The Stage 15 release machinery is built and `--snapshot`-validated; `go.mod`
  stays at 3 direct deps; GoReleaser is build-time (never in `go.mod`).
- `scorecard.yml` already exists with its automatic triggers commented out and a
  note that they re-enable when the repo goes public.
- Pre-flip scan already run once (see the checklist below for the formal version):
  183 commits show no secret-value patterns; `.gstack` lives in a dangling commit
  unreachable from any ref (not public on flip); no tags; 9 stale remote branches
  exist; the author's real email is in master's history.

### External-docs verification (per CLAUDE.md non-negotiable)

- **cosign keyless (Sigstore OIDC) is the boring, standard trust layer** — it signs
  the `checksums.txt` (one signature verifies every artifact), needs **no key
  management** (no custody, rotation, or HSM — the most error-prone part is simply
  absent), and is build-time (not in `go.mod`). GoReleaser has native `signs:`
  cosign support. Keyless via GitHub Actions OIDC satisfies **SLSA Build L3** for
  provenance. Self-managed signing keys would be the adventurous path — rejected.
- **Action SHAs pinned at source (CLAUDE.md rule for CI tooling), to re-verify at
  implementation time:**
  - `sigstore/cosign-installer` — **v4.1.2 = `6f9f17788090df1f26f669e9d70d6ae9567deba6`**
  - `actions/attest-build-provenance` (only if SLSA provenance enters Phase A) —
    **v4.1.1 = `0f67c3f4856b2e3261c31976d6725780e5e4c373`**
  - `goreleaser/goreleaser-action` and `anchore/sbom-action/download-syft` stay on
    the Stage 15 pins; `actions/checkout` / `actions/setup-go` stay on `@v6`.
- **systemd hardening directives** for a network service are standard
  (`ProtectSystem=strict`, `ProtectHome`, `PrivateTmp`, `PrivateDevices`,
  `ProtectKernel*`, `NoNewPrivileges`, `SystemCallFilter=@system-service`,
  `RestrictAddressFamilies`, `ProtectProc=invisible`). **Korvun-specific catch:
  `DynamicUser` does NOT fit** — Korvun writes the SQLite DB, and DynamicUser's
  unpredictable UID breaks ownership of the persistent file across restarts. Use a
  **static system user + `StateDirectory=korvun` + `ReadWritePaths`**, reconciled
  with `ProtectSystem=strict`.

## Decision

Ship Stage 16 as **three phases ordered by reversibility**, with the pre-flip
checklist (below) as the gate on the one irreversible act.

```
FASE A  ── pre-flip, ALL additive/reversible (Claude Code builds + runs the gate)
   signing · hardened systemd · docs consolidation · delete stale branches · run checklist
        │
        ▼   GATE: the Phase A checklist must be green
FASE B  ── THE FLIP (the one hard one-way door — Sebastián, in Settings)
   repo → public · re-enable scorecard.yml · badges · address findings
        │
        ▼   deliberate, recoverable
FASE C  ── first public release (Sebastián pushes the tag)
   push v0.1.0 → first public, signed release + SBOM
```

### Phase A — pre-flip hardening (additive, reversible, private)

1. **Signing — cosign keyless.** Add GoReleaser `signs:` signing the
   `checksums.txt` with keyless cosign (Sigstore OIDC); install cosign in
   `release.yml` via `sigstore/cosign-installer@<pinned SHA>`; grant the workflow
   `id-token: write`. Validate with `goreleaser release --snapshot` (signing is
   skipped on snapshot, but `goreleaser check` validates the config) and a CI
   dry-run. **SLSA provenance** via `actions/attest-build-provenance@<pinned SHA>`
   is a **fast-follow in Phase A if the cost is low, deferrable if it bites** — the
   keyless OIDC already gets most of the trust; the attestation is the next tier.
2. **Hardened systemd unit** — replace the basic Stage 15 example with the
   hardened directives above, using a **static `User=korvun` + `StateDirectory=korvun`**
   (systemd creates `/var/lib/korvun` owned correctly) + `ReadWritePaths` for the
   storage path, `ProtectSystem=strict`, `ProtectHome=yes`, `PrivateTmp=yes`,
   `PrivateDevices=yes`, `ProtectKernelTunables/Modules/Logs=yes`,
   `ProtectControlGroups=yes`, `NoNewPrivileges=yes`,
   `SystemCallFilter=@system-service`,
   `RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX`, `ProtectProc=invisible`,
   `CapabilityBoundingSet=` (empty — admin binds loopback:2112, Telegram is
   outbound). The config / `storage.path` points under the StateDirectory.
3. **Developer-facing docs — CONSOLIDATE, do not write from scratch.** The material
   already exists: 15 STAGE docs, 26 ADRs, godoc, and Stage 15's `INSTALL.md`.
   Phase A produces a **quickstart**, a **config reference** (distilled from the
   ADRs + example configs), and a **README entry point** that links the STAGE docs
   / ADRs. Expose what exists (Brooks: do not create new complexity). Extension
   guide, control-API reference, and builder guide are deferred (the builder does
   not exist yet).
4. **Delete the 9 stale remote branches** (`ci/diagnose-coverage-macos`,
   `dependabot/*`, `stage-1/*`, `stage-2/*`) — they would otherwise become public
   clutter. master is the only branch the public should see.
5. **Run the pre-flip checklist** (the artefact below) and record its result.

### Phase B — the flip (Sebastián, gated on Phase A green)

Make the repo public in Settings; re-enable `scorecard.yml` (uncomment the
`push`/`schedule`/`branch_protection_rule` triggers); confirm the shields.io / Go
Report Card / Scorecard / CI badges resolve; review and address the first
Scorecard findings (expected: the badge, possibly `Token-Permissions` /
`Pinned-Dependencies` hygiene — most controls already pass: SHA-pinned Actions,
branch protection, minimal `go.mod`, SBOM).

### Phase C — the first public release (Sebastián)

Push `v0.1.0` (not `v1.0.0` — Korvun is functional but pre-1.0: no external users,
the config/API may still change; SemVer 0.x signals "usable, not stable"). The
release workflow fires on the tag and produces the **first public, signed release
with SBOM**. Green before pushing: `make quality` + the signed `release.yml`
validated by `--snapshot` / a CI dry-run.

### The pre-flip checklist — the artefact (the heart of the gate)

Run by **Claude Code on the Mac against the real git history** (NOT a remote
tool). Every item must be green before Sebastián flips:

1. **Secret-scan ALL history with a dedicated scanner** — `gitleaks` AND/OR
   `trufflehog` over the full history (not just hand-grep). Re-confirm
   `korvun.local.json` never carried real values in ANY commit (the Stage 11
   `git add -A` scare), and that the rotated `GROQ_API_KEY` / Telegram token do not
   appear in the 183 commits.
2. **GitHub Actions logs** (90-day retention, become visible on flip) — verify or
   delete old runs, or confirm none printed a secret (`quality.yml` uses the fake
   `KORVUN_TEST_TOKEN`, passes no real secrets).
3. **Non-git surface** — Issues, PRs (descriptions + comments), Wiki, Projects,
   the repo description/topics, and run artifacts: confirm nothing sensitive.
4. **`.gitignore` covers everything sensitive** — `*.local.json`, `dist/`,
   `.gstack/`.
5. **The parked `CLAUDE.md` is RESOLVED** (committed / discarded / kept local) —
   **Sebastián's decision**, part of the gate. Claude Code does NOT resolve it.
6. **Author email** (`morenosebastian117@gmail.com`) — a recorded conscious
   decision. Recommendation: **accept it** (rewriting 183 commits to scrub one
   email is expensive and breaks the commit SHAs the docs cite).
7. **GitHub panel settings** (branch protection on the public repo, security
   advisories, private vulnerability reporting) — Sebastián's work, not delegable.

### Minimal "trusted release" cut

cosign keyless signing of `checksums.txt` is **essential** (one signature, every
artifact). **SLSA provenance** (`attest-build-provenance`) is a **fast-follow**.
**SBOM attestation** and **system packages** (`.deb`/`.rpm`/Homebrew/containers)
are deferred until demand.

## Consequences

### What this enables

- Korvun becomes **publicly installable** (the Stage 15 machinery's external
  consumer is finally unlocked), with a **signed, verifiable** first release and a
  reviving Scorecard badge — and a hardened service unit for production.
- The irreversible flip happens **only after a formal, re-runnable gate** is green.

### Trade-offs accepted

- **The flip is irreversible** — mitigated entirely by the pre-flip gate.
- **cosign keyless** ties trust to the GitHub OIDC identity (no offline key) — the
  accepted, standard trade-off for zero key management.
- **Author email is public** — a recorded conscious decision (scrubbing is not
  worth the history rewrite).

## Alternatives Considered

- **Order 1 (CHOSEN)** — Phase A (reversible) → gate → flip → first tag. The first
  thing the public sees is signed and public.
- **Order 2** — a private `v0.1.0` test tag before the flip. **Rejected:** spends
  `v0.1.0` on a test and the first release is born under a private-repo workflow
  identity; `--snapshot` already validates the signed machinery without a tag.
- **Order 3** — flip first, then harden/sign/docs. **Rejected:** the irreversible
  act happens before the gate, the trust layer, and the cleanup.
- **Self-managed signing keys** instead of cosign keyless. **Rejected:** key
  custody/rotation is the most error-prone part; keyless removes it.

## Out of scope (recorded)

SBOM attestation, system packages (`.deb`/`.rpm`/Homebrew/container images), the
extension / control-API / builder docs, and any Stage 14 Phase 2 builder work.

## Delivery — phased, Claude Code builds A, Sebastián owns B and C

Phase A is build-time/CI/docs (reviewable, additive, reversible) plus the pre-flip
checklist run. The **flip (Phase B) and the first tag (Phase C) are Sebastián's
manual acts**, performed with the checklist green. Each Action SHA is re-verified
at source before it lands in a workflow. `make quality` stays green; `go.mod`
stays at 3 deps (cosign + GoReleaser are build-time).