// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package prom is the Prometheus implementation of the metrics.Metrics seam
// (ADR-0020 §2). It is the ONLY package in Korvun that imports
// client_golang, so the choice of backend stays a leaf-local, reversible
// decision: the domain depends on the metrics.Metrics interface, never on
// Prometheus.
//
// It owns a PRIVATE registry (prometheus.NewRegistry) rather than the library's
// global DefaultRegisterer — the default auto-registers collectors in an
// init(), i.e. mutable global state, which CLAUDE.md forbids. The Go runtime and
// process collectors are therefore registered explicitly on the private
// registry.
package prom

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/Sebastian197/korvun/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// providerDurationBuckets are LLM-shaped, not HTTP-shaped: provider calls run to
// the 30s per-model timeout (app.DefaultPerModelTimeout), well past the 10s tail
// of prometheus.DefBuckets. Buckets span 50ms..30s so the timeout tail is
// visible (ADR-0020 §3).
var providerDurationBuckets = []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 20, 30}

// Metrics is the Prometheus-backed metrics.Metrics. Construct with New; the
// zero value is not usable. Its instruments are concurrency-safe by
// construction, satisfying the metrics.Metrics concurrency contract.
type Metrics struct {
	reg *prometheus.Registry

	messages       *prometheus.CounterVec
	providerDur    *prometheus.HistogramVec
	providerFail   *prometheus.CounterVec
	providerRetry  *prometheus.CounterVec
	retryExhausted *prometheus.CounterVec
	routerErrors   *prometheus.CounterVec
	turnsPersisted prometheus.Counter
}

// Compile-time assertion that *Metrics satisfies the domain seam.
var _ metrics.Metrics = (*Metrics)(nil)

// New builds a Metrics over a fresh private registry with the Go runtime and
// process collectors registered, plus every Korvun instrument.
func New() *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	m := &Metrics{
		reg: reg,
		messages: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "korvun_messages_processed_total",
			Help: "Inbound messages handed to a brain, by channel.",
		}, []string{"channel"}),
		providerDur: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "korvun_provider_request_duration_seconds",
			Help:    "Provider call latency, by provider and outcome.",
			Buckets: providerDurationBuckets,
		}, []string{"provider", "outcome"}),
		providerFail: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "korvun_provider_failures_total",
			Help: "Failed provider calls, by provider.",
		}, []string{"provider"}),
		providerRetry: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "korvun_provider_retries_total",
			Help: "Effective provider retries, by provider.",
		}, []string{"provider"}),
		retryExhausted: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "korvun_provider_retry_budget_exhausted_total",
			Help: "Provider retry budgets exhausted without success, by provider.",
		}, []string{"provider"}),
		routerErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "korvun_router_errors_total",
			Help: "Asynchronous router failures, by kind.",
		}, []string{"kind"}),
		turnsPersisted: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "korvun_conversation_turns_persisted_total",
			Help: "Turns durably appended on a successful reply.",
		}),
	}
	reg.MustRegister(m.messages, m.providerDur, m.providerFail, m.providerRetry,
		m.retryExhausted, m.routerErrors, m.turnsPersisted)
	return m
}

// Gatherer exposes the private registry as a prometheus.Gatherer so the admin
// HTTP server can build a promhttp handler over it (ADR-0020 §4) without
// importing Prometheus elsewhere in the domain.
func (m *Metrics) Gatherer() prometheus.Gatherer { return m.reg }

// Handler returns the http.Handler that serves the private registry's metrics
// in Prometheus text format. Built here so promhttp stays inside this leaf; the
// admin server mounts the returned handler at /metrics as a plain http.Handler.
// Collection errors are logged through the supplied slog logger rather than the
// default (which writes to the response body only).
func (m *Metrics) Handler(logger *slog.Logger) http.Handler {
	return promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{
		ErrorLog: slogPromLogger{logger: logger},
	})
}

// slogPromLogger adapts a *slog.Logger to promhttp.Logger (a single Println
// method) so metric-collection errors land in Korvun's structured logs.
type slogPromLogger struct{ logger *slog.Logger }

