#!/bin/bash
# scripts/adversary-gate-probe.sh — the probe table of the push gate
# (R12-H8, B4; hardened after the adversary's train pass). Runs on
# every `make quality`. It exercises BOTH doors against REAL fixtures:
#
#   Fixtures are real git repositories (git init + one commit), so the
#   check's freshness branch runs for real — a temp dir without a repo
#   would make HEAD unreadable and, before the cure, "fresh by default".
#   - NOVERDICT: a repo with no verdict file.
#   - FRESH:     a repo whose verdict (first line exactly VETO LEVANTADO)
#                is newer than HEAD.
#   - STALE:     the same verdict, mtime forced BEFORE HEAD's commit.
#   - VETOED:    first line "VETO MANTENIDO (el VETO LEVANTADO anterior
#                queda revocado)" — a substring match would authorize it.
#   - NOCHECK:   FRESH, but the check script removed.
#
#   Door 1, the session hook (.claude/hooks/adversary-gate.sh), fed the
#   PreToolUse JSON; exact exit codes demanded.
#   Door 2, git's pre-push (.githooks/pre-push) installed in the fixture
#   repos and fired by a REAL `git push` to a local bare remote.
set -u
ROOT=$(git rev-parse --show-toplevel 2>/dev/null || dirname "$(dirname "$(readlink -f "$0")")")
HOOK="$ROOT/.claude/hooks/adversary-gate.sh"
command -v jq >/dev/null || { echo "adversary-gate-probe: jq is required (the hook uses it)"; exit 1; }
[ -x "$HOOK" ] || { echo "adversary-gate-probe: hook not executable: $HOOK"; exit 1; }
[ -x "$ROOT/scripts/adversary-gate-check.sh" ] || { echo "adversary-gate-probe: check script not executable"; exit 1; }
[ -x "$ROOT/.githooks/pre-push" ] || { echo "adversary-gate-probe: pre-push hook not executable"; exit 1; }

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
fails=0

# fixture <name>: a real repo with one commit, the check script and the
# pre-push hook installed exactly as `make install-hooks` does.
fixture() {
  local dir="$TMP/$1"
  mkdir -p "$dir/scripts" "$dir/.githooks"
  cp "$ROOT/scripts/adversary-gate-check.sh" "$dir/scripts/"
  cp "$ROOT/.githooks/pre-push" "$dir/.githooks/"
  # `git init -b` needs git 2.28; the house git is 2.27, so the branch
  # is named through symbolic-ref instead.
  git -C "$dir" init -q
  git -C "$dir" symbolic-ref HEAD refs/heads/probe
  git -C "$dir" -c user.name=probe -c user.email=probe@example.invalid \
    -c commit.gpgsign=false commit -q --allow-empty -m "probe fixture"
  ln -sf ../../.githooks/pre-push "$dir/.git/hooks/pre-push"
  echo "$dir"
}
verdict() { # $1 = dir, $2 = first line
  mkdir -p "$1/.claude/adversary"
  printf '%s\n\nprobe fixture\n' "$2" > "$1/.claude/adversary/last-verdict.md"
}

NOVERDICT=$(fixture noverdict)
FRESH=$(fixture fresh);   verdict "$FRESH" "VETO LEVANTADO"
STALE=$(fixture stale);   verdict "$STALE" "VETO LEVANTADO"
touch -t 200001010000 "$STALE/.claude/adversary/last-verdict.md"
VETOED=$(fixture vetoed); verdict "$VETOED" "VETO MANTENIDO (el VETO LEVANTADO anterior queda revocado)"
NOCHECK=$(fixture nocheck); verdict "$NOCHECK" "VETO LEVANTADO"; rm "$NOCHECK/scripts/adversary-gate-check.sh"

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

echo "--- door 1, no verdict recorded: every push shape must be blocked (2)"
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

echo "--- door 1, fresh VETO LEVANTADO: plain pushes pass (0), hatches blocked in any spelling (2)"
probe "$FRESH" 0 'git push origin master'
probe "$FRESH" 0 'if ! git push origin ensayo; then echo no; fi'
probe "$FRESH" 2 'git push --no-verify origin master'
probe "$FRESH" 2 'git push --no-verif origin master'
probe "$FRESH" 2 'git push --no-ver origin master'
probe "$FRESH" 2 'git -c core.hooksPath=/dev/null push origin master'
probe "$FRESH" 2 'git -c core.hookspath=/dev/null pus\h origin master'
probe "$FRESH" 2 'git -c CORE.HOOKSPATH=/dev/null commit -m x'
probe "$FRESH" 2 'git commit --no-verify -m x'

echo "--- door 1, the decision itself: stale (2), vetoed (2), check script gone (2)"
probe "$STALE" 2 'git push origin master'
probe "$VETOED" 2 'git push origin master'
probe "$NOCHECK" 2 'git push origin master'

echo "--- door 1, negatives: no push, no hatch (0)"
probe "$NOVERDICT" 0 'git commit -m "wip"'
probe "$NOVERDICT" 0 'git log --oneline -3'
probe "$NOVERDICT" 0 'echo hello'
probe "$NOVERDICT" 0 'go test ./...'

# Door 2: a REAL push from each fixture to a local bare remote; the
# installed pre-push hook decides. Expected: blocked (push exit != 0
# and the BLOCKED line) or allowed (push exit 0).
BARE="$TMP/remote.git"; git init -q --bare "$BARE"
push_probe() { # $1 = dir, $2 = expected: blocked|allowed
  local out code
  out=$(git -C "$1" push -q "$BARE" probe 2>&1); code=$?
  case "$2" in
    blocked) if [ "$code" -eq 0 ] || ! echo "$out" | grep -q "BLOCKED by adversary gate"; then
               printf 'FAIL  pre-push %s: expected BLOCKED, got exit %s\n      %s\n' "$(basename "$1")" "$code" "$out"; fails=$((fails + 1))
             else printf 'ok    pre-push %s blocked\n' "$(basename "$1")"; fi ;;
    allowed) if [ "$code" -ne 0 ]; then
               printf 'FAIL  pre-push %s: expected allowed, got exit %s\n      %s\n' "$(basename "$1")" "$code" "$out"; fails=$((fails + 1))
             else printf 'ok    pre-push %s allowed\n' "$(basename "$1")"; fi ;;
  esac
}
echo "--- door 2, git's pre-push fired by a real push to a local bare remote"
push_probe "$NOVERDICT" blocked
push_probe "$STALE" blocked
push_probe "$VETOED" blocked
push_probe "$NOCHECK" blocked
push_probe "$FRESH" allowed

if [ "$fails" -ne 0 ]; then
  echo "adversary-gate-probe: $fails probe(s) FAILED"
  exit 1
fi
echo "adversary-gate-probe: all probes hold."
