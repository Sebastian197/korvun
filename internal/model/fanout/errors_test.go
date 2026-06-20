// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package fanout

import (
	"errors"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/model"
)

// The fanout sentinels live in a different package from the model.* sentinels
// and describe mechanism-level configuration bugs, NOT per-provider failures.
// The grammars MUST stay disjoint: a policy that branches on a model.* sentinel
// must never accidentally branch on a fanout-level sentinel and vice versa.
// These tests pin that disjointness as a contract, not the trivial "two
// distinct errors.New values are unequal" property the Go runtime already
// guarantees.

func TestFanoutSentinels_disjointFromModelSentinels(t *testing.T) {
	modelSentinels := []struct {
		name string
		err  error
	}{
		{"ErrNilRequest", model.ErrNilRequest},
		{"ErrEmptyModel", model.ErrEmptyModel},
		{"ErrEmptyMessages", model.ErrEmptyMessages},
		{"ErrInvalidRole", model.ErrInvalidRole},
		{"ErrEmptyContent", model.ErrEmptyContent},
		{"ErrProviderUnavailable", model.ErrProviderUnavailable},
		{"ErrProviderResponse", model.ErrProviderResponse},
		{"ErrAuthInvalid", model.ErrAuthInvalid},
		{"ErrRateLimited", model.ErrRateLimited},
	}
	fanoutSentinels := []struct {
		name string
		err  error
	}{
		{"ErrNoModels", ErrNoModels},
		{"ErrNilModel", ErrNilModel},
	}
	for _, fo := range fanoutSentinels {
		for _, ms := range modelSentinels {
			if errors.Is(fo.err, ms.err) {
				t.Errorf("fanout.%s must NOT satisfy errors.Is(model.%s) — grammars must stay disjoint",
					fo.name, ms.name)
			}
			if errors.Is(ms.err, fo.err) {
				t.Errorf("model.%s must NOT satisfy errors.Is(fanout.%s) — grammars must stay disjoint",
					ms.name, fo.name)
			}
		}
	}
}

func TestFanoutSentinels_disjointFromEachOther(t *testing.T) {
	// Mutual distinctness: a policy that branches on ErrNoModels must
	// never accidentally hit on ErrNilModel and vice versa. (Stronger
	// than the previous tautological pair: this catches an accidental
	// alias if someone later rewrote one as fmt.Errorf("...: %w", other).)
	if errors.Is(ErrNoModels, ErrNilModel) {
		t.Error("ErrNoModels must not satisfy errors.Is(ErrNilModel)")
	}
	if errors.Is(ErrNilModel, ErrNoModels) {
		t.Error("ErrNilModel must not satisfy errors.Is(ErrNoModels)")
	}
}

func TestErrNoModels_message(t *testing.T) {
	if !strings.Contains(ErrNoModels.Error(), "fanout:") {
		t.Errorf("ErrNoModels.Error() = %q, missing 'fanout:' prefix", ErrNoModels.Error())
	}
	if !strings.Contains(ErrNoModels.Error(), "no models") {
		t.Errorf("ErrNoModels.Error() = %q, missing semantic phrase", ErrNoModels.Error())
	}
}

func TestErrNilModel_message(t *testing.T) {
	if !strings.Contains(ErrNilModel.Error(), "fanout:") {
		t.Errorf("ErrNilModel.Error() = %q, missing 'fanout:' prefix", ErrNilModel.Error())
	}
	if !strings.Contains(ErrNilModel.Error(), "nil model") {
		t.Errorf("ErrNilModel.Error() = %q, missing semantic phrase", ErrNilModel.Error())
	}
}
