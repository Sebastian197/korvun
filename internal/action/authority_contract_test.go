// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package action

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAuthorityGrantV2_CanonicalRoundTripAndSignatures(t *testing.T) {
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	grant := authorityTestChild()
	grant.Channels = []string{"telegram", "console"}
	grant.AllowedData = []string{"public", "supplier_invoice"}
	grant.AllowedResources = []ResourceRef{
		{Kind: "url", ID: "https://pay.example/api/invoices"},
		{Kind: "path", ID: "/cage/invoices"},
	}
	raw := grant.CanonicalBytes()
	parsed, err := ParseAuthorityGrantV2(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(parsed.CanonicalBytes(), raw) || parsed.Digest() != grant.Digest() {
		t.Fatal("canonical grant did not round trip")
	}
	signed := SignAuthorityGrantV2(private, grant)
	if err := VerifyAuthorityGrantV2(pub, signed); err != nil {
		t.Fatal(err)
	}
	badDigest := signed
	badDigest.Digest = "sha256:bad"
	if !errors.Is(VerifyAuthorityGrantV2(pub, badDigest), ErrAuthorityEvidenceCorrupt) {
		t.Fatal("changed digest verified")
	}
	badKey := signed
	badKey.SigningKeyID = "kid_bad"
	if !errors.Is(VerifyAuthorityGrantV2(pub, badKey), ErrAuthorityEvidenceCorrupt) {
		t.Fatal("changed key id verified")
	}
	badSignature := signed
	badSignature.Signature = "not-hex"
	if !errors.Is(VerifyAuthorityGrantV2(pub, badSignature), ErrAuthorityEvidenceCorrupt) {
		t.Fatal("malformed signature verified")
	}
	badGrant := signed
	badGrant.Grant.Version = 0
	if !errors.Is(VerifyAuthorityGrantV2(pub, badGrant), ErrAuthorityEvidenceCorrupt) {
		t.Fatal("malformed signed grant verified")
	}
}

func TestAuthorityGrantV2_RejectsMalformedTermsAndWire(t *testing.T) {
	base := authorityTestChild()
	negative := int64(-1)
	cases := []struct {
		name   string
		mutate func(*AuthorityGrantV2)
	}{
		{"identity", func(g *AuthorityGrantV2) { g.GrantID = "" }},
		{"parent-without-version", func(g *AuthorityGrantV2) { g.ParentGrantVersion = 0 }},
		{"version-without-parent", func(g *AuthorityGrantV2) { g.ParentGrantID = "" }},
		{"negative-depth", func(g *AuthorityGrantV2) { g.DelegationDepthRemaining = -1 }},
		{"excess-depth", func(g *AuthorityGrantV2) { g.DelegationDepthRemaining = maxAuthorityDepth + 1 }},
		{"zero-window", func(g *AuthorityGrantV2) { g.ValidFrom = time.Time{} }},
		{"reversed-window", func(g *AuthorityGrantV2) { g.ExpiresAt = g.ValidFrom }},
		{"no-operations", func(g *AuthorityGrantV2) { g.Operations = nil }},
		{"negative-total", func(g *AuthorityGrantV2) { g.Budget.Total = &negative }},
		{"empty-per-operation", func(g *AuthorityGrantV2) { g.Budget.PerOperation = map[string]int64{"": 1} }},
		{"negative-per-operation", func(g *AuthorityGrantV2) { g.Budget.PerOperation = map[string]int64{"tool/x@1": -1} }},
		{"unknown-effect", func(g *AuthorityGrantV2) { g.EffectCeiling = EffectClass("future") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			grant := base
			tc.mutate(&grant)
			if !errors.Is(grant.Validate(), ErrAuthorityMalformed) {
				t.Fatal("malformed grant was accepted")
			}
		})
	}

	var wire map[string]any
	if err := json.Unmarshal(base.CanonicalBytes(), &wire); err != nil {
		t.Fatal(err)
	}
	badWires := [][]byte{
		[]byte(`{"unknown":true}`),
		append(append([]byte(nil), base.CanonicalBytes()...), []byte(` {}`)...),
	}
	for _, field := range []string{"valid_from", "expires_at"} {
		clone := make(map[string]any, len(wire))
		for key, value := range wire {
			clone[key] = value
		}
		clone[field] = "not-a-time"
		raw, err := json.Marshal(clone)
		if err != nil {
			t.Fatal(err)
		}
		badWires = append(badWires, raw)
	}
	for _, raw := range badWires {
		if _, err := ParseAuthorityGrantV2(raw); !errors.Is(err, ErrAuthorityMalformed) {
			t.Fatalf("malformed wire %s: error = %v, want %v", raw, err, ErrAuthorityMalformed)
		}
	}
}

func TestConfigAuthorityClause_ValidationAndCanonicalIdentity(t *testing.T) {
	clause := ConfigAuthorityClause{
		SchemaVersion: 1, ProfileID: "profile_a", BrainPrincipal: "principal_agent",
		ToolName: "read_file", Channels: []string{"telegram", "console"}, CageDigest: "sha256:cage",
	}
	clause.ClauseID = "cfg_" + strings.TrimPrefix(clause.Digest(), "sha256:")
	if err := clause.Validate(); err != nil {
		t.Fatal(err)
	}
	reordered := clause
	reordered.Channels = []string{"console", "telegram"}
	if reordered.Digest() != clause.Digest() {
		t.Fatal("channel ordering changed clause identity")
	}
	cases := []func(*ConfigAuthorityClause){
		func(c *ConfigAuthorityClause) { c.ProfileID = "" },
		func(c *ConfigAuthorityClause) { c.Channels = nil },
		func(c *ConfigAuthorityClause) { c.Channels = []string{""} },
		func(c *ConfigAuthorityClause) { c.Channels = []string{"console", "console"} },
		func(c *ConfigAuthorityClause) { c.Channels = []string{"*", "console"} },
		func(c *ConfigAuthorityClause) { c.ClauseID = "cfg_wrong" },
	}
	for i, mutate := range cases {
		bad := clause
		mutate(&bad)
		if !errors.Is(bad.Validate(), ErrAuthorityMalformed) {
			t.Fatalf("case %d accepted", i)
		}
	}
}

func TestAuthorityOperationUse_ParsersAndScopeFailures(t *testing.T) {
	registry := NewOperationUseRegistry()
	if err := RegisterBuiltInOperationUse(registry); err != nil {
		t.Fatal(err)
	}
	if err := RegisterBuiltInOperationUse(registry); !errors.Is(err, ErrAuthorityUseUnresolved) {
		t.Fatalf("duplicate built-ins error = %v", err)
	}
	if err := (*OperationUseRegistry)(nil).Register("x", func(string) (OperationUse, error) { return OperationUse{}, nil }); !errors.Is(err, ErrAuthorityUseUnresolved) {
		t.Fatal("nil registry accepted registration")
	}
	if err := registry.Register("", func(string) (OperationUse, error) { return OperationUse{}, nil }); !errors.Is(err, ErrAuthorityUseUnresolved) {
		t.Fatal("empty operation accepted")
	}
	if err := registry.Register("x", nil); !errors.Is(err, ErrAuthorityUseUnresolved) {
		t.Fatal("nil analyzer accepted")
	}
	if _, err := (*OperationUseRegistry)(nil).Analyze("x", `{}`); !errors.Is(err, ErrAuthorityUseUnresolved) {
		t.Fatal("nil registry analyzed")
	}
	for _, tc := range []struct {
		operation string
		args      string
	}{
		{"missing", `{}`},
		{"read_file", ""},
		{"read_file", "   "},
		// A relative path: the tool joins it to a jail root this layer does
		// not know, so it is unresolved rather than judged from another base.
		{"read_file", "relative/file.txt"},
		{"http_fetch", "relative"},
		{"http_fetch", ""},
		{"webhook_call", `https://user@example.com/x {}`},
		// webhook_call is the URL, one space, then a JSON body.
		{"webhook_call", "https://example.com/x"},
		{"webhook_call", "https://example.com/x not-json"},
		// A URL path that is not already clean is refused, not cleaned: the
		// tool sends it as written.
		{"http_fetch", "https://example.com/a//b"},
		{"http_fetch", "https://example.com/a/./b"},
	} {
		if _, err := registry.Analyze(tc.operation, tc.args); !errors.Is(err, ErrAuthorityUseUnresolved) {
			t.Fatalf("%s(%s) error = %v", tc.operation, tc.args, err)
		}
	}
	pathUse, err := registry.Analyze("read_file", "  /cage/a/../b  ")
	if err != nil || len(pathUse.Resources) != 1 || pathUse.Resources[0].ID != filepath.Clean("/cage/b") {
		t.Fatalf("path use = %#v, %v", pathUse, err)
	}
	urlUse, err := registry.Analyze("webhook_call", `HTTPS://EXAMPLE.COM/b/#fragment {"note":"x"}`)
	if err != nil || urlUse.Resources[0].ID != "https://example.com/b" ||
		!bytes.Equal([]byte(strings.Join(urlUse.Data, ",")), []byte("payload")) ||
		urlUse.Destinations[0] != "example.com" {
		t.Fatalf("URL use = %#v, %v", urlUse, err)
	}
	if !URLResourceIncludes("https://example.com/api", "https://EXAMPLE.com/api/v1") ||
		URLResourceIncludes("https://example.com/api", "http://example.com/api") ||
		URLResourceIncludes("https://example.com/api", "https://example.com/apix") ||
		URLResourceIncludes("%", "https://example.com/api") ||
		URLResourceIncludes("https://example.com/api", "%") {
		t.Fatal("URL containment boundary failed")
	}
	if !PathResourceIncludes("/cage", "/cage/a") || PathResourceIncludes("/cage", "/cage-other/a") {
		t.Fatal("path containment boundary failed")
	}

	grant := authorityTestChild()
	grant.AllowedResources = []ResourceRef{{Kind: "url", ID: "https://example.com/api"}}
	grant.DeniedResources = []ResourceRef{{Kind: "url", ID: "https://example.com/api/private"}}
	grant.AllowedData = []string{"payload"}
	grant.DeniedData = []string{"secret"}
	grant.OutputDestinations = []string{"example.com"}
	matchers := ResourceMatchers{"url": URLResourceIncludes}
	checks := []struct {
		use  OperationUse
		want error
	}{
		{OperationUse{Resources: []ResourceRef{{Kind: "url", ID: "https://outside.example/x"}}}, ErrResourceOutOfScope},
		{OperationUse{Resources: []ResourceRef{{Kind: "url", ID: "https://example.com/api/private/x"}}}, ErrResourceOutOfScope},
		{OperationUse{Data: []string{"other"}}, ErrDataOutOfScope},
		{OperationUse{Data: []string{"secret"}}, ErrDataOutOfScope},
		{OperationUse{Destinations: []string{"outside.example"}}, ErrDestinationOutOfScope},
	}
	for _, tc := range checks {
		if err := ValidateAuthorityUse(grant, tc.use, matchers); !errors.Is(err, tc.want) {
			t.Fatalf("scope error = %v, want %v", err, tc.want)
		}
	}
	validUse := OperationUse{
		Resources: []ResourceRef{{Kind: "url", ID: "https://example.com/api/public"}},
		Data:      []string{"payload"}, Destinations: []string{"example.com"},
	}
	if err := ValidateAuthorityUse(grant, validUse, matchers); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorizationSnapshotV1_ClosedWireAndSignature(t *testing.T) {
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	remaining := int64(2)
	snapshot := AuthorizationSnapshotV1{
		Kind: AuthorizationSnapshotPending, ActionID: "act3_1", ApprovalID: "apr3_1",
		RequesterPrincipalID: "principal_requester", ActorPrincipalID: "principal_agent",
		IdentityEvidenceDigest: "sha256:identity", IntentID: "int_a", IntentVersion: 1,
		IntentDigest: "sha256:intent", IntentPurpose: "Send an approved update",
		PrincipalChain: []string{"principal_operator", "principal_agent"},
		BudgetKind:     AuthorizationBudgetFinite, BudgetRemaining: &remaining,
		RecordedAt: time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC),
	}
	raw := snapshot.CanonicalBytes()
	parsed, err := ParseAuthorizationSnapshotV1(raw)
	if err != nil || !bytes.Equal(parsed.CanonicalBytes(), raw) {
		t.Fatalf("snapshot round trip = %#v, %v", parsed, err)
	}
	signature := SignAuthorizationSnapshotV1(private, snapshot)
	if err := VerifyAuthorizationSnapshotV1(pub, snapshot, signature); err != nil {
		t.Fatal(err)
	}
	if got, err := VerifyAuthorizationSnapshotBytesV1(pub, raw, signature); err != nil || got.ActionID != snapshot.ActionID {
		t.Fatalf("verified bytes = %#v, %v", got, err)
	}
	tampered := append([]byte(nil), raw...)
	tampered = bytes.Replace(tampered, []byte("approved"), []byte("altered!"), 1)
	if _, err := VerifyAuthorizationSnapshotBytesV1(pub, tampered, signature); !errors.Is(err, ErrAuthorityEvidenceCorrupt) {
		t.Fatalf("tampered snapshot error = %v", err)
	}
	badSignature := signature
	badSignature.Signature = "00"
	if _, err := VerifyAuthorizationSnapshotBytesV1(pub, raw, badSignature); !errors.Is(err, ErrAuthorityEvidenceCorrupt) {
		t.Fatalf("tampered signature error = %v", err)
	}

	negative := int64(-1)
	cases := []func(*AuthorizationSnapshotV1){
		func(s *AuthorizationSnapshotV1) { s.ActionID = "" },
		func(s *AuthorizationSnapshotV1) { s.PrincipalChain = []string{""} },
		func(s *AuthorizationSnapshotV1) { s.ApprovalID = "" },
		func(s *AuthorizationSnapshotV1) { s.Kind = AuthorizationSnapshotKind("future") },
		func(s *AuthorizationSnapshotV1) { s.BudgetRemaining = nil },
		func(s *AuthorizationSnapshotV1) { s.BudgetRemaining = &negative },
		func(s *AuthorizationSnapshotV1) { s.BudgetKind = AuthorizationBudgetUnlimited },
		func(s *AuthorizationSnapshotV1) { s.BudgetKind = AuthorizationBudgetKind("future") },
	}
	for i, mutate := range cases {
		bad := snapshot
		mutate(&bad)
		if !errors.Is(bad.Validate(), ErrAuthorizationSnapshotMalformed) {
			t.Fatalf("invalid snapshot %d accepted", i)
		}
	}
	start := snapshot
	start.Kind, start.ApprovalID = AuthorizationSnapshotStart, ""
	start.BudgetKind, start.BudgetRemaining = AuthorizationBudgetUnlimited, nil
	if err := start.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, malformed := range [][]byte{
		[]byte(`{"schema_version":2}`),
		append(append([]byte(nil), raw...), []byte(` {}`)...),
		bytes.Replace(raw, []byte(snapshot.RecordedAt.Format(time.RFC3339Nano)), []byte("not-a-time"), 1),
		bytes.Replace(raw, []byte(`"schema_version":1`), []byte(`"schema_version":1, "extra":true`), 1),
	} {
		if _, err := ParseAuthorizationSnapshotV1(malformed); !errors.Is(err, ErrAuthorizationSnapshotMalformed) {
			t.Fatalf("malformed snapshot %s: error = %v, want %v", malformed, err, ErrAuthorizationSnapshotMalformed)
		}
	}
}

// TestAuthorityGrantV2_AbsentAndEmptyShareOneDigest pins a defect the delivery
// session found: the "canonical" encoding had TWO forms for the same terms. A
// nil collection encoded as `null` and an empty one as `[]` or `{}`, and the
// store's doors turn an absent per-operation map into an empty one before
// persisting — so a root issued with a nil map was stored and signed under a
// digest its authenticated act had never covered.
//
// Evidence level: unit, in-process. The store-side consequence — the persisted
// digest equals the digest the actor authorized — is pinned beside it by
// TestAuthority_IssuedDigestIsTheAuthorizedDigest in internal/action/sqlite.
// Probing mutation executed: drop the absent-to-empty step for the
// per-operation map — red with two digests for one set of terms.
func TestAuthorityGrantV2_AbsentAndEmptyShareOneDigest(t *testing.T) {
	absent := authorityTestParent()
	absent.AllowedResources, absent.DeniedResources = nil, nil
	absent.AllowedData, absent.DeniedData, absent.OutputDestinations = nil, nil, nil
	absent.Budget.PerOperation = nil

	empty := absent
	empty.AllowedResources, empty.DeniedResources = []ResourceRef{}, []ResourceRef{}
	empty.AllowedData, empty.DeniedData, empty.OutputDestinations = []string{}, []string{}, []string{}
	empty.Budget.PerOperation = map[string]int64{}

	if string(absent.CanonicalBytes()) != string(empty.CanonicalBytes()) {
		t.Errorf("one set of terms, two encodings:\nabsent %s\nempty  %s", absent.CanonicalBytes(), empty.CanonicalBytes())
	}
	if absent.Digest() != empty.Digest() {
		t.Errorf("one set of terms, two digests: %s and %s", absent.Digest(), empty.Digest())
	}
	parsed, err := ParseAuthorityGrantV2(absent.CanonicalBytes())
	if err != nil {
		t.Fatalf("the canonical bytes do not parse back: %v", err)
	}
	if parsed.Digest() != absent.Digest() {
		t.Errorf("digest after a round trip = %s, want %s", parsed.Digest(), absent.Digest())
	}
}
