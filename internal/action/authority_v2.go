// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Authority v2 is the strict, signed delegation contract used by
// StartAuthorization. It is deliberately separate from the readable v1 grant:
// old rows remain history until an explicit administrative import creates v2
// evidence.
package action

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

const (
	authorityGrantDomain = "korvun.authority-grant.v2"
	maxAuthorityDepth    = 16
)

var (
	// ErrAuthorityMalformed reports invalid or unbounded v2 terms.
	ErrAuthorityMalformed = errors.New("action: authority grant malformed")
	// ErrAuthorityEvidenceCorrupt reports a digest or signature mismatch.
	ErrAuthorityEvidenceCorrupt = errors.New("action: authority evidence corrupt")
	// ErrAuthorityUseUnresolved reports an operation whose actual use cannot be
	// derived from its canonical arguments.
	ErrAuthorityUseUnresolved = errors.New("action: authority use unresolved")
	// ErrResourceOutOfScope reports an actual resource outside the grant.
	ErrResourceOutOfScope = errors.New("action: resource out of authority scope")
	// ErrDataOutOfScope reports an actual data class outside the grant.
	ErrDataOutOfScope = errors.New("action: data outside authority scope")
	// ErrDestinationOutOfScope reports an actual output target outside the grant.
	ErrDestinationOutOfScope = errors.New("action: destination outside authority scope")
)

// AuthorityGrantV2 carries every enforceable delegation dimension. Runtime
// Status is excluded from its digest and proven by signed lifecycle events.
type AuthorityGrantV2 struct {
	GrantID                  string
	SchemaVersion            int
	Version                  int
	ProfileID                string
	IntentID                 string
	IntentVersion            int
	IntentDigest             string
	IssuerPrincipalID        string
	SubjectPrincipalID       string
	ParentGrantID            string
	ParentGrantVersion       int
	Operations               []OperationRef
	Channels                 []string
	AllowedResources         []ResourceRef
	DeniedResources          []ResourceRef
	AllowedData              []string
	DeniedData               []string
	OutputDestinations       []string
	EffectClasses            []EffectClass
	EffectCeiling            EffectClass
	Budget                   IntentBudgetV2
	ValidFrom                time.Time
	ExpiresAt                time.Time
	DelegationDepthRemaining int
	Approval                 ApprovalRequirementV2
	Status                   LifecycleStatus
}

// ConfigAuthorityClause is one exact tool-to-channel relation derived from
// SelectTools input. Its identifier is the full digest of these terms; budget
// accounting uses the separate stable brain-and-tool identity.
type ConfigAuthorityClause struct {
	ClauseID       string
	SchemaVersion  int
	ProfileID      string
	BrainPrincipal string
	ToolName       string
	Channels       []string
	CageDigest     string
}

type configAuthorityClauseWire struct {
	SchemaVersion  int      `json:"schema_version"`
	ProfileID      string   `json:"profile_id"`
	BrainPrincipal string   `json:"brain_principal_id"`
	ToolName       string   `json:"tool_name"`
	Channels       []string `json:"channels"`
	CageDigest     string   `json:"cage_digest"`
}

// CanonicalBytes returns the exact immutable config-clause terms.
func (c ConfigAuthorityClause) CanonicalBytes() []byte {
	channels := slices.Clone(c.Channels)
	sort.Strings(channels)
	raw, _ := json.Marshal(configAuthorityClauseWire{
		SchemaVersion: c.SchemaVersion, ProfileID: c.ProfileID,
		BrainPrincipal: c.BrainPrincipal, ToolName: c.ToolName,
		Channels: channels, CageDigest: c.CageDigest,
	})
	return raw
}

// Digest is the full terms digest used as the clause identity.
func (c ConfigAuthorityClause) Digest() string { return digestAuthorityBytes(c.CanonicalBytes()) }

// Validate checks the bounded exact-clause shape.
func (c ConfigAuthorityClause) Validate() error {
	if c.SchemaVersion != 1 || c.ProfileID == "" || c.BrainPrincipal == "" ||
		c.ToolName == "" || c.CageDigest == "" || len(c.Channels) == 0 ||
		len(c.Channels) > maxIntentSetEntries {
		return ErrAuthorityMalformed
	}
	seen := make(map[string]bool, len(c.Channels))
	for _, channel := range c.Channels {
		if channel == "" || seen[channel] || channel == "*" && len(c.Channels) != 1 {
			return ErrAuthorityMalformed
		}
		seen[channel] = true
	}
	wantID := "cfg_" + strings.TrimPrefix(c.Digest(), "sha256:")
	if c.ClauseID != wantID {
		return ErrAuthorityMalformed
	}
	return nil
}

