# ADR-0037: Dependency — `zalando/go-keyring` (OS keychain access)

> **Status:** accepted
> **Date:** 2026-07-19
> **Deciders:** Sebastián Moreno Saavedra
>
> **Accepted 2026-07-25, copilot on-disk review of SP3 passed.**
>
> The dependency decision ADR-0035 §4 left open ("the keychain access library
> is a dependency decision with its own four-axis gate in the TDD sub-phase").
> Companion spec: the SP3 design spec. `go.mod` is NOT touched by this ADR —
> the dependency lands with SP3's implementation commit (ADR-0034 precedent).

## Context

ADR-0035 §4 fixes the desktop secret contract: channel/provider secrets live
in the OS keychain (service `korvun`, account = env-var name, value = the
secret), read by the shell at Start and injected into the shell's own process
environment before `app.Build` — env-only (ADR-0010 §3) intact. The mandated
backends are **Keychain Services (macOS), Credential Manager (Windows),
Secret Service D-Bus (Linux)**, and **no file-based fallback** — a Linux
desktop without a Secret Service provider gets an honest error, never a
silently weaker store.

This is the **6th direct dependency**. A structural constraint shapes the
gate: `internal/shell` compiles in the DEFAULT suite (no build tag), so the
library enters `go list ./...`, the 3-OS quality gate, lint, and govulncheck
— unlike wails, it cannot hide in a tagged lane. Anything with cgo or a
heavy tree would poison the default gate. The headless binary must still
never link it (it does not import `internal/shell`), enforced by the same
`go version -m` diff discipline as SP1/SP3-rider.

Everything below was verified 2026-07-19 against Context7
(`/zalando/go-keyring`, reputation High, benchmark 89.8) and primary sources
(the repo's `go.mod`, `keyring_darwin.go`, releases; the module proxy for the
version list) — not from memory.

## Decision

Adopt **`github.com/zalando/go-keyring`**, pinned at **`v0.2.8`** (latest per
the module proxy), used ONLY by the SP3 keychain backend package behind the
shell's `SecretStore` seam.

**Verified facts the decision rests on:**

- **API:** `Set(service, user, secret) error` / `Get(service, user) (string,
  error)` / `Delete(service, user) error`, with the **`ErrNotFound`**
  sentinel — maps 1:1 onto the ADR-0035 §4 storage contract and the seam's
  needs (delete removes the entry, satisfying the no-orphans rule).
- **Backends: exactly the three mandated, and NO file fallback.** macOS via
  the Apple-shipped `/usr/bin/security` binary; Linux via Secret Service
  D-Bus (`godbus/dbus/v5`); Windows via Credential Manager
  (`danieljoos/wincred`). A missing Secret Service surfaces as an error —
  the honest-failure behavior ADR-0035 §4 wants comes free.
- **Secret hygiene on the macOS exec path (checked in source,
  `keyring_darwin.go`):** the secret is piped to `security -i` via **stdin**
  (base64-wrapped, shell-escaped), **never argv** — no `ps` exposure window.
  This was the make-or-break check on the "execs a system binary" concern.
- **No cgo; tiny tree:** the tagged v0.2.8 `go.mod` declares `go 1.18` and
  exactly **two direct dependencies** (`danieljoos/wincred v1.2.3`,
  `godbus/dbus/v5 v5.2.2`), both pure Go. The default 3-OS gate keeps
  compiling everywhere without system headers. The darwin shell-escape
  helper is **internal to the library** (`internal/shellescape`) — no extra
  module enters the graph.
- **Maintained:** active releases through March 2026 (v0.2.8), Zalando org.
- **Test story:** `keyring.MockInit()` swaps an in-memory provider — the
  backend package's own tests run hermetically; the 3-OS gate never touches
  a real keyring (CI ubuntu has no Secret Service session).

### Four-axis dependency test (capability vs hand-roll cost vs maintenance vs risk/volatility)

| Axis | Verdict |
|------|---------|
| **Capability gain** | Three per-OS credential-store integrations behind one 3-function API with a proper not-found sentinel and no file fallback — precisely the ADR-0035 §4 contract, including the stdin-not-argv discipline on macOS that a naive integration would get wrong. |
| **Hand-roll cost** | **Moderate but misplaced.** Reimplementing means: the `security` exec protocol with correct escaping and stdin piping (macOS), a Secret Service D-Bus client (Linux — the dbus dependency returns anyway), and wincred syscalls (Windows) — a re-derivation of exactly this library, per-OS security-sensitive plumbing that is not Korvun's value. |
| **Maintenance / cross-compile** | **Strong.** Pure Go, no cgo, two-dep tree: the default suite and the ×6 `CGO_ENABLED=0` cross-compile stay untouched even though the library rides the UNTAGGED lane. Actively maintained. The headless binary never links it (does not import `internal/shell`) — verified by the `go version -m` gate at the `go get` commit. |
| **Risk / volatility** | Low. Small, stable API (v0.2.x line), reputable org, high adoption. Seam-bounded: only the SP3 backend package imports it behind the `SecretStore` interface, so swapping it is local. The macOS exec-of-`security` surface is Apple's own tooling at a fixed path with stdin-only secret transfer. |

**Net:** the gate passes cleanly — the mandated backend set with no file
fallback, no cgo in the untagged lane, secret-hygiene verified at source,
and blast radius bounded by the seam. Hand-roll would re-derive this exact
library.

**Honest gap (ADR-0034 style), half CLOSED at implementation:** Context7
verified the API surface, macOS mechanism, and mock; the exact dependency
set of the pinned tag was then CONFIRMED at `go get` time (v0.2.8: wincred
v1.2.3 + dbus v5.2.2, `x/sys` indirect; the shell-escape helper is internal
to the library; `stretchr/objx` enters `go.sum` only via wincred's test
deps, never the build), and the headless `go version -m` diff came back
IDENTICAL. The remaining gap: the Linux path's behavior against a MISSING
Secret Service daemon is asserted by the backend's error mapping, not by a
live CI daemon.

## Consequences

- `go.mod` goes from 5 to 6 direct dependencies; the tidy graph gains
  go-keyring plus its two pure-Go backends. All of it enters the default
  gate's `go list` — acceptable because the tree is tiny and cgo-free (the
  deliberate contrast with wails' tagged lane).
- The desktop's secret provisioning becomes testable end-to-end with fakes:
  the shell seam uses its own test double; the backend package uses
  `MockInit`; real-keyring tests ride an opt-in tag/skip (SP3 spec).
- Reversible: the backend package is the only importer; replacing the
  library (or hand-rolling one OS) never touches the seam's consumers.

## Alternatives Considered

- **`99designs/keyring`** — rejected on two axes: it ships a **FileBackend**
  (the ADR-0035 §4 forbidden fallback; excludable via `AllowedBackends` but
  a standing foot-gun) and its maintenance is stale (no merged activity in
  ~3 years; forks exist for aws-vault). Broader backend matrix (pass,
  KWallet, keyctl) is surface Korvun does not want.
- **`keybase/go-keychain`** — rejected: macOS/iOS-focused via cgo against
  Security.framework (cgo in the untagged lane poisons the 3-OS gate), and
  Windows is not covered — it would need combining with two more libraries.
- **Hand-roll per OS** — rejected on the four-axis table: re-deriving the
  exec/dbus/wincred plumbing this library already does, security-sensitive
  and commodity. Retained as the documented fallback if go-keyring ever
  goes unmaintained (the seam makes that swap local).
