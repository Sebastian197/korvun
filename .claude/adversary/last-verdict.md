VETO LEVANTADO 016172330ad32c307c047c021d5aa05a7be24c8d
Round: the registerGuard race train (2026-10-12), product P2 cured in
its own PR before the v0.16.2 tag.
Legacy marker by the director's order: the versioned procedure of
docs/INTEGRATION.md cannot be followed without force for a new train
(P2 of the procedure, adjudicated 2026-10-10; the checker cure goes in
its own PR). After rebase this marker names a rewritten SHA; the
director repairs master's marker by hand, as after #69 and #74.

WHAT THE COMMIT IS. fix(sqlite): the connection guard's hook is
registered in internal/action/sqlite's init() instead of at the first
ledger open (registerGuard's sync.Once). modernc.org/sqlite v1.59.0
requires registration before the first Open and Driver.Open reads the
hook list without the mutex (driver.go:18, :57-58, :266, :294). The
hook returns nil on a DSN without the _korvun_guard nonce. Plus the
mould TestRegisterGuard_firstLedgerOpenAgainstConcurrentOpens and the
HANDOFF section: the tag waits for this PR and a green master CI.

EVIDENCE (mould v7, sha256 00a4fbac..., compiled test binary in a
separate OS process, under -race). Full -count=20 runs on v6, whose
ledger rows and machinery v7 keeps (v7 only splits the probe row's
failure message and corrects one comment):
- red, pre-cure ledger_identity.go: test 20/20 FAIL; for 18/20,
  operator 19/20 FAIL; 38 reports naming registerGuard.func1 and
  Driver.Open; readonly and probe 20/20 PASS.
- green, cured: all four rows 20/20 PASS (589 s).
- probe row under 4 busy loops on 4 cores: 100/100 PASS (v6 and v7).
- mutations, all red: wait neutered 12/12 rows; readonly sentinel;
  empty mutex orders every opener 5/5; v3's fmt.Sprintf + os.WriteFile
  witness 10/10; one opener ordered 5/5 (v7); a planted foreign race
  3/3 "a race the probe did not plant" (v7).
- v7 delta: red 5/5, green 5/5, probe under load 100/100.
- internal/brain -race -count=20 green on the cure (production code
  changed since only in a godoc).
make quality green in the pre-commit gate (the hook prints nothing on
success).

THE ADVERSARY: four passes. Pass 1 VETO (P2: the oracle printed a
constant and claimed a schema over size>0) -> witnesses counted.
Pass 2 VETO (P2: fmt.Sprintf's sync.Pool and os.WriteFile ordered
openers) -> os.Mkdir, strconv, probe row. Pass 3 VETO (P2: probe row
false reds under load) -> openers park until the read, full history,
strict count. Pass 4 VETO LEVANTADO, zero P1/P2. P3s: (A) "the stack
of an old write" in a ledger door is an old read -> folded; (B) a
ledger door's per-child detection is probabilistic (13/40 at default
history), the door runs four children, filed with its capture; (C) the
probe's message did not tell a foreign race from an ordered opener ->
folded, split; (D) the park, not history_size, carries the probe ->
declared.
Not verified: Linux and Windows runs (CI is the evidence there); the
tsan internals behind lost reports (inferred from behaviour).
Filed, not cured: e2_trap (ledger_e2_harness_test.go) and e4_trap
(internal/app/ledger_e4_harness_test.go) register scalar functions
lazily, the same class, tests only.

SECOND ROUND, after the PR's first CI run (director's adjudication,
2026-10-12): quality (windows-latest), job 114877740941, go1.26.9
windows/amd64, failed on the probe row only: the probe child reported
no race (RACECHILD door=probe opened_ahead=8, child PASS). The probe row
is the detector's positive control, so it now skips on Windows with the
reason in the test. No cause is established and the candidates named
(the wait orders the openers there; the opener's work after its write
costs the detector the write's history; the Windows detector does not
report the pattern) are not known to be the only ones.
WINDOWS: green doors, detector not certified. The for/operator/readonly
rows keep running there; their green is not evidence.
LINUX: quality (ubuntu-latest) green at 82a736f, internal/action/sqlite
ok in both the test and the coverage jobs; no skip existed there, so the
probe row ran (inferred from the package's ok, the log is not verbose).
MACOS, after the skip: go vet with GOOS=windows ok; the mould -count=3
all rows PASS, probe not skipped; the probe child run directly three
times: exit 1, 8 reports, 8 on readProbe each; mutation of the skip's
condition to darwin: probe SKIP on macOS; after the comment cures, one
more -count=1 all rows PASS. Freeze sha256 6bc79d3d....
Adversary, short pass over the delta: VETO (P2: the HANDOFF recipe
concluded "es la espera" from an outcome its experiment does not
isolate) -> cured; P3s folded (handshake fixed as a spin on the variable
itself, with its 1/2/0 reports measured by the adversary on macOS; three
candidates named; godoc scoped; the cure commit's probe sentence scoped
to Linux and macOS in the new commit's message). Scoped re-pass: VETO
LEVANTADO; its two P3s (what the mutation proves; a broken comment wrap)
folded.
Closing list, next train's CI: the detector row recipe in docs/HANDOFF.md.
