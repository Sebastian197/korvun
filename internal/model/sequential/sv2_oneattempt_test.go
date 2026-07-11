// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sequential_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/sequential"
)

// TestRun_failingModelConsumesExactlyOneAttempt pins ADR-0031's SV2 invariant at
// the mechanism level: a sequential model that fails is tried EXACTLY ONCE
// before the coordinator fails over to the next — the serial fail-over IS the
// retry story, so no per-model retry may multiply the attempt.
//
// NOTE (honest): this is GREEN today — the retry decorator (sub-phase 4) does
// not exist yet, so a sequential model is already called once. It is an
// invariant GUARD: when the decorator lands, sub-phase 4 must keep retry OFF in
// sequential, and this test is the tripwire that catches a regression. It is
// included now because the SV2 contract is defined now (ADR-0031 Decision 3).
func TestRun_failingModelConsumesExactlyOneAttempt(t *testing.T) {
	t.Parallel()

	failing := &fakeModel{name: "ollama", err: errors.New("down")}
	succeeding := &fakeModel{name: "groq", resp: okResp("groq")}

	res, err := sequential.New().Run(context.Background(), validReq(), []model.Model{failing, succeeding})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if failing.calls != 1 {
		t.Errorf("failing model calls = %d, want exactly 1 (no retry before fail-over — SV2)", failing.calls)
	}
	if succeeding.calls != 1 {
		t.Errorf("succeeding model calls = %d, want 1 (fail-over reached it)", succeeding.calls)
	}
	// The fail-over produced a usable outcome from the second model.
	last := res.Outcomes[len(res.Outcomes)-1]
	if last.Err != nil || last.Provider != "groq" {
		t.Errorf("final outcome = %+v, want groq success", last)
	}
}
