// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package tool

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Estreno E-6 (Codex C-4 / adversarial H9a): a FIFO inside the jail made
// os.Open block forever — ctx is not consulted by the OS open — parking the
// brain worker past every timeout. The cage must refuse NON-REGULAR files
// loudly and fast.

func TestReadFile_fifoRefusedFastAsCageViolation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	fifo := filepath.Join(root, "trap.fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}
	rf, err := ReadFile(ReadFileConfig{Root: root})
	if err != nil {
		t.Fatalf("NewReadFile: %v", err)
	}

	type result struct {
		out string
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, execErr := rf.Execute(context.Background(), "trap.fifo")
		done <- result{out, execErr}
	}()
	select {
	case r := <-done:
		if r.err == nil {
			t.Fatalf("reading a FIFO succeeded (%q); want a refusal", r.out)
		}
		if !errors.Is(r.err, ErrCageViolation) {
			t.Fatalf("err = %v; want errors.Is(_, ErrCageViolation) — a non-regular file is a cage refusal", r.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Execute blocked on the FIFO — the worker-parking defect")
	}
}

func TestReadFile_regularFileStillReads(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ok.txt"), []byte("hola"), 0o600); err != nil {
		t.Fatal(err)
	}
	rf, err := ReadFile(ReadFileConfig{Root: root})
	if err != nil {
		t.Fatalf("NewReadFile: %v", err)
	}
	out, err := rf.Execute(context.Background(), "ok.txt")
	if err != nil || out != "hola" {
		t.Fatalf("Execute = (%q, %v), want (hola, nil)", out, err)
	}
}

// Estreno E-13: the documented info-leak guard on NONEXISTENT paths — an
// out-of-jail path that does not exist answers ErrCageViolation, never
// "not found" ("existence outside the jail is not information this tool
// leaks") — was untested; so was the ordinary in-jail not-found form.
func TestReadFile_nonexistentOutsideJailLeaksNothing(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	rf, err := ReadFile(ReadFileConfig{Root: root})
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "no-such-file.txt")
	_, execErr := rf.Execute(context.Background(), outside)
	if !errors.Is(execErr, ErrCageViolation) {
		t.Fatalf("err = %v, want ErrCageViolation (never a not-found that leaks existence)", execErr)
	}
	if strings.Contains(execErr.Error(), "not found") {
		t.Fatalf("error %q leaks existence information", execErr)
	}
}

func TestReadFile_nonexistentInsideJailIsOrdinaryNotFound(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	rf, err := ReadFile(ReadFileConfig{Root: root})
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	_, execErr := rf.Execute(context.Background(), "missing.txt")
	if execErr == nil {
		t.Fatal("want a not-found error")
	}
	if errors.Is(execErr, ErrCageViolation) {
		t.Fatalf("err = %v; an in-jail miss is an ordinary tool error, not a cage refusal", execErr)
	}
	if !strings.Contains(execErr.Error(), "not found") {
		t.Fatalf("error %q is not the not-found form", execErr)
	}
}
