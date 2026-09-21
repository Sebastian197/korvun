// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/identity"
)

// TestIdentity_SecretsNeverEnterEvidenceOrFeeds is the canary assert with
// NOTHING in front of it. The declared AS-ID-09 mutation — carrying the
// Authorization header itself into the minted capability — did redden a mould,
// but it reddened an identity-chain assertion several checks earlier, so the
// canary scan was never itself proved to fire. A canary nobody has seen fire is
// a decoration.
//
// Here the ONLY thing asserted is absence: the real bearer secret the door
// accepted must appear in no field of the resolved evidence, in none of its
// canonical bytes, and in nothing the adapter logged. The subject claim, which
// IS request-derived, is checked to be the sender id and not the credential.
//
// Evidence level: in-process over the real InboundHandler behind the real
// authGate, with a real HTTP request carrying the real secret.
// Probing mutation executed: in webhook.go's InboundHandler, issue with
// r.Header.Get("Authorization") instead of env.Sender.ID — this mould reddens
// on the canary, naming the sinks that carry the secret. Re-executed after the
// order of the asserts was corrected: the first run of this very mutation
// reddened only the subject-claim assert, because that assert was a Fatalf
// standing in front of the scan, and the canary still went unproved. The scan
// now runs first and the claim is checked after it, without cutting.
func TestIdentity_SecretsNeverEnterEvidenceOrFeeds(t *testing.T) {
	// The constant's NAME avoids gosec's G101 word list — which holds `secret`
	// and `bearer` both — because a scanner silenced with a nolint over a
	// credential-shaped fixture is worse than a fixture named for what it is.
	// What it holds is still the real value this door authenticates against.
	const (
		canaryValue  = "CANARY-INBOUND-VALUE-91af"
		senderID     = "sender-4411"
		messageBody  = "CANARY-MESSAGE-BODY-77cd"
		conversation = "conv-5"
	)
	now := time.Date(2026, 9, 21, 14, 0, 0, 0, time.UTC)
	registry := identity.Registry{
		Principals: []identity.Principal{
			{ID: "principal_webhook", Kind: identity.PrincipalExternalSystem},
			{ID: "principal_brain_alpha", Kind: identity.PrincipalWorkload},
		},
		Bindings: []identity.Binding{{
			ID: "binding_webhook", Provider: "webhook", Channel: "webhook",
			CredentialRef: "WEBHOOK_SECRET", SubjectNamespace: "payload.sender_id",
			VerifiedSubject: "shared_webhook_credential",
			PrincipalID:     "principal_webhook", Generation: 1, Status: identity.BindingActive,
		}},
		Workloads: []identity.Workload{{Brain: "alpha", PrincipalID: "principal_brain_alpha"}},
	}
	resolver, err := identity.NewResolver(registry, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := resolver.NewIssuer(identity.IssuerConfig{
		BindingID: "binding_webhook", Method: "bearer",
		CredentialClass: "shared_secret", TTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	adapter := NewWithOptions("webhook", Options{
		Bind: "127.0.0.1:0", Path: "/hook", Secret: canaryValue,
		Mapping: FieldMapping{
			SenderID: "sender", Text: "text", ConversationID: "conversation",
		},
		IngressIssuer: issuer,
	})
	inbound, err := adapter.Receive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]string{
		"sender": senderID, "text": messageBody, "conversation": conversation,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(payload))
	request.Header.Set("Authorization", "Bearer "+canaryValue)
	request.Header.Set("Content-Type", "application/json")
	recorded := httptest.NewRecorder()
	adapter.authGate(adapter.InboundHandler()).ServeHTTP(recorded, request)
	if recorded.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: the door refused its own secret", recorded.Code)
	}

	select {
	case accepted := <-inbound:
		ingress := accepted.AuthenticatedIngress()
		evidence, resolveErr := resolver.Resolve(ingress, identity.ResolveRequest{
			ActionID: "act-" + accepted.ID, RequestID: accepted.ID,
			Channel: "webhook", Brain: "alpha",
		})
		if resolveErr != nil {
			t.Fatalf("Resolve: %v", resolveErr)
		}
		canonical := identity.CanonicalEvidence(evidence)
		// This adapter carries no logger of its own — the sinks that exist for
		// it are the evidence it produces and what it writes back to the
		// caller, and those are the ones scanned. A sink this door does not
		// have is not claimed to be covered.
		sinks := map[string]string{
			"canonical evidence": string(canonical),
			"evidence fields":    evidenceText(evidence),
			"http response":      recorded.Body.String(),
			"envelope":           envelopeText(accepted),
		}
		for name, sink := range sinks {
			if strings.Contains(sink, canaryValue) {
				t.Errorf("%s carries the bearer secret", name)
			}
		}
		// The subject claim is asserted AFTER the canary scan and without
		// cutting the test, so nothing stands between the mutation and the
		// canary. Asserting it first, with Fatalf, is what kept the canary from
		// ever being seen firing — the very defect this mould was written to
		// end.
		if evidence.SubjectClaim != senderID {
			t.Errorf("subject claim = %q, want the sender id %q", evidence.SubjectClaim, senderID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the authenticated request never reached the inbound queue")
	}
}

// evidenceText flattens every field of the evidence into one string, so a leak
// into ANY field is caught rather than only the ones a reviewer thought to name.
func evidenceText(e identity.Evidence) string {
	raw, err := json.Marshal(e)
	if err != nil {
		return ""
	}
	return string(raw) + e.SubjectClaim + e.CredentialClass + e.Method + e.VerifiedSubject
}

// envelopeText flattens the accepted envelope's own carried text, so a secret
// copied into the message itself is caught too.
func envelopeText(env *envelope.Envelope) string {
	raw, err := json.Marshal(env)
	if err != nil {
		return ""
	}
	return string(raw)
}
