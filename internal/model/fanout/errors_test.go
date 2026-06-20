// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package fanout

import (
	"errors"
	"strings"
	"testing"
)

func TestErrNoModels_isDistinct(t *testing.T) {
	if errors.Is(ErrNoModels, ErrNilModel) {
		t.Error("ErrNoModels must not satisfy errors.Is(ErrNilModel)")
	}
}

func TestErrNilModel_isDistinct(t *testing.T) {
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