type authorityGrantWireV2 struct {
	GrantID          string                `json:"grant_id"`
	SchemaVersion    int                   `json:"schema_version"`
	Version          int                   `json:"version"`
	ProfileID        string                `json:"profile_id"`
	IntentID         string                `json:"intent_id"`
	IntentVersion    int                   `json:"intent_version"`
	IntentDigest     string                `json:"intent_digest"`
	Issuer           string                `json:"issuer_principal_id"`
	Subject          string                `json:"subject_principal_id"`
	ParentID         string                `json:"parent_grant_id"`
	ParentVersion    int                   `json:"parent_grant_version"`
	Operations       []OperationRef        `json:"operations"`
	Channels         []string              `json:"channels"`
	AllowedResources []ResourceRef         `json:"allowed_resources"`
	DeniedResources  []ResourceRef         `json:"denied_resources"`
	AllowedData      []string              `json:"allowed_data"`
	DeniedData       []string              `json:"denied_data"`
	Destinations     []string              `json:"output_destinations"`
	EffectClasses    []EffectClass         `json:"effect_classes"`
	EffectCeiling    EffectClass           `json:"effect_ceiling"`
	Budget           IntentBudgetV2        `json:"budget"`
	ValidFrom        string                `json:"valid_from"`
	ExpiresAt        string                `json:"expires_at"`
	Depth            int                   `json:"delegation_depth_remaining"`
	Approval         ApprovalRequirementV2 `json:"approval"`
}

// Validate checks structural bounds without consulting a parent or store.
func (g AuthorityGrantV2) Validate() error {
	if g.SchemaVersion != 2 || g.Version <= 0 || g.GrantID == "" || g.ProfileID == "" ||
		g.IntentID == "" || g.IntentVersion <= 0 || g.IntentDigest == "" ||
		g.IssuerPrincipalID == "" || g.SubjectPrincipalID == "" {
		return ErrAuthorityMalformed
	}
	if g.ParentGrantID == "" && g.ParentGrantVersion != 0 || g.ParentGrantID != "" && g.ParentGrantVersion <= 0 {
		return fmt.Errorf("%w: parent version", ErrAuthorityMalformed)
	}
	if g.DelegationDepthRemaining < 0 || g.DelegationDepthRemaining > maxAuthorityDepth {
		return fmt.Errorf("%w: delegation depth", ErrAuthorityMalformed)
	}
	if g.ValidFrom.IsZero() || g.ExpiresAt.IsZero() || !g.ValidFrom.Before(g.ExpiresAt) {
		return fmt.Errorf("%w: validity window", ErrAuthorityMalformed)
	}
	if len(g.Operations) == 0 || len(g.Channels) == 0 || len(g.EffectClasses) == 0 {
		return fmt.Errorf("%w: required authority set", ErrAuthorityMalformed)
	}
	if len(g.Operations) > maxIntentSetEntries || len(g.Channels) > maxIntentSetEntries ||
		len(g.AllowedResources) > maxIntentSetEntries ||
		len(g.DeniedResources) > maxIntentSetEntries ||
		len(g.AllowedData) > maxIntentSetEntries || len(g.DeniedData) > maxIntentSetEntries ||
		len(g.OutputDestinations) > maxIntentSetEntries ||
		len(g.EffectClasses) > maxIntentSetEntries ||
		len(g.Budget.PerOperation) > maxIntentSetEntries {
		return fmt.Errorf("%w: authority set exceeds %d entries", ErrAuthorityMalformed, maxIntentSetEntries)
	}
	if g.Budget.Total != nil && *g.Budget.Total < 0 {
		return fmt.Errorf("%w: total budget", ErrAuthorityMalformed)
	}
	seen := make(map[string]bool)
	unique := func(kind, value string) error {
		if value == "" || seen[kind+"\x00"+value] {
			return fmt.Errorf("%w: duplicate or empty %s", ErrAuthorityMalformed, kind)
		}
		seen[kind+"\x00"+value] = true
		return nil
	}
	for _, operation := range g.Operations {
		if operation.Namespace == "" || operation.Name == "" || operation.Version <= 0 {
			return fmt.Errorf("%w: operation", ErrAuthorityMalformed)
		}
		if err := unique("operation", operationKey(operation)); err != nil {
			return err
		}
	}
	for _, channel := range g.Channels {
		if err := unique("channel", channel); err != nil {
			return err
		}
	}
	for _, resource := range append(slices.Clone(g.AllowedResources), g.DeniedResources...) {
		if resource.Kind == "" || resource.ID == "" {
			return fmt.Errorf("%w: resource", ErrAuthorityMalformed)
		}
		if err := unique("resource", resource.Kind+"\x00"+resource.ID); err != nil {
			return err
		}
	}
	for kind, values := range map[string][]string{
		"allowed_data": g.AllowedData, "denied_data": g.DeniedData,
		"destination": g.OutputDestinations,
	} {
		for _, value := range values {
			if err := unique(kind, value); err != nil {
				return err
			}
		}
	}
	for _, effect := range g.EffectClasses {
		if !effect.Known() {
			return fmt.Errorf("%w: effect class", ErrAuthorityMalformed)
		}
		if err := unique("effect", string(effect)); err != nil {
			return err
		}
	}
	for key, value := range g.Budget.PerOperation {
		if key == "" || value < 0 {
			return fmt.Errorf("%w: per-operation budget", ErrAuthorityMalformed)
		}
	}
	if g.EffectCeiling != "" && !g.EffectCeiling.Known() {
		return fmt.Errorf("%w: effect ceiling", ErrAuthorityMalformed)
	}
	return nil
}

