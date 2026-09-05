#!/bin/bash
# Adversary gate (PreToolUse, Bash matcher) — the FIRST line, not the
# definitive one (R12-H8, B3). A push must carry a FRESH internal-
# adversary verdict; the decision lives in ONE place,
# scripts/adversary-gate-check.sh, shared with git's pre-push hook
# (.githooks/pre-push, installed by `make install-hooks`), which git
# fires whatever shell form invoked the push.
#
# THE REAL PERIMETER, declared honestly:
#   COVERS — any line of the command where the word `git` is followed
#   by the word `push` (keywords, wrappers, `time`, `nohup`, absolute
#   paths, global options, backslash-escaped letters: backslashes are
#   stripped before matching, so `pus\h` is `push`), and EXPLICITLY
#   BLOCKS, whatever the verdict says, any git command carrying the two
#   escape hatches git itself offers — `--no-verify` (any unique prefix
#   git accepts: `--no-ver`, `--no-verif`...) or `core.hooksPath` (any
#   letter case; git config keys are case-insensitive). False positives
#   are ACCEPTED: `echo "git push"`, a heredoc quoting the words, or
#   `git add .githooks/pre-push` are gated too (a block too many costs a
#   fresh verdict; a block too few costs an ungated push).
#   FAILS CLOSED — if jq or the check script is missing or not
#   executable, a push-shaped command is BLOCKED (exit 2), never let
#   through on a tooling error.
#   DOES NOT COVER — a push whose text never shows `git ... push` on one
#   line (`sh -c` strings built from variables, aliases, a script file
#   that pushes, `eval`, GUI clients, another terminal outside this
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
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty' | tr -d '\\')
PUSH_RE='(^|[^[:alnum:]_])git([^[:alnum:]_].*)?[^[:alnum:]_]push([^[:alnum:]_]|$)'
GIT_RE='(^|[^[:alnum:]_])git[^[:alnum:]_]'
HATCH_RE='--no-ver[a-z]*|core\.hookspath'
if echo "$COMMAND" | grep -qE "$GIT_RE" && echo "$COMMAND" | grep -qiE -- "$HATCH_RE"; then
  echo "BLOCKED by adversary gate: --no-verify / core.hooksPath (any spelling git accepts) skip git's pre-push hook. Not allowed in any git command." >&2
  exit 2
fi
if ! echo "$COMMAND" | grep -qE "$PUSH_RE"; then
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
