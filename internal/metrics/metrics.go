// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package metrics owns the observability seam (ADR-0020 §2): a push interface
// the domain records operational events through, plus a Nop default so a nil
// backend is never possible. It imports only the standard library; the
// Prometheus implementation lives in a leaf subpackage that is the only place
// importing client_golang. The domain (router, fan-out, brain, telegram)
// depends on this interface, never on Prometheus, which keeps the backend
// swappable (the same seam discipline as conversation.Store and model.Model).
package metrics

import "time"

// Metrics records domain observations on the hot path's existing funnels.
//
// Implementations MUST be safe for concurrent use: the router's N brain
// workers and the fan-out's per-provider goroutines record concurrently, the
// same concurrency discipline model.Model and conversation.Store carry. The
// pull-collected drop gauge is NOT part of this push interface; it is a
// Collector wired in the Prometheus implementation that reads an adapter's
// cumulative DroppedCount at scrape time (ADR-0020 §3).
type Metrics interface {
	// IncMessages counts one inbound message handed to a brain, by channel.
	IncMessages(channel string)
	// ObserveProviderDuration records one provider call's latency and whether
	// it succeeded. Sourced from fanout.Outcome (Latency + Err) AFTER coord.Run,
	// off the hot path, so it adds no contention.
	//
	// F8 (ADR-0031 sub-phase 7): because the wired model is the retry-decorated
	// one, the observed duration is the TOTAL of the provider call — all retry
	// attempts plus their backoff waits — not a single Generate. This is a
	// deliberate, test-pinned semantic (the metric does not lie): a slow value
	// here means "the provider took this long including retries", which is what
	// an operator wants for latency SLOs.
	ObserveProviderDuration(provider string, ok bool, d time.Duration)
	// IncProviderFailure counts one failed provider call, by provider. Kept
	// distinct from the duration histogram's outcome label so failure rate is a
	// direct monotonic counter for alerting (ADR-0020 §3).
	IncProviderFailure(provider string)
	// IncRouterError counts one asynchronous router failure, by
	// RouterError.Kind.String() (the WithErrorHandler funnel).
	IncRouterError(kind string)
	// ObserveTurnsPersisted counts turns durably appended (the AppendTurns
	// group size) on a successful reply.
	ObserveTurnsPersisted(n int)
	// IncProviderRetry counts one EFFECTIVE retry the retry decorator committed
	// to (after the parent/budget checks passed, before the backoff sleep), by
	// provider (ADR-0031 sub-phase 7).
	IncProviderRetry(provider string)
	// IncProviderRetryBudgetExhausted counts one retry budget exhausted without
	// success — the decorator gave up on a RETRYABLE error, either at
	// max_retries or because the next wait would exceed the parent budget
	// (FR-A2) — by provider. A non-retryable failure never bumps it (ADR-0031
	// sub-phase 7).
	IncProviderRetryBudgetExhausted(provider string)
}

// Nop is the no-op Metrics: every method does nothing. It is the default
// backend, injected the way slog.Default() is, so every domain object holds a
// non-nil Metrics and never guards a nil. Using a struct (not nil) means a
// caller can always call methods safely.
type Nop struct{}

// IncMessages does nothing.
func (Nop) IncMessages(string) {}

// ObserveProviderDuration does nothing.
func (Nop) ObserveProviderDuration(string, bool, time.Duration) {}

// IncProviderFailure does nothing.
func (Nop) IncProviderFailure(string) {}

// IncRouterError does nothing.
func (Nop) IncRouterError(string) {}

// ObserveTurnsPersisted does nothing.
func (Nop) ObserveTurnsPersisted(int) {}

// IncProviderRetry does nothing.
func (Nop) IncProviderRetry(string) {}

// IncProviderRetryBudgetExhausted does nothing.
func (Nop) IncProviderRetryBudgetExhausted(string) {}