// CanonicalBytes returns the deterministic terms encoding.
func (g AuthorityGrantV2) CanonicalBytes() []byte {
	c := g
	c.Operations = slices.Clone(g.Operations)
	c.Channels = sortedStrings(g.Channels)
	c.AllowedResources = sortedResources(g.AllowedResources)
	c.DeniedResources = sortedResources(g.DeniedResources)
	c.AllowedData = sortedStrings(g.AllowedData)
	c.DeniedData = sortedStrings(g.DeniedData)
	c.OutputDestinations = sortedStrings(g.OutputDestinations)
	c.EffectClasses = slices.Clone(g.EffectClasses)
	sort.Slice(c.Operations, func(i, j int) bool { return operationKey(c.Operations[i]) < operationKey(c.Operations[j]) })
	sort.Slice(c.EffectClasses, func(i, j int) bool { return c.EffectClasses[i] < c.EffectClasses[j] })
	// ONE encoding for "none". A nil collection and an empty one are the same
	// terms, and encoding/json spells them `null` and `[]`/`{}` — two byte
	// strings, two digests. The doors normalize an absent per-operation map to
	// an empty one before persisting, so a root issued with a nil map was stored
	// and SIGNED under a digest its authenticated act had never covered, and a
	// binding made with the digest of what the operator wrote failed every
	// start as "evidence corrupt" with nothing corrupt. Canonical means one
	// form: absent is written as empty, everywhere.
	c.Operations, c.Channels = emptyIfNil(c.Operations), emptyIfNil(c.Channels)
	c.AllowedResources, c.DeniedResources = emptyIfNil(c.AllowedResources), emptyIfNil(c.DeniedResources)
	c.AllowedData, c.DeniedData = emptyIfNil(c.AllowedData), emptyIfNil(c.DeniedData)
	c.OutputDestinations, c.EffectClasses = emptyIfNil(c.OutputDestinations), emptyIfNil(c.EffectClasses)
	if c.Budget.PerOperation == nil {
		c.Budget.PerOperation = map[string]int64{}
	}
	raw, err := json.Marshal(authorityGrantWireV2{
		c.GrantID, 2, c.Version, c.ProfileID, c.IntentID, c.IntentVersion, c.IntentDigest,
		c.IssuerPrincipalID, c.SubjectPrincipalID, c.ParentGrantID, c.ParentGrantVersion,
		c.Operations, c.Channels, c.AllowedResources, c.DeniedResources, c.AllowedData,
		c.DeniedData, c.OutputDestinations, c.EffectClasses, c.EffectCeiling, c.Budget,
		timeTerm(c.ValidFrom), timeTerm(c.ExpiresAt), c.DelegationDepthRemaining, c.Approval,
	})
	if err != nil {
		panic(err)
	}
	return raw
}

