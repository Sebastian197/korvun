#!/bin/bash
# scripts/adversary-gate-check.sh — THE push-authorization check
# (R12-H8 B1; R13-G7, D9 option (i): the marker counts ONLY committed).
#
# One script, one question: "is the commit J authorized for a push?"
# Both doors call it — the session PreToolUse hook
# (.claude/hooks/adversary-gate.sh, door 1, judging the HEAD of
# CLAUDE_PROJECT_DIR) and git's own pre-push hook (.githooks/pre-push,
# door 2, judging the pushed shas from git's stdin in order, one call
# per line, STOPPING at the first failing ref — later refs are not
# judged).
#
#   scripts/adversary-gate-check.sh <root> <sha-to-judge>
#
# THE PERIMETER, said once so no reader has to infer it: this check is a
# DISCIPLINE AID inside the perimeter declared below, NOT a security
# boundary. The hatches it does not cover are declared in this header
# and in the sibling door. "Fails closed" in this file means: when the
# check cannot READ what it must judge, it blocks instead of guessing.
# It never means a push is impossible.
#
# Both arguments are mandatory and positional: no default to `.`, to the
# top level, or to HEAD. Exit 0 = authorized. Exit 2 = blocked, ONE named
# reason on stderr. The DECISION loop is `git` ONLY (bash builtins do
# the rest: no grep, no head, no wc, no stat); the one external binary
# in the file is a `tr` inside the LAST reason string, after the
# decision is already taken — without it the reason loses its spacing,
# never its exit (the eighteenth pass, P3-3).
#
# WHAT IS ENFORCED (the wire, in THIS order):
#  -1. the caller's git environment overrides dropped (GIT_DIR and its
#      family, GIT_GRAFT_FILE — the list below), before anything runs;
#   0. argument presence (an empty root → "no root argument"; an empty
#      sha → "no sha argument" — judged before anything runs, so
#      `git -C ""` can never silently mean "here");
#   1. tools: git;
#   2. the ROOT wall — <root> must be an existing directory whose
#      canonical path (`cd`, `pwd -P`, CDPATH unset) equals the physical
#      top level git names for it (`rev-parse --show-toplevel`, stdout
#      only; on failure a SECOND call fetches git's words for the
#      reason text — the judgement is the first call's exit);
#   3. the argument resolved through
#      `git rev-parse --verify --end-of-options "<arg>^{commit}"` — J is
#      its output: an annotated tag is peeled to its commit, a
#      well-formed but nonexistent 40-hex is "not resolvable";
#   4. J's parents from `rev-list --parents -n 1 J`: the first field must
#      be J (a closed guard) and there must be EXACTLY one parent — a
#      root commit (or a shallow clone showing J as a root) and a merge
#      are blocked, named;
#   5. the marker ENTRY from `ls-tree --full-tree J -- <marker path>`:
#      absent → "no verdict marker committed at J"; more than one line →
#      "not a single entry"; the MODE decides regularity (100644/100755
#      ok; 120000/040000/160000 "not a regular file"; other 100xxx "not a
#      canonical regular-file mode"; anything else "unknown mode") — a
#      symlink is a blob with mode 120000, which is why the mode and not
#      the object type is judged;
#   6. the blob by OBJECT ID: `cat-file -s` (its exit tested, its output
#      validated as digits; 0 → "empty verdict marker"), then
#      `cat-file -p` (its exit tested; never `git show`: no textconv, no
#      pager, NO pipe into head — the first line is a bash substitution);
#   7. the FIRST line exactly `VETO LEVANTADO <40 lowercase hex>` (bash's
#      [[ =~ ]] under LC_ALL=C; CRLF, leading or trailing whitespace, a
#      BOM, uppercase hex, a 7-hex prefix, a bare "VETO LEVANTADO", a
#      "VETO MANTENIDO … VETO LEVANTADO <sha>" sentence all fail here);
#   8. the recorded 40-hex equals J's sole parent (both printed when not);
#   9. `diff-tree -r --no-renames --name-only J^ J` lists EXACTLY the
#      marker path (an empty diff → "changes nothing at the marker path";
#      anything else → "carries more than the marker", the files listed).
#   Every git call runs with `--no-replace-objects` — a WALL, not
#   hygiene: a replace ref (`git replace K M`) would otherwise make
#   rev-list, ls-tree, cat-file and diff-tree read another commit's
#   history and authorize a commit other than the one pushed (the
#   probe's REPLACE fixture and its mutant m-replace).
#
# WHAT THE LETRERO MEANS — "a marker whose FIRST LINE names the direct
# sole parent was committed", never "audited": only the first line is
# read (a body saying anything else is not judged); the wire enforces
# ONLY that. "Amend, never stack" is guidance: a chain of pure marker
# commits each naming its parent PASSES (declared). Legacy grafts
# (.git/info/grafts, still honored by git 2.27) rewrite parents without
# a replace ref — not covered, declared. SHA-256 repositories are out of
# scope. NUL bytes in the blob are dropped or truncated by the shell
# substitution depending on the bash version — not captured, not a
# claim. The marker is never read from the working tree; the check
# script and the hooks themselves DO run from it — an edited check
# decides, and so does an OLD one: door 2 execs the check of the
# checkout it pushes from, so a linked worktree checked out at an
# older commit runs that commit's `scripts/` copy (the session hook's
# header confesses both hatches).
#
# No mtime, no freshness by clock (D2 revoked, definitive): a blob has
# no mtime, and the SHA binding above is what a fresh clone or worktree
# could not fake.
set -u
# Exported (the seventh pass): bash's [[ =~ ]] needs it for a byte-exact
# hex class, and git's own messages — which the reasons quote and the
# probe reads ("Permission denied") — must not depend on the runner's
# locale.
export LC_ALL=C
MARKER=".claude/adversary/last-verdict.md"
# The repository judged is the one <root> names, never one the caller's
# environment points git at: the object-directory and work-tree
# overrides are dropped here (fixture ENVDIR, mutant m-env). GIT_TRACE
# and friends write to stderr, which no capture that feeds a JUDGEMENT
# folds into a value (fixture GITTRACE, mutant m-n7tr); the one capture
# of stderr, ERR below, feeds a reason text on the failure path only
# (under GIT_TRACE=1 that text carries trace lines — cosmetic,
# declared). NOT dropped, declared: the
# config layer — GIT_CONFIG_*, a swapped HOME, a config file carrying
# core.useReplaceRefs=false or an alias — the same class of hatch the
# session hook's header confesses (reached without hatch text); of
# these the probe asserts only the replace switches in its own
# environment (GIT_NO_REPLACE_OBJECTS unset, core.useReplaceRefs not
# false), the rest declared.
# GIT_GRAFT_FILE is the ENVIRONMENT form of the graft hole declared
# below (a graft rewrites parents like a replace ref does): dropped
# (fixture GRAFTENV, mutant m-graft). Declared consequence of the
# drop: an environment-only layout (GIT_DIR + GIT_WORK_TREE with no
# .git in the tree) is refused "git cannot name a top level" — a false
# block, fail-closed, outside this check's contract.
unset GIT_DIR GIT_WORK_TREE GIT_COMMON_DIR GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_INDEX_FILE GIT_NAMESPACE GIT_GRAFT_FILE

