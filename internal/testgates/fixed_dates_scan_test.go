// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package testgates

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// rfc3339Instant is the shape the gate looks for inside a string literal: a
// date and a time to the second, then Z or an offset.
var rfc3339Instant = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(Z|[+-]\d{2}:\d{2})`)

// prunedDirs are the directory names the walk never enters: node_modules and
// .git, as the Makefile prunes them, and design-drafts, the scratch area git
// ignores, where a working checkout keeps copies of tests.
var prunedDirs = []string{"node_modules", ".git", "design-drafts"}

// fixedDate is one date the gate sees.
type fixedDate struct {
	file string // slash path from the root of the walk
	line int
	key  string // the RFC3339 match, or the time.Date call as timeDateKey prints it
}

// fixedDateException excuses, in one file, the dates it lists, for its reason.
type fixedDateException struct {
	file   string
	reason string
	dates  []string
}

// testFiles lists the *_test.go files under root as slash paths from it, in
// lexical order, never entering a directory named in prunedDirs.
func testFiles(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if slices.Contains(prunedDirs, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	return out, err
}

// scanFile is scanSource over the file rel under root, read by the parser.
func scanFile(root, rel string, year int) ([]fixedDate, error) {
	return scan(rel, filepath.Join(root, filepath.FromSlash(rel)), nil, year)
}

// scanSource returns the dates of year or later that the gate sees in src, a
// file named rel.
func scanSource(rel string, src []byte, year int) ([]fixedDate, error) {
	return scan(rel, rel, src, year)
}

// scan parses path, or src when it is not nil, and returns the dates of year or
// later it holds in either shape, keyed and placed under rel.
func scan(rel, path string, src any, year int) ([]fixedDate, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return nil, err
	}
	var out []fixedDate
	var failed error
	ast.Inspect(f, func(n ast.Node) bool {
		if failed != nil {
			return false
		}
		switch n := n.(type) {
		case *ast.BasicLit:
			if n.Kind != token.STRING {
				return true
			}
			v, err := strconv.Unquote(n.Value)
			if err != nil {
				failed = fmt.Errorf("%s: unquote %s: %w", fset.Position(n.Pos()), n.Value, err)
				return false
			}
			for _, m := range rfc3339Instant.FindAllString(v, -1) {
				y, err := strconv.Atoi(m[:4])
				if err != nil {
					failed = fmt.Errorf("%s: year of %s: %w", fset.Position(n.Pos()), m, err)
					return false
				}
				if ofYearOrLater(y, year) {
					out = append(out, fixedDate{file: rel, line: fset.Position(n.Pos()).Line, key: m})
				}
			}
		case *ast.CallExpr:
			y, ok, err := timeDateYear(n)
			if err != nil {
				failed = fmt.Errorf("%s: %w", fset.Position(n.Pos()), err)
				return false
			}
			if ok && ofYearOrLater(y, year) {
				key, err := timeDateKey(fset, n)
				if err != nil {
					failed = fmt.Errorf("%s: %w", fset.Position(n.Pos()), err)
					return false
				}
				out = append(out, fixedDate{file: rel, line: fset.Position(n.Pos()).Line, key: key})
			}
		}
		return true
	})
	if failed != nil {
		return nil, failed
	}
	return out, nil
}

// timeDateYear is the year of a call written time.Date(<integer literal>, …);
// ok is false for any other call.
func timeDateYear(call *ast.CallExpr) (year int, ok bool, err error) {
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if !isSel || sel.Sel.Name != "Date" || len(call.Args) == 0 {
		return 0, false, nil
	}
	if pkg, isIdent := sel.X.(*ast.Ident); !isIdent || pkg.Name != "time" {
		return 0, false, nil
	}
	lit, isLit := call.Args[0].(*ast.BasicLit)
	if !isLit || lit.Kind != token.INT {
		return 0, false, nil
	}
	y, err := strconv.ParseInt(lit.Value, 0, 0)
	if err != nil {
		return 0, false, fmt.Errorf("time.Date year %s: %w", lit.Value, err)
	}
	return int(y), true, nil
}

// timeDateKey prints a time.Date call as the gate keys it: its arguments as
// written, joined by ", ".
func timeDateKey(fset *token.FileSet, call *ast.CallExpr) (string, error) {
	args := make([]string, len(call.Args))
	for i, a := range call.Args {
		var b bytes.Buffer
		if err := printer.Fprint(&b, fset, a); err != nil {
			return "", fmt.Errorf("print a time.Date argument: %w", err)
		}
		args[i] = b.String()
	}
	return "time.Date(" + strings.Join(args, ", ") + ")", nil
}

// ofYearOrLater is the gate's threshold: a year that is the current one or
// later.
func ofYearOrLater(y, year int) bool {
	return y >= year
}

// unexcused returns the dates no exception excuses, in order: an exception
// excuses a date only in its own file.
func unexcused(found []fixedDate, exceptions []fixedDateException) []fixedDate {
	var out []fixedDate
	for _, d := range found {
		excused := slices.ContainsFunc(exceptions, func(e fixedDateException) bool {
			return e.file == d.file && slices.Contains(e.dates, d.key)
		})
		if !excused {
			out = append(out, d)
		}
	}
	return out
}

// reasonless returns the exceptions whose reason is empty or blank.
func reasonless(exceptions []fixedDateException) []fixedDateException {
	var out []fixedDateException
	for _, e := range exceptions {
		if strings.TrimSpace(e.reason) == "" {
			out = append(out, e)
		}
	}
	return out
}
