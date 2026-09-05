#!/bin/bash
# scripts/adversary-gate-check.sh — THE push-authorization check (R12-H8, B1).
#
# One script, no command detection: it answers "is a push authorized
# RIGHT NOW?" and nothing else. Both doors call it — the Claude Code
# PreToolUse hook (.claude/hooks/adversary-gate.sh, the first line) and
# git's own pre-push hook (.githooks/pre-push, the definitive local
# gate: git fires it whatever shell form invoked the push).
#
# What it checks, exactly (unchanged from the R12 hook it was extracted
# from — the semantics were NOT widened here):
#   1. <repo root>/.claude/adversary/last-verdict.md exists;
#   2. its FIRST line is "VETO LEVANTADO";
#   3. its modification time is not older than HEAD's commit time
#      (a verdict recorded before the commit being pushed is stale).
# What it does NOT check (recorded, B5): the verdict names the audited
# HEAD only in prose ("HEAD=<sha> at verdict time"); this check does
# not parse or compare that SHA. Freshness is by mtime alone.
#
# Exit 0 = authorized. Exit 2 = blocked, reason on stderr.
set -u
ROOT="${1:-}"
if [ -z "$ROOT" ]; then
  ROOT=$(git rev-parse --show-toplevel 2>/dev/null || echo "${CLAUDE_PROJECT_DIR:-.}")
fi
VERDICT="$ROOT/.claude/adversary/last-verdict.md"
if [ ! -f "$VERDICT" ]; then
  echo "BLOCKED by adversary gate: no internal-adversary verdict recorded (.claude/adversary/last-verdict.md). Run the adversary subagent over the complete diff, record its verdict, then push." >&2
  exit 2
fi
if ! head -1 "$VERDICT" | grep -q "VETO LEVANTADO"; then
  echo "BLOCKED by adversary gate: the recorded verdict is not VETO LEVANTADO. Cure or adjudicate the findings first." >&2
  exit 2
fi
HEAD_TIME=$(git -C "$ROOT" log -1 --format=%ct 2>/dev/null || echo 0)
VERDICT_TIME=$(stat -f %m "$VERDICT" 2>/dev/null || stat -c %Y "$VERDICT" 2>/dev/null || echo 0)
if [ "$VERDICT_TIME" -lt "$HEAD_TIME" ]; then
  echo "BLOCKED by adversary gate: the verdict predates HEAD — the audited diff is not the one being pushed. Re-run the adversary over the current diff." >&2
  exit 2
fi
exit 0
