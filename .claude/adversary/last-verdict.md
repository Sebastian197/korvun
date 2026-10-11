VETO LEVANTADO d32749fa14d06dc2ed2826d47ac7a1f2719a4768
Round: the go1.26.9 toolchain train (2026-10-11), critical path.
Legacy marker by the director's order: the versioned procedure of
docs/INTEGRATION.md cannot be followed without force for a new train
(P2 of the procedure, adjudicated 2026-10-10; the checker cure goes in
its own PR). After rebase this marker names a rewritten SHA; the
director repairs master's marker by hand, as after #69 and #74.

WHAT THE COMMITS ARE.
- fix(deps) 746f795: go.mod's go directive 1.26.6 -> 1.26.9, the single
  Go pin (the six setup-go steps read go-version-file: go.mod). Cures
  the ten reachable stdlib advisories govulncheck v1.7.0 reports on
  go1.26.6: GO-2026-6603, -6605, -6607..-6613, -6617 (net/http,
  net/textproto, crypto/tls), fixed in go1.26.9 (released 2026-10-08,
  a Go-team security release, verified at go.dev/dl).
- docs(release): one bullet in docs/releases/v0.16.2.md under
  "Corregido antes de publicar." naming the advisories and go1.26.9.

EVIDENCE (local). govulncheck v1.7.0 over the CI pruned list (43
packages): go1.26.6 exit 3 with the ten; go1.26.9 exit 0, "No
vulnerabilities found". go vet GOOS=linux/windows/darwin exit 0. make
dmg (wails v2.16.0) exit 0; both slices of the universal binary report
go1.26.9. make quality green in the pre-commit gate of both commits
(the hook prints nothing on success).

THE ADVERSARY'S SHORT PASS: VETO LEVANTADO, zero P1, zero P2. It
reproduced the ten on go1.26.6 (darwin, linux, windows) and zero on
go1.26.9, vet x3, go mod tidy unchanged, the dmg slices, -race tests of
the HTTP packages, and that the stdlib sources the code comments cite
as "Go 1.26.6" did not change the cited mechanisms in 1.26.9.
P3s, declared, for the director: (1) the gate logs carry no transcript
and the vet/govulncheck captures no header (the adversary reproduced
the substance); (2) QUICKSTART.md:218 "Go 1.26.4+" and the website
quickstart "Go 1.26.5+" (en/es) were stale before this train and stay
so; (3) codeql.yml autobuilds Go without setup-go, no version pinned,
not verified which Go it uses; (4) the user-level GOTOOLCHAIN=go1.26.6
outside the repo makes every go command fail loudly on this tree until
changed.
Not verified: make quality's own transcript; CI; desktop builds for
Windows and Linux.
