// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The CI trap (spec FR-EXEC-2, lote 3c): the single-path invariant —
// Tool.Execute/ExecuteScoped reachable ONLY through this package — is
// enforced by machine, forever. The sweep parses every production .go
// file: a file that imports the tool seam and contains an Execute call
// outside the allow-list fails the build. internal/tool itself is allowed
// (cage/shield wrappers ARE tool composition, not agent execution paths);
// tests are excluded (fixtures may call tools directly).

package executor

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var executeCall = regexp.MustCompile(`\.Execute(Scoped)?\(`)

func TestTripwire_theOnlyPathToExecuteIsThisPackage(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	var violations []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if name == "node_modules" || name == ".git" || name == "website" || name == "web" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "internal/action/executor/") ||
			strings.HasPrefix(rel, "internal/tool/") {
			return nil
		}
		src, err := os.ReadFile(path) // #nosec G304 -- repo sweep over WalkDir's own paths, test-only
		if err != nil {
			return err
		}
		if !strings.Contains(string(src), `"github.com/Sebastian197/korvun/internal/tool"`) {
			return nil
		}
		if executeCall.Match(src) {
			violations = append(violations, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("sweep failed: %v", err)
	}
	if len(violations) > 0 {
		t.Fatalf("Tool.Execute must be reachable ONLY through the Executor Registry; direct callers found in: %v", violations)
	}
}
