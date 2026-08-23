// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package controlapi serves the read-only operator control API (ADR-0022,
// Stage 13): two GET endpoints under /api exposing the live, resolved wiring of
// a running Korvun process. It is a leaf — it depends only on the standard
// library and the small Reader seam internal/app implements, never on the
// router or brain concrete types.
//
// The cut is READ-ONLY by decision. It mounts on the existing loopback admin
// server (ADR-0020 §4) and never mutates state, so it keeps that server's
// no-auth security calculus exactly intact: deferring mutation IS the security
// decision. Mutation — and the auth it would require — is Stage 14 (ADR-0022 §4).
//
// Responses are a fixed, secret-free shape: brain and channel wiring facts only,
// never a secret value and never even an env-var NAME (ADR-0022 §4).
package controlapi

import (
	"encoding/json"
	"net/http"
)

// ModelSummary is the minimal secret-free identity of a model that survived the
// privacy selector: provider name + model id, nothing that grazes credentials
// (ADR-0022 §2). Locality is deliberately not exposed (the brain's sensitivity
// already conveys the privacy posture).
type ModelSummary struct {
	Provider string `json:"provider"`
	ModelID  string `json:"model_id"`
	// Health is the model's observed liveness — "unknown" (never probed),
	// "warming" (boot probe in flight), "ready" (probe answered), or
	// "unreachable" (probe failed). Only warmup-enabled models are ever
	// probed: probing an opted-out model would cost tokens (and, for a
	// private brain, leak a probe to a cloud provider), so absence of
	// evidence stays an honest "unknown", never an invented "ready" (N6).
	Health string `json:"health"`
	// HealthDetail carries the provider's error for an unreachable model —
	// the model layer's errors are secret-free by construction (H8) — and is
	// omitted otherwise.
	HealthDetail string `json:"health_detail,omitempty"`
}

// BrainSummary is the resolved view of one registered brain: its declared
// attributes plus the models that actually remain dispatchable after the privacy
// selector ran at boot (ADR-0015). That survivor set exists only in the running
// binary — not in the config file, not in /metrics.
type BrainSummary struct {
	Name        string         `json:"name"`
	Sensitivity string         `json:"sensitivity"`
	Policy      string         `json:"policy"`
	Dispatch    string         `json:"dispatch"`
	Models      []ModelSummary `json:"models"`
}

// ChannelSummary is the resolved view of one channel. Dropped is the live
// cumulative inbound-drop count; it is omitted for a channel with no counter.
type ChannelSummary struct {
	Type    string  `json:"type"`
	Mode    string  `json:"mode"`
	Name    string  `json:"name"`
	Dropped *uint64 `json:"dropped,omitempty"`
}

// Reader is the read-only seam the control API depends on, implemented by
// internal/app. Implementations return defensive copies; the handlers only read.
//
// Today App serves a boot SNAPSHOT for brains (immutable at runtime in this
// read-only cut). When Stage 14 adds mutation, the implementation moves to a
// LIVE registry view — and THIS interface is unchanged across that transition
// (ADR-0022 §3). So the snapshot is the correct implementation of a seam that
// survives Stage 14, not debt.
type Reader interface {
	BrainSummaries() []BrainSummary
	ChannelSummaries() []ChannelSummary
}

// Mounter is the subset of *httpserver.Server (and of *http.ServeMux) the
// control API needs to register its routes. Taking an interface keeps controlapi
// a leaf that does not import internal/httpserver.
type Mounter interface {
	Handle(pattern string, h http.Handler)
}

// Register mounts the read-only control API on m: exactly two GET routes under
// /api, no mutating route by construction (ADR-0022 §1). Call it before the
// server starts — the mux is not safe to mutate once serving.
func Register(m Mounter, r Reader) {
	m.Handle("GET /api/brains", brainsHandler(r))
	m.Handle("GET /api/channels", channelsHandler(r))
}

func brainsHandler(r Reader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, r.BrainSummaries())
	})
}

func channelsHandler(r Reader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, r.ChannelSummaries())
	})
}

// writeJSON marshals v FIRST so a marshal error becomes a 500 before any header
// is written (a half-written 200 body cannot be corrected). The payload is one
// of the fixed summary types above, so it is secret-free by construction.
func writeJSON(w http.ResponseWriter, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}
