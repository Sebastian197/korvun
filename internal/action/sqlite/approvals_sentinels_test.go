// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The DIRECTION of every approvals sentinel, walked whole.
//
// Born from a defect that shipped: ErrApprovalNotFound WRAPS ErrNotFound, so
// `errors.Is(specific, generic)` is true and `errors.Is(generic, specific)` is
// false — and a door that returned the generic one made every caller's
// errors.Is(err, ErrApprovalNotFound) silently false. The refusal then fell
// through a switch's default and was published as transient, or as a sealed
// decision that did not exist.
//
// One mould for the whole set, not one for the case that was caught: a
// direction bug is a CLASS, and a class needs a test that walks every member.

package sqlite

import (
	"errors"
	"os"
	"regexp"
	"testing"

	"github.com/Sebastian197/korvun/internal/action"
)

// approvalSentinels is the set the walks below cover, and
// TestApprovalSentinels_theSetIsClosed keeps it from going stale: it counts the
// ErrApproval* declarations in the package source and fails when one is missing
// here. Without that count the list would rot silently behind the next
// sentinel, and a rotted list is worse than no list — it reads like coverage.
var approvalSentinels = map[string]error{
	"ErrApprovalNotFound":             ErrApprovalNotFound,
	"ErrApprovalParamsEmpty":          ErrApprovalParamsEmpty,
	"ErrApprovalClaimSkipped":         ErrApprovalClaimSkipped,
	"ErrApprovalInvalidated":          ErrApprovalInvalidated,
	"ErrApprovalEvidenceCorrupt":      ErrApprovalEvidenceCorrupt,
	"ErrApprovalUnreadable":           ErrApprovalUnreadable,
	"ErrApprovalParamsDigestMismatch": ErrApprovalParamsDigestMismatch,
	"ErrApprovalActionNotPending":     ErrApprovalActionNotPending,
}

// TestApprovalSentinels_noneIsReachableFromAnother is the direction, both ways,
// with no exceptions. Two sentinels that answer each other's errors.Is are two
// names the caller cannot tell apart, which is the whole reason they are typed.
//
// Probing mutation: make any two of them wrap each other ⇒ this reddens.
func TestApprovalSentinels_noneIsReachableFromAnother(t *testing.T) {
	t.Parallel()
	for aName, a := range approvalSentinels {
		for bName, b := range approvalSentinels {
			if aName == bName {
				continue
			}
			if errors.Is(a, b) {
				t.Errorf("errors.Is(%s, %s) is true — two sentinels the caller cannot separate", aName, bName)
			}
		}
	}
}

// TestApprovalSentinels_theGenericNeverAnswersForTheSpecific pins the direction
// that shipped wrong. The three «you will not get the params» sentinels keep
// ErrNotFound in their chain so older callers are unchanged — and that is a
// ONE-WAY street: a bare ErrNotFound must never satisfy any of them.
//
// Probing mutation: declare any of the three as a plain errors.New and have the
// doors return it ⇒ the compatibility half reddens; make ErrNotFound wrap one
// of them ⇒ this half reddens.
func TestApprovalSentinels_theGenericNeverAnswersForTheSpecific(t *testing.T) {
	t.Parallel()
	backwardCompatible := []struct {
		name string
		err  error
	}{
		{"ErrApprovalNotFound", ErrApprovalNotFound},
		{"ErrApprovalParamsEmpty", ErrApprovalParamsEmpty},
		{"ErrApprovalClaimSkipped", ErrApprovalClaimSkipped},
	}
	for _, tc := range backwardCompatible {
		if !errors.Is(tc.err, ErrNotFound) {
			t.Errorf("%s no longer keeps ErrNotFound in its chain — older callers break", tc.name)
		}
		if errors.Is(ErrNotFound, tc.err) {
			t.Errorf("a bare ErrNotFound answers for %s — the direction that shipped wrong", tc.name)
		}
	}
}

// TestApprovalDoors_everyMissingRowNamesItself is the cure for the root cause:
// EVERY door that can be handed an id must answer ErrApprovalNotFound for a row
// that is not there. One door returning the generic one is all it takes for a
// caller's switch to fall through to its default and publish a lie.
//
// Probing mutation: return a bare ErrNotFound from any of these ⇒ its row
// reddens.
func TestApprovalDoors_everyMissingRowNamesItself(t *testing.T) {
	t.Parallel()
	doors := map[string]func(s *Store) error{
		"GetApproval": func(s *Store) error {
			_, _, err := s.GetApproval(t.Context(), "apr_nope")
			return err
		},
		"GetApprovalByAction": func(s *Store) error {
			_, _, err := s.GetApprovalByAction(t.Context(), "act_nope")
			return err
		},
		"ApprovalParams": func(s *Store) error {
			_, err := s.ApprovalParams(t.Context(), "apr_nope")
			return err
		},
		"ClaimApprovalParams": func(s *Store) error {
			_, err := s.ClaimApprovalParams(t.Context(), "apr_nope", nil)
			return err
		},
		"ClaimApprovalParamsUnderDigest": func(s *Store) error {
			_, _, err := s.ClaimApprovalParamsUnderDigest(t.Context(), "apr_nope", nil, "sha256:x")
			return err
		},
		"ApprovalDetail": func(s *Store) error {
			_, err := s.ApprovalDetail(t.Context(), "apr_nope")
			return err
		},
		"ApprovalStatusOf": func(s *Store) error {
			_, err := s.ApprovalStatusOf(t.Context(), "apr_nope")
			return err
		},
	}
	for name, call := range doors {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store, _ := openTemp(t)
			err := call(store)
			if !errors.Is(err, ErrApprovalNotFound) {
				t.Fatalf("err = %v, want ErrApprovalNotFound", err)
			}
			if errors.Is(err, ErrApprovalUnreadable) {
				t.Fatalf("an absent row must never read as a driver failure")
			}
			if errors.Is(err, ErrApprovalEvidenceCorrupt) {
				t.Fatalf("an absent row must never read as corrupt evidence")
			}
		})
	}
	_ = action.ApprovalPending
}

// TestApprovalSentinels_theSetIsClosed counts the ErrApproval* sentinels the
// package declares and refuses to pass while approvalSentinels does not name
// them all.
//
// It exists because its absence was a published falsehood: the comment above
// invoked it by name for a whole commit while the test did not exist, so the
// list it promised to keep honest was covered by nothing.
//
// Probing mutation: remove any entry from approvalSentinels ⇒ this reddens.
func TestApprovalSentinels_theSetIsClosed(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile("approvals_v15.go")
	if err != nil {
		t.Fatalf("read the sentinel declarations: %v", err)
	}
	declared := regexp.MustCompile(`(?m)^\t(ErrApproval\w+)\s+=`).FindAllStringSubmatch(string(src), -1)
	if len(declared) == 0 {
		t.Fatal("found no ErrApproval* declarations — the scan is broken, not the set")
	}
	for _, m := range declared {
		if _, listed := approvalSentinels[m[1]]; !listed {
			t.Errorf("%s is declared and NOT in approvalSentinels — the walks below skip it in silence", m[1])
		}
	}
	if len(declared) != len(approvalSentinels) {
		t.Errorf("declared %d sentinels, listed %d", len(declared), len(approvalSentinels))
	}
}
