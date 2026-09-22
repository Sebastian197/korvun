# Copyright 2026 Sebastián Moreno Saavedra
# SPDX-License-Identifier: Apache-2.0
"""Attack tests for the Wails pin guard. Synthetic trees, never the real one.

Evidence level, honest: in-process, over temporary directories shaped like the
repository. The guard's behaviour against the REAL tree is proved by the gate
step that runs it, not by these rows.

The rows that matter most enter through `main()` — the production door, whose
exit code is the whole of what "it reddens the pull request" means. An earlier
shape of this suite called only `scan()`, and neutralising `main()`'s returns
left every row green while a fully drifted tree passed. That gap is what P1-1
of the 2026-09-22 adversarial pass named.
"""
import contextlib
import importlib.util
import io
from pathlib import Path
import sys
import tempfile
import unittest

sys.dont_write_bytecode = True

SPEC = importlib.util.spec_from_file_location("wails_pin", Path(__file__).with_name("wails_pin.py"))
wails_pin = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(wails_pin)

GO_MOD = """module github.com/Sebastian197/korvun

go 1.26.6

require (
\tgithub.com/wailsapp/wails/v2 {version}
\tmodernc.org/sqlite v1.59.0
)
"""

WORKFLOW = """name: Release Desktop
jobs:
  build:
    steps:
      # wails' own apt requirements, read at the source
      # (wails/v2@{comment}/internal/system/packagemanager/apt.go): libgtk-3-dev,
      - name: Install wails CLI {label}
        run: go install github.com/wailsapp/wails/v2/cmd/wails@{pin}
"""

MAKEFILE = "# Linux: wails {comment} gates its webkit2gtk detection\ndesktop:\n\t@true\n"


class Tree:
    """A temporary directory shaped like the repository's build surface."""

    def __init__(self, *, library="v2.16.0", pin="v2.16.0", label="v2.16.0",
                 workflow_comment="v2.16.0", makefile_comment="v2.16.0",
                 go_mod=None, with_workflow=True, with_makefile=True):
        self.dir = tempfile.TemporaryDirectory()
        root = Path(self.dir.name)
        root.joinpath("go.mod").write_text(
            GO_MOD.format(version=library) if go_mod is None else go_mod, encoding="utf-8")
        if with_makefile:
            root.joinpath("Makefile").write_text(
                MAKEFILE.format(comment=makefile_comment), encoding="utf-8")
        workflows = root / ".github" / "workflows"
        workflows.mkdir(parents=True)
        if with_workflow:
            workflows.joinpath("release-desktop.yml").write_text(
                WORKFLOW.format(pin=pin, label=label, comment=workflow_comment), encoding="utf-8")
        self.root = root

    def add(self, relative: str, body: str) -> Path:
        path = self.root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(body, encoding="utf-8")
        return path

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        self.dir.cleanup()
        return False


def pin_line(version: str) -> str:
    """Compose an install pin without spelling one literally in this file.

    `scripts/` is on the guard's install-pin surface, so a fixture that wrote
    `.../cmd/wails@v2.15.0` verbatim would make the guard red over its own
    attack tests. Composing it keeps the surface honest and needs no exemption
    in the production code. Found by running the guard over the tree after
    these rows were added — it reported four drifts, all of them here.
    """
    return "go install " + wails_pin.CLI_MODULE + "@" + version


def run_main(root: Path) -> tuple[int, str, str]:
    """Drive the production door and capture its code and both streams."""
    out, err = io.StringIO(), io.StringIO()
    with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
        code = wails_pin.main([str(root)])
    return code, out.getvalue(), err.getvalue()


