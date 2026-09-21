// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package identity resolves authenticated ingress capabilities into the
// requester, workload actor, and responsible principal of one action.
package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// ErrIdentityEvidenceMissing reports a zero or absent ingress capability.
	ErrIdentityEvidenceMissing = errors.New("identity: authenticated ingress missing")
	// ErrIdentityEvidenceExpired reports an ingress outside its validity window.
	ErrIdentityEvidenceExpired = errors.New("identity: authenticated ingress expired")
	// ErrIdentityBindingMismatch reports a foreign request, channel, binding,
	// generation, issuer, or adapter instance.
	ErrIdentityBindingMismatch = errors.New("identity: ingress binding mismatch")
	// ErrIdentityEvidenceCorrupt reports signed identity bytes that do not verify.
	ErrIdentityEvidenceCorrupt = errors.New("identity: evidence corrupt")
	// ErrIdentityEvidenceUnreadable reports a durable identity read failure.
	ErrIdentityEvidenceUnreadable = errors.New("identity: evidence unreadable")
	// ErrPrincipalDisabled reports a requester, actor, or responsible principal
	// disabled before the action start committed.
	ErrPrincipalDisabled = errors.New("identity: principal disabled")
	// ErrPrincipalEvidenceCorrupt reports a principal projection that does not
	// match its latest signed lifecycle event.
	ErrPrincipalEvidenceCorrupt = errors.New("identity: principal evidence corrupt")
	// ErrSignedObjectMutated reports a signer that changed the object it received.
	ErrSignedObjectMutated = errors.New("identity: signer mutated protected fields")
	// ErrSigningKeyRetired reports an attempt to birth evidence under retired ink.
	ErrSigningKeyRetired = errors.New("identity: signing key retired")
)

// PrincipalKind is the finite identity family stored in the principal registry.
type PrincipalKind string

const (
	// PrincipalHuman is a configured accountability principal. A shared
	// credential never creates one from a sender claim.
	PrincipalHuman PrincipalKind = "human"
	// PrincipalAgent is an autonomous principal not tied to a running workload.
	PrincipalAgent PrincipalKind = "agent"
	// PrincipalWorkload is a configured brain process identity.
	PrincipalWorkload PrincipalKind = "workload"
	// PrincipalExternalSystem is a shared channel or remote system identity.
	PrincipalExternalSystem PrincipalKind = "external_system"
)

// Principal is a configured authority identity. DisplayName is decoration.
type Principal struct {
	ID          string
	Kind        PrincipalKind
	DisplayName string
	CreatedAt   time.Time
	DisabledAt  time.Time
	Revision    int64
}

// BindingStatus is the finite lifecycle of an authentication binding.
type BindingStatus string

const (
	// BindingActive admits ingress under the configured generation.
	BindingActive BindingStatus = "active"
	// BindingRevoked refuses new starts while preserving historical evidence.
	BindingRevoked BindingStatus = "revoked"
)

// Binding maps one authenticated channel credential reference to a principal.
// CredentialRef is a configuration reference, never the credential value.
type Binding struct {
	ID               string
	Provider         string
	Channel          string
	CredentialRef    string
	SubjectNamespace string
	VerifiedSubject  string
	PrincipalID      string
	Generation       int64
	Status           BindingStatus
}

// Workload maps a configured brain to its actor and optional responsible
// principal. The actor never comes from message content.
type Workload struct {
	Brain                  string
	PrincipalID            string
	ResponsiblePrincipalID string
}

// Registry is the configuration snapshot used by a resolver and persisted at
// boot. Slices are copied at construction.
type Registry struct {
	Principals []Principal
	Bindings   []Binding
	Workloads  []Workload
}

// IssuerConfig fixes the non-secret authentication facts of one adapter.
type IssuerConfig struct {
	BindingID       string
	Method          string
	CredentialClass string
	TTL             time.Duration
}

type ingressSeal struct {
	resolver *Resolver
	binding  string
	instance string
}

// AuthenticatedIngress is an opaque, non-serializable capability minted by an
// adapter only after its transport authentication succeeds. Its zero value is
// invalid and JSON input cannot populate it.
type AuthenticatedIngress struct {
	seal         *ingressSeal
	requestID    string
	subjectClaim string
	observedAt   time.Time
	expiresAt    time.Time
	method       string
	credential   string
}

// Issuer mints ingress for one resolver-owned adapter instance.
type Issuer struct {
	seal       *ingressSeal
	resolver   *Resolver
	binding    Binding
	method     string
	credential string
	ttl        time.Duration
	issued     atomic.Uint64
}

// Resolver owns a sealed registry snapshot and its live adapter issuers.
type Resolver struct {
	mu         sync.RWMutex
	principals map[string]Principal
	bindings   map[string]Binding
	workloads  map[string]Workload
	issuers    map[string]*ingressSeal
	now        func() time.Time
}

