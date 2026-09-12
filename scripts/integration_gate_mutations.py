#!/usr/bin/env python3
# Copyright 2026 Sebastián Moreno Saavedra
# SPDX-License-Identifier: Apache-2.0
"""Run scoped destructive probes in temporary copies; never edit the checkout."""
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parent
# One dangerous guard is removed per mutant; the named attack must detect it.
MUTATIONS = [
    ("rename", "[path] + ([previous] if previous is not None else [])", "[path]", "late_rename"),
    ("unknown", "else:\n                lanes.update(LANES)", "else:\n                pass", "unknown_and_shared"),
    ("file-count", "len(files) == expected and expected <= 3000", "True", "late_rename"),
    ("path", '".." not in candidate.split("/")', "True", "unreadable_or_lying"),
    ("failed-job", 'if job.get("conclusion") == "failure":', "if False:", "failed_website"),
    ("duplicate", 'len(matches) == 1', "True", "missing_duplicate"),
    ("job-sha", 'job.get("head_sha") == head', "True", "missing_duplicate"),
    ("step", 'benign_skip or (step.get("status") == "completed" and step.get("conclusion") == "success")', "True", "skipped_mandatory"),
    ("custodian", 'r.get("user", {}).get("id") in policy.get("custodian_ids", [])', "True", "outsiders_cannot"),
    ("digest", 'hashlib.sha256(report.encode()).hexdigest() == body.get("report_sha256")', "True", "evidence_custody"),
    ("findings", 'type(body.get("p1")) is int and body["p1"] == 0', "True", "evidence_custody"),
    ("governance", 'body.get("governance_scope") == identity["scope"]', "True", "evidence_custody"),
    ("review-binding", 'body.get("candidate") == identity', "True", "stale_bindings"),
    ("review-state", 'record.get("state") == "COMMENTED" and record.get("submitted_at")', "True", "stale_bindings"),
    ("public-docs", 'lanes.add("website")\n                if not candidate.endswith', 'pass\n                if not candidate.endswith', "unknown_and_shared"),
    ("capture", 'before == after', "True", "changed_capture"),
    ("latest-run", 'max(eligible, key=lambda item: item["id"])', 'min(eligible, key=lambda item: item["id"])', "newer_cancelled"),
    ("run-event", 'run.get("event") == "pull_request"', "True", "newer_cancelled"),
    ("page-total", 'data.get("total_count") == len(result)', "True", "pagination_never"),
    ("credential-sink", 'base == "https://api.github.com"', "True", "credential_sink"),
    ("final-read", 'check_stable(before, after)\n    return dict', 'pass\n    return dict', "final_collection"),
    ("sha-syntax", 'all(re.fullmatch(r"[0-9a-f]{40}", sha) for sha in (args.head, args.base))', "True", "cli_rejects"),
    ("deadline", 'remaining > 0', "True", "deadline_prevents"),
    ("changes-requested", '"CHANGES_REQUESTED" not in decisions.values()', "True", "later_comment"),
    ("outsider-veto", 'trusted_blocker and review.get', 'True and review.get', "outsiders_cannot"),
    ("foreign-run", 'eligible = [run for run in runs if matches(run)]', 'eligible = runs', "foreign_pr_run"),
    ("duplicate-run", 'len({run["id"] for run in eligible}) == len(eligible)', 'True', "foreign_pr_run"),
]


def main():
    original = (ROOT / "integration_gate.py").read_text()
    results = []
    for name, source, replacement, test in MUTATIONS:
        if original.count(source) != 1:
            raise SystemExit(f"Mutation site {name} is not unique")
        with tempfile.TemporaryDirectory(prefix="korvun-integration-mutant-") as directory:
            target = Path(directory)
            for filename in ("integration_gate_test.py", "integration-policy.json"):
                shutil.copyfile(ROOT / filename, target / filename)
            (target / "integration_gate.py").write_text(original.replace(source, replacement, 1))
            run = subprocess.run([sys.executable, str(target / "integration_gate_test.py"), "-k", test],
                                 text=True, capture_output=True, timeout=20)
            killed = run.returncode != 0 and ("FAILED (" in run.stderr)
            results.append(dict(mutation=name, test=test, detected=killed, exit_code=run.returncode,
                                output=run.stdout + run.stderr))
    print(json.dumps(dict(source_sha256=hashlib.sha256(original.encode()).hexdigest(),
                         evidence_level="unit and separate Python CLI process; no GitHub merge", results=results), indent=2))
    return 0 if all(result["detected"] for result in results) else 1


if __name__ == "__main__":
    sys.exit(main())