block() { echo "BLOCKED by adversary gate: $1" >&2; exit 2; }

# 0. Argument presence.
ROOT="${1:-}"
[ -n "$ROOT" ] || block "no root argument — the check judges ONE repository root, given explicitly; nothing is authorized."
SHA="${2:-}"
[ -n "$SHA" ] || block "no sha argument — the check judges ONE commit, given explicitly (never a default to HEAD); nothing is authorized."

# 1. Tools.
command -v git >/dev/null 2>&1 || block "tool missing: git — the check cannot read the repository, so it fails closed."

# 2. The ROOT wall, in canonical forms.
[ -d "$ROOT" ] || block "root is not a directory: $ROOT"
# A directory that exists but cannot be entered (no search permission)
# leaves CANON empty and FALLS to the git wall below (git cannot chdir
# into it either) — one reason, NOTAREPO's, as the paper's §4 disposes;
# were git to name a top level anyway, the equality wall blocks.
CANON=$(unset CDPATH; cd -- "$ROOT" >/dev/null 2>&1 && pwd -P)
# stdout only into TOP (a tracing environment writes to stderr and must
# never become the "top level"); git's own words are fetched for the
# reason only on the failure path.
if ! TOP=$(git --no-replace-objects -C "$ROOT" rev-parse --show-toplevel 2>/dev/null </dev/null); then
  ERR=$(git --no-replace-objects -C "$ROOT" rev-parse --show-toplevel 2>&1 >/dev/null </dev/null)
  block "git cannot name a top level for $ROOT: $ERR"
