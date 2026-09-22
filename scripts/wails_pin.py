#!/usr/bin/env python3
# Copyright 2026 Sebastián Moreno Saavedra
# SPDX-License-Identifier: Apache-2.0
"""Assert that every build-time mention of Wails names the version go.mod pins.

ADR-0036's amendment of 2026-09-19 states the rule this enforces: the library
and the build-time CLI must name one version. Nothing enforced it, so the rule
was broken silently twice — once by Dependabot #34 and again by #56, each time
leaving `release-desktop.yml` installing a CLI one minor behind the library the
published desktop artifacts link against.

Two separate checks, because they fail for different reasons and an operator
should be told which one fired:

  1. INSTALL PINS, over the governed files plus `scripts/` and any
     `Dockerfile*`. Every `.../cmd/wails@vX.Y.Z` must equal
     go.mod's `github.com/wailsapp/wails/v2` version. This is the executable
     check: it decides which binary builds the artifacts users download, so it
     is hunted across every surface that could install one.
  2. BUILD-FILE LETREROS, over the governed files only — `Makefile`,
     `.github/workflows/`, `.github/actions/`. Every Wails version there must equal it
     too. A step labelled "Install wails CLI v2.15.0" beside a `@v2.16.0`
     install is a public-truth violation even when the build is correct.

In a build file a version string is an instruction, not prose: a Wails version
other than the pinned one is a failure there even when it is meant as history
or as contrast (a `v3` line included). Prose is another matter and is
deliberately OUT of reach — `docs/adr/0036-wails-v2-dependency.md` records
v2.13.0 and v2.15.0 as history and must keep naming them. Documentation is
covered by review, not by this script.

Fails CLOSED. Each of these is a failure, never a pass: an unreadable go.mod; a
go.mod naming Wails zero times or more than once; a `replace` directive for
Wails, because the effective version is then not the one the require line
states; an absent `Makefile`; an unreadable file anywhere on a scanned surface;
and a tree carrying no install pin at all. A guard that cannot read its own
inputs must not report "no drift".

What it does NOT see, declared rather than implied: a pin assembled at run time
rather than written literally — split across a shell line continuation, or
composed from an `env:`/`matrix` value — and a version named in a makefile
reached through `include` rather than in `Makefile` itself. Those shapes are
named in the canto and remain the reviewer's job.
"""
import re
import sys
from pathlib import Path

LIBRARY_MODULE = "github.com/wailsapp/wails/v2"
CLI_MODULE = "github.com/wailsapp/wails/v2/cmd/wails"

# The module path, optionally preceded by `require`, followed by its version.
# The MANDATORY whitespace is what keeps the CLI's own path
# (`.../wails/v2/cmd/wails`) from being read as the library's: after `/v2` it
# carries a `/`, not a space. A negative lookahead was written here first to do
# that job; it could not be reddened by any input, because this `\s+` already
# does it, so it was deleted rather than kept as decoration.
_GO_MOD_RE = re.compile(
    r"^\s*(?:require\s+)?" + re.escape(LIBRARY_MODULE) + r"\s+(v\S+)",
    re.MULTILINE,
)
# A `replace` for Wails, in either the single-line or the block form, with or
# without a version on the left-hand side. `go list -m` would report the
# replacement; this file reads text, so the honest answer is to refuse.
_REPLACE_RE = re.compile(
    r"^\s*(?:replace\s+)?" + re.escape(LIBRARY_MODULE) + r"\s+(?:v\S+\s+)?=>",
    re.MULTILINE,
)
_INSTALL_PIN_RE = re.compile(re.escape(CLI_MODULE) + r"@(v?\d[\w.+-]*)")
# A version ATTACHED to the word `wails`, never merely sharing its line. The
# two alternatives are the only shapes the tree uses: `wails@v…` / `wails/v2@v…`
# for a module citation, and `wails v…` / `wails CLI v…` for a label. Requiring
# the attachment is what keeps an unrelated tag on the same line — a
# `wailsapp/setup-action@v2.3.1`, a goreleaser image — from being read as a
# Wails version. The version is captured WHOLE, suffix included, so a
# `v2.16.0-rc1` is never reported against its own prefix.
_LETRERO_RE = re.compile(
    r"wails(?:/v2)?@(v?\d[\w.+-]*)"
    r"|wails(?:\s+CLI)?\s+(v?\d[\w.+-]*)",
    re.IGNORECASE,
)


class GuardError(ValueError):
    """An input this guard cannot read. Always a failure, never a pass."""


def _normalise(version: str) -> str:
    """Compare `2.16.0` and `v2.16.0` as one version, report either verbatim."""
    return version[1:] if version.startswith(("v", "V")) else version


