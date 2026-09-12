#!/usr/bin/env python3
"""Execute destructive variants in temporary copies, never the checkout."""
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parent.parent
SOURCE = (ROOT / 'scripts/rebase_evidence.py').read_text()
MUTANTS = [
    ('ignore-intermediate-tree', [("before['semantic'] == after['semantic']", 'True')], 'test_intermediate_tree_change_restored_at_tip_is_rejected'),
    ('ignore-author', [("fields[b'tree'], fields[b'author'],", "fields[b'tree'], b'',")], 'test_semantic_changes_including_marker_commit'),
    ('ignore-message', [("fields.get(b'encoding'), message)", "fields.get(b'encoding'), b'')")], 'test_semantic_changes_including_marker_commit'),
    ('ignore-encoding', [("fields.get(b'encoding')", 'None')], 'test_encoding_change_is_semantic'),
    ('ignore-marker-equivalence', [("source_code == code and source_record == record and source_marker == marker", 'True'),
                                   ('zip(source_history, target_history)', 'zip(source_history[:-1], target_history[:-1])')], 'test_marker_body_mutation'),
    ('allow-unmerged-pr', [("pr['merged'] is True", 'True')], 'test_provenance_fields_strict'),
    ('accept-truthy-merged', [("pr['merged'] is True", "bool(pr['merged'])")], 'test_provenance_fields_strict'),
    ('ignore-merge-result', [("pr['merge_commit_sha'] == integrated", 'True')], 'test_provenance_fields_strict'),
    ('ignore-repository-identity', [("type(side['repo'].get('id')) is int and side['repo']['id'] == REPOSITORY_ID", 'True')], 'test_provenance_fields_strict'),
    ('ignore-base-branch', [("pr['base'].get('ref') == 'master'", 'True')], 'test_provenance_fields_strict'),
    ('allow-unknown-header', [("b'encoding', b'gpgsig'}", "b'encoding', b'gpgsig', b'x-unknown'}")], 'test_unknown_or_duplicate_commit_headers'),
    ('allow-duplicate-json', [("key not in result", 'True')], 'test_invalid_original_record_never_passes'),
    ('weaken-output-limit', [('LIMIT = 65536', 'LIMIT = 2097152')], 'test_bounded_subprocess_and_global_deadline'),
    ('ignore-global-deadline', [('left = self.deadline - time.monotonic()', 'left = 20')], 'test_bounded_subprocess_and_global_deadline'),
    ('omit-source-purity', [('runner.pure(source, code)', 'pass')], 'test_source_marker_must_be_pure'),
    ('ignore-history-count', [("expected is None or len(sequence) == expected", 'True')], 'test_shortened_history_rejected_even_with_same_final_tree'),
    ('weaken-history-limit', [('MAX_COMMITS = 128', 'MAX_COMMITS = 256')], 'test_history_count_cap'),
    ('ignore-reader-failure', [("not read_errors", 'True')], 'test_reader_failure_cannot_accept_valid_prefix'),
]


def main():
    results = []
    for name, changes, test in MUTANTS:
        variant = SOURCE
        for before, after in changes:
            if variant.count(before) != 1:
                raise RuntimeError(name + ': mutation anchor must occur exactly once')
            variant = variant.replace(before, after)
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'scripts').mkdir()
            (root / '.githooks').mkdir()
            for path in ['scripts/adversary-gate-check.sh', 'scripts/rebase_evidence_test.py', '.githooks/pre-push']:
                shutil.copy(ROOT / path, root / path)
            (root / 'scripts/rebase_evidence.py').write_text(variant)
            result = subprocess.run([sys.executable, str(root / 'scripts/rebase_evidence_test.py'),
                                     'RebaseEvidenceTests.' + test], capture_output=True, text=True, timeout=60)
            results.append(dict(name=name, detected=result.returncode != 0,
                                exit_code=result.returncode, output=(result.stdout + result.stderr)[-5000:]))
    print(json.dumps(dict(source_sha256=hashlib.sha256(SOURCE.encode()).hexdigest(), results=results), indent=2))
    return 0 if all(item['detected'] for item in results) else 1


if __name__ == '__main__':
    raise SystemExit(main())
