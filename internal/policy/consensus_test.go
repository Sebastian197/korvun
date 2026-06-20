// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/fanout"
)

// usedProviders returns the set of providers whose Contribution is Used.
func usedProviders(d *Decision) map[string]bool {
	out := map[string]bool{}
	for _, c := range d.Provenance.Considered {
		if c.Used {
			out[c.Provider] = true
		}
	}
	return out
}

func TestConsensusReducer_Apply_majority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		order     []string
		normalize func(string) string
		outcomes  []fanout.Outcome
		// wantRep is the Provider whose Response must become Decision.Response.
		wantRep string
		// wantUsed is the exact set of providers expected to carry Used==true
		// (the winning class members).
		wantUsed []string
	}{
		{
			name:  "3 of 5 agree, representative by lowest index (no Order)",
			order: nil,
			outcomes: []fanout.Outcome{
				okOutcome("a", "positive", 1*time.Millisecond),
				okOutcome("b", "negative", 2*time.Millisecond),
				okOutcome("c", "positive", 3*time.Millisecond),
				okOutcome("d", "positive", 4*time.Millisecond),
				okOutcome("e", "negative", 5*time.Millisecond),
			},
			wantRep:  "a", // lowest index among {a,c,d}
			wantUsed: []string{"a", "c", "d"},
		},
		{
			name:  "3 of 5 agree, representative by Order priority",
			order: []string{"d", "c"}, // d outranks c outranks unlisted a
			outcomes: []fanout.Outcome{
				okOutcome("a", "positive", 1*time.Millisecond),
				okOutcome("b", "negative", 2*time.Millisecond),
				okOutcome("c", "positive", 3*time.Millisecond),
				okOutcome("d", "positive", 4*time.Millisecond),
				okOutcome("e", "negative", 5*time.Millisecond),
			},
			wantRep:  "d", // highest priority among {a,c,d}
			wantUsed: []string{"a", "c", "d"},
		},
		{
			name:  "majority over a failed outcome (3 usable, 2 agree)",
			order: nil,
			outcomes: []fanout.Outcome{
				okOutcome("a", "yes", 1*time.Millisecond),
				errOutcome("b", model.ErrProviderUnavailable, 2*time.Millisecond),
				okOutcome("c", "yes", 3*time.Millisecond),
				okOutcome("d", "no", 4*time.Millisecond),
			},
			wantRep:  "a", // {a,c} agree on "yes"; 2*2=4 > 3 usable
			wantUsed: []string{"a", "c"},
		},
		{
			name:  "default normalize groups casing/whitespace into one class",
			order: nil,
			outcomes: []fanout.Outcome{
				okOutcome("a", "  YES ", 1*time.Millisecond),
				okOutcome("b", "yes", 2*time.Millisecond),
				okOutcome("c", "no", 3*time.Millisecond),
			},
			wantRep:  "a", // "  YES " and "yes" both normalize to "yes" → 2 of 3
			wantUsed: []string{"a", "b"},
		},
		{
			name:      "custom normalize (case-sensitive) splits a class the default would merge",
			order:     nil,
			normalize: func(s string) string { return strings.TrimSpace(s) }, // no lowercasing
			outcomes: []fanout.Outcome{
				okOutcome("a", "Yes", 1*time.Millisecond),
				okOutcome("b", "yes", 2*time.Millisecond),
				okOutcome("c", "yes", 3*time.Millisecond),
			},
			wantRep:  "b", // "Yes" != "yes" under custom; {b,c} agree on "yes"
			wantUsed: []string{"b", "c"},
		},
		{
			name:  "exactly two agree is the minimal consensus",
			order: nil,
			outcomes: []fanout.Outcome{
				okOutcome("a", "x", 1*time.Millisecond),
				okOutcome("b", "x", 2*time.Millisecond),
			},
			wantRep:  "a", // 2 of 2, floor satisfied; lowest index represents
			wantUsed: []string{"a", "b"},
		},
		{
			name:  "empty normalized content is a real, winnable class (ADR-0013 §2)",
			order: nil,
			outcomes: []fanout.Outcome{
				okOutcome("a", "", 1*time.Millisecond),
				okOutcome("b", "   ", 2*time.Millisecond), // → "" after trim
				okOutcome("c", "x", 3*time.Millisecond),
			},
			wantRep:  "a", // {a,b} share the empty class, 2 of 3
			wantUsed: []string{"a", "b"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := ConsensusReducer{Order: tt.order, Normalize: tt.normalize}
			got, err := r.Apply(context.Background(), resultOf(tt.outcomes...))
			if err != nil {
				t.Fatalf("Apply returned unexpected error: %v", err)
			}
			if got == nil || got.Response == nil {
				t.Fatal("Apply returned nil Decision/Response, want a consensus reply")
			}
			if got.Response.Provider != tt.wantRep {
				t.Errorf("representative Provider = %q, want %q", got.Response.Provider, tt.wantRep)
			}

			wantUsed := map[string]bool{}
			for _, p := range tt.wantUsed {
				wantUsed[p] = true
			}
			gotUsed := usedProviders(got)
			if len(gotUsed) != len(wantUsed) {
				t.Errorf("Used set = %v, want %v", gotUsed, wantUsed)
			}
			for p := range wantUsed {
				if !gotUsed[p] {
					t.Errorf("provider %q expected Used==true, was not", p)
				}
			}

			// Provenance + accounting cover every outcome, in fan-out order.
			if len(got.Provenance.Considered) != len(tt.outcomes) {
				t.Fatalf("Considered len = %d, want %d", len(got.Provenance.Considered), len(tt.outcomes))
			}
			for i, c := range got.Provenance.Considered {
				if c.Provider != tt.outcomes[i].Provider {
					t.Errorf("Considered[%d].Provider = %q, want %q", i, c.Provider, tt.outcomes[i].Provider)
				}
				if !errors.Is(c.Err, tt.outcomes[i].Err) {
					t.Errorf("Considered[%d].Err = %v, want %v", i, c.Err, tt.outcomes[i].Err)
				}
			}
			if len(got.Accounting) != len(tt.outcomes) {
				t.Fatalf("Accounting len = %d, want %d", len(got.Accounting), len(tt.outcomes))
			}
			for i, a := range got.Accounting {
				if a.Provider != tt.outcomes[i].Provider {
					t.Errorf("Accounting[%d].Provider = %q, want %q", i, a.Provider, tt.outcomes[i].Provider)
				}
				if a.Latency != tt.outcomes[i].Latency {
					t.Errorf("Accounting[%d].Latency = %v, want %v", i, a.Latency, tt.outcomes[i].Latency)
				}
			}
		})
	}
}