def library_version(root: Path) -> str:
    """Return the Wails version go.mod pins, or raise GuardError."""
    try:
        text = (root / "go.mod").read_text(encoding="utf-8")
    except OSError as err:
        raise GuardError(f"go.mod is unreadable: {err}") from err
    if _REPLACE_RE.search(text):
        raise GuardError(
            f"go.mod carries a `replace` for {LIBRARY_MODULE}; the effective "
            "version is not the one the require line states, and this guard "
            "reads text rather than asking the module graph"
        )
    found = _GO_MOD_RE.findall(text)
    if len(found) != 1:
        raise GuardError(
            f"go.mod names {LIBRARY_MODULE} {len(found)} times; expected exactly one"
        )
    return found[0]


def _yaml_under(directory: Path) -> list[Path]:
    if not directory.is_dir():
        return []
    return sorted(p for p in directory.rglob("*") if p.suffix in (".yml", ".yaml") and p.is_file())


def governed_files(root: Path) -> list[Path]:
    """Build files where BOTH checks apply: install pins and human labels."""
    return [root / "Makefile"] + _yaml_under(root / ".github" / "workflows") \
        + _yaml_under(root / ".github" / "actions")


def pin_only_files(root: Path) -> list[Path]:
    """Surfaces beyond the governed ones where only an INSTALL PIN is judged.

    `scripts/` is here rather than among the governed files because this
    guard's own attack fixtures live there and name old versions by necessity.
    Their install pins are still hunted, and that is not a loophole — the
    fixtures compose their pins from `CLI_MODULE` rather than spelling one, so
    the surface stays wide without an exemption. The first shape of those
    fixtures did spell them, and the guard went red over its own tests; the
    fixtures moved, the surface did not.

    The two lists are DISJOINT on purpose. An earlier shape had the pin surface
    contain the governed one, and a mutation that narrowed the pin surface left
    a row green because the file was still reached through the other list — the
    guarantee sat in a function that did not name it. Twice in this train.
    """
    files = []
    scripts = root / "scripts"
    if scripts.is_dir():
        files.extend(sorted(p for p in scripts.rglob("*") if p.is_file()))
    files.extend(sorted(p for p in root.glob("Dockerfile*") if p.is_file()))
    return files


def _read(path: Path) -> str:
    """Read a scanned file, or raise GuardError. Never returns an empty guess."""
    try:
        return path.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError) as err:
        raise GuardError(f"{path.name} is unreadable: {err}") from err


def scan(root: Path) -> tuple[list[str], list[str]]:
    """Return (install-pin failures, letrero failures) against go.mod's version."""
    want = library_version(root)
    makefile = root / "Makefile"
    if not makefile.is_file():
        raise GuardError(
            "Makefile is absent or is not a regular file; one of the governed "
            "sites lives in it, and its disappearance is a signal rather than "
            "an exemption"
        )

    pins: list[str] = []
    letreros: list[str] = []
    seen_pin = False
    governed = governed_files(root)
    governed_set = {p.resolve() for p in governed}

    for path in governed + pin_only_files(root):
        if not path.is_file():
            continue
        rel = path.relative_to(root).as_posix()  # stable on the Windows runner too
        judge_letreros = path.resolve() in governed_set
        for number, line in enumerate(_read(path).splitlines(), 1):
            for version in _INSTALL_PIN_RE.findall(line):
                seen_pin = True
                if _normalise(version) != _normalise(want):
                    pins.append(
                        f"{rel}:{number}: installs the Wails CLI {version}, "
                        f"but go.mod pins the library {want}"
                    )
            if not judge_letreros:
                continue
            for attached, spaced in _LETRERO_RE.findall(line):
                version = attached or spaced
                if _normalise(version) != _normalise(want):
                    letreros.append(
                        f"{rel}:{number}: names Wails {version}, "
                        f"but go.mod pins {want}"
                    )

    if not seen_pin:
        pins.append(
            f"no `{CLI_MODULE}@v…` install pin found on any scanned surface; "
            "the desktop release lane must install a pinned CLI"
        )
    return pins, letreros


def main(argv: list[str] | None = None) -> int:
    """The production door. 0 clean, 1 drift, 2 an input the guard cannot read."""
    args = sys.argv[1:] if argv is None else argv
    root = Path(args[0] if args else ".").resolve()
    try:
        want = library_version(root)
        pins, letreros = scan(root)
    except GuardError as err:
        print(f"FAIL: wails pin guard cannot read its inputs: {err}", file=sys.stderr)
        return 2
    if pins or letreros:
        for line in pins:
            print(f"FAIL: wails CLI pin drift: {line}", file=sys.stderr)
        for line in letreros:
            print(f"FAIL: stale wails letrero: {line}", file=sys.stderr)
        print(
            "ADR-0036: the library and the build-time CLI name one version. "
            "Move the pin and its labels together with the go.mod bump.",
            file=sys.stderr,
        )
        return 1
    print(
        f"wails pin guard: go.mod pins {want}, and so does every install pin "
        "and every label on the scanned surfaces (Makefile, .github/workflows/, "
        ".github/actions/, scripts/, Dockerfile*)."
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
