// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Identity domain (Trust Layer Etapa 2, lote 1, spec FR-PRIN/FR-EVID):
// the Principal born from authenticated provenance and the evidence of
// HOW each request authenticated. tenant_id stays RESERVED (single fixed
// local tenant until Etapa 10); no field in this file can hold secret
// material — identities carry names, kinds, digests and times only.
package action

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// PrincipalType is the finite actor family (§10.1 subset for Etapa 2).
type PrincipalType string

const (
	// PrincipalOperatorHuman is the single operator of this instance.
	PrincipalOperatorHuman PrincipalType = "operator_human"
	// PrincipalAgentBrain is a configured brain acting as an agent.
	PrincipalAgentBrain PrincipalType = "agent_brain"
	// PrincipalChannelPeer is the remote party behind a network channel.
	PrincipalChannelPeer PrincipalType = "channel_peer"
)

// Principal is the authenticated actor (§10.1). DisplayName is decoration
// and is NEVER used for authorization — the resolver does not even accept
// one as input.
type Principal struct {
	// PrincipalID is the stable internal identity.
	PrincipalID string
	// Type is the actor family.
	Type PrincipalType
	// DisplayName is presentation only; never an authorization input.
	DisplayName string
	// ResponsibleHumanID names the human behind an agent principal
	// (§14.2); empty for channel peers.
	ResponsibleHumanID string
	// CreatedAt / DisabledAt are lifecycle facts owned by the store
	// (lote 3); zero until persisted.
	CreatedAt  time.Time
	DisabledAt time.Time
}

// CredentialType is the FINITE enum of transport credentials (§10.2) —
// kinds, never values, so secret material is unrepresentable here.
type CredentialType string

const (
	// CredentialBotTokenSession backs Telegram-style bot API sessions.
	CredentialBotTokenSession CredentialType = "bot_token_session" // #nosec G101 -- credential KIND label, not a credential; values are unrepresentable here by design
	// CredentialInboundBearer backs the webhook's shared inbound Bearer.
	CredentialInboundBearer CredentialType = "inbound_bearer"
	// CredentialGatewaySession backs Discord-style gateway sessions.
	CredentialGatewaySession CredentialType = "gateway_session"
	// CredentialLoopbackInProcess backs the desktop console: in-process,
	// loopback-only — the operator's own hands.
	CredentialLoopbackInProcess CredentialType = "loopback_inprocess" // #nosec G101 -- credential KIND label, not a credential
)

// IdentityEvidence records HOW one request authenticated (§10.2): kinds,
// names, digests and times — no secret material by construction.
type IdentityEvidence struct {
	// EvidenceID is the kernel-generated identity ("evd_" + random hex).
	EvidenceID string
	// Provider is the channel class ("telegram", "webhook", ...).
	Provider string
	// Subject is the channel-scoped sender claim (data, never a card).
	Subject string
	// Credential is the transport credential KIND.
	Credential CredentialType
	// IssuedAt is the request instant, UTC.
	IssuedAt time.Time
	// TransportBinding names the configured channel the request rode.
	TransportBinding string
	// ClaimsDigest is the pinned-algorithm digest of the non-secret
	// claims, via the fuzzed lote-1 canonicalizer.
	ClaimsDigest string
}

// ErrUnknownProvenance reports a channel absent from the registry: an
// unauthenticated origin fails CLOSED (§7.5).
var ErrUnknownProvenance = errors.New("action: unknown provenance")

// Provenance is one channel's config-pinned identity: its class and the
// transport credential kind that authenticates it.
type Provenance struct {
	// Class is the channel type ("console", "telegram", "discord",
	// "webhook").
	Class string
	// Credential is the transport credential kind backing the class.
	Credential CredentialType
}

// ProvenanceRegistry maps configured channel NAMES to their provenance.
// The app wires it from config at boot; adapters change zero lines.
type ProvenanceRegistry map[string]Provenance

// operatorPrincipalID is the stable identity of the single operator.
const operatorPrincipalID = "principal_operator"

// classConsole is the provenance class that IS the operator's own hands.
const classConsole = "console"

// OperatorPrincipal returns the single operator of this instance (§14.2,
// single-operator form): the root of every responsibility chain.
func OperatorPrincipal() Principal {
	return Principal{PrincipalID: operatorPrincipalID, Type: PrincipalOperatorHuman}
}

// BrainPrincipal returns the agent principal for a configured brain: a
// distinct principal per brain name, with the operator as the responsible
// human behind it (§14.2).
func BrainPrincipal(name string) Principal {
	return Principal{
		PrincipalID:        "principal_brain_" + name,
		Type:               PrincipalAgentBrain,
		ResponsibleHumanID: operatorPrincipalID,
	}
}

// NewEvidenceID generates a fresh evidence identity: "evd_" plus 16
// random bytes in lowercase hex (the NewID mold).
func NewEvidenceID() string {
	b := make([]byte, 16)
	// crypto/rand.Read is documented (Go ≥1.24) to always succeed; the
	// explicit ignore keeps errcheck honest about that contract.
	_, _ = rand.Read(b)
	return "evd_" + hex.EncodeToString(b)
}

// HashCanonical digests arbitrary raw content through the fuzzed lote-1
// canonicalizer, rendered in the pinned-algorithm form ("sha256:<hex>").
func HashCanonical(raw string) string {
	sum := sha256.Sum256(CanonicalParams(raw))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// ResolvePrincipal derives the principal and its evidence from
// AUTHENTICATED provenance ONLY (§14.1): the registry's config-pinned
// channel class decides the principal — console IS the operator, every
// other class yields ONE channel_peer principal per channel (sealed
// decision 4) — and the sender survives strictly as the evidence subject.
// A channel absent from the registry fails CLOSED with the sentinel.
func ResolvePrincipal(reg ProvenanceRegistry, channelName, senderID string, at time.Time) (Principal, IdentityEvidence, error) {
	provenance, ok := reg[channelName]
	if !ok {
		return Principal{}, IdentityEvidence{}, fmt.Errorf("%w: channel %q", ErrUnknownProvenance, channelName)
	}
	principal := OperatorPrincipal()
	if provenance.Class != classConsole {
		principal = Principal{
			PrincipalID: "principal_ch_" + channelName,
			Type:        PrincipalChannelPeer,
		}
	}
	claims, err := json.Marshal(map[string]string{
		"channel": channelName,
		"class":   provenance.Class,
		"subject": senderID,
	})
	if err != nil {
		// Unreachable for a map of strings; kept for honesty.
		return Principal{}, IdentityEvidence{}, fmt.Errorf("action: marshal claims: %w", err)
	}
	evidence := IdentityEvidence{
		EvidenceID:       NewEvidenceID(),
		Provider:         provenance.Class,
		Subject:          senderID,
		Credential:       provenance.Credential,
		IssuedAt:         at.UTC(),
		TransportBinding: channelName,
		ClaimsDigest:     HashCanonical(string(claims)),
	}
	return principal, evidence, nil
}
