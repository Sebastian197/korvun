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
	"errors"
	"strings"
	"testing"
	"time"
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
	principal, evidence, err := ResolvePrincipal(testRegistry(), "console", "whatever-the-ui-sent", atE2)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if principal.Type != PrincipalOperatorHuman || principal.PrincipalID != OperatorPrincipal().PrincipalID {
		t.Fatalf("console provenance IS the operator, got %+v", principal)
	}
	if evidence.Credential != CredentialLoopbackInProcess {
		t.Fatalf("console evidence credential = %s", evidence.Credential)
	}
}

func TestResolve_principalPerChannelSenderIsOnlySubject(t *testing.T) {
	t.Parallel()
	a, evA, err := ResolvePrincipal(testRegistry(), "telegram", "user-1", atE2)
	if err != nil {
		t.Fatalf("resolve a: %v", err)
	}
	b, evB, err := ResolvePrincipal(testRegistry(), "telegram", "user-2", atE2)
	if err != nil {
		t.Fatalf("resolve b: %v", err)
	}
	if a.PrincipalID != b.PrincipalID {
		t.Fatalf("sealed decision 4: ONE principal per channel; got %q vs %q", a.PrincipalID, b.PrincipalID)
	}
	if evA.Subject == evB.Subject {
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
	_, evidence, err := ResolvePrincipal(testRegistry(), "telegram", "user-7", atE2)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !strings.HasPrefix(evidence.EvidenceID, "evd_") {
		t.Fatalf("evidence ids carry the evd_ prefix, got %q", evidence.EvidenceID)
	}
	if !evidence.IssuedAt.Equal(atE2) || evidence.IssuedAt.Location() != time.UTC {
		t.Fatalf("issued_at must be the request instant in UTC, got %v", evidence.IssuedAt)
	}
	if evidence.TransportBinding != "telegram" {
		t.Fatalf("transport binding names the channel, got %q", evidence.TransportBinding)
	}
	if !strings.HasPrefix(evidence.ClaimsDigest, "sha256:") {
		t.Fatalf("claims digest reuses the pinned-algorithm form, got %q", evidence.ClaimsDigest)
	}
	// B10 standard, by construction: the evidence type has NO field that
	// could hold secret material — only names, kinds, digests and times.
	// (Enforced structurally; this test documents the contract.)
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
