// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package action

import (
	"errors"
	"path/filepath"
	"testing"
)

// TestAuthority_ResourceMatcherBindsActualArguments (AS-AUTH-16) hands REAL tool
// arguments to the production analyzer and then to the production scope check:
// what the grant is compared with is what the arguments actually reach, derived
// by the store-side analyzer — never an id the caller declares. Each refusal
// demands ErrResourceOutOfScope by name.
//
// This mould's rows are few. The RULE that makes the URL matcher safe against
// what these rows cannot enumerate — refuse every non-canonical encoding — is
// pinned by TestAuthority_URLMatcherRefusesNonCanonicalEncoding, and "the
// hostile URL and path corpus for AS-AUTH-16" is FILED by name.
//
// NOT covered, and said so: symbolic links. Path inclusion is LEXICAL
// (filepath.Abs and Rel); a link inside the granted directory that points
// outside it is not resolved here. The tool's own cage is what stands there.
//
// Evidence level: unit, in-process; no store, no network.
// Probing mutation executed: compare the raw path argument without
// canonicalizing it — the canonical-id row reddens.
func TestAuthority_ResourceMatcherBindsActualArguments(t *testing.T) {
	registry := NewOperationUseRegistry()
	if err := RegisterBuiltInOperationUse(registry); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ operation, args, wantID string }{
		{"read_file", hostAbs("/cage/a/report.txt"), filepath.Clean(hostAbs("/cage/a/report.txt"))},
		{"read_file", hostAbs("/cage/a/../b/secret.txt"), filepath.Clean(hostAbs("/cage/b/secret.txt"))},
		{"http_fetch", "https://a.example.evil/x", "https://a.example.evil/x"},
		{"webhook_call", `https://b.example/hook {"note":"x"}`, "https://b.example/hook"},
	}
	for _, tc := range cases {
		use, err := registry.Analyze(tc.operation, tc.args)
		if err != nil {
			t.Fatalf("%s: %v", tc.operation, err)
		}
		if len(use.Resources) != 1 || use.Resources[0].ID != tc.wantID {
			t.Fatalf("%s resources = %#v, want canonical %q", tc.operation, use.Resources, tc.wantID)
		}
	}
	grant := authorityTestChild()
	use, err := registry.Analyze("read_file", hostAbs("/cage/a/../b/secret.txt"))
	if err != nil {
		t.Fatal(err)
	}
	grant.AllowedResources = []ResourceRef{{Kind: "path", ID: hostAbs("/cage/a/")}}
	if err := ValidateAuthorityUse(grant, use, ResourceMatchers{"path": PathResourceIncludes}); !errors.Is(err, ErrResourceOutOfScope) {
		t.Fatalf("path escape under authority for cage A: error = %v, want %v", err, ErrResourceOutOfScope)
	}
	if URLResourceIncludes("https://a.example/safe", "https://a.example/safe/../secret") {
		t.Fatal("non-canonical URL traversal widened the parent path")
	}
	urlUse, err := registry.Analyze("http_fetch", "https://a.example.evil/safe")
	if err != nil {
		t.Fatal(err)
	}
	grant.AllowedResources = []ResourceRef{{Kind: "url", ID: "https://a.example/safe"}}
	if err := ValidateAuthorityUse(grant, urlUse, ResourceMatchers{"url": URLResourceIncludes}); !errors.Is(err, ErrResourceOutOfScope) {
		t.Fatalf("sibling host under authority for a.example: error = %v, want %v", err, ErrResourceOutOfScope)
	}
}
