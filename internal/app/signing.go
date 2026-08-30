// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Profile keystore (Trust Layer Etapa 4, lote 2, spec FR-KEY-1): the
// ledger's private ink lives beside the store — a 0600 file under a
// 0700 keys dir — because the mandatory backup/restore contract and
// headless Linux rule over the OS keychain (the spec's declared
// trade-off; the threat model is unchanged: the operator already
// controls the disk, which is exactly why the chain is tamper-EVIDENT
// and never "immutable"). Generation is boot-idempotent (the
// root-intent mold); permissions are VERIFIED on every boot — a
// world-readable private key is refused CLOSED; a corrupt file is
// boot-fatal and never silently regenerated (regeneration would orphan
// every historical receipt).
package app

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
)

// signingKeyFile is the private seed's filename under <profile>/keys.
const signingKeyFile = "receipt-signing.key"

// ensureSigningKey materializes the profile's signing key: present and
// healthy → verified no-op (re-registering the public key when the DB
// lost it — the partial-restore reality: the FILE is the truth of the
// private key); absent → generate + persist + register; wrong
// permissions, unreadable or corrupt → boot-fatal, closed.
func ensureSigningKey(ctx context.Context, store *actionsqlite.Store, profileDir string) (ed25519.PrivateKey, error) {
	keysDir := filepath.Join(profileDir, "keys")
	keyPath := filepath.Join(keysDir, signingKeyFile)

	if info, err := os.Stat(keyPath); err == nil {
		// Every boot verifies the permissions: the private ink must be
		// owner-only. Anything looser is refused CLOSED.
		if info.Mode().Perm() != 0o600 {
			return nil, fmt.Errorf("app: signing key %q has mode %o; it must be 0600 — refusing to boot with a readable private key", keyPath, info.Mode().Perm())
		}
		raw, err := os.ReadFile(keyPath) // #nosec G304 -- profile-owned fixed path derived from the storage dir
		if err != nil {
			return nil, fmt.Errorf("app: read signing key: %w", err)
		}
		priv, err := action.ParseSigningKeySeed(raw)
		if err != nil {
			return nil, fmt.Errorf("app: signing key %q is corrupt (%w) — refusing to regenerate: that would orphan every historical receipt", keyPath, err)
		}
		if err := registerPublicKey(ctx, store, priv); err != nil {
			return nil, err
		}
		return priv, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("app: stat signing key: %w", err)
	}

	// First boot: generate, persist (0700 dir, 0600 file), register.
	if err := os.MkdirAll(keysDir, 0o700); err != nil {
		return nil, fmt.Errorf("app: create keys dir: %w", err)
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("app: generate signing key: %w", err)
	}
	if err := os.WriteFile(keyPath, action.EncodeSigningKeySeed(priv), 0o600); err != nil {
		return nil, fmt.Errorf("app: write signing key: %w", err)
	}
	if err := registerPublicKey(ctx, store, priv); err != nil {
		return nil, err
	}
	return priv, nil
}

// registerPublicKey puts the key's public half into the ink registry
// when it is not there yet. A DIFFERENT active key in the registry with
// our file key unregistered is an identity conflict — boot-fatal, never
// silently resolved.
func registerPublicKey(ctx context.Context, store *actionsqlite.Store, priv ed25519.PrivateKey) error {
	pub := priv.Public().(ed25519.PublicKey)
	keyID := action.SigningKeyID(pub)
	if _, err := store.GetSigningKey(ctx, keyID); err == nil {
		return nil // registered — the verified no-op
	} else if !errors.Is(err, actionsqlite.ErrNotFound) {
		return fmt.Errorf("app: read signing key registry: %w", err)
	}
	active, err := store.ActiveSigningKey(ctx)
	switch {
	case err == nil:
		return fmt.Errorf("app: the profile key file (%s) is not the registry's active key (%s) — ink identity conflict; refusing to boot (restore the matching file or rotate explicitly)", keyID, active.KeyID)
	case errors.Is(err, actionsqlite.ErrNotFound):
		if err := store.PutSigningKey(ctx, keyID, hex.EncodeToString(pub), time.Now().UTC()); err != nil {
			return fmt.Errorf("app: register signing key: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("app: read active signing key: %w", err)
	}
}
