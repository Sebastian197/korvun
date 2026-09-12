# Copyright 2026 Sebastián Moreno Saavedra
# SPDX-License-Identifier: Apache-2.0
"""Attack tests for the integration reducer. Synthetic data, never review evidence."""
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import unittest
from unittest import mock

sys.dont_write_bytecode = True

SPEC = importlib.util.spec_from_file_location("gate", Path(__file__).with_name("integration_gate.py"))
gate = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(gate)


class IntegrationAttacks(unittest.TestCase):
    def assert_code(self, code, function, *args):
        with self.assertRaises(gate.GateError) as caught:
            function(*args)
        self.assertEqual(code, caught.exception.code)

    def identity(self):
        return dict(repository_id=1, pr=8, head="a" * 40, base="b" * 40,
                    code="c" * 40, tree="d" * 40, scope="e" * 64)

    def evidence(self):
        identity = self.identity()
        report = "Synthetic adversarial report for fixture only."
        body = dict(schema=1, candidate=identity,
                    reviewer="fixture-reviewer", origin="fixture-origin",
                    report=report, report_sha256=hashlib.sha256(report.encode()).hexdigest(),
                    p1=0, p2=0, p3=[], governance_scope=identity["scope"])
        # GET reviews returns COMMENTED (captured actions/checkout PR 2521);
        # COMMENT is the submission event, not the stored state.
        review = dict(id=10, user=dict(id=99), state="COMMENTED", submitted_at="2026-09-12T00:00:00Z",
                      commit_id=identity["head"], body="korvun-review-v1\n" + json.dumps(body))
        policy = dict(custodian_ids=[99], reviewers={"fixture-reviewer": ["fixture-origin"]})
        return review, body, policy

    def test_late_rename_cannot_hide_website(self):
        files = [dict(filename="docs/superpowers/a.md", status="modified")]
        files.append(dict(filename="docs/superpowers/moved.md", previous_filename="website/gone.md", status="renamed"))
        self.assertIn("website", gate.classify(files, 2)[0])
        self.assert_code("INPUT_INCOMPLETE", gate.classify, files[:1], 2)

    def test_unknown_and_shared_paths_cannot_skip_lanes(self):
        for path in ["new-tool.config", "scripts/check.py", "Makefile", "go.mod", "docs/release-facts.json"]:
            with self.subTest(path=path):
                lanes, _, _ = gate.classify([dict(filename=path, status="modified")], 1)
                self.assertEqual(set(gate.LANES), set(lanes))
        lanes, _, _ = gate.classify([dict(filename="docs/internal-notes.md", status="modified")], 1)
        self.assertEqual({"quality", "codeql", "website"}, set(lanes))

    def test_unreadable_or_lying_file_lists_fail(self):
        for files, count, code in [([], 1, "INPUT_INCOMPLETE"), ([], 0, "INPUT_INVALID"),
                                   ([dict(filename="../x", status="modified")], 1, "INPUT_INVALID"),
                                   ([dict(filename="x", status="renamed")], 1, "INPUT_INVALID")]:
            self.assert_code(code, gate.classify, files, count)

    def jobs(self):
        return [dict(name=name, head_sha="a" * 40, status="completed", conclusion="success",
                     steps=[dict(name="Required execution", conclusion="success", status="completed")])
                for name in gate.LANES["website"][1]]

    def test_failed_website_cannot_hide_behind_green_jobs(self):
        jobs = self.jobs()
        jobs[0]["conclusion"] = "failure"
        self.assert_code("CHECK_FAILED", gate.check_jobs, "website", jobs, "a" * 40)

    def test_missing_duplicate_stale_and_nonsuccess_jobs(self):
        self.assert_code("CHECK_MISSING", gate.check_jobs, "website", [], "a" * 40)
        self.assert_code("CHECK_DUPLICATE", gate.check_jobs, "website", self.jobs() * 2, "a" * 40)
        self.assert_code("RUN_STALE", gate.check_jobs, "website", self.jobs(), "f" * 40)
        for conclusion in ["skipped", "neutral", "cancelled", "timed_out", "action_required", None]:
            jobs = self.jobs(); jobs[0]["conclusion"] = conclusion
            self.assert_code("CHECK_NOT_SUCCESS", gate.check_jobs, "website", jobs, "a" * 40)

    def test_skipped_mandatory_step_and_empty_steps_fail(self):
        jobs = self.jobs(); jobs[0]["steps"][0]["conclusion"] = "skipped"
        self.assert_code("STEP_NOT_SUCCESS", gate.check_jobs, "website", jobs, "a" * 40)
        jobs[0]["steps"] = []
        self.assert_code("STEP_MISSING", gate.check_jobs, "website", jobs, "a" * 40)

    def test_evidence_custody_digest_and_findings_attacks(self):
        review, body, policy = self.evidence()
        for field, value, code in [("report_sha256", "0" * 64, "EVIDENCE_DIGEST"),
                                   ("report", "", "EVIDENCE_DIGEST"),
                                   ("reviewer", "impostor", "EVIDENCE_REVIEWER"),
                                   ("origin", "unknown", "EVIDENCE_REVIEWER"),
                                   ("p1", 1, "EVIDENCE_FINDINGS"),
                                   ("p2", True, "EVIDENCE_FINDINGS"),
                                   ("p3", [dict(id="P3-1")], "EVIDENCE_FINDINGS"),
                                   ("governance_scope", "other", "GOVERNANCE_REVIEW_REQUIRED")]:
            altered = copy.deepcopy(body); altered[field] = value
            record = dict(review, body="korvun-review-v1\n" + json.dumps(altered))
            self.assert_code(code, gate.check_evidence, [record], policy, self.identity(), True)
        review["user"]["id"] = 100
        self.assert_code("EVIDENCE_UNAUTHORIZED", gate.check_evidence, [review], policy, self.identity(), True)

    def test_stale_bindings_and_review_state_cannot_authorize(self):
        review, body, policy = self.evidence()
        for field in self.identity():
            changed = copy.deepcopy(body); changed["candidate"][field] = "wrong"
            record = dict(review, body="korvun-review-v1\n" + json.dumps(changed))
            self.assert_code("EVIDENCE_STALE", gate.check_evidence, [record], policy, self.identity(), False)
        for state in ["PENDING", "DISMISSED", "CHANGES_REQUESTED", "APPROVED", "COMMENT"]:
            self.assert_code("EVIDENCE_STATE", gate.check_evidence, [dict(review, state=state)], policy, self.identity(), False)
        self.assert_code("EVIDENCE_MISSING", gate.check_evidence, [], policy, self.identity(), False)
        self.assert_code("EVIDENCE_REVIEWER", gate.check_evidence, [review], dict(policy, reviewers={}), self.identity(), False)

    def test_later_comment_cannot_erase_request_for_changes(self):
        review, _, policy = self.evidence()
        policy["blocking_reviewer_ids"] = [101]
        request = dict(id=9, user=dict(id=101), state="CHANGES_REQUESTED", body="Fix the CI bypass.")
        comment = dict(id=11, user=dict(id=101), state="COMMENTED", body="Still investigating.")
        self.assert_code("REVIEW_CHANGES_REQUESTED", gate.check_evidence,
                         [request, review, comment], policy, self.identity(), False)

    def test_outsiders_cannot_veto_authorized_custody(self):
        review, _, policy = self.evidence()
        forged = dict(review, id=11, user=dict(id=123))
        request = dict(id=12, user=dict(id=123), state="CHANGES_REQUESTED", body="Untrusted request")
        for outsider in [forged, request]:
            self.assertEqual(10, gate.check_evidence([review, outsider], policy, self.identity(), False))
        revoked = dict(review, id=13, state="DISMISSED")
        self.assert_code("EVIDENCE_STATE", gate.check_evidence,
                         [review, forged, revoked], policy, self.identity(), False)

    def test_changed_capture_fails_even_with_same_head(self):
        original = dict(head="a", base="b", reviews=[dict(id=1, body="original")], runs=[dict(id=1, run_attempt=1)])
        for change in [dict(base="c"), dict(reviews=[dict(id=1, body="edited")]),
                       dict(runs=[dict(id=1, run_attempt=2)])]:
            self.assert_code("CAPTURE_CHANGED", gate.check_stable, original, dict(original, **change))

    def test_newer_cancelled_attempt_cannot_reuse_old_green(self):
        old = dict(id=1, workflow_id=2, path=".github/workflows/website-pr.yml", event="pull_request",
                   head_sha="a" * 40, repository=dict(id=1), pull_requests=[dict(number=8)], run_attempt=1)
        new = dict(old, id=2, run_attempt=2, conclusion="cancelled")
        self.assertEqual(new, gate.select_run([old, new], 2, "website-pr.yml", 1, 8, "a" * 40))
        for field, value in [("workflow_id", 9), ("path", "other.yml"), ("event", "push"),
                             ("head_sha", "b" * 40), ("repository", dict(id=9)), ("pull_requests", [])]:
            self.assert_code("RUN_SOURCE_MISMATCH", gate.select_run, [dict(old, **{field: value})],
                             2, "website-pr.yml", 1, 8, "a" * 40)

    def test_foreign_pr_run_cannot_displace_current_pr(self):
        valid = dict(id=1, workflow_id=2, path=".github/workflows/website-pr.yml", event="pull_request",
                     head_sha="a" * 40, repository=dict(id=1), pull_requests=[dict(number=8)], run_attempt=1)
        foreign = dict(valid, id=2, pull_requests=[dict(number=9)])
        self.assertEqual(valid, gate.select_run([valid, foreign], 2, "website-pr.yml", 1, 8, "a" * 40))
        self.assert_code("RUN_SOURCE_MISMATCH", gate.select_run, [foreign], 2, "website-pr.yml", 1, 8, "a" * 40)
        self.assert_code("RUN_AMBIGUOUS", gate.select_run, [valid, dict(valid, run_attempt=2)],
                         2, "website-pr.yml", 1, 8, "a" * 40)

    def test_pagination_never_returns_partial_success(self):
        api = gate.API("fixture")
        api.get = mock.Mock(side_effect=[[{}] * 100, gate.GateError("INPUT_UNAVAILABLE", "page 2 denied")])
        self.assert_code("INPUT_UNAVAILABLE", api.pages, "/repos/a/b/pulls/1/files")
        self.assertEqual(2, api.get.call_count)
        api.get = mock.Mock(return_value=dict(total_count=2, jobs=[{}]))
        self.assert_code("INPUT_INCOMPLETE", api.pages, "/repos/a/b/jobs", "jobs")

    def test_credential_sink_and_malformed_http_fail_closed(self):
        self.assert_code("INPUT_INVALID", gate.API, "fixture", "https://attacker.invalid")
        api = gate.API("fixture")
        self.assert_code("INPUT_INVALID", api.get, "//attacker.invalid")
        with mock.patch.object(gate.urllib.request, "build_opener") as opener:
            opener.return_value.open.side_effect = gate.urllib.error.URLError("synthetic 403")
            self.assert_code("INPUT_UNAVAILABLE", api.get, "/repos/a/b")
            response = mock.MagicMock()
            response.__enter__.return_value.read.return_value = b"not-json"
            opener.return_value.open.side_effect = None
            opener.return_value.open.return_value = response
            self.assert_code("INPUT_INVALID", api.get, "/repos/a/b")

    def test_final_collection_detects_base_and_review_change(self):
        review, _, policy = self.evidence()
        pr = dict(state="open", draft=False, number=8, changed_files=1,
                  head=dict(sha="a" * 40), base=dict(sha="b" * 40, ref="master", repo=dict(id=1)))
        run = dict(id=5, workflow_id=2, event="pull_request", head_sha="a" * 40,
                   repository=dict(id=1), pull_requests=[dict(number=8)], run_attempt=1,
                   status="completed", conclusion="success")
        class FakeAPI:
            def __init__(self, alteration):
                self.reads = 0
                self.review_reads = 0
                self.alteration = alteration
            def get(self, path):
                if "/workflows/" in path:
                    return dict(id=2)
                self.reads += 1
                value = copy.deepcopy(pr)
                if self.reads > 1 and self.alteration == "base":
                    value["base"]["sha"] = "f" * 40
                return value
            def pages(self, path, key=None):
                if path.endswith("/files"):
                    return [dict(filename="docs/a.md", status="modified")]
                if path.endswith("/reviews"):
                    self.review_reads += 1
                    if self.review_reads > 1 and self.alteration == "review":
                        return [dict(review, body=review["body"] + " ")]
                    return [review]
                if "/runs?" in path:
                    filename = path.split("/workflows/")[1].split("/")[0]
                    return [dict(run, path=".github/workflows/" + filename)]
                return []
        with mock.patch.object(gate, "candidate", return_value=self.identity()), mock.patch.object(gate, "check_jobs"):
            for alteration in ["base", "review"]:
                self.assert_code("CAPTURE_CHANGED", gate.collect, FakeAPI(alteration), "a/b", 8,
                                 "a" * 40, "b" * 40, policy)

    def test_cli_rejects_invalid_sha_before_network(self):
        result = subprocess.run([sys.executable, str(Path(gate.__file__)), "--repo", "a/b", "--pr", "1",
                                 "--head", "not-a-sha", "--base", "b" * 40, "--timeout", "0"],
                                text=True, capture_output=True, timeout=5)
        self.assertEqual(1, result.returncode)
        self.assertEqual("INPUT_INVALID: full lowercase SHA required\n", result.stderr)
        self.assertEqual("", result.stdout)

    def test_deadline_prevents_any_network_request(self):
        api = gate.API("fixture", deadline=0)
        with mock.patch.object(gate.urllib.request, "build_opener") as opener:
            self.assert_code("INPUT_UNAVAILABLE", api.get, "/repos/a/b")
            opener.assert_not_called()


if __name__ == "__main__":
    unittest.main()
