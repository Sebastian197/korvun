#!/bin/bash
# scripts/adversary-gate-probe.sh — the probe table of the push gate
# (R13-G7, D9 option (i): the marker counts ONLY committed). Runs on
# every `make quality` and, in CI, on the Linux and macOS runners. It
# exercises the check at THREE levels against REAL git fixtures and
# demands the exact exit code AND the exact BLOCKED reason:
#   - check-direct: `scripts/adversary-gate-check.sh <root> <sha>` (the
#     argument walls, the tool loop, fake gits, the ROOT wall — shapes
#     the doors never produce);
#   - door 1: the session hook (.claude/hooks/adversary-gate.sh) fed the
#     PreToolUse JSON, judging the HEAD of CLAUDE_PROJECT_DIR;
#   - door 2: git's pre-push (.githooks/pre-push) installed in the
#     fixture and fired by a REAL `git push` into a bare remote, judging
#     every pushed sha from git's stdin.
# The contract the fixtures pin is the check's header (§7 of the R13
# paper): argument presence → tools → ROOT wall → the sha resolved
# through rev-parse --verify --end-of-options <arg>^{commit} → exactly
# one parent → the marker entry by ls-tree --full-tree (mode judged) →
# the blob by object id (size, then content) → the first line exactly
# `VETO LEVANTADO <40-hex>` → the hex equals the parent → diff-tree
# exactly the marker path.
# Fixture names and expectations are the R13 paper's §6 G7 table;
# outcomes marked "captured" there are observed here, not assumed.
set -u
ROOT=$(git rev-parse --show-toplevel 2>/dev/null || dirname "$(dirname "$(readlink -f "$0")")")
HOOK="$ROOT/.claude/hooks/adversary-gate.sh"
CHECK="$ROOT/scripts/adversary-gate-check.sh"
PREPUSH="$ROOT/.githooks/pre-push"
MARKER=".claude/adversary/last-verdict.md"
command -v jq >/dev/null || { echo "adversary-gate-probe: jq is required (the hook uses it)"; exit 1; }
[ -x "$HOOK" ] || { echo "adversary-gate-probe: hook not executable: $HOOK"; exit 1; }
[ -x "$CHECK" ] || { echo "adversary-gate-probe: check script not executable"; exit 1; }
[ -x "$PREPUSH" ] || { echo "adversary-gate-probe: pre-push hook not executable"; exit 1; }
# m-replace's red needs replace refs ACTIVE: both switches captured off.
[ -z "${GIT_NO_REPLACE_OBJECTS:-}" ] || { echo "adversary-gate-probe: GIT_NO_REPLACE_OBJECTS is set — the REPLACE fixture cannot be judged"; exit 1; }
if [ "$(git config --get core.useReplaceRefs 2>/dev/null)" = "false" ]; then echo "adversary-gate-probe: core.useReplaceRefs=false — the REPLACE fixture cannot be judged"; exit 1; fi

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
fails=0
ZERO40=0000000000000000000000000000000000000000
GITENV=(-c user.name=probe -c user.email=probe@example.invalid -c commit.gpgsign=false)

pass() { printf 'ok    %s\n' "$1"; }
fail() { printf 'FAIL  %s\n      %s\n' "$1" "$2"; fails=$((fails + 1)); }

# ---- fixture builders (real repositories) ----------------------------------
mkfix() { # $1 = name → prints the dir; branch `probe`, hooks installed, scripts copied
  local dir="$TMP/$1"
  mkdir -p "$dir/scripts" "$dir/.githooks"
  cp "$CHECK" "$dir/scripts/"
  cp "$PREPUSH" "$dir/.githooks/"
  git -C "$dir" init -q
  git -C "$dir" symbolic-ref HEAD refs/heads/probe
  ln -sf ../../.githooks/pre-push "$dir/.git/hooks/pre-push"
  echo "$dir"
}
gcommit() { git -C "$1" "${GITENV[@]}" commit -q "${@:2}"; }
code() { # $1 = dir, $2 = file content marker → commits a code change, prints the sha
  printf 'code %s\n' "$2" >> "$1/code.txt"
  git -C "$1" add code.txt
  gcommit "$1" -m "code $2"
  git -C "$1" rev-parse HEAD
}
marker_text() { printf 'VETO LEVANTADO %s\n\nprobe fixture: recorded by hand\n' "$1"; }
marker() { # $1 = dir, $2 = sha named (or raw content when $3 = raw) → commits the marker, prints the sha
  mkdir -p "$1/.claude/adversary"
  if [ "${3:-}" = raw ]; then printf '%s' "$2" > "$1/$MARKER"; else marker_text "$2" > "$1/$MARKER"; fi
  git -C "$1" add -f "$MARKER"
  gcommit "$1" -m "marker"
  git -C "$1" rev-parse HEAD
}
parentok() { # $1 = name → dir with C0 ← M(names C0); prints dir
  local d; d=$(mkfix "$1"); code "$d" c0 >/dev/null; marker "$d" "$(git -C "$d" rev-parse HEAD)" >/dev/null; echo "$d"
}
# marker committed as an index entry of a given mode from a given blob (no working-tree file)
marker_entry() { # $1 = dir, $2 = mode, $3 = object id → commits, prints sha
  git -C "$1" update-index --add --cacheinfo "$2,$3,$MARKER"
  gcommit "$1" -m "marker entry $2"
  git -C "$1" rev-parse HEAD
}

# ---- probes at the three levels --------------------------------------------
check_probe() { # $1 = label, $2 = expected exit, $3 = required substring (or ""), then the args
  local label=$1 want=$2 sub=$3; shift 3
  local out code
  out=$(bash "$CHECK" "$@" 2>&1); code=$?
  if [ "$code" -ne "$want" ] || { [ -n "$sub" ] && ! printf '%s' "$out" | grep -qF -- "$sub"; }; then
    fail "check-direct $label: expected $want${sub:+ + \"$sub\"}, got $code" "$out"
  else pass "check-direct $label (exit $code)"; fi
}
door1() { # $1 = label, $2 = project dir, $3 = expected exit, $4 = command, [$5 = substring]
  local out code
  out=$(jq -cn --arg c "$4" '{tool_input:{command:$c}}' | CLAUDE_PROJECT_DIR="$2" "$HOOK" 2>&1); code=$?
  if [ "$code" -ne "$3" ] || { [ -n "${5:-}" ] && ! printf '%s' "$out" | grep -qF -- "$5"; }; then
    fail "door 1 $1: expected $3${5:+ + \"$5\"}, got $code  [$4]" "$out"
  else pass "door 1 $1 (exit $code)  [$4]"; fi
}
newbare() { git init -q --bare "$TMP/$1.git"; echo "$TMP/$1.git"; }
door2() { # $1 = label, $2 = dir, $3 = blocked|allowed, $4 = bare, $5 = refspec(s) (space separated), [$6 = substring]
  local out code
  # shellcheck disable=SC2086
  out=$(git -C "$2" push -q "$4" $5 2>&1); code=$?
  case "$3" in
    blocked) if [ "$code" -ne 1 ] || ! printf '%s' "$out" | grep -q "BLOCKED by adversary gate" || { [ -n "${6:-}" ] && ! printf '%s' "$out" | grep -qF -- "$6"; }; then
               fail "door 2 $1: expected BLOCKED (git exit 1)${6:+ + \"$6\"}, got exit $code" "$out"
             else pass "door 2 $1 blocked (git exit 1)"; fi ;;
    allowed) if [ "$code" -ne 0 ]; then fail "door 2 $1: expected allowed, got exit $code" "$out"
             else pass "door 2 $1 allowed"; fi ;;
  esac
}
# A fake git that locates the SUBCOMMAND after the global options
# (--no-replace-objects, -C <dir>) — never $1 — and misbehaves for ONE
# subcommand (optionally only when a flag is present); everything else
# goes to the real git.
REALGIT=$(command -v git)
fakegit() { # $1 = dir name, $2 = subcommand, $3 = flag ("" = any), $4 = behavior: fail | print:<text> | empty
  local d="$TMP/$1"; mkdir -p "$d"
  cat > "$d/git" <<EOF
#!/bin/bash
sub=""; skip=0; hasflag=0
for a in "\$@"; do
  if [ "\$skip" = 1 ]; then skip=0; continue; fi
  case "\$a" in
    --no-replace-objects) continue ;;
    -C) skip=1; continue ;;
    -*) if [ -n "\$sub" ] && [ "\$a" = "$3" ]; then hasflag=1; fi; continue ;;
  esac
  if [ -z "\$sub" ]; then sub="\$a"; fi
done
if [ "\$sub" = "$2" ] && { [ -z "$3" ] || [ "\$hasflag" = 1 ]; }; then
  case "$4" in
    fail) echo "fatal: probe fake git: $2 fails on purpose" >&2; exit 128 ;;
    print:*) printf '%s\n' "${4#print:}"; exit 0 ;;
    empty) exit 0 ;;
  esac
fi
exec "$REALGIT" "\$@"
EOF
  chmod +x "$d/git"; echo "$d"
}

# ============================================================================
echo "--- fixtures"
POK=$(parentok parentok); J_POK=$(git -C "$POK" rev-parse HEAD); C0_POK=$(git -C "$POK" rev-parse HEAD~1)
TAGD=$(parentok tag); git -C "$TAGD" "${GITENV[@]}" tag -a -m "release probe" t1
SMUG=$(mkfix smuggle); code "$SMUG" c0 >/dev/null
  mkdir -p "$SMUG/.claude/adversary"; marker_text "$(git -C "$SMUG" rev-parse HEAD)" > "$SMUG/$MARKER"; printf 'code smuggled\n' >> "$SMUG/code.txt"
  git -C "$SMUG" add -f "$MARKER" code.txt; gcommit "$SMUG" -m "marker and code"
DIRTY=$(mkfix dirty); C0_DIRTY=$(code "$DIRTY" c0); Y=1111111111111111111111111111111111111111
  marker "$DIRTY" "$Y" >/dev/null; marker_text "$C0_DIRTY" > "$DIRTY/$MARKER"   # working tree names the true parent; the blob names Y
NOBLOB=$(mkfix noblob); code "$NOBLOB" c0 >/dev/null; K_NOBLOB=$(code "$NOBLOB" k)
# The four-commit chain: C0 ← M1(names C0) ← C2(code) ← M(names C2). HEAD = M.
CHAIN=$(mkfix chain); C0_CH=$(code "$CHAIN" c0); M1_CH=$(marker "$CHAIN" "$C0_CH"); C2_CH=$(code "$CHAIN" c2); M_CH=$(marker "$CHAIN" "$C2_CH")
OLDSHA=$(mkfix oldsha); C0_OS=$(code "$OLDSHA" c0); M1_OS=$(marker "$OLDSHA" "$C0_OS"); C2_OS=$(code "$OLDSHA" c2); git -C "$OLDSHA" reset -q --hard "$C2_OS"   # HEAD = C2
GP=$(mkfix grandparent); C0_GP=$(code "$GP" c0); C1_GP=$(code "$GP" c1); marker "$GP" "$C0_GP" >/dev/null
MERGE=$(mkfix merge); C0_MG=$(code "$MERGE" c0)
  E_MG=$(git -C "$MERGE" "${GITENV[@]}" commit-tree "$(git -C "$MERGE" rev-parse HEAD^{tree})" -p "$C0_MG" -m "empty second parent")
  mkdir -p "$MERGE/.claude/adversary"; marker_text "$C0_MG" > "$MERGE/$MARKER"; git -C "$MERGE" add -f "$MARKER"
  T_MG=$(git -C "$MERGE" write-tree); J_MG=$(git -C "$MERGE" "${GITENV[@]}" commit-tree "$T_MG" -p "$C0_MG" -p "$E_MG" -m "merge marker"); git -C "$MERGE" update-ref refs/heads/probe "$J_MG"; git -C "$MERGE" reset -q --hard
ROOTF=$(mkfix rootfix); mkdir -p "$ROOTF/.claude/adversary"; marker_text "$Y" > "$ROOTF/$MARKER"; printf 'code\n' > "$ROOTF/code.txt"; git -C "$ROOTF" add -f "$MARKER" code.txt; gcommit "$ROOTF" -m "root with marker"
firstline_fix() { # $1 = name, $2 = raw marker content → dir (C0 ← M(raw))
  local d; d=$(mkfix "$1"); local c0; c0=$(code "$d" c0); local content; content=$(printf "$2" "$c0"); marker "$d" "$content" raw >/dev/null; echo "$d"
}
NOSHA=$(firstline_fix nosha 'VETO LEVANTADO\n\nno sha at all\n')
SHORTSHA=$(mkfix shortsha); c=$(code "$SHORTSHA" c0); marker "$SHORTSHA" "$(printf 'VETO LEVANTADO %s\n' "${c:0:7}")" raw >/dev/null
PREFIX40=$(mkfix prefix40); c=$(code "$PREFIX40" c0); marker "$PREFIX40" "$(printf 'VETO LEVANTADO %s%s\n' "${c:0:7}" "abcdef0123456789abcdef0123456789a")" raw >/dev/null
TRAILWS=$(firstline_fix trailws 'VETO LEVANTADO %s \n')
LEADWS=$(firstline_fix leadws ' VETO LEVANTADO %s\n')
BOM=$(firstline_fix bom '\xEF\xBB\xBFVETO LEVANTADO %s\n')
UPPER=$(mkfix upperhex); c=$(code "$UPPER" c0); marker "$UPPER" "$(printf 'VETO LEVANTADO %s\n' "$(printf '%s' "$c" | tr 'a-f' 'A-F')")" raw >/dev/null
EMPTYFIRST=$(firstline_fix emptyfirstline '\nVETO LEVANTADO %s\n')
# NEWLINESONLY is written straight to the file: a $(...) capture would strip the trailing newlines and leave an EMPTY blob.
NLONLY=$(mkfix newlinesonly); code "$NLONLY" c0 >/dev/null; mkdir -p "$NLONLY/.claude/adversary"; printf '\n\n' > "$NLONLY/$MARKER"; git -C "$NLONLY" add -f "$MARKER"; gcommit "$NLONLY" -m "marker newlines only"
REVOKED=$(firstline_fix revoked 'VETO MANTENIDO — anterior VETO LEVANTADO %s\n')
EMPTYBLOB=$(mkfix emptyblob); code "$EMPTYBLOB" c0 >/dev/null; marker "$EMPTYBLOB" "" raw >/dev/null
BIGBLOB=$(mkfix bigblob); c=$(code "$BIGBLOB" c0); marker "$BIGBLOB" "$(printf 'VETO LEVANTADO %s\n%s\n' "$c" "$(head -c 200000 /dev/zero | tr '\0' 'x')")" raw >/dev/null
NONL=$(mkfix nonewline); c=$(code "$NONL" c0); marker "$NONL" "$(printf 'VETO LEVANTADO %s' "$c")" raw >/dev/null
CRLF=$(mkfix crlf); c=$(code "$CRLF" c0); printf 'VETO LEVANTADO %s\r\n' "$c" > "$TMP/crlf.txt"
  b=$(git -C "$CRLF" hash-object -w --no-filters "$TMP/crlf.txt"); marker_entry "$CRLF" 100644 "$b" >/dev/null
SYML=$(mkfix notblob-symlink); c=$(code "$SYML" c0); b=$(printf 'VETO LEVANTADO %s' "$c" | git -C "$SYML" hash-object -w --stdin); marker_entry "$SYML" 120000 "$b" >/dev/null
DIRF=$(mkfix notblob-dir); c=$(code "$DIRF" c0); mkdir -p "$DIRF/$MARKER"; marker_text "$c" > "$DIRF/$MARKER/inner"; git -C "$DIRF" add -f "$MARKER/inner"; gcommit "$DIRF" -m "marker dir"
GITLINK=$(mkfix notblob-gitlink); c=$(code "$GITLINK" c0); marker_entry "$GITLINK" 160000 "$c" >/dev/null
NOCHECKD=$(parentok nocheck); rm "$NOCHECKD/scripts/adversary-gate-check.sh"
# REPLACE: C0 ← K (no marker) and, as a sibling, C0 ← M′ (marker naming C0); `git replace K M′`.
REPL=$(mkfix replace); C0_RP=$(code "$REPL" c0); K_RP=$(code "$REPL" k)
  git -C "$REPL" checkout -q -b sibling "$C0_RP"; MP_RP=$(marker "$REPL" "$C0_RP"); git -C "$REPL" checkout -q probe
  git -C "$REPL" replace "$K_RP" "$MP_RP"
# Hypotheses via mktree: DUPENTRY, UNKNOWNMODE, MODE664 — captured, each becomes a fixture only if git materializes it.
MK=$(mkfix mktree); C0_MK=$(code "$MK" c0); B_MK=$(marker_text "$C0_MK" | git -C "$MK" hash-object -w --stdin)
mktree_fix() { # $1 = entries → prints commit sha or "" if mktree refuses
  local t; t=$(printf '%b' "$1" | git -C "$MK" mktree 2>/dev/null) || { echo ""; return; }
  git -C "$MK" "${GITENV[@]}" commit-tree "$t" -p "$C0_MK" -m "mktree probe" 2>/dev/null
}
# ls-tree entries are per path component: a tree named .claude/adversary holding the leaf; build nested trees.
leaf_tree() { printf '%b' "$1" | git -C "$MK" mktree 2>/dev/null; }
nest() { # $1 = leaf tree (the adversary/ directory's content) → full tree sha: .claude/ → adversary/ → leaf, plus code.txt from C0
  local adv codeblob; adv=$(printf '040000 tree %s\tadversary\n' "$1" | git -C "$MK" mktree) || return 1
  codeblob=$(git -C "$MK" rev-parse "$C0_MK:code.txt")
  # mktree wants its entries in tree order: ".claude" sorts before "code.txt".
  printf '040000 tree %s\t.claude\n100644 blob %s\tcode.txt\n' "$adv" "$codeblob" | git -C "$MK" mktree
}
mode_of() { git -C "$MK" ls-tree --full-tree "$1" -- "$MARKER" | head -1 | cut -c1-6; }
DUP_LEAF=$(leaf_tree "100644 blob $B_MK\tlast-verdict.md\n100644 blob $B_MK\tlast-verdict.md\n" || true)
UNK_LEAF=$(leaf_tree "000000 blob $B_MK\tlast-verdict.md\n" || true)
M664_LEAF=$(leaf_tree "100664 blob $B_MK\tlast-verdict.md\n" || true)
DUP_J=""; UNK_J=""; M664_J=""
[ -n "$DUP_LEAF" ] && DUP_J=$(git -C "$MK" "${GITENV[@]}" commit-tree "$(nest "$DUP_LEAF")" -p "$C0_MK" -m dup 2>/dev/null || true)
[ -n "$UNK_LEAF" ] && UNK_J=$(git -C "$MK" "${GITENV[@]}" commit-tree "$(nest "$UNK_LEAF")" -p "$C0_MK" -m unk 2>/dev/null || true)
[ -n "$M664_LEAF" ] && M664_J=$(git -C "$MK" "${GITENV[@]}" commit-tree "$(nest "$M664_LEAF")" -p "$C0_MK" -m m664 2>/dev/null || true)
echo "mktree hypotheses captured: DUPENTRY=${DUP_J:-refused} UNKNOWNMODE=${UNK_J:-refused} MODE664=${M664_J:-refused}$( [ -n "$M664_J" ] && printf ' (ls-tree mode %s)' "$(git -C "$MK" ls-tree --full-tree "$M664_J" -- "$MARKER" | cut -c1-6)")"

# ============================================================================
echo "--- check-direct: the argument walls, the tool loop, the ROOT wall"
check_probe NOROOT-none 2 "no root argument"
check_probe NOROOT-empty 2 "no root argument" "" "$J_POK"
# NOROOT from the fixture's own top level: a default to `.` or to show-toplevel (m-n7m2) would PASS here — observable.
out=$(cd "$POK" && bash "$CHECK" "" "$J_POK" 2>&1); code=$?
if [ "$code" -ne 2 ] || ! printf '%s' "$out" | grep -qF "no root argument"; then fail "check-direct NOROOT-from-top: expected 2 + \"no root argument\", got $code" "$out"; else pass "check-direct NOROOT-from-top (exit 2)"; fi
check_probe NOARG 2 "no sha argument" "$POK"
check_probe NOARG-empty 2 "no sha argument" "$POK" ""
check_probe BADARG 2 "not resolvable" "$POK" "not-a-sha"
check_probe BADARG-40 2 "not resolvable" "$POK" "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
check_probe NODIR 2 "root is not a directory" "$TMP/does-not-exist" "$J_POK"
NR="$TMP/notarepo"; mkdir -p "$NR"
out=$(cd "$TMP" && GIT_CEILING_DIRECTORIES="$(cd "$TMP" && pwd -P)" bash "$CHECK" "$NR" "$J_POK" 2>&1); code=$?
if [ "$code" -ne 2 ] || ! printf '%s' "$out" | grep -qF "git cannot name a top level for"; then fail "check-direct NOTAREPO: expected 2 + \"git cannot name a top level for\", got $code" "$out"; else pass "check-direct NOTAREPO (exit 2)"; fi
check_probe WRONGROOT 2 "root is not the repository top level" "$POK/scripts" "$J_POK"
ln -s "$POK" "$TMP/rootlink"
check_probe SYMLINKROOT 0 "" "$TMP/rootlink" "$J_POK"
check_probe TRAILSLASH 0 "" "$POK/" "$J_POK"
NOGIT="$TMP/nogit"; mkdir -p "$NOGIT"; for t in bash; do ln -sf "$(command -v $t)" "$NOGIT/$t"; done
out=$(PATH="$NOGIT" bash "$CHECK" "$POK" "$J_POK" 2>&1); code=$?
if [ "$code" -ne 2 ] || ! printf '%s' "$out" | grep -qF "tool missing: git"; then fail "check-direct NOTOOL-git: expected 2 + \"tool missing: git\", got $code" "$out"; else pass "check-direct NOTOOL-git (exit 2)"; fi
echo "--- check-direct: fake gits (the subcommand located after the global options)"
fake_probe() { # $1 = label, $2 = fakedir, $3 = expected exit, $4 = substring, $5 = root, $6 = sha
  local out code
  out=$(PATH="$2:$PATH" bash "$CHECK" "$5" "$6" 2>&1); code=$?
  if [ "$code" -ne "$3" ] || { [ -n "$4" ] && ! printf '%s' "$out" | grep -qF -- "$4"; }; then fail "check-direct $1: expected $3 + \"$4\", got $code" "$out"; else pass "check-direct $1 (exit $code)"; fi
}
fake_probe GITFAIL-REVLIST "$(fakegit fg-revlist rev-list "" fail)" 2 "parents unreadable" "$POK" "$J_POK"
fake_probe GITFAIL-LSTREE "$(fakegit fg-lstree ls-tree "" fail)" 2 "marker unreadable" "$POK" "$J_POK"
fake_probe GITFAIL-CATFILE "$(fakegit fg-catfile cat-file -s fail)" 2 "marker blob SIZE unreadable" "$POK" "$J_POK"
fake_probe GARBAGE-SIZE "$(fakegit fg-garbage cat-file -s print:abc)" 2 "marker blob SIZE unreadable" "$POK" "$J_POK"
fake_probe EMPTY-SIZE "$(fakegit fg-emptysize cat-file -s empty)" 2 "marker blob SIZE unreadable" "$POK" "$J_POK"
fake_probe GITFAIL-CATFILE-P "$(fakegit fg-catfilep cat-file -p fail)" 2 "marker blob CONTENT unreadable" "$POK" "$J_POK"
fake_probe EMPTY-CONTENT "$(fakegit fg-emptycontent cat-file -p empty)" 2 "first line is not exactly" "$POK" "$J_POK"
fake_probe GITFAIL-DIFFTREE "$(fakegit fg-difftree diff-tree "" fail)" 2 "diff unreadable" "$POK" "$J_POK"

