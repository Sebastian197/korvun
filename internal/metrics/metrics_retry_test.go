// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package metrics_test

import (
	"testing"

	"github.com/Sebastian197/korvun/internal/metrics"
)

// TestNop_retryCountersNoOp pins the two new retry counters onto the Metrics
// interface (ADR-0031 sub-phase 7, FR-M4) and that Nop no-ops them.
//
// RED note: IncProviderRetry / IncProviderRetryBudgetExhausted do not exist on
// the interface yet, so this file fails to build — the red for the interface
// addition; GREEN adds both methods to Metrics, Nop, and the prom impl.
func TestNop_retryCountersNoOp(t *testing.T) {
	var m metrics.Metrics = metrics.Nop{}
	m.IncProviderRetry("ollama")
	m.IncProviderRetryBudgetExhausted("ollama")
}
