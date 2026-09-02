# ADR-0045: Exclusive profile lock via golang.org/x/sys

Date: 2026-09-02
Status: Accepted

## Context

The third external audit's P1: `korvun receipt rotate-key` beside a
LIVE server retired the signing key under the server's feet — the
server kept sealing with the retired key, so every later receipt was
born `key_window_violated` until a restart. The cure needs mutual
exclusion between the server process and the rotation act that (a)
is released automatically when either process DIES (no stale-lockfile
recovery logic), and (b) works on Linux, macOS and Windows — Korvun's
one-binary promise.

## Decision

An advisory OS file lock on `<profile>/korvun.lock`:

- unix (linux/darwin): `unix.Flock(fd, LOCK_EX|LOCK_NB)` — exclusive,
  non-blocking; the kernel releases it when the process exits.
- windows: `windows.LockFileEx(handle, LOCKFILE_EXCLUSIVE_LOCK|
  LOCKFILE_FAIL_IMMEDIATELY, ...)` — same semantics; the OS releases
  the region lock when the handle closes (process death included).

Both come from `golang.org/x/sys` (v0.47.0 — already an indirect
dependency, promoted to direct; signatures verified at source on
pkg.go.dev 2026-09-02, Context7's coverage of this repo being thin).
The stdlib alternative (`syscall.Flock`) exists only for unix;
Windows would need the same x/sys call anyway. A PID lockfile was
rejected: it survives crashes and needs liveness heuristics — exactly
the fragility the audit punishes.

The server acquires the lock for its whole life (storage-configured
boots; released on Shutdown and by the OS on death). `rotate-key`
acquires it only around the rotation; a held lock refuses with the
stable rule `signing_key_in_use` and ZERO mutations. House amendment:
no other operator act takes the lock — approve/reject/execute keep
working beside a live server, pinned by test.

## Consequences

- A second server on the same profile now refuses at boot (named) —
  the "one live session" law gains an OS-level enforcement for free.
- x/sys becomes a direct dependency (tiny, official, already in the
  build graph).
- The lock file is advisory: processes that do not ask cannot be
  stopped by it — every Korvun door that must respect it asks.
