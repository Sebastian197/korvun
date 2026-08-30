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
	"runtime"
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
		// owner-only. Anything looser is refused CLOSED. POSIX only —
		// Windows has no POSIX permission bits (Perm() reports 0666
		// regardless); there the profile directory's ACLs are the
		// protection, honestly declared rather than pretended.
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
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
// silently resolved. A file key that is registered but RETIRED is
// refused the same way (Etapa 4, the rotation crash window): retired
// ink must never sign again.
func registerPublicKey(ctx context.Context, store *actionsqlite.Store, priv ed25519.PrivateKey) error {
	pub := priv.Public().(ed25519.PublicKey)
	keyID := action.SigningKeyID(pub)
	if key, err := store.GetSigningKey(ctx, keyID); err == nil {
		if !key.RetiredAt.IsZero() {
			return fmt.Errorf("app: the profile key file carries %s, which was retired %s — retired ink never signs; swap in the rotated seed (a staged .new file beside it, if present) or rotate again", keyID, key.RetiredAt.Format(time.RFC3339))
		}
		return nil // registered and active — the verified no-op
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

// EnsureSigningKey exposes the boot keystore to the operator CLI (Etapa
// 4 FR-VER): same idempotent generation, same verified permissions, same
// boot-fatal refusals — the operator's acts sign with the SAME profile
// ink the server uses.
func EnsureSigningKey(ctx context.Context, store *actionsqlite.Store, profileDir string) (ed25519.PrivateKey, error) {
	return ensureSigningKey(ctx, store, profileDir)
}

// RotateProfileSigningKey rotates the profile ink (Etapa 4 FR-VER, the
// operator's rotate-key): a fresh Ed25519 pair is generated, the seed is
// staged beside the live file, the registry rotation (retire-and-insert,
// one transaction) lands FIRST, and the file swap is the atomic last
// step. A crash between the two leaves the OLD file against the NEW
// registry — a state ensureSigningKey refuses closed, with the staged
// seed sitting beside it for manual recovery.
func RotateProfileSigningKey(ctx context.Context, store *actionsqlite.Store, profileDir string) (ed25519.PrivateKey, error) {
	keysDir := filepath.Join(profileDir, "keys")
	keyPath := filepath.Join(keysDir, signingKeyFile)
	stagedPath := keyPath + ".new"
	if err := os.MkdirAll(keysDir, 0o700); err != nil {
		return nil, fmt.Errorf("app: create keys dir: %w", err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("app: generate rotation key: %w", err)
	}
	if err := os.WriteFile(stagedPath, action.EncodeSigningKeySeed(priv), 0o600); err != nil {
		return nil, fmt.Errorf("app: stage rotation seed: %w", err)
	}
	if err := store.RotateSigningKey(ctx, action.SigningKeyID(pub), hex.EncodeToString(pub), time.Now().UTC()); err != nil {
		_ = os.Remove(stagedPath)
		return nil, fmt.Errorf("app: rotate signing key registry: %w", err)
	}
	if err := os.Rename(stagedPath, keyPath); err != nil {
		// The registry rotation IS effective: the new key is returned
		// WITH the error so the caller can still seal with the active
		// ink — otherwise the act's own FAILED receipt would be signed
		// with retired ink and stain the chain's key windows forever.
		return priv, fmt.Errorf("app: swap rotated seed into place (the staged seed remains at %s for recovery): %w", stagedPath, err)
	}
	return priv, nil
}
