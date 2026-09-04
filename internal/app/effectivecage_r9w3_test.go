// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// R9-W3 (eighth Codex pass, P2): the EXACT site includes the
// receiver. The authorization distinguishes a top-level function from
// a method — a briber method NAMED like an allowed function no longer
// matches its site — and NO declaration called ResolveEffectiveCage
// is exempt from having its body scanned: a homonymous wrapper that
// forwards to the real resolver is seen like any other site. The
// auditor's fixtures, permanent. Reproduction-first contract.

package app

import "testing"

// R8 — a method homonymous with an ALLOWED site, inside internal/app:
// the receiver is part of the site key, so it can never match the
// top-level function's authorization.
func TestCageGuard_methodHomonymOfAllowedSiteIsItsOwnSite(t *testing.T) {
	t.Parallel()
	src := `package app
type helperT struct{}
func (h helperT) buildAgentBrain(bc BrainConfig) { _, _ = ResolveEffectiveCage(bc) }`
	refs, err := cageResolverRefs("app_method.go", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(refs["buildAgentBrain"]) != 0 {
		t.Fatalf("AUDIT R9-W3: a METHOD must never occupy the top-level function's site: %v", refs)
	}
	if len(refs["(helperT).buildAgentBrain"]) != 1 {
		t.Fatalf("the method's reference must be seen under its OWN receiver-qualified site: %v", refs)
	}
	ptr := `package app
type helperT struct{}
func (h *helperT) buildAgentBrain(bc BrainConfig) { _, _ = ResolveEffectiveCage(bc) }`
	refs, err = cageResolverRefs("app_ptr_method.go", []byte(ptr))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(refs["(*helperT).buildAgentBrain"]) != 1 {
		t.Fatalf("the pointer-receiver method gets its own site too: %v", refs)
	}
}

// R9 — a declaration CALLED ResolveEffectiveCage is not exempt: its
// body is scanned, and the forwarded reference is seen at its site.
func TestCageGuard_homonymousDeclarationBodyIsScanned(t *testing.T) {
	t.Parallel()
	src := `package cli
import "github.com/Sebastian197/korvun/internal/app"
func ResolveEffectiveCage(bc BrainConfig) { _, _ = app.ResolveEffectiveCage(bc) }`
	refs, err := cageResolverRefs("cli_homonym.go", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(refs["ResolveEffectiveCage"]) != 1 {
		t.Fatalf("AUDIT R9-W3: the homonymous wrapper's body must be scanned, never exempt: %v", refs)
	}
}