// ResolveRequest binds ingress to one canonical action birth.
type ResolveRequest struct {
	ActionID  string
	RequestID string
	Channel   string
	Brain     string
}

// Evidence is the fixed, secret-free identity statement for one action.
type Evidence struct {
	EvidenceID             string    `json:"evidence_id"`
	ActionID               string    `json:"action_id"`
	RequestID              string    `json:"request_id"`
	RequesterPrincipalID   string    `json:"requester_principal_id"`
	ActorPrincipalID       string    `json:"actor_principal_id"`
	ResponsiblePrincipalID string    `json:"responsible_principal_id,omitempty"`
	BindingID              string    `json:"binding_id"`
	BindingGeneration      int64     `json:"binding_generation"`
	AdapterInstanceID      string    `json:"adapter_instance_id"`
	Provider               string    `json:"provider"`
	Method                 string    `json:"method"`
	CredentialClass        string    `json:"credential_class"`
	Issuer                 string    `json:"issuer"`
	SubjectNamespace       string    `json:"subject_namespace"`
	VerifiedSubject        string    `json:"verified_subject"`
	SubjectClaim           string    `json:"subject_claim"`
	ObservedAt             time.Time `json:"observed_at"`
	ExpiresAt              time.Time `json:"expires_at"`
	ClaimsDigest           string    `json:"claims_digest"`
}

// NewResolver validates and copies a configuration registry.
func NewResolver(reg Registry, now func() time.Time) (*Resolver, error) {
	if now == nil {
		now = time.Now
	}
	r := &Resolver{
		principals: make(map[string]Principal, len(reg.Principals)),
		bindings:   make(map[string]Binding, len(reg.Bindings)),
		workloads:  make(map[string]Workload, len(reg.Workloads)),
		issuers:    make(map[string]*ingressSeal),
		now:        now,
	}
	for _, p := range reg.Principals {
		if p.ID == "" || !validPrincipalKind(p.Kind) {
			return nil, fmt.Errorf("identity: invalid principal %q", p.ID)
		}
		if _, exists := r.principals[p.ID]; exists {
			return nil, fmt.Errorf("identity: duplicate principal %q", p.ID)
		}
		r.principals[p.ID] = p
	}
	for _, b := range reg.Bindings {
		if b.ID == "" || b.Provider == "" || b.Channel == "" ||
			b.CredentialRef == "" || b.SubjectNamespace == "" || b.VerifiedSubject == "" ||
			b.Generation <= 0 || (b.Status != BindingActive && b.Status != BindingRevoked) {
			return nil, fmt.Errorf("identity: invalid binding %q", b.ID)
		}
		if _, ok := r.principals[b.PrincipalID]; !ok {
			return nil, fmt.Errorf("identity: binding %q references unknown principal %q", b.ID, b.PrincipalID)
		}
		if _, exists := r.bindings[b.ID]; exists {
			return nil, fmt.Errorf("identity: duplicate binding %q", b.ID)
		}
		r.bindings[b.ID] = b
	}
	for _, w := range reg.Workloads {
		actor, ok := r.principals[w.PrincipalID]
		if w.Brain == "" || !ok || actor.Kind != PrincipalWorkload {
			return nil, fmt.Errorf("identity: invalid workload %q", w.Brain)
		}
		if w.ResponsiblePrincipalID != "" {
			if _, ok := r.principals[w.ResponsiblePrincipalID]; !ok {
				return nil, fmt.Errorf("identity: workload %q references unknown responsible principal", w.Brain)
			}
		}
		if _, exists := r.workloads[w.Brain]; exists {
			return nil, fmt.Errorf("identity: duplicate workload %q", w.Brain)
		}
		r.workloads[w.Brain] = w
	}
	return r, nil
}

// NewIssuer creates a fresh adapter-instance capability for one binding.
//
// ONE LIVE ISSUER PER BINDING, said plainly because the wire enforces it: a
// second NewIssuer over the same binding SUPERSEDES the first, and every
// capability the first still has in flight stops resolving from that moment,
// with ErrIdentityBindingMismatch — the sentinel names the seal that no longer
// belongs to the binding, which is what happened, not the adapter's intent. The
// refusal is fail-closed and no evidence is ever born from a superseded seal. A
// caller that restarts an adapter instance therefore drains the old one first
// (the twentieth pass, P3-4).
func (r *Resolver) NewIssuer(cfg IssuerConfig) (*Issuer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.bindings[cfg.BindingID]
	if !ok || cfg.Method == "" || cfg.CredentialClass == "" || cfg.TTL < 0 {
		return nil, fmt.Errorf("%w: issuer configuration", ErrIdentityBindingMismatch)
	}
	seal := &ingressSeal{resolver: r, binding: b.ID, instance: newID("adapter_")}
	r.issuers[b.ID] = seal
	return &Issuer{
		seal: seal, resolver: r, binding: b, method: cfg.Method,
		credential: cfg.CredentialClass, ttl: cfg.TTL,
	}, nil
}

