#!/bin/bash
# Re-captures the C1 guard evidence: the red of README at the base commit and
# every probing mutation, each with the command, the worktree HEAD, the sha256
# of the guard and of the mutated file, and the exit code. Mutations run on
# the working copy and are reverted from a backup after each run.
set -u
ROOT=$(git rev-parse --show-toplevel)
cd "$ROOT"
OUT=docs/superpowers/specs/evidence/v0151-c
GUARD=website/scripts/check-current-release.mjs
EN=website/docs/releases/index.md
ES=website/i18n/es/docusaurus-plugin-content-docs/current/releases/index.md
FACTS=website/src/releaseFacts.ts
BAK=$(mktemp -d)
for f in README.md "$EN" "$ES" "$FACTS"; do mkdir -p "$BAK/$(dirname "$f")"; cp "$f" "$BAK/$f"; done
restore() { for f in README.md "$EN" "$ES" "$FACTS"; do cp "$BAK/$f" "$f"; done; }

run() { # $1 = id, $2 = description, $3 = mutated file
  {
    echo "## $1 — $2"
    echo "HEAD: $(git rev-parse HEAD) (working tree carries the uncommitted train)"
    echo "guard sha256: $(shasum -a 256 "$GUARD" | cut -d' ' -f1)"
    echo "file under test: $3 sha256 $(shasum -a 256 "$3" | cut -d' ' -f1)"
    echo "command: (cd website && node scripts/check-current-release.mjs)"
    (cd website && node scripts/check-current-release.mjs) 2>&1
    echo "exit=$?"
    echo
  } >> "$OUT/$4"
  restore
}

: > "$OUT/c1-red-today.txt"
git show 61ae5827f4798f9cf469870483bee4958993bcef:README.md > README.md
run RED "README.md exactly as committed at 61ae582 (git show 61ae582:README.md > README.md)" README.md c1-red-today.txt

: > "$OUT/c1-mutations.txt"
run CLEAN "the train's tree, no mutation" README.md c1-mutations.txt
sed -i '' 's/\*\*`v0.15.0` — Beta — is the current release/**`v0.14.0` — Beta — is the current release/' README.md
run M1 "README status line back to v0.14.0" README.md c1-mutations.txt
python3 -c "p='README.md';s=open(p).read();i=s.index('**Download v0.15.0**');open(p,'w').write(s+'\n'+s[i:s.index('\n',i)]+'\n')"
run M2 "README Download line duplicated" README.md c1-mutations.txt
python3 -c "p='README.md';s=open(p).read();open(p,'w').write(s.replace('**Download v0.15.0**','Download'))"
run M3 "README Download line removed" README.md c1-mutations.txt
python3 -c "p='$FACTS';s=open(p).read();open(p,'w').write(s.replace('tag: \"v0.15.0\"','tag: \"v0.15.1\"'))"
run M4 "releaseFacts.tag moved to v0.15.1, README untouched" "$FACTS" c1-mutations.txt
python3 -c "p='README.md';s=open(p).read();open(p,'w').write(s.replace('(docs/releases/v0.15.0.md),\n[ROADMAP','(docs/releases/v0.14.0.md),\n[ROADMAP'))"
run M5 "README Status release-notes link to v0.14.0" README.md c1-mutations.txt
python3 -c "
p='$EN';L=open(p).read().split('\n')
i=[k for k,l in enumerate(L) if '| **Current release' in l][0]
L[i]=L[i].replace('| [v0.15.0]','| [v0.14.0]',1);open(p,'w').write('\n'.join(L))"
run M6 "EN Current-release row link text to v0.14.0" "$EN" c1-mutations.txt
python3 -c "
p='$EN';L=open(p).read().split('\n')
i=[k for k,l in enumerate(L) if '| **Current release' in l][0]
L[i]=L[i].replace('releases/tag/v0.15.0','releases/tag/v0.14.0',1);open(p,'w').write('\n'.join(L))"
run M7 "EN Current-release row href to v0.14.0, link text untouched" "$EN" c1-mutations.txt
python3 -c "
p='$ES';L=open(p).read().split('\n')
i=[k for k,l in enumerate(L) if '| **Release actual' in l][0]
L[i]=L[i].replace('releases/tag/v0.15.0','releases/tag/v0.14.0',1);open(p,'w').write('\n'.join(L))"
run M8 "ES Release-actual row href to v0.14.0, link text untouched" "$ES" c1-mutations.txt
python3 -c "p='$ES';s=open(p).read();open(p,'w').write(s.replace('| **Release actual','| **Release',1))"
run M9 "ES Release-actual marker removed" "$ES" c1-mutations.txt
python3 -c "
p='$EN';L=open(p).read().split('\n')
i=[k for k,l in enumerate(L) if l.startswith('| [v0.14.0]')][0]
L[i]=L[i].replace('| **','| **Current release — ',1);open(p,'w').write('\n'.join(L))"
run M10 "EN Current-release marker also on the v0.14.0 row" "$EN" c1-mutations.txt
rm -rf "$BAK"

