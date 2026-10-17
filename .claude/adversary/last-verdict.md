VETO LEVANTADO 741dc2f9d8aee57e5ff1496ca2cf0203858f0b7c

Round: the destructive-command permission rules train (2026-10-17),
ordered by the director: rm in ask, project settings only.
Legacy marker, as for #81: the versioned procedure of
docs/INTEGRATION.md cannot be followed without force for a new train
(P2 adjudicated 2026-10-10). After rebase this marker names a rewritten
SHA; the director repairs master's marker by hand.

WHAT THE COMMIT IS. The project's agent settings file gains permissions.deny (46:
44 Bash patterns + GitHub MCP merge_pull_request and delete_repository)
and permissions.ask (34). scripts/permission_rules_test.py pins the
exact set and that the block carries only deny and ask; it runs in
make quality (permission-rules-probe) and in quality.yml. docs/HANDOFF.md
records rules, evidence and limits.

EVIDENCE.
- Static probe: red before the rules (no permissions block, 3 failures);
  green after; on the final tree seven mutations all red (drop a deny,
  drop an ask, duplicate, move ask to deny, add allow, add defaultMode,
  remove the block).
- Live, real headless agent sessions (print mode, small model), throwaway
  project, local bare remote, --setting-sources project (no hooks),
  default mode, plus allow Bash(*) and --allowedTools Bash so unruled
  commands run: one row per rule, 44 deny rows show the deny text and
  34 ask rows the ask text; one mutation per rule, with only that rule
  removed each of the 78 commands executed. Compound rows: 5 refused,
  1 asked. Routine rows: 6 ran. Not covered, captured running: /bin/rm
  -rf, rm -vrf, git -c x.y=z push --force-with-lease, git push -uf,
  git push origin :feat, git push origin refs/tags/v0.0.1,
  gh api -fx=y. MCP: both tools absent with the rule, present without
  (user+project settings, where the plugin is enabled).
- Level: real agent sessions; not repeatable in make quality.
- make quality green in the pre-commit gate.

THE ADVERSARY. Paper: draft 1 (custom hook) VETO, nine P2; the director
chose permission rules; draft 3 VETO (P1 flag-first forms, three P2);
draft 4 VETO LEVANTADO. Implementation: VETO, four P2 (a HANDOFF
sentence false against adversary-gate.sh; 34 rules without a live row;
HANDOFF wider than the patterns; an APFS case collision in the
mutation harness) and ten P3; cured by the per-rule table and the
rewritten HANDOFF; scoped re-pass VETO LEVANTADO.
Finding of the live run: Bash(git push * :*) never matched (the
command deleted the lab's remote branch); removed, declared.
Director's decision left open: the agent can no longer push a release tag.
