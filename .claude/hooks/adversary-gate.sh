#!/bin/bash
# Adversary gate (PreToolUse, Bash matcher) — the FIRST line, not the
# definitive one (R12-H8, B3). A push must carry a FRESH internal-
# adversary verdict; the decision lives in ONE place,
# scripts/adversary-gate-check.sh, shared with git's pre-push hook
# (.githooks/pre-push, installed by `make install-hooks`), which is the
# definitive local gate: git fires it whatever shell form invoked the
# push, so the shapes below that fool a regex cannot fool it.
#
# THE REAL PERIMETER, declared:
#   COVERS — any command text where the word `git` is followed anywhere
#   on the same line by the word `push` (keywords, wrappers, `time`,
#   `nohup`, `\git`, absolute paths, global options: all of them), and
#   EXPLICITLY BLOCKS, whatever the verdict says, any git command
#   carrying `--no-verify` or `core.hooksPath` — the two ways git itself
#   lets a push skip the pre-push hook. False positives are ACCEPTED:
#   `echo "git push"` or a heredoc quoting the words is gated too (a
#   block too many costs a fresh verdict; a block too few costs an
#   ungated push).
#   DOES NOT COVER — a push whose text never shows `git ... push` on
#   one line: `sh -c` strings assembled from variables, aliases, a
#   script file that pushes, `eval`, GUI clients. For those the
#   pre-push hook is the gate; this hook is a convenience wall only.
#   Multi-line commands are scanned line by line.
# The probe script scripts/adversary-gate-probe.sh runs the fourteen
# shapes (plus negatives) through this hook on every `make quality`.
INPUT=$(cat)
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')
if ! echo "$COMMAND" | grep -qE '(^|[^[:alnum:]_])git([^[:alnum:]_].*)?[^[:alnum:]_]push([^[:alnum:]_]|$)'; then
  # No push shape — but a git command reaching for the hook escape
  # hatches is blocked on its own.
  if echo "$COMMAND" | grep -qE '(^|[^[:alnum:]_])git[^[:alnum:]_]' && echo "$COMMAND" | grep -qE -- '--no-verify|core\.hooksPath'; then
    echo "BLOCKED by adversary gate: --no-verify / core.hooksPath skip git's pre-push hook — the definitive gate. Not allowed in any git command." >&2
    exit 2
  fi
  exit 0
fi
if echo "$COMMAND" | grep -qE -- '--no-verify|core\.hooksPath'; then
  echo "BLOCKED by adversary gate: --no-verify / core.hooksPath skip git's pre-push hook — the definitive gate. Not allowed in any git command." >&2
  exit 2
fi
ROOT="${CLAUDE_PROJECT_DIR:-$(pwd)}"
exec "$ROOT/scripts/adversary-gate-check.sh" "$ROOT"
