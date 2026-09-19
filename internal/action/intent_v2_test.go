// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package action

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func validIntentV2JSON() string {
	return `{"intent_id":"int_report","schema_version":2,"version":1,"profile_id":"profile_a","owner_principal_id":"principal_operator","purpose":"read reports","operations":[{"namespace":"tool","name":"read_file","version":1}],"allowed_resources":[{"kind":"cage","id":"reports"}],"denied_resources":[],"data_scope":["internal"],"output_destinations":["console"],"effect_classes":["read_external"],"budget":{"total":null,"per_operation":{"tool/read_file@1":0}},"valid_from":"2026-09-19T12:00:00Z","expires_at":"2026-09-20T12:00:00Z","approval":{"required":false},"max_delegation_depth":0}`
}

func TestIntentV2_RejectsAmbiguousSchema(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		raw  string
		want error
	}{
		{"duplicate", strings.Replace(validIntentV2JSON(), `"purpose":"read reports"`, `"purpose":"read reports","purpose":"write reports"`, 1), ErrIntentDuplicateField},
		{"negative", strings.Replace(validIntentV2JSON(), `"total":null`, `"total":-1`, 1), ErrIntentBudget},
		{"float", strings.Replace(validIntentV2JSON(), `"total":null`, `"total":1.5`, 1), ErrIntentBudget},
		{"future", strings.Replace(validIntentV2JSON(), `"schema_version":2`, `"schema_version":3`, 1), ErrIntentSchemaVersion},
		{"unknown", strings.Replace(validIntentV2JSON(), `"purpose":"read reports"`, `"purpose":"read reports","surprise":true`, 1), ErrIntentUnknownField},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseIntentContractV2([]byte(tc.raw)); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestIntentV2_OnlyDeclaredLifecycleEdges(t *testing.T) {
	t.Parallel()
	states := []LifecycleStatus{LifecycleDraft, LifecycleActive, LifecycleExpired, LifecycleRevoked}
	for _, from := range states {
		for _, to := range states {
			wantOK := from == LifecycleDraft && to == LifecycleActive ||
				from == LifecycleActive && (to == LifecycleExpired || to == LifecycleRevoked)
			err := IntentV2LifecycleTransition(from, to)
			if (err == nil) != wantOK {
				t.Fatalf("%s -> %s error = %v, wantOK %v", from, to, err, wantOK)
			}
		}
	}
}

func TestIntentV2_CanonicalDigestAndDomainSeparatedSignatures(t *testing.T) {
	t.Parallel()
	c, err := ParseIntentContractV2([]byte(validIntentV2JSON()))
	if err != nil {
		t.Fatal(err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signed := SignIntentContractV2(priv, c)
	if err := VerifyIntentContractV2(pub, signed); err != nil {
		t.Fatal(err)
	}
	if signed.Digest != c.Digest() || !strings.HasPrefix(signed.Digest, "sha256:") {
		t.Fatalf("digest = %q", signed.Digest)
	}
	tampered := signed
	tampered.Contract.Purpose = "write reports"
	if !errors.Is(VerifyIntentContractV2(pub, tampered), ErrIntentEvidenceCorrupt) {
		t.Fatal("tampered terms must fail")
	}
	event := IntentEventV1{EventID: "iev_1", IntentID: c.IntentID, Version: 1, Revision: 1, From: LifecycleDraft, To: LifecycleActive, ActorPrincipalID: "principal_operator", OccurredAt: time.Date(2026, 9, 19, 12, 1, 0, 0, time.UTC)}
	signedEvent := SignIntentEventV1(priv, event)
	if err := VerifyIntentEventV1(pub, signedEvent); err != nil {
		t.Fatal(err)
	}
	if ed25519.Verify(pub, intentSigningMessage(c.CanonicalBytes()), signedEvent.signatureBytes()) {
		t.Fatal("an event signature must not verify in the contract domain")
	}
}

func TestIntentV2_ValidationBoundaries(t *testing.T) {
	base, err := ParseIntentContractV2([]byte(validIntentV2JSON()))
	if err != nil {
		t.Fatal(err)
	}
	negative := int64(-1)
	cases := []struct {
		name   string
		mutate func(*IntentContractV2)
	}{
		{"schema", func(c *IntentContractV2) { c.SchemaVersion = 3 }}, {"identity", func(c *IntentContractV2) { c.IntentID = "" }},
		{"purpose-empty", func(c *IntentContractV2) { c.Purpose = "" }}, {"purpose-long", func(c *IntentContractV2) { c.Purpose = strings.Repeat("x", maxIntentPurpose+1) }},
		{"window-zero", func(c *IntentContractV2) { c.ValidFrom = time.Time{} }}, {"window-reversed", func(c *IntentContractV2) { c.ExpiresAt = c.ValidFrom }},
		{"depth", func(c *IntentContractV2) { c.MaxDelegationDepth = -1 }}, {"total", func(c *IntentContractV2) { c.Budget.Total = &negative }},
		{"per-op-negative", func(c *IntentContractV2) { c.Budget.PerOperation = map[string]int64{"x": -1} }}, {"no-ops", func(c *IntentContractV2) { c.Operations = nil }},
		{"bad-op", func(c *IntentContractV2) { c.Operations = []OperationRef{{Name: "x", Version: 1}} }}, {"duplicate-op", func(c *IntentContractV2) { c.Operations = append(c.Operations, c.Operations[0]) }},
		{"bad-resource", func(c *IntentContractV2) { c.AllowedResources = []ResourceRef{{Kind: "cage"}} }}, {"duplicate-resource", func(c *IntentContractV2) { c.DeniedResources = append(c.DeniedResources, c.AllowedResources[0]) }},
		{"empty-label", func(c *IntentContractV2) { c.DataScope = []string{""} }}, {"duplicate-label", func(c *IntentContractV2) { c.OutputDestinations = []string{"same"}; c.DataScope = []string{"same"} }},
		{"unknown-effect", func(c *IntentContractV2) { c.EffectClasses = []EffectClass{"future"} }}, {"duplicate-effect", func(c *IntentContractV2) { c.EffectClasses = []EffectClass{EffectPure, EffectPure} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			tc.mutate(&c)
			if c.Validate() == nil {
				t.Fatal("invalid contract accepted")
			}
		})
	}
	tooMany := base
	tooMany.DataScope = make([]string, maxIntentSetEntries+1)
	for i := range tooMany.DataScope {
		tooMany.DataScope[i] = fmt.Sprintf("d%d", i)
	}
	if tooMany.Validate() == nil {
		t.Fatal("oversized set accepted")
	}
	tooManyBudgets := base
	tooManyBudgets.Budget.PerOperation = map[string]int64{}
	for i := 0; i <= maxIntentSetEntries; i++ {
		tooManyBudgets.Budget.PerOperation[fmt.Sprint(i)] = 0
	}
	if tooManyBudgets.Validate() == nil {
		t.Fatal("oversized budget map accepted")
	}
}

func TestIntentV2_ParserAndSignatureFailures(t *testing.T) {
	if _, err := ParseIntentContractV2(bytes.Repeat([]byte(" "), maxIntentBytes+1)); err == nil {
		t.Fatal("oversized input accepted")
	}
	if _, err := ParseIntentContractV2([]byte(validIntentV2JSON() + ` {}`)); err == nil {
		t.Fatal("trailing input accepted")
	}
	badTime := strings.Replace(validIntentV2JSON(), "2026-09-19T12:00:00Z", "not-time", 1)
	if _, err := ParseIntentContractV2([]byte(badTime)); err == nil {
		t.Fatal("invalid time accepted")
	}
	c, _ := ParseIntentContractV2([]byte(validIntentV2JSON()))
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	signed := SignIntentContractV2(priv, c)
	bad := signed
	bad.SigningKeyID = "ed25519:wrong"
	if !errors.Is(VerifyIntentContractV2(pub, bad), ErrIntentEvidenceCorrupt) {
		t.Fatal("wrong key id accepted")
	}
	bad = signed
	bad.Signature = "zz"
	if !errors.Is(VerifyIntentContractV2(pub, bad), ErrIntentEvidenceCorrupt) {
		t.Fatal("bad signature accepted")
	}
	bad = signed
	bad.Contract.SchemaVersion = 3
	if !errors.Is(VerifyIntentContractV2(pub, bad), ErrIntentEvidenceCorrupt) {
		t.Fatal("invalid signed terms accepted")
	}
	e := IntentEventV1{EventID: "e", IntentID: c.IntentID, Version: 1, Revision: 1, To: LifecycleDraft, ActorPrincipalID: "p", OccurredAt: c.ValidFrom}
	se := SignIntentEventV1(priv, e)
	se.Digest = "sha256:bad"
	if !errors.Is(VerifyIntentEventV1(pub, se), ErrIntentEvidenceCorrupt) {
		t.Fatal("event digest accepted")
	}
	se = SignIntentEventV1(priv, e)
	se.Signature = "00"
	if !errors.Is(VerifyIntentEventV1(pub, se), ErrIntentEvidenceCorrupt) {
		t.Fatal("event signature accepted")
	}
}

func TestImportLegacyIntentV2_IsExplicitNewVersion(t *testing.T) {
	if _, err := ImportLegacyIntentV2(IntentContract{}, ""); err == nil {
		t.Fatal("empty import accepted")
	}
	// Status is now part of the contract the import judges, so the fixture
	// states it. Before the twenty-second pass this field was never read.
	legacy := IntentContract{IntentID: "int_old", SchemaVersion: 1, Version: 2, OwnerPrincipalID: "p", Purpose: "old", AllowedOperations: []string{"calc"}, AllowedResources: []string{"*"}, Budgets: Budgets{MaxActions: 4, MaxActionsPerOperation: map[string]int{"calc": 2}}, Status: LifecycleActive}
	got, err := ImportLegacyIntentV2(legacy, "profile")
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 3 || got.Budget.Total == nil || *got.Budget.Total != 4 || got.ExpiresAt.Year() != 9999 {
		t.Fatalf("import = %+v", got)
	}
	if len(got.EffectClasses) != 6 || got.Operations[0].Namespace != "legacy" {
		t.Fatalf("import dimensions = %+v", got)
	}
}

func TestIntentV2_CanonicalizesEverySet(t *testing.T) {
	c, _ := ParseIntentContractV2([]byte(validIntentV2JSON()))
	c.Operations = append(c.Operations, OperationRef{Namespace: "a", Name: "z", Version: 2}, OperationRef{Namespace: "tool", Name: "a", Version: 1})
	c.AllowedResources = append(c.AllowedResources, ResourceRef{Kind: "a", ID: "z"}, ResourceRef{Kind: "cage", ID: "a"})
	c.DeniedResources = []ResourceRef{{Kind: "z", ID: "a"}, {Kind: "a", ID: "z"}}
	c.DataScope = []string{"z", "a"}
	c.OutputDestinations = []string{"z", "a"}
	c.EffectClasses = []EffectClass{EffectWriteReversible, EffectPure}
	first := c.CanonicalBytes()
	c.Operations[0], c.Operations[2] = c.Operations[2], c.Operations[0]
	c.AllowedResources[0], c.AllowedResources[2] = c.AllowedResources[2], c.AllowedResources[0]
	c.DeniedResources[0], c.DeniedResources[1] = c.DeniedResources[1], c.DeniedResources[0]
	if !bytes.Equal(first, c.CanonicalBytes()) {
		t.Fatal("set order changed canonical bytes")
	}
	deep := []byte(`{"a":[[[[[[[[[0]]]]]]]]]}`)
	if _, err := ParseIntentContractV2(deep); err == nil {
		t.Fatal("deep JSON accepted")
	}
}

// TestIntentV2_MalformedBytesAreRefusedNotPanics is the mould the strict
// scanner never had. The twenty-second pass reproduced, with the compiled
// binary, that `korvun intent create-v2` over a file with one stray comma
// PANICKED — and, worse, that a corrupt `canonical_terms` row panicked inside
// the resolve path whose whole job is to fail closed. A panic is not a
// refusal: every row below must come back with a NAMED error.
func TestIntentV2_MalformedBytesAreRefusedNotPanics(t *testing.T) {
	for _, raw := range []string{
		`{"intent_id":"int_x","schema_version":2,}`,
		`{1:2}`,
		`{"a":{2:3}}`,
		`{"intent_id":"x",}`,
		`{`,
		`{"a":`,
	} {
		t.Run(raw, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("ParseIntentContractV2 panicked over %q: %v", raw, r)
				}
			}()
			_, err := ParseIntentContractV2([]byte(raw))
			if err == nil {
				t.Fatalf("ParseIntentContractV2(%q) = nil error", raw)
			}
			if !errors.Is(err, ErrIntentMalformed) {
				t.Fatalf("error = %v, want ErrIntentMalformed", err)
			}
		})
	}
}

