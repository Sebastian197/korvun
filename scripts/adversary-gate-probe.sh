#!/bin/bash
# scripts/adversary-gate-probe.sh — the probe table of the push gate
# (R12-H8, B4; hardened after the adversary's two train passes). Runs
# on every `make quality`. It exercises BOTH doors against REAL fixtures
# and demands the exact exit code AND, where the class matters, the
# exact BLOCKED reason (an exit 2 from a crashed exec is not the gate).
#
#   Fixtures are real git repositories (git init + one commit), so the
#   check's freshness branch runs for real.
#   - NOVERDICT: no verdict file.
#   - FRESH:     verdict (first line exactly VETO LEVANTADO) newer than HEAD.
#   - STALE:     same verdict, mtime forced BEFORE HEAD's commit.
#   - VETOED:    first line "VETO MANTENIDO (el VETO LEVANTADO anterior
#                queda revocado)" — a substring match would authorize it.
#   - NOCHECK:   FRESH, but the check script removed.
#
#   Door 1, the session hook (.claude/hooks/adversary-gate.sh), fed the
#   PreToolUse JSON. Door 2, git's pre-push (.githooks/pre-push) installed
#   in the fixture repos and fired by a REAL `git push` to a local bare
#   remote. Plus two tooling probes: the check under an emulated GNU
#   `stat` (the P2-A class: `-f` prints and fails) must still block the
#   stale fixture; the hook with jq absent from PATH must block.
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

probe() { # $1 = project dir, $2 = expected exit, $3 = command shape, [$4 = required substring of the output]
  local out code
  out=$(jq -cn --arg c "$3" '{tool_input:{command:$c}}' | CLAUDE_PROJECT_DIR="$1" "$HOOK" 2>&1)
  code=$?
  if [ "$code" -ne "$2" ] || { [ -n "${4:-}" ] && ! echo "$out" | grep -qF -- "$4"; }; then
    printf 'FAIL  expected %s%s got %s  %s\n      %s\n' "$2" "${4:+ + \"$4\"}" "$code" "$3" "$out"
    fails=$((fails + 1))
  else
    printf 'ok    exit %s  %s\n' "$code" "$3"
  fi
}

echo "--- door 1, no verdict recorded: every push shape must be blocked (2) with the no-verdict reason"
while IFS= read -r shape; do probe "$NOVERDICT" 2 "$shape" "no internal-adversary verdict recorded"; done <<'EOF'
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
Git push origin master
git pu''sh origin master
git pus\h origin master
git pu$'s'h origin master
git pu$"s"h origin master
git send-pack origin master
C:\tools\git\bin\git.exe push origin master
EOF
probe "$NOVERDICT" 2 'git push --no-verify origin master' 'skip git'"'"'s pre-push hook'
probe "$NOVERDICT" 2 'git -c core.hooksPath=/dev/null push origin master' 'skip git'"'"'s pre-push hook'

echo "--- door 1, fresh VETO LEVANTADO: plain pushes pass (0), hatches blocked in any spelling (2)"
probe "$FRESH" 0 'git push origin master'
probe "$FRESH" 0 'if ! git push origin ensayo; then echo no; fi'
HATCH='skip git'"'"'s pre-push hook'
probe "$FRESH" 2 'git push --no-verify origin master' "$HATCH"
probe "$FRESH" 2 'git push --no-verif origin master' "$HATCH"
probe "$FRESH" 2 'git push --no-ver origin master' "$HATCH"
probe "$FRESH" 2 'git -c core.hooksPath=/dev/null push origin master' "$HATCH"
probe "$FRESH" 2 'git -c core.hookspath=/dev/null pus\h origin master' "$HATCH"
probe "$FRESH" 2 'git -c CORE.HOOKSPATH=/dev/null commit -m x' "$HATCH"
probe "$FRESH" 2 'git commit --no-verify -m x' "$HATCH"
probe "$FRESH" 2 'Git push --no-verify origin ensayo' "$HATCH"
probe "$FRESH" 2 'git pu'"''"'sh --no-"verify" origin ensayo' "$HATCH"
probe "$FRESH" 2 'git push --no-$'"'"'v'"'"'erify origin master' "$HATCH"
probe "$FRESH" 2 'git push --no-$"v"erify origin master' "$HATCH"

echo "--- door 1, the decision itself: stale (2), vetoed (2), check script gone (2), each by its reason"
probe "$STALE" 2 'git push origin master' 'predates HEAD'
probe "$VETOED" 2 'git push origin master' 'not exactly VETO LEVANTADO'
probe "$NOCHECK" 2 'git push origin master' 'missing or not executable'

