// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/fanout"
)

// okOutcome builds a successful Outcome for provider with the given
// reply content and latency.
func okOutcome(provider, content string, latency time.Duration) fanout.Outcome {
	return fanout.Outcome{
		Provider: provider,
		Response: &model.Response{
			Message:   model.Message{Role: model.RoleAssistant, Content: content},
			Provider:  provider,
			ModelName: provider + "-model",
		},
		Latency: latency,
	}
}

// errOutcome builds a failed Outcome for provider carrying err.
func errOutcome(provider string, err error, latency time.Duration) fanout.Outcome {
	return fanout.Outcome{Provider: provider, Err: err, Latency: latency}
}

// resultOf wraps outcomes into a *fanout.Result.
func resultOf(outcomes ...fanout.Outcome) *fanout.Result {
	return &fanout.Result{Outcomes: outcomes}
}

func TestPriorityReducer_Apply_selection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// order is PriorityReducer.Order.
		order []string
		// outcomes is the fan-out result, in fan-out order.
		outcomes []fanout.Outcome
		// wantProvider is the Provider of the chosen outcome, and
		// wantContent its reply content.
		wantProvider string
		wantContent  string
		// wantUsedIdx is the index in Provenance.Considered expected to
		// carry Used == true.
		wantUsedIdx int
	}{
		{
			name:         "single success is chosen",
			order:        []string{"ollama", "groq"},
			outcomes:     []fanout.Outcome{okOutcome("ollama", "hola", 5*time.Millisecond)},
			wantProvider: "ollama",
			wantContent:  "hola",
			wantUsedIdx:  0,
		},
		{
			name:  "higher-priority provider wins despite later fan-out index",
			order: []string{"ollama", "groq"},
			outcomes: []fanout.Outcome{
				okOutcome("groq", "from-groq", 2*time.Millisecond),
				okOutcome("ollama", "from-ollama", 9*time.Millisecond),
			},
			wantProvider: "ollama",
			wantContent:  "from-ollama",
			wantUsedIdx:  1,
		},
		{
			name:  "highest-priority SUCCESSFUL outcome wins (failed higher-priority skipped)",
			order: []string{"ollama", "groq"},
			outcomes: []fanout.Outcome{
				errOutcome("ollama", model.ErrProviderUnavailable, 3*time.Millisecond),
				okOutcome("groq", "from-groq", 4*time.Millisecond),
			},
			wantProvider: "groq",
			wantContent:  "from-groq",
			wantUsedIdx:  1,
		},
		{
			name:  "tie on rank broken by lower fan-out index (both unlisted)",
			order: nil,
			outcomes: []fanout.Outcome{
				okOutcome("a", "first", time.Millisecond),
				okOutcome("b", "second", time.Millisecond),
			},
			wantProvider: "a",
			wantContent:  "first",
			wantUsedIdx:  0,
		},
		{
			name:  "tie on same provider rank broken by lower fan-out index",
			order: []string{"x"},
			outcomes: []fanout.Outcome{
				okOutcome("x", "x-first", time.Millisecond),
				okOutcome("x", "x-second", time.Millisecond),
			},
			wantProvider: "x",
			wantContent:  "x-first",
			wantUsedIdx:  0,
		},
		{
			name:  "unlisted provider still eligible when it is the only success",
			order: []string{"ollama"},
			outcomes: []fanout.Outcome{
				errOutcome("ollama", model.ErrProviderUnavailable, time.Millisecond),
				okOutcome("groq", "fallback", 2*time.Millisecond),
			},
			wantProvider: "groq",
			wantContent:  "fallback",
			wantUsedIdx:  1,
		},
		{
			name:  "listed provider beats unlisted even at a later index",
			order: []string{"groq"},
			outcomes: []fanout.Outcome{
				okOutcome("local", "from-local", time.Millisecond),
				okOutcome("groq", "from-groq", time.Millisecond),
			},
			wantProvider: "groq",
			wantContent:  "from-groq",
			wantUsedIdx:  1,
		},
		{
			name:  "winner in the middle of a 3-element slice locks the loop",
			order: []string{"groq"},
			outcomes: []fanout.Outcome{
				okOutcome("local-a", "a", time.Millisecond),
				okOutcome("groq", "winner", time.Millisecond),
				okOutcome("local-b", "b", time.Millisecond),
			},
			wantProvider: "groq",
			wantContent:  "winner",
			wantUsedIdx:  1,
		},
		{
			name:  "duplicate Order entries are inert (first index wins the rank)",
			order: []string{"groq", "groq", "local"},
			outcomes: []fanout.Outcome{
				okOutcome("local", "from-local", time.Millisecond),
				okOutcome("groq", "from-groq", time.Millisecond),
			},
			wantProvider: "groq",
			wantContent:  "from-groq",
			wantUsedIdx:  1,
		},
		{
			name:  "both-non-nil outcome (invariant violation) is skipped even at higher priority",
			order: []string{"ollama", "groq"},
			outcomes: []fanout.Outcome{
				{
					// Contract violation: Response AND Err both set. The
					// conservative guard treats Err != nil as failure, so this
					// higher-priority outcome is skipped, never chosen, and its
					// Err is preserved in provenance.
					Provider: "ollama",
					Response: &model.Response{
						Message:  model.Message{Role: model.RoleAssistant, Content: "tainted"},
						Provider: "ollama",
					},
					Err:     model.ErrProviderResponse,
					Latency: time.Millisecond,
				},
				okOutcome("groq", "clean", 2*time.Millisecond),
			},
			wantProvider: "groq",
			wantContent:  "clean",
			wantUsedIdx:  1,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := PriorityReducer{Order: tt.order}
			got, err := r.Apply(context.Background(), resultOf(tt.outcomes...))
			if err != nil {
				t.Fatalf("Apply returned unexpected error: %v", err)
			}
			if got == nil {
				t.Fatal("Apply returned nil Decision")
			}
			if got.Response == nil {
				t.Fatal("Decision.Response is nil, want a chosen response")
			}
			if got.Response.Provider != tt.wantProvider {
				t.Errorf("chosen Provider = %q, want %q", got.Response.Provider, tt.wantProvider)
			}
			if got.Response.Message.Content != tt.wantContent {
				t.Errorf("chosen Content = %q, want %q", got.Response.Message.Content, tt.wantContent)
			}

			// Provenance: one Contribution per outcome, in fan-out order,
			// exactly one Used (the chosen one), failures carry their Err.
			if len(got.Provenance.Considered) != len(tt.outcomes) {
				t.Fatalf("Provenance.Considered len = %d, want %d",
					len(got.Provenance.Considered), len(tt.outcomes))
			}
			usedCount := 0
			for i, c := range got.Provenance.Considered {
				if c.Provider != tt.outcomes[i].Provider {
					t.Errorf("Considered[%d].Provider = %q, want %q",
						i, c.Provider, tt.outcomes[i].Provider)
				}
				// errors.Is(c.Err, nil) is true exactly when c.Err is nil, so
				// this positively asserts the success rows (nil) AND the
				// failed rows (the preserved sentinel) with one check.
				if !errors.Is(c.Err, tt.outcomes[i].Err) {
					t.Errorf("Considered[%d].Err = %v, want %v", i, c.Err, tt.outcomes[i].Err)
				}
				if c.Used {
					usedCount++
					if i != tt.wantUsedIdx {
						t.Errorf("Used set on index %d, want %d", i, tt.wantUsedIdx)
					}
				}
			}
			if usedCount != 1 {
				t.Errorf("Used count = %d, want exactly 1", usedCount)
			}

			// Accounting: one row per outcome, in order, carrying latency.
			if len(got.Accounting) != len(tt.outcomes) {
				t.Fatalf("Accounting len = %d, want %d", len(got.Accounting), len(tt.outcomes))
			}
			for i, a := range got.Accounting {
				if a.Provider != tt.outcomes[i].Provider {
					t.Errorf("Accounting[%d].Provider = %q, want %q",
						i, a.Provider, tt.outcomes[i].Provider)
				}
				if a.Latency != tt.outcomes[i].Latency {
					t.Errorf("Accounting[%d].Latency = %v, want %v",
						i, a.Latency, tt.outcomes[i].Latency)
				}
			}
		})
	}
}

