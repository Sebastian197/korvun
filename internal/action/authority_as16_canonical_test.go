// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package action

import (
	"errors"
	"testing"
)

// TestAuthority_URLMatcherRefusesNonCanonicalEncoding pins the cure of a defect
// the delivery session found inside AS-AUTH-16's own territory. The URL matcher
// used to normalize the path while it was still PERCENT-ENCODED, so an encoded
// segment was never recognized for what a server would decode it into, and a
// URL could be judged inside a granted resource it actually left. Before the
// cure, five of eight deceptive URLs probed passed as included.
//
// The cure is a RULE, not a list: the matcher judges the DECODED path, cleaned
// with path.Clean, and it REFUSES any URL whose encoded form is not the
// canonical encoding of its decoded path — ambiguous normalization fails closed
// (FR-AUTH-03). Because it is a rule, this mould needs no hostile input to
// prove it: a percent-encoded ORDINARY LETTER is non-canonical and harmless,
// and it must be refused exactly as anything else non-canonical would be.
//
// FILED by name, and deliberately not written here: "the hostile URL and path
// corpus for AS-AUTH-16" — rows of deceptive arguments judged by hand.
//
// Evidence level: unit, in-process — the production analyzer and the production
// scope check; no store, no network.
// Probing mutations executed, each alone: drop the non-canonical-encoding
// refusal — the encoded-letter rows then pass as included, red; replace the
// segment-boundary comparison with a bare string prefix — the sibling-prefix
// row then passes as included, red.
func TestAuthority_URLMatcherRefusesNonCanonicalEncoding(t *testing.T) {
	const granted = "https://a.example/safe"
	// "%6f" is the letter "o": the decoded path is the ordinary /safe/report,
	// and the ENCODED form is not the canonical encoding of it.
	const nonCanonical = "https://a.example/safe/rep%6frt"
	const canonical = "https://a.example/safe/report"
	const siblingPrefix = "https://a.example/safe-and-more/report"

	registry := NewOperationUseRegistry()
	if err := RegisterBuiltInOperationUse(registry); err != nil {
		t.Fatal(err)
	}
	grant := authorityTestChild()
	grant.AllowedResources = []ResourceRef{{Kind: "url", ID: granted}}
	grant.DeniedResources = nil
	grant.OutputDestinations = []string{"a.example"}
	matchers := ResourceMatchers{"url": URLResourceIncludes}

	t.Run("the analyzer refuses to derive a use from a non-canonical encoding", func(t *testing.T) {
		_, err := registry.Analyze("http_fetch", nonCanonical)
		if !errors.Is(err, ErrAuthorityUseUnresolved) {
			t.Errorf("error = %v, want %v", err, ErrAuthorityUseUnresolved)
		}
	})

	t.Run("the matcher never includes a non-canonical encoding", func(t *testing.T) {
		if URLResourceIncludes(granted, nonCanonical) {
			t.Error("a non-canonical encoding was judged included")
		}
		use := OperationUse{Resources: []ResourceRef{{Kind: "url", ID: nonCanonical}}}
		if err := ValidateAuthorityUse(grant, use, matchers); !errors.Is(err, ErrResourceOutOfScope) {
			t.Errorf("error = %v, want %v", err, ErrResourceOutOfScope)
		}
	})

	t.Run("a non-canonical GRANT includes nothing either", func(t *testing.T) {
		if URLResourceIncludes("https://a.example/s%61fe", canonical) {
			t.Error("a non-canonically encoded grant was accepted as a parent")
		}
	})

	t.Run("the canonical form of the same URL is inside the grant", func(t *testing.T) {
		use, err := registry.Analyze("http_fetch", canonical)
		if err != nil {
			t.Fatalf("the canonical URL was refused: %v", err)
		}
		if len(use.Resources) != 1 || use.Resources[0].ID != canonical {
			t.Errorf("derived resources = %#v, want exactly %q", use.Resources, canonical)
		}
		if err := ValidateAuthorityUse(grant, use, matchers); err != nil {
			t.Errorf("error = %v, want the canonical URL inside the grant", err)
		}
	})

	t.Run("canonicalization is idempotent", func(t *testing.T) {
		once, err := canonicalAuthorityURL(canonical)
		if err != nil {
			t.Fatal(err)
		}
		twice, err := canonicalAuthorityURL(once)
		if err != nil || twice != once {
			t.Errorf("canonical of canonical = %q, %v; want %q unchanged", twice, err, once)
		}
	})

	t.Run("a sibling that merely shares the prefix is outside", func(t *testing.T) {
		if URLResourceIncludes(granted, siblingPrefix) {
			t.Error("a path sharing only a textual prefix was judged included")
		}
		use := OperationUse{Resources: []ResourceRef{{Kind: "url", ID: siblingPrefix}}}
		if err := ValidateAuthorityUse(grant, use, matchers); !errors.Is(err, ErrResourceOutOfScope) {
			t.Errorf("error = %v, want %v", err, ErrResourceOutOfScope)
		}
	})
}