func TestConsensusReducer_Apply_noConsensus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		outcomes []fanout.Outcome
	}{
		{
			name: "2 vs 2 tie is not a majority",
			outcomes: []fanout.Outcome{
				okOutcome("a", "x", time.Millisecond),
				okOutcome("b", "y", time.Millisecond),
				okOutcome("c", "x", time.Millisecond),
				okOutcome("d", "y", time.Millisecond),
			},
		},
		{
			name: "three-way split, no class over half",
			outcomes: []fanout.Outcome{
				okOutcome("a", "x", time.Millisecond),
				okOutcome("b", "y", time.Millisecond),
				okOutcome("c", "z", time.Millisecond),
			},
		},
		{
			name: "plurality without majority (2-2-1) is not consensus",
			outcomes: []fanout.Outcome{
				okOutcome("a", "x", time.Millisecond),
				okOutcome("b", "y", time.Millisecond),
				okOutcome("c", "x", time.Millisecond),
				okOutcome("d", "y", time.Millisecond),
				okOutcome("e", "z", time.Millisecond),
			},
		},
		{
			name: "single success is not a consensus (floor of two)",
			outcomes: []fanout.Outcome{
				okOutcome("a", "x", time.Millisecond),
			},
		},
		{
			name: "single success among failures is no-consensus, not all-failed",
			outcomes: []fanout.Outcome{
				okOutcome("a", "x", time.Millisecond),
				errOutcome("b", model.ErrProviderUnavailable, time.Millisecond),
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := ConsensusReducer{}
			got, err := r.Apply(context.Background(), resultOf(tt.outcomes...))

			if !errors.Is(err, ErrNoConsensus) {
				t.Errorf("want ErrNoConsensus, got %v", err)
			}
			// Must NOT be confused with the all-failed sentinel.
			if errors.Is(err, ErrNoUsableOutcome) {
				t.Errorf("no-consensus must not satisfy ErrNoUsableOutcome: %v", err)
			}
			if got == nil {
				t.Fatal("no-consensus: Decision is nil, want non-nil with provenance")
			}
			if got.Response != nil {
				t.Errorf("no-consensus: Response = %+v, want nil", got.Response)
			}
			// Nothing won, so nothing is Used.
			for i, c := range got.Provenance.Considered {
				if c.Used {
					t.Errorf("no-consensus: Considered[%d].Used = true, want false", i)
				}
			}
			if len(got.Provenance.Considered) != len(tt.outcomes) {
				t.Errorf("Considered len = %d, want %d", len(got.Provenance.Considered), len(tt.outcomes))
			}
			// Accounting stays fully populated for the log (ADR-0013 §7).
			if len(got.Accounting) != len(tt.outcomes) {
				t.Fatalf("Accounting len = %d, want %d", len(got.Accounting), len(tt.outcomes))
			}
			for i, a := range got.Accounting {
				if a.Provider != tt.outcomes[i].Provider || a.Latency != tt.outcomes[i].Latency {
					t.Errorf("Accounting[%d] = {%q,%v}, want {%q,%v}",
						i, a.Provider, a.Latency, tt.outcomes[i].Provider, tt.outcomes[i].Latency)
				}
			}
		})
	}
}

