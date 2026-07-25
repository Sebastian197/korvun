# Piece 5 — SP7: packaging + desktop CI — Design Spec

> **Status:** draft (awaiting copilot review — TWO-TIME delivery: this is the
> spec + cut plan ONLY; zero implementation lands with it). **All clarifications
> resolved 2026-07-25:** NC-1 (name) + NC-2 (artifact scheme) copilot-confirmed;
> NC-3 (release window) resolved by Chano — CLOSE via draft-until-complete (§1e),
> a bounded exception to "headless untouched" (visibility flag only).
> **Governing ADRs:** ADR-0035 §2 (release cadence NC-5), §6 (packaging matrix
> NC-4 + ephemeral-port policy), §7 (signing NC-1/2), §8-of-framing (closure
> criteria); ADR-0036 (`wailsapp/wails/v2@v2.13.0` — capability includes
> "packaging glue (.app, NSIS)"); ADR-0025 (GoReleaser headless pipeline,
> `-ldflags -X …version`); ADR-0029 §4/§6 (committed-stub embed, dist never
> trusted from git). External-docs note: every Wails CLI fact below is
> Context7-verified against `/websites/wails_io` (the v2 stable docs) on
> 2026-07-25 — `wails build` flags, the `build/` scaffold, `wails.json`
> `Info`/`frontend:*` fields, `darwin/universal`, `-nsis`, `-webview2`,
> `-s`; the `actions/setup-node` Node-24 fact is source-verified at the v5.0.0
> release notes. No fact here is from memory. **Inherited as law:** the
> headless `cmd/korvun` **binary** stays byte-for-byte unaffected (ADR-0035
> consequence; proven by a `go version -m` + compiled-bytes diff). The only
> headless-pipeline change is a **Chano-authorized, bounded exception**
> (2026-07-25): `.goreleaser.yaml` `release.draft: false → true`, a
> release-VISIBILITY flag that closes the NC-3 window (§1e) — the compiled bytes,
> checksums, SBOM, and signing are all unchanged. The env-only secret contract is
> untouched (this sub-phase adds no secret surface).

## Goal

After SP7, Korvun Desktop is **packageable into the ADR-0035 §6 v1 artifacts**
from a **single deterministic recipe** and a **tag-triggered CI lane on native
runners**, attached to the **same GitHub release** as the headless binary and
**cosign-signed on the free chain** — with **zero paid platform signing**. What
exists afterwards that does not today: a `make desktop` target that rebuilds
**both** frontends and then the desktop binary/bundle with no reliance on cache
or the working tree; a `wails.json` + committed `build/` scaffold that turns
`wails build` into the universal `.dmg` (macOS), the NSIS installer (Windows
AMD64), and the `tar.gz` (Linux AMD64); a real stamped version (goodbye "dev");
a `release-desktop.yml` workflow that builds on macos/windows/ubuntu runners and
uploads to the tag's release without touching the headless pipeline; and
`govulncheck -tags desktop,production` in that lane. **Out of scope (deferred):**
the hardware validation of the artifacts (SP8, on Chano's iMac — the install-guide
screenshots of the Gatekeeper/SmartScreen workarounds are captured there); every
v1 exclusion ADR-0035 already fixed (Windows/Linux ARM64 desktop, `.deb`/AppImage,
auto-update, tray/login-start, app stores, remote mode).

## The central decision — packaging path (§1a)

**DECISION: adopt the `wails build` CLI (Option B).** `go build` stays the
inner-loop/dev compiler, but the **release artifacts are produced by
`wails build`**, and the scaffold (`wails.json`, `build/appicon.png`,
`build/darwin/Info.plist` + `entitlements.plist`, `build/windows/installer/*`)
is part of cut 7a.

### The two options, evaluated against verified v2.13 docs

