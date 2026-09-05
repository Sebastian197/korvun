#!/bin/bash
# Adversary gate (PreToolUse, Bash matcher) — the FIRST line, not the
# definitive one (R12-H8, B3). A push must carry a FRESH internal-
# adversary verdict; the decision lives in ONE place,
# scripts/adversary-gate-check.sh, shared with git's pre-push hook
# (.githooks/pre-push, installed by `make install-hooks`), which git
# fires for `git push` from this checkout whatever the shell form.
#
# THE REAL PERIMETER, declared honestly (a discipline aid for this
# session; NOT a security boundary — director's adjudication):
#   NORMALIZATION before matching — the command text is lower-cased and
#   stripped of quote characters (' and "), of `$` and of backslashes;
#   a second form with backslashes turned into "/" is matched too. So
#   `Git`, `pus\h`, `pu''sh`, `pu$'s'h`, `--no-"verify"`, `--no-$'v'erify`
#   and `C:\tools\git.exe` all read as their plain spelling.
#   COVERS — any line of the normalized command where the word `git` is
#   followed by the word `push` or by `send-pack` (the plumbing that
#   pushes WITHOUT firing the pre-push hook), and EXPLICITLY BLOCKS,
#   whatever the verdict says, any git command whose text carries one
#   of the two escape hatches — `--no-verify` (any unique prefix git
#   accepts: `--no-ver`, `--no-verif`...) or `core.hooksPath` (any
#   letter case). This block is by TEXT and unconditional: a `git log
#   --grep=no-verify`, a commit message mentioning the hatch words, or
#   a path containing them are blocked too — the accepted cost; write
#   such messages from a file.
#   False positives are ACCEPTED: `echo "git push"`, a heredoc quoting
#   the words, or `git add .githooks/pre-push` are gated too (a block
#   too many costs a fresh verdict; a block too few costs an ungated
#   push).
#   FAILS CLOSED — if cat, jq, grep or tr is missing, or jq cannot parse the
#   hook input, or the normalization pipeline fails, EVERY Bash command
#   is blocked (the shape cannot be read); if the check script is
#   missing or not executable, a push-shaped command is blocked.
#   DOES NOT COVER — a push whose normalized text never shows `git ...
#   push` (or send-pack) on ONE line: `sh -c` strings built from
#   variables, `$(...)`/`${...}` expansions, brace expansion
#   (`p{u..u}sh`), a backslash line continuation splitting the words,
#   aliases defined earlier (`git config alias.p push` in another
#   command), a script file that pushes, `eval`, GUI clients, another
#   terminal outside this session. Hatches reached WITHOUT hatch text:
#   a config file written by another command or outside Bash
#   (`GIT_CONFIG_GLOBAL=<file>`, a swapped `HOME`, `.git/config`
#   edited with the Edit tool). The ways to DISARM the pre-push door
#   with no git word: `rm .git/hooks/pre-push`, `chmod -x` on it, or
#   editing scripts/adversary-gate-check.sh outside Bash. And the
#   freshness gap of a fresh clone or worktree (see the check script).
# The probe script scripts/adversary-gate-probe.sh runs the shapes
# through this hook AND through the pre-push door on every `make
# quality` and, in CI, on the Linux and macOS runners.
set -o pipefail
# The tools come BEFORE the read: a missing `cat` would leave INPUT
# empty and an empty command reads as "no push" (adversary fifth-pass
# mutation m-notr exposed it).
for tool in cat jq grep tr; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "BLOCKED by adversary gate: $tool is missing — the hook cannot read the command, so it fails closed." >&2
    exit 2
  fi
done
if ! INPUT=$(cat) || [ -z "$INPUT" ]; then
  # A failed or EMPTY read is not "no push": the shape is unknown.
  echo "BLOCKED by adversary gate: the hook input could not be read or is empty — the command shape is unknown, so it fails closed." >&2
  exit 2
fi
if ! RAW=$(echo "$INPUT" | jq -r '.tool_input.command // empty' 2>/dev/null); then
  echo "BLOCKED by adversary gate: the hook input is not parseable JSON — the command shape cannot be read, so it fails closed." >&2
  exit 2
fi
if ! LOWER=$(printf '%s' "$RAW" | tr 'A-Z' 'a-z' | tr -d "'\"\$"); then
  echo "BLOCKED by adversary gate: the command could not be normalized — it fails closed." >&2
  exit 2
fi
if ! N1=$(printf '%s' "$LOWER" | tr -d '\\') || ! N2=$(printf '%s' "$LOWER" | tr '\\' '/'); then
  echo "BLOCKED by adversary gate: the command could not be normalized — it fails closed." >&2
  exit 2
fi
PUSH_RE='(^|[^[:alnum:]_])git([^[:alnum:]_].*)?[^[:alnum:]_](push|send-pack)([^[:alnum:]_]|$)'
GIT_RE='(^|[^[:alnum:]_])git[^[:alnum:]_]'
HATCH_RE='--no-ver[a-z]*|core\.hookspath'
# Here-strings, never a pipe into `grep -q`: with pipefail on, a
# producer killed by SIGPIPE when grep exits early on a match would
# turn a MATCH into "no match" for any command longer than the pipe
# buffer — the fail-open the repo already documented in quality.yml
# (adversary fourth pass, P3-1).
# grep's three outcomes are kept apart: 0 match, 1 no match, anything
# else (a broken grep, a failed here-string) is a tooling failure and
# BLOCKS — never read as "no match" (adversary fifth pass, P3-A).
match_one() {
  grep -qE -- "$1" <<<"$2"
  local s=$?
  if [ "$s" -gt 1 ]; then
    echo "BLOCKED by adversary gate: grep failed with status $s — the command shape cannot be judged, so it fails closed." >&2
    exit 2
  fi
  return "$s"
}
matches() { match_one "$1" "$N1" || match_one "$1" "$N2"; }
if matches "$GIT_RE" && matches "$HATCH_RE"; then
  echo "BLOCKED by adversary gate: --no-verify / core.hooksPath (the hatch words, any case, any quoting) skip git's pre-push hook. Not allowed in any git command; write commit messages that mention them from a file." >&2
  exit 2
fi
if ! matches "$PUSH_RE"; then
  exit 0
fi
ROOT="${CLAUDE_PROJECT_DIR:-$(pwd)}"
CHECK="$ROOT/scripts/adversary-gate-check.sh"
if [ ! -x "$CHECK" ]; then
  echo "BLOCKED by adversary gate: $CHECK is missing or not executable — the decision script is gone, so the push fails closed." >&2
  exit 2
fi
"$CHECK" "$ROOT"
STATUS=$?
if [ "$STATUS" -ne 0 ]; then
  exit 2
fi
exit 0
