// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The live ink wired at boot — Etapa 4, lote 3, pieza 2 (spec FR-LED,
// NC-3): Build connects the ledger's sealer to the profile's ACTIVE
// signing key, so every terminal outcome is born signed with ink the
// verifier can check against the registered public key; and the raw
// result NEVER touches the disk — only its digest does, pinned by a
// negative byte-scan over the store file. Approved-red contract.

package app

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
)

func ledgerApp(t *testing.T) (*actionsqlite.Store, string) {
	t.Helper()
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "korvun.db")
	app, err := Build(kernelWiringConfig(dbPath), withChannelFactory(okFactory(newFakeChannel("telegram"))))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { shutdownApp(t, app) })
	store, ok := app.actions.(*actionsqlite.Store)
	if !ok {
		t.Fatalf("app.actions is %T, want the kernel store", app.actions)
	}
	return store, dbPath
}

func TestBuild_wiresTheProfileInkIntoTheLedger(t *testing.T) {
	store, _ := ledgerApp(t)
	ctx := context.Background()
	env := action.NewEnvelope("act_ink1", "env-ink",
		action.Source{Kind: "agent_brain", Protocol: "text", Channel: "telegram"},
		action.Operation{Namespace: "tool", Name: "echo", Version: 1},
		`{}`, time.Now().UTC())
	if err := store.RecordAttempt(ctx, env,
		actionsqlite.Decision{Outcome: "deny", Rule: "not_granted"}, action.StateDenied); err != nil {
		t.Fatalf("record: %v", err)
	}
	receipts, err := store.ReceiptsByAction(ctx, "act_ink1")
	if err != nil || len(receipts) != 1 {
		t.Fatalf("the terminal outcome births its receipt through the boot-wired sealer: %v %d", err, len(receipts))
	}
	r := receipts[0]
	if r.Signature == "" || r.SigningKeyID == "" {
		t.Fatalf("the receipt is born SIGNED with the profile ink: %+v", r)
	}
	active, err := store.ActiveSigningKey(ctx)
	if err != nil {
		t.Fatalf("active key: %v", err)
	}
	if r.SigningKeyID != active.KeyID {
		t.Fatalf("the ink is the ACTIVE profile key: receipt %q active %q", r.SigningKeyID, active.KeyID)
	}
	pubBytes, err := hex.DecodeString(active.PublicKey)
	if err != nil {
		t.Fatalf("registered public key: %v", err)
	}
	if err := action.VerifyReceiptSignature(ed25519.PublicKey(pubBytes), r); err != nil {
		t.Fatalf("the receipt must verify against the REGISTERED public key: %v", err)
	}
}

func TestLedger_rawResultNeverTouchesTheDisk(t *testing.T) {
	store, dbPath := ledgerApp(t)
	ctx := context.Background()
	const raw = `RAW-OBSERVATION-NEVER-ON-DISK-7f3a9c`
	env := action.NewEnvelope("act_nc3", "env-nc3",
		action.Source{Kind: "agent_brain", Protocol: "text", Channel: "telegram"},
		action.Operation{Namespace: "tool", Name: "echo", Version: 1},
		`{}`, time.Now().UTC())
	if err := store.RecordAttempt(ctx, env,
		actionsqlite.Decision{Outcome: "allow", Rule: "granted"}, action.StateAuthorized); err != nil {
		t.Fatalf("record: %v", err)
	}
	digest := action.HashCanonical(raw)
	if err := store.FinishWithResult(ctx, "act_nc3", action.StateSucceeded,
		time.Now().UTC(), digest); err != nil {
		t.Fatalf("finish: %v", err)
	}
	receipts, err := store.ReceiptsByAction(ctx, "act_nc3")
	if err != nil || len(receipts) != 1 || receipts[0].ResultDigest != digest {
		t.Fatalf("the receipt attests the digest: %v %+v", err, receipts)
	}
	// NC-3 negative pin: the raw observation is in NO byte of the store
	// family (db + wal + shm).
	for _, suffix := range []string{"", "-wal", "-shm"} {
		// #nosec G304 -- test-owned temp path, scanned on purpose
		data, readErr := os.ReadFile(dbPath + suffix)
		if readErr != nil {
			if os.IsNotExist(readErr) && suffix != "" {
				continue
			}
			t.Fatalf("read %s: %v", dbPath+suffix, readErr)
		}
		if bytes.Contains(data, []byte(raw)) {
			t.Fatalf("NC-3 VIOLATED: the raw result reached the disk in %q", dbPath+suffix)
		}
	}
	if strings.Contains(digest, raw) {
		t.Fatal("sanity: the digest must not embed the raw result")
	}
}
