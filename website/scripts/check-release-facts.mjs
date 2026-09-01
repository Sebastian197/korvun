// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0
//
// check-release-facts: the public-truth guard for release versions.
//
// The 2026-08-30 field find: korvun.dev served a MIX of versions — hero
// and install block at v0.11.0, release-notes link text at v0.13.0
// pointing at the v0.11.0 tag — because the version was hardcoded in
// four places and only one was updated. The root fix: releaseFacts.ts
// is the ONLY place in src/ allowed to spell a release version; every
// component interpolates it. This guard fails the build if any version
// literal appears anywhere else under src/, naming file and line.

import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('..', import.meta.url));
const srcDir = join(root, 'src');
const factsFile = join(srcDir, 'releaseFacts.ts');

// A release version literal: v0.13.0 / 0.13.0 (word-bounded, three
// numeric parts). Package semver ranges do not live under src/.
const versionLiteral = /\bv?\d+\.\d+\.\d+\b/g;

// Non-version noise allowed under src/: pure numeric triplets that are
// not release versions do not exist there today; if one ever appears,
// it must move behind releaseFacts or extend this allowlist EXPLICITLY.
const allowedFiles = new Set([factsFile]);

const offenders = [];

function walk(dir) {
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    const st = statSync(path);
    if (st.isDirectory()) {
      walk(path);
      continue;
    }
    if (!/\.(ts|tsx|js|jsx|mjs)$/.test(entry)) continue;
    if (/\.test\.mjs$/.test(entry)) continue;
    if (allowedFiles.has(path)) continue;
    const lines = readFileSync(path, 'utf8').split('\n');
    lines.forEach((line, i) => {
      const matches = line.match(versionLiteral);
      if (!matches) return;
      for (const m of matches) {
        offenders.push(`${relative(root, path)}:${i + 1}: hardcoded version literal ${JSON.stringify(m)} — spell it through releaseFacts.ts`);
      }
    });
  }
}

// Phase 2 (2026-09-01, after the v0.14.1 lag): the fact must MATCH the
// published reality. releaseFacts sat at v0.14.0 while the latest tag
// was v0.14.1 — a stale token the literal sweep cannot see. In CI (and
// any online run) the guard compares releaseFacts.tag against the
// repository's published latest release and BREAKS on divergence;
// offline it SKIPS OUT LOUD, never silently.
async function checkAgainstLatest() {
  const factsSrc = readFileSync(factsFile, 'utf8');
  const tagMatch = factsSrc.match(/tag:\s*"(v\d+\.\d+\.\d+)"/);
  if (!tagMatch) {
    console.error('check-release-facts: FAIL — cannot read releaseFacts.tag');
    process.exit(1);
  }
  const factTag = tagMatch[1];
  let latest;
  try {
    const res = await fetch(
      'https://api.github.com/repos/Sebastian197/korvun/releases/latest',
      { headers: { accept: 'application/vnd.github+json' }, signal: AbortSignal.timeout(10000) },
    );
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    latest = (await res.json()).tag_name;
  } catch (err) {
    console.log(`check-release-facts: latest-release comparison SKIPPED (offline/unreachable: ${err.message}) — the literal sweep above still ran`);
    return;
  }
  if (latest !== factTag) {
    console.error(`check-release-facts: FAIL — releaseFacts.tag is ${factTag} but the published latest release is ${latest}: bump src/releaseFacts.ts`);
    process.exit(1);
  }
  console.log(`check-release-facts: latest-release comparison OK (${factTag})`);
}

walk(srcDir);

if (offenders.length > 0) {
  console.error('check-release-facts: FAIL — the release version must live ONLY in src/releaseFacts.ts:');
  for (const line of offenders) console.error('  ' + line);
  process.exit(1);
}
console.log('check-release-facts: OK — every release version flows from releaseFacts.ts')
await checkAgainstLatest();
