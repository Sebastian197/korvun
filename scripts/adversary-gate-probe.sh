#!/bin/bash
# scripts/adversary-gate-probe.sh — the probe table of the Claude Code
# adversary gate (R12-H8, B4). Runs on every `make quality` (well under
# a second: jq + grep per shape). It feeds each command shape to the
# REAL hook (.claude/hooks/adversary-gate.sh) as the PreToolUse JSON
# and demands the exact exit code:
#   - with NO verdict recorded, every push shape must be BLOCKED (2);
#   - with a fresh VETO LEVANTADO recorded, plain push shapes pass (0)
#     while --no-verify / core.hooksPath stay BLOCKED (2) — those skip
#     git's pre-push hook, the definitive gate;
#   - negatives (no push, no escape hatch) pass (0).
# The twelve shapes are the twelfth review's probe table (six of them
# bypassed the pre-H8 regex; the red run is recorded in the R12 ledger),
# plus the two escape hatches the review did not list.
set -u
ROOT=$(git rev-parse --show-toplevel 2>/dev/null || dirname "$(dirname "$(readlink -f "$0")")")
HOOK="$ROOT/.claude/hooks/adversary-gate.sh"
command -v jq >/dev/null || { echo "adversary-gate-probe: jq is required (the hook uses it)"; exit 1; }
[ -x "$HOOK" ] || { echo "adversary-gate-probe: hook not executable: $HOOK"; exit 1; }

NOVERDICT=$(mktemp -d)
FRESH=$(mktemp -d)
mkdir -p "$FRESH/.claude/adversary"
printf 'VETO LEVANTADO\n\nprobe fixture\n' > "$FRESH/.claude/adversary/last-verdict.md"
cp "$ROOT/scripts/adversary-gate-check.sh" "$NOVERDICT/" 2>/dev/null; mkdir -p "$NOVERDICT/scripts" "$FRESH/scripts"
cp "$ROOT/scripts/adversary-gate-check.sh" "$NOVERDICT/scripts/"
cp "$ROOT/scripts/adversary-gate-check.sh" "$FRESH/scripts/"
trap 'rm -rf "$NOVERDICT" "$FRESH"' EXIT

fails=0
probe() { # $1 = project dir, $2 = expected exit, $3 = command shape
  local out code
  out=$(jq -cn --arg c "$3" '{tool_input:{command:$c}}' | CLAUDE_PROJECT_DIR="$1" "$HOOK" 2>&1)
  code=$?
  if [ "$code" -ne "$2" ]; then
    printf 'FAIL  expected %s got %s  %s\n      %s\n' "$2" "$code" "$3" "$out"
    fails=$((fails + 1))
  else
    printf 'ok    exit %s  %s\n' "$code" "$3"
  fi
}

echo "--- no verdict recorded: every push shape must be blocked (2)"
while IFS= read -r shape; do probe "$NOVERDICT" 2 "$shape"; done <<'EOF'
git push origin master
if ! git push origin ensayo; then echo no; fi
if true; then git push origin ensayo; fi
for b in ensayo; do git push origin $b; done
time git push origin ensayo
nohup git push origin ensayo &
\git push origin ensayo
git -C . push
echo x > f; git push
git push
	git push origin master
git -c a=b --no-pager push
git push --no-verify origin master
git -c core.hooksPath=/dev/null push origin master
EOF

echo "--- fresh VETO LEVANTADO: plain pushes pass (0), escape hatches stay blocked (2)"
probe "$FRESH" 0 'git push origin master'
probe "$FRESH" 0 'if ! git push origin ensayo; then echo no; fi'
probe "$FRESH" 2 'git push --no-verify origin master'
probe "$FRESH" 2 'git -c core.hooksPath=/dev/null push origin master'
probe "$FRESH" 2 'git commit --no-verify -m x'

echo "--- negatives: no push, no escape hatch (0)"
probe "$NOVERDICT" 0 'git commit -m "wip"'
probe "$NOVERDICT" 0 'git log --oneline -3'
probe "$NOVERDICT" 0 'echo hello'
probe "$NOVERDICT" 0 'go test ./...'

if [ "$fails" -ne 0 ]; then
  echo "adversary-gate-probe: $fails probe(s) FAILED"
  exit 1
fi
echo "adversary-gate-probe: all probes hold."
