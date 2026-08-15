// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package tool

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// DefaultReadFileMaxBytes caps a read when the operator does not set one.
// 64 KiB is prompt-sized: a tool result is fed back to a model, so a larger
// read would blow the context long before it helps.
const DefaultReadFileMaxBytes = 64 * 1024

// ReadFileConfig is the operator cage of the read_file tool (ADR-0041 §4).
type ReadFileConfig struct {
	// Root is the jail: every resolved path MUST stay under it. Required —
	// construction fails without a valid directory.
	Root string
	// MaxBytes caps the file size (0 => DefaultReadFileMaxBytes).
	MaxBytes int64
}

// readFileTool is the caged read-only file reader. Sensitive by house default
// (BuiltinAttrs): its output enters a model prompt, so the locality rule
// keeps it off cloud models unless the operator overrides the class.
type readFileTool struct {
	// root is the EvalSymlinks-resolved jail root — the form every RESOLVED
	// path is checked against. rootAbs is the pre-resolution absolute form,
	// kept for the lexical fallback on nonexistent paths (an ancestor
	// symlink — /var vs /private/var on macOS — must not misclassify an
	// in-jail path).
	root     string
	rootAbs  string
	maxBytes int64
}

// ReadFile constructs the caged read_file tool. It fails loud at wiring on a
// missing/invalid root (a jail that does not exist is not a jail): the root
// is resolved (Abs + EvalSymlinks) ONCE here, so every per-call check
// compares against the real directory, not a symlink to somewhere else.
func ReadFile(cfg ReadFileConfig) (Tool, error) {
	if strings.TrimSpace(cfg.Root) == "" {
		return nil, fmt.Errorf("read_file: root is required")
	}
	abs, err := filepath.Abs(cfg.Root)
	if err != nil {
		return nil, fmt.Errorf("read_file: resolve root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("read_file: resolve root: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, fmt.Errorf("read_file: stat root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("read_file: root %q is not a directory", cfg.Root)
	}
	maxBytes := cfg.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultReadFileMaxBytes
	}
	return readFileTool{root: resolved, rootAbs: abs, maxBytes: maxBytes}, nil
}

func (readFileTool) Name() string { return "read_file" }
func (r readFileTool) Description() string {
	return "reads a text file from the operator-configured directory. args = the file path (relative to that directory)."
}

// Execute resolves args against the jail and returns the file content. The
// cage checks, in order: the RESOLVED path (Abs + EvalSymlinks — a symlink
// escape dies here, on the real target, never on the lexical path) must stay
// under the resolved root, and the size must fit the cap. A cage breach
// returns an error wrapping ErrCageViolation (audited as a denial); a
// missing file is an ordinary tool error the model can react to.
func (r readFileTool) Execute(_ context.Context, args string) (string, error) {
	path := strings.TrimSpace(args)
	if path == "" {
		return "", fmt.Errorf("read_file: a file path is required")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.root, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("read_file: resolve path: %w", err)
	}
	// The jail decision is taken on the RESOLVED path (Abs + EvalSymlinks):
	// this is the check a symlink escape dies at, and it also normalizes
	// ancestor symlinks (/var vs /private/var) so a legitimate absolute path
	// is never misclassified by its lexical form.
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if os.IsNotExist(err) {
			// The path does not exist, so there is nothing to resolve. The
			// LEXICAL fallback still enforces the cage: an out-of-jail path
			// answers "cage violation", never "not found" — existence
			// outside the jail is not information this tool leaks.
			if !r.inJailLexical(abs) {
				return "", fmt.Errorf("read_file: path %q is outside the configured directory: %w", args, ErrCageViolation)
			}
			return "", fmt.Errorf("read_file: %q not found", args)
		}
		return "", fmt.Errorf("read_file: resolve path: %w", err)
	}
	if !under(r.root, resolved) {
		return "", fmt.Errorf("read_file: path %q resolves outside the configured directory: %w", args, ErrCageViolation)
	}

	// Only REGULAR files are readable (estreno E-6): opening a FIFO blocks
	// in the kernel with no ctx to save the worker, and devices/sockets are
	// not jail content. Checked BEFORE the open (the realistic accidental-
	// FIFO case never blocks) and re-verified on the open descriptor (an
	// fstat cannot be raced). The check-to-open symlink-swap window remains
	// a documented residual: it requires a local writer inside the jail,
	// outside the current threat model (estreno triage, Codex C-3).
	fi, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("read_file: stat: %w", err)
	}
	if !fi.Mode().IsRegular() {
		return "", fmt.Errorf("read_file: %q is not a regular file: %w", args, ErrCageViolation)
	}
	f, err := os.Open(resolved)
	if err != nil {
		return "", fmt.Errorf("read_file: open: %w", err)
	}
	defer f.Close() //nolint:errcheck // read-only descriptor
	if ofi, err := f.Stat(); err != nil || !ofi.Mode().IsRegular() {
		return "", fmt.Errorf("read_file: %q is not a regular file: %w", args, ErrCageViolation)
	}
	// Read through a hard limit rather than trusting a pre-Stat size (the
	// file can grow between the stat and the read): one byte past the cap
	// proves the breach.
	data, err := io.ReadAll(io.LimitReader(f, r.maxBytes+1))
	if err != nil {
		return "", fmt.Errorf("read_file: read: %w", err)
	}
	if int64(len(data)) > r.maxBytes {
		return "", fmt.Errorf("read_file: %q exceeds the %d-byte cap: %w", args, r.maxBytes, ErrCageViolation)
	}
	return string(data), nil
}

// inJailLexical is the fallback check for paths that do not exist: the
// pre-resolution absolute path must sit under EITHER form of the root (the
// resolved one or the as-configured absolute one — an ancestor symlink makes
// them differ without any escape).
func (r readFileTool) inJailLexical(abs string) bool {
	return under(r.root, abs) || under(r.rootAbs, abs)
}

// under reports whether abs is root itself or inside it.
func under(root, abs string) bool {
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
