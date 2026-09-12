#!/usr/bin/env python3
# Copyright 2026 Sebastián Moreno Saavedra
# SPDX-License-Identifier: Apache-2.0
"""Check exact review ancestry and GitHub provenance after a rebase.

This authenticates GitHub's PR record, not the reviewer's intellectual work.
Original publication is offline. Rewritten histories need gh and source objects.
"""
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import threading
import time

LIMIT = 65536
MAX_COMMITS = 128
MARKER = '.claude/adversary/last-verdict.md'
VERSION = b'KORVUN-REBASE-EVIDENCE v1'
REPOSITORY = 'Sebastian197/korvun'
REPOSITORY_ID = 1268356234
SHA = re.compile(r'[0-9a-f]{40}\Z')


class EvidenceError(Exception):
    pass


def require(condition, code, message):
    if not condition:
        raise EvidenceError(code + ': ' + message)


def sha(value):
    return isinstance(value, str) and SHA.fullmatch(value) is not None


def unique(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result, 'REBASE_FORMAT', 'duplicate JSON key')
        result[key] = value
    return result


def decode(data):
    try:
        return json.loads(data, object_pairs_hook=unique)
    except (ValueError, UnicodeError) as exc:
        raise EvidenceError('REBASE_FORMAT: invalid JSON') from exc


