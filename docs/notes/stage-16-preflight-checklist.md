# Stage 16 — pre-flip checklist report (Phase A gate)

> **Run by Claude Code on the Mac against the real git history**, ADR-0026 §"The
> pre-flip checklist". This is the artefact the gate produces. The **flip
> (Phase B) and the first tag (Phase C) are Sebastián's manual acts** — this
> report is what he reads before flipping. Items 5–7 are **Sebastián's decisions**
> and are left PENDING here by design.

**Date of run:** 2026-07-04 · **Repo state:** `master` (public flip NOT done) ·
**Commits in history:** 177 reachable.

## Result summary

| # | Item | Owner | Verdict |
|---|------|-------|---------|
| 1 | Secret-scan ALL history (gitleaks + trufflehog) | Claude Code | ✅ GREEN |
| 2 | Actions logs print no secrets | Claude Code | ✅ GREEN |
| 3 | Non-git surface (issues/PRs/wiki/projects/desc/topics/artifacts) | Claude Code | ✅ GREEN (1 note) |
| 4 | `.gitignore` covers `*.local.json`, `dist/`, `.gstack/` | Claude Code | ✅ GREEN |
| 5 | Parked `CLAUDE.md` resolved | **Sebastián** | ⏸ PENDING |
| 6 | Author email decision | **Sebastián** | ⏸ PENDING |
| 7 | GitHub panel settings (branch protection / advisories / private reporting) | **Sebastián** | ⏸ PENDING |

The four technical items are green. The three items below the line are Sebastián's
to decide (see the bottom of this file).

---

## 1. Secret-scan of the full history — ✅ GREEN

Two independent dedicated scanners, over the whole history (not hand-grep):

- **gitleaks 8.30.1** — `gitleaks git` on `HEAD` and again with `--log-opts=--all`
  (all refs): **`no leaks found`**, 160 commits scanned, both runs.
- **trufflehog 3.95.8** — `trufflehog git file://.` over full history
  (verified + unverified): **0 findings**.

Targeted confirmations the ADR calls out:

- **`*.local.json` never committed** — `git log --all -- '*.local.json'
  '**/*.local.json'` is empty. `configs/korvun.local.json` exists only in the
  working tree, is **untracked**, and is **git-ignored**. The Stage 11 `git add -A`
  scare left no trace.
- **Telegram bot token** — regex `[0-9]{8,10}:[A-Za-z0-9_-]{35}` across every blob
  in `git rev-list --all`: **absent**.
- **Groq key** — pickaxe `gsk_` hits only **test fixtures** (`gsk_k`,
  `gsk_testkey`, `gsk_envkey`, and the deliberately-named
  `gsk_super_secret_key_should_never_leak` used by a test that asserts the key is
  NOT leaked into output). No real key (a real Groq key is `gsk_` + ~48 chars).
- **`GROQ_API_KEY=` / `TELEGRAM_BOT_TOKEN=`** occurrences are all `...` placeholders
  in docs or the env-var **name** in tests/demos — never a value.

## 2. GitHub Actions logs — ✅ GREEN

Logs have 90-day retention and become visible on the flip.

- **`quality.yml` passes NO repository secret** — no `secrets.*`, no
  `TELEGRAM_*`/`GROQ_*` env. Tests set fake keys internally via `t.Setenv`. Nothing
  secret can reach a log line.
- **`release.yml`** references only `GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}`
  (the auto-provided token) passed as **env to the GoReleaser step**, never echoed.
  The new cosign step signs keyless via OIDC (no secret in the log); the SLSA
  attestation uses the ambient token.
- No workflow step `echo`s a secret.

## 3. Non-git surface — ✅ GREEN (one note)

- **Visibility:** PRIVATE (flips to public in Phase B — Sebastián).
- **Description:** set, product summary only, no secrets.
- **Issues:** 0 (feature enabled, none filed).
- **Wiki:** disabled (nothing there).
- **Projects:** feature enabled; no issues/cards exist. (Sebastián: a glance at
  Projects before flip is cheap; expected empty.)
- **Pull requests:** 4 total. #1–#3 were CI/merge PRs (no secrets). **#4 is an
  OPEN Dependabot PR** (see note below) — auto-generated content, no secrets.
- **Run artifacts:** the `sbom` job uploads SPDX JSON (dependency manifests) — no
  secrets by nature.
- **Topics:** none set (Sebastián planned `go, ai, llm, messaging-gateway,
  self-hosted, orchestration` — optional, cosmetic, post-flip).

## 4. `.gitignore` coverage — ✅ GREEN

`git check-ignore` confirms all three are ignored: `configs/x.local.json`,
`dist/foo`, `.gstack/bar`. The file covers `*.local.json` + `configs/*.local.json`,
`/dist/` + `dist/`, `.gstack/`, `.env`, `*.db*`, and SARIF/SPDX artefacts.

---

## Items for Sebastián (the gate's human half) — ⏸ PENDING

### 5. The parked `CLAUDE.md`

`CLAUDE.md` is modified in the working tree (the "Design spec first" workflow step)
and has been held uncommitted across many stages. Claude Code did **not** resolve
it (ADR-0026 §checklist item 5 — Sebastián's call). **Decide one of:** commit it
(it becomes public with the repo), discard the change, or keep it local
(`.gitignore` it / `git stash`). It contains no secrets, but it is contributor-facing.

### 6. Author email

`morenosebastian117@gmail.com` is in every commit's authorship across the 177-commit
history. **Recommendation (ADR-0026 §6): accept it.** Rewriting the history to scrub
one email would break every commit SHA the docs/ADRs cite and is not worth it for an
email the author already uses publicly. If Sebastián disagrees, the scrub must happen
**before** the flip (it is a history rewrite).

### 7. GitHub panel settings

Sebastián's, not delegable to Claude Code:
- Branch protection on the (soon) public `master` (already on for private — reconfirm
  it survives the flip / status-check names still match).
- Enable security advisories + **private vulnerability reporting** (needed for the
  `SECURITY.md` flow once public).
- Repo description/topics/social-preview polish (optional).

---

## Additional findings surfaced during Phase A (not gate items, for Sebastián)

- **Open Dependabot PR #4** bumps `actions/checkout` 6→7 and
  `goreleaser/goreleaser-action` 7.0.0→7.2.3. It was **NOT deleted** with the other
  stale branches: unlike the `stage-*` branches (deleted) and `ci/diagnose-*`
  (already gone), this is a **live PR**, and deleting its branch would close it.
  Its CI currently fails. **Sebastián's call** to merge/close it. Note: the
  release workflow pins these Actions by SHA, so the bump is a hygiene update, not
  a break — but v7.2.3 of goreleaser-action ships cosign-v3 test coverage worth a
  look if we later modernise the signing to the v3 bundle format (see below).
- **cosign pinned to v2.6.3** in `release.yml` (installer still SHA-pinned to
  v4.1.2). cosign v3 defaults to `--new-bundle-format` and **drops** the classic
  `--output-signature`/`--output-certificate` outputs GoReleaser's `signs:` block
  produces (verified locally: v3.0.6 errors "create bundle file"; v2.6.3 signs and
  verifies cleanly). Moving to the cosign-v3 bundle format is a **deferred
  modernization**, not this release.
- **Prior master CI run on `adde69d` (docs-only) failed in `sbom` + `cross-compile`
  jobs** while all three `quality` jobs passed. Transient/infra: a docs commit
  can't break a build, and Phase A's `goreleaser release --snapshot` just
  cross-compiled all six targets locally and green. Not a code issue.