// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package action

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"time"
)

const (
	intentContractDomain = "korvun.intent.v2"
	intentEventDomain    = "korvun.intent-event.v1"
	maxIntentBytes       = 64 << 10
	maxIntentDepth       = 8
	maxIntentSetEntries  = 256
	maxIntentPurpose     = 4096
)

var (
	// ErrIntentDuplicateField reports an object key repeated in input.
	ErrIntentDuplicateField = errors.New("action: intent duplicate field")
	// ErrIntentMalformed reports bytes the strict scanner cannot walk at all:
	// a truncated object, a stray comma, a key that is not a string. It is a
	// REFUSAL with a name, which is what the scanner owed and did not pay: the
	// earlier code asserted the key token and panicked instead.
	ErrIntentMalformed = errors.New("action: intent malformed")
	// ErrIntentUnknownField reports a field outside the version 2 schema.
	ErrIntentUnknownField = errors.New("action: intent unknown field")
	// ErrIntentSchemaVersion reports a schema version other than 2.
	ErrIntentSchemaVersion = errors.New("action: unsupported intent schema version")
	// ErrIntentBudget reports a non-integer or negative budget.
	ErrIntentBudget = errors.New("action: invalid intent budget")
	// ErrIntentEvidenceCorrupt reports terms or signed evidence that do not verify.
	ErrIntentEvidenceCorrupt = errors.New("action: intent evidence corrupt")
	// ErrSignedObjectMutated reports a signer that changed the object it received.
	ErrSignedObjectMutated = errors.New("action: signed object mutated")
	// ErrSigningKeyRetired reports an attempted new act with a retired key.
	ErrSigningKeyRetired = errors.New("action: signing key retired")
	// ErrIntentInactive reports a DRAFT intent presented for use.
	ErrIntentInactive = errors.New("action: intent inactive")
	// ErrIntentRevoked reports a revoked intent presented for use.
	ErrIntentRevoked = errors.New("action: intent revoked")
	// ErrIntentExpired reports an intent outside its half-open time window.
	ErrIntentExpired = errors.New("action: intent expired")
	// ErrAmbiguousIntentBinding reports equally specific active bindings.
	ErrAmbiguousIntentBinding = errors.New("action: ambiguous intent binding")
)

// OperationRef is one registered operation triple.
type OperationRef struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Version   int    `json:"version"`
}

// ResourceRef names an application-registered resource descriptor.
type ResourceRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// IntentBudgetV2 holds finite action counts. Nil total means unlimited.
type IntentBudgetV2 struct {
	Total        *int64           `json:"total"`
	PerOperation map[string]int64 `json:"per_operation"`
}

// ApprovalRequirementV2 records whether an approval is required.
type ApprovalRequirementV2 struct {
	Required bool `json:"required"`
}

// IntentContractV2 is the bounded, immutable terms object signed on activation.
type IntentContractV2 struct {
	IntentID           string                `json:"intent_id"`
	SchemaVersion      int                   `json:"schema_version"`
	Version            int                   `json:"version"`
	ProfileID          string                `json:"profile_id"`
	OwnerPrincipalID   string                `json:"owner_principal_id"`
	Purpose            string                `json:"purpose"`
	Operations         []OperationRef        `json:"operations"`
	AllowedResources   []ResourceRef         `json:"allowed_resources"`
	DeniedResources    []ResourceRef         `json:"denied_resources"`
	DataScope          []string              `json:"data_scope"`
	OutputDestinations []string              `json:"output_destinations"`
	EffectClasses      []EffectClass         `json:"effect_classes"`
	Budget             IntentBudgetV2        `json:"budget"`
	ValidFrom          time.Time             `json:"-"`
	ExpiresAt          time.Time             `json:"-"`
	Approval           ApprovalRequirementV2 `json:"approval"`
	MaxDelegationDepth int                   `json:"max_delegation_depth"`
}