class Runner:
    def __init__(self, root, seconds=25):
        self.root = str(root)
        self.deadline = time.monotonic() + seconds
        self.env = {key: value for key, value in os.environ.items() if not key.startswith('GIT_')}

    def remaining(self):
        left = self.deadline - time.monotonic()
        require(left > 0, 'REBASE_TIMEOUT', 'global validation deadline expired')
        return min(left, 20)

    def run(self, args):
        timeout = self.remaining()
        try:
            executable = shutil.which(args[0])
            require(executable is not None, 'REBASE_COMMAND', 'required command unavailable')
            process = subprocess.Popen([executable, *args[1:]], stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                                       env=self.env)
        except OSError as exc:
            raise EvidenceError('REBASE_COMMAND: required command unavailable') from exc
        output = bytearray()
        overflow = []
        read_errors = []

        def consume():
            try:
                while True:
                    chunk = process.stdout.read(4096)
                    if not chunk:
                        return
                    if len(output) + len(chunk) > LIMIT:
                        overflow.append(True)
                        process.kill()
                        return
                    output.extend(chunk)
            except (OSError, ValueError):
                read_errors.append(True)

        reader = threading.Thread(target=consume, daemon=True)
        reader.start()
        try:
            process.wait(timeout=timeout)
            reader.join(timeout=self.remaining())
            require(not reader.is_alive(), 'REBASE_TIMEOUT', 'output stream did not close')
            require(not read_errors, 'REBASE_READ', 'command output could not be read completely')
            require(not overflow, 'REBASE_LIMIT', 'command output exceeded 64 KiB')
            require(process.returncode == 0, 'REBASE_COMMAND', 'required command failed')
            return bytes(output)
        except subprocess.TimeoutExpired as exc:
            raise EvidenceError('REBASE_TIMEOUT: command deadline expired') from exc
        finally:
            if process.poll() is None:
                process.kill()
                process.wait(timeout=1)
            # A descendant retaining stdout cannot hold validation open.
            if not reader.is_alive():
                process.stdout.close()

    def git(self, *args):
        return self.run(['git', '--no-replace-objects', '-C', self.root, *args])

    def commit(self, oid):
        require(sha(oid), 'REBASE_FORMAT', 'invalid commit identity')
        try:
            raw = self.git('cat-file', 'commit', oid)
        except EvidenceError as exc:
            if str(exc).startswith('REBASE_COMMAND'):
                raise EvidenceError('REBASE_HISTORY: source/history object unavailable; fetch the preserved PR branch') from exc
            raise
        actual = hashlib.sha1(b'commit ' + str(len(raw)).encode() + b'\0' + raw).hexdigest()
        require(actual == oid, 'REBASE_HISTORY', 'raw commit identity mismatch')
        require(b'\0' not in raw and b'\n\n' in raw, 'REBASE_FORMAT', 'malformed commit bytes')
        header, message = raw.split(b'\n\n', 1)
        fields = {}
        parents = []
        previous = None
        for line in header.split(b'\n'):
            if line.startswith(b' '):
                require(previous == b'gpgsig', 'REBASE_FORMAT', 'unsupported continued header')
                continue
            key, sep, value = line.partition(b' ')
            require(sep and value and key in {b'tree', b'parent', b'author', b'committer', b'encoding', b'gpgsig'},
                    'REBASE_FORMAT', 'unsupported commit header')
            if key == b'parent':
                parents.append(value)
            else:
                require(key not in fields, 'REBASE_FORMAT', 'duplicate commit header')
                fields[key] = value
            previous = key
        require({b'tree', b'author', b'committer'} <= fields.keys(), 'REBASE_FORMAT', 'missing commit header')
        require(len(parents) <= 1, 'REBASE_HISTORY', 'merge commit in evidence history')
        require(re.fullmatch(rb'[0-9a-f]{40}', fields[b'tree']) is not None,
                'REBASE_FORMAT', 'invalid tree identity')
        for parent in parents:
            require(re.fullmatch(rb'[0-9a-f]{40}', parent) is not None, 'REBASE_FORMAT', 'invalid parent identity')
        for key in (b'author', b'committer'):
            require(re.fullmatch(rb'.+ <[^<>\r\n]+> [0-9]+ [+-][0-9]{4}', fields[key]) is not None,
                    'REBASE_FORMAT', 'malformed identity metadata')
        return dict(oid=oid, parent=parents[0].decode() if parents else None,
                    tree=fields[b'tree'].decode(),
                    semantic=(fields[b'tree'], fields[b'author'], fields.get(b'encoding'), message))

    def history(self, head, base, expected=None):
        require(sha(base), 'REBASE_FORMAT', 'invalid base identity')
        try:
            require(self.git('cat-file', '-t', base).strip() == b'commit', 'REBASE_BASE', 'base is not a commit')
        except EvidenceError as exc:
            if str(exc).startswith('REBASE_COMMAND'):
                raise EvidenceError('REBASE_BASE: base object unavailable') from exc
            raise
        sequence, seen, current = [], set(), head
        while current != base:
            if expected is not None:
                require(len(sequence) < expected, 'REBASE_BASE', 'integration base differs from reviewed base')
            require(len(sequence) < MAX_COMMITS, 'REBASE_LIMIT', 'history exceeds 128 commits')
            require(current not in seen, 'REBASE_HISTORY', 'history cycle')
            seen.add(current)
            commit = self.commit(current)
            sequence.append(commit)
            require(commit['parent'] is not None, 'REBASE_BASE', 'history does not reach reviewed base')
            current = commit['parent']
        require(len(sequence) >= 2, 'REBASE_HISTORY', 'code and marker sequence required')
        require(expected is None or len(sequence) == expected, 'REBASE_HISTORY', 'history count changed')
        return list(reversed(sequence))

    def marker(self, head):
        entry = self.git('ls-tree', '--full-tree', head, '--', MARKER).strip()
        match = re.fullmatch(rb'(100644|100755) blob ([0-9a-f]{40})\t' + MARKER.encode(), entry)
        require(match is not None, 'REBASE_PROVENANCE', 'source marker is missing or not a regular file')
        raw = self.git('cat-file', 'blob', match.group(2).decode())
        lines = raw.split(b'\n', 2)
        require(len(lines) == 3 and lines[1] == VERSION, 'REBASE_FORMAT', 'unsupported marker version')
        first = re.fullmatch(rb'VETO LEVANTADO ([0-9a-f]{40})', lines[0])
        require(first is not None, 'REBASE_FORMAT', 'invalid reviewed code identity')
        record = decode(lines[2])
        require(isinstance(record, dict) and set(record) == {'schema', 'repository', 'pr', 'base', 'review'},
                'REBASE_FORMAT', 'unexpected marker fields')
        require(type(record['schema']) is int and record['schema'] == 1
                and record['repository'] == REPOSITORY
                and type(record['pr']) is int and 0 < record['pr'] <= 2147483647
                and sha(record['base']) and isinstance(record['review'], str) and record['review'].strip(),
                'REBASE_FORMAT', 'invalid marker fields')
        return first.group(1).decode(), record, raw

    def pure(self, head, parent):
        require(parent is not None, 'REBASE_HISTORY', 'marker has no parent')
        changed = self.git('diff-tree', '-r', '--no-renames', '--name-only', parent, head).strip()
        require(changed == MARKER.encode(), 'REBASE_MARKER', 'marker commit is not pure')

    def provenance(self, number, integrated):
        # Extract a bounded record at the fixed host; never accept a marker URL.
        query = '{number,merged,merge_commit_sha,head:{sha:.head.sha,repo:{id:.head.repo.id}},base:{ref:.base.ref,repo:{id:.base.repo.id}}}'
        try:
            data = self.run(['gh', 'api', '--hostname', 'github.com',
                             f'repos/{REPOSITORY}/pulls/{number}', '--jq', query])
            pr = decode(data)
            require(isinstance(pr, dict), 'REBASE_FORMAT', 'API record is not an object')
        except EvidenceError as exc:
            raise EvidenceError('REBASE_API: GitHub provenance unavailable or invalid') from exc
        valid = (isinstance(pr, dict) and set(pr) == {'number', 'merged', 'merge_commit_sha', 'head', 'base'}
                 and type(pr['number']) is int and pr['number'] == number and pr['merged'] is True
                 and pr['merge_commit_sha'] == integrated)
        for field in ('head', 'base'):
            side = pr.get(field) if isinstance(pr, dict) else None
            valid = valid and isinstance(side, dict) and isinstance(side.get('repo'), dict)
            if isinstance(side, dict) and isinstance(side.get('repo'), dict):
                valid = (valid and set(side) == ({'sha', 'repo'} if field == 'head' else {'ref', 'repo'})
                         and set(side['repo']) == {'id'}
                         and type(side['repo'].get('id')) is int and side['repo']['id'] == REPOSITORY_ID)
        require(valid and sha(pr['head'].get('sha')) and pr['base'].get('ref') == 'master',
                'REBASE_PROVENANCE', 'PR does not bind source and integrated history')
        return pr['head']['sha']


