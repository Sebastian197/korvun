#!/usr/bin/env python3
# Copyright 2026 Sebastián Moreno Saavedra
# SPDX-License-Identifier: Apache-2.0
"""Derive the probing-mutation tally from an evidence log, by a written rule.

The log (docs/superpowers/specs/evidence/<release>/mutations.txt) is a series
of BLOCKS. A block starts at a header line `M<n>[-bis|-ter] · <title>` and
runs to the next header. The rule, literal:

  not applied   the body says `ANCHOR COUNT` or `FILE EXISTS` (the runner
                could not place the mutation);
  declared      the body says `DECLARADA SIN ROJO` (no reachable red, said);
  executed      every other block;
  no compile    an executed block whose body says `LA MUTACIÓN NO COMPILA`
                or whose CAPTURE shows `[build failed]`;
  green         an executed, compiled block whose body says `!! VERDE`;
  red           an executed, compiled, not green block whose CAPTURE shows a
                failure: a Go `--- FAIL`/`FAIL` line, a vitest `FAIL`/`×`/
                `Tests  N failed` line, or a `panic:`;
  unmarked      an executed, compiled, not green block with NO captured
                failure — a defect of the log, listed, never counted as red.

The CAPTURE of a block is everything from its first `$ ` command line on;
the lines before it (the title, the `mutación:` line, a note quoting an
earlier run) are prose and decide nothing. A section heading (`## ...`)
ends the block it follows, so a declared caveat never counts as a capture.
Duplicated headers are listed. A range (`--from M224`) selects the blocks
whose number is at least that, whatever their order in the file.

Declared limits of the rule (the log is written by a runner, and the rule
reads what the runner writes): a `--- FAIL` of ANOTHER test inside a
whole-package capture counts as red (the runner filters with -run; a hand
capture that does not is on its author); `!! VERDE` is the runner's word
and wins over a captured failure (the runner never writes both); prose
after the command line is capture (a `×` or `FAIL` written there counts);
only `-bis` and `-ter` repeats are headers (`-quater` is not); a `$ ` at the
start of a prose line begins the capture early.
"""
import argparse
import json
import re
import sys

HEADER = re.compile(r"^M(\d+)(-bis|-ter)? · ")
GO_FAIL = re.compile(r"^(--- FAIL|FAIL\b)")
VITEST_FAIL = re.compile(r"(^\s*FAIL\s|\s×\s|Tests\s+\d+ failed)")
BUILD_FAILED = re.compile(r"\[build failed\]")


def blocks(text):
    """Split the log into (label, number, body) blocks, in file order.

    A block runs from its header to the next header or to the next section
    heading (`## ...`), whichever comes first.
    """
    lines = text.split("\n")
    heads = [(i, l) for i, l in enumerate(lines) if HEADER.match(l)]
    out = []
    for k, (i, line) in enumerate(heads):
        j = heads[k + 1][0] if k + 1 < len(heads) else len(lines)
        for n in range(i + 1, j):
            if lines[n].startswith("## "):
                j = n
                break
        m = HEADER.match(line)
        out.append((line.split(" · ")[0], int(m.group(1)), "\n".join(lines[i:j])))
    return out


def capture(body):
    """The block's captured output: from its first `$ ` command line on."""
    lines = body.split("\n")
    for i, line in enumerate(lines):
        if line.startswith("$ "):
            return lines[i:]
    return []


def classify(body):
    """One of: not_applied, declared, no_compile, green, red, unmarked."""
    if "ANCHOR COUNT" in body or "FILE EXISTS" in body:
        return "not_applied"
    if "DECLARADA SIN ROJO" in body:
        return "declared"
    captured = capture(body)
    if "LA MUTACIÓN NO COMPILA" in body or any(BUILD_FAILED.search(l) for l in captured):
        return "no_compile"
    if "!! VERDE" in body:
        return "green"
    for line in captured:
        if GO_FAIL.match(line) or VITEST_FAIL.search(line) or line.startswith("panic:"):
            return "red"
    return "unmarked"


def tally(text, start=0):
    """The counts and the id lists for the blocks numbered at least start."""
    seen, duplicates = set(), []
    counts = {k: [] for k in ("not_applied", "declared", "no_compile", "green", "red", "unmarked")}
    headers = 0
    for label, number, body in blocks(text):
        if label in seen:
            duplicates.append(label)
        seen.add(label)
        if number < start:
            continue
        headers += 1
        counts[classify(body)].append(label)
    executed = headers - len(counts["not_applied"]) - len(counts["declared"])
    return {
        "from": start,
        "headers": headers,
        "executed": executed,
        "duplicated_headers": duplicates,
        **{k: v for k, v in counts.items()},
    }


def render(t):
    """The tally as one paragraph, the way the papers quote it."""
    return (
        f"desde M{t['from']}: {t['headers']} cabeceras, {len(t['not_applied'])} no aplicadas "
        f"{t['not_applied']}, {len(t['declared'])} declaradas sin rojo {t['declared']}, "
        f"{t['executed']} ejecutadas, {len(t['no_compile'])} que no compilaban {t['no_compile']}, "
        f"{len(t['green'])} verdes {t['green']}, {len(t['red'])} rojas, "
        f"{len(t['unmarked'])} sin marcador {t['unmarked']}; cabeceras duplicadas {t['duplicated_headers']}"
    )


def main(argv):
    parser = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    parser.add_argument("log")
    parser.add_argument("--from", dest="start", default="M0", help="first mutation number to count, e.g. M224")
    parser.add_argument("--json", action="store_true")
    args = parser.parse_args(argv)
    start = int(re.sub(r"[^0-9]", "", args.start) or 0)
    with open(args.log, encoding="utf-8") as fh:
        t = tally(fh.read(), start)
    print(json.dumps(t, ensure_ascii=False) if args.json else render(t))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
