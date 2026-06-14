#!/bin/bash
# .claude/hooks/quality-gate.sh
# PreToolUse hook: runs `make quality` before any git commit and
# blocks (exit 2) if it fails.
INPUT=$(cat)
CMD=$(echo "$INPUT" | jq -r '.tool_input.command // empty')
CWD=$(echo "$INPUT" | jq -r '.cwd // empty')
if ! echo "$CMD" | grep -qE '\bgit\b.*\bcommit\b'; then
    exit 0
fi
cd "$CWD" || { echo "quality-gate: cannot cd into $CWD" >&2; exit 2; }
if [ ! -f "Makefile" ]; then
    exit 0
fi
OUTPUT=$(make quality 2>&1)
STATUS=$?
if [ "$STATUS" -ne 0 ]; then
    echo "BLOCKED: 'make quality' failed, commit not allowed. Fix and retry." >&2
    echo "--- make quality output (tail) ---" >&2
    echo "$OUTPUT" | tail -n 25 >&2
    exit 2
fi
exit 0