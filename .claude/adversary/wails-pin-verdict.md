# VETO MANTENIDO — internal adversary, the Wails pin train, 2026-09-22

Written to disk BEFORE any cure of its findings began. Verbatim as returned.
Tree audited read-only: `korvun-wailscli.nosync`, branch `wails-cli-v2160`,
base `a5c66c4`. One pass, per the director's working mode of 2026-09-22.

---

## P1-1 — The only guarantee the train sells has no mould: `main()`'s red is untestable and untested

Three letreros promise "it reddens the bump's own pull request"
(`quality.yml:167-168`, `Makefile:240-241`, `ADR-0036:93-94`). That promise is
carried entirely by `main()`'s return values (`wails_pin.py:121` `return 2`,
`:132` `return 1`, `:134` `return 0`, `:138` `sys.exit(main())`), and all eleven
rows stay GREEN when those returns are neutralised. Every row calls `scan()` or
`library_version()` directly; no row calls `main()` and no row observes an exit
code.

Reproduction executed: replace `return 1` with `return 0` at `:132` and
`return 2` with `return 0` at `:121`, then run the suite and the mutated guard
over a fully drifted tree.

    $ python3 $S/mut/wails_pin_test.py 2>&1 | tail -3
    Ran 11 tests in 0.023s

    OK
    $ python3 $S/mut/wails_pin.py $S/atk/drift >/dev/null 2>&1; echo $?
    0
    $ python3 $SRC/scripts/wails_pin.py $S/atk/drift >/dev/null 2>&1; echo $?
    1

The guard is silently disarmed, the fully drifted tree passes, and the eleven
attack rows do not notice. The canto's table declares five guarantees with five
mutations; the guarantee the letreros actually sell is not among them.

Required outcome: either `main`, its stderr taxonomy and its 0/1/2 get a mould
with an executed red, or the three sentences promising the PR red are struck.

## P2-1 — Two false numbers in ADR-0036, one repeated in the canto (Rule 2h)

`ADR-0036:73` — "both hold 1037 blobs". The real count is **1036**: the trees
API returns `{"blobs":1036, "trees":366, "total":1402, "truncated":false}` for
both `ee0be80a…` and `513d492b…`. No arithmetic produces 1037.

`ADR-0036:87` and the canto `:129-130` — "nine files of
`internal/frontend/desktop/linux`". The real count is **8**, by every counting:
`grep -ril webkit2_41` → 8, `grep -rn "#cgo webkit2_41 pkg-config"` → 8.
Files: clipboard.go gtk.go keys.go screen.go window.go frontend.go menu.go
webkit2.go. The same 8 at v2.15.0, so it was never nine.

## P2-2 — ADR-0036 states a leak wider than the leak (Tone law, class e)

`ADR-0036:78-79` — "every C string the darwin and linux frontends allocated
leaked." Only strings routed through `Calloc.String` leaked. The frontends
allocate 20+ C strings that never touch `Calloc` and free them explicitly
(`darwin/window.go:178,192,210` `Run`, `ExecJS`, `SetTitle` — allocate and
`C.free`); v2.16.0 does not touch them. The true sentence scopes to
`Calloc.String`.

## P2-3 — The ADR shipped the exact false causal claim the canto says it cured

`ADR-0036:93-94` — "`scripts/wails_pin.py` runs in `make quality`, **so** it
reddens the bump's own pull request". `grep -n "make " quality.yml` → no make
invocation. The canto records catching and curing this inference (`:47-51`,
`:122-125`) and the Makefile was corrected; the ADR was not. The same sentence,
killed once, survives in the more permanent document.

## P2-4 — The guard prints an unscoped affirmative over genuinely drifted trees

`wails_pin.py:133` prints "library and CLI both name X" — an absolute about
"the CLI" on the strength of scanning exactly two locations (`:69-78`).
Executed reproductions, each carrying a working drifted `go install`, all
printing the affirmative with exit 0: a composite action under
`.github/actions/`; a script under `scripts/`; a Dockerfile; a bash line
continuation splitting the pin across lines inside the real release lane; an
`env:` indirection. Positive control: a second file under `.github/workflows/`
IS caught — the limitation is the scan surface, not the matcher.