fi
[ "$CANON" = "$TOP" ] || block "root is not the repository top level (root $CANON, top level $TOP)."

G() { git --no-replace-objects -C "$ROOT" "$@" </dev/null; }

# 3. The commit J, resolved (an annotated tag peeled; a name or a
#    nonexistent 40-hex refused).
J=$(G rev-parse --verify --quiet --end-of-options "$SHA^{commit}" 2>/dev/null) || block "not resolvable to a commit in $ROOT: $SHA"
[ -n "$J" ] || block "not resolvable to a commit in $ROOT: $SHA"

# 4. Exactly one parent.
PARENTS=$(G rev-list --parents -n 1 "$J" 2>/dev/null) || block "parents unreadable (rev-list failed for $J)."
set -- $PARENTS
[ "${1:-}" = "$J" ] || block "rev-list's first field is not J (got '${1:-}', judging $J)."
if [ $# -lt 2 ]; then block "the judged commit $J has no parent (a root commit, or a shallow clone showing it as one) — a marker commit names its direct sole parent."; fi
if [ $# -gt 2 ]; then block "the judged commit $J is a merge (more than one parent) — a marker commit names its direct sole parent."; fi
PARENT=$2

# 5. The marker entry, by ls-tree: one line, a regular file by MODE.
ENTRY=$(G ls-tree --full-tree "$J" -- "$MARKER" 2>/dev/null) || block "marker unreadable (ls-tree failed at $J)."
[ -n "$ENTRY" ] || block "no verdict marker committed at $J ($MARKER) — record the adversary's VETO LEVANTADO in a pure marker commit on top of the audited commit, then push."
case "$ENTRY" in
  *$'\n'*) block "marker path is not a single entry at $J ($MARKER)." ;;
esac
MODE=${ENTRY%% *}
REST=${ENTRY#* }
REST=${REST#* }
OBJ=${REST%%$'\t'*}
case "$MODE" in
  100644|100755) ;;
  120000|040000|160000) block "marker is not a regular file (mode $MODE) at $J." ;;
  100???) block "marker is not a canonical regular-file mode (mode $MODE) at $J." ;;
  *) block "unknown mode $MODE for the marker at $J." ;;
esac

# 6. The blob by object id: size (digits), then content.
SIZE=$(G cat-file -s "$OBJ" 2>/dev/null) || block "marker blob SIZE unreadable (object $OBJ)."
DIGITS='^[0-9]+$'
[[ $SIZE =~ $DIGITS ]] || block "marker blob SIZE unreadable (object $OBJ): got '$SIZE'."
[ "$SIZE" -ne 0 ] || block "empty verdict marker (object $OBJ) at $J."
CONTENT=$(G cat-file -p "$OBJ" 2>/dev/null) || block "marker blob CONTENT unreadable (object $OBJ)."

# 7. The first line, exactly.
FIRST=${CONTENT%%$'\n'*}
RE='^VETO LEVANTADO ([0-9a-f]{40})$'
[[ $FIRST =~ $RE ]] || block "the marker's first line is not exactly 'VETO LEVANTADO <40-hex sha>' at $J (got '${FIRST:0:80}')."
RECORDED=${BASH_REMATCH[1]}

# 8. The recorded sha is J's direct sole parent.
[ "$RECORDED" = "$PARENT" ] || block "the marker at $J records $RECORDED, but the judged commit's parent is $PARENT — the recording commit does not name its direct parent."

# 9. The recording commit changes exactly the marker.
DIFF=$(G diff-tree -r --no-renames --name-only "$PARENT" "$J" 2>/dev/null) || block "diff unreadable (diff-tree failed for $PARENT..$J)."
[ -n "$DIFF" ] || block "the recording commit $J changes nothing at the marker path."
[ "$DIFF" = "$MARKER" ] || block "the recording commit $J carries more than the marker: $(printf '%s' "$DIFF" | tr '\n' ' ')"
exit 0