| | **A — keep `go build`, hand-roll packaging** | **B — adopt `wails build`** |
|---|---|---|
| macOS `.app` bundle | hand-build `Contents/{MacOS,Resources,Info.plist}`, icon `.icns` conversion, dylib layout | `wails build` bundles `Info.plist` + assets, writes `build/bin/<App>.app` (verified) |
| Universal (amd64+arm64) | build both arches, `lipo -create` by hand | `wails build -platform darwin/universal` does the dual-arch lipo internally (verified) |
| Windows installer | author + invoke an NSIS script by hand, wire the WebView2 bootstrap | `wails build -nsis -webview2 download` emits the NSIS installer with the download-strategy bootstrap (verified; matches ADR-0035 R4/SP1 Gate 2) |
| WebView2 strategy | hand-roll the runtime bootstrap | `-webview2 download` build flag (verified — the SP1 Gate-2 precision) |
| New build-time tool | none | the `wails` CLI (`go install …/cmd/wails@v2.13.0`) — build-time only, **not** a `go.mod` dep (GoReleaser precedent) |
| Scaffold to own | none | `wails.json` + `build/` assets committed and customized |

**Rationale.** ADR-0036 already adopted `wailsapp/wails/v2` and **counted
"packaging glue (`.app`, NSIS) on 3 OSes with `darwin/universal`" as the
capability gain** and named hand-rolling it "the hand-roll axis returning through
the back door." Option A re-incurs exactly that: the `.app` structure, `lipo`
universal, the NSIS script, and the WebView2 bootstrap are all non-trivial
per-OS machinery that `wails build` already provides and that upstream
maintains. Option B keeps the framework and its packaging half consistent, uses
only source-verified flags, and adds no runtime dependency (the CLI is
build-time like GoReleaser/Syft). The residual manual steps (below) are small
and are needed under **either** option, so they do not tilt the decision.

### What `wails build` does NOT give (honest residue, manual under either option)

- **macOS `.dmg`:** `wails build` produces a `.app`, **not** a `.dmg` (verified —
  the darwin output is `build/bin/<App>.app`). NC-4's "`.dmg` universal" is a
  **wrap step** around the universal `.app`: **`hdiutil create`** (macOS
  built-in, zero external tool — folded over `create-dmg`, which needs a Homebrew
  install on the runner for only cosmetic window styling).
- **Linux `tar.gz` (binary + `.desktop`):** `wails build` on Linux emits a bare
  binary; the `tar.gz` bundling the binary + a `.desktop` launcher file is a
  **manual `tar` step** (NC-4 spells this exact shape).
- **macOS min version / SP6 CGO note:** the universal build carries
  `CGO_LDFLAGS="-framework UniformTypeIdentifiers"` (the macOS-13 SP6 fact) and,
  if a floor is set, `-mmacosx-version-min` via `CGO_CFLAGS/CGO_LDFLAGS`
  (verified as the documented override path).

## The deterministic recipe (§1b) — law: both frontends, always, before the binary

The re-baking lesson (private pass, 2026-07-25): a build that trusts a cached or
working-tree `dist/` ships a stale/absent frontend. **Rule:** the release recipe
**always rebuilds BOTH frontends fresh**, then the binary, in one target.

**`make desktop`** (new target) chains, in order, failing hard on any step:

1. `web/builder` → `npm ci && npm run build` (regenerates `web/builder/dist`
   fresh — ADR-0029 §6 "never trust a committed dist").
2. `cmd/korvun-desktop/frontend` → `npm ci && npm run build` (regenerates the
   chrome `dist` fresh).
