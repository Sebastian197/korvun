// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package action is the Action Kernel's pure domain (Trust Layer Etapa 1,
// spec 2026-08-30-action-kernel.md): the ActionEnvelope v1, its
// deterministic canonicalization and digest, and the action state machine.
// It is a LEAF package with the same seam discipline as internal/tool: it
// imports only the standard library, so the brain adapts TO it and never
// the other way around.
//
// ActionEnvelope v1 carries the sealed subset of the blueprint's §10.5.
// The remaining §10.5 fields are RESERVED and arrive with their stages:
// intent_id, principal, authority_refs (Etapa 2); resource and the full
// effect classification (Etapa 3); protected_parameters_ref (Etapa 4);
// transaction_id, idempotency_key, expires_at (Etapa 6); tenant_id
// (Etapa 10). They are deliberately NOT fields yet — an unreachable field
// is a field nobody can misuse.
package action

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// Source names where an action request entered the kernel (§10.5 subset).
type Source struct {
	// Kind is the requester family; Etapa 1 only produces "agent_brain".
	Kind string
	// Protocol distinguishes the brain's lanes: "text" or "native".
	Protocol string
	// Channel is the inbound envelope's channel (finite, config-owned).
	Channel string
}

// Operation identifies WHAT is being requested, independent of arguments.
type Operation struct {
	// Namespace scopes the name; Etapa 1 only produces "tool".
	Namespace string
	// Name is the operation's protocol name (the tool.Tool Name()).
	Name string
	// Version is the operation schema version, pinned at 1 in Etapa 1.
	Version int
}

// Effect is the Etapa-1 placeholder for the blueprint's effect
// classification (§10.6): every action carries Class "unclassified" until
// the Effect Engine arrives in Etapa 3.
type Effect struct {
	// Class is the effect class; Etapa 1 pins "unclassified".
	Class string
}

// Envelope is the ActionEnvelope v1 (spec FR-DOM-2): the canonical,
// secret-free description of one requested action. It carries digests and
// identifiers, never raw prompt text and never secret material.
type Envelope struct {
	// SchemaVersion is pinned at 1 for this envelope generation.
	SchemaVersion int
	// ActionID is the kernel-generated identity ("act_" + 16 random bytes hex).
	ActionID string
	// CorrelationID ties the action to the inbound envelope.Envelope.ID.
	CorrelationID string
	// Source names where the request entered.
	Source Source
	// Operation names what is requested.
	Operation Operation
	// ParametersDigest is "sha256:<hex>" over the canonical parameters
	// (see Digest); the raw arguments themselves are NOT stored here.
	ParametersDigest string
	// Effect is the Etapa-1 placeholder classification.
	Effect Effect
	// RequestedAt is the request instant, normalized to UTC.
	RequestedAt time.Time
}

// CanonicalParams returns the deterministic canonical byte form of a tool's
// raw argument string (spec FR-DOM-3). If raw is exactly one JSON value
// (with nothing but whitespace around it), it is decoded with UseNumber —
// numeric literals survive verbatim, duplicate object keys resolve
// last-wins — and re-marshaled: encoding/json emits object keys sorted at
// every nesting level, which makes the byte form stable under key order
// and whitespace. Anything else (empty, plain text, trailing garbage,
// broken JSON) canonicalizes as its raw bytes verbatim.
func CanonicalParams(raw string) []byte {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return []byte(raw)
	}
	if _, err := dec.Token(); err != io.EOF {
		// A second token (or a syntax error) after the first value means
		// raw is NOT one JSON value; the raw bytes are the canon.
		return []byte(raw)
	}
	out, err := json.Marshal(value)
	if err != nil {
		// Unreachable for values produced by Decode, kept for honesty:
		// the raw bytes remain the deterministic fallback.
		return []byte(raw)
	}
	return out
}

// Digest computes the pinned-algorithm parameters digest ("sha256:<hex>")
// over the operation triple and the canonical arguments. Every field is
// length-prefixed ("<len>:<bytes>") before hashing so boundaries are
// unambiguous by construction — ("a","bc") can never collide with
// ("ab","c"). The algorithm identifier lives IN the string so a future
// algorithm can coexist with historical digests.
func Digest(op Operation, rawArgs string) string {
	h := sha256.New()
	part := func(b []byte) {
		_, _ = fmt.Fprintf(h, "%d:", len(b))
		_, _ = h.Write(b)
	}
	part([]byte(op.Namespace))
	part([]byte(op.Name))
	part([]byte(strconv.Itoa(op.Version)))
	part(CanonicalParams(rawArgs))
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// NewID generates a fresh action identity: "act_" plus 16 random bytes in
// lowercase hex.
func NewID() string {
	b := make([]byte, 16)
	// crypto/rand.Read is documented (Go ≥1.24) to always succeed; the
	// explicit ignore keeps errcheck honest about that contract.
	_, _ = rand.Read(b)
	return "act_" + hex.EncodeToString(b)
}

// NewEnvelope builds an ActionEnvelope v1 from its parts: schema pinned at
// 1, the parameters digest computed at construction, the Etapa-1 effect
// placeholder, and the request instant normalized to UTC.
func NewEnvelope(id, correlationID string, src Source, op Operation, rawArgs string, at time.Time) Envelope {
	return Envelope{
		SchemaVersion:    1,
		ActionID:         id,
		CorrelationID:    correlationID,
		Source:           src,
		Operation:        op,
		ParametersDigest: Digest(op, rawArgs),
		Effect:           Effect{Class: "unclassified"},
		RequestedAt:      at.UTC(),
	}
}