// ParseAuthorityGrantV2 parses the exact canonical wire form.
func ParseAuthorityGrantV2(raw []byte) (AuthorityGrantV2, error) {
	if len(raw) > maxIntentBytes {
		return AuthorityGrantV2{}, fmt.Errorf("%w: grant exceeds %d bytes", ErrAuthorityMalformed, maxIntentBytes)
	}
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return AuthorityGrantV2{}, fmt.Errorf("%w: %v", ErrAuthorityMalformed, err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var w authorityGrantWireV2
	if err := dec.Decode(&w); err != nil {
		return AuthorityGrantV2{}, fmt.Errorf("%w: %v", ErrAuthorityMalformed, err)
	}
	if err := ensureJSONEOF(dec); err != nil {
		// One class for every malformed wire: a second JSON value after the
		// grant is a malformed grant, not an error with no name.
		return AuthorityGrantV2{}, fmt.Errorf("%w: %w", ErrAuthorityMalformed, err)
	}
	from, err := time.Parse(time.RFC3339Nano, w.ValidFrom)
	if err != nil {
		return AuthorityGrantV2{}, ErrAuthorityMalformed
	}
	until, err := time.Parse(time.RFC3339Nano, w.ExpiresAt)
	if err != nil {
		return AuthorityGrantV2{}, ErrAuthorityMalformed
	}
	g := AuthorityGrantV2{
		w.GrantID, w.SchemaVersion, w.Version, w.ProfileID, w.IntentID, w.IntentVersion,
		w.IntentDigest, w.Issuer, w.Subject, w.ParentID, w.ParentVersion, w.Operations,
		w.Channels, w.AllowedResources, w.DeniedResources, w.AllowedData, w.DeniedData,
		w.Destinations, w.EffectClasses, w.EffectCeiling, w.Budget, from.UTC(), until.UTC(),
		w.Depth, w.Approval, "",
	}
	if err := g.Validate(); err != nil {
		return AuthorityGrantV2{}, err
	}
	return g, nil
}

// Digest returns the terms digest.
func (g AuthorityGrantV2) Digest() string { return digestAuthorityBytes(g.CanonicalBytes()) }

// emptyIfNil gives an absent collection the encoding of an empty one.
func emptyIfNil[T any](in []T) []T {
	if in == nil {
		return []T{}
	}
	return in
}

func sortedStrings(in []string) []string { out := slices.Clone(in); sort.Strings(out); return out }
func sortedResources(in []ResourceRef) []ResourceRef {
	out := slices.Clone(in)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].ID < out[j].ID
	})
	return out
}
func operationKey(op OperationRef) string {
	return fmt.Sprintf("%s/%s@%d", op.Namespace, op.Name, op.Version)
}

// AuthoritySignature is the shared signature envelope for authority evidence.
type AuthoritySignature struct{ Digest, SigningKeyID, Signature string }

// SignAuthorityBytes signs exact canonical bytes under a domain separator.
func SignAuthorityBytes(priv ed25519.PrivateKey, domain string, canonical []byte) AuthoritySignature {
	return AuthoritySignature{digestAuthorityBytes(canonical), SigningKeyID(priv.Public().(ed25519.PublicKey)), hex.EncodeToString(ed25519.Sign(priv, authoritySigningMessage(domain, canonical)))}
}

// VerifyAuthorityBytes verifies a generic authority signature.
func VerifyAuthorityBytes(pub ed25519.PublicKey, domain string, canonical []byte, signature AuthoritySignature) error {
	if signature.Digest != digestAuthorityBytes(canonical) || signature.SigningKeyID != SigningKeyID(pub) {
		return ErrAuthorityEvidenceCorrupt
	}
	sig, err := hex.DecodeString(signature.Signature)
	if err != nil || !ed25519.Verify(pub, authoritySigningMessage(domain, canonical), sig) {
		return ErrAuthorityEvidenceCorrupt
	}
	return nil
}
func digestAuthorityBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}
func authoritySigningMessage(domain string, raw []byte) []byte {
	sum := sha256.Sum256(append(append([]byte(domain), 0), raw...))
	return sum[:]
}

