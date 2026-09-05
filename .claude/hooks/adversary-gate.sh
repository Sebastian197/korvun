#!/bin/bash
# Adversary gate (PreToolUse, Bash matcher) — the FIRST line, not the
# definitive one (R12-H8, B3). A push must carry a FRESH internal-
# adversary verdict; the decision lives in ONE place,
# scripts/adversary-gate-check.sh, shared with git's pre-push hook
# (.githooks/pre-push, installed by `make install-hooks`), which git
# fires whatever shell form invoked the push.
#
# THE REAL PERIMETER, declared honestly:
#   NORMALIZATION before matching — the command text is lower-cased,
#   its backslashes and quote characters (' and ") are removed, and a
#   second form with backslashes turned into "/" is matched too (so
#   `Git`, `pus\h`, `pu''sh`, `--no-"verify"` and `C:\tools\git.exe`
#   all read as their plain spelling).
#   COVERS — any line of the normalized command where the word `git`
#   is followed by the word `push` (keywords, wrappers, `time`, `nohup`,
#   absolute paths, global options), and EXPLICITLY BLOCKS, whatever
#   the verdict says, any git command carrying the two escape hatches
#   git itself offers — `--no-verify` (any unique prefix git accepts:
#   `--no-ver`, `--no-verif`...) or `core.hooksPath` (any letter case).
#   False positives are ACCEPTED: `echo "git push"`, a heredoc quoting
#   the words, or `git add .githooks/pre-push` are gated too (a block
#   too many costs a fresh verdict; a block too few costs an ungated
#   push).
#   FAILS CLOSED — if jq is missing, or jq cannot parse the hook input,
#   EVERY Bash command is blocked (the shape cannot be read); if the
#   check script is missing or not executable, a push-shaped command
#   is blocked. A tooling error never lets a push through.
#   DOES NOT COVER — a push whose normalized text never shows
#   `git ... push` on one line (`sh -c` strings built from variables,
#   `$(...)` and `${...}` expansions, aliases, a script file that
#   pushes, `eval`, GUI clients, another terminal outside this
#   session), and the ways to DISARM the pre-push door that carry no
#   git word at all: `rm .git/hooks/pre-push`, `chmod -x` on the hook,
#   or editing scripts/adversary-gate-check.sh (the Edit tool is not
#   under this Bash matcher). Nor the freshness gap of a fresh clone or
#   worktree (see the check script's header). The gate is a discipline
#   aid for this session; it is not a security boundary.
# The probe script scripts/adversary-gate-probe.sh runs the shapes
# through this hook AND through the pre-push door on every `make quality`.
INPUT=$(cat)
if ! command -v jq >/dev/null 2>&1; then
  echo "BLOCKED by adversary gate: jq is missing — the hook cannot read the command, so it fails closed." >&2
  exit 2
fi
if ! RAW=$(echo "$INPUT" | jq -r '.tool_input.command // empty' 2>/dev/null); then
  echo "BLOCKED by adversary gate: the hook input is not parseable JSON — the command shape cannot be read, so it fails closed." >&2
  exit 2
fi
LOWER=$(printf '%s' "$RAW" | tr 'A-Z' 'a-z' | tr -d "'\"")
N1=$(printf '%s' "$LOWER" | tr -d '\\')
N2=$(printf '%s' "$LOWER" | tr '\\' '/')
PUSH_RE='(^|[^[:alnum:]_])git([^[:alnum:]_].*)?[^[:alnum:]_]push([^[:alnum:]_]|$)'
GIT_RE='(^|[^[:alnum:]_])git[^[:alnum:]_]'
HATCH_RE='--no-ver[a-z]*|core\.hookspath'
matches() { printf '%s\n' "$N1" | grep -qE -- "$1" || printf '%s\n' "$N2" | grep -qE -- "$1"; }
if matches "$GIT_RE" && matches "$HATCH_RE"; then
  echo "BLOCKED by adversary gate: --no-verify / core.hooksPath (any spelling git accepts) skip git's pre-push hook. Not allowed in any git command." >&2
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