type intentContractWireV2 struct {
	IntentID           string                `json:"intent_id"`
	SchemaVersion      int                   `json:"schema_version"`
	Version            int                   `json:"version"`
	ProfileID          string                `json:"profile_id"`
	OwnerPrincipalID   string                `json:"owner_principal_id"`
	Purpose            string                `json:"purpose"`
	Operations         []OperationRef        `json:"operations"`
	AllowedResources   []ResourceRef         `json:"allowed_resources"`
	DeniedResources    []ResourceRef         `json:"denied_resources"`
	DataScope          []string              `json:"data_scope"`
	OutputDestinations []string              `json:"output_destinations"`
	EffectClasses      []EffectClass         `json:"effect_classes"`
	Budget             IntentBudgetV2        `json:"budget"`
	ValidFrom          string                `json:"valid_from"`
	ExpiresAt          string                `json:"expires_at"`
	Approval           ApprovalRequirementV2 `json:"approval"`
	MaxDelegationDepth int                   `json:"max_delegation_depth"`
}

// ParseIntentContractV2 parses and validates the strict version 2 JSON schema.
func ParseIntentContractV2(raw []byte) (IntentContractV2, error) {
	if len(raw) > maxIntentBytes {
		return IntentContractV2{}, fmt.Errorf("action: intent exceeds %d bytes", maxIntentBytes)
	}
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return IntentContractV2{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	var wire intentContractWireV2
	if err := dec.Decode(&wire); err != nil {
		if strings.Contains(err.Error(), "unknown field") {
			return IntentContractV2{}, fmt.Errorf("%w: %v", ErrIntentUnknownField, err)
		}
		if strings.Contains(err.Error(), "budget") {
			return IntentContractV2{}, fmt.Errorf("%w: %v", ErrIntentBudget, err)
		}
		return IntentContractV2{}, fmt.Errorf("action: parse intent v2: %w", err)
	}
	if err := ensureJSONEOF(dec); err != nil {
		return IntentContractV2{}, err
	}
	from, err := time.Parse(time.RFC3339Nano, wire.ValidFrom)
	if err != nil {
		return IntentContractV2{}, fmt.Errorf("action: invalid valid_from: %w", err)
	}
	until, err := time.Parse(time.RFC3339Nano, wire.ExpiresAt)
	if err != nil {
		return IntentContractV2{}, fmt.Errorf("action: invalid expires_at: %w", err)
	}
	c := IntentContractV2{wire.IntentID, wire.SchemaVersion, wire.Version, wire.ProfileID, wire.OwnerPrincipalID, wire.Purpose, wire.Operations, wire.AllowedResources, wire.DeniedResources, wire.DataScope, wire.OutputDestinations, wire.EffectClasses, wire.Budget, from.UTC(), until.UTC(), wire.Approval, wire.MaxDelegationDepth}
	if err := c.Validate(); err != nil {
		return IntentContractV2{}, err
	}
	return c, nil
}

// Validate checks every bounded version 2 term.
func (c IntentContractV2) Validate() error {
	if c.SchemaVersion != 2 {
		return fmt.Errorf("%w: %d", ErrIntentSchemaVersion, c.SchemaVersion)
	}
	if c.IntentID == "" || c.ProfileID == "" || c.OwnerPrincipalID == "" || c.Version <= 0 {
		return errors.New("action: intent identity fields are required")
	}
	if len(c.Purpose) == 0 || len(c.Purpose) > maxIntentPurpose {
		return errors.New("action: intent purpose is empty or too long")
	}
	if c.ValidFrom.IsZero() || c.ExpiresAt.IsZero() || !c.ValidFrom.Before(c.ExpiresAt) {
		return errors.New("action: intent window must be non-empty")
	}
	if c.MaxDelegationDepth < 0 {
		return errors.New("action: negative delegation depth")
	}
	if c.Budget.Total != nil && *c.Budget.Total < 0 {
		return ErrIntentBudget
	}
	if len(c.Budget.PerOperation) > maxIntentSetEntries {
		return errors.New("action: too many per-operation budgets")
	}
	for _, v := range c.Budget.PerOperation {
		if v < 0 {
			return ErrIntentBudget
		}
	}
	if err := validateIntentSets(c); err != nil {
		return err
	}
	return nil
}

func validateIntentSets(c IntentContractV2) error {
	if len(c.Operations) == 0 {
		return errors.New("action: intent needs an operation")
	}
	seen := map[string]struct{}{}
	check := func(key string) error {
		if _, ok := seen[key]; ok {
			return fmt.Errorf("action: duplicate intent set member %q", key)
		}
		seen[key] = struct{}{}
		return nil
	}
	if len(c.Operations) > maxIntentSetEntries || len(c.AllowedResources) > maxIntentSetEntries || len(c.DeniedResources) > maxIntentSetEntries || len(c.DataScope) > maxIntentSetEntries || len(c.OutputDestinations) > maxIntentSetEntries || len(c.EffectClasses) > maxIntentSetEntries {
		return errors.New("action: intent set exceeds 256 entries")
	}
	for _, op := range c.Operations {
		if op.Namespace == "" || op.Name == "" || op.Version <= 0 {
			return errors.New("action: invalid operation reference")
		}
		if err := check("op:" + op.Namespace + "/" + op.Name + fmt.Sprintf("@%d", op.Version)); err != nil {
			return err
		}
	}
	for _, r := range append(append([]ResourceRef{}, c.AllowedResources...), c.DeniedResources...) {
		if r.Kind == "" || r.ID == "" {
			return errors.New("action: invalid resource reference")
		}
		if err := check("res:" + r.Kind + ":" + r.ID); err != nil {
			return err
		}
	}
	for _, v := range append(append([]string{}, c.DataScope...), c.OutputDestinations...) {
		if v == "" {
			return errors.New("action: empty set member")
		}
		if err := check("str:" + v); err != nil {
			return err
		}
	}
	for _, e := range c.EffectClasses {
		if !e.Known() {
			return fmt.Errorf("action: unknown effect class %q", e)
		}
		if err := check("effect:" + string(e)); err != nil {
			return err
		}
	}
	return nil
}

// CanonicalBytes returns the strict canonical terms form.
func (c IntentContractV2) CanonicalBytes() []byte {
	// The sorts below are IN PLACE, so they must run over copies of the slices.
	// `copyC := c` duplicates the STRUCT and shares every backing array, so the
	// canonicaliser used to reorder the CALLER's own Operations, DataScope and
	// the rest (the twenty-second pass, P2-6). slices.Clone preserves nil-ness,
	// which the wire encoding depends on: a nil slice marshals as null and an
	// empty one as [], and those are different canonical bytes and a different
	// digest.
	copyC := c
	copyC.Operations = slices.Clone(c.Operations)
	copyC.AllowedResources = slices.Clone(c.AllowedResources)
	copyC.DeniedResources = slices.Clone(c.DeniedResources)
	copyC.DataScope = slices.Clone(c.DataScope)
	copyC.OutputDestinations = slices.Clone(c.OutputDestinations)
	copyC.EffectClasses = slices.Clone(c.EffectClasses)
	sort.Slice(copyC.Operations, func(i, j int) bool {
		a, b := copyC.Operations[i], copyC.Operations[j]
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Version < b.Version
	})
	sort.Slice(copyC.AllowedResources, func(i, j int) bool {
		a, b := copyC.AllowedResources[i], copyC.AllowedResources[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.ID < b.ID
	})
	sort.Slice(copyC.DeniedResources, func(i, j int) bool {
		a, b := copyC.DeniedResources[i], copyC.DeniedResources[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.ID < b.ID
	})
	sort.Strings(copyC.DataScope)
	sort.Strings(copyC.OutputDestinations)
	sort.Slice(copyC.EffectClasses, func(i, j int) bool { return copyC.EffectClasses[i] < copyC.EffectClasses[j] })
	w := intentContractWireV2{copyC.IntentID, 2, copyC.Version, copyC.ProfileID, copyC.OwnerPrincipalID, copyC.Purpose, copyC.Operations, copyC.AllowedResources, copyC.DeniedResources, copyC.DataScope, copyC.OutputDestinations, copyC.EffectClasses, copyC.Budget, timeTerm(copyC.ValidFrom), timeTerm(copyC.ExpiresAt), copyC.Approval, copyC.MaxDelegationDepth}
	raw, err := json.Marshal(w)
	if err != nil {
		panic(err)
	}
	return raw
}

// Digest returns the SHA-256 digest of canonical version 2 terms.
func (c IntentContractV2) Digest() string {
	sum := sha256.Sum256(c.CanonicalBytes())
	return "sha256:" + hex.EncodeToString(sum[:])
}

// SignedIntentContractV2 carries exact terms and their ledger signature.
type SignedIntentContractV2 struct {
	Contract                        IntentContractV2
	Digest, SigningKeyID, Signature string
}

// SignIntentContractV2 signs exact terms in the intent domain.
func SignIntentContractV2(priv ed25519.PrivateKey, c IntentContractV2) SignedIntentContractV2 {
	return SignedIntentContractV2{c, c.Digest(), SigningKeyID(priv.Public().(ed25519.PublicKey)), hex.EncodeToString(ed25519.Sign(priv, intentSigningMessage(c.CanonicalBytes())))}
}

func intentSigningMessage(canonical []byte) []byte {
	sum := sha256.Sum256(append(append([]byte(intentContractDomain), 0), canonical...))
	return sum[:]
}
func eventSigningMessage(canonical []byte) []byte {
	sum := sha256.Sum256(append(append([]byte(intentEventDomain), 0), canonical...))
	return sum[:]
}

// VerifyIntentContractV2 verifies digest, key identity, and signature.
func VerifyIntentContractV2(pub ed25519.PublicKey, s SignedIntentContractV2) error {
	if err := s.Contract.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrIntentEvidenceCorrupt, err)
	}
	if s.Digest != s.Contract.Digest() || s.SigningKeyID != SigningKeyID(pub) {
		return ErrIntentEvidenceCorrupt
	}
	sig, err := hex.DecodeString(s.Signature)
	if err != nil || !ed25519.Verify(pub, intentSigningMessage(s.Contract.CanonicalBytes()), sig) {
		return ErrIntentEvidenceCorrupt
	}
	return nil
}

// IntentEventV1 is one signed lifecycle transition.
type IntentEventV1 struct {
	EventID, IntentID                string
	Version, Revision                int
	From, To                         LifecycleStatus
	ActorPrincipalID, EvidenceDigest string
	OccurredAt                       time.Time
	PreviousEventDigest              string
}

func (e IntentEventV1) canonicalBytes() []byte {
	raw, _ := json.Marshal(struct {
		EventID  string          `json:"event_id"`
		IntentID string          `json:"intent_id"`
		Version  int             `json:"version"`
		Revision int             `json:"revision"`
		From     LifecycleStatus `json:"from"`
		To       LifecycleStatus `json:"to"`
		Actor    string          `json:"actor_principal_id"`
		Evidence string          `json:"evidence_digest"`
		Occurred string          `json:"occurred_at"`
		Previous string          `json:"previous_event_digest"`
	}{e.EventID, e.IntentID, e.Version, e.Revision, e.From, e.To, e.ActorPrincipalID, e.EvidenceDigest, timeTerm(e.OccurredAt), e.PreviousEventDigest})
	return raw
}

// CanonicalBytes returns the strict signable lifecycle event form.
func (e IntentEventV1) CanonicalBytes() []byte { return e.canonicalBytes() }

// SignedIntentEventV1 carries an event, digest, key, and signature.
type SignedIntentEventV1 struct {
	Event                           IntentEventV1
	Digest, SigningKeyID, Signature string
}

// SignIntentEventV1 signs one event in the event domain.
func SignIntentEventV1(priv ed25519.PrivateKey, e IntentEventV1) SignedIntentEventV1 {
	b := e.canonicalBytes()
	sum := sha256.Sum256(b)
	return SignedIntentEventV1{e, "sha256:" + hex.EncodeToString(sum[:]), SigningKeyID(priv.Public().(ed25519.PublicKey)), hex.EncodeToString(ed25519.Sign(priv, eventSigningMessage(b)))}
}
func (s SignedIntentEventV1) signatureBytes() []byte { b, _ := hex.DecodeString(s.Signature); return b }

// VerifyIntentEventV1 verifies an event without requiring its key to remain active.
func VerifyIntentEventV1(pub ed25519.PublicKey, s SignedIntentEventV1) error {
	b := s.Event.canonicalBytes()
	sum := sha256.Sum256(b)
	if s.Digest != "sha256:"+hex.EncodeToString(sum[:]) || s.SigningKeyID != SigningKeyID(pub) {
		return ErrIntentEvidenceCorrupt
	}
	if !ed25519.Verify(pub, eventSigningMessage(b), s.signatureBytes()) {
		return ErrIntentEvidenceCorrupt
	}
	return nil
}

// IntentV2LifecycleTransition validates the declared version 2 lifecycle table.
func IntentV2LifecycleTransition(from, to LifecycleStatus) error {
	return LifecycleTransition(from, to)
}

// BindingStatus is the lifecycle of an execution binding.
type BindingStatus string

const ( // BindingActive selects an intent; BindingRevoked is terminal.
	BindingActive  BindingStatus = "ACTIVE"
	BindingRevoked BindingStatus = "REVOKED"
)

// ExecutionBinding binds an operator-selected execution context to exact terms.
type ExecutionBinding struct {
	BindingID, ActorPrincipalID, Channel, ConversationID, IntentID string
	IntentVersion                                                  int
	IntentDigest                                                   string
	GrantID                                                        string
	GrantVersion                                                   int
	GrantDigest                                                    string
	Revision                                                       int
	Status                                                         BindingStatus
}

// IntentProvenance classifies signed version 2 and legacy unsigned reads.
type IntentProvenance string

const (
	IntentProvenanceSignedV2 IntentProvenance = "signed_v2"
	// IntentProvenanceUnsignedV2 is a version 2 row that exists but carries no
	// signature yet: a DRAFT that was never activated. Calling it signed_v2
	// named evidence that does not exist.
	IntentProvenanceUnsignedV2     IntentProvenance = "unsigned_v2"
	IntentProvenanceLegacyUnsigned IntentProvenance = "legacy_unsigned"
)

// IntentAnyVersion is a version-tagged read result.
type IntentAnyVersion struct {
	Provenance IntentProvenance
	SignedV2   *SignedIntentContractV2
	Legacy     *IntentContract
}

// ImportLegacyIntentV2 translates v1 limits without signing historical bytes.
// The returned terms are a new DRAFT version that needs explicit activation.
func ImportLegacyIntentV2(legacy IntentContract, profileID string) (IntentContractV2, error) {
	if legacy.IntentID == "" || profileID == "" {
		return IntentContractV2{}, errors.New("action: legacy intent and profile are required")
	}
	// The legacy row's LIFECYCLE is read before its terms are translated. FR-INT-10
	// says a revoked root is never silently recreated, and the import path was
	// the hole: it never looked at Status, so a consent a human had REVOKED came
	// back as fresh signed terms with every effect class, ACTIVE, exit 0 (the
	// twenty-second pass, P1-2). Only an ACTIVE legacy contract may be carried
	// forward; DRAFT, EXPIRED and REVOKED are refused BY NAME.
	switch legacy.Status {
	case LifecycleActive:
	case LifecycleRevoked:
		return IntentContractV2{}, fmt.Errorf("%w: legacy intent %s", ErrIntentRevoked, legacy.IntentID)
	case LifecycleExpired:
		return IntentContractV2{}, fmt.Errorf("%w: legacy intent %s", ErrIntentExpired, legacy.IntentID)
	default:
		return IntentContractV2{}, fmt.Errorf("%w: legacy intent %s is %q", ErrIntentInactive, legacy.IntentID, legacy.Status)
	}
	ops := make([]OperationRef, 0, len(legacy.AllowedOperations))
	for _, name := range legacy.AllowedOperations {
		ops = append(ops, OperationRef{Namespace: "legacy", Name: name, Version: 1})
	}
	resources := make([]ResourceRef, 0, len(legacy.AllowedResources))
	for _, id := range legacy.AllowedResources {
		resources = append(resources, ResourceRef{Kind: "legacy", ID: id})
	}
	var total *int64
	if legacy.Budgets.MaxActions > 0 {
		v := int64(legacy.Budgets.MaxActions)
		total = &v
	}
	perOperation := make(map[string]int64, len(legacy.Budgets.MaxActionsPerOperation))
	for name, value := range legacy.Budgets.MaxActionsPerOperation {
		perOperation["legacy/"+name+"@1"] = int64(value)
	}
	until := legacy.ExpiresAt
	if until.IsZero() {
		until = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
	}
	c := IntentContractV2{
		IntentID: legacy.IntentID, SchemaVersion: 2, Version: legacy.Version + 1,
		ProfileID: profileID, OwnerPrincipalID: legacy.OwnerPrincipalID,
		Purpose: legacy.Purpose, Operations: ops, AllowedResources: resources,
		EffectClasses: []EffectClass{EffectPure, EffectReadExternal, EffectWriteReversible, EffectWriteCompensatable, EffectWriteIrreversible, EffectCritical},
		Budget:        IntentBudgetV2{Total: total, PerOperation: perOperation}, ValidFrom: legacy.ValidFrom.UTC(), ExpiresAt: until.UTC(),
	}
	if c.ValidFrom.IsZero() {
		c.ValidFrom = time.Unix(0, 0).UTC()
	}
	if err := c.Validate(); err != nil {
		return IntentContractV2{}, err
	}
	return c, nil
}

func rejectDuplicateJSONKeys(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	depth := 0
	var walk func() error
	walk = func() error {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		d, ok := tok.(json.Delim)
		if !ok {
			return nil
		}
		depth++
		if depth > maxIntentDepth {
			return errors.New("action: intent JSON exceeds depth 8")
		}
		defer func() { depth-- }()
		switch d {
		case '{':
			seen := map[string]struct{}{}
			for dec.More() {
				// The key token is JUDGED, never asserted. A malformed object
				// ("{...,}" or a non-string key) makes Token report an error or
				// return a delimiter; the earlier `kt.(string)` PANICKED on
				// both, which turned an operator's stray comma into a process
				// crash and, worse, turned a corrupt `canonical_terms` row into
				// a crash inside the path whose whole job is to fail closed
				// (the twenty-second pass, P1-1).
				kt, terr := dec.Token()
				if terr != nil {
					return fmt.Errorf("%w: %v", ErrIntentMalformed, terr)
				}
				k, ok := kt.(string)
				if !ok {
					return fmt.Errorf("%w: object key is %T, not a string", ErrIntentMalformed, kt)
				}
				if _, ok := seen[k]; ok {
					return fmt.Errorf("%w: %s", ErrIntentDuplicateField, k)
				}
				seen[k] = struct{}{}
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = dec.Token()
			return err
		case '[':
			for dec.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = dec.Token()
			return err
		}
		return nil
	}
	// ONE taxonomy out of the scanner: a repeated key keeps its own name, and
	// everything else the scanner cannot walk — a truncated object, a stray
	// comma, a non-string key, a document deeper than the cap — comes back as
	// ErrIntentMalformed. Before the twenty-second pass some of these paths
	// panicked and the rest leaked the encoder's raw error.
	if err := walk(); err != nil {
		if errors.Is(err, ErrIntentDuplicateField) || errors.Is(err, ErrIntentMalformed) {
			return err
		}
		return fmt.Errorf("%w: %v", ErrIntentMalformed, err)
	}
	return nil
}
func ensureJSONEOF(dec *json.Decoder) error {
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("action: trailing JSON value")
		}
		return err
	}
	return nil
}