class TheProductionDoor(unittest.TestCase):
    """P1-1's cure: every row here observes the exit code, not a return value."""

    def test_aDriftedTreeExitsOneAndNamesThePinOnStderr(self):
        with Tree(library="v2.16.0", pin="v2.15.0", label="v2.15.0",
                  workflow_comment="v2.15.0", makefile_comment="v2.15.0") as tree:
            code, stdout, stderr = run_main(tree.root)
            self.assertEqual(1, code)
            self.assertIn("FAIL: wails CLI pin drift:", stderr)
            self.assertIn("installs the Wails CLI v2.15.0", stderr)
            self.assertIn("ADR-0036:", stderr)
            self.assertEqual("", stdout)

    def test_aMatchedTreeExitsZeroAndSaysNothingOnStderr(self):
        with Tree() as tree:
            code, stdout, stderr = run_main(tree.root)
            self.assertEqual(0, code)
            self.assertEqual("", stderr)
            self.assertIn("go.mod pins v2.16.0", stdout)

    def test_anUnreadableInputExitsTwoAndIsDistinctFromDrift(self):
        """2 and 1 are different answers: 'I cannot read' is not 'it drifted'."""
        with Tree() as tree:
            tree.root.joinpath("go.mod").unlink()
            code, stdout, stderr = run_main(tree.root)
            self.assertEqual(2, code)
            self.assertIn("cannot read its inputs", stderr)
            self.assertNotIn("pin drift", stderr)
            self.assertEqual("", stdout)

    def test_aLetreroOnlyDriftStillExitsOne(self):
        """A half repair must redden the gate, not only the report."""
        with Tree(pin="v2.16.0", label="v2.15.0",
                  workflow_comment="v2.15.0", makefile_comment="v2.15.0") as tree:
            code, _, stderr = run_main(tree.root)
            self.assertEqual(1, code)
            self.assertIn("FAIL: stale wails letrero:", stderr)
            self.assertNotIn("pin drift", stderr)


