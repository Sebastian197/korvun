// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package identity_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/identity"
)

var phase1Now = time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)

func phase1Registry(t *testing.T) (*identity.Resolver, *identity.Issuer) {
	t.Helper()
	resolver, err := identity.NewResolver(identity.Registry{
		Principals: []identity.Principal{
			{ID: "principal_webhook", Kind: identity.PrincipalExternalSystem},
			{ID: "principal_brain_alpha", Kind: identity.PrincipalWorkload},
			{ID: "principal_console_admin", Kind: identity.PrincipalHuman},
		},
		Bindings: []identity.Binding{{
			ID: "binding_webhook", Provider: "webhook", Channel: "webhook",
			CredentialRef: "WEBHOOK_SECRET", SubjectNamespace: "payload.sender_id",
			VerifiedSubject: "shared_webhook_credential",
			PrincipalID:     "principal_webhook", Generation: 1, Status: identity.BindingActive,
		}},
		Workloads: []identity.Workload{{
			Brain: "alpha", PrincipalID: "principal_brain_alpha",
			ResponsiblePrincipalID: "principal_console_admin",
		}},
	}, func() time.Time { return phase1Now })
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	issuer, err := resolver.NewIssuer(identity.IssuerConfig{
		BindingID: "binding_webhook", Method: "bearer",
		CredentialClass: "shared_secret", TTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("NewIssuer: %v", err)
	}
	return resolver, issuer
}

func TestIngress_CannotTransplantEvidence(t *testing.T) {
	resolver, issuer := phase1Registry(t)
	ingress, err := issuer.Issue("request-a", "sender-a")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	tests := []struct {
		name string
		in   identity.AuthenticatedIngress
		req  identity.ResolveRequest
		want error
	}{
		{
			name: "request transplant",
			in:   ingress,
			req:  identity.ResolveRequest{ActionID: "act-b", RequestID: "request-b", Channel: "webhook", Brain: "alpha"},
			want: identity.ErrIdentityBindingMismatch,
		},
		{
			name: "channel transplant",
			in:   ingress,
			req:  identity.ResolveRequest{ActionID: "act-a", RequestID: "request-a", Channel: "console", Brain: "alpha"},
			want: identity.ErrIdentityBindingMismatch,
		},
		{
			name: "missing evidence",
			req:  identity.ResolveRequest{ActionID: "act-a", RequestID: "request-a", Channel: "webhook", Brain: "alpha"},
			want: identity.ErrIdentityEvidenceMissing,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := resolver.Resolve(tt.in, tt.req); !errors.Is(err, tt.want) {
				t.Fatalf("Resolve error = %v, want %v", err, tt.want)
			}
		})
	}

	foreignResolver, foreignIssuer := phase1Registry(t)
	foreignIngress, err := foreignIssuer.Issue("request-a", "sender-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve(foreignIngress, identity.ResolveRequest{
		ActionID: "act-a", RequestID: "request-a", Channel: "webhook", Brain: "alpha",
	}); !errors.Is(err, identity.ErrIdentityBindingMismatch) {
		t.Fatalf("foreign resolver ingress = %v, want binding mismatch", err)
	}
	if _, err := foreignResolver.Resolve(ingress, identity.ResolveRequest{
		ActionID: "act-a", RequestID: "request-a", Channel: "webhook", Brain: "alpha",
	}); !errors.Is(err, identity.ErrIdentityBindingMismatch) {
		t.Fatalf("foreign adapter ingress = %v, want binding mismatch", err)
	}

	replacement, err := resolver.NewIssuer(identity.IssuerConfig{
		BindingID: "binding_webhook", Method: "bearer",
		CredentialClass: "shared_secret", TTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve(ingress, identity.ResolveRequest{
		ActionID: "act-a", RequestID: "request-a", Channel: "webhook", Brain: "alpha",
	}); !errors.Is(err, identity.ErrIdentityBindingMismatch) {
		t.Fatalf("replaced adapter ingress = %v, want binding mismatch", err)
	}
	ingress, err = replacement.Issue("request-a", "sender-a")
	if err != nil {
		t.Fatal(err)
	}

	phase1Now = phase1Now.Add(2 * time.Minute)
	t.Cleanup(func() { phase1Now = phase1Now.Add(-2 * time.Minute) })
	if _, err := resolver.Resolve(ingress, identity.ResolveRequest{
		ActionID: "act-a", RequestID: "request-a", Channel: "webhook", Brain: "alpha",
	}); !errors.Is(err, identity.ErrIdentityEvidenceExpired) {
		t.Fatalf("expired Resolve error = %v, want ErrIdentityEvidenceExpired", err)
	}
}

func TestIdentity_SharedCredentialDoesNotAssertHuman(t *testing.T) {
	resolver, issuer := phase1Registry(t)
	for n, sender := range []string{"alice", "bob"} {
		reqID := "request-" + sender
		ingress, err := issuer.Issue(reqID, sender)
		if err != nil {
			t.Fatalf("Issue(%s): %v", sender, err)
		}
		evidence, err := resolver.Resolve(ingress, identity.ResolveRequest{
			ActionID: "act-" + sender, RequestID: reqID,
			Channel: "webhook", Brain: "alpha",
		})
		if err != nil {
			t.Fatalf("Resolve(%s): %v", sender, err)
		}
		if evidence.RequesterPrincipalID != "principal_webhook" {
			t.Fatalf("row %d requester = %q", n, evidence.RequesterPrincipalID)
		}
		if evidence.SubjectClaim != sender {
			t.Fatalf("row %d subject = %q, want %q", n, evidence.SubjectClaim, sender)
		}
		if evidence.RequesterPrincipalID == sender || strings.Contains(evidence.ClaimsDigest, sender) {
			t.Fatalf("sender became authority or raw digest content: %+v", evidence)
		}
	}

	ingress, err := issuer.Issue("request-fanout", "one-subject")
	if err != nil {
		t.Fatal(err)
	}
	first, err := resolver.Resolve(ingress, identity.ResolveRequest{
		ActionID: "act-fanout-1", RequestID: "request-fanout",
		Channel: "webhook", Brain: "alpha",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := resolver.Resolve(ingress, identity.ResolveRequest{
		ActionID: "act-fanout-2", RequestID: "request-fanout",
		Channel: "webhook", Brain: "alpha",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.EvidenceID == second.EvidenceID {
		t.Fatalf("fan-out reused evidence id %q", first.EvidenceID)
	}
	if first.RequesterPrincipalID != second.RequesterPrincipalID ||
		first.BindingID != second.BindingID ||
		first.AdapterInstanceID != second.AdapterInstanceID ||
		!first.ObservedAt.Equal(second.ObservedAt) {
		t.Fatalf("fan-out changed authenticated fact: first=%+v second=%+v", first, second)
	}
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	firstSigned := identity.SignEvidence(privateKey, first)
	secondSigned := identity.SignEvidence(privateKey, second)
	if firstSigned.Signature == secondSigned.Signature {
		t.Fatal("fan-out reused an evidence signature")
	}
}

func TestIdentity_EvidenceAndActionCommitTogether(t *testing.T) {
	resolver, issuer := phase1Registry(t)
	ingress, err := issuer.Issue("request-sign", "sender")
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := resolver.Resolve(ingress, identity.ResolveRequest{
		ActionID: "act-sign", RequestID: "request-sign", Channel: "webhook", Brain: "alpha",
	})
	if err != nil {
		t.Fatal(err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signed := identity.SignEvidence(priv, evidence)
	if err := identity.VerifyEvidence(pub, signed); err != nil {
		t.Fatalf("VerifyEvidence: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*identity.SignedEvidence)
	}{
		{"canonical", func(s *identity.SignedEvidence) { s.Canonical = append(s.Canonical, ' ') }},
		{"digest", func(s *identity.SignedEvidence) { s.Digest = "sha256:00" }},
		{"key", func(s *identity.SignedEvidence) { s.SigningKeyID = "key_wrong" }},
		{"signature", func(s *identity.SignedEvidence) { s.Signature = "00" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed := signed
			changed.Canonical = append([]byte(nil), signed.Canonical...)
			tt.mutate(&changed)
			if err := identity.VerifyEvidence(pub, changed); !errors.Is(err, identity.ErrIdentityEvidenceCorrupt) {
				t.Fatalf("VerifyEvidence error = %v, want corrupt", err)
			}
		})
	}
}

func TestIdentity_SecretsNeverEnterEvidenceOrFeeds(t *testing.T) {
	resolver, issuer := phase1Registry(t)
	const secret = "CANARY-BEARER-7da9d7" // #nosec G101 -- deliberate leak-detection canary, not a credential
	const prompt = "CANARY-PROMPT-219d"
	const payload = "CANARY-PAYLOAD-ae91"
	ingress, err := issuer.Issue("request-canary", "sender-claim")
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := resolver.Resolve(ingress, identity.ResolveRequest{
		ActionID: "act-canary", RequestID: "request-canary", Channel: "webhook", Brain: "alpha",
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	for _, canary := range []string{secret, prompt, payload} {
		if strings.Contains(string(raw), canary) || strings.Contains(string(identity.CanonicalEvidence(evidence)), canary) {
			t.Fatalf("identity evidence contains secret canary %q: %s", canary, raw)
		}
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"token", "authorization", "header", "prompt", "payload"} {
		if _, ok := decoded[forbidden]; ok {
			t.Fatalf("forbidden evidence field %q exists", forbidden)
		}
	}
}

func TestIdentity_RegistryRejectsMalformedAuthorityGraph(t *testing.T) {
	valid := func() identity.Registry {
		return identity.Registry{
			Principals: []identity.Principal{
				{ID: "principal_requester", Kind: identity.PrincipalExternalSystem},
				{ID: "principal_actor", Kind: identity.PrincipalWorkload},
				{ID: "principal_responsible", Kind: identity.PrincipalHuman},
			},
			Bindings: []identity.Binding{{
				ID: "binding", Provider: "webhook", Channel: "webhook",
				CredentialRef: "WEBHOOK_SECRET", SubjectNamespace: "payload.sender_id",
				VerifiedSubject: "shared_webhook_credential",
				PrincipalID:     "principal_requester", Generation: 1,
				Status: identity.BindingActive,
			}},
			Workloads: []identity.Workload{{
				Brain: "alpha", PrincipalID: "principal_actor",
				ResponsiblePrincipalID: "principal_responsible",
			}},
		}
	}
	tests := []struct {
		name   string
		mutate func(*identity.Registry)
	}{
		{"principal without id", func(r *identity.Registry) { r.Principals[0].ID = "" }},
		{"principal with unknown kind", func(r *identity.Registry) { r.Principals[0].Kind = "robot" }},
		{"duplicate principal", func(r *identity.Registry) { r.Principals = append(r.Principals, r.Principals[0]) }},
		{"binding without fixed subject", func(r *identity.Registry) { r.Bindings[0].VerifiedSubject = "" }},
		{"binding with unknown principal", func(r *identity.Registry) { r.Bindings[0].PrincipalID = "principal_missing" }},
		{"duplicate binding", func(r *identity.Registry) { r.Bindings = append(r.Bindings, r.Bindings[0]) }},
		{"workload without actor", func(r *identity.Registry) { r.Workloads[0].PrincipalID = "principal_missing" }},
		{"workload actor is not workload", func(r *identity.Registry) { r.Workloads[0].PrincipalID = "principal_requester" }},
		{"workload with unknown responsible", func(r *identity.Registry) { r.Workloads[0].ResponsiblePrincipalID = "principal_missing" }},
		{"duplicate workload", func(r *identity.Registry) { r.Workloads = append(r.Workloads, r.Workloads[0]) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := valid()
			tt.mutate(&registry)
			if _, err := identity.NewResolver(registry, nil); err == nil {
				t.Fatal("NewResolver accepted a malformed authority graph")
			}
		})
	}
}

func TestIdentity_IssuerAndResolverFailClosed(t *testing.T) {
	resolver, issuer := phase1Registry(t)
	if got := (*identity.Issuer)(nil).IssuedCount(); got != 0 {
		t.Fatalf("nil issuer count = %d, want 0", got)
	}
	tests := []struct {
		name string
		cfg  identity.IssuerConfig
	}{
		{"unknown binding", identity.IssuerConfig{BindingID: "missing", Method: "bearer", CredentialClass: "shared", TTL: time.Minute}},
		{"missing method", identity.IssuerConfig{BindingID: "binding_webhook", CredentialClass: "shared", TTL: time.Minute}},
		{"missing credential class", identity.IssuerConfig{BindingID: "binding_webhook", Method: "bearer", TTL: time.Minute}},
		{"negative lifetime", identity.IssuerConfig{BindingID: "binding_webhook", Method: "bearer", CredentialClass: "shared", TTL: -time.Second}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := resolver.NewIssuer(tt.cfg); !errors.Is(err, identity.ErrIdentityBindingMismatch) {
				t.Fatalf("NewIssuer error = %v, want binding mismatch", err)
			}
		})
	}
	for _, request := range []struct {
		requestID string
		subject   string
	}{{subject: "sender"}, {requestID: "request"}} {
		if _, err := issuer.Issue(request.requestID, request.subject); !errors.Is(err, identity.ErrIdentityEvidenceMissing) {
			t.Fatalf("Issue(%q, %q) error = %v, want missing", request.requestID, request.subject, err)
		}
	}
	if _, err := (*identity.Issuer)(nil).Issue("request", "sender"); !errors.Is(err, identity.ErrIdentityEvidenceMissing) {
		t.Fatalf("nil issuer error = %v, want missing", err)
	}
	ingress, err := issuer.Issue("request", "sender")
	if err != nil {
		t.Fatal(err)
	}
	if got := issuer.IssuedCount(); got != 1 {
		t.Fatalf("issued count = %d, want 1", got)
	}
	for _, req := range []identity.ResolveRequest{
		{RequestID: "request", Channel: "webhook", Brain: "alpha"},
		{ActionID: "act", RequestID: "request", Channel: "webhook", Brain: "missing"},
	} {
		if _, err := resolver.Resolve(ingress, req); !errors.Is(err, identity.ErrIdentityBindingMismatch) {
			t.Fatalf("Resolve(%+v) error = %v, want binding mismatch", req, err)
		}
	}
}

func TestIdentity_PrincipalDisablementAndBindingRevocationWin(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*identity.Registry)
		want   error
	}{
		{"requester disabled", func(r *identity.Registry) { r.Principals[0].DisabledAt = phase1Now }, identity.ErrPrincipalDisabled},
		{"actor disabled", func(r *identity.Registry) { r.Principals[1].DisabledAt = phase1Now }, identity.ErrPrincipalDisabled},
		{"responsible disabled", func(r *identity.Registry) { r.Principals[2].DisabledAt = phase1Now }, identity.ErrPrincipalDisabled},
		{"binding revoked", func(r *identity.Registry) { r.Bindings[0].Status = identity.BindingRevoked }, identity.ErrIdentityBindingMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := identity.Registry{
				Principals: []identity.Principal{
					{ID: "requester", Kind: identity.PrincipalExternalSystem},
					{ID: "actor", Kind: identity.PrincipalWorkload},
					{ID: "responsible", Kind: identity.PrincipalHuman},
				},
				Bindings: []identity.Binding{{
					ID: "binding", Provider: "webhook", Channel: "webhook",
					CredentialRef: "WEBHOOK_SECRET", SubjectNamespace: "payload.sender_id",
					VerifiedSubject: "shared_webhook_credential", PrincipalID: "requester",
					Generation: 1, Status: identity.BindingActive,
				}},
				Workloads: []identity.Workload{{
					Brain: "alpha", PrincipalID: "actor", ResponsiblePrincipalID: "responsible",
				}},
			}
			tt.mutate(&registry)
			resolver, err := identity.NewResolver(registry, func() time.Time { return phase1Now })
			if err != nil {
				t.Fatal(err)
			}
			issuer, err := resolver.NewIssuer(identity.IssuerConfig{
				BindingID: "binding", Method: "bearer", CredentialClass: "shared", TTL: 0,
			})
			if err != nil {
				t.Fatal(err)
			}
			ingress, err := issuer.Issue("request", "sender")
			if err != nil {
				t.Fatal(err)
			}
			_, err = resolver.Resolve(ingress, identity.ResolveRequest{
				ActionID: "act", RequestID: "request", Channel: "webhook", Brain: "alpha",
			})
			if !errors.Is(err, tt.want) {
				t.Fatalf("Resolve error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestIdentity_SignedWiresBindEveryStoredTerm(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	evidence := identity.Evidence{
		EvidenceID: "evd_1", ActionID: "act_1", RequestID: "req_1",
		RequesterPrincipalID: "requester", ActorPrincipalID: "actor",
		BindingID: "binding", BindingGeneration: 1, AdapterInstanceID: "adapter_1",
		Provider: "webhook", Method: "bearer", CredentialClass: "shared",
		Issuer: "webhook", SubjectNamespace: "payload.sender_id",
		VerifiedSubject: "shared_webhook_credential", SubjectClaim: "sender",
		ObservedAt: phase1Now, ExpiresAt: phase1Now.Add(time.Minute), ClaimsDigest: "sha256:claims",
	}
	signedEvidence := identity.SignEvidence(priv, evidence)
	parsedEvidence, err := identity.ParseCanonicalEvidence(signedEvidence.Canonical)
	if err != nil || parsedEvidence != evidence {
		t.Fatalf("ParseCanonicalEvidence = %+v, %v", parsedEvidence, err)
	}
	keyID, err := identity.RegisteredKeyID(pub)
	if err != nil || keyID != signedEvidence.SigningKeyID {
		t.Fatalf("RegisteredKeyID = %q, %v; want %q", keyID, err, signedEvidence.SigningKeyID)
	}

	event := identity.PrincipalEvent{
		EventID: "pev_1", Principal: identity.Principal{ID: "requester", Kind: identity.PrincipalExternalSystem, Revision: 1},
		Revision: 1, Kind: "registered", OccurredAt: phase1Now,
	}
	signedEvent := identity.SignPrincipalEvent(priv, event)
	parsedEvent, err := identity.ParseCanonicalPrincipalEvent(signedEvent.Canonical)
	if err != nil || parsedEvent != event {
		t.Fatalf("ParseCanonicalPrincipalEvent = %+v, %v", parsedEvent, err)
	}
	if err := identity.VerifyPrincipalEvent(pub, signedEvent); err != nil {
		t.Fatalf("VerifyPrincipalEvent: %v", err)
	}
	foreignPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name   string
		pub    ed25519.PublicKey
		mutate func(*identity.SignedPrincipalEvent)
	}{
		{"canonical", pub, func(s *identity.SignedPrincipalEvent) { s.Canonical = append(s.Canonical, ' ') }},
		{"digest", pub, func(s *identity.SignedPrincipalEvent) { s.Digest = "sha256:00" }},
		{"key id", pub, func(s *identity.SignedPrincipalEvent) { s.SigningKeyID = "ed25519:wrong" }},
		{"signature", pub, func(s *identity.SignedPrincipalEvent) { s.Signature = "00" }},
		{"foreign key", foreignPub, func(*identity.SignedPrincipalEvent) {}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			changed := signedEvent
			changed.Canonical = append([]byte(nil), signedEvent.Canonical...)
			tt.mutate(&changed)
			if err := identity.VerifyPrincipalEvent(tt.pub, changed); !errors.Is(err, identity.ErrIdentityEvidenceCorrupt) {
				t.Fatalf("VerifyPrincipalEvent error = %v, want corrupt", err)
			}
		})
	}

	legacy := identity.BirthSnapshot{}
	if !identity.SnapshotIsLegacy(legacy) {
		t.Fatal("zero snapshot is not legacy")
	}
	base := identity.BirthSnapshot{
		Version: 1, Digest: signedEvidence.Digest, Canonical: signedEvidence.Canonical,
		SigningKeyID: signedEvidence.SigningKeyID, Signature: signedEvidence.Signature,
	}
	baseDigest := identity.SnapshotDigest(base)
	changes := []identity.BirthSnapshot{
		{Version: 2, Digest: base.Digest, Canonical: base.Canonical, SigningKeyID: base.SigningKeyID, Signature: base.Signature},
		{Version: base.Version, Digest: "sha256:other", Canonical: base.Canonical, SigningKeyID: base.SigningKeyID, Signature: base.Signature},
		{Version: base.Version, Digest: base.Digest, Canonical: []byte("other"), SigningKeyID: base.SigningKeyID, Signature: base.Signature},
		{Version: base.Version, Digest: base.Digest, Canonical: base.Canonical, SigningKeyID: "ed25519:other", Signature: base.Signature},
		{Version: base.Version, Digest: base.Digest, Canonical: base.Canonical, SigningKeyID: base.SigningKeyID, Signature: "other"},
	}
	for _, changed := range changes {
		if identity.SnapshotIsLegacy(changed) || identity.SnapshotDigest(changed) == baseDigest {
			t.Fatalf("changed snapshot retained legacy/digest classification: %+v", changed)
		}
	}

	for _, raw := range [][]byte{
		append(append([]byte(nil), signedEvidence.Canonical...), []byte(` {}`)...),
		[]byte(`{"unknown":true}`),
		[]byte(`{"evidence_id":`),
	} {
		if _, err := identity.ParseCanonicalEvidence(raw); err == nil {
			t.Fatalf("ParseCanonicalEvidence accepted %q", raw)
		}
	}
	if _, err := identity.ParseCanonicalPrincipalEvent([]byte(`{} {}`)); err == nil {
		t.Fatal("ParseCanonicalPrincipalEvent accepted trailing content")
	}
}
