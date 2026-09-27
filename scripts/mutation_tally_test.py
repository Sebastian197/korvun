# Copyright 2026 Sebastián Moreno Saavedra
# SPDX-License-Identifier: Apache-2.0
"""Attack tests for the mutation tally: every class of block, forced."""
import importlib.util
from pathlib import Path
import sys
import unittest

sys.dont_write_bytecode = True

SPEC = importlib.util.spec_from_file_location("tally", Path(__file__).with_name("mutation_tally.py"))
tally = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(tally)

LOG = """M1 · a red one
$ go test ./x -run TestA
--- FAIL: TestA (0.01s)
FAIL

M2 · a green one
$ go test ./x -run TestB
ok  	x	0.1s
!! VERDE: la mutación NO enrojece el molde

M3 · not applied
!! ANCHOR COUNT 0 — mutation NOT applied

M4 · declared
DECLARADA SIN ROJO ALCANZABLE. The branch is defence in depth.

M5 · did not compile
$ go test ./x -run TestC
FAIL	x [build failed]
!! VERDE: la mutación NO enrojece el molde

M5-bis · repeated, red
$ go test ./x -run TestC
--- FAIL: TestC (0.01s)

M6 · vitest red
$ npx vitest run src/a.test.tsx
     × shows the row 10ms
      Tests  1 failed | 3 passed (4)

M7 · a block with nothing captured
$ go test ./x -run TestD
ok  	x	0.1s

M8 · a file that exists
!! FILE EXISTS — mutation NOT applied

M1 · a duplicated header
$ go test ./x -run TestA
--- FAIL: TestA (0.01s)
"""


class TallyAttacks(unittest.TestCase):
    def test_everyClassIsCountedWhereTheRuleSays(self):
        t = tally.tally(LOG)
        self.assertEqual(10, t["headers"])
        self.assertEqual(["M3", "M8"], t["not_applied"])
        self.assertEqual(["M4"], t["declared"])
        self.assertEqual(7, t["executed"])
        self.assertEqual(["M5"], t["no_compile"], "a [build failed] capture is never green, whatever the runner appended")
        self.assertEqual(["M2"], t["green"])
        self.assertEqual(["M1", "M5-bis", "M6", "M1"], t["red"])
        self.assertEqual(["M7"], t["unmarked"], "a block with no captured failure is a defect of the log, not a red")
        self.assertEqual(["M1"], t["duplicated_headers"])

    def test_theRangeSelectsByNumberNotByPosition(self):
        t = tally.tally(LOG, 5)
        self.assertEqual(5, t["headers"])
        self.assertEqual({"M5", "M5-bis", "M6", "M7", "M8"}, set(t["not_applied"] + t["no_compile"] + t["red"] + t["unmarked"]))
        self.assertEqual([], t["green"] + t["declared"])

    def test_proseBeforeTheCaptureDecidesNothing(self):
        body = "M9 · repeated (M8 did not compile: [build failed])\nmutación: x — FAIL was quoted here\n$ go test ./x\n--- FAIL: TestE (0s)"
        self.assertEqual("red", tally.classify(body))
        body = "M10 · a note that says FAIL and × in prose\nmutación: y\n$ go test ./x\nok  \tx\t0.1s"
        self.assertEqual("unmarked", tally.classify(body))

    def test_aGreenMarkerOnAnUncompiledBlockIsNotGreen(self):
        self.assertEqual("no_compile", tally.classify("x\n$ go test ./x\nFAIL\tx [build failed]\n!! VERDE"))

    def test_aRedNeedsACapturedFailure(self):
        self.assertEqual("unmarked", tally.classify("$ go test\nok  \tx\t0.1s"))
        self.assertEqual("red", tally.classify("$ go test\n--- FAIL: T (0s)"))
        self.assertEqual("red", tally.classify("$ npx vitest run\n FAIL  src/a.test.tsx > a"))
        self.assertEqual("red", tally.classify("$ go test\npanic: nil deref"))
        self.assertEqual("unmarked", tally.classify("--- FAIL: T (0s) quoted in prose, no command line"))

    def test_aSectionHeadingEndsTheBlockBeforeIt(self):
        log = "M20 · a green block followed by a caveat section\n$ go test ./x\nok  \tx\t0.1s\n!! VERDE\n\n## Salvedades declaradas\n$ go test ./x\n--- FAIL: quoted in the caveat (0s)\n"
        t = tally.tally(log)
        self.assertEqual(1, t["headers"])
        self.assertEqual(["M20"], t["green"], "the caveat's quoted FAIL belongs to no block")
        self.assertEqual([], t["red"])

    def test_theDeclaredLimitsHoldAsWritten(self):
        self.assertEqual("red", tally.classify("M30 · package capture\n$ go test ./x\n--- FAIL: TestOther (0s)"), "another test's FAIL in the capture counts as red, declared")
        self.assertEqual("green", tally.classify("M31 · both marks\n$ go test ./x\n--- FAIL: T (0s)\n!! VERDE"), "the runner's word wins, declared")
        self.assertEqual(0, tally.tally("M4-quater · not a header\n$ go test\n--- FAIL: T (0s)")["headers"])

    def test_theRenderedLineCarriesEveryCount(self):
        line = tally.render(tally.tally(LOG))
        for piece in ("10 cabeceras", "2 no aplicadas", "1 declaradas", "7 ejecutadas", "1 que no compilaban", "1 verdes", "4 rojas", "1 sin marcador", "['M1']"):
            self.assertIn(piece, line)


if __name__ == "__main__":
    unittest.main()