## P2-5 — `go.mod` is read as text, so a `replace` defeats the guard (class b)

`wails_pin.py:45-48` matches the require line only. Executed: append
`replace github.com/wailsapp/wails/v2 => github.com/wailsapp/wails/v2 v2.13.0`.

    $ GOFLAGS=-mod=mod go list -m github.com/wailsapp/wails/v2
    github.com/wailsapp/wails/v2 v2.16.0 => github.com/wailsapp/wails/v2 v2.13.0
    $ python3 scripts/wails_pin.py .
    wails pin guard: library and CLI both name v2.16.0.
    exit=0

The effective library is v2.13.0 and the guard asserts v2.16.0. The
authoritative reader exists and is not used. The block form happens to fail
closed with "names … 2 times" — accidental, and the message misdescribes the
cause.

## P2-6 — `build_files` silently skips a missing/renamed/non-file Makefile (class c)

`wails_pin.py:72-74`: the `else` is silence. Executed, all three printing the
affirmative with exit 0 while a stale `wails v2.15.0` survives: `Makefile`
renamed to `GNUmakefile`; the comment moved into an included `make/desktop.mk`;
`Makefile` replaced by a directory. One of the four governed sites is
`Makefile:84`; any of the three retires it without a word.

## P2-7 — The letrero wire is `v2\.\d+\.\d+`, the documentation says "every Wails version" (class e)

`wails_pin.py:52` against `wails_pin.py:18-19` and `ADR-0036:95-96`. Executed,
both genuinely stale and both passing: `- name: Install wails CLI 2.15.0` (no
leading v) → exit 0; `# wails v3.0.0-alpha.20 is NOT adopted` → exit 0.

## P2-8 — The guard emits diagnostics its own cited line contradicts

`wails_pin.py:100-105` runs the letrero regex over the line the pin matcher
already read, so a suffixed version is reported against its own prefix. Executed
on a CONSISTENT prerelease tree (`go.mod` and the pin both `v2.16.0-rc1`):

    FAIL: stale wails letrero: .github/workflows/release-desktop.yml:156: names Wails v2.16.0, but go.mod pins v2.16.0-rc1

Line 156 literally reads `…/cmd/wails@v2.16.0-rc1`. Same with a pseudo-version.
Second shape, a false red on a correct tree: `- uses: wailsapp/setup-action@v2.3.1`
beside the correct pin →

    FAIL: stale wails letrero: …:154: names Wails v2.3.1, but go.mod pins v2.16.0

Any unrelated `v2.x.y` on a line containing "wails" is reported as a Wails
letrero. `release.yml:98` carries `…/goreleaser/…/v2.18.0/…` and is saved only
by not containing the word.

## P3 findings

- **P3-1.** The canto `:133-134` — "the door is four lines above the functions
  the rows use". `main` is 34 lines BELOW `scan` and 60 below `library_version`.
  CLAUDE.md's "Comments carry no relative positions" forbids the form; here it
  is also false in direction and distance.
- **P3-2.** The canto `:81` declares "M5-bis separator no longer pins the path's
  end — Red: yes". Both literal readings stay GREEN (`\s*(v\S+)`, `(v\S+)`);
  `.*?\s+(v\S+)` and `[^ ]*\s+(v\S+)` do redden. The row is not tautological,
  but an auditor re-running the declared mutation verbatim gets green. The
  other five claims reproduced exactly (M1:1, M2:1, M3:1, M4:2 failures, and
  the dead lookahead confirmed genuinely unreachable).
- **P3-3.** The canto `:66` excuses both control rows "by construction". But
  `test_aHistoricalVersionInProseIsNotTheGuardsBusiness` asserts a real
  guarantee and its mutation exists and reddens: widen `build_files` with
  `root.rglob("*.md")` → that row fails. Doctrine point 4 is universal.
