VETO LEVANTADO d38c9fd70d859e8fb63103dedf96265bb60ed9d9
Round: v0.16.2, PR #69, the cures after the first CI run (2026-09-27). The
first run over ac114930 went red on Ubuntu, Windows and the chrome e2e;
macOS was green, and AS07 was not among the failures. The director
adjudicated seven classes (A to G) and, after the adversary's first pass,
the chrome e2e harness fix.

WHAT THE CURES ARE.
- A, scoped and not cured (train G): createdHere, CreateLedger and the
  created field say what os.SameFile compares on each platform; the
  replaced-file mould skips on Linux and Windows and runs everywhere else;
  the release notes list the limit and its damage.
- B: dsnPathFor, pure and taking the GOOS, gives back the Windows drive path
  a DSN was built from, so the hook's seams arm there.
- C: TE47 and TE56 sandbox AppData too and assert the resolved ledger path
  before anything is written.
- D: TE49 finds the ledger's directory as the JSON body spells it.
- E: the CLI binary test builds korvun.exe on Windows, fails by name when a
  command cannot run, and pins ledger check's exit status to 1.
- F: the profile identity test keeps its profile under the working
  directory.
- G: four chrome e2e specs expect the redesigned /healthz badge; the
  activity chip is found by its test id and still required visible.
- The harness (director's decision): isolationEnv and isolate point AppData
  and LocalAppData into its temp dir; a test of the built binary, run as a
  separate process with its core started, proves it never touches a marker
  profile standing for the user's real one.

THE ADVERSARY. Six passes over the delta: VETO MANTENIDO (one P2, nine P3),
VETO LEVANTADO (four P3), VETO MANTENIDO (one P2, three P3), VETO LEVANTADO
(three P3), VETO LEVANTADO (no findings), and a sixth after the gate forced
a prettier reformat of two specs: VETO LEVANTADO, with one P3 left to the
director (the verbatim record keeps the adversary's own relative locators).
Every other finding was cured in the delta. The six passes, verbatim:
v0162-pr69-cures-verdict.md in this directory.

EVIDENCE. Every cure red where its defect exists on this host, green, and
its probing mutations red and restored by sha256; each of the 14 changed e2e
assertions seen red under its mutant; go vet for windows, linux and darwin
and cross-built test binaries; the chrome e2e 50 passed, 1 skipped; make
quality through the cures commit's pre-commit gate: exit 0, 20:34:23 to 20:46:47, on the cures commit d38c9fd7 (a first
attempt, 19:26:38 to 20:23:40, passed every step up to its last,
desktop-frontend-check, which failed on prettier; the second reused go
test's cache for the unchanged Go code). The Windows
half (the hook's moulds armed there for the first time, the harness binary
test, C, D, E and F) is this PR's CI.
