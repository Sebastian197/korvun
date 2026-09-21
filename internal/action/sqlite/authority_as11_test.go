// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/identity"
)

// TestAuthority_CrashAfterStartKeepsDebitAndUnknownOutcome (AS-AUTH-11) ends the
// process at the two named boundaries of a start and reads what a NEW process
// finds. Before the commit: nothing — no start, no debit, no strict action.
// After the commit and before the return: the start and its debits are durable,
// recovery closes the action OUTCOME_UNKNOWN, and recovery REFUNDS NOTHING — the
// debits are counted before and after it, and a second recovery changes
// nothing.
//
// What "crash" means here, exactly: the child is THIS test binary re-executed
// as a separate OS process, and at the named probe it calls os.Exit — no
// deferred rollback runs, no Close, the connection dies with the process. It is
// not a signal sent by the parent, and the mould does not claim it is.
//
// NOT covered, and said so: "never retries dispatch". This is the store; it has
// no dispatcher to retry with. What is proved is that recovery leaves the
// action terminal (OUTCOME_UNKNOWN), which is the state no coordinator resumes.
//
// The recovery it runs is the strict boot's own, through the production doors
// (see authorityStrictBootRecover), over a profile activated before the child is
// spawned — not a convenience only tests could reach.
//
// Evidence level: COMPILED TEST BINARY IN A SEPARATE OS PROCESS, terminated at
// the exact named probe; the parent reads the file afterwards through its own
// connection.
// Probing mutations executed, each alone: delete the before-commit probe, then
// the after-commit probe — the child runs to completion and exits 95 instead of
// dying at the boundary, red each time; and make recovery delete the debits of
// the action it closes ("refund") — red on «after recovery pass 1, debits = 0,
// want the same 4», and the second pass then finds the start proof corrupt.
func TestAuthority_CrashAfterStartKeepsDebitAndUnknownOutcome(t *testing.T) {
	if mode, path := os.Getenv("KORVUN_AUTH_CRASH_CHILD"), os.Getenv("KORVUN_AUTH_CRASH_DB"); mode != "" {
		store, err := Open(path)
		if err != nil {
			os.Exit(96)
		}
		privateKey, err := hex.DecodeString(os.Getenv("KORVUN_AUTH_CRASH_KEY"))
		if err != nil || len(privateKey) != ed25519.PrivateKeySize {
			os.Exit(94)
		}
		key := ed25519.PrivateKey(privateKey)
		store.SetAuthoritySigner(func(domain string, canonical []byte) action.AuthoritySignature {
			return action.SignAuthorityBytes(key, domain, canonical)
		})
		store.SetIdentitySigners(
			func(e identity.Evidence) identity.SignedEvidence { return identity.SignEvidence(key, e) },
			func(e identity.PrincipalEvent) identity.SignedPrincipalEvent {
				return identity.SignPrincipalEvent(key, e)
			},
		)
		now := time.Date(2026, 9, 21, 16, 0, 0, 0, time.UTC)
		registry := identityRegistryFixture()
		resolver, err := identity.NewResolver(registry, func() time.Time { return now })
		if err != nil {
			os.Exit(93)
		}
		issuer, err := resolver.NewIssuer(identity.IssuerConfig{
			BindingID: "binding_webhook", Method: "bearer",
			CredentialClass: "shared_secret", TTL: time.Minute,
		})
		if err != nil {
			os.Exit(92)
		}
		fixture := authoritySQLiteFixture{store: store, resolver: resolver, issuer: issuer, now: now}
		req := authorityStartRequest(fixture, "", now)
		req.Probe = func(point AuthorityStartProbe) error {
			if string(point) == mode {
				os.Exit(97)
			}
			return nil
		}
		_, _ = store.StartAuthorization(context.Background(), req)
		os.Exit(95)
	}
	for _, mode := range []AuthorityStartProbe{AuthorityProbeBeforeCommit, AuthorityProbeAfterCommitBeforeReturn} {
		t.Run(string(mode), func(t *testing.T) {
			f := newAuthoritySQLiteFixture(t, 2)
			activation := activateAuthorityFixture(t, f)
			// #nosec G204 -- os.Args[0] is this test binary and the sole argument is a fixed test selector.
			cmd := exec.Command(os.Args[0], "-test.run=^TestAuthority_CrashAfterStartKeepsDebitAndUnknownOutcome$")
			cmd.Env = append(os.Environ(), "KORVUN_AUTH_CRASH_CHILD="+string(mode), "KORVUN_AUTH_CRASH_DB="+f.store.path, "KORVUN_AUTH_CRASH_KEY="+hex.EncodeToString(f.private))
			err := cmd.Run()
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 97 {
				t.Fatalf("child = %v", err)
			}
			starts := authorityScalar(t, f.store, `SELECT COUNT(*) FROM authorization_starts`)
			if mode == AuthorityProbeBeforeCommit {
				for what, n := range map[string]int{
					"durable starts": starts,
					"budget debits":  authorityScalar(t, f.store, `SELECT COUNT(*) FROM budget_debits`),
					"strict actions": authorityScalar(t, f.store, `SELECT COUNT(*) FROM actions WHERE action_id LIKE 'act3_%'`),
				} {
					if n != 0 {
						t.Errorf("before-commit %s = %d, want 0: a process that died before its commit left something durable", what, n)
					}
				}
				return
			}
			if starts != 1 {
				t.Fatalf("after-commit starts = %d, want 1", starts)
			}
			debits := authorityScalar(t, f.store, `SELECT COUNT(*) FROM budget_debits`)
			if debits != 4 {
				t.Fatalf("after-commit debits = %d, want the 4 of one committed start", debits)
			}
			for pass := 1; pass <= 2; pass++ {
				if err := authorityStrictBootRecover(f, activation); err != nil {
					t.Fatalf("recovery pass %d: %v", pass, err)
				}
				if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM actions JOIN authorization_starts USING(action_id) WHERE state=?`, string(action.StateOutcomeUnknown)); n != 1 {
					t.Errorf("after recovery pass %d, unknown outcomes = %d, want 1", pass, n)
				}
				if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM budget_debits`); n != debits {
					t.Errorf("after recovery pass %d, debits = %d, want the same %d: a confirmed start is never refunded", pass, n, debits)
				}
				if n := authorityScalar(t, f.store, `SELECT COUNT(*) FROM authorization_starts`); n != 1 {
					t.Errorf("after recovery pass %d, durable starts = %d, want 1", pass, n)
				}
			}
		})
	}
}