echo "--- door 1, negatives: no push, no hatch (0)"
probe "$NOVERDICT" 0 'git commit -m "wip"'
probe "$NOVERDICT" 0 'git log --oneline -3'
probe "$NOVERDICT" 0 'echo hello'
probe "$NOVERDICT" 0 'go test ./...'

echo "--- tooling: the check under an emulated GNU stat (-f prints and fails) still blocks STALE (2)"
FAKEBIN="$TMP/fakebin"; mkdir -p "$FAKEBIN"
REALSTAT=$(command -v stat)
cat > "$FAKEBIN/stat" <<EOF
#!/bin/bash
# GNU dialect emulation: -f is --file-system (no argument): print a
# multi-line block to stdout and fail on the '%m' operand; -c %Y works.
case "\$1" in
  -f) printf '  File: "%s"\n    ID: 1000 Namelen: 255 Type: apfs\n' "\$3"; echo "stat: cannot read file system information for '%m'" >&2; exit 1 ;;
  # The -c arm must answer on ANY host: GNU stat has -c, BSD stat has
  # -f %m — try the host's real dialects in turn (the emulation is not
  # tied to the BSD host it was written on; adversary third pass P2-1).
  -c) "$REALSTAT" -c %Y "\$3" 2>/dev/null || exec "$REALSTAT" -f %m "\$3" ;;
  *)  exec "$REALSTAT" "\$@" ;;
esac
EOF
chmod +x "$FAKEBIN/stat"
out=$(PATH="$FAKEBIN:$PATH" bash "$ROOT/scripts/adversary-gate-check.sh" "$STALE" 2>&1); code=$?
if [ "$code" -ne 2 ] || ! echo "$out" | grep -qF "predates HEAD"; then
  printf 'FAIL  gnu-stat emulation: expected 2 + "predates HEAD", got %s\n      %s\n' "$code" "$out"; fails=$((fails + 1))
else echo "ok    exit 2  check under emulated GNU stat blocks STALE by its reason"; fi
out=$(PATH="$FAKEBIN:$PATH" bash "$ROOT/scripts/adversary-gate-check.sh" "$FRESH" 2>&1); code=$?
if [ "$code" -ne 0 ]; then
  printf 'FAIL  gnu-stat emulation: FRESH expected 0, got %s\n      %s\n' "$code" "$out"; fails=$((fails + 1))
else echo "ok    exit 0  check under emulated GNU stat still authorizes FRESH"; fi

# A stat that SUCCEEDS with garbage on every dialect: only the integer
# validation stands between it and "[ garbage -lt N ]" — which errors,
# reads as false, and would authorize.
GARBAGE="$TMP/garbagebin"; mkdir -p "$GARBAGE"
printf '#!/bin/bash\nprintf "  File: x\\n    ID: 1000 Namelen: 255\\n"\nexit 0\n' > "$GARBAGE/stat"; chmod +x "$GARBAGE/stat"
out=$(PATH="$GARBAGE:$PATH" bash "$ROOT/scripts/adversary-gate-check.sh" "$STALE" 2>&1); code=$?
if [ "$code" -ne 2 ] || ! echo "$out" | grep -qF "as an integer"; then
  printf 'FAIL  garbage-stat: expected 2 + "as an integer", got %s\n      %s\n' "$code" "$out"; fails=$((fails + 1))
else echo "ok    exit 2  check under a garbage-printing stat blocks by the integer rule"; fi

# A git that prints non-digits with exit 0 before the epoch (the
# log.showSignature shape): HEAD's time must be refused as unreadable,
# never compared as an integer expression that errors into "fresh".
FAKEGIT="$TMP/fakegit"; mkdir -p "$FAKEGIT"
REALGIT=$(command -v git)
cat > "$FAKEGIT/git" <<EOF
#!/bin/bash
for a in "\$@"; do
  if [ "\$a" = "log" ]; then printf 'gpg: Good signature from "probe"\n%s\n' "\$("$REALGIT" "\$@")"; exit 0; fi
done
exec "$REALGIT" "\$@"
EOF
chmod +x "$FAKEGIT/git"
out=$(PATH="$FAKEGIT:$PATH" bash "$ROOT/scripts/adversary-gate-check.sh" "$FRESH" 2>&1); code=$?
if [ "$code" -ne 2 ] || ! echo "$out" | grep -qF "cannot read HEAD's commit time"; then
  printf 'FAIL  garbage-git: expected 2 + "cannot read HEAD'"'"'s commit time", got %s\n      %s\n' "$code" "$out"; fails=$((fails + 1))
