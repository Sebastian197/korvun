#!/bin/bash
# Adversary gate (PreToolUse, Bash matcher): a push to ensayo or
# master must carry a FRESH internal-adversary verdict. The main
# session records the verdict marker after running the adversary
# subagent over the complete diff; this hook only enforces presence
# and freshness — the adversary itself never writes files.
INPUT=$(cat)
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')
# R12-P2-3: gate by SITE, not by text. The command must BEGIN with an
# actual git-push invocation (allowing cd/&& prefixes ending in git),
# not merely contain the substring anywhere — a heredoc QUOTING the
# words must not trip the gate, and a bare `git push` with an
# upstream set is gated like an explicit refspec.
if ! echo "$COMMAND" | grep -qE '(^|&&|;)[[:space:]]*git[[:space:]]+push([[:space:]]|$)'; then
  exit 0
fi
VERDICT="$CLAUDE_PROJECT_DIR/.claude/adversary/last-verdict.md"
if [ ! -f "$VERDICT" ]; then
  echo "BLOCKED by adversary gate: no internal-adversary verdict recorded (.claude/adversary/last-verdict.md). Run the adversary subagent over the complete diff, record its verdict, then push." >&2
  exit 2
fi
if ! head -1 "$VERDICT" | grep -q "VETO LEVANTADO"; then
  echo "BLOCKED by adversary gate: the recorded verdict is not VETO LEVANTADO. Cure or adjudicate the findings first." >&2
  exit 2
fi
HEAD_TIME=$(git -C "$CLAUDE_PROJECT_DIR" log -1 --format=%ct 2>/dev/null || echo 0)
VERDICT_TIME=$(stat -f %m "$VERDICT" 2>/dev/null || stat -c %Y "$VERDICT" 2>/dev/null || echo 0)
if [ "$VERDICT_TIME" -lt "$HEAD_TIME" ]; then
  echo "BLOCKED by adversary gate: the verdict predates HEAD — the audited diff is not the one being pushed. Re-run the adversary over the current diff." >&2
  exit 2
fi
exit 0