# The full harness, unfiltered, with make's own exit code.
{
  echo "command: make website-check"
  echo "HEAD: $(git rev-parse HEAD) (working tree carries the uncommitted train)"
  echo "Makefile sha256: $(shasum -a 256 Makefile | cut -d' ' -f1)"
  echo "guard sha256: $(shasum -a 256 "$GUARD" | cut -d' ' -f1)"
  echo "node $(node --version) (CI uses 24)"
  echo "---- make output, unfiltered ----"
} > "$OUT/website-check-final.txt"
make website-check >> "$OUT/website-check-final.txt" 2>&1
echo "make exit=$?" >> "$OUT/website-check-final.txt"

# C2: the whole-tree SBOM grep, unfiltered, with its command and exit code.
SBOM_CMD="git grep -n -i sbom -- ':!website/node_modules' ':!docs/cantos' ':!docs/stages' ':!docs/HANDOFF.md' ':!docs/superpowers' ':!docs/adr' ':!docs/releases/v0.[0-9].*' ':!docs/releases/v0.1[0-4]*' ':!.github' ':!**/package-lock.json' ':!**/*.go'"
{
  echo "command: $SBOM_CMD"
  echo "HEAD: $(git rev-parse HEAD) (git grep reads the working tree, which carries the uncommitted train)"
  echo "---- output, unfiltered ----"
  eval "$SBOM_CMD"
  echo "exit=$?"
} > "$OUT/c2-sbom-grep.txt"

# P2-2: the full replacement body of the published v0.15.0 release, for the
# director (editing the published body is the director's act). It is the
# corrected docs/releases/v0.15.0.md without its BORRADOR banner — exactly the
# shape the published body has against the tag's notes. The diff lists every
# divergence between what is published and the replacement.
python3 - "$OUT/v0150-release-body-replacement.md" <<'PY'
import re, sys
s = open('docs/releases/v0.15.0.md').read()
s = re.sub(r'\A(# [^\n]*\n)\n(> [^\n]*\n)+\n', r'\1\n', s, count=1)
open(sys.argv[1], 'w').write(s)
PY
{
  echo "command: gh release view v0.15.0 --json body --jq .body | diff - $OUT/v0150-release-body-replacement.md"
  echo "HEAD: $(git rev-parse HEAD) (working tree carries the uncommitted train)"
  echo "replacement sha256: $(shasum -a 256 "$OUT/v0150-release-body-replacement.md" | cut -d' ' -f1)"
  echo "---- diff, unfiltered ----"
  gh release view v0.15.0 --json body --jq .body | diff - "$OUT/v0150-release-body-replacement.md"
  echo "exit=$? (1 = the bodies differ)"
} > "$OUT/v0150-release-body-divergences.txt"
