// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The decision verb vocabulary PIN (R12-H2 refactor, director's
// condition b): every verb constant is frozen byte for byte to the
// literal the code carried before the constants existed — these bytes
// are digest terms of every stored tombstone and approval row, so a
// renamed verb would silently orphan every sealed digest of the era
// (the pre-E3 grant-id precedent). Evidence level: in-process unit.
// The literals below are COPIED from the pre-refactor code
// (internal/action/sqlite/approvals.go and internal/cli/approvals.go
// at 3a5bacf), not from memory.

package action

import "testing"

func TestDecisionVocabulary_pinnedByteForByte(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, got, want string }{
		{"DecisionApproved", DecisionApproved, "approved"},
		{"DecisionRejected", DecisionRejected, "rejected"},
		{"DecisionCancelled", DecisionCancelled, "cancelled"},
		{"DecisionClock", DecisionClock, "clock"},
	} {
		if c.got != c.want {
			t.Fatalf("PIN: %s must be exactly %q byte for byte (digest term of the era), got %q", c.name, c.want, c.got)
		}
	}
}

// The predicate is the ONE boundary of the human vocabulary: the three
// human verbs are in; the clock, a case variant, a status spelling and
// the empty string are out (fail closed). Its probing mutation
// (accept any non-empty string) reddens the second loop.
func TestIsHumanDecision_finiteSetFailClosed(t *testing.T) {
	t.Parallel()
	for _, verb := range []string{"approved", "rejected", "cancelled"} {
		if !IsHumanDecision(verb) {
			t.Fatalf("%q is a human verb", verb)
		}
	}
	for _, verb := range []string{"clock", "CLOCK", "Approved", "APPROVED", "expired", "bogus", "approve", ""} {
		if IsHumanDecision(verb) {
			t.Fatalf("%q must NOT count as a human verb", verb)
		}
	}
}