// SignedAuthorityGrantV2 carries exact grant terms and signature.
type SignedAuthorityGrantV2 struct {
	Grant AuthorityGrantV2
	AuthoritySignature
}

// SignAuthorityGrantV2 signs one grant.
func SignAuthorityGrantV2(priv ed25519.PrivateKey, grant AuthorityGrantV2) SignedAuthorityGrantV2 {
	return SignedAuthorityGrantV2{grant, SignAuthorityBytes(priv, authorityGrantDomain, grant.CanonicalBytes())}
}

// VerifyAuthorityGrantV2 verifies one signed grant.
func VerifyAuthorityGrantV2(pub ed25519.PublicKey, signed SignedAuthorityGrantV2) error {
	if err := signed.Grant.Validate(); err != nil {
		return ErrAuthorityEvidenceCorrupt
	}
	return VerifyAuthorityBytes(pub, authorityGrantDomain, signed.Grant.CanonicalBytes(), signed.AuthoritySignature)
}

// AttenuationError identifies the first widened dimension.
type AttenuationError struct{ Dimension, Detail string }

func (e *AttenuationError) Error() string {
	return fmt.Sprintf("%v: %s (%s)", ErrAttenuationViolated, e.Dimension, e.Detail)
}
func (e *AttenuationError) Unwrap() error            { return ErrAttenuationViolated }
func authorityWidens(dimension, detail string) error { return &AttenuationError{dimension, detail} }

// AuthorityBudgetRemaining is the transaction-local unspent parent balance.
type AuthorityBudgetRemaining struct {
	Total        *int64
	PerOperation map[string]int64
}

// ResourceMatcher proves that a child descriptor is contained by a parent.
type ResourceMatcher func(parent, child string) bool

// ResourceMatchers is the closed registry used for one validation.
type ResourceMatchers map[string]ResourceMatcher

