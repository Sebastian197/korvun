# AGENTS.md — Korvun

Operating guide for autonomous coding agents (Codex CLI, and any other agent
that reads this file) working in this repository.

> **Read this before your first command.**
> On this machine Codex runs with `approval_policy = "never"` and
> `sandbox_mode = "danger-full-access"`. No sandbox will stop a write, and no
> prompt will ask before a command runs. This file is the guardrail. Treat
> every "MUST NOT" in this file as if it were enforced, because nothing else enforces it.

---

## 0. The governing document is CLAUDE.md

`CLAUDE.md` at the repository root is the project's law: engineering mission,
the Stage → Phase → Task workflow, the destructive-testing doctrine, the
adversarial-review requirement, scope control, and the delivery shape. This
file does not replace it and does not summarise it. **Read `CLAUDE.md` in full
at the start of every session and follow it strictly.**

What this file adds: the verified stack, the verified commands, and the
explicit exclusion list.

Conversation with the user is in **Spanish**. Everything written into the repo
— code, identifiers, comments, commit messages, docs — is in **English**.

---

## 1. Stack (verified 2026-10-10)

| Layer | What | Verified how |
|---|---|---|
| Language | Go 1.26.6 (`darwin/amd64` locally) | `go version`; `go.mod` declares `go 1.26.6` |
| Module | `github.com/Sebastian197/korvun` | `go.mod` line 1 |
| Shape | Modular **single Go binary** (`cmd/korvun`) | `Makefile` `build` target |
| Packages | 43 Go packages, `node_modules`-free | `make guard-gopkgs` |
| Lint | `golangci-lint` v1.64.8 + `gofmt` + `goimports` | `$(go env GOPATH)/bin/golangci-lint --version` |
| Vuln scan | `govulncheck` (built with go1.26.6) | `$(go env GOPATH)/bin/govulncheck` |
| Builder UI | React + TypeScript + Vite + `@xyflow/react` (React Flow), `web/builder/` | `web/builder/package.json` |
| Desktop | Wails; own frontend at `cmd/korvun-desktop/frontend/` | `Makefile` desktop targets; `wails` present in `$(go env GOPATH)/bin` |
| Website | **Docusaurus 3.9.2**, `website/` | `website/package.json` |
| Node | Node v22.23.2 / npm 12.0.2 | `node --version`, `npm --version` |
| ADRs | 47 records in `docs/adr/` (plus `TEMPLATE.md`) | `ls docs/adr/0*.md \| wc -l` |

> Note: the `Makefile` `website-check` recipe calls the website build
> "vitepress", and the build script writes to `.vitepress/dist`. Both are
> leftovers from a previous generator. The site is Docusaurus. Do not repeat
> the "vitepress" wording in new docs.

---

## 2. Commands

The Go pipeline is driven entirely by `make`. The `Makefile` discovers packages
with `find` + `go list` (never `./...`) because a single stray `.go` file under
`node_modules` empties `go list ./...` and makes every tool pass over nothing.
**Never replace the package discovery with `./...`.**

### Verified green on 2026-09-12

| Command | Does | Observed |
|---|---|---|
| `make guard-gopkgs` | Asserts the package list is non-empty and `node_modules`-free | **PASS** — "41 packages, node_modules-free" |
| `make fmt` | `gofmt -l` + `goimports -l` must be empty | **PASS** |

### Not executed in this session — run them yourself before relying on them

| Command | Does |
|---|---|
| `make build` | Builds `web/builder` first, then `go build ./cmd/korvun` |
| `make test` | `go test -race` over the pruned package list |
| `make lint` | `fmt` + `vet` + `golangci-lint run` |
| `make cover` | `go test -race -coverprofile` over `./internal/...`; **fails under 85%** |
| `make fuzz-smoke` | 8 fuzz targets in `internal/action`, 25000x each |
| `make hook-probe` | `scripts/adversary-gate-probe.sh` |
| `make quality` | `guard-gopkgs lint test cover fuzz-smoke hook-probe integration-probe wails-pin-probe mutation-tally-probe desktop-frontend-check` — **the gate that must be green before any phase closes** |
| `make desktop-frontend-check` | typecheck + lint + format:check + coverage for the desktop chrome |
| `make website-check` | 9-step website gate; the steps are named in the `website-check` recipe |

Coverage scope is deliberately `./internal/...` only; `cmd/` is excluded from
the threshold. Do not widen or narrow it casually.

---

## 3. Files and paths you MUST NOT touch

This is the section that matters most under no-approvals mode. If your task
requires changing anything here, **stop and ask the user first**.

### Never write, never delete

