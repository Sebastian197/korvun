// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"testing"
	"time"
)

// TestNop_satisfiesMetricsAndDoesNotPanic pins the seam: Nop is a Metrics, and
// every method is a safe no-op. Nop is the default backend (injected like
// slog.Default), so the domain holds a non-nil Metrics and never nil-checks
// (ADR-0020 §2).
func TestNop_satisfiesMetricsAndDoesNotPanic(t *testing.T) {
	var m Metrics = Nop{}
	m.IncMessages("telegram")
	m.ObserveProviderDuration("groq", true, 250*time.Millisecond)
	m.ObserveProviderDuration("ollama", false, 30*time.Second)
	m.IncProviderFailure("groq")
	m.IncRouterError("handle")
	m.ObserveTurnsPersisted(2)
}
