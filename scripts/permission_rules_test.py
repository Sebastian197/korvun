#!/usr/bin/env python3
# Copyright 2026 Sebastián Moreno Saavedra
# SPDX-License-Identifier: Apache-2.0
"""Pins the destructive-command permission rules in .claude/settings.json.

What this proves: the project settings file carries exactly the rules below.
What it does not prove: that Claude Code enforces them. The rules are
evaluated by Claude Code itself; their live evidence (a real session, one
row and one mutation per family) is recorded in the pull request that
introduced them, and is not repeatable here.

The rules cover the forms an agent usually writes and are not a security
boundary: a program invoked in another form (an absolute path, `sh -c`,
`git -C` for anything but force) is not matched. See docs/HANDOFF.md.
"""
import json
from pathlib import Path
import sys
import unittest

sys.dont_write_bytecode = True
ROOT = Path(__file__).resolve().parent.parent
SETTINGS = ROOT / '.claude' / 'settings.json'
NV = "--no-" + "verify"
HP = "core." + "hooks"

DENY = [
    "Bash(git push --force*)", "Bash(git push * --force*)",
    "Bash(git push -f*)", "Bash(git push * -f*)",
    "Bash(git push * +*)",
    "Bash(git push --mirror*)", "Bash(git push * --mirror*)",
    "Bash(git push --delete*)", "Bash(git push * --delete*)",
    "Bash(git push -d *)", "Bash(git push * -d *)",
    "Bash(git push --tags*)", "Bash(git push * --tags*)",
    "Bash(git push * v0.*)", "Bash(git push * v1.*)",
    "Bash(git -C * push --force*)", "Bash(git -C * push * --force*)",
    "Bash(git -C * push -f*)", "Bash(git -C * push * -f*)",
    "Bash(git commit *" + NV + "*)", "Bash(git push *" + NV + "*)",
    "Bash(git commit -n*)", "Bash(git commit * -n*)",
    "Bash(git * " + HP + "Path*)", "Bash(git * " + HP + "path*)",
    "Bash(gh pr merge*)",
    "Bash(gh release create*)", "Bash(gh release edit*)", "Bash(gh release delete*)",
    "Bash(gh repo delete*)",
    "Bash(gh api -X*)", "Bash(gh api * -X*)",
    "Bash(gh api --method*)", "Bash(gh api * --method*)",
    "Bash(gh api -f *)", "Bash(gh api * -f *)",
    "Bash(gh api -F *)", "Bash(gh api * -F *)",
    "Bash(gh api --field*)", "Bash(gh api * --field*)",
    "Bash(gh api --raw-field*)", "Bash(gh api * --raw-field*)",
    "Bash(gh api --input*)", "Bash(gh api * --input*)",
    "mcp__plugin_github_github__merge_pull_request",
    "mcp__plugin_github_github__delete_repository",
]

ASK = [
    "Bash(rm -r*)", "Bash(rm -R*)", "Bash(rm -fr*)", "Bash(rm -fR*)", "Bash(rm --recursive*)",
    "Bash(rm * -r*)", "Bash(rm * -R*)",
    "Bash(find * -delete*)", "Bash(find * -exec rm*)",
    "Bash(git reset *--ha*)",
    "Bash(git clean *)",
    "Bash(git checkout -- *)", "Bash(git checkout * -- *)", "Bash(git checkout .*)", "Bash(git checkout -f*)",
    "Bash(git restore *)",
    "Bash(git switch -f*)", "Bash(git switch *--discard-changes*)",
    "Bash(git branch -D*)", "Bash(git branch * -D*)", "Bash(git branch *--force*)",
    "Bash(git branch -f*)", "Bash(git branch * -f *)",
    "Bash(git stash drop*)", "Bash(git stash clear*)",
    "Bash(git worktree remove -f*)", "Bash(git worktree remove * -f*)", "Bash(git worktree remove *--force*)",
    "Bash(git update-ref -d*)", "Bash(git tag -d*)",
    "Bash(git filter-branch*)", "Bash(git filter-repo*)",
    "Bash(git reflog expire*)", "Bash(git gc *--prune*)",
]


class PermissionRulesTests(unittest.TestCase):
    def setUp(self):
        settings = json.loads(SETTINGS.read_text())
        self.assertIn('permissions', settings, 'no permissions block in .claude/settings.json')
        self.permissions = settings['permissions']

    def test_deny_rules_are_exactly_the_pinned_set(self):
        deny = self.permissions['deny']
        self.assertEqual(len(deny), len(set(deny)), 'duplicate deny rule')
        self.assertEqual(sorted(deny), sorted(DENY))

    def test_ask_rules_are_exactly_the_pinned_set(self):
        ask = self.permissions['ask']
        self.assertEqual(len(ask), len(set(ask)), 'duplicate ask rule')
        self.assertEqual(sorted(ask), sorted(ASK))

    def test_permissions_carry_only_deny_and_ask(self):
        # Nothing else rides in the block: no allow (each user's local file
        # holds those), no defaultMode, no additionalDirectories.
        self.assertEqual(sorted(self.permissions), ['ask', 'deny'])


if __name__ == '__main__':
    unittest.main()
