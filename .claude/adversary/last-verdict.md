VETO LEVANTADO 2f88dda8ea3e32f5db74c22925990f2657ed0df4
Round: v0.16.2, master sanitation before the tag (2026-10-03). Since
2026-09-30T00:00Z master's suite has failed on every tree: issueGrantID
issued its parent grant with a fixed --expires, and two delegation
tests answer authority_expired. AS07 fell twice more in master's
Windows CI (run 36520026213). The director ordered one PR with two
test commits and this marker; nothing in production changes.

WHAT THE COMMITS ARE.
- fix(test) a408f0b9: the validity bounds the tests hand the CLI follow
  the wall clock; the one window past on purpose keeps its two dates;
  the sqlite claim test's 2099 tamper value is relative. The gate the
  director dictated, internal/testgates: no string literal holding an
  RFC3339 instant to the second, and no time.Date with a literal year,
  of the current year or later in any test file, unless its exception
  list excuses that date in that file with a written reason.
- test(sqlite) 2f88dda8: AS07 retries ledger_busy in the test, never in
  the door, as prepared. The HANDOFF moves point 17 to cured and files
  point 18 (BEGIN IMMEDIATE does not stop waiting when the caller's
  context ends), a P2 for v0.16.3 train H, with a line in the release
  notes' known limits.

THE ADVERSARY. Commit 1 and commit 2's filing: seven passes. The first
five, each VETO MANTENIDO, judged a name evaluator the director retired
(option D); the sixth, over the dictated gate, VETO MANTENIDO (the
commit message missing, comments naming the retired gate); the
seventh, VETO LEVANTADO, with three text P3 folded before the commit.
Verbatim: v0162-date-bomb-verdict.md in this directory. The AS07 cure:
four passes, the last VETO LEVANTADO without findings; verbatim:
v0162-as07-cure-verdict.md.

EVIDENCE. Commit 1: the two delegation tests red on the base and green
after; the gate red against a stub and red with the fixed date put
back, naming the file and the date; 23 probing mutations over the
final bytes, all red by the runner's jsonl and restored by sha256;
golangci-lint, goimports, go vet for three systems, internal/testgates
and internal/cli whole with -race. Commit 2: as recorded in its verdict
(20 probing mutations red and one control green). make quality through
each commit's pre-commit gate: exit 0 on a408f0b9 (12:22:47 to 13:20:05
WEST) and exit 0 on 2f88dda8 (13:20:19 to 14:17:55 WEST).