// NormalizeAuthorityDelegation validates every attenuation dimension and
// materializes inherited per-operation maxima.
func NormalizeAuthorityDelegation(parent, child AuthorityGrantV2, intent IntentContractV2, remaining AuthorityBudgetRemaining, matchers ResourceMatchers) (AuthorityGrantV2, error) {
	if err := parent.Validate(); err != nil {
		return AuthorityGrantV2{}, err
	}
	if err := child.Validate(); err != nil {
		return AuthorityGrantV2{}, err
	}
	checks := []struct {
		bad               bool
		dimension, detail string
	}{
		{child.ProfileID != parent.ProfileID || child.ProfileID != intent.ProfileID, "profile", "profile differs"},
		{child.IntentID != parent.IntentID || child.IntentID != intent.IntentID, "intent", "intent differs"},
		{child.IntentVersion != parent.IntentVersion || child.IntentVersion != intent.Version, "intent_version", "intent version differs"},
		{child.IntentDigest != parent.IntentDigest || child.IntentDigest != intent.Digest(), "intent_digest", "intent digest differs"},
		{child.ParentGrantID != parent.GrantID || child.ParentGrantVersion != parent.Version, "parent", "parent link differs"},
		{child.IssuerPrincipalID != parent.SubjectPrincipalID, "issuer", "issuer is not parent subject"},
	}
	for _, check := range checks {
		if check.bad {
			return AuthorityGrantV2{}, authorityWidens(check.dimension, check.detail)
		}
	}
	if !operationSubset(child.Operations, parent.Operations) || !operationSubset(child.Operations, intent.Operations) {
		return AuthorityGrantV2{}, authorityWidens("operations", "operation set widens")
	}
	if !stringSubset(child.Channels, parent.Channels) {
		return AuthorityGrantV2{}, authorityWidens("channels", "channel set widens")
	}
	if !resourcesIncluded(child.AllowedResources, parent.AllowedResources, matchers) || !resourcesIncluded(child.AllowedResources, intent.AllowedResources, matchers) {
		return AuthorityGrantV2{}, authorityWidens("allowed_resources", "allowed resource widens")
	}
	if !resourceSuperset(child.DeniedResources, append(slices.Clone(parent.DeniedResources), intent.DeniedResources...)) {
		return AuthorityGrantV2{}, authorityWidens("denied_resources", "inherited denial disappeared")
	}
	if !stringSubset(child.AllowedData, parent.AllowedData) || !stringSubset(child.AllowedData, intent.DataScope) {
		return AuthorityGrantV2{}, authorityWidens("allowed_data", "data set widens")
	}
	if !stringSuperset(child.DeniedData, parent.DeniedData) {
		return AuthorityGrantV2{}, authorityWidens("denied_data", "inherited denial disappeared")
	}
	if !stringSubset(child.OutputDestinations, parent.OutputDestinations) || !stringSubset(child.OutputDestinations, intent.OutputDestinations) {
		return AuthorityGrantV2{}, authorityWidens("destinations", "destination set widens")
	}
	if !effectSubset(child.EffectClasses, parent.EffectClasses) || !effectSubset(child.EffectClasses, intent.EffectClasses) {
		return AuthorityGrantV2{}, authorityWidens("effect_classes", "effect set widens")
	}
	if parent.EffectCeiling != "" && (child.EffectCeiling == "" || child.EffectCeiling.Rank() > parent.EffectCeiling.Rank()) {
		return AuthorityGrantV2{}, authorityWidens("effect_ceiling", "effect ceiling widens")
	}
	if remaining.Total != nil && (child.Budget.Total == nil || *child.Budget.Total > *remaining.Total) {
		return AuthorityGrantV2{}, authorityWidens("budget_total_remaining", "total exceeds remaining")
	}
	if child.Budget.PerOperation == nil {
		child.Budget.PerOperation = map[string]int64{}
	} else {
		child.Budget.PerOperation = cloneInt64Map(child.Budget.PerOperation)
	}
	for operation, maximum := range remaining.PerOperation {
		childMaximum, exists := child.Budget.PerOperation[operation]
		if !exists {
			child.Budget.PerOperation[operation] = maximum
			continue
		}
		if childMaximum > maximum {
			return AuthorityGrantV2{}, authorityWidens("budget_operation_remaining", "operation exceeds remaining")
		}
	}
	if child.ValidFrom.Before(parent.ValidFrom) || child.ValidFrom.Before(intent.ValidFrom) {
		return AuthorityGrantV2{}, authorityWidens("valid_from", "window starts too early")
	}
	if child.ExpiresAt.After(parent.ExpiresAt) || child.ExpiresAt.After(intent.ExpiresAt) {
		return AuthorityGrantV2{}, authorityWidens("expires_at", "window ends too late")
	}
	if parent.DelegationDepthRemaining <= 0 || child.DelegationDepthRemaining < 0 || child.DelegationDepthRemaining >= parent.DelegationDepthRemaining || child.DelegationDepthRemaining > intent.MaxDelegationDepth {
		return AuthorityGrantV2{}, authorityWidens("depth", "depth does not decrease")
	}
	if (parent.Approval.Required || intent.Approval.Required) && !child.Approval.Required {
		return AuthorityGrantV2{}, authorityWidens("approval", "required approval disappeared")
	}
	return child, nil
}

