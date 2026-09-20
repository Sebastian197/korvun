// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0
//
// check-current-release: every public sentence that names the CURRENT
// release outside website/src/ must name the same release as
// website/src/releaseFacts.ts.
//
// The 2026-09-19 find: after v0.15.0 was published the README still said
// "Download v0.14.0" and "`v0.14.0` — Beta — is the current release", while
// releaseFacts.ts said v0.15.0. check-release-facts.mjs guards src/ only, so
// nothing compared these sentences against the fact.
//
// Scope, stated exactly: five claims, each of which must appear exactly
// once and name releaseFacts.tag — README's "**Download vX.Y.Z**" line, its
// "**`vX.Y.Z` — … — is the current release.**" line and the Status
// paragraph's "[the release notes](docs/releases/vX.Y.Z.md)" link, and the
// row of the website's releases table marked "**Current release" (EN) and
// "**Release actual" (ES), where BOTH the link text and the href's tag are
// judged (the 2026-08-30 find was link text and href disagreeing). A
// missing or duplicated claim fails: an absent claim is not treated as a
// matching one. Other version literals (feature
// history such as "Approvals (v0.15.0)", older rows of the table) are
// history, not current-release claims, and are not judged here.

import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const websiteRoot = fileURLToPath(new URL('..', import.meta.url));
const factsFile = join(websiteRoot, 'src', 'releaseFacts.ts');
const readmeFile = join(websiteRoot, '..', 'README.md');
const releasesEn = join(websiteRoot, 'docs', 'releases', 'index.md');
const releasesEs = join(
  websiteRoot, 'i18n', 'es', 'docusaurus-plugin-content-docs', 'current', 'releases', 'index.md',
);

const fail = (msg) => {
  console.error(`check-current-release: FAIL — ${msg}`);
  process.exit(1);
};

const factsSrc = readFileSync(factsFile, 'utf8');
const tagMatch = factsSrc.match(/tag:\s*"(v\d+\.\d+\.\d+)"/);
if (!tagMatch) fail('cannot read releaseFacts.tag');
const factTag = tagMatch[1];

const claims = [
  { file: readmeFile, label: 'README.md', name: 'the Download line', re: /\*\*Download (v\d+\.\d+\.\d+)\*\*/g },
  { file: readmeFile, label: 'README.md', name: 'the current-release line', re: /\*\*`(v\d+\.\d+\.\d+)` — [^*\n]* — is the current release\.\*\*/g },
  { file: readmeFile, label: 'README.md', name: 'the Status release-notes link', re: /\[the release notes\]\(docs\/releases\/(v\d+\.\d+\.\d+)\.md\)/g },
  { file: releasesEn, label: 'docs/releases/index.md', name: 'the "Current release" row', re: /^\| \[(v\d+\.\d+\.\d+)\]\(https:\/\/github\.com\/Sebastian197\/korvun\/releases\/tag\/(v\d+\.\d+\.\d+)\) \| \*\*Current release/gm },
  { file: releasesEs, label: 'i18n es releases/index.md', name: 'the "Release actual" row', re: /^\| \[(v\d+\.\d+\.\d+)\]\(https:\/\/github\.com\/Sebastian197\/korvun\/releases\/tag\/(v\d+\.\d+\.\d+)\) \| \*\*Release actual/gm },
];

for (const { file, label, name, re } of claims) {
  const found = [...readFileSync(file, 'utf8').matchAll(re)];
  if (found.length !== 1) {
    fail(`${label} carries ${found.length} occurrences of ${name}; exactly one is required`);
  }
  const [, text, href] = found[0];
  if (text !== factTag) {
    fail(`${label} ${name} names ${text} but releaseFacts.tag is ${factTag}`);
  }
  if (href !== undefined && href !== factTag) {
    fail(`${label} ${name} links to ${href} but releaseFacts.tag is ${factTag}`);
  }
}

console.log(`check-current-release: OK — the five current-release claims name ${factTag}`);