// TestImportLegacyIntentV2_RefusesConsentThatIsNotActive is the mould FR-INT-10
// never had. The twenty-second pass adopted a REVOKED legacy root through the
// ordinary CLI and got back signed, ACTIVE version-2 terms carrying EVERY
// effect class, exit 0 — a human's withdrawal of consent manufactured back into
// consent. Each refusal below demands ITS name, because "some error" would hide
// which lifecycle the row was in.
func TestImportLegacyIntentV2_RefusesConsentThatIsNotActive(t *testing.T) {
	base := IntentContract{IntentID: "int_old", SchemaVersion: 1, Version: 2, OwnerPrincipalID: "p", Purpose: "old", AllowedOperations: []string{"calc"}, AllowedResources: []string{"*"}}
	for _, row := range []struct {
		status LifecycleStatus
		want   error
	}{
		{status: LifecycleRevoked, want: ErrIntentRevoked},
		{status: LifecycleExpired, want: ErrIntentExpired},
		{status: LifecycleDraft, want: ErrIntentInactive},
		{status: "", want: ErrIntentInactive},
	} {
		t.Run(string(row.status), func(t *testing.T) {
			legacy := base
			legacy.Status = row.status
			got, err := ImportLegacyIntentV2(legacy, "profile")
			if !errors.Is(err, row.want) {
				t.Fatalf("error = %v, want %v", err, row.want)
			}
			if got.IntentID != "" {
				t.Fatalf("refused import still returned terms: %+v", got)
			}
		})
	}
}
