// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The operator's key rotation — Etapa 4, lote 4, pieza 3 (spec FR-VER,
// the blueprint's MANDATORY test): `korvun receipt rotate-key` exposes
// the lote-2 retire-and-activate atomic rotation, the rotation act
// leaves its OWN receipt signed with the NEW key, and every historical
// receipt keeps verifying — each era verifies with its era's key. A
// stale file key (crash between the registry rotation and the file
// swap) is refused CLOSED: retired ink never signs again.
// Approved-red contract.

package cli

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
)

func TestReceiptRotateKey_historicalReceiptsStillVerify(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath := seedChain(t, 2) // era A: two receipts under the first key
	code, stdout, stderr := runIntentCLI(t, "receipt", "rotate-key", "--config", cfgPath)
	if code != 0 {
		t.Fatalf("rotate-key: %d %q", code, stderr)
	}
	if !strings.Contains(stdout, "rotated") {
		t.Fatalf("the act names itself: %q", stdout)
	}
	// Era B: more receipts under the new key.
	if code, _, stderr := runIntentCLI(t, "intent", "create", "--config", cfgPath,
		"--purpose", "era B", "--operations", "calc"); code != 0 {
		t.Fatalf("era B act: %d %q", code, stderr)
	}
	// THE MANDATORY TEST: the whole chain — both eras plus the rotation
	// act's own receipt — verifies end to end.
	code, stdout, stderr = runIntentCLI(t, "ledger", "check", "--config", cfgPath)
	if code != 0 {
		t.Fatalf("historical receipts must keep verifying after rotation: %d %q %q", code, stdout, stderr)
	}
	// Each era used ITS key: the ledger must carry both key ids.
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	receipts, err := store.ListReceipts(context.Background(), "main")
	if err != nil || len(receipts) < 4 {
		t.Fatalf("chain: %v %d", err, len(receipts))
	}
	keys := map[string]bool{}
	for _, r := range receipts {
		keys[r.SigningKeyID] = true
	}
	if len(keys) != 2 {
		t.Fatalf("two eras must carry two key ids, got %v", keys)
	}
	// The rotation act's own receipt is signed with the NEW key (the
	// judge judges itself, ink included): find it and check its key is
	// the currently active one.
	active, err := store.ActiveSigningKey(context.Background())
	if err != nil {
		t.Fatalf("active: %v", err)
	}
	var rotationSigned string
	for _, r := range receipts {
		rec, err := store.Get(context.Background(), r.ActionID)
		if err == nil && rec.Envelope.Operation.Name == "rotate-key" {
			rotationSigned = r.SigningKeyID
		}
	}
	if rotationSigned != active.KeyID {
		t.Fatalf("the rotation act's receipt must be signed with the NEW key: %q vs active %q", rotationSigned, active.KeyID)
	}
}

func TestReceiptRotateKey_retiresTheOldKeyAndSwapsTheFile(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath := seedChain(t, 1)
	if code, _, stderr := runIntentCLI(t, "receipt", "rotate-key", "--config", cfgPath); code != 0 {
		t.Fatalf("rotate-key: %d %q", code, stderr)
	}
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()
	keys, err := store.ListSigningKeys(context.Background())
	if err != nil || len(keys) != 2 {
		t.Fatalf("rotation keeps the old key FOREVER: %v %d", err, len(keys))
	}
	var actives, retired int
	for _, k := range keys {
		if k.RetiredAt.IsZero() {
			actives++
		} else {
			retired++
		}
	}
	if actives != 1 || retired != 1 {
		t.Fatalf("exactly one active, one retired: %d/%d", actives, retired)
	}
	// The seed file now carries the NEW key.
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(dbPath), "keys", "receipt-signing.key")) // #nosec G304 -- test-owned path
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	priv, err := action.ParseSigningKeySeed(raw)
	if err != nil {
		t.Fatalf("parse seed: %v", err)
	}
	active, err := store.ActiveSigningKey(context.Background())
	if err != nil {
		t.Fatalf("active: %v", err)
	}
	if action.SigningKeyID(priv.Public().(ed25519.PublicKey)) != active.KeyID {
		t.Fatal("the file must carry the ACTIVE key after rotation")
	}
}

func TestReceiptRotateKey_staleFileKeyIsRefusedClosed(t *testing.T) {
	t.Parallel()
	cfgPath, dbPath := seedChain(t, 1)
	// Simulate the crash window: the registry rotates but the file swap
	// never lands — the file still carries the RETIRED key.
	store, err := actionsqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if err := store.RotateSigningKey(context.Background(),
		action.SigningKeyID(pub), hex.EncodeToString(pub), time.Now().UTC()); err != nil {
		t.Fatalf("registry-only rotate: %v", err)
	}
	_ = store.Close()
	// Any mutating act must now refuse CLOSED: retired ink never signs.
	code, _, stderr := runIntentCLI(t, "intent", "create", "--config", cfgPath,
		"--purpose", "must not sign", "--operations", "calc")
	if code == 0 {
		t.Fatal("a stale (retired) file key must refuse to seal new acts")
	}
	if !strings.Contains(stderr, "retired") {
		t.Fatalf("the refusal names the retirement: %q", stderr)
	}
}

func TestReceiptRotateKey_usage(t *testing.T) {
	t.Parallel()
	if code, _, stderr := runIntentCLI(t, "receipt", "rotate-key"); code != 2 || !strings.Contains(stderr, "--config") {
		t.Fatalf("missing config: %d %q", code, stderr)
	}
}