echo "--- check-direct and door 2: the commit shapes"
BARE=$(newbare pok); door2 PARENTOK "$POK" allowed "$BARE" probe
check_probe PARENTOK 0 "" "$POK" "$J_POK"
door2 TAG "$TAGD" allowed "$(newbare tag)" t1
check_probe TAG-peeled 0 "" "$TAGD" t1
door2 SMUGGLE "$SMUG" blocked "$(newbare smuggle)" probe "carries more than the marker"
door2 DIRTY "$DIRTY" blocked "$(newbare dirty)" probe "records $Y"
door2 NOBLOB "$NOBLOB" blocked "$(newbare noblob)" "$K_NOBLOB:refs/heads/probe" "no verdict marker committed at $K_NOBLOB"
door2 OLDSHA "$OLDSHA" blocked "$(newbare oldsha)" probe "records $C0_OS"
door1 OLDSHA "$OLDSHA" 2 'git push origin probe' "records $C0_OS"
door2 GRANDPARENT "$GP" blocked "$(newbare gp)" probe "records $C0_GP"
door2 MERGE "$MERGE" blocked "$(newbare merge)" probe "is a merge"
door2 ROOT "$ROOTF" blocked "$(newbare rootfix)" probe "has no parent"
door2 NOSHA "$NOSHA" blocked "$(newbare nosha)" probe "first line is not exactly"
door2 SHORTSHA "$SHORTSHA" blocked "$(newbare shortsha)" probe "first line is not exactly"
door2 PREFIX40 "$PREFIX40" blocked "$(newbare prefix40)" probe "records"
door2 TRAILWS "$TRAILWS" blocked "$(newbare trailws)" probe "first line is not exactly"
door2 LEADWS "$LEADWS" blocked "$(newbare leadws)" probe "first line is not exactly"
door2 BOM "$BOM" blocked "$(newbare bom)" probe "first line is not exactly"
door2 UPPERHEX "$UPPER" blocked "$(newbare upperhex)" probe "first line is not exactly"
door2 EMPTYFIRSTLINE "$EMPTYFIRST" blocked "$(newbare emptyfirst)" probe "first line is not exactly"
door2 NEWLINESONLY "$NLONLY" blocked "$(newbare nlonly)" probe "first line is not exactly"
door2 REVOKED-WITH-SHA "$REVOKED" blocked "$(newbare revoked)" probe "first line is not exactly"
door2 EMPTYBLOB "$EMPTYBLOB" blocked "$(newbare emptyblob)" probe "empty verdict marker"
door2 BIGBLOB "$BIGBLOB" allowed "$(newbare bigblob)" probe
door2 NONEWLINE "$NONL" allowed "$(newbare nonewline)" probe
door2 CRLF "$CRLF" blocked "$(newbare crlf)" probe "first line is not exactly"
door2 NOTBLOB-SYMLINK "$SYML" blocked "$(newbare symlink)" probe "not a regular file (mode 120000)"
door2 NOTBLOB-DIR "$DIRF" blocked "$(newbare dirf)" probe "not a regular file (mode 040000)"
door2 NOTBLOB-GITLINK "$GITLINK" blocked "$(newbare gitlink)" probe "not a regular file (mode 160000)"
door2 NONHEAD-REF "$CHAIN" blocked "$(newbare nonhead)" "$C2_CH:refs/heads/probe" "records $C0_CH"
door1 NONHEAD-REF-head-is-marker "$CHAIN" 0 'git push origin probe'
MB=$(newbare multiref)
out=$(git -C "$CHAIN" push -q "$MB" "$M_CH:refs/heads/ok" "$C2_CH:refs/heads/probe" 2>&1); code=$?
if [ "$code" -ne 1 ] || ! printf '%s' "$out" | grep -qF "records $C0_CH" || ! printf '%s' "$out" | grep -qF "refs/heads/probe"; then fail "door 2 MULTIREF: expected BLOCKED naming refs/heads/probe with OLDSHA's reason, got $code" "$out"; else pass "door 2 MULTIREF blocked, the failing ref named"; fi
DB=$(newbare delete); git -C "$DB" fetch -q "$POK" probe:probe
door2 DELETE "$POK" blocked "$DB" ":refs/heads/probe" "deleting a remote ref"
UB=$(newbare uptodate); git -C "$UB" fetch -q "$POK" probe:probe
out=$(git -C "$POK" push "$UB" probe 2>&1); code=$?
echo "UPTODATE captured: git exit $code; output: $(printf '%s' "$out" | tr '\n' ' ' | cut -c1-200)"
if printf '%s' "$out" | grep -q "BLOCKED by adversary gate"; then
  if [ "$code" -ne 1 ] || ! printf '%s' "$out" | grep -qF "no ref to judge"; then fail "door 2 UPTODATE: the hook ran and must block by its reason" "$out"; else pass "door 2 UPTODATE blocked (the hook ran with empty stdin)"; fi
else
  echo "note  UPTODATE: git did not fire the pre-push hook for an up-to-date push (captured; the empty-stdin wall is exercised directly below)"
  out=$(cd "$POK" && printf '' | bash "$PREPUSH" origin "$UB" 2>&1); code=$?
  if [ "$code" -ne 2 ] || ! printf '%s' "$out" | grep -qF "no ref to judge"; then fail "pre-push on EMPTY stdin: expected 2 + \"no ref to judge\", got $code" "$out"; else pass "pre-push on empty stdin blocked by its reason (exit 2)"; fi
fi
door2 NOCHECK "$NOCHECKD" blocked "$(newbare nocheck)" probe "missing or not executable"
door1 NOCHECK "$NOCHECKD" 2 'git push origin probe' "missing or not executable"
# REPLACE: the replacement is ACTIVE (positive control), yet the wall reads the real K.
if [ "$(git -C "$REPL" log -1 --format=%T "$K_RP")" != "$(git -C "$REPL" rev-parse "$MP_RP^{tree}")" ]; then fail "REPLACE positive control: the replace ref is not active" ""; else pass "REPLACE positive control: the replace ref is active (K's tree reads as M′'s without the wall)"; fi
echo "REPLACE captured: rev-parse --verify K^{commit} under replace prints $(git -C "$REPL" rev-parse --verify "$K_RP^{commit}") (K = $K_RP, M′ = $MP_RP)"
check_probe REPLACE 2 "no verdict marker committed at $K_RP" "$REPL" "$K_RP"
door2 REPLACE "$REPL" blocked "$(newbare replace)" "$K_RP:refs/heads/probe" "no verdict marker committed at $K_RP"
# The three mktree hypotheses, each decided by what THIS git materializes (captured, never assumed):
if [ -n "$DUP_J" ]; then check_probe DUPENTRY 2 "not a single entry" "$MK" "$DUP_J"; else echo "note  DUPENTRY: mktree refused the duplicate entry — the single-entry wall is a CLOSED GUARD (declared)"; fi
if [ -n "$UNK_J" ]; then
  m=$(mode_of "$UNK_J"); echo "UNKNOWNMODE captured: mktree wrote mode 000000 as ls-tree mode '$m'"
  if [ "$m" = "000000" ]; then check_probe UNKNOWNMODE 2 "unknown mode" "$MK" "$UNK_J"
  else echo "note  UNKNOWNMODE: this git does not materialize mode 000000 (it lands as $m) — the unknown-mode branch is a CLOSED GUARD (declared); the materialized shape is judged by ITS mode:"; check_probe "UNKNOWNMODE-as-$m" 2 "not a regular file (mode $m)" "$MK" "$UNK_J"; fi