3. `wails build -s -skipbindings -tags desktop,production -ldflags "-X main.version=$(VERSION)"`
   (+ `-platform`/`-nsis`/`-webview2` per OS, §1c) with the macOS `CGO_LDFLAGS`.
   **`-s` skips wails' own single-frontend hook** (verified flag) — Korvun embeds
   **both** dists through its **own** `go:embed` (the SP6a/ADR-0029 committed-stub
   pattern), which wails' one-`frontend/`-dir model cannot express. So wails is
   used for the window + packaging, never for asset building; the two-frontend
   determinism lives in steps 1–2, not in `wails.json`. **`-skipbindings`
   (review #6):** with `-s` alone, `Build()` still runs `GenerateBindings`
   (verified at `wails/v2@v2.13.0/pkg/commands/build/build.go:132-138`), which
   would regenerate `wailsjs/` TS AFTER step 2 already built the chrome — so step
   3 is pinned to a pure compile+package by skipping bindings (Korvun's binary
   embeds its own `frontend/dist`, not wails' bindings; bindings stay committed).

`wails.json`'s `frontend:build`/`frontend:install` are set to **no-op/echo** (or
point at the make steps) precisely so a stray `wails build` without `-s` cannot
silently rebuild only one frontend. **CI never trusts the working tree:** the
lane runs `make desktop` from a clean checkout every time.

## The matrix, exactly (§1c) — nothing beyond ADR-0035 §6 v1

| OS | Recipe | Output | v1 artifact |
|---|---|---|---|
| **macOS** | `wails build -platform darwin/universal -clean -s …` → `build/bin/Korvun.app` (universal via wails' internal lipo), then `hdiutil create` wrapping the `.app` | universal `.app` → `.dmg` | **`korvun-desktop_<ver>_darwin_universal.dmg`** |
| **Windows AMD64** | `wails build -platform windows/amd64 -nsis -webview2 download -s …` | NSIS installer (`build/bin/…-installer.exe`) | **`korvun-desktop_<ver>_windows_amd64-installer.exe`** |

> **NSIS silent-failure guard (source-verified, review #2):** `wails build
> -nsis` calls `GenerateNSISInstaller`, which — verified at
> `wails/v2@v2.13.0/pkg/commands/build/nsis_installer.go:53-56` — prints
> *"Warning: Cannot create installer: makensis not found"*, returns `nil`, and
> **exits 0 with no installer** if `makensis` is not on PATH. The 7b Windows
> job therefore MUST (a) ensure `makensis` is installed on `windows-latest`
> (install NSIS if not preinstalled) and (b) **assert the installer file exists
> before `gh release upload`** — never rely on `wails build` to fail. This is
> the Windows twin of the Linux build-deps step.
| **Linux AMD64** | `wails build -platform linux/amd64 -s …` → bare binary, then `tar czf` bundling the binary + a `korvun-desktop.desktop` launcher | `tar.gz` | **`korvun-desktop_<ver>_linux_amd64.tar.gz`** |

Universal is achieved by wails' `darwin/universal` platform (dual-arch compile +
lipo, verified — no hand `lipo`). All desktop builds run on **native runners**
(cgo forbids the ×6 cross-compile; ADR-0036 maintenance axis). **Nothing else in
v1** — Windows/Linux ARM64 desktop, `.deb`, AppImage stay deferred (ADR-0035 §6).

## Versioning (§1d) — the tag stamps the binary; "dev" retires

- **Desktop:** `cmd/korvun-desktop/main.go` already declares `var version = "dev"`
  wired via `shell.WithDesktopVersion(version)` (verified in-tree). The release
  recipe stamps `-ldflags "-X main.version=v<tag>"` — target is **`main.version`**
  because the desktop var lives in package `main` (correct as-is; no retarget
  needed for the desktop binary). The version comes from the git tag
  (`v{{.Version}}` shape, matching the documented `vX.Y.Z`).
- **Headless retarget "parked since Piece 3":** already **RESOLVED** in
  `.goreleaser.yaml` (line 35 targets `internal/cli.version=v{{.Version}}`, done
  at v0.2.0, 2026-07-18 — ROAD-TO-BETA §5). The only residue is a **stale
  comment** in `internal/cli/cli.go:39-41` still claiming ".goreleaser.yaml
  still targets `main.version`" — contradicted by the actual goreleaser file.
  Since SP7 touches the same `-ldflags -X …version` mechanism, the spec folds a
  **one-line comment correction** there (7b or a rider) — a doc-hygiene fix, NOT
  a functional retarget (there is nothing functional left parked).

## Release integration (§1e) — same tag, same GitHub release, draft-until-complete

**One SemVer tag, one GitHub release, two artifact families.** A **new
`release-desktop.yml`** workflow, `on: push: tags`, mirrors the headless trigger
but runs a **native-runner matrix** (macos-latest, windows-latest,
ubuntu-latest). Each job runs `make desktop` for its OS, packages per §1c, cosign-
signs per §1f, and **uploads to the tag's release** with `gh release upload
<tag> <artifacts>` (verified-standard GitHub CLI; the alternative — GoReleaser
`release.extra_files` — is rejected because those artifacts are built on other
runners than GoReleaser's ubuntu job, so a cross-runner handoff is needed
regardless, and `gh release upload` is the simpler seam).

### Draft-until-complete (NC-3 RESOLVED — Chano, 2026-07-25: CLOSE the window)

The partial-published-release window is **closed**: the release is **born a
draft** and goes public **only when BOTH artifact families are up**.

1. **Headless GoReleaser** flips to **`release.draft: true`** in
   `.goreleaser.yaml` — it creates the release as a **draft** (invisible to the
   public) and uploads the headless family into it.
2. **Desktop matrix** builds/packages/signs on the 3 native runners and
   `gh release upload`s the 3 artifacts + their cosign signatures into that same
   draft (the desktop jobs `gh release view <tag>` poll until the draft exists —
   still needed so upload never precedes creation).
3. **Finalizer step** (desktop lane, after all uploads succeed):
   `gh release edit <tag> --draft=false` — the single publish moment, fired only
   once all 3 desktop artifacts **and** their signatures are confirmed present.

**Bounded exception to "headless untouched" (Chano-authorized, 2026-07-25).** The
ONLY headless change is `release.draft: false → true` — a **release-visibility**
flag. The **headless binary and every build step stay byte-for-byte identical**
(the `-X internal/cli.version` ldflags, the ×6 `CGO_ENABLED=0` compile, the
checksums, the SBOM, the cosign signing are all unchanged); `go version -m` on
the headless artifact and the compiled bytes are unaffected. The law's intent —
never alter what the headless USERS receive — is fully preserved; only WHEN the
release becomes visible moves.

**Failure mode (documented, by design):** if the **desktop lane goes red** (any
OS build/package/sign/upload fails, or the finalizer does not run), the release
**stays a draft — invisible to the public** — until the lane is fixed and re-run,
or **Chano manually decides** (`gh release edit --draft=false`, or delete the
draft). A broken desktop build can therefore never expose a half-populated public
release; the worst case is a delayed, still-invisible release. The headless
family sits complete inside the draft the whole time.

## Signing (§1f) — 0€, cosign for everything, same chain as headless

**No Apple Developer Program, no Windows cert (ADR-0035 §7, engraved law).** The
free **cosign keyless chain** (Stage 16, already proving `checksums.txt` for the
headless) signs **all** desktop artifacts: the desktop workflow computes a
`checksums-desktop.txt` over the `.dmg` / installer / `tar.gz` and **cosign
`sign-blob`**s it (keyless OIDC, same Fulcio/Rekor path already GREEN in CI),
producing the `.sig` + `.pem` uploaded alongside. **cosign pin (review #4):**
`release-desktop.yml` pins the cosign binary to **`v2.6.3`**, matching the
headless `release.yml` — cosign v3 defaults to `--new-bundle-format` and drops
the classic `--output-signature`/`--output-certificate` outputs, so an unpinned
`cosign-installer` would silently produce a different artifact shape than the
headless chain. Integrity + tlog transparency covered for desktop exactly as for
headless; **platform identity is the deferred part**. The **Gatekeeper right-click→Open (macOS) and SmartScreen
"run anyway" (Windows)** workarounds are documented in the install guide **with
screenshots in SP8** (when the real artifacts exist — same TODO-VERIFY discipline
as the Discord intent step).

## Desktop CI lane (§1g) — the triage's standing note + an honest smoke

1. **`govulncheck -tags desktop,production ./cmd/korvun-desktop`** — the
   Dependabot-triage **standing note**: the untagged CI run cannot see desktop
   reachability (x/net links only in the desktop binary), so the desktop lane
   must run govulncheck **with the tags**. This is the honest vuln gate for the
   artifact users actually download.
2. **Startup smoke — honest per OS, and honest about where it can't:**
   - **Non-GUI sanity (all 3 OSes):** the packaged binary answers a non-GUI probe
     — the version/identity path (`main.version` now stamped) — proving the
     binary links and boots its non-window code. This is real and cheap on every
     hosted runner.
   - **GUI launch:** launching the actual WebView on a **hosted** runner is
     **not honest** — hosted macos/windows runners give **no reliable headless
     GUI-webview validation** (WebView2/WKWebView rendering in CI is flaky, not
     that a session is strictly absent — review #5), and Linux needs a virtual
     display (`xvfb`) plus GTK/WebKitGTK. Folded position: attempt a **`xvfb-run`
     smoke on Linux only** (where it is honest), and **explicitly do NOT claim** a
     GUI smoke on hosted macOS/Windows — **true end-to-end GUI validation is SP8
     on Chano's hardware.** Saying so is the point (CLAUDE.md: name where it
     can't be done).
   - **Artifact-exists assertions (review #2):** the smoke asserts the expected
     artifact exists per OS (the `.app`/`.dmg`, the NSIS `.exe`, the `tar.gz`) —
     never trusting a green `wails build`, which can exit 0 with no installer.
   - **Linux build deps:** the ubuntu job installs the WebKitGTK/GTK dev packages
     `wails build` needs on Linux (exact package names — e.g.
     `libgtk-3-dev libwebkit2gtk-4.1-dev` — **verified at source in 7b via
     `wails doctor`**, not pinned from memory here).

## Cut plan (§1h)

### 7a — local reproducible packaging (feeds SP8)

**Deliverable:** the recipe + a fabricable universal `.app`/`.dmg` on Chano's
Intel iMac (macOS 13). Contents: `wails.json` (`Info.productName=Korvun`,
`productVersion` from tag, company/copyright); the `build/` scaffold
(`appicon.png` from `assets/brand/korvun-avatar-512.png`, `darwin/Info.plist` +
`entitlements.plist`, `windows/installer/*` NSIS template); the `make desktop`
target (both-frontends-then-binary, §1b); the macOS `.dmg` wrap (`hdiutil`); the
version stamp (§1d). **Verifications:** `make desktop` from a clean tree produces
`build/bin/Korvun.app` that launches and shows the real builder (the re-bake
lesson made a gate); `hdiutil` yields a mountable `.dmg`; the stamped
`--version`/identity reports the tag not "dev"; **headless `go version -m` diff
proves the headless binary is byte-identical** (no shared-minimum drift from any
`go.mod` touch — expected none, but the check is the rule). `make quality` green
`-race` over the whole suite. **Xcode Command Line Tools** are the one host
prerequisite for the universal cross-slice on the Intel iMac (documented; the
universal cross is verified feasible in 7a, not assumed).

### 7b — CI on native runners + release integration

**Deliverable:** `release-desktop.yml` (native-runner matrix, §1e) building +
signing + uploading the 3 artifacts to the tag's **draft** release, then the
**finalizer** (`gh release edit <tag> --draft=false`) once all 3 + signatures are
up; the one-line **`release.draft: true`** flip in `.goreleaser.yaml` (the
Chano-authorized bounded exception, §1e); the `govulncheck
-tags desktop,production` + smoke lane (§1g); the Linux build-deps step **and
the Windows `makensis` install + installer-exists assertion** (§1c review #2);
the cosign `v2.6.3` pin (§1f review #4); the stale-comment correction in
`internal/cli/cli.go` (§1d). **NC-3 resolved (§1e) — 7b is unblocked.** **Verifications:** a
`workflow_dispatch` `--snapshot`/dry-run proves the matrix builds all 3 artifacts
and the cosign keyless signing is GREEN **before any real tag** (mirrors the
Stage-16 Phase-A dry-run discipline); **the headless BINARY stays byte-identical
across the `release.draft` flip** (`go version -m` + a compiled-bytes diff — the
only `.goreleaser.yaml` change is the visibility flag, §1e); pinned Action SHAs
re-verified at source before landing (repo convention); `make quality` green.
**No real tag is pushed** — the tag stays Chano's explicit call (release-outlook
law).

## Success criteria

- The 3 v1 artifacts (§1c) are produced by `make desktop` + the workflow, on
  native runners, cosign-signed on the free chain.
- **Headless binary byte-for-byte unaffected**, proven by a `go version -m` diff
  + a compiled-bytes diff. The ONLY headless-pipeline change is the
  Chano-authorized `release.draft: false → true` visibility flag (§1e); the ×6
  `CGO_ENABLED=0` compile, checksums, SBOM, and cosign signing are untouched.
- `govulncheck -tags desktop,production ./cmd/korvun-desktop` runs in the desktop
  lane and is green (or its findings triaged).
- `make quality` green with `-race` over the WHOLE suite at each cut's close.
- No new `go.mod` dependency (the `wails` CLI is build-time); `go.mod` stays at 5
  direct deps.
- The desktop binary reports the tag-stamped version, not "dev".

## Decisions folded in (review may veto without archaeology)

1. **`wails build` over `go build` for release artifacts** — ADR-0036 already
   bought the packaging capability; hand-rolling it is the rejected back-door.
2. **`.dmg` via `hdiutil` (built-in), not `create-dmg`** — zero external runner
   tool; styling is cosmetic and not v1.
3. **`gh release upload` to the tag's DRAFT release + a `--draft=false`
   finalizer, not GoReleaser `extra_files`** — cross-runner artifacts need a
   handoff regardless; `gh` is the simpler seam. The release is born a draft
   (headless `release.draft: true`) and published only when both families are up
   (§1e, NC-3 resolved) — the sole headless change is the visibility flag, the
   binary stays byte-identical.
4. **`-s` skip-frontend + Korvun's own `go:embed`** — the two-frontend model
   cannot ride wails' single-`frontend/` hook; determinism lives in `make
   desktop`, and `wails.json:frontend:build` is neutered so a bare `wails build`
   can't rebuild only one frontend.
5. **Desktop version target stays `main.version`** (var is in package main) — no
   retarget; the headless retarget is already done, leaving only a stale comment
   to correct.
6. **cosign `sign-blob` over a `checksums-desktop.txt`** — mirrors the headless
   `checksums.txt` chain exactly; one signed manifest covers all 3 artifacts.
7. **Honest smoke:** Linux `xvfb` GUI smoke + a non-GUI identity probe on all 3;
   **no** hosted macOS/Windows GUI-launch claim — real GUI validation is SP8.

## Clarifications — all resolved (nothing blocks TDD/implementation)

ADR-0035/0036 fixed the hard forks (matrix, signing policy, release cadence,
in-process form, WebView2 strategy). The three points once open are now closed —
**7a and 7b are both unblocked**:

1. **App/product name — CONFIRMED (copilot, 2026-07-25):** `Info.productName` =
   **`Korvun`**, bundle id **`com.korvun.desktop`**.
2. **Desktop artifact naming — CONFIRMED (copilot, 2026-07-25):** aligned to the
   headless GoReleaser scheme —
   **`korvun-desktop_<ver>_<os>_<arch>…`**:
   `korvun-desktop_<ver>_darwin_universal.dmg`,
   `korvun-desktop_<ver>_windows_amd64-installer.exe`,
   `korvun-desktop_<ver>_linux_amd64.tar.gz` (§1c updated to match).
3. **The partial-published-release window — RESOLVED (Chano, 2026-07-25): CLOSE
   the window** via **draft-until-complete** (§1e). The release is born a draft
   (`.goreleaser.yaml`: `release.draft: true`) and is published
   (`gh release edit <tag> --draft=false`) only once **both** artifact families +
   the desktop signatures are up. This is a **Chano-authorized, bounded exception
   to "headless untouched"**: the only headless change is the release-visibility
   flag; the headless **binary and every build step stay byte-identical**.
   Failure mode documented (§1e): a red desktop lane leaves the release an
   invisible draft until fixed or Chano's manual call — a broken build can never
   expose a half-populated public release.
