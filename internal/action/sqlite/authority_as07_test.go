// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
	"github.com/Sebastian197/korvun/internal/identity"
)

func TestAuthority_ConcurrentStartsShareAncestorBudget(t *testing.T) {
	const maximum = 12
	f := newAuthoritySQLiteFixture(t, maximum*2)
	common := f.delegate(t, "grant_common_ancestor", f.root.SubjectPrincipalID, maximum, f.root)
	left := f.delegate(t, "grant_sibling_left", f.root.SubjectPrincipalID, maximum, common)
	right := f.delegate(t, "grant_sibling_right", f.root.SubjectPrincipalID, maximum, common)
	for _, binding := range []action.ExecutionBinding{
		{BindingID: "binding_sibling_left", ActorPrincipalID: f.root.SubjectPrincipalID,
			Channel: "webhook", ConversationID: "left", IntentID: f.intent.IntentID,
			IntentVersion: f.intent.Version, IntentDigest: f.intent.Digest(), GrantID: left.GrantID,
			GrantVersion: left.Version, GrantDigest: left.Digest(), Revision: 1, Status: action.BindingActive},
		{BindingID: "binding_sibling_right", ActorPrincipalID: f.root.SubjectPrincipalID,
			Channel: "webhook", ConversationID: "right", IntentID: f.intent.IntentID,
			IntentVersion: f.intent.Version, IntentDigest: f.intent.Digest(), GrantID: right.GrantID,
			GrantVersion: right.Version, GrantDigest: right.Digest(), Revision: 1, Status: action.BindingActive},
	} {
		if err := f.store.PutExecutionBinding(context.Background(), binding); err != nil {
			t.Fatal(err)
		}
	}
	path := f.store.path
	second, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	second.authoritySigner = f.store.authoritySigner
	second.SetIdentitySigners(
		func(e identity.Evidence) identity.SignedEvidence { return identity.SignEvidence(f.private, e) },
		func(e identity.PrincipalEvent) identity.SignedPrincipalEvent {
			return identity.SignPrincipalEvent(f.private, e)
		},
	)
	var committed, exhausted atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 4*maximum; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			store := f.store
			if i%2 == 1 {
				store = second
			}
			conversation := "left"
			if i%2 == 1 {
				conversation = "right"
			}
			_, err := store.StartAuthorization(context.Background(), authorityStartRequest(f, conversation, f.now))
			switch {
			case err == nil:
				committed.Add(1)
			case errors.Is(err, ErrBudgetExhausted):
				exhausted.Add(1)
			default:
				t.Errorf("start %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	if committed.Load() != maximum || exhausted.Load() != 3*maximum {
		t.Fatalf("committed=%d exhausted=%d", committed.Load(), exhausted.Load())
	}
}