else echo "note  UNKNOWNMODE: mktree refused mode 000000 — the unknown-mode branch is a CLOSED GUARD (declared)"; fi
if [ -n "$M664_J" ]; then
  m=$(mode_of "$M664_J"); echo "MODE664 captured: mktree wrote mode 100664 as ls-tree mode '$m'"
  if [ "$m" = "100664" ]; then check_probe MODE664 2 "not a canonical regular-file mode (mode 100664)" "$MK" "$M664_J"
  else echo "note  MODE664: this git canonicalizes 100664 to $m — the non-canonical branch is a CLOSED GUARD (declared); the hand-built marker tree is a POSITIVE control:"; check_probe MODE664-canonicalized 0 "" "$MK" "$M664_J"; fi
else echo "note  MODE664: mktree refused 100664 — the non-canonical branch is a CLOSED GUARD (declared)"; fi
# NOTOPLEVEL: a push from a BARE repository — the pre-push runs where show-toplevel fails.
NTL="$TMP/notoplevel.git"; git clone -q --bare "$POK" "$NTL"; cp "$PREPUSH" "$NTL/hooks/pre-push"; chmod +x "$NTL/hooks/pre-push"; mkdir -p "$NTL/scripts"; cp "$CHECK" "$NTL/scripts/"
out=$(git -C "$NTL" push -q "$(newbare notoplevel-target)" probe 2>&1); code=$?
if [ "$code" -ne 1 ] || ! printf '%s' "$out" | grep -qF "no top level for the pushing repository"; then fail "door 2 NOTOPLEVEL: expected BLOCKED + \"no top level for the pushing repository\", got $code" "$out"; else pass "door 2 NOTOPLEVEL blocked (a push from a bare repository)"; fi

