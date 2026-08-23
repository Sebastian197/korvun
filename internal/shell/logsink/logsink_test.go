// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Red for the desktop's file log sink (v0.9.1, app-audit finding B3): a
// Finder-launched app discards stderr entirely, so the gateway's slog was
// unreadable in exactly the situations where it is needed — the morning's
// blind diagnosis of a failed boot. Open gives the desktop a plain append
// file in the profile directory with one-deep size rotation (current →
// .old), stdlib only.
package logsink

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpen_createsAndReceives(t *testing.T) {
	dir := t.TempDir()

	f, err := Open(dir)
	if err != nil {
		t.Fatalf("Open(%q) = %v, want nil", dir, err)
	}
	defer func() { _ = f.Close() }()

	logger := slog.New(slog.NewTextHandler(f, nil))
	logger.Info("desktop log sink alive", "piece", "v0.9.1")

	// #nosec G304 -- fixture path is supplied by the test itself, not user input.
	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), "desktop log sink alive") {
		t.Fatalf("log file does not carry the record: %q", data)
	}
}

func TestOpen_appendsBelowLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte("previous line\n"), 0o600); err != nil {
		t.Fatalf("seed log file: %v", err)
	}

	f, err := Open(dir)
	if err != nil {
		t.Fatalf("Open = %v, want nil", err)
	}
	if _, err := f.Write([]byte("new line\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = f.Close()

	// #nosec G304 -- fixture path is supplied by the test itself, not user input.
	data, _ := os.ReadFile(path)
	got := string(data)
	if !strings.Contains(got, "previous line") || !strings.Contains(got, "new line") {
		t.Fatalf("append lost content: %q", got)
	}
	if _, err := os.Stat(path + ".old"); err == nil {
		t.Fatal("below-limit open must not rotate")
	}
}

func TestOpen_missingDirErrorsHonestly(t *testing.T) {
	f, err := Open(filepath.Join(t.TempDir(), "does", "not", "exist"))
	if err == nil {
		_ = f.Close()
		t.Fatal("Open on a missing directory must error, not silently succeed")
	}
	if !strings.Contains(err.Error(), "logsink: open") {
		t.Fatalf("error does not name the operation: %v", err)
	}
}

func TestOpen_rotatesAboveLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	big := strings.Repeat("x", MaxBytes+1)
	if err := os.WriteFile(path, []byte(big), 0o600); err != nil {
		t.Fatalf("seed oversized log file: %v", err)
	}

	f, err := Open(dir)
	if err != nil {
		t.Fatalf("Open = %v, want nil", err)
	}
	if _, err := f.Write([]byte("fresh after rotation\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = f.Close()

	// #nosec G304 -- fixture path is supplied by the test itself, not user input.
	rotated, err := os.ReadFile(path + ".old")
	if err != nil {
		t.Fatalf("rotated file missing: %v", err)
	}
	if len(rotated) != len(big) {
		t.Fatalf("rotated file size = %d, want %d", len(rotated), len(big))
	}
	// #nosec G304 -- fixture path is supplied by the test itself, not user input.
	fresh, _ := os.ReadFile(path)
	if strings.Contains(string(fresh), "xxx") || !strings.Contains(string(fresh), "fresh after rotation") {
		t.Fatalf("current file not fresh after rotation: %q", fresh)
	}
}
