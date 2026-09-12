#!/usr/bin/env python3
# Copyright 2026 Sebastián Moreno Saavedra
# SPDX-License-Identifier: Apache-2.0
"""Evaluate existing PR checks and authenticated custody of an external report.

This read-only client does not authenticate an external reviewer's work or make
GitHub's merge operation atomic with its reads. Run it from the protected base.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

LANES = {
    "quality": ("quality.yml", ["quality (ubuntu-latest)", "quality (macos-latest)",
        "quality (windows-latest)"] + [f"cross-compile ({system}, {arch})"
        for system in ("linux", "darwin", "windows") for arch in ("amd64", "arm64")] + ["sbom"]),
    "codeql": ("codeql.yml", ["Analyze (Go)"]),
    "frontend": ("frontend.yml", ["builder (lint · typecheck · test · build)",
        "builder e2e (Playwright · mock control API)", "chrome (lint · typecheck · test · build)",
        "chrome e2e (Playwright · SP4 proxy + real no-network core)"]),
    "website": ("website-pr.yml", ["website (full harness via make website-check)"]),
}
PREFIX = "korvun-review-v1\n"
MAX_BYTES = 4 * 1024 * 1024


class GateError(Exception):
    def __init__(self, code, detail):
        self.code = code
        super().__init__(f"{code}: {detail}")


def require(condition, code, detail):
    if not condition:
        raise GateError(code, detail)


def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(",", ":"),
                                    ensure_ascii=False).encode()).hexdigest()


def classify(files, expected):
    require(isinstance(files, list) and type(expected) is int and expected > 0,
            "INPUT_INVALID", "nonempty complete file list required")
    require(len(files) == expected and expected <= 3000, "INPUT_INCOMPLETE", "file count differs or exceeds cap")
    lanes = {"quality", "codeql"}
    scope = []
    governance = False
    seen = set()
    for item in files:
        require(isinstance(item, dict), "INPUT_INVALID", "file entry must be an object")
        path, status = item.get("filename"), item.get("status")
        require(status in {"added", "removed", "modified", "renamed", "copied", "changed", "unchanged"},
                "INPUT_INVALID", "unknown change status")
        previous = item.get("previous_filename")
        require(status != "renamed" or bool(previous), "INPUT_INVALID", "rename lacks previous path")
        require(isinstance(path, str) and path not in seen, "INPUT_INVALID", "missing or duplicate filename")
        seen.add(path)
        for candidate in [path] + ([previous] if previous is not None else []):
            require(isinstance(candidate, str) and candidate and not candidate.startswith("/")
                    and ".." not in candidate.split("/") and not any(ord(c) < 32 for c in candidate),
                    "INPUT_INVALID", "invalid repository path")
            if candidate.startswith((".github/", ".githooks/", ".claude/hooks/", "scripts/")) or candidate in {"Makefile", "CLAUDE.md", "AGENTS.md"}:
                governance = True
                lanes.update(LANES)
            elif candidate.startswith(("website/", "assets/")):
                lanes.add("website")
            elif candidate in {"go.mod", "go.sum"}:
                lanes.update(LANES)
            elif candidate.startswith(("internal/", "cmd/", "web/")):
                lanes.add("frontend")
            elif candidate.startswith(("docs/superpowers/", "docs/adr/", "docs/stages/")) or candidate in {"LICENSE", "NOTICE"}:
                pass
            elif candidate.startswith("docs/") or candidate in {"README.md", "CHANGELOG.md"}:
                # Public operational docs can be mirrored into the site; use the
                # site lane conservatively until that dependency map is explicit.
                lanes.add("website")
                if not candidate.endswith(".md"):
                    lanes.update(LANES)
            elif candidate == ".claude/adversary/last-verdict.md":
                pass
            else:
                lanes.update(LANES)
        scope.append({key: item.get(key) for key in ("filename", "previous_filename", "status", "sha")})
    return sorted(lanes), digest(sorted(scope, key=lambda item: item["filename"])), governance


def check_jobs(lane, jobs, head):
    require(isinstance(jobs, list), "INPUT_INVALID", "jobs must be an array")
    for name in LANES[lane][1]:
        matches = [job for job in jobs if job.get("name") == name]
        require(matches, "CHECK_MISSING", name)
        require(len(matches) == 1, "CHECK_DUPLICATE", name)
        job = matches[0]
        require(job.get("head_sha") == head, "RUN_STALE", name)
        if job.get("conclusion") == "failure":
            raise GateError("CHECK_FAILED", name)
        require(job.get("status") == "completed" and job.get("conclusion") == "success",
                "CHECK_NOT_SUCCESS", name)
        require(isinstance(job.get("steps"), list) and job["steps"], "STEP_MISSING", name)
        for step in job["steps"]:
            # The existing Windows lane deliberately does not probe Unix hooks.
            benign_skip = (name == "quality (windows-latest)"
                           and step.get("name") == "Push-gate probe table (Linux, macOS)"
                           and step.get("conclusion") == "skipped")
            require(benign_skip or (step.get("status") == "completed" and step.get("conclusion") == "success"),
                    "STEP_NOT_SUCCESS", f"{name}: {step.get('name')}")


def check_evidence(reviews, policy, identity, governance):
    decisions = {}
    for review in sorted(reviews, key=lambda item: item["id"]):
        trusted_blocker = review.get("user", {}).get("id") in policy.get("blocking_reviewer_ids", policy.get("custodian_ids", []))
        if trusted_blocker and review.get("state") in {"CHANGES_REQUESTED", "APPROVED"}:
            decisions[review.get("user", {}).get("id")] = review["state"]
    records = [r for r in reviews if isinstance(r.get("body"), str) and r["body"].startswith(PREFIX)]
    require(records, "EVIDENCE_MISSING", "submitted custodian COMMENT required")
    records = [r for r in records if r.get("user", {}).get("id") in policy.get("custodian_ids", [])]
    require(records, "EVIDENCE_UNAUTHORIZED", "no declaration from an authorized custodian")
    # Only an authorized custodian's later declaration can supersede evidence.
    record = max(records, key=lambda item: item["id"])
    require(record.get("user", {}).get("id") in policy.get("custodian_ids", []),
            "EVIDENCE_UNAUTHORIZED", "record author is not an authorized custodian")
    require(record.get("state") == "COMMENTED" and record.get("submitted_at"),
            "EVIDENCE_STATE", "custody records must be submitted COMMENT reviews")
    require("CHANGES_REQUESTED" not in decisions.values(), "REVIEW_CHANGES_REQUESTED",
            "a comment does not resolve an outstanding request for changes")
    try:
        body = json.loads(record["body"][len(PREFIX):])
    except (ValueError, TypeError) as exc:
        raise GateError("INPUT_INVALID", "review JSON malformed") from exc
    require(isinstance(body, dict) and type(body.get("schema")) is int and body["schema"] == 1,
            "INPUT_INVALID", "unknown evidence schema")
    require(record.get("commit_id") == identity["head"] and body.get("candidate") == identity,
            "EVIDENCE_STALE", "review must bind the exact candidate, base and scope")
    reviewer, origin = body.get("reviewer"), body.get("origin")
    require(isinstance(reviewer, str) and isinstance(origin, str)
            and origin in policy.get("reviewers", {}).get(reviewer, []),
            "EVIDENCE_REVIEWER", "reviewer and report origin must be configured")
    report = body.get("report")
    require(isinstance(report, str) and bool(report.strip()) and len(report.encode()) <= MAX_BYTES
            and hashlib.sha256(report.encode()).hexdigest() == body.get("report_sha256"),
            "EVIDENCE_DIGEST", "accessible inline report bytes must match their SHA256")
    require(type(body.get("p1")) is int and body["p1"] == 0
            and type(body.get("p2")) is int and body["p2"] == 0,
            "EVIDENCE_FINDINGS", "zero P1/P2 required")
    require(isinstance(body.get("p3"), list) and all(isinstance(item, dict)
            and isinstance(item.get("id"), str) and item["id"].strip()
            and isinstance(item.get("disposition"), str) and item["disposition"].strip()
            for item in body["p3"]), "EVIDENCE_FINDINGS", "each P3 needs a disposition")
    if governance:
        require(body.get("governance_scope") == identity["scope"], "GOVERNANCE_REVIEW_REQUIRED",
                "custodian must explicitly record review of workflow/script/policy changes")
    return record["id"]


def check_stable(before, after):
    require(before == after, "CAPTURE_CHANGED", "head/base, reviews or selected run attempt changed during collection")


def select_run(runs, workflow_id, filename, repository_id, pr, head):
    require(runs, "CHECK_MISSING", filename)
    def matches(run):
        return (run.get("workflow_id") == workflow_id and run.get("path") == ".github/workflows/" + filename
            and run.get("event") == "pull_request" and run.get("head_sha") == head
            and run.get("repository", {}).get("id") == repository_id
            and any(item.get("number") == pr for item in run.get("pull_requests", [])))
    eligible = [run for run in runs if matches(run)]
    require(eligible, "RUN_SOURCE_MISMATCH", filename)
    require(len({run["id"] for run in eligible}) == len(eligible), "RUN_AMBIGUOUS", "duplicate run identity")
    run = max(eligible, key=lambda item: item["id"])
    require(type(run.get("run_attempt")) is int and run["run_attempt"] > 0,
            "INPUT_INVALID", "run attempt absent")
    return run


class API:
    def __init__(self, token, base="https://api.github.com", deadline=None):
        require(base == "https://api.github.com", "INPUT_INVALID", "only GitHub API is a credential sink")
        self.token, self.base = token, base
        self.deadline = deadline

    def get(self, path):
        remaining = 20 if self.deadline is None else self.deadline - time.monotonic()
        require(remaining > 0, "INPUT_UNAVAILABLE", "API collection deadline reached")
        require(path.startswith("/repos/") and not path.startswith("//"), "INPUT_INVALID", "repository API path required")
        request = urllib.request.Request(self.base + path, headers={
            "Accept": "application/vnd.github+json", "Authorization": "Bearer " + self.token,
            "X-GitHub-Api-Version": "2022-11-28"})
        # GitHub API reads do not require redirects. Do not forward credentials.
        class NoRedirect(urllib.request.HTTPRedirectHandler):
            def redirect_request(self, req, fp, code, msg, headers, newurl):
                return None
        try:
            with urllib.request.build_opener(NoRedirect).open(request, timeout=min(20, remaining)) as response:
                raw = response.read(MAX_BYTES + 1)
            require(len(raw) <= MAX_BYTES, "INPUT_INCOMPLETE", "API response exceeds cap")
            return json.loads(raw)
        except (urllib.error.URLError, TimeoutError, OSError) as exc:
            raise GateError("INPUT_UNAVAILABLE", "GitHub read failed; response body withheld") from exc
        except ValueError as exc:
            raise GateError("INPUT_INVALID", "GitHub response is not JSON") from exc

    def pages(self, path, key=None):
        result = []
        for page in range(1, 101):
            separator = "&" if "?" in path else "?"
            data = self.get(f"{path}{separator}per_page=100&page={page}")
            rows = data.get(key) if key and isinstance(data, dict) else data
            require(isinstance(rows, list), "INPUT_INVALID", "paginated collection is not an array")
            result.extend(rows)
            if len(rows) < 100:
                if key:
                    require(data.get("total_count") == len(result), "INPUT_INCOMPLETE", "collection total differs")
                return result
        raise GateError("INPUT_INCOMPLETE", "pagination cap reached")


def git(*args):
    result = subprocess.run(["git", "--no-replace-objects", *args], capture_output=True, text=True, timeout=30)
    require(result.returncode == 0, "INPUT_UNAVAILABLE", "required Git object or ancestry unavailable")
    return result.stdout.strip()


def candidate(pr, head, base, scope):
    require(pr.get("state") == "open" and not pr.get("draft"), "PR_INELIGIBLE", "open ready PR required")
    require(pr["head"]["sha"] == head and pr["base"]["sha"] == base and pr["base"]["ref"] == "master",
            "CANDIDATE_STALE", "event head/base no longer current")
    git("merge-base", "--is-ancestor", base, head)
    parents = git("rev-list", "--parents", "-n", "1", head).split()
    require(len(parents) == 2, "MARKER_INVALID", "head must be a single-parent marker")
    root = git("rev-parse", "--show-toplevel")
    checked = subprocess.run(["bash", str(Path(__file__).with_name("adversary-gate-check.sh")), root, head],
                             capture_output=True, text=True, timeout=30)
    require(checked.returncode == 0, "MARKER_INVALID", "legacy marker check rejected candidate")
    return dict(repository_id=pr["base"]["repo"]["id"], pr=pr["number"], head=head, base=base,
                code=parents[1], tree=git("rev-parse", f"{head}^{{tree}}"), scope=scope)


def collect(api, repo, number, head, base, policy, describe=False):
    root = f"/repos/{repo}"
    pr_path = f"{root}/pulls/{number}"
    pr = api.get(pr_path)
    files = api.pages(pr_path + "/files")
    lanes, scope, governance = classify(files, pr["changed_files"])
    identity = candidate(pr, head, base, scope)
    if describe:
        return dict(candidate=identity, applicable_lanes=lanes, governance_review_required=governance,
                    status="DESCRIPTOR_ONLY; no checks or review approved")
    reviews = api.pages(pr_path + "/reviews")
    run_paths, runs = {}, {}
    for lane in lanes:
        filename = LANES[lane][0]
        workflow = api.get(f"{root}/actions/workflows/{filename}")
        path = f"{root}/actions/workflows/{filename}/runs?event=pull_request&head_sha={head}"
        runs[lane] = select_run(api.pages(path, "workflow_runs"), workflow["id"], filename,
                                identity["repository_id"], number, head)
        run_paths[lane] = (path, workflow["id"])
    pending = [lane for lane, run in runs.items() if run.get("status") != "completed"]
    require(not pending, "CHECK_PENDING", ", ".join(pending))
    for lane, run in runs.items():
        require(run.get("conclusion") == "success", "CHECK_NOT_SUCCESS", f"{lane} run {run['id']}")
        jobs = api.pages(f"{root}/actions/runs/{run['id']}/attempts/{run['run_attempt']}/jobs", "jobs")
        check_jobs(lane, jobs, head)
    review_id = check_evidence(reviews, policy, identity, governance)
    latest_runs = {}
    for lane, (path, workflow_id) in run_paths.items():
        latest_runs[lane] = select_run(api.pages(path, "workflow_runs"), workflow_id, LANES[lane][0],
                                      identity["repository_id"], number, head)
    latest_reviews = api.pages(pr_path + "/reviews")
    latest_pr = api.get(pr_path)
    before = dict(head=pr["head"]["sha"], base=pr["base"]["sha"], state=pr["state"], draft=pr["draft"],
                  reviews=reviews, runs=runs)
    after = dict(head=latest_pr["head"]["sha"], base=latest_pr["base"]["sha"], state=latest_pr["state"],
                 draft=latest_pr["draft"], reviews=latest_reviews, runs=latest_runs)
    check_stable(before, after)
    return dict(candidate=identity, lanes=lanes, review_id=review_id,
                runs={lane: dict(id=run["id"], attempt=run["run_attempt"]) for lane, run in runs.items()},
                limitation="evaluation-time custody and CI evidence; not atomic merge authorization")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", required=True)
    parser.add_argument("--pr", type=int, required=True)
    parser.add_argument("--head", required=True)
    parser.add_argument("--base", required=True)
    parser.add_argument("--policy", default=str(Path(__file__).with_name("integration-policy.json")))
    parser.add_argument("--timeout", type=int, default=3600)
    parser.add_argument("--describe", action="store_true", help="print candidate descriptor without approving it")
    args = parser.parse_args()
    require(re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", args.repo) and args.pr > 0,
            "INPUT_INVALID", "invalid repository/PR")
    require(all(re.fullmatch(r"[0-9a-f]{40}", sha) for sha in (args.head, args.base)),
            "INPUT_INVALID", "full lowercase SHA required")
    require(0 <= args.timeout <= 5400, "INPUT_INVALID", "timeout must be 0..5400 seconds")
    policy = json.loads(Path(args.policy).read_text())
    deadline = time.monotonic() + args.timeout
    api = API(os.environ.get("GH_TOKEN", ""), deadline=deadline)
    while True:
        try:
            result = collect(api, args.repo, args.pr, args.head, args.base, policy, args.describe)
            print(json.dumps(result, sort_keys=True))
            return
        except GateError as exc:
            if exc.code not in {"CHECK_PENDING", "CHECK_MISSING"} or time.monotonic() >= deadline:
                raise
            time.sleep(min(15, max(0, deadline - time.monotonic())))


if __name__ == "__main__":
    try:
        main()
    except GateError as error:
        print(str(error), file=sys.stderr)
        sys.exit(1)
    except (KeyError, TypeError, ValueError, AttributeError, OSError, subprocess.TimeoutExpired):
        print("INPUT_INVALID: required input unreadable or malformed", file=sys.stderr)
        sys.exit(1)