echo "--- door 1: the shapes, the hatches, the tooling (the R12 table, the reason now the marker's)"
NOB_REASON="no verdict marker committed at"
while IFS= read -r shape; do door1 shape "$NOBLOB" 2 "$shape" "$NOB_REASON"; done <<'EOF'
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
HATCH='skip git'"'"'s pre-push hook'
door1 hatch "$NOBLOB" 2 'git push --no-verify origin master' "$HATCH"
door1 hatch "$NOBLOB" 2 'git -c core.hooksPath=/dev/null push origin master' "$HATCH"
door1 pass "$POK" 0 'git push origin master'
door1 pass "$POK" 0 'if ! git push origin ensayo; then echo no; fi'
door1 hatch "$POK" 2 'git push --no-verif origin master' "$HATCH"
door1 hatch "$POK" 2 'git push --no-ver origin master' "$HATCH"
door1 hatch "$POK" 2 'git -c core.hookspath=/dev/null pus\h origin master' "$HATCH"
door1 hatch "$POK" 2 'git -c CORE.HOOKSPATH=/dev/null commit -m x' "$HATCH"
door1 hatch "$POK" 2 'git commit --no-verify -m x' "$HATCH"
door1 hatch "$POK" 2 'Git push --no-verify origin ensayo' "$HATCH"
door1 hatch "$POK" 2 'git pu'"''"'sh --no-"verify" origin ensayo' "$HATCH"
door1 hatch "$POK" 2 'git push --no-$'"'"'v'"'"'erify origin master' "$HATCH"
door1 hatch "$POK" 2 'git push --no-$"v"erify origin master' "$HATCH"
door1 negative "$NOBLOB" 0 'git commit -m "wip"'
door1 negative "$NOBLOB" 0 'git log --oneline -3'
door1 negative "$NOBLOB" 0 'echo hello'
door1 negative "$NOBLOB" 0 'go test ./...'
# NOPROJECTDIR: unset AND empty, push-shaped only; a no-push command with the variable unset is ALLOWED.
out=$(cd "$POK" && jq -cn '{tool_input:{command:"git push origin master"}}' | env -u CLAUDE_PROJECT_DIR "$HOOK" 2>&1); code=$?
if [ "$code" -ne 2 ] || ! printf '%s' "$out" | grep -qF "no project dir"; then fail "door 1 NOPROJECTDIR (unset): expected 2 + \"no project dir\", got $code" "$out"; else pass "door 1 NOPROJECTDIR unset blocked (exit 2)"; fi
out=$(cd "$POK" && jq -cn '{tool_input:{command:"git push origin master"}}' | CLAUDE_PROJECT_DIR="" "$HOOK" 2>&1); code=$?
if [ "$code" -ne 2 ] || ! printf '%s' "$out" | grep -qF "no project dir"; then fail "door 1 NOPROJECTDIR (empty): expected 2 + \"no project dir\", got $code" "$out"; else pass "door 1 NOPROJECTDIR empty blocked (exit 2)"; fi
out=$(cd "$POK" && jq -cn '{tool_input:{command:"go test ./..."}}' | env -u CLAUDE_PROJECT_DIR "$HOOK" 2>&1); code=$?
if [ "$code" -ne 0 ]; then fail "door 1 NOPROJECTDIR (unset, no push): expected 0, got $code" "$out"; else pass "door 1 NOPROJECTDIR unset, no push shape, allowed"; fi
echo "--- door 1 tooling (kept from R12)"
NOJQ="$TMP/nojq"; mkdir -p "$NOJQ"; for t in bash grep tr printf echo cat git head; do p=$(command -v "$t" 2>/dev/null) && [ -n "$p" ] && ln -sf "$p" "$NOJQ/$t"; done
out=$(printf '{"tool_input":{"command":"echo hello"}}' | PATH="$NOJQ" CLAUDE_PROJECT_DIR="$POK" bash "$HOOK" 2>&1); code=$?
if [ "$code" -ne 2 ] || ! printf '%s' "$out" | grep -qF "jq is missing"; then fail "no-jq: expected 2 + \"jq is missing\", got $code" "$out"; else pass "hook without jq fails closed"; fi
ONLYJQ="$TMP/onlyjq"; mkdir -p "$ONLYJQ"; for t in bash cat jq printf; do p=$(command -v "$t" 2>/dev/null) && [ -n "$p" ] && ln -sf "$p" "$ONLYJQ/$t"; done
NOCAT="$TMP/nocat"; mkdir -p "$NOCAT"; for t in bash jq grep tr printf; do p=$(command -v "$t" 2>/dev/null) && [ -n "$p" ] && ln -sf "$p" "$NOCAT/$t"; done
out=$(printf '{"tool_input":{"command":"git push --no-verify origin master"}}' | PATH="$NOCAT" CLAUDE_PROJECT_DIR="$NOBLOB" bash "$HOOK" 2>&1); code=$?
if [ "$code" -ne 2 ] || ! printf '%s' "$out" | grep -qF "cat is missing"; then fail "no-cat: expected 2 + \"cat is missing\", got $code" "$out"; else pass "hook without cat fails closed by its reason"; fi
out=$(printf '{"tool_input":{"command":"git push --no-verify origin master"}}' | PATH="$ONLYJQ" CLAUDE_PROJECT_DIR="$NOBLOB" bash "$HOOK" 2>&1); code=$?
if [ "$code" -ne 2 ] || ! printf '%s' "$out" | grep -qF "grep is missing"; then fail "no-grep: expected 2 + \"grep is missing\", got $code" "$out"; else pass "hook without grep fails closed by its reason"; fi
NOTR="$TMP/notr"; mkdir -p "$NOTR"; for t in bash cat jq printf grep; do p=$(command -v "$t" 2>/dev/null) && [ -n "$p" ] && ln -sf "$p" "$NOTR/$t"; done
out=$(printf '{"tool_input":{"command":"git push --no-verify origin master"}}' | PATH="$NOTR" CLAUDE_PROJECT_DIR="$NOBLOB" bash "$HOOK" 2>&1); code=$?
if [ "$code" -ne 2 ] || ! printf '%s' "$out" | grep -qF "tr is missing"; then fail "no-tr: expected 2 + \"tr is missing\", got $code" "$out"; else pass "hook without tr fails closed by its reason"; fi
BIG="git push origin master
$(yes x | head -c 300000)"
out=$(printf '%s' "$BIG" | jq -Rs '{tool_input:{command:.}}' | CLAUDE_PROJECT_DIR="$NOBLOB" "$HOOK" 2>&1); code=$?
if [ "$code" -ne 2 ] || ! printf '%s' "$out" | grep -qF "$NOB_REASON"; then fail "big-command: expected 2 + the marker reason, got $code" "$(printf '%s' "$out" | head -c 300)"; else pass "a push at the head of a 300 KB command is still gated"; fi
BADGREP="$TMP/badgrep"; mkdir -p "$BADGREP"; for t in bash cat jq tr printf; do p=$(command -v "$t" 2>/dev/null) && [ -n "$p" ] && ln -sf "$p" "$BADGREP/$t"; done
printf '#!/bin/bash\nexit 2\n' > "$BADGREP/grep"; chmod +x "$BADGREP/grep"
out=$(printf '{"tool_input":{"command":"git push origin master"}}' | PATH="$BADGREP" CLAUDE_PROJECT_DIR="$NOBLOB" bash "$HOOK" 2>&1); code=$?
if [ "$code" -ne 2 ] || ! printf '%s' "$out" | grep -qF "grep failed with status 2"; then fail "broken-grep: expected 2 + \"grep failed with status 2\", got $code" "$out"; else pass "hook with a broken grep fails closed by its reason"; fi
out=$(printf '' | CLAUDE_PROJECT_DIR="$NOBLOB" bash "$HOOK" 2>&1); code=$?
if [ "$code" -ne 2 ] || ! printf '%s' "$out" | grep -qF "could not be read or is empty"; then fail "empty-input: expected 2, got $code" "$out"; else pass "hook on empty input fails closed by its reason"; fi
out=$(printf 'not json' | CLAUDE_PROJECT_DIR="$POK" bash "$HOOK" 2>&1); code=$?
if [ "$code" -ne 2 ] || ! printf '%s' "$out" | grep -qF "not parseable JSON"; then fail "bad-json: expected 2, got $code" "$out"; else pass "hook on unparseable input fails closed"; fi

if [ "$fails" -ne 0 ]; then
  echo "adversary-gate-probe: $fails probe(s) FAILED"
  exit 1
fi
echo "adversary-gate-probe: all probes hold."
