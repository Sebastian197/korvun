#!/usr/bin/env python3
"""Trim evidence captures for the repository.

Keeps, per capture: every header line (starting with '#'), the mutation diff
(between '## diff of the mutation' and the next section), section markers
('## ...'), the verdict lines ('--- FAIL', 'FAIL', 'ok', 'Coverage:',
'Quality gate passed', 'No vulnerabilities found', '# exit:') and the first
failing assertion line under each '--- FAIL' (a line matching
'_test.go:<n>:'). Everything else — '--- PASS' lines, race-detector noise,
logs, goroutine dumps — is dropped. Lines are cut at 400 characters. Usage:
trim.py <capture.txt>... (rewrites each file in place).
"""
import os, re, sys

ASSERT = re.compile(r'_test\.go:\d+:')
VERDICT = re.compile(r'^\s*(--- FAIL|FAIL\b|ok\s|Coverage:|Quality gate passed|No vulnerabilities found|# exit:)')


def trim(text):
    out = []
    in_diff = False
    want_assert = False
    for raw in text.splitlines():
        line = raw[:400]
        s = line.strip()
        if line.startswith('## '):
            in_diff = line.startswith('## diff of the mutation')
            out.append(line)
            want_assert = False
            continue
        if in_diff:
            out.append(line)
            continue
        if line.startswith('#'):
            out.append(line)
            continue
        if VERDICT.match(line):
            out.append(line)
            want_assert = s.startswith('--- FAIL')
            continue
        if want_assert and ASSERT.search(line):
            out.append(line)
            want_assert = False
    return '\n'.join(out) + '\n'


for p in sys.argv[1:]:
    t = open(p, errors='replace').read()
    open(p, 'w').write(trim(t))
