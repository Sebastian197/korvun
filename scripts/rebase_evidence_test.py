#!/usr/bin/env python3
"""Real Git histories; the GitHub boundary is a controlled gh executable."""
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

sys.dont_write_bytecode = True
SOURCE = Path(__file__).resolve().parent
MARKER = '.claude/adversary/last-verdict.md'


class RebaseEvidenceTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name) / 'repo'
        self.root.mkdir()
        self.env = dict(os.environ, GIT_AUTHOR_NAME='reviewed author',
                        GIT_AUTHOR_EMAIL='author@example.invalid',
                        GIT_COMMITTER_NAME='source integrator',
                        GIT_COMMITTER_EMAIL='source@example.invalid',
                        GIT_AUTHOR_DATE='2026-09-12T00:00:00+00:00',
                        GIT_COMMITTER_DATE='2026-09-12T00:00:00+00:00')
        self.git('init', '-q')
        self.git('config', 'commit.gpgsign', 'false')
        (self.root / 'scripts').mkdir()
        shutil.copy(SOURCE / 'adversary-gate-check.sh', self.root / 'scripts')
        if (SOURCE / 'rebase_evidence.py').exists():
            shutil.copy(SOURCE / 'rebase_evidence.py', self.root / 'scripts')
        (self.root / 'payload').write_text('base\n')
        self.git('add', '.')
        self.git('commit', '-qm', 'base')
        self.base = self.git('rev-parse', 'HEAD')
        (self.root / 'payload').write_text('reviewed first\n')
        self.git('add', 'payload')
        self.git('commit', '-qm', 'first reviewed change')
        self.first = self.git('rev-parse', 'HEAD')
        (self.root / 'payload').write_text('reviewed final\n')
        self.git('add', 'payload')
        self.git('commit', '-qm', 'second reviewed change')
        self.code = self.git('rev-parse', 'HEAD')
        self.record = dict(schema=1, repository='Sebastian197/korvun', pr=32,
                           base=self.base, review='Synthetic fixture verdict, never real evidence.')
        self.head = self.marker(self.code, self.content())
        self.git('update-ref', 'refs/heads/source', self.head)
        self.target = self.rewrite()
        self.response = dict(number=32, merged=True, merge_commit_sha=self.target,
                             head=dict(sha=self.head, repo=dict(id=1268356234)),
                             base=dict(ref='master', repo=dict(id=1268356234)))
        self.api_file = Path(self.tmp.name) / 'api.json'
        self.calls = Path(self.tmp.name) / 'calls'
        self.bin = Path(self.tmp.name) / 'bin'
        self.bin.mkdir()
        self.fakegh = self.bin / 'gh'
        self.fakegh.write_text('#!' + sys.executable + '\nimport os,sys,pathlib\n'
                              'pathlib.Path(os.environ["TEST_GH_CALLS"]).write_text("\\n".join(sys.argv[1:]))\n'
                              'sys.stdout.write(pathlib.Path(os.environ["TEST_GH_RESPONSE"]).read_text())\n')
        self.fakegh.chmod(0o755)
        # Git Bash resolves gh.exe on Windows; an executable shell shim handles gh.
        if os.name == 'nt':
            (self.bin / 'gh.cmd').write_text('@"' + sys.executable + '" "' + str(self.fakegh) + '" %*\r\n')
        self.env.update(PATH=str(self.bin) + os.pathsep + self.env['PATH'],
                        TEST_GH_CALLS=str(self.calls), TEST_GH_RESPONSE=str(self.api_file))

    def git(self, *args, data=None):
        result = subprocess.run(['git', '--no-replace-objects', '-C', str(self.root), *args],
                                input=data, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                env=self.env, check=True)
        return result.stdout.decode().strip()

    def raw(self, sha):
        return subprocess.check_output(['git', '-C', str(self.root), 'cat-file', 'commit', sha])

    def obj(self, raw):
        return self.git('hash-object', '-t', 'commit', '-w', '--stdin', data=raw)

    def content(self):
        return 'VETO LEVANTADO ' + self.code + '\nKORVUN-REBASE-EVIDENCE v1\n' + json.dumps(self.record) + '\n'

    def marker(self, parent, text):
        self.git('read-tree', parent)
        blob = self.git('hash-object', '-w', '--stdin', data=text.encode())
        self.git('update-index', '--add', '--cacheinfo', '100644,' + blob + ',' + MARKER)
        tree = self.git('write-tree')
        return self.git('commit-tree', tree, '-p', parent, data=b'record actual review\n')

    def rewrite(self, mutate=None, base=None):
        parent = base or self.base
        for index, sha in enumerate([self.first, self.code, self.head]):
            raw = self.raw(sha)
            lines = raw.split(b'\n')
            lines = [b'parent ' + parent.encode() if l.startswith(b'parent ') else
                     b'committer GitHub <noreply@github.com> 1789190000 +0000'
                     if l.startswith(b'committer ') else l for l in lines]
            raw = b'\n'.join(lines)
            if mutate:
                raw = mutate(index, raw)
            parent = self.obj(raw)
        return parent

    def check(self, target=None, reason=None):
        self.api_file.write_text(json.dumps(self.response))
        # The legacy shell gate explicitly covers Unix, not Git Bash path
        # canonicalization on Windows. Exercise the helper there directly.
        entry = ([sys.executable, str(self.root / 'scripts/rebase_evidence.py')] if os.name == 'nt'
                 else ['bash', str(self.root / 'scripts/adversary-gate-check.sh')])
        result = subprocess.run([*entry, str(self.root), target or self.target],
                                capture_output=True, text=True, env=self.env, timeout=30)
        if reason is None:
            self.assertEqual(result.returncode, 0, result.stderr)
        else:
            self.assertEqual(result.returncode, 2, result.stderr)
            self.assertIn(reason, result.stderr)
        return result

    def test_original_and_rebased_marker(self):
        self.check(self.head)
        self.assertFalse(self.calls.exists(), 'original publication must work offline')
        self.check()
        self.assertIn('github.com', self.calls.read_text())
        self.assertIn('repos/Sebastian197/korvun/pulls/32', self.calls.read_text())

    @unittest.skipIf(os.name == 'nt', 'legacy Bash gate is covered on Linux/macOS')
    def test_legacy_still_requires_exact_parent(self):
        h = self.marker(self.code, 'VETO LEVANTADO ' + self.code + '\n\nlegacy\n')
        self.check(h)
        raw = self.raw(h).replace(b'parent ' + self.code.encode(), b'parent ' + self.first.encode())
        self.check(self.obj(raw), 'does not name its direct parent')

    def test_intermediate_tree_change_restored_at_tip_is_rejected(self):
        tree = self.git('rev-parse', self.base + '^{tree}').encode()
        target = self.rewrite(lambda i, r: b'tree ' + tree + r[r.index(b'\n'):] if i == 0 else r)
        self.response['merge_commit_sha'] = target
        self.check(target, 'REBASE_EQUIVALENCE')

    def test_semantic_changes_including_marker_commit(self):
        for idx, old, new in [(0, b'author reviewed author', b'author unreviewed author'),
                              (1, b'second reviewed change', b'unreviewed message'),
                              (2, b'record actual review', b'other marker message')]:
            with self.subTest(index=idx):
                target = self.rewrite(lambda i, r: r.replace(old, new) if i == idx else r)
                self.response['merge_commit_sha'] = target
                self.check(target, 'REBASE_EQUIVALENCE')

    def test_advanced_base_even_empty_commit(self):
        tree = self.git('rev-parse', self.base + '^{tree}')
        advanced = self.git('commit-tree', tree, '-p', self.base, data=b'concurrent integration\n')
        target = self.rewrite(base=advanced)
        self.response['merge_commit_sha'] = target
        self.check(target, 'REBASE_BASE')

    def test_unknown_or_duplicate_commit_headers(self):
        for line in [b'x-unknown value\n', b'encoding utf-8\nencoding utf-8\n',
                     b'author duplicate <duplicate@example.invalid> 1 +0000\n']:
            target = self.rewrite(lambda i, r: r.replace(b'\n\n', b'\n' + line + b'\n', 1) if i == 1 else r)
            self.response['merge_commit_sha'] = target
            self.check(target, 'REBASE_FORMAT')

    def test_merge_in_sequence_rejected(self):
        target = self.rewrite(lambda i, r: r.replace(b'\nauthor ', b'\nparent ' + self.base.encode() + b'\nauthor ', 1) if i == 1 else r)
        self.response['merge_commit_sha'] = target
        self.check(target, 'REBASE_HISTORY')

    def test_provenance_fields_strict(self):
        changes = [('merged', False), ('merged', 1), ('merged', 'true'),
                   ('number', True), ('number', 33), ('merge_commit_sha', self.head),
                   ('head', None), ('head', dict(sha=self.head, repo=dict(id=True))),
                   ('head', dict(sha=self.head, repo=dict(id=999))),
                   ('base', dict(ref='other', repo=dict(id=1268356234))),
                   ('base', dict(ref='master', repo=dict(id='1268356234')))]
        original = dict(self.response)
        for key, value in changes:
            with self.subTest(key=key, value=value):
                self.response = dict(original, **{key: value})
                self.check(reason='REBASE_PROVENANCE')

    def test_unrelated_source_head_rejected(self):
        self.response['head']['sha'] = self.first
        self.check(reason='REBASE_PROVENANCE')

    def test_missing_source_object(self):
        self.response['head']['sha'] = '1' * 40
        self.check(reason='REBASE_HISTORY')

    def test_marker_body_mutation(self):
        target_parent = self.git('rev-parse', self.target + '^')
        target = self.marker(target_parent, self.content().replace('Synthetic fixture', 'Forged fixture'))
        self.response['merge_commit_sha'] = target
        self.check(target, 'REBASE_EQUIVALENCE')

    def test_invalid_original_record_never_passes(self):
        for record, reason in [(dict(self.record, pr=True), 'REBASE_FORMAT'),
                               (dict(self.record, schema=2), 'REBASE_FORMAT'),
                               (dict(self.record, repository='other/repository'), 'REBASE_FORMAT'),
                               (dict(self.record, extra=True), 'REBASE_FORMAT'),
                               (dict(self.record, base='1' * 40), 'REBASE_BASE'),
                               (dict(self.record, review=''), 'REBASE_FORMAT')]:
            text = 'VETO LEVANTADO ' + self.code + '\nKORVUN-REBASE-EVIDENCE v1\n' + json.dumps(record)
            h = self.marker(self.code, text)
            self.check(h, reason)
        duplicate = self.content().replace('"schema": 1', '"schema": 1, "schema": 1')
        self.check(self.marker(self.code, duplicate), 'REBASE_FORMAT')

    def test_api_failure_and_invalid_json(self):
        for body in ['raise SystemExit(1)', 'print("invalid")', 'print("[]")', 'print("x" * 1000000)']:
            self.fakegh.write_text('#!' + sys.executable + '\n' + body + '\n')
            self.check(reason='REBASE_API')

    @unittest.skipIf(os.name == 'nt', 'legacy pre-push hook is covered on Linux/macOS')
    def test_real_pre_push_door(self):
        hooks = self.root / '.git/hooks/pre-push'
        shutil.copy(SOURCE.parent / '.githooks/pre-push', hooks)
        hooks.chmod(0o755)
        remote = Path(self.tmp.name) / 'remote.git'
        subprocess.run(['git', 'init', '-q', '--bare', str(remote)], check=True)
        self.api_file.write_text(json.dumps(self.response))
        result = subprocess.run(['git', '-C', str(self.root), 'push', str(remote), self.target + ':refs/heads/test'],
                                capture_output=True, text=True, env=self.env)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.response['merged'] = False
        self.api_file.write_text(json.dumps(self.response))
        result = subprocess.run(['git', '-C', str(self.root), 'push', str(remote), self.target + ':refs/heads/blocked'],
                                capture_output=True, text=True, env=self.env)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('REBASE_PROVENANCE', result.stderr)

    def test_bounded_subprocess_and_global_deadline(self):
        path = SOURCE / 'rebase_evidence.py'
        self.assertTrue(path.exists(), 'bounded subprocess implementation absent')
        spec = importlib.util.spec_from_file_location('rebase_under_test', path)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        runner = module.Runner(self.root, seconds=0.15)
        with self.assertRaisesRegex(module.EvidenceError, 'REBASE_TIMEOUT'):
            runner.run([sys.executable, '-c', 'import time; time.sleep(5)'])
        with self.assertRaisesRegex(module.EvidenceError, 'REBASE_TIMEOUT'):
            runner.run([sys.executable, '-c', 'print(1)'])
        runner = module.Runner(self.root)
        with self.assertRaisesRegex(module.EvidenceError, 'REBASE_LIMIT'):
            runner.run([sys.executable, '-c', 'import sys; sys.stdout.write("x" * 1000000)'])

    def test_marker_size_cap_before_capture(self):
        for text in [self.content() + 'x' * 65536, 'VETO LEVANTADO ' + self.code + '\n' + 'x' * 65536]:
            self.check(self.marker(self.code, text), 'REBASE_LIMIT' if os.name == 'nt' else 'exceeds the 64 KiB limit')

    def test_signature_metadata_only_may_change(self):
        target = self.rewrite(lambda i, r: r.replace(b'\n\n', b'\ngpgsig fixture\n signature continuation\n\n', 1))
        self.response['merge_commit_sha'] = target
        self.check(target)

    def test_encoding_change_is_semantic(self):
        target = self.rewrite(lambda i, r: r.replace(b'\n\n', b'\nencoding utf-8\n\n', 1) if i == 1 else r)
        self.response['merge_commit_sha'] = target
        self.check(target, 'REBASE_EQUIVALENCE')

    def test_source_marker_must_be_pure(self):
        self.git('read-tree', self.code)
        blob = self.git('hash-object', '-w', '--stdin', data=self.content().encode())
        self.git('update-index', '--add', '--cacheinfo', '100644,' + blob + ',' + MARKER)
        self.git('update-index', '--add', '--cacheinfo', '100644,' + blob + ',smuggled')
        tree = self.git('write-tree')
        self.head = self.git('commit-tree', tree, '-p', self.code, data=b'record actual review\n')
        # Keep target's final tree pure while pointing API at a tainted source.
        self.response['head']['sha'] = self.head
        self.check(reason='REBASE_MARKER')

    def test_shortened_history_rejected_even_with_same_final_tree(self):
        raw = self.raw(self.target)
        parent = self.obj(self.raw(self.code).replace(b'parent ' + self.first.encode(), b'parent ' + self.base.encode()))
        target = self.obj(raw.replace(b'parent ' + self.git('rev-parse', self.target + '^').encode(), b'parent ' + parent.encode()))
        self.response['merge_commit_sha'] = target
        self.check(target, 'REBASE_HISTORY')

    def test_raw_commit_size_cap(self):
        self.code = self.obj(self.raw(self.code) + b'x' * 65536)
        self.check(self.marker(self.code, self.content()), 'REBASE_LIMIT')

    def test_history_count_cap(self):
        parent = self.base
        raw = self.raw(self.first)
        for _ in range(128):
            parent = self.obj(raw.replace(b'parent ' + self.base.encode(), b'parent ' + parent.encode()))
        self.code = parent
        self.check(self.marker(self.code, self.content()), 'REBASE_LIMIT')

    def test_reader_failure_cannot_accept_valid_prefix(self):
        spec = importlib.util.spec_from_file_location('rebase_reader_test', SOURCE / 'rebase_evidence.py')
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        process = mock.Mock()
        process.stdout.read.side_effect = [b'{"valid_prefix":true}', OSError('truncated stream')]
        process.poll.return_value = 0
        process.returncode = 0
        with mock.patch.object(module.subprocess, 'Popen', return_value=process):
            with self.assertRaisesRegex(module.EvidenceError, 'REBASE_READ'):
                module.Runner(self.root).run([sys.executable, '-c', 'unused'])


if __name__ == '__main__':
    unittest.main()
