// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package tool

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The read_file cage (ADR-0041 §4, mandate SP3.1): read-only, every path
// resolved (Abs + EvalSymlinks) MUST stay under the operator root — symlink
// escapes die at the resolved-path check — plus a size cap. Sensitive by
// house default (the attrs tripwire).

// jail builds a temp root with a file inside and a file outside, plus a
// symlink inside the root pointing at the outside file.
func jail(t *testing.T) (root, insidePath, outsidePath, escapeLink string) {
	t.Helper()
	root = t.TempDir()
	outside := t.TempDir()

	insidePath = filepath.Join(root, "notes.txt")
	if err := os.WriteFile(insidePath, []byte("inside content"), 0o600); err != nil {
		t.Fatalf("write inside: %v", err)
	}
	outsidePath = filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsidePath, []byte("OUTSIDE SECRET"), 0o600); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	escapeLink = filepath.Join(root, "escape")
	if err := os.Symlink(outsidePath, escapeLink); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	return root, insidePath, outsidePath, escapeLink
}

func mustReadFile(t *testing.T, cfg ReadFileConfig) Tool {
	t.Helper()
	rf, err := ReadFile(cfg)
	if err != nil {
		t.Fatalf("ReadFile(%+v): %v", cfg, err)
	}
	return rf
}

func TestReadFile_readsInsideTheJail(t *testing.T) {
	t.Parallel()
	root, inside, _, _ := jail(t)
	rf := mustReadFile(t, ReadFileConfig{Root: root})

	// Relative path, resolved against the root.
	got, err := rf.Execute(context.Background(), "notes.txt")
	if err != nil {
		t.Fatalf("Execute(relative): %v", err)
	}
	if got != "inside content" {
		t.Fatalf("got %q, want the file content", got)
	}

	// Absolute path under the root works too.
	got, err = rf.Execute(context.Background(), inside)
	if err != nil {
		t.Fatalf("Execute(absolute): %v", err)
	}
	if got != "inside content" {
		t.Fatalf("got %q, want the file content", got)
	}
}

func TestReadFile_dotDotEscapeDiesAtTheCage(t *testing.T) {
	t.Parallel()
	root, _, outside, _ := jail(t)
	rf := mustReadFile(t, ReadFileConfig{Root: root})

	rel, err := filepath.Rel(root, outside)
	if err != nil {
		t.Fatalf("rel: %v", err)
	}
	_, execErr := rf.Execute(context.Background(), rel) // "../.../secret.txt"
	if !errors.Is(execErr, ErrCageViolation) {
		t.Fatalf("err = %v, want errors.Is(_, ErrCageViolation)", execErr)
	}
}

func TestReadFile_absoluteOutsideDiesAtTheCage(t *testing.T) {
	t.Parallel()
	root, _, outside, _ := jail(t)
	rf := mustReadFile(t, ReadFileConfig{Root: root})

	_, err := rf.Execute(context.Background(), outside)
	if !errors.Is(err, ErrCageViolation) {
		t.Fatalf("err = %v, want errors.Is(_, ErrCageViolation)", err)
	}
}

// The load-bearing case: a symlink INSIDE the root pointing OUTSIDE must die
// at the RESOLVED-path check, not slip through the lexical one.
func TestReadFile_symlinkEscapeDiesAtTheResolvedCheck(t *testing.T) {
	t.Parallel()
	root, _, _, escape := jail(t)
	rf := mustReadFile(t, ReadFileConfig{Root: root})

	_, err := rf.Execute(context.Background(), escape)
	if !errors.Is(err, ErrCageViolation) {
		t.Fatalf("err = %v, want errors.Is(_, ErrCageViolation) — symlink escaped the jail", err)
	}
}

func TestReadFile_sizeCapDiesAtTheCage(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	big := filepath.Join(root, "big.txt")
	if err := os.WriteFile(big, []byte(strings.Repeat("x", 100)), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	rf := mustReadFile(t, ReadFileConfig{Root: root, MaxBytes: 10})

	_, err := rf.Execute(context.Background(), "big.txt")
	if !errors.Is(err, ErrCageViolation) {
		t.Fatalf("err = %v, want errors.Is(_, ErrCageViolation) — cap not enforced", err)
	}
}

// A missing file is an ORDINARY tool error (the model can react), never a
// cage violation — honesty about nonexistence, not governance theater.
func TestReadFile_missingFileIsOrdinaryError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	rf := mustReadFile(t, ReadFileConfig{Root: root})

	_, err := rf.Execute(context.Background(), "no-such-file.txt")
	if err == nil {
		t.Fatal("Execute succeeded on a missing file")
	}
	if errors.Is(err, ErrCageViolation) {
		t.Fatalf("missing file misclassified as a cage violation: %v", err)
	}
}

func TestReadFile_emptyArgsIsOrdinaryError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	rf := mustReadFile(t, ReadFileConfig{Root: root})

	_, err := rf.Execute(context.Background(), "  ")
	if err == nil {
		t.Fatal("Execute succeeded on empty args")
	}
	if errors.Is(err, ErrCageViolation) {
		t.Fatalf("empty args misclassified as a cage violation: %v", err)
	}
}

// Construction fails loud without a valid jail (fail-closed at wiring).
func TestReadFile_constructionFailsLoud(t *testing.T) {
	t.Parallel()
	if _, err := ReadFile(ReadFileConfig{}); err == nil {
		t.Fatal("ReadFile with no root must fail")
	}
	if _, err := ReadFile(ReadFileConfig{Root: filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Fatal("ReadFile with a nonexistent root must fail")
	}
	file := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := ReadFile(ReadFileConfig{Root: file}); err == nil {
		t.Fatal("ReadFile with a non-directory root must fail")
	}
}

func TestReadFile_identity(t *testing.T) {
	t.Parallel()
	rf := mustReadFile(t, ReadFileConfig{Root: t.TempDir()})
	if rf.Name() != "read_file" {
		t.Fatalf("Name() = %q, want read_file", rf.Name())
	}
	if rf.Description() == "" {
		t.Fatal("Description() empty")
	}
}

// THE ATTRS TRIPWIRE (mandate SP3.1, spec SP3 rider): the house catalog MUST
// declare read_file Sensitive — a forgotten declaration would silently bypass
// the locality rule (zero attrs = not sensitive).
func TestBuiltinAttrs_readFileIsSensitiveByHouseDefault(t *testing.T) {
	t.Parallel()
	a, ok := BuiltinAttrs("read_file")
	if !ok {
		t.Fatal("BuiltinAttrs does not know read_file")
	}
	if !a.Sensitive {
		t.Fatal("read_file MUST be Sensitive by house default (ADR-0041 §4, R-2)")
	}
	if a.Network {
		t.Fatal("read_file must not be Network-classed")
	}
}

// The pure tools carry zero attrs (house default: not sensitive, not network).
func TestBuiltinAttrs_pureToolsAreZero(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"time", "echo", "calc"} {
		a, ok := BuiltinAttrs(name)
		if !ok {
			t.Fatalf("BuiltinAttrs does not know %q", name)
		}
		if a.Sensitive || a.Network {
			t.Fatalf("pure tool %q carries non-zero attrs: %+v", name, a)
		}
	}
}

func TestBuiltinAttrs_unknownToolIsNotKnown(t *testing.T) {
	t.Parallel()
	if _, ok := BuiltinAttrs("shell"); ok {
		t.Fatal("BuiltinAttrs must not know a dangerous name")
	}
}