// Println implements promhttp.Logger.
func (l slogPromLogger) Println(v ...any) {
	l.logger.Error("metrics handler error", "detail", fmt.Sprint(v...))
}

// IncMessages counts one inbound message handed to a brain, by channel.
func (m *Metrics) IncMessages(channel string) {
	m.messages.WithLabelValues(channel).Inc()
}

// ObserveProviderDuration records one provider call's latency under the
// (provider, outcome) labels; outcome is "ok" or "error".
//
// F8 (ADR-0031 sub-phase 7): d is the TOTAL provider-call time — all retry
// attempts plus their backoff waits — because the wired model is retry-decorated
// and fanout.CallOne times that whole decorated call. The histogram therefore
// measures end-to-end provider latency including retries, by design.
func (m *Metrics) ObserveProviderDuration(provider string, ok bool, d time.Duration) {
	outcome := "error"
	if ok {
		outcome = "ok"
	}
	m.providerDur.WithLabelValues(provider, outcome).Observe(d.Seconds())
}

// IncProviderFailure counts one failed provider call, by provider.
func (m *Metrics) IncProviderFailure(provider string) {
	m.providerFail.WithLabelValues(provider).Inc()
}

// IncProviderRetry counts one effective provider retry, by provider (ADR-0031
// sub-phase 7).
func (m *Metrics) IncProviderRetry(provider string) {
	m.providerRetry.WithLabelValues(provider).Inc()
}

// IncProviderRetryBudgetExhausted counts one retry budget exhausted without
// success, by provider (ADR-0031 sub-phase 7).
func (m *Metrics) IncProviderRetryBudgetExhausted(provider string) {
	m.retryExhausted.WithLabelValues(provider).Inc()
}

// IncRouterError counts one asynchronous router failure, by kind.
func (m *Metrics) IncRouterError(kind string) {
	m.routerErrors.WithLabelValues(kind).Inc()
}

// ObserveTurnsPersisted adds the size of one persisted turn group to the total.
func (m *Metrics) ObserveTurnsPersisted(n int) {
	if n > 0 {
		m.turnsPersisted.Add(float64(n))
	}
}

// RegisterDroppedSource registers a PULL counter exposing
// korvun_channel_messages_dropped_total{channel} sourced from a cumulative
// counter function (e.g. telegram.Adapter.DroppedCount), read at scrape time.
// Using prometheus.NewCounterFunc (a built-in pull collector) avoids
// double-instrumenting: the adapter already maintains the atomic counter, so the
// metric layer reads it rather than incrementing a parallel one (ADR-0020 §3).
// The channel name is a ConstLabel because CounterFunc carries no variable
// labels; call once per channel.
//
// It returns the registration error rather than panicking (Register, not
// MustRegister): a duplicate channel name yields prometheus.AlreadyRegisteredError
// instead of crashing boot. The caller logs and continues — a metrics
// registration must never take down the serve path (review F2).
func (m *Metrics) RegisterDroppedSource(channel string, count func() uint64) error {
	return m.reg.Register(prometheus.NewCounterFunc(prometheus.CounterOpts{
		Name:        "korvun_channel_messages_dropped_total",
		Help:        "Inbound messages dropped after enqueue timeout, by channel.",
		ConstLabels: prometheus.Labels{"channel": channel},
	}, func() float64 { return float64(count()) }))
}

// RegisterPullCounter registers a named PULL counter sourced from a cumulative
// counter function read at scrape time (prometheus.NewCounterFunc), the same
// no-double-instrument pattern as RegisterDroppedSource (ADR-0020 §3). The
// live-view's bus drop count (bus.DroppedCount) and SSE drop count
// (liveview.DroppedCount) are exposed this way (ADR-0024 §1). Like
// RegisterDroppedSource it returns the registration error rather than panicking,
// so a duplicate name never takes down boot (review F2).
func (m *Metrics) RegisterPullCounter(name, help string, count func() uint64) error {
	return m.reg.Register(prometheus.NewCounterFunc(prometheus.CounterOpts{
		Name: name,
		Help: help,
	}, func() float64 { return float64(count()) }))
}