- **P3-4.** `wails_pin.py:90` reads workflow files with no guard; only
  `ValueError` is caught at `:119`. Executed: non-UTF-8 workflow → exit 2 with a
  FAIL line only because `UnicodeDecodeError` subclasses `ValueError`
  (accidental, undocumented, untested); `chmod 000` → exit 1 with a
  `PermissionError` traceback and no FAIL line; a directory named `*.yml` →
  exit 1 with an `IsADirectoryError` traceback. All fail closed, so not a
  fail-open, but the docstring's taxonomy names only go.mod cases.
- **P3-5.** The step sits at `quality.yml:169` after `Install tools`, `Build`,
  `Lint` and `Ensure sqlite3` (network installs). GitHub stops a job at the
  first failing step, so any of those reds means the drift is never reported.
  The check is pure Python and needs neither Go nor sqlite3. In `Makefile:250`
  it is the LAST prerequisite, behind `test` and `cover`.
- **P3-6.** `ADR-0036:98-100` says "a guard that rewrote history to match the
  present would be worse than the drift it catches". Inside `.github/workflows/`
  the guard demands exactly that: a workflow comment naming v2.13.0 and v2.15.0
  as history → exit 1. The exemption is docs-only; the ADR sentence does not say
  so.
- **P3-7.** `wails_pin.py:133` re-reads `go.mod` to print a value it already
  computed (`want`, at `:83`). Class (b) in miniature.
- **P3-8.** `wails_pin_test.py:85` — `any("release-desktop.yml:8" in line …)` is
  a substring match that also accepts `:80`, `:81`, `:8x`.
- **P3-9.** The fixture builds one workflow file, a 4-line Makefile and a 9-line
  go.mod. Untested: multiple workflow files, the `.yaml` extension, two pins
  disagreeing with each other, an absent Makefile.
- **P3-10.** Scope: the webpackbar HANDOFF filing, 31 lines about a different
  dependency and a different train, rides this commit's subject and gate.
  Defensible under Scope control step 4; the corollary about batching cuts the
  other way. Director's call. The guard itself is NOT scope creep: ADR-0036's
  rule is the commission's subject and the guard is the only thing that makes
  the pin move durable.

## What the adversary verified as TRUE

The pre-cure red reproduced exactly with all four sites named; every line-number
citation in the canto §1; "exactly three blobs differ" confirmed twice
independently; the tag→commit resolution and its 2026-09-14 date; `v2/go.mod`
byte-identical declaring `go 1.25.0`; `apt.go` unchanged and naming the four
packages; the `Calloc` mechanism (value→pointer receivers, `Free()` over an
empty pool); `python3` under `shell: bash` green on `windows-latest` in this
repo's own runs; the wiring (no `if:` on the new step, the matrix, `make
quality`'s dependency, the target aborting on its first failing line, the
pre-commit hook running from the git top level); the history (#56 = `cf05372`,
#34 = `45d046b`, both go.mod+go.sum only, both leaving the CLI behind, #56 fully
green); every factual claim in the HANDOFF filing including the captured job-log
bytes; the sqlite/libc pairing; the eleven rows green on the real tree; and that
no Go source moves.

## Scope and limits the adversary declared

- **PREDICTION, not executed:** that `shell: bash` supplies `-eo pipefail`, so a
  red `wails_pin_test.py` aborts the step before `wails_pin.py .` runs. A
  GitHub runner could not be executed. The repo's own `Ensure sqlite3` step
  writes `set -euo pipefail` explicitly.
- **UNVERIFIABLE from the tree:** `ADR-0036:92` "the second was caught by a
  human reading a diff" — narrative, no artifact.
- **Not checked:** whether `quality` is a required check in branch protection.
- **Refusal hit, reported not worked around:** the push-gate hook blocked a
  read-only `git config core.hooksPath` query because the command text carried a
  hatch word. Answered by other means rather than re-phrased to slip the gate.
- **Unexamined:** the full `make quality`; the guard on Linux or Windows Python
  (all local runs Python 3.11.9 on macOS 13.7.8); the rest of the Makefile and
  the other 15 workflows beyond a targeted grep.