else echo "ok    exit 2  check under a git that prints signature lines blocks by the HEAD-time rule"; fi

echo "--- tooling: the hook with jq absent from PATH blocks every command (2)"
NOJQ="$TMP/nojq"; mkdir -p "$NOJQ"
for tool in bash grep tr printf echo cat git head stat; do
  p=$(command -v "$tool" 2>/dev/null) && [ -n "$p" ] && ln -sf "$p" "$NOJQ/$tool"
done
out=$(printf '{"tool_input":{"command":"echo hello"}}' | PATH="$NOJQ" CLAUDE_PROJECT_DIR="$FRESH" bash "$HOOK" 2>&1); code=$?
if [ "$code" -ne 2 ] || ! echo "$out" | grep -qF "jq is missing"; then
  printf 'FAIL  no-jq: expected 2 + "jq is missing", got %s\n      %s\n' "$code" "$out"; fails=$((fails + 1))
else echo "ok    exit 2  hook without jq fails closed"; fi
# Only bash, jq and printf on PATH: without grep/tr the shape cannot be
# read and the hook must block, never fall through to exit 0.
ONLYJQ="$TMP/onlyjq"; mkdir -p "$ONLYJQ"
for tool in bash jq printf; do
  p=$(command -v "$tool" 2>/dev/null) && [ -n "$p" ] && ln -sf "$p" "$ONLYJQ/$tool"
done
out=$(printf '{"tool_input":{"command":"git push --no-verify origin master"}}' | PATH="$ONLYJQ" CLAUDE_PROJECT_DIR="$STALE" bash "$HOOK" 2>&1); code=$?
if [ "$code" -ne 2 ] || ! echo "$out" | grep -qE "(grep|tr) is missing"; then
  printf 'FAIL  no-grep-tr: expected 2 + "grep/tr is missing", got %s\n      %s\n' "$code" "$out"; fails=$((fails + 1))
else echo "ok    exit 2  hook without grep/tr fails closed"; fi
out=$(printf 'not json' | CLAUDE_PROJECT_DIR="$FRESH" bash "$HOOK" 2>&1); code=$?
if [ "$code" -ne 2 ] || ! echo "$out" | grep -qF "not parseable JSON"; then
  printf 'FAIL  bad-json: expected 2 + "not parseable JSON", got %s\n      %s\n' "$code" "$out"; fails=$((fails + 1))
else echo "ok    exit 2  hook on unparseable input fails closed"; fi

# Door 2: a REAL push from each fixture to a local bare remote.
BARE="$TMP/remote.git"; git init -q --bare "$BARE"
push_probe() { # $1 = dir, $2 = blocked|allowed, [$3 = required substring]
  local out code
  out=$(git -C "$1" push -q "$BARE" probe 2>&1); code=$?
  case "$2" in
    blocked) if [ "$code" -eq 0 ] || ! echo "$out" | grep -q "BLOCKED by adversary gate" || { [ -n "${3:-}" ] && ! echo "$out" | grep -qF -- "$3"; }; then
               printf 'FAIL  pre-push %s: expected BLOCKED%s, got exit %s\n      %s\n' "$(basename "$1")" "${3:+ + \"$3\"}" "$code" "$out"; fails=$((fails + 1))
             else printf 'ok    pre-push %s blocked\n' "$(basename "$1")"; fi ;;
    allowed) if [ "$code" -ne 0 ]; then
               printf 'FAIL  pre-push %s: expected allowed, got exit %s\n      %s\n' "$(basename "$1")" "$code" "$out"; fails=$((fails + 1))
             else printf 'ok    pre-push %s allowed\n' "$(basename "$1")"; fi ;;
  esac
}
echo "--- door 2, git's pre-push fired by a real push to a local bare remote"
push_probe "$NOVERDICT" blocked "no internal-adversary verdict recorded"
push_probe "$STALE" blocked "predates HEAD"
push_probe "$VETOED" blocked "not exactly VETO LEVANTADO"
push_probe "$NOCHECK" blocked "missing or not executable"
push_probe "$FRESH" allowed

if [ "$fails" -ne 0 ]; then
  echo "adversary-gate-probe: $fails probe(s) FAILED"
  exit 1
fi
echo "adversary-gate-probe: all probes hold."
