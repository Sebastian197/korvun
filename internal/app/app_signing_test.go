// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The profile keystore — Etapa 4, lote 2, pieza 3 (spec FR-KEY-1): boot
// generates the Ed25519 pair idempotently (the root-intent mold), the
// private seed lives in a 0600 file beside the store (backup/restore and
// headless rule over the keychain — the declared trade-off), permissions
// are VERIFIED on every boot (a world-readable private key is refused
// closed), and a corrupt or unreadable file is boot-fatal.
// Approved-red contract.

package app

import (
	"context"
	"crypto/ed25519"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
)

func signingHarness(t *testing.T) (*actionsqlite.Store, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := actionsqlite.Open(filepath.Join(dir, "korvun.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, dir
}

func TestEnsureSigningKey_generatesOnFirstBootWithTightPermissions(t *testing.T) {
	t.Parallel()
	store, dir := signingHarness(t)
	priv, err := ensureSigningKey(context.Background(), store, dir)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	keyPath := filepath.Join(dir, "keys", "receipt-signing.key")
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("the private seed file must exist: %v", err)
	}
	// POSIX permission asserts only: Windows has no POSIX bits (Perm()
	// reports 0666 regardless) — there the profile ACLs protect the key,
	// declared in the keystore doc.
	if runtime.GOOS != "windows" {
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("the private key file must be 0600, got %o", info.Mode().Perm())
		}
		dirInfo, err := os.Stat(filepath.Join(dir, "keys"))
		if err != nil || dirInfo.Mode().Perm() != 0o700 {
			t.Fatalf("the keys dir must be 0700, got %v %o", err, dirInfo.Mode().Perm())
		}
	}
	active, err := store.ActiveSigningKey(context.Background())
	if err != nil {
		t.Fatalf("the public key must be registered active: %v", err)
	}
	if active.KeyID != action.SigningKeyID(priv.Public().(ed25519.PublicKey)) {
		t.Fatalf("the registered id must derive from THIS key, got %q", active.KeyID)
	}
	if !strings.HasPrefix(active.KeyID, "ed25519:") {
		t.Fatalf("the registered id carries the sealed format, got %q", active.KeyID)
	}
	if active.RetiredAt != (time.Time{}) {
		t.Fatal("the first key is ACTIVE")
	}
}

func TestEnsureSigningKey_idempotentAcrossBoots(t *testing.T) {
	t.Parallel()
	store, dir := signingHarness(t)
	first, err := ensureSigningKey(context.Background(), store, dir)
	if err != nil {
		t.Fatalf("boot 1: %v", err)
	}
	second, err := ensureSigningKey(context.Background(), store, dir)
	if err != nil {
		t.Fatalf("boot 2: %v", err)
	}
	if !first.Equal(second) {
		t.Fatal("the same profile must keep the same key — verified no-op")
	}
	keys, err := store.ListSigningKeys(context.Background())
	if err != nil || len(keys) != 1 {
		t.Fatalf("no duplicate registrations: %v %d", err, len(keys))
	}
}

func TestEnsureSigningKey_worldReadableIsRefusedClosed(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission refusal does not apply on Windows (no POSIX bits; profile ACLs protect the key)")
	}
	store, dir := signingHarness(t)
	if _, err := ensureSigningKey(context.Background(), store, dir); err != nil {
		t.Fatalf("boot 1: %v", err)
	}
	keyPath := filepath.Join(dir, "keys", "receipt-signing.key")
	chmodErr := os.Chmod(keyPath, 0o644) // #nosec G302 -- loosening ON PURPOSE: the refusal under test
	if chmodErr != nil {
		t.Fatalf("chmod: %v", chmodErr)
	}
	_, err := ensureSigningKey(context.Background(), store, dir)
	if err == nil {
		t.Fatal("a world-readable private key must be refused CLOSED")
	}
	if !strings.Contains(err.Error(), "0600") {
		t.Fatalf("the refusal must name the required permissions, got %v", err)
	}
}

func TestEnsureSigningKey_corruptFileIsBootFatal(t *testing.T) {
	t.Parallel()
	store, dir := signingHarness(t)
	keysDir := filepath.Join(dir, "keys")
	if err := os.MkdirAll(keysDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(keysDir, "receipt-signing.key"), []byte("not hex at all"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := ensureSigningKey(context.Background(), store, dir); err == nil {
		t.Fatal("a corrupt key file must be boot-fatal, never regenerated silently")
	}
}

func TestEnsureSigningKey_orphanFileKeyIsReRegistered(t *testing.T) {
	t.Parallel()
	// The partial-restore reality: the key file survives, the DB is fresh.
	store, dir := signingHarness(t)
	priv, err := ensureSigningKey(context.Background(), store, dir)
	if err != nil {
		t.Fatalf("boot 1: %v", err)
	}
	fresh, err := actionsqlite.Open(filepath.Join(t.TempDir(), "korvun.db"))
	if err != nil {
		t.Fatalf("fresh store: %v", err)
	}
	defer func() { _ = fresh.Close() }()
	again, err := ensureSigningKey(context.Background(), fresh, dir)
	if err != nil {
		t.Fatalf("the file is the truth of the private key — re-register: %v", err)
	}
	if !priv.Equal(again) {
		t.Fatal("the orphan file key must be kept, not replaced")
	}
	if _, err := fresh.ActiveSigningKey(context.Background()); err != nil {
		t.Fatalf("the public key must be registered on the fresh store: %v", err)
	}
}

func TestBuild_generatesTheSigningKeyBesideTheStore(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "korvun.db")
	app, err := Build(kernelWiringConfig(dbPath), withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	shutdownApp(t, app)
	if _, err := os.Stat(filepath.Join(dbDir, "keys", "receipt-signing.key")); err != nil {
		t.Fatalf("the boot must generate the signing key beside the store: %v", err)
	}
}

func TestRotateProfileSigningKey_directContract(t *testing.T) {
	t.Parallel()
	store, dir := signingHarness(t)
	first, err := ensureSigningKey(context.Background(), store, dir)
	if err != nil {
		t.Fatalf("seed key: %v", err)
	}
	rotated, err := RotateProfileSigningKey(context.Background(), store, dir)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if first.Equal(rotated) {
		t.Fatal("rotation must mint a NEW key")
	}
	// The staged .new file must be gone after the swap.
	if _, err := os.Stat(filepath.Join(dir, "keys", "receipt-signing.key.new")); !os.IsNotExist(err) {
		t.Fatalf("staged seed must be swapped away: %v", err)
	}
	// The file now parses to the rotated key and the registry agrees.
	again, err := ensureSigningKey(context.Background(), store, dir)
	if err != nil {
		t.Fatalf("post-rotation boot: %v", err)
	}
	if !again.Equal(rotated) {
		t.Fatal("the profile must boot with the rotated key")
	}
	// A failing registry rotation cleans the staged seed and keeps the
	// old file untouched.
	closed, dir2 := signingHarness(t)
	if _, err := ensureSigningKey(context.Background(), closed, dir2); err != nil {
		t.Fatalf("seed key 2: %v", err)
	}
	_ = closed.Close()
	if _, err := RotateProfileSigningKey(context.Background(), closed, dir2); err == nil {
		t.Fatal("a failing registry rotation must fail the rotate")
	}
	if _, err := os.Stat(filepath.Join(dir2, "keys", "receipt-signing.key.new")); !os.IsNotExist(err) {
		t.Fatalf("the staged seed must be cleaned after a registry failure: %v", err)
	}
}