class WailsPinAttacks(unittest.TestCase):
    # --- THE FAULT: the state Dependabot #34 and #56 each left behind ---------

    def test_theCliOneMinorBehindTheLibraryIsCaughtAsAPin(self):
        """The executable drift: the CLI that builds the artifacts is not the library's."""
        with Tree(library="v2.16.0", pin="v2.15.0", label="v2.15.0",
                  workflow_comment="v2.15.0", makefile_comment="v2.15.0") as tree:
            pins, letreros = wails_pin.scan(tree.root)
            self.assertEqual(1, len(pins), pins)
            self.assertIn("installs the Wails CLI v2.15.0", pins[0])
            self.assertIn("go.mod pins the library v2.16.0", pins[0])
            # Four labels, not three: the `go install` line is itself a letrero
            # as well as the pin, and is reported under both headings.
            self.assertEqual(4, len(letreros), letreros)
            self.assertTrue(any(line.startswith(".github/workflows/release-desktop.yml:8:")
                                for line in letreros), letreros)

    # --- THE CONTROL: a matched tree is silent -------------------------------

    def test_aMatchedTreePassesAndSaysWhichVersion(self):
        with Tree() as tree:
            self.assertEqual(([], []), wails_pin.scan(tree.root))
            self.assertEqual("v2.16.0", wails_pin.library_version(tree.root))

    def test_aHistoricalVersionInProseIsNotTheGuardsBusiness(self):
        """The ADR must keep naming v2.13.0 and v2.15.0; only build files are governed."""
        with Tree() as tree:
            tree.add("docs/adr/0036-wails-v2-dependency.md",
                     "Adopt wails v2.13.0, later amended to wails v2.15.0.")
            self.assertEqual(([], []), wails_pin.scan(tree.root))

    def test_anUnrelatedTagOnAWailsLineIsNotAWailsVersion(self):
        """A version merely SHARING the line is not one attached to the word."""
        with Tree() as tree:
            tree.add(".github/workflows/extra.yml",
                     "steps:\n  - uses: wailsapp/setup-action@v2.3.1\n"
                     "  - run: docker pull goreleaser/goreleaser:v2.18.0\n")
            self.assertEqual(([], []), wails_pin.scan(tree.root))

    # --- THE REPAIR: moving the pin alone is NOT the repair -------------------

    def test_movingThePinButNotTheLabelsIsStillCaught(self):
        """A half-repair leaves a step labelled for a CLI it no longer installs."""
        with Tree(library="v2.16.0", pin="v2.16.0", label="v2.15.0",
                  workflow_comment="v2.15.0", makefile_comment="v2.15.0") as tree:
            pins, letreros = wails_pin.scan(tree.root)
            self.assertEqual([], pins)
            self.assertEqual(3, len(letreros), letreros)
            self.assertTrue(all("names Wails v2.15.0" in line for line in letreros), letreros)

    def test_movingThePinAndEveryLabelClearsTheGuard(self):
        with Tree(library="v2.16.0", pin="v2.16.0", label="v2.16.0",
                  workflow_comment="v2.16.0", makefile_comment="v2.16.0") as tree:
            self.assertEqual(([], []), wails_pin.scan(tree.root))

    # --- THE SURFACE: an install pin is hunted wherever it can be written ----

    def test_aPinInACompositeActionIsCaught(self):
        with Tree() as tree:
            tree.add(".github/actions/setup-wails/action.yml",
                     "runs:\n  steps:\n    - run: " + pin_line("v2.15.0") + "\n")
            pins, _ = wails_pin.scan(tree.root)
            self.assertEqual(1, len(pins), pins)
            self.assertIn(".github/actions/setup-wails/action.yml", pins[0])

    def test_aPinInAScriptIsCaught(self):
        with Tree() as tree:
            tree.add("scripts/install-wails.sh",
                     "#!/bin/sh\n" + pin_line("v2.13.0") + "\n")
            pins, letreros = wails_pin.scan(tree.root)
            self.assertEqual(1, len(pins), pins)
            self.assertIn("installs the Wails CLI v2.13.0", pins[0])
            # scripts/ is outside the letrero surface, by the stated reason.
            self.assertEqual([], letreros)

    def test_aPinInADockerfileIsCaught(self):
        with Tree() as tree:
            tree.add("Dockerfile", "FROM golang\nRUN " + pin_line("v2.11.0") + "\n")
            pins, _ = wails_pin.scan(tree.root)
            self.assertEqual(1, len(pins), pins)
            self.assertIn("installs the Wails CLI v2.11.0", pins[0])

    def test_aSecondWorkflowDisagreeingWithTheFirstIsCaught(self):
        with Tree() as tree:
            tree.add(".github/workflows/canary.yaml",
                     "steps:\n  - run: " + pin_line("v2.13.0") + "\n")
            pins, letreros = wails_pin.scan(tree.root)
            self.assertEqual(1, len(pins), pins)
            self.assertIn("canary.yaml", pins[0])
            self.assertEqual(1, len(letreros), letreros)

    # --- THE VERSION IS READ WHOLE ------------------------------------------

    def test_aConsistentPrereleaseTreeIsNotReportedAgainstItsOwnPrefix(self):
        """A suffixed version compared against its own prefix is a false red."""
        with Tree(library="v2.16.0-rc1", pin="v2.16.0-rc1", label="v2.16.0-rc1",
                  workflow_comment="v2.16.0-rc1", makefile_comment="v2.16.0-rc1") as tree:
            self.assertEqual(([], []), wails_pin.scan(tree.root))

    def test_aLabelWithNoLeadingVIsStillJudged(self):
        with Tree(label="2.15.0") as tree:
            _, letreros = wails_pin.scan(tree.root)
            self.assertEqual(1, len(letreros), letreros)
            self.assertIn("names Wails 2.15.0", letreros[0])

    def test_aLabelWithNoLeadingVThatMatchesIsAccepted(self):
        with Tree(label="2.16.0") as tree:
            self.assertEqual(([], []), wails_pin.scan(tree.root))

    # --- FAIL CLOSED: an unreadable input is never a pass --------------------

    def test_anUnreadableGoModIsAFailureNotAnEmptyAnswer(self):
        with Tree() as tree:
            tree.root.joinpath("go.mod").unlink()
            with self.assertRaises(wails_pin.GuardError) as caught:
                wails_pin.scan(tree.root)
            self.assertIn("unreadable", str(caught.exception))

    def test_aReplaceDirectiveIsAFailureAndSaysWhy(self):
        """`go list -m` would report the replacement; this guard reads text."""
        replaced = GO_MOD.format(version="v2.16.0") + (
            "\nreplace github.com/wailsapp/wails/v2 => github.com/wailsapp/wails/v2 v2.13.0\n")
        with Tree(go_mod=replaced) as tree:
            with self.assertRaises(wails_pin.GuardError) as caught:
                wails_pin.scan(tree.root)
            self.assertIn("`replace`", str(caught.exception))

    def test_aReplaceInsideABlockIsAlsoAFailure(self):
        blocked = GO_MOD.format(version="v2.16.0") + (
            "\nreplace (\n\tgithub.com/wailsapp/wails/v2 v2.16.0 => ../local-wails\n)\n")
        with Tree(go_mod=blocked) as tree:
            with self.assertRaises(wails_pin.GuardError) as caught:
                wails_pin.scan(tree.root)
            self.assertIn("`replace`", str(caught.exception))

    def test_aGoModThatNamesWailsTwiceIsAFailure(self):
        doubled = GO_MOD.format(version="v2.16.0").replace(
            "\tmodernc.org/sqlite v1.59.0",
            "\tgithub.com/wailsapp/wails/v2 v2.15.0")
        with Tree(go_mod=doubled) as tree:
            with self.assertRaises(wails_pin.GuardError) as caught:
                wails_pin.scan(tree.root)
            self.assertIn("2 times", str(caught.exception))

    def test_aGoModThatNamesWailsNeverIsAFailure(self):
        with Tree(go_mod="module x\n\ngo 1.26.6\n") as tree:
            with self.assertRaises(wails_pin.GuardError) as caught:
                wails_pin.scan(tree.root)
            self.assertIn("0 times", str(caught.exception))

    def test_anAbsentMakefileIsAFailureNotAnExemption(self):
        """One governed site lives in it; its disappearance retires that site."""
        with Tree(with_makefile=False) as tree:
            with self.assertRaises(wails_pin.GuardError) as caught:
                wails_pin.scan(tree.root)
            self.assertIn("Makefile is absent", str(caught.exception))

    def test_anUnreadableScannedFileIsAFailureNotASkip(self):
        with Tree() as tree:
            tree.root.joinpath(".github/workflows/bad.yml").write_bytes(b"wails \xe9\xff\n")
            with self.assertRaises(wails_pin.GuardError) as caught:
                wails_pin.scan(tree.root)
            self.assertIn("bad.yml is unreadable", str(caught.exception))

    def test_aTreeWithNoInstallPinAtAllIsAFailure(self):
        """The release lane losing its pin must redden, not read as no drift."""
        with Tree(with_workflow=False) as tree:
            pins, letreros = wails_pin.scan(tree.root)
            self.assertEqual([], letreros)
            self.assertEqual(1, len(pins), pins)
            self.assertIn("no `github.com/wailsapp/wails/v2/cmd/wails@v…` install pin", pins[0])

    def test_theCliModulePathDoesNotSatisfyTheLibrarysGoModLookup(self):
        """A go.mod naming only the CLI module must not be read as the library's pin."""
        with Tree(go_mod="module x\n\nrequire github.com/wailsapp/wails/v2/cmd/wails v2.16.0\n") as tree:
            with self.assertRaises(wails_pin.GuardError) as caught:
                wails_pin.library_version(tree.root)
            self.assertIn("0 times", str(caught.exception))

    def test_aSingleLineRequireIsReadTheSameAsABlock(self):
        with Tree(go_mod="module x\n\nrequire github.com/wailsapp/wails/v2 v2.16.0\n") as tree:
            self.assertEqual("v2.16.0", wails_pin.library_version(tree.root))


if __name__ == "__main__":
    unittest.main(verbosity=2)
