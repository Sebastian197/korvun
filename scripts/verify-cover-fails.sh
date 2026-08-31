#!/bin/bash
# verify-cover-fails.sh — the R3 demonstrator (external audit 2026-08-31).
# Plants a failing test inside internal/, runs `make cover`, and demands
# a NON-ZERO exit: the coverage gate must never swallow a test failure.
# Run from the repo root. Leaves no trace on success or failure.
set -u
dir="internal/tmpcoverguard"
cleanup() { rm -rf "$dir"; }
trap cleanup EXIT
mkdir -p "$dir"
cat > "$dir/planted_test.go" << 'GO'
package tmpcoverguard

import "testing"

func TestPlantedFailure(t *testing.T) { t.Fatal("planted: make cover must fail loud") }
GO
if make cover > /dev/null 2>&1; then
  echo "FAIL: make cover exited 0 with a planted failing test — the R3 bug is back"
  exit 1
fi
echo "OK: make cover fails loud on a failing test"