func cloneInt64Map(in map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
func operationSubset(child, parent []OperationRef) bool {
	for _, c := range child {
		found := false
		for _, p := range parent {
			if c == p {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func stringSubset(child, parent []string) bool {
	for _, c := range child {
		if !slices.Contains(parent, c) && !slices.Contains(parent, "*") {
			return false
		}
	}
	return true
}
func stringSuperset(child, parent []string) bool { return stringSubset(parent, child) }
func effectSubset(child, parent []EffectClass) bool {
	for _, c := range child {
		if !slices.Contains(parent, c) {
			return false
		}
	}
	return true
}
func resourceSuperset(child, parent []ResourceRef) bool {
	for _, p := range parent {
		if !slices.Contains(child, p) {
			return false
		}
	}
	return true
}
func resourcesIncluded(children, parents []ResourceRef, matchers ResourceMatchers) bool {
	for _, child := range children {
		covered := false
		for _, parent := range parents {
			if parent.Kind != child.Kind {
				continue
			}
			matcher := matchers[child.Kind]
			if matcher != nil && matcher(parent.ID, child.ID) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

// OperationUse is derived from actual canonical arguments.
type OperationUse struct {
	Resources    []ResourceRef
	Data         []string
	Destinations []string
}
type OperationUseAnalyzer func(string) (OperationUse, error)
type OperationUseRegistry struct {
	analyzers map[string]OperationUseAnalyzer
}

func NewOperationUseRegistry() *OperationUseRegistry {
	return &OperationUseRegistry{analyzers: map[string]OperationUseAnalyzer{}}
}
func (r *OperationUseRegistry) Register(operation string, analyzer OperationUseAnalyzer) error {
	if r == nil || operation == "" || analyzer == nil || r.analyzers[operation] != nil {
		return ErrAuthorityUseUnresolved
	}
	r.analyzers[operation] = analyzer
	return nil
}

// ErrOperationUseNoAnalyzer is Analyze's answer for an operation NOBODY
// registered an analyzer for. It wraps ErrAuthorityUseUnresolved, and it is a
// class of its own on purpose: FR-AUTH-03 lets an operation WITHOUT an analyzer
// start when no term restricts it, and lets nothing else through. An analyzer
// that IS registered and cannot resolve the arguments it was given is the other
// case — arguments that cannot be proved inside the grant — and fails closed
// whatever the terms say. Folding the two into one error let a registered
// analyzer's failure pass as «no analyzer» and start with nothing bound (the
// adversary's pass over piece 3 phase 3, F2).
var ErrOperationUseNoAnalyzer = fmt.Errorf("%w: no analyzer registered for the operation", ErrAuthorityUseUnresolved)

func (r *OperationUseRegistry) Analyze(operation, args string) (OperationUse, error) {
	if r == nil || r.analyzers[operation] == nil {
		return OperationUse{}, ErrOperationUseNoAnalyzer
	}
	return r.analyzers[operation](args)
}

// RegisterBuiltInOperationUse registers the analyzers of the three effectful
// built-in tools. Each one speaks the argument grammar ITS TOOL speaks, because
// the string it is handed is the very string the executor hands the tool:
//
//   - read_file: the whole trimmed string is the path (internal/tool/readfile.go);
//   - http_fetch: the whole trimmed string is the URL (internal/tool/httpfetch.go);
//   - webhook_call: the URL, one space, then the JSON body
//     (internal/tool/webhookcall.go).
//
// An earlier shape parsed a JSON object — {"path":…}, {"url":…,"body":…} — that
// no shipped tool accepts, so against the real tools a scoped grant refused
// every start and an unscoped one bound nothing (F2 of the same pass). The
// agreement with the real tools is held by a mould in package tool that runs
// both over the same strings.
//
// What an analyzer binds is the REQUESTED target, judged lexically. Where the
// tool goes from there is its cage's business and not this layer's: read_file
// resolves symbolic links under its jail, http_fetch follows redirects under
// its host allow-list. Both are FILED by name in the phase's canto.
//
// A RELATIVE read_file path is refused as unresolved: the tool joins it to a
// jail root this layer does not know, and a path judged from one base and read
// from another is exactly the ambiguity FR-AUTH-03 fails closed on.
func RegisterBuiltInOperationUse(r *OperationUseRegistry) error {
	pathAnalyzer := func(raw string) (OperationUse, error) {
		p := strings.TrimSpace(raw)
		if p == "" || !filepath.IsAbs(p) {
			return OperationUse{}, ErrAuthorityUseUnresolved
		}
		return OperationUse{Resources: []ResourceRef{{Kind: "path", ID: filepath.Clean(p)}}}, nil
	}
	fetchAnalyzer := func(raw string) (OperationUse, error) {
		u, err := canonicalAuthorityURL(strings.TrimSpace(raw))
		if err != nil {
			return OperationUse{}, ErrAuthorityUseUnresolved
		}
		return OperationUse{Resources: []ResourceRef{{Kind: "url", ID: u}},
			Destinations: []string{mustAuthorityHost(u)}}, nil
	}
	callAnalyzer := func(raw string) (OperationUse, error) {
		parts := strings.SplitN(strings.TrimSpace(raw), " ", 2)
		if len(parts) < 2 || !json.Valid([]byte(strings.TrimSpace(parts[1]))) {
			return OperationUse{}, ErrAuthorityUseUnresolved
		}
		u, err := canonicalAuthorityURL(parts[0])
		if err != nil {
			return OperationUse{}, ErrAuthorityUseUnresolved
		}
		// A webhook call always carries a body out: the payload tag is not
		// conditional on what the body says.
		return OperationUse{Resources: []ResourceRef{{Kind: "url", ID: u}},
			Data: []string{"payload"}, Destinations: []string{mustAuthorityHost(u)}}, nil
	}
	if err := r.Register("read_file", pathAnalyzer); err != nil {
		return err
	}
	if err := r.Register("http_fetch", fetchAnalyzer); err != nil {
		return err
	}
	return r.Register("webhook_call", callAnalyzer)
}

// canonicalAuthorityURL returns the one form of raw the matcher judges, or an
// error when raw has no such form.
//
// The path judged is the DECODED one, cleaned with path.Clean — the slash
// semantics of a URL, identical on every platform; filepath's follow the host
// OS and have no business here. And a URL whose ENCODED path is not the
// canonical encoding of its decoded path is REFUSED, not normalized: url.Parse
// records exactly that condition by keeping RawPath, so a non-empty RawPath is
// the refusal. Different servers decode such a path differently, and a matcher
// cannot know which one will answer; FR-AUTH-03 makes ambiguous normalization
// fail closed.
//
// The earlier shape cleaned the path while it was still percent-encoded, so an
// encoded segment was never recognized for what it decodes into and a URL could
// be judged inside a resource it left; it also wrote the escaped text back into
// Path, which double-encoded it and made the function non-idempotent.
func canonicalAuthorityURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil {
		return "", errors.New("invalid URL")
	}
	if u.RawPath != "" {
		return "", errors.New("URL path is not canonically encoded")
	}
	// A path that is not ALREADY clean is refused too, not cleaned. The tool
	// sends the URL as it was written — it resolves nothing — so a cleaned copy
	// would be judged while another path travelled, and which of the two the
	// server honours is the server's choice. A trailing slash is the one
	// difference allowed: it does not move a path in or out of any resource.
	cleaned := path.Clean("/" + strings.TrimPrefix(u.Path, "/"))
	if written := "/" + strings.TrimPrefix(u.Path, "/"); written != cleaned && written != cleaned+"/" {
		return "", errors.New("URL path is not in its clean form")
	}
	u.Fragment = ""
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Path = cleaned
	return u.String(), nil
}
func mustAuthorityHost(raw string) string {
	u, _ := url.Parse(raw)
	return strings.ToLower(u.Hostname())
}

// PathResourceIncludes compares canonical path segments, not prefixes.
func PathResourceIncludes(parent, child string) bool {
	p, err := filepath.Abs(parent)
	if err != nil {
		return false
	}
	c, err := filepath.Abs(child)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(p), filepath.Clean(c))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// URLResourceIncludes compares scheme, host, and path boundary.
func URLResourceIncludes(parent, child string) bool {
	canonicalParent, err := canonicalAuthorityURL(parent)
	if err != nil {
		return false
	}
	canonicalChild, err := canonicalAuthorityURL(child)
	if err != nil {
		return false
	}
	p, _ := url.Parse(canonicalParent)
	c, _ := url.Parse(canonicalChild)
	if !strings.EqualFold(p.Scheme, c.Scheme) || !strings.EqualFold(p.Host, c.Host) {
		return false
	}
	// A resource scoped BY its query includes only that query. Ignoring the
	// query made «…/x?tenant=1» silently every tenant. A resource with no query
	// says nothing about it, and includes any.
	if p.RawQuery != "" && p.RawQuery != c.RawQuery {
		return false
	}
	base := strings.TrimSuffix(p.Path, "/")
	return c.Path == base || strings.HasPrefix(c.Path, base+"/")
}

// ValidateAuthorityUse checks derived runtime use against grant terms.
func ValidateAuthorityUse(grant AuthorityGrantV2, use OperationUse, matchers ResourceMatchers) error {
	if !resourcesIncluded(use.Resources, grant.AllowedResources, matchers) {
		return ErrResourceOutOfScope
	}
	for _, r := range use.Resources {
		for _, d := range grant.DeniedResources {
			if r.Kind == d.Kind && matchers[r.Kind] != nil && matchers[r.Kind](d.ID, r.ID) {
				return ErrResourceOutOfScope
			}
		}
	}
	if !stringSubset(use.Data, grant.AllowedData) {
		return ErrDataOutOfScope
	}
	for _, v := range use.Data {
		if slices.Contains(grant.DeniedData, v) {
			return ErrDataOutOfScope
		}
	}
	if !stringSubset(use.Destinations, grant.OutputDestinations) {
		return ErrDestinationOutOfScope
	}
	return nil
}