// Issue mints ingress for one authenticated request. Callers pass only the
// request correlation and the channel's honest subject claim, never a token.
func (i *Issuer) Issue(requestID, subjectClaim string) (AuthenticatedIngress, error) {
	if i == nil || i.seal == nil || requestID == "" || subjectClaim == "" {
		return AuthenticatedIngress{}, ErrIdentityEvidenceMissing
	}
	at := i.resolver.now().UTC()
	expires := time.Time{}
	if i.ttl > 0 {
		expires = at.Add(i.ttl)
	}
	i.issued.Add(1)
	return AuthenticatedIngress{
		seal: i.seal, requestID: requestID, subjectClaim: subjectClaim,
		observedAt: at, expiresAt: expires, method: i.method,
		credential: i.credential,
	}, nil
}

// IssuedCount returns the number of capabilities minted by this adapter
// issuer. It contains no request or credential data and supports gate audits.
func (i *Issuer) IssuedCount() uint64 {
	if i == nil {
		return 0
	}
	return i.issued.Load()
}

// Resolve consumes no capability state; fan-out may resolve one ingress into
// distinct action evidence while preserving the authenticated request fact.
func (r *Resolver) Resolve(in AuthenticatedIngress, req ResolveRequest) (Evidence, error) {
	if in.seal == nil {
		return Evidence{}, ErrIdentityEvidenceMissing
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if in.seal.resolver != r || r.issuers[in.seal.binding] != in.seal ||
		in.requestID != req.RequestID || req.ActionID == "" || req.RequestID == "" {
		return Evidence{}, ErrIdentityBindingMismatch
	}
	binding, ok := r.bindings[in.seal.binding]
	if !ok || binding.Channel != req.Channel || binding.Status != BindingActive {
		return Evidence{}, ErrIdentityBindingMismatch
	}
	now := r.now().UTC()
	if !in.expiresAt.IsZero() && !now.Before(in.expiresAt) {
		return Evidence{}, ErrIdentityEvidenceExpired
	}
	workload, ok := r.workloads[req.Brain]
	if !ok {
		return Evidence{}, ErrIdentityBindingMismatch
	}
	for _, principalID := range []string{binding.PrincipalID, workload.PrincipalID, workload.ResponsiblePrincipalID} {
		if principalID == "" {
			continue
		}
		principal, exists := r.principals[principalID]
		if !exists {
			return Evidence{}, ErrIdentityBindingMismatch
		}
		if !principal.DisabledAt.IsZero() && !principal.DisabledAt.After(now) {
			return Evidence{}, ErrPrincipalDisabled
		}
	}
	claims := struct {
		BindingID        string `json:"binding_id"`
		Generation       int64  `json:"generation"`
		Provider         string `json:"provider"`
		Method           string `json:"method"`
		SubjectNamespace string `json:"subject_namespace"`
		VerifiedSubject  string `json:"verified_subject"`
		SubjectClaim     string `json:"subject_claim"`
	}{binding.ID, binding.Generation, binding.Provider, in.method, binding.SubjectNamespace,
		binding.VerifiedSubject, in.subjectClaim}
	// claims is a closed struct of strings and one int64, so encoding/json
	// cannot fail on it; the blank is that fact (the twentieth pass, P3-8).
	raw, _ := json.Marshal(claims)
	return Evidence{
		EvidenceID: newID("evd_"), ActionID: req.ActionID, RequestID: req.RequestID,
		RequesterPrincipalID:   binding.PrincipalID,
		ActorPrincipalID:       workload.PrincipalID,
		ResponsiblePrincipalID: workload.ResponsiblePrincipalID,
		BindingID:              binding.ID, BindingGeneration: binding.Generation,
		AdapterInstanceID: in.seal.instance, Provider: binding.Provider,
		Method: in.method, CredentialClass: in.credential,
		Issuer: binding.Channel, SubjectNamespace: binding.SubjectNamespace,
		VerifiedSubject: binding.VerifiedSubject, SubjectClaim: in.subjectClaim,
		ObservedAt: in.observedAt,
		ExpiresAt:  in.expiresAt, ClaimsDigest: hash(raw),
	}, nil
}

func validPrincipalKind(kind PrincipalKind) bool {
	switch kind {
	case PrincipalHuman, PrincipalAgent, PrincipalWorkload, PrincipalExternalSystem:
		return true
	default:
		return false
	}
}

func newID(prefix string) string {
	raw := make([]byte, 16)
	// crypto/rand.Read's own documentation, on the toolchain this module pins
	// (go 1.26.6): «It never returns an error, and always fills b entirely»,
	// and it crashes the program irrecoverably rather than reporting a failed
	// system source. The blank is that fact, not a swallowed failure (the
	// twentieth pass, P3-8).
	_, _ = rand.Read(raw)
	return prefix + hex.EncodeToString(raw)
}

func hash(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}
