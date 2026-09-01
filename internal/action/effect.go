// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Effect domain (Trust Layer Etapa 3, lote 1, spec FR-DESC, blueprint
// §10.6): the FINITE effect-class enum with its DECLARED total order —
// the law that makes a ceiling comparable — and the Effect Descriptor v1
// subset. Data sensitivity stays a SEPARATE dimension (tool.Attrs); the
// two compose, never merge. The engine classifies ONLY from declared
// registry descriptors, never from model text (§9.7).
package action

// EffectClass is the finite classification of an operation's consequence
// (§10.6). The zero value is NOT a valid class: unknown strings —
// including the E1 "unclassified" placeholder on historical rows — rank
// ABOVE critical, the folded-in fail-closed law.
type EffectClass string

// The sealed classes, in ladder order.
const (
	// EffectPure is local computation within limits.
	EffectPure EffectClass = "pure"
	// EffectReadExternal consults external state.
	EffectReadExternal EffectClass = "read_external"
	// EffectWriteReversible writes state with a documented undo.
	EffectWriteReversible EffectClass = "write_reversible"
	// EffectWriteCompensatable writes state with a known compensation.
	EffectWriteCompensatable EffectClass = "write_compensatable"
	// EffectWriteIrreversible writes state with no undo and no known
	// compensation.
	EffectWriteIrreversible EffectClass = "write_irreversible"
	// EffectCritical moves money, credentials, or equivalent.
	EffectCritical EffectClass = "critical"
)

// effectRanks is the DECLARED total order: pure < read_external <
// write_reversible < write_compensatable < write_irreversible < critical.
var effectRanks = map[EffectClass]int{
	EffectPure:               0,
	EffectReadExternal:       1,
	EffectWriteReversible:    2,
	EffectWriteCompensatable: 3,
	EffectWriteIrreversible:  4,
	EffectCritical:           5,
}

// unknownEffectRank sits strictly ABOVE critical: a corrupt, future or
// placeholder class can never sneak under a ceiling (fail-closed §7.5).
const unknownEffectRank = 6

// Rank places a class on the total order. Unknown classes — anything
// outside the finite enum, stored-row garbage included — rank above
// critical.
// OnLadder reports whether c is one of the six declared classes — the
// validation seam for config-provided ceilings (an unknown class must
// die at boot naming the ladder, never rank silently).
func (c EffectClass) OnLadder() bool {
	_, ok := effectRanks[c]
	return ok
}

func (c EffectClass) Rank() int {
	if rank, known := effectRanks[c]; known {
		return rank
	}
	return unknownEffectRank
}

// Known reports whether c is one of the sealed finite classes.
func (c EffectClass) Known() bool {
	_, known := effectRanks[c]
	return known
}

// EffectDescriptor is the §10.6 v1 subset: the declared, registry-verified
// semantics of one operation. RESERVED §10.6 fields and the stages that
// wake them — deliberately NOT fields yet (an unreachable field is a
// field nobody can misuse, the E2 discipline): financial, credential_use,
// criticality (E5 approval policy needs them comparable to act);
// prepare_supported, status_query_supported (E5/E6 prepare and preview);
// idempotency_supported (E6 transactions and replay protection).
type EffectDescriptor struct {
	// Class is the operation's place on the ladder.
	Class EffectClass
	// ReadsExternalState / WritesExternalState name the state touched.
	ReadsExternalState  bool
	WritesExternalState bool
	// DataEgress is true when request or result content leaves the
	// kernel's boundary (into a prompt, onto the network).
	DataEgress bool
	// Reversible / Compensatable state the honest §7.10 semantics.
	Reversible    bool
	Compensatable bool
}