- `.git/` — the repository itself.
- `.githooks/pre-push` and `.claude/hooks/` (`adversary-gate.sh`,
  `block-attribution.sh`, `quality-gate.sh`) — these are the project's own
  gates. Weakening, bypassing or removing a gate is out of bounds. Do not pass
  flags or configuration that skip git hooks.
- `.github/workflows/` — CI. Every step is pinned; changing a pin without the
  verification the project requires breaks the supply-chain rule.
- `docs/adr/` — the ADRs are a historical record. New decisions get a
  **new** ADR; accepted ones are not rewritten.
- `LICENSE`, `NOTICE`, `SECURITY.md`.
- `CLAUDE.md` and this `AGENTS.md` — edits must be proposed as an explicit diff
  and approved, never applied silently.

### Secrets — never read into output, never commit, never edit

- `korvun.local.json`, `configs/*.local.json`, any `*.local.json`, `.env` —
  gitignored local overrides that may hold credentials.
- `~/.codex/config.toml` — **contains plaintext API tokens.** Do not print it,
  do not copy it into the repo, do not paste it into a commit, an issue, or a
  message.
- Secrets belong in the environment or a secret manager. Never in the tree.

### Generated — do not hand-edit; regenerate instead

- `graphify-out/` (`graph.json`, `GRAPH_REPORT.md`, `graph.html`) — rebuilt by
  the post-commit hook; refresh docs changes with `graphify --update`.
- `coverage.out` and any `*.out`.
- `web/builder/dist/` and `cmd/korvun-desktop/frontend/dist/` — committed
  placeholders consumed by `//go:embed`. The Go pipeline depends on them
  existing; do not clear them to "clean up".
- `korvun` (compiled binary at the root).

### Dependencies — no opportunistic upgrades

- `go.mod` / `go.sum` — a new dependency requires a justifying ADR **and**
  documentation verification first. Never bump a version because a newer one
  exists.
- `web/builder/package-lock.json`, `website/package-lock.json`,
  `cmd/korvun-desktop/frontend/package-lock.json` — if a lockfile genuinely must
  be regenerated, use `npm install --include=optional` and then confirm
  `npm ci` runs clean **twice in a row**. A plain `npm install` on macOS silently
  drops the optional-of-optional platform WASM subtree, which passes locally and
  breaks Linux CI.

### Somebody else's work in progress — leave it alone

- The narrative session files at the root (`Korvun — 2026-*.md`),
  `claude-code-report.md`, `BRIDGE-STATUS-*.md`, `design-drafts/`,
  `website-redesign-evidence/`.
- Any untracked file you did not create. Untracked does not mean disposable.

### Outside the repository

Full-access mode means you *can* write anywhere on this machine. Don't.
Confine writes to this repository and to a temporary directory. Never touch
`~/.ssh`, `~/.aws`, `~/.config`, keychains, browser profiles, or any other
project directory.

---

## 4. Git

- The branch is `master`; the remote is `https://github.com/Sebastian197/korvun.git`.
- **`master` is protected. Never push to it directly.** Work travels on its own
  branch, rehearses green, and reaches `master` only through a pull request the
  user merges. The merge is theirs, not yours.
- **Merges are never squashed** — pull requests are integrated by GitHub
  rebase only, under the procedure in `docs/INTEGRATION.md`; squash and merge
  commits are disabled in the repository settings.
- **Do not commit or push unless the user explicitly asks.**
- Commit messages: Conventional Commits, SemVer. They **must not** contain any
  AI attribution — no co-author trailer, no "Generated with/by" line, no mention
  of the assistant or its vendor. A repository hook enforces this.
- Never run a git command that skips a hook.

---

## 5. Scope

Do the assigned task and only the assigned task. When you find an unrelated
problem: record the evidence, decide whether it blocks you, report it, and move
on. No drive-by refactors, no opportunistic upgrades, no reformatting files the
task did not touch. A smaller correct diff beats a broad cleanup.

Do not describe work as fixed, safe, race-free, secure, or production-ready
without evidence of that strength. `IMPLEMENTED` is not `VERIFIED`, and
`VERIFIED` is not `ACCEPTED`.

---

## 6. One residual guard still exists

`approval_policy = "never"` and `sandbox_mode = "danger-full-access"` do **not**
disable Codex's built-in exec policy, which is compiled into the binary
(`core/src/exec_policy.rs`). It rejects some command shapes outright, before
the sandbox layer is consulted. Observed on 2026-09-12:

```
rm -f style commands are not permitted. Use a safer approach
```

If a command is refused with wording like that, it is this policy — not a
sandbox denial and not a pending approval. Reach for a safer formulation rather
than trying to defeat it.
