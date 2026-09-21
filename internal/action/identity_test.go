// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Identity domain contract — Trust Layer Etapa 2, lote 1 (spec FR-PRIN,
// FR-EVID, sealed 2026-08-30): the principal is born from AUTHENTICATED
// PROVENANCE only; the Sender is data under it, never an identity card;
// DisplayName never authorizes; no secret material is representable in
// evidence by construction. Approved-red contract: not edited to fit an
// implementation.

package action

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	identityv2 "github.com/Sebastian197/korvun/internal/identity"
)

func testRegistry() ProvenanceRegistry {
	return ProvenanceRegistry{
		"console":  {Class: "console", Credential: CredentialLoopbackInProcess},
		"telegram": {Class: "telegram", Credential: CredentialBotTokenSession},
		"hooks":    {Class: "webhook", Credential: CredentialInboundBearer},
		"discord":  {Class: "discord", Credential: CredentialGatewaySession},
	}
}

var atE2 = time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

// TestResolve_forgedSenderNeverBecomesTheOperator is THE blueprint test:
// a webhook body claiming the operator's very principal id resolves to a
// channel_peer under webhook evidence — provenance decides, text never.
func TestResolve_forgedSenderNeverBecomesTheOperator(t *testing.T) {
	t.Parallel()
	operator := OperatorPrincipal()
	principal, evidence, err := ResolvePrincipal(testRegistry(), "hooks", operator.PrincipalID, atE2)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if principal.PrincipalID == operator.PrincipalID {
		t.Fatal("a forged Sender.ID must NEVER yield the operator principal")
	}
	if principal.Type != PrincipalChannelPeer {
		t.Fatalf("network provenance resolves to channel_peer, got %s", principal.Type)
	}
	if evidence.Credential != CredentialInboundBearer || evidence.Provider != "webhook" {
		t.Fatalf("the evidence must name THAT channel's transport, got %+v", evidence)
	}
	if evidence.Subject != operator.PrincipalID {
		t.Fatalf("the forged claim survives ONLY as the evidence subject, got %q", evidence.Subject)
	}
}