func TestPriorityReducer_Apply_allFailed(t *testing.T) {
	t.Parallel()

	outcomes := []fanout.Outcome{
		errOutcome("ollama", model.ErrProviderUnavailable, 3*time.Millisecond),
		errOutcome("groq", model.ErrAuthInvalid, 7*time.Millisecond),
	}
	r := PriorityReducer{Order: []string{"ollama", "groq"}}

	got, err := r.Apply(context.Background(), resultOf(outcomes...))

	// Non-nil Decision so provenance/accounting survive for the log.
	if got == nil {
		t.Fatal("all-failed: Decision is nil, want non-nil with provenance")
	}
	if got.Response != nil {
		t.Errorf("all-failed: Decision.Response = %+v, want nil", got.Response)
	}

	// Sentinel is reachable, AND the upstream causes are joined behind it.
	if !errors.Is(err, ErrNoUsableOutcome) {
		t.Errorf("error does not satisfy errors.Is(_, ErrNoUsableOutcome): %v", err)
	}
	if !errors.Is(err, model.ErrProviderUnavailable) {
		t.Errorf("joined error lost model.ErrProviderUnavailable: %v", err)
	}
	if !errors.Is(err, model.ErrAuthInvalid) {
		t.Errorf("joined error lost model.ErrAuthInvalid: %v", err)
	}

	// Provenance lists every failure, none Used.
	if len(got.Provenance.Considered) != len(outcomes) {
		t.Fatalf("Considered len = %d, want %d", len(got.Provenance.Considered), len(outcomes))
	}
	for i, c := range got.Provenance.Considered {
		if c.Provider != outcomes[i].Provider {
			t.Errorf("Considered[%d].Provider = %q, want %q", i, c.Provider, outcomes[i].Provider)
		}
		if c.Used {
			t.Errorf("Considered[%d].Used = true, want false (nothing chosen)", i)
		}
		if c.Err == nil {
			t.Errorf("Considered[%d].Err = nil, want the provider failure", i)
		}
	}

	// Accounting carries provider + latency for diagnosis — the all-failed
	// path is exactly where this matters (ADR-0012 §5a). Assert the values,
	// not just the length.
	if len(got.Accounting) != len(outcomes) {
		t.Fatalf("Accounting len = %d, want %d", len(got.Accounting), len(outcomes))
	}
	for i, a := range got.Accounting {
		if a.Provider != outcomes[i].Provider {
			t.Errorf("Accounting[%d].Provider = %q, want %q", i, a.Provider, outcomes[i].Provider)
		}
		if a.Latency != outcomes[i].Latency {
			t.Errorf("Accounting[%d].Latency = %v, want %v", i, a.Latency, outcomes[i].Latency)
		}
	}
}

