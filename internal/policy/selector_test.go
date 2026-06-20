// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/model"
)

// stubModel is a minimal model.Model for catalog tests. SelectModels never
// calls Generate (it is a pure pre-dispatch filter), so Generate is a no-op;
// only Name distinguishes instances for order/identity assertions.
type stubModel struct{ name string }

func (s stubModel) Generate(context.Context, *model.Request) (*model.Response, error) {
	return nil, nil
}

func (s stubModel) Name() string { return s.name }

// names extracts the provider names of a model slice, in order, so tests can
// assert both membership AND ordering (determinism) in one comparison.
func names(models []model.Model) []string {
	out := make([]string, len(models))
	for i, m := range models {
		out[i] = m.Name()
	}
	return out
}

func TestSelectModels(t *testing.T) {
	t.Parallel()

	ollama := stubModel{name: "ollama"}
	groq := stubModel{name: "groq"}
	localA := stubModel{name: "local-a"}
	cloudB := stubModel{name: "cloud-b"}
	localC := stubModel{name: "local-c"}

	tests := []struct {
		name        string
		catalog     []CatalogEntry
		sensitivity Sensitivity
		wantNames   []string // expected provider names in order (nil when wantErr)
		wantErr     error
		wantErrText string // substring the error message must contain (optional)
	}{
		{
			name: "public passes all, order preserved",
			catalog: []CatalogEntry{
				{Model: ollama, Locality: Local},
				{Model: groq, Locality: Cloud},
			},
			sensitivity: Public,
			wantNames:   []string{"ollama", "groq"},
		},
		{
			name: "private filters to local only",
			catalog: []CatalogEntry{
				{Model: ollama, Locality: Local},
				{Model: groq, Locality: Cloud},
			},
			sensitivity: Private,
			wantNames:   []string{"ollama"},
		},
		{
			name: "private preserves order across interleaved localities",
			catalog: []CatalogEntry{
				{Model: localA, Locality: Local},
				{Model: cloudB, Locality: Cloud},
				{Model: localC, Locality: Local},
			},
			sensitivity: Private,
			wantNames:   []string{"local-a", "local-c"},
		},
		{
			name: "public preserves order across interleaved localities",
			catalog: []CatalogEntry{
				{Model: localA, Locality: Local},
				{Model: cloudB, Locality: Cloud},
				{Model: localC, Locality: Local},
			},
			sensitivity: Public,
			wantNames:   []string{"local-a", "cloud-b", "local-c"},
		},
		{
			name: "private single local entry passes",
			catalog: []CatalogEntry{
				{Model: ollama, Locality: Local},
			},
			sensitivity: Private,
			wantNames:   []string{"ollama"},
		},
		{
			name: "private with no local models errors",
			catalog: []CatalogEntry{
				{Model: groq, Locality: Cloud},
			},
			sensitivity: Private,
			wantErr:     ErrNoEligibleModels,
		},
		{
			name:        "empty catalog errors under public",
			catalog:     nil,
			sensitivity: Public,
			wantErr:     ErrNoEligibleModels,
		},
		{
			name:        "empty catalog errors under private",
			catalog:     nil,
			sensitivity: Private,
			wantErr:     ErrNoEligibleModels,
		},
		{
			name: "unknown sensitivity errors (zero value)",
			catalog: []CatalogEntry{
				{Model: ollama, Locality: Local},
			},
			sensitivity: Sensitivity(0),
			wantErr:     ErrUnknownSensitivity,
			wantErrText: "0", // the offending value is surfaced for debugging
		},
		{
			name: "unknown sensitivity errors (out of range)",
			catalog: []CatalogEntry{
				{Model: ollama, Locality: Local},
			},
			sensitivity: Sensitivity(99),
			wantErr:     ErrUnknownSensitivity,
			wantErrText: "99",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := SelectModels(tt.catalog, tt.sensitivity)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want errors.Is(%v)", err, tt.wantErr)
				}
				if tt.wantErrText != "" && !strings.Contains(err.Error(), tt.wantErrText) {
					t.Fatalf("err = %q, want it to contain %q", err.Error(), tt.wantErrText)
				}
				if got != nil {
					t.Fatalf("models = %v, want nil on error", names(got))
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected err = %v", err)
			}
			if gotNames := names(got); !slices.Equal(gotNames, tt.wantNames) {
				t.Fatalf("models = %v, want %v", gotNames, tt.wantNames)
			}
		})
	}
}