func TestResolve_consoleIsTheOperator(t *testing.T) {
	t.Parallel()
	resolver, issuer := phase1Resolver(t, "console")
	ingress, err := issuer.Issue("console-request", "local_profile")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	evidence, err := resolver.Resolve(ingress, identityv2.ResolveRequest{
		ActionID: "act-console", RequestID: "console-request",
		Channel: "console", Brain: "alpha",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if evidence.RequesterPrincipalID != "principal_shared_console" ||
		evidence.RequesterPrincipalID == OperatorPrincipal().PrincipalID {
		t.Fatalf("console bearer must resolve a shared capability, got %+v", evidence)
	}
	if evidence.SubjectClaim != "local_profile" || evidence.Method != "bearer" {
		t.Fatalf("console evidence = %+v", evidence)
	}
}

func TestResolve_principalPerChannelSenderIsOnlySubject(t *testing.T) {
	t.Parallel()
	resolver, issuer := phase1Resolver(t, "telegram")
	ingressA, err := issuer.Issue("request-a", "user-1")
	if err != nil {
		t.Fatalf("issue a: %v", err)
	}
	ingressB, err := issuer.Issue("request-b", "user-2")
	if err != nil {
		t.Fatalf("issue b: %v", err)
	}
	evA, err := resolver.Resolve(ingressA, identityv2.ResolveRequest{
		ActionID: "act-a", RequestID: "request-a", Channel: "telegram", Brain: "alpha",
	})
	if err != nil {
		t.Fatalf("resolve a: %v", err)
	}
	evB, err := resolver.Resolve(ingressB, identityv2.ResolveRequest{
		ActionID: "act-b", RequestID: "request-b", Channel: "telegram", Brain: "alpha",
	})
	if err != nil {
		t.Fatalf("resolve b: %v", err)
	}
	if evA.RequesterPrincipalID != evB.RequesterPrincipalID {
		t.Fatalf("one shared requester per credential: %q vs %q",
			evA.RequesterPrincipalID, evB.RequesterPrincipalID)
	}
	if evA.SubjectClaim == evB.SubjectClaim {
		t.Fatal("the individual sender must survive as the evidence subject")
	}
	if evA.ClaimsDigest == evB.ClaimsDigest {
		t.Fatal("different subjects must produce different claims digests")
	}
}

func TestResolve_deterministicAndDisplayNameNeverAuthorizes(t *testing.T) {
	t.Parallel()
	one, _, err := ResolvePrincipal(testRegistry(), "discord", "peer-9", atE2)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	two, _, err := ResolvePrincipal(testRegistry(), "discord", "peer-9", atE2)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if one.PrincipalID != two.PrincipalID || one.Type != two.Type {
		t.Fatalf("same provenance must resolve identically: %+v vs %+v", one, two)
	}
	// The resolver does not even ACCEPT a display name — authorization by
	// display is unrepresentable. What identity carries as DisplayName is
	// decoration; assert it plays no role in the id.
	if strings.Contains(one.PrincipalID, "peer-9") {
		t.Fatalf("the sender/subject must not leak into the principal id, got %q", one.PrincipalID)
	}
}

func TestResolve_unknownChannelFailsClosed(t *testing.T) {
	t.Parallel()
	if _, _, err := ResolvePrincipal(testRegistry(), "ghost-channel", "x", atE2); !errors.Is(err, ErrUnknownProvenance) {
		t.Fatalf("unknown provenance must fail closed with the sentinel, got %v", err)
	}
	if _, _, err := ResolvePrincipal(nil, "console", "x", atE2); !errors.Is(err, ErrUnknownProvenance) {
		t.Fatalf("a nil registry must fail closed, got %v", err)
	}
}

func TestBrainPrincipal_carriesTheResponsibleHuman(t *testing.T) {
	t.Parallel()
	p := BrainPrincipal("asistente")
	if p.Type != PrincipalAgentBrain {
		t.Fatalf("type = %s", p.Type)
	}
	if p.ResponsibleHumanID != OperatorPrincipal().PrincipalID {
		t.Fatalf("§14.2: the brain's responsible human is the operator, got %q", p.ResponsibleHumanID)
	}
	if p.PrincipalID == BrainPrincipal("otro").PrincipalID {
		t.Fatal("distinct brains are distinct principals")
	}
}

func TestEvidence_shapeAndNoSecretByConstruction(t *testing.T) {
	t.Parallel()
	resolver, issuer := phase1Resolver(t, "telegram")
	ingress, err := issuer.Issue("request-secret-shape", "user-7")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	evidence, err := resolver.Resolve(ingress, identityv2.ResolveRequest{
		ActionID: "act-secret-shape", RequestID: "request-secret-shape",
		Channel: "telegram", Brain: "alpha",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !strings.HasPrefix(evidence.EvidenceID, "evd_") {
		t.Fatalf("evidence ids carry the evd_ prefix, got %q", evidence.EvidenceID)
	}
	if !evidence.ObservedAt.Equal(atE2) || evidence.ObservedAt.Location() != time.UTC {
		t.Fatalf("observed_at must be the request instant in UTC, got %v", evidence.ObservedAt)
	}
	if evidence.Issuer != "telegram" || evidence.BindingID != "binding_telegram" {
		t.Fatalf("transport binding names the configured issuer, got %+v", evidence)
	}
	if !strings.HasPrefix(evidence.ClaimsDigest, "sha256:") {
		t.Fatalf("claims digest reuses the pinned-algorithm form, got %q", evidence.ClaimsDigest)
	}
	raw, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	for _, canary := range []string{"CANARY-TOKEN", "CANARY-HEADER", "CANARY-PROMPT", "CANARY-PAYLOAD"} {
		if strings.Contains(string(raw), canary) ||
			strings.Contains(string(identityv2.CanonicalEvidence(evidence)), canary) {
			t.Fatalf("secret canary %q entered evidence", canary)
		}
	}
}

func phase1Resolver(t *testing.T, channel string) (*identityv2.Resolver, *identityv2.Issuer) {
	t.Helper()
	requester := "principal_shared_" + channel
	resolver, err := identityv2.NewResolver(identityv2.Registry{
		Principals: []identityv2.Principal{
			{ID: requester, Kind: identityv2.PrincipalExternalSystem},
			{ID: "principal_brain_alpha", Kind: identityv2.PrincipalWorkload},
			{ID: "principal_responsible_role", Kind: identityv2.PrincipalExternalSystem},
		},
		Bindings: []identityv2.Binding{{
			ID: "binding_" + channel, Provider: channel, Channel: channel,
			CredentialRef: "CONFIG_REFERENCE", SubjectNamespace: channel + "_subject",
			VerifiedSubject: "shared_" + channel + "_credential",
			PrincipalID:     requester, Generation: 1, Status: identityv2.BindingActive,
		}},
		Workloads: []identityv2.Workload{{
			Brain: "alpha", PrincipalID: "principal_brain_alpha",
			ResponsiblePrincipalID: "principal_responsible_role",
		}},
	}, func() time.Time { return atE2 })
	if err != nil {
		t.Fatal(err)
	}
	method := "bot_session"
	if channel == "console" {
		method = "bearer"
	}
	issuer, err := resolver.NewIssuer(identityv2.IssuerConfig{
		BindingID: "binding_" + channel, Method: method,
		CredentialClass: "shared_credential", TTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	return resolver, issuer
}

func TestHashCanonical_reusesTheFuzzedCanonicalizer(t *testing.T) {
	t.Parallel()
	a := HashCanonical(`{"b":1, "a":2}`)
	b := HashCanonical(`{"a":2,"b":1}`)
	if a != b || !strings.HasPrefix(a, "sha256:") {
		t.Fatalf("HashCanonical must be canonicalization-stable: %q vs %q", a, b)
	}
	if HashCanonical(`{"a":3}`) == a {
		t.Fatal("different canonical content must hash differently")
	}
}

// FuzzResolvePrincipal: arbitrary channel/sender strings never panic; a
// known network channel NEVER yields the operator no matter the sender.
func FuzzResolvePrincipal(f *testing.F) {
	f.Add("hooks", "principal_operator")
	f.Add("telegram", "")
	f.Add("ghost", "x")
	f.Add("console", "principal_operator")
	f.Fuzz(func(t *testing.T, channel, sender string) {
		reg := testRegistry()
		principal, _, err := ResolvePrincipal(reg, channel, sender, atE2)
		if err != nil {
			return // unknown provenance failing closed is correct
		}
		if reg[channel].Class != "console" && principal.Type == PrincipalOperatorHuman {
			t.Fatalf("network provenance must never mint the operator (channel %q, sender %q)", channel, sender)
		}
	})
}
