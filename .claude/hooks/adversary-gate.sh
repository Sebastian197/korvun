#!/bin/bash
# Adversary gate (PreToolUse, Bash matcher): a push to ensayo or
# master must carry a FRESH internal-adversary verdict. The main
# session records the verdict marker after running the adversary
# subagent over the complete diff; this hook only enforces presence
# and freshness — the adversary itself never writes files.
INPUT=$(cat)
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')
case "$COMMAND" in
  *"git push"*ensayo*|*"git push"*master*) ;;
  *) exit 0 ;;
esac
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
