// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package prom

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// These pin the Prometheus contract for ADR-0031 sub-phase 7's retry counters
// (FR-M1/FR-M3): korvun_provider_retries_total and
// korvun_provider_retry_budget_exhausted_total, labelled by provider only.
//
// RED note: New() has no IncProviderRetry / IncProviderRetryBudgetExhausted yet,
// so this file fails to build — the red for the prom impl.

func TestIncProviderRetry_countsByProvider(t *testing.T) {
	m := New()
	m.IncProviderRetry("ollama")
	m.IncProviderRetry("ollama")

	const expected = `
# HELP korvun_provider_retries_total Effective provider retries, by provider.
# TYPE korvun_provider_retries_total counter
korvun_provider_retries_total{provider="ollama"} 2
`
	if err := testutil.GatherAndCompare(m.Gatherer(), strings.NewReader(expected), "korvun_provider_retries_total"); err != nil {
		t.Errorf("provider retries counter mismatch:\n%v", err)
	}
}

func TestIncProviderRetryBudgetExhausted_countsByProvider(t *testing.T) {
	m := New()
	m.IncProviderRetryBudgetExhausted("groq")

	const expected = `
# HELP korvun_provider_retry_budget_exhausted_total Provider retry budgets exhausted without success, by provider.
# TYPE korvun_provider_retry_budget_exhausted_total counter
korvun_provider_retry_budget_exhausted_total{provider="groq"} 1
`
	if err := testutil.GatherAndCompare(m.Gatherer(), strings.NewReader(expected), "korvun_provider_retry_budget_exhausted_total"); err != nil {
		t.Errorf("retry budget exhausted counter mismatch:\n%v", err)
	}
}
