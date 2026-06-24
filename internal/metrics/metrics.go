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
