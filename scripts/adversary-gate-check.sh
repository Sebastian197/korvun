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
#   3. HEAD's commit time is readable AND an integer (no repo, no HEAD,
#      garbage → BLOCKED, never "fresh by default");
#   4. the verdict file's modification time is readable AND an integer
#      (BSD `stat -f %m` on macOS, GNU `stat -c %Y` on Linux — probed
#      each by its own exit code and validated as digits: GNU `stat -f`
#      means --file-system and PRINTS while failing, so a bare `||`
#      chain would have authorized on garbage — adversary P2-A);
#   5. the verdict's mtime is not older than HEAD's commit time.
# What it does NOT check (recorded; B5 filed to the next train): the
# audited HEAD SHA. The verdict names it in prose only and this script
# does not compare it. Consequence, declared: the file is TRACKED, so a
# fresh clone or `git worktree add` writes it with mtime = now and the
# freshness check passes there for ANY verdict text. Until B5 lands,
# freshness holds only in the checkout where the verdict was recorded.
# Also outside this check (B5 family): the time judged is HEAD's, not
# the pushed ref's; a backdated GIT_COMMITTER_DATE rejuvenates it.
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
is_epoch() { case "$1" in ''|*[!0-9]*) return 1 ;; *) return 0 ;; esac; }
HEAD_TIME=$(git -C "$ROOT" log -1 --format=%ct 2>/dev/null)
if ! is_epoch "$HEAD_TIME"; then
  echo "BLOCKED by adversary gate: cannot read HEAD's commit time in $ROOT (got '$HEAD_TIME') — freshness cannot be judged, so nothing is authorized." >&2
  exit 2
fi
# mtime, each stat dialect probed by ITS OWN exit code, output taken only
# on success; then validated as digits whatever the dialect printed.
VERDICT_TIME=""
if T=$(stat -c %Y "$VERDICT" 2>/dev/null); then
  VERDICT_TIME=$T
elif T=$(stat -f %m "$VERDICT" 2>/dev/null); then
  VERDICT_TIME=$T
fi
if ! is_epoch "$VERDICT_TIME"; then
  echo "BLOCKED by adversary gate: cannot read the verdict's modification time as an integer (got '$VERDICT_TIME') — nothing is authorized." >&2
  exit 2
fi
if [ "$VERDICT_TIME" -lt "$HEAD_TIME" ]; then
  echo "BLOCKED by adversary gate: the verdict predates HEAD — the audited diff is not the one being pushed. Re-run the adversary over the current diff." >&2
  exit 2
fi
exit 0