func TestPriorityReducer_Apply_bothNilOutcome(t *testing.T) {
	t.Parallel()

	// Contract violation: an Outcome with neither Response nor Err. The
	// Response == nil guard skips it as unusable, so the all-failed path is
	// taken — but this outcome contributes no cause to the join, so its
	// Considered entry carries a nil Err and the error is the bare sentinel.
	outcomes := []fanout.Outcome{
		{Provider: "ollama", Latency: 4 * time.Millisecond}, // Response nil, Err nil
	}
	r := PriorityReducer{}

	got, err := r.Apply(context.Background(), resultOf(outcomes...))

	if !errors.Is(err, ErrNoUsableOutcome) {
		t.Errorf("both-nil: want ErrNoUsableOutcome (even with no cause to join), got %v", err)
	}
	if got == nil {
		t.Fatal("both-nil: Decision is nil, want non-nil")
	}
	if got.Response != nil {
		t.Errorf("both-nil: Response = %+v, want nil", got.Response)
	}
	if len(got.Provenance.Considered) != 1 {
		t.Fatalf("both-nil: Considered len = %d, want 1", len(got.Provenance.Considered))
	}
	if got.Provenance.Considered[0].Err != nil {
		t.Errorf("both-nil: Considered[0].Err = %v, want nil (no cause to record)",
			got.Provenance.Considered[0].Err)
	}
	if got.Provenance.Considered[0].Used {
		t.Error("both-nil: Considered[0].Used = true, want false")
	}
	if got.Accounting[0].Latency != 4*time.Millisecond {
		t.Errorf("both-nil: Accounting latency not preserved = %v", got.Accounting[0].Latency)
	}
}

func TestPriorityReducer_Apply_emptyOutcomes(t *testing.T) {
	t.Parallel()

	r := PriorityReducer{}
	got, err := r.Apply(context.Background(), resultOf())

	if !errors.Is(err, ErrNoUsableOutcome) {
		t.Errorf("empty outcomes: want ErrNoUsableOutcome, got %v", err)
	}
	if got == nil {
		t.Fatal("empty outcomes: Decision is nil, want non-nil")
	}
	if got.Response != nil {
		t.Errorf("empty outcomes: Response = %+v, want nil", got.Response)
	}
	if len(got.Provenance.Considered) != 0 {
		t.Errorf("empty outcomes: Considered len = %d, want 0", len(got.Provenance.Considered))
	}
}

func TestPriorityReducer_Apply_nilResult(t *testing.T) {
	t.Parallel()

	r := PriorityReducer{}
	got, err := r.Apply(context.Background(), nil)

	if !errors.Is(err, ErrNilResult) {
		t.Errorf("nil result: want ErrNilResult, got %v", err)
	}
	if got != nil {
		t.Errorf("nil result: Decision = %+v, want nil", got)
	}
}
