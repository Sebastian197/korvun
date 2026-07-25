# Korvun Desktop — Wails build scaffold (SP7)

This directory is the committed Wails packaging scaffold for `korvun-desktop`
(ADR-0035 / ADR-0036; the SP7 design spec is
`docs/superpowers/specs/2026-07-25-piece-5-sp7-packaging-desktop-ci-design.md`).
`wails build` runs in this package's directory (`cmd/korvun-desktop/`) and reads
this `build/` tree; the compiled `.app` / installer / binary land in `build/bin/`
(gitignored — the scaffold is versioned, the output is not).

## Contents

- **`appicon.png`** — the single icon source of truth (Korvun's brand avatar,
  `assets/brand/korvun-avatar-512.png`). Wails derives the macOS `iconfile.icns`
  and, when absent, the Windows `windows/icon.ico` from it at build time — so
  neither is committed.
- **`darwin/Info.plist`** — the macOS bundle Info.plist template (Wails renders
  the `{{.Info.*}}` fields from `wails.json`). `CFBundleIdentifier` is pinned to
  **`com.korvun.desktop`** (NC-1); everything else is the canonical Wails
  template.
- **`darwin/Info.dev.plist`** — the `wails dev` variant (dev inner loop only).
- **`darwin/entitlements.plist`** — a placeholder for a FUTURE hardened-runtime
  codesign/notarization; NOT consumed by the unsigned v1 recipe (see the file).
- **`windows/`** — the Windows resource scaffold (`info.json`,
  `wails.exe.manifest`) and `installer/` NSIS templates (`project.nsi`,
  `wails_tools.nsh`). Committed for the 7b Windows lane; not exercised by the
  macOS-local 7a recipe.

## How it is built

Never by hand and never trusting a cached frontend — always via the
deterministic recipe:

```
make desktop   # both frontends fresh, then wails build -s -skipbindings …
make dmg       # wrap the universal .app into a .dmg (macOS)
```

`wails.json`'s `frontend:build`/`frontend:install` are **neutralized** (echo
no-ops) so a stray `wails build` without `-s` can never rebuild only one of
Korvun's two frontends; the two-frontend determinism lives in `make desktop`.
