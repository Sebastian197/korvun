// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Effect domain contract — Etapa 3, lote 1, pieza 1 (spec FR-DESC, §10.6):
// the FINITE class enum with its DECLARED total order — the law that makes
// a ceiling comparable — and the folded-in fail-closed rule: an unknown
// class ALWAYS ranks above critical (garbage is never cheaper than the
// worst known class). Classes read from stored rows are fuzzed from
// birth. Approved-red contract.

package action

import (
	"math/rand"
	"testing"
)

// effectLadder is the sealed total order, restated literally in the test
// (the oracle discipline): position IS rank.
var effectLadder = []EffectClass{
	EffectPure,
	EffectReadExternal,
	EffectWriteReversible,
	EffectWriteCompensatable,
	EffectWriteIrreversible,
	EffectCritical,
}

func TestEffectClass_sealedLiteralsAndTotalOrder(t *testing.T) {
	t.Parallel()
	literals := map[EffectClass]string{
		EffectPure:               "pure",
		EffectReadExternal:       "read_external",
		EffectWriteReversible:    "write_reversible",
		EffectWriteCompensatable: "write_compensatable",
		EffectWriteIrreversible:  "write_irreversible",
		EffectCritical:           "critical",
	}
	for class, want := range literals {
		if string(class) != want {
			t.Fatalf("class literal %q must equal its sealed name %q", class, want)
		}
	}
	// The declared order: rank equals ladder position, strictly increasing.
	for i, class := range effectLadder {
		if class.Rank() != i {
			t.Fatalf("Rank(%s) = %d, want ladder position %d", class, class.Rank(), i)
		}
		if i > 0 && effectLadder[i-1].Rank() >= class.Rank() {
			t.Fatalf("the order must be strictly increasing at %s", class)
		}
	}
}

// TestEffectClass_unknownRanksAboveCritical is the folded-in fail-closed
// law: any string outside the finite enum ranks ABOVE critical, so a
// corrupt or future class can never sneak under a ceiling.
func TestEffectClass_unknownRanksAboveCritical(t *testing.T) {
	t.Parallel()
	for _, garbage := range []EffectClass{
		"", "unclassified", "PURE", "pure ", "write", "super_safe", "critical2",
	} {
		if garbage.Rank() <= EffectCritical.Rank() {
			t.Fatalf("unknown class %q must rank ABOVE critical, got %d", garbage, garbage.Rank())
		}
	}
	if EffectClass("unclassified").Rank() <= EffectCritical.Rank() {
		t.Fatal("the E1 placeholder itself is an unknown class: above critical")
	}
}

func TestEffectClass_knownIsTheFiniteSet(t *testing.T) {
	t.Parallel()
	for _, class := range effectLadder {
		if !class.Known() {
			t.Fatalf("%s is a sealed class and must be Known", class)
		}
	}
	for _, garbage := range []EffectClass{"", "unclassified", "bogus"} {
		if garbage.Known() {
			t.Fatalf("%q must not be Known", garbage)
		}
	}
}

// TestEffectClass_orderProperties: totality and antisymmetry over the
// whole known set plus adversarial unknowns, property-style.
func TestEffectClass_orderProperties(t *testing.T) {
	t.Parallel()
	// #nosec G404 -- seeded math/rand ON PURPOSE: deterministic property rounds.
	rng := rand.New(rand.NewSource(20260831))
	pool := append([]EffectClass{}, effectLadder...)
	pool = append(pool, "unclassified", "", "future_class")
	for round := 0; round < 300; round++ {
		a := pool[rng.Intn(len(pool))]
		b := pool[rng.Intn(len(pool))]
		// Totality: every pair is comparable through Rank.
		ra, rb := a.Rank(), b.Rank()
		if !(ra < rb || ra > rb || ra == rb) {
			t.Fatalf("incomparable pair %q %q", a, b)
		}
		// Antisymmetry over the KNOWN set: equal ranks imply equal classes.
		if a.Known() && b.Known() && ra == rb && a != b {
			t.Fatalf("distinct known classes with equal rank: %q %q", a, b)
		}
	}
}

// FuzzEffectClassRank: classes read from stored rows are arbitrary
// strings — never a panic, and NEVER an unknown class at or below
// critical's rank.
func FuzzEffectClassRank(f *testing.F) {
	for _, class := range effectLadder {
		f.Add(string(class))
	}
	f.Add("unclassified")
	f.Add("")
	f.Add("PURE\x00")
	f.Fuzz(func(t *testing.T, raw string) {
		class := EffectClass(raw)
		rank := class.Rank()
		if class.Known() {
			if rank < 0 || rank > EffectCritical.Rank() {
				t.Fatalf("known class %q with out-of-ladder rank %d", raw, rank)
			}
			return
		}
		if rank <= EffectCritical.Rank() {
			t.Fatalf("unknown class %q must rank above critical, got %d", raw, rank)
		}
	})
}

// TestEffectDescriptor_v1SubsetShape pins the sealed v1 fields; the
// RESERVED §10.6 fields must NOT exist as struct fields yet (compile-time
// truth lives in the struct itself; this test documents the contract).
func TestEffectDescriptor_v1SubsetShape(t *testing.T) {
	t.Parallel()
	d := EffectDescriptor{
		Class:               EffectWriteIrreversible,
		ReadsExternalState:  false,
		WritesExternalState: true,
		DataEgress:          true,
		Reversible:          false,
		Compensatable:       false,
	}
	if d.Class.Rank() != EffectWriteIrreversible.Rank() {
		t.Fatal("descriptor carries its class")
	}
}