// TestAuthority_URLMatcherJudgesWhatTravels pins the two rules that became
// REACHABLE the moment the analyzers started speaking the real tools' grammar
// (the adversary's pass over piece 3 phase 3, F2, its bypass classes ii and
// iii). Both are about the same thing: the matcher must judge the URL that
// actually travels, not a tidier relative of it.
//
//   - A path that is not already clean is REFUSED, not cleaned. http_fetch and
//     webhook_call send the URL as it was written; a cleaned copy would be
//     judged while another path travelled. The rows use harmless forms — a
//     doubled slash, a single-dot segment — because the rule is about the form,
//     and the hostile corpus stays FILED by name. A trailing slash is the one
//     difference allowed.
//   - A resource scoped BY its query includes only that query; a resource with
//     no query says nothing about it.
//
// Evidence level: unit, in-process; no store, no network.
// Probing mutations executed, each alone: (1) clean the path instead of
// refusing it — red on both unclean rows with «error = <nil>»; (2) drop the
// query comparison — red with «a resource scoped to one query included
// another».
func TestAuthority_URLMatcherJudgesWhatTravels(t *testing.T) {
	registry := NewOperationUseRegistry()
	if err := RegisterBuiltInOperationUse(registry); err != nil {
		t.Fatal(err)
	}
	for _, unclean := range []string{"https://a.example/safe//report", "https://a.example/safe/./report"} {
		if _, err := registry.Analyze("http_fetch", unclean); !errors.Is(err, ErrAuthorityUseUnresolved) {
			t.Errorf("%s: error = %v, want %v", unclean, err, ErrAuthorityUseUnresolved)
		}
		if URLResourceIncludes("https://a.example/safe", unclean) {
			t.Errorf("%s was judged included: a path nobody cleaned is what travels", unclean)
		}
	}
	use, err := registry.Analyze("http_fetch", "https://a.example/safe/")
	if err != nil || len(use.Resources) != 1 || use.Resources[0].ID != "https://a.example/safe" {
		t.Errorf("a trailing slash: use = %#v, %v; want it resolved to the same resource", use, err)
	}

	const scoped = "https://api.example/orders?tenant=1"
	if URLResourceIncludes(scoped, "https://api.example/orders?tenant=2") {
		t.Error("a resource scoped to one query included another")
	}
	if URLResourceIncludes(scoped, "https://api.example/orders") {
		t.Error("a resource scoped to one query included the same path with no query")
	}
	if !URLResourceIncludes(scoped, "https://api.example/orders?tenant=1") {
		t.Error("a resource scoped to one query refused that very query")
	}
	if !URLResourceIncludes("https://api.example/orders", "https://api.example/orders?tenant=2") {
		t.Error("a resource with no query refused a query: it says nothing about queries")
	}
}