func TestConsensusReducer_Apply_allFailed(t *testing.T) {
	t.Parallel()

	outcomes := []fanout.Outcome{
		errOutcome("ollama", model.ErrProviderUnavailable, 3*time.Millisecond),
		errOutcome("groq", model.ErrAuthInvalid, 7*time.Millisecond),
	}
	r := ConsensusReducer{}

	got, err := r.Apply(context.Background(), resultOf(outcomes...))

	// All failed → ErrNoUsableOutcome (nothing to vote on), NOT ErrNoConsensus.
	if !errors.Is(err, ErrNoUsableOutcome) {
		t.Errorf("all-failed: want ErrNoUsableOutcome, got %v", err)
	}
	if errors.Is(err, ErrNoConsensus) {
		t.Errorf("all-failed must not satisfy ErrNoConsensus: %v", err)
	}
	// Upstream causes joined behind the sentinel.
	if !errors.Is(err, model.ErrProviderUnavailable) || !errors.Is(err, model.ErrAuthInvalid) {
		t.Errorf("all-failed: joined error lost a cause: %v", err)
	}
	if got == nil || got.Response != nil {
		t.Fatalf("all-failed: want non-nil Decision with nil Response, got %+v", got)
	}
	for i, c := range got.Provenance.Considered {
		if c.Used {
			t.Errorf("all-failed: Considered[%d].Used = true, want false", i)
		}
		if c.Err == nil {
			t.Errorf("all-failed: Considered[%d].Err = nil, want the failure", i)
		}
	}
	for i, a := range got.Accounting {
		if a.Provider != outcomes[i].Provider || a.Latency != outcomes[i].Latency {
			t.Errorf("all-failed: Accounting[%d] = {%q,%v}, want {%q,%v}",
				i, a.Provider, a.Latency, outcomes[i].Provider, outcomes[i].Latency)
		}
	}
}

// A both-non-nil Outcome (contract violation: Response AND Err set) must NOT
// vote — Err != nil makes it a failure. Constructed so that if it DID vote it
// would tip a 1-1 split into a false "yes" majority; the reducer must instead
// return ErrNoConsensus, with the tainted Outcome's Err preserved and unused.
func TestConsensusReducer_Apply_bothNonNilDoesNotVote(t *testing.T) {
	t.Parallel()

	outcomes := []fanout.Outcome{
		{
			Provider: "a",
			Response: &model.Response{
				Message:  model.Message{Role: model.RoleAssistant, Content: "yes"},
				Provider: "a",
			},
			Err:     model.ErrProviderResponse,
			Latency: time.Millisecond,
		},
		okOutcome("b", "yes", time.Millisecond),
		okOutcome("c", "no", time.Millisecond),
	}

	got, err := ConsensusReducer{}.Apply(context.Background(), resultOf(outcomes...))

	if !errors.Is(err, ErrNoConsensus) {
		t.Errorf("want ErrNoConsensus (tainted must not vote), got %v", err)
	}
	if got.Provenance.Considered[0].Used {
		t.Error("tainted both-non-nil Outcome must not be Used")
	}
	if !errors.Is(got.Provenance.Considered[0].Err, model.ErrProviderResponse) {
		t.Errorf("tainted Outcome's Err not preserved: %v", got.Provenance.Considered[0].Err)
	}
}

// A both-nil Outcome (neither Response nor Err) is unusable but contributes no
// cause. A result of only such outcomes is all-failed → ErrNoUsableOutcome, and
// the returned error joins NO causes (the bare sentinel), distinct from a result
// of genuine failures.
func TestConsensusReducer_Apply_bothNilNoUsable(t *testing.T) {
	t.Parallel()

	outcomes := []fanout.Outcome{
		{Provider: "a", Latency: 4 * time.Millisecond}, // Response nil, Err nil
	}

	got, err := ConsensusReducer{}.Apply(context.Background(), resultOf(outcomes...))

	if !errors.Is(err, ErrNoUsableOutcome) {
		t.Errorf("both-nil: want ErrNoUsableOutcome, got %v", err)
	}
	// Nothing was joined behind the sentinel (no cause existed).
	if err.Error() != ErrNoUsableOutcome.Error() {
		t.Errorf("both-nil: error should be the bare sentinel, got %q", err.Error())
	}
	if got == nil || got.Response != nil {
		t.Fatalf("both-nil: want non-nil Decision with nil Response, got %+v", got)
	}
	if got.Provenance.Considered[0].Err != nil {
		t.Errorf("both-nil: Considered[0].Err = %v, want nil", got.Provenance.Considered[0].Err)
	}
}

func TestConsensusReducer_Apply_nilResult(t *testing.T) {
	t.Parallel()

	got, err := ConsensusReducer{}.Apply(context.Background(), nil)
	if !errors.Is(err, ErrNilResult) {
		t.Errorf("nil result: want ErrNilResult, got %v", err)
	}
	if got != nil {
		t.Errorf("nil result: Decision = %+v, want nil", got)
	}
}