def verify(root, head, seconds=25):
    runner = Runner(root, seconds)
    code, record, marker = runner.marker(head)
    current = runner.commit(head)
    runner.pure(head, current['parent'])
    if current['parent'] == code:
        history = runner.history(head, record['base'])
        return dict(mode='original', reviewed_code=code, source_head=head,
                    integrated_head=None, base=record['base'], commits=len(history))
    source = runner.provenance(record['pr'], head)
    require(source != head, 'REBASE_PROVENANCE', 'rewritten head cannot be its own source')
    original = runner.commit(source)
    require(original['parent'] == code, 'REBASE_PROVENANCE', 'PR source does not name reviewed code')
    source_code, source_record, source_marker = runner.marker(source)
    require(source_code == code and source_record == record and source_marker == marker,
            'REBASE_EQUIVALENCE', 'source marker bytes differ')
    runner.pure(source, code)
    source_history = runner.history(source, record['base'])
    target_history = runner.history(head, record['base'], expected=len(source_history))
    for before, after in zip(source_history, target_history):
        require(before['semantic'] == after['semantic'], 'REBASE_EQUIVALENCE', 'commit content or identity semantics changed')
    return dict(mode='github-rebase', reviewed_code=code, source_head=source,
                integrated_head=head, base=record['base'], commits=len(source_history))


def main():
    try:
        require(len(sys.argv) == 3 and sha(sys.argv[2]), 'REBASE_FORMAT', 'root and full commit SHA required')
        print(json.dumps(verify(Path(sys.argv[1]), sys.argv[2]), sort_keys=True))
    except EvidenceError as exc:
        print('BLOCKED by adversary gate: ' + str(exc), file=sys.stderr)
        return 2
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
