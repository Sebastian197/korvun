VETO LEVANTADO 903562a67fc21218336292cfceb90fbf994f0753

Round: the go1.26.10 toolchain train (2026-10-24), a minimal cure for
GO-2026-6809 on the critical path, in the shape of #79.
Legacy marker, as for #81: the versioned procedure of
docs/INTEGRATION.md cannot be followed without force for a new train
(P2 adjudicated 2026-10-10). After rebase this marker names a rewritten
SHA; the director repairs master's marker by hand.

WHAT THE COMMIT IS. go.mod's go directive moves from 1.26.9 to 1.26.10;
go mod tidy leaves go.mod and go.sum unchanged. The v0.16.2 notes'
"fixed before publishing" bullet names go1.26.10 and GO-2026-6809
(CVE-2026-111943, crypto/x509 Certificate.Verify). The three "build
from source" minimums (docs/QUICKSTART.md "Go 1.26.4+", the website
quickstart's "Go 1.26.5+" in English and Spanish) now say Go 1.26.10+.
The records of past runs that name go1.26.9 (docs/HANDOFF.md, the
comment in register_guard_race_test.go) stay as they are.

EVIDENCE.
- govulncheck v1.7.0 over the pruned package list (43 packages) with
  GOTOOLCHAIN=go1.26.9: exit status 3, one finding, GO-2026-6809,
  "Found in: crypto/x509@go1.26.9 / Fixed in: crypto/x509@go1.26.10",
  trace webhook.Start calls http.Server.Serve, which eventually calls
  x509.Certificate.Verify (internal/channel/webhook/lifecycle.go).
- The same scan with GOTOOLCHAIN=go1.26.10 (go version go1.26.10
  darwin/amd64): "No vulnerabilities found.", exit 0; over ./... the
  same. The red-before/green-after pair is the bump's probing evidence;
  a toolchain bump admits no other mold.
- go1.26.10 and the advisory verified at go.dev/dl and vuln.go.dev.
- make quality green with go1.26.10 (62 min), and again in the
  pre-commit gate over the code commit's tree; its message was later
  amended with the tree unchanged.
- Locally GOTOOLCHAIN=auto resolves to the installed go1.27.1, so every
  local run pinned GOTOOLCHAIN=go1.26.10 per process. CI reads
  go-version-file: go.mod in every setup-go step; that the three OS
  jobs print go1.26.10 is unverified until the branch is pushed.

THE ADVERSARY. One pass on the diff, read only: VETO MANTENIDO with one
P2 and two P3. P2: the three quickstart minimums were known false
public claims (already false against go 1.26.9); cured in this commit
by setting them to Go 1.26.10+. P3: no test goes red on an ineffective
bump; declared, the govulncheck pair above is the evidence. P3:
CONTRIBUTING.md "Run govulncheck ./... locally with the pinned
toolchain" contradicts the pruned-list law and is not a pin locally;
pre-existing, filed. No re-pass was run after the P2 cure: this
VETO LEVANTADO rests on the author's fold of a single P2, declared
as such for the director.
