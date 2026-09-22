// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestAuthority_ActivationRootsReadsTheStoreAndFailsClosed attacks the reader
// the boot door asks before it refuses a non-strict boot. Its three arms went
// in with no test at all, and the adversary's short pass showed two of them
// could be neutralised — the multi-root arm and every error arm — with the
// whole suite still green. A reader whose whole job is to fail closed and to
// carry a remedy is exactly the code that must not be believed on reading.
//
// Three guarantees, one row each: it sees what the store holds, with the digest
// beside each profile; SEVERAL roots are reported, not judged, because
// `authority activate` creates that state; and a store that cannot answer is an
// error, never an empty answer that would read as «nothing is activated».
//
// Evidence level: in-process, one real SQLite store; the unreadable store is a
// CLOSED one, which is the driver's own refusal and not a fake.
// Probing mutations executed, each alone: the multi-root arm returns the first
// root instead of all — red, «roots = 1, want both»; and the scan's error arm
// returns an empty result with no error — red, «a closed store answered «not
// activated»».
func TestAuthority_ActivationRootsReadsTheStoreAndFailsClosed(t *testing.T) {
	ctx := context.Background()

	t.Run("a store with no activation says so", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		roots, err := f.store.ActivatedAuthorityRoots(ctx)
		if err != nil || len(roots) != 0 {
			t.Errorf("roots = %#v, %v; want none and no error", roots, err)
		}
	})

	t.Run("one activation comes back with its digest", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		digest := activateAuthorityFixture(t, f)
		roots, err := f.store.ActivatedAuthorityRoots(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(roots) != 1 || roots[0].ProfileID != f.intent.ProfileID || roots[0].ActivationDigest != digest {
			t.Errorf("roots = %#v, want the one profile %q with digest %q", roots, f.intent.ProfileID, digest)
		}
	})

	t.Run("two activations are both reported, neither judged", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		first := activateAuthorityFixture(t, f)
		// A second profile id: the door refuses only a repeat of the same one.
		const other = "profile_second"
		manifest, err := f.store.AuthorityActivationManifest(ctx)
		if err != nil {
			t.Fatal(err)
		}
		reason := "a second profile on the same store"
		act := authorityActorAct(t, f.store, f.resolver, f.issuer, "activate",
			CanonicalAuthorityActivation(other, manifest, reason), f.now.Add(2*time.Second))
		second, err := f.store.ActivateAuthority(ctx, other, act, reason, f.now.Add(2*time.Second))
		if err != nil {
			t.Fatalf("the second activation the CLI allows: %v", err)
		}
		roots, err := f.store.ActivatedAuthorityRoots(ctx)
		if err != nil {
			t.Fatalf("two roots: %v — a state the CLI creates is not an error", err)
		}
		if len(roots) != 2 {
			t.Fatalf("roots = %#v, want both", roots)
		}
		got := map[string]string{roots[0].ProfileID: roots[0].ActivationDigest,
			roots[1].ProfileID: roots[1].ActivationDigest}
		if got[f.intent.ProfileID] != first || got[other] != second {
			t.Errorf("roots = %#v, want %q->%q and %q->%q", roots, f.intent.ProfileID, first, other, second)
		}
	})

	t.Run("a store that cannot answer is an error, not an empty answer", func(t *testing.T) {
		f := newAuthoritySQLiteFixture(t, 2)
		activateAuthorityFixture(t, f)
		if err := f.store.Close(); err != nil {
			t.Fatal(err)
		}
		roots, err := f.store.ActivatedAuthorityRoots(ctx)
		if err == nil {
			t.Fatalf("a closed store answered «not activated»: roots = %#v", roots)
		}
		if len(roots) != 0 {
			t.Errorf("roots = %#v, want none beside the error", roots)
		}
		if errors.Is(err, ErrAuthorizationSnapshotCorrupt) {
			t.Errorf("error = %v: a store that did not answer is not corrupt evidence", err)
		}
	})
}
