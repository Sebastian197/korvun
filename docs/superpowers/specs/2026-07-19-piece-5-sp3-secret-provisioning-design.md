# Piece 5 SP3 — Secret provisioning via OS keychain: Design Spec

> **Status:** approved for TDD (SP3). Governing ADRs: ADR-0035 §4 (the secret
> contract: keychain → own-process env before `app.Build`; env-only ADR-0010
> §3 intact; storage contract service `korvun` / account = env-var name; no
> file fallback) and ADR-0037 (the `zalando/go-keyring` dependency, verified
> 2026-07-19 with Context7 + source). Inherited law: the `internal/shell`
> doc.go contract (no Wails import); the SP2 bearer is NOT touched — this
> spec covers channel/provider secrets only. UI (SP6) and the first-run
> template (SP5) stay out.

## Goal

The desktop shell can provision channel and provider secrets (bot tokens,
API keys) from the OS keychain into its own process environment at Start,
before the core builds — so a double-clicked app reaches a working core with
no terminal — while a variable already present in the environment ALWAYS
wins and the keychain is never consulted for it. Afterwards `internal/shell`
exposes the storage seam the future UI (SP6) will drive (Set/Delete without
orphans). Nothing changes for terminal users: env keeps absolute precedence.

## Functional requirements

- **FR-SEC-1 `SecretStore` seam (`internal/shell`)** — `Get(name string)
  (string, error)` (sentinel `ErrSecretNotFound`), `Set(name, value string)
  error`, `Delete(name string) error`. The seam is what the Controller and
  the future UI consume; the real backend lives behind it. Delete REMOVES
  the entry (no orphans, never an emptied value).
- **FR-SEC-2 `SecretEnvNames(cfg *config.Config) []string`** — the
  deduplicated, sorted enumeration of every secret env-var NAME the config
  references: `channels[].token_env` + `brains[].models[].api_key_env`
  (non-empty). `admin.token_env` is EXCLUDED (the SP2 per-cycle bearer).
- **FR-SEC-3 Provisioning in `Start`, before `app.Build`** — with a store
  injected (`WithSecretStore`): for each enumerated name, if the variable is
  ALREADY in the environment (present AND non-empty — a set-but-empty var
  counts as absent, matching the core's `ErrMissingSecret` semantics) the
  env wins and the store is NOT consulted for it (sacred precedence,
  ADR-0035 §4); if absent, `Get` from the store and `os.Setenv` on hit; on
  `ErrSecretNotFound` skip silently — the boot then fails with today's
  `ErrMissingSecret` naming the variable; on any OTHER store error, `Start`
  fails wrapped naming the VARIABLE (never a value). A config that reuses
  `admin.token_env` as a channel/model secret variable is REJECTED loudly
  (the bearer's per-cycle Setenv would silently clobber it). With no store
  injected (nil, the default), provisioning is skipped entirely — SP2
  behavior byte-for-byte.
- **FR-SEC-4 Cycle hygiene** — the shell unsets ONLY the variables it
  provisioned, on every cycle end (clean Stop, failed boot, cancelled wait,
  reaped core) — symmetric with the SP2 bearer. Pre-existing env variables
  are never unset. Reloads within a cycle reuse the provisioned env (no
  re-provisioning mid-cycle).
- **FR-SEC-5 Controller passthrough for the future UI** —
  `SetSecret(name, value string) error` / `DeleteSecret(name string) error`
  delegate to the store (error if none injected); effective on the NEXT
  cycle (documented — provisioning happens at Start).
- **FR-SEC-6 Backend package `internal/shell/keyring`** — implements
  `shell.SecretStore` over `zalando/go-keyring` with service `korvun`,
  account = env-var name; maps the library's `ErrNotFound` to
  `shell.ErrSecretNotFound`. The ONLY importer of the library (ADR-0037
  seam boundary).
- **FR-SEC-7 No secret values in logs or errors** — every log line and
  error on these paths carries variable NAMES only; test-asserted.

## Acceptance scenarios (Given / When / Then)

- **AS-1 (precedence)** Given `X` set in the env AND a store holding a
  DIFFERENT value for `X` behind a spy, When Start, Then the spy shows `X`
  was never requested and `os.Getenv(X)` still returns the env value.
- **AS-2 (provisioning)** Given `X` absent from the env and present in the
  store, When Start (fake channel factory), Then `os.Getenv(X)` returns the
  store value while running, and after Stop the variable is UNSET.
- **AS-3 (absent both, honest boot failure)** Given a discord channel config
  whose token env is unset and a store without it, When Start with the REAL
  channel factory (discord's factory pre-checks env presence with no
  network), Then Start fails wrapping `ErrMissingSecret` naming the
  variable, and no secret value appears in the error chain.
- **AS-4 (store failure)** Given a store whose Get returns a non-not-found
  error, When Start, Then Start fails, wrapped, naming the variable only.
- **AS-5 (seam roundtrip)** Set → Get returns the value; Delete → Get
  returns `ErrSecretNotFound` (fake store in shell tests; `MockInit`-backed
  real backend in `internal/shell/keyring` tests).
- **AS-6 (no leakage)** Given a capturing slog handler through a provisioned
  cycle and a failed boot, Then no captured log line and no returned error
  string contains any secret value.
- **AS-7 (hygiene boundary)** Given one variable provisioned by the shell
  and another pre-existing in the env, When Stop, Then the provisioned one
  is unset and the pre-existing one survives.

## Success criteria

- Coverage ≥ 85% on the new surface (`internal/shell` stays ≥ 85% overall;
  `internal/shell/keyring` ≥ 85%).
- `make quality` green with `-race` over the WHOLE suite on the default
  (untagged) lane — the 3-OS CI gate NEVER touches a real keyring: shell
  tests use the fake; backend tests use `keyring.MockInit()`.
- Real-keyring tests exist but are opt-in via the `keyring_live` BUILD TAG
  in their own file (run isolated: `go test -tags keyring_live -run
  TestStore_liveKeychain ./internal/shell/keyring`), with a mocked-guard
  that fails loudly instead of passing vacuously against a sibling test's
  residual in-memory mock (go-keyring cannot restore the real provider) —
  CI runners have no Secret Service (ubuntu) nor a guaranteed unlocked
  keychain session.
- Headless `cmd/korvun` untouched; the `go version -m` diff gate runs at
  the `go get` commit (ADR-0037 Honest gap).

## Decisions folded in

Provisioned secrets are unset on every cycle end (bearer symmetry — the
shell removes only what it wrote); a nil store skips provisioning (SP2
compatibility, desktop main always injects the real backend);
`SetSecret`/`DeleteSecret` are thin delegations effective next cycle;
enumeration order is sorted for determinism.

## `[NEEDS CLARIFICATION]`

None arose — every point above resolves inside ADR-0035 §4 / ADR-0037.
