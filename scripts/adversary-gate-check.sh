#!/bin/bash
# scripts/adversary-gate-check.sh — THE push-authorization check (R12-H8, B1).
#
# One script, no command detection: it answers "is a push authorized
# RIGHT NOW?" and nothing else. Both doors call it — the session
# PreToolUse hook (.claude/hooks/adversary-gate.sh, the first line) and
# git's own pre-push hook (.githooks/pre-push, the local gate git fires
# whatever shell form invoked the push).
#
# What it checks, exactly:
#   1. <repo root>/.claude/adversary/last-verdict.md exists;
#   2. its FIRST line is exactly "VETO LEVANTADO" (whole line — a
#      "VETO MANTENIDO (... VETO LEVANTADO ...)" first line does not pass);
#   3. HEAD's commit time can be read (no repo, no HEAD → BLOCKED,
#      never "fresh by default");
#   4. the verdict file's modification time is not older than HEAD's
#      commit time.
# What it does NOT check (recorded; B5 filed to the next train): the
# audited HEAD SHA. The verdict names it in prose only and this script
# does not compare it. Consequence, declared: the file is TRACKED, so a
# fresh clone or `git worktree add` writes it with mtime = now and the
# freshness check passes there for ANY verdict text. Until B5 lands,
# freshness holds only in the checkout where the verdict was recorded.
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
if [ "$(head -1 "$VERDICT")" != "VETO LEVANTADO" ]; then
  echo "BLOCKED by adversary gate: the recorded verdict's first line is not exactly VETO LEVANTADO. Cure or adjudicate the findings first." >&2
  exit 2
fi
HEAD_TIME=$(git -C "$ROOT" log -1 --format=%ct 2>/dev/null)
if [ -z "$HEAD_TIME" ]; then
  echo "BLOCKED by adversary gate: cannot read HEAD's commit time in $ROOT — freshness cannot be judged, so nothing is authorized." >&2
  exit 2
fi
VERDICT_TIME=$(stat -f %m "$VERDICT" 2>/dev/null || stat -c %Y "$VERDICT" 2>/dev/null)
if [ -z "$VERDICT_TIME" ]; then
  echo "BLOCKED by adversary gate: cannot read the verdict's modification time." >&2
  exit 2
fi
if [ "$VERDICT_TIME" -lt "$HEAD_TIME" ]; then
  echo "BLOCKED by adversary gate: the verdict predates HEAD — the audited diff is not the one being pushed. Re-run the adversary over the current diff." >&2
  exit 2
fi
exit 0
