// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"fmt"
	"time"

	"github.com/Sebastian197/korvun/internal/brain"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/model/retry"
	"github.com/Sebastian197/korvun/internal/policy"
	"github.com/Sebastian197/korvun/internal/tool"
)

// The router-ceiling derivation (ADR-0031 Decision 2). The app derives the
// per-Handle ceiling from each brain's per-model timeouts and dispatch shape so
// an operator can never misconfigure it into a guillotine; the router-wide
// timeout is the MAX over brains (>= every brain's worst case). An explicit
// config override is honored only if it clears that max.

// Dispatch-shape tags for deriveBrainCeiling. They match the config dispatch
// strings ("fanout"/"sequential") plus the agent shape the app detects from a
// brain's agent block.
const (
	shapeFanout     = "fanout"
	shapeSequential = "sequential"
	shapeAgent      = "agent"
)

// defaultCeilingMargin is the slack added to every derived ceiling above the
// worst-case model time: it absorbs the non-model parts of Handle (translation,
// policy reduction, store append) so the ceiling never guillotines a model that
// finishes right at its per-attempt window. It is deliberately modest — the
// per-attempt/backoff terms dominate.
const defaultCeilingMargin = 500 * time.Millisecond

// brainCeilingSpec is the resolved input to deriveBrainCeiling: all durations
// are already resolved (per-model timeouts parsed, defaults applied). perAttempt
// and backoffBudget are per-model and index-aligned; agentMaxIter and
// perToolTimeout apply only to the agent shape.
type brainCeilingSpec struct {
	shape          string
	perAttempt     []time.Duration
	backoffBudget  []time.Duration
	agentMaxIter   int
	perToolTimeout time.Duration
	margin         time.Duration
}

// deriveBrainCeiling returns the router ceiling for one brain, per ADR-0031
// Decision 2 (after SV1/SV2):
//
//   - fanout    : max_i( perAttempt_i + backoffBudget_i ) + margin
//     (cancel-on-first-usable-success means the worst INDIVIDUAL model governs,
//     never a sum; retries never multiply the per-attempt window — SV1)
//   - sequential: Σ_i( perAttempt_i ) + margin
//     (the serial fail-over walks the models once; retry is off by construction,
//     so no retry multiplication and no backoff term — SV2)
//   - agent     : maxIterations × ( perAttempt_model + perToolTimeout ) + margin
//     (the bounded single-model loop, SV1)
//
// An unknown shape is a programmer error (callers build the spec from validated
// config), so it panics loudly rather than returning a silent zero that would
// under-bound the ceiling.
func deriveBrainCeiling(spec brainCeilingSpec) time.Duration {
	switch spec.shape {
	case shapeFanout:
		var worst time.Duration
		for i, pa := range spec.perAttempt {
			c := pa + backoffAt(spec.backoffBudget, i)
			if c > worst {
				worst = c
			}
		}
		return worst + spec.margin
	case shapeSequential:
		var sum time.Duration
		for _, pa := range spec.perAttempt {
			sum += pa
		}
		return sum + spec.margin
	case shapeAgent:
		var per time.Duration
		if len(spec.perAttempt) > 0 {
			per = spec.perAttempt[0]
		}
		return time.Duration(spec.agentMaxIter)*(per+spec.perToolTimeout) + spec.margin
	default:
		panic(fmt.Sprintf("app: deriveBrainCeiling: unknown dispatch shape %q", spec.shape))
	}
}

// agentToolBudget derives the worst-case tool time ONE agent iteration may
// legitimately spend (re-review follow-up F4): the per-turn call cap times
// the widest hard bound among the LISTED tools — http_fetch and webhook_call
// carry their own defaults, local tools get a 1s allowance. No tools, no
// budget. Deliberately static (the tools' default bounds, not per-config
// refinements): the ceiling errs generous, never guillotines (ADR-0031).
func agentToolBudget(bc config.BrainConfig) time.Duration {
	if bc.Agent == nil || len(bc.Agent.Tools) == 0 {
		return 0
	}
	var worst time.Duration
	for _, name := range bc.Agent.Tools {
		var bound time.Duration
		switch name {
		case "http_fetch":
			bound = tool.DefaultFetchTimeout
		case "webhook_call":
			bound = tool.DefaultWebhookTimeout
		default:
			bound = time.Second // local, in-process tools
		}
		if bound > worst {
			worst = bound
		}
	}
	return time.Duration(brain.MaxNativeCallsPerTurn) * worst
}

// backoffAt reads the i-th backoff budget, tolerating a nil/short slice (0).
func backoffAt(budget []time.Duration, i int) time.Duration {
	if i < len(budget) {
		return budget[i]
	}
	return 0
}

// deriveRouterCeiling computes the router-wide brain-handler timeout: the MAX of
// each brain's derived ceiling, or an explicit config override that clears that
// max (ErrCeilingOverrideTooLow otherwise).
func deriveRouterCeiling(cfg *config.Config) (time.Duration, error) {
	var maxCeiling time.Duration
	for _, bc := range cfg.Brains {
		if c := ceilingForBrain(cfg, bc); c > maxCeiling {
			maxCeiling = c
		}
	}
	if cfg.BrainHandlerTimeout == "" {
		return maxCeiling, nil
	}
	override, err := time.ParseDuration(cfg.BrainHandlerTimeout)
	if err != nil {
		// config.Validate already rejected an unparseable override; this is the
		// defensive guard for a Config built without Load.
		return 0, fmt.Errorf("app: brain_handler_timeout %q: %w", cfg.BrainHandlerTimeout, err)
	}
	if override < maxCeiling {
		return 0, fmt.Errorf("%w: %v < derived %v", ErrCeilingOverrideTooLow, override, maxCeiling)
	}
	return override, nil
}

// ceilingForBrain builds the spec for one brain and derives its ceiling. Every
// shape sources its per-attempt window from the resolved per-model
// request_timeout (the value the retry decorator applies): fan-out/sequential
// from each model, the agent from its single model (bc.Models[0]) — the retry
// decorator owns the agent's per-attempt deadline since ADR-0031 sub-phase 4.
// The fan-out backoffBudget is maxRetries × retry.MaxBackoffPerWait (FR-A3);
// sequential and agent carry no backoff term (retry off / single loop).
func ceilingForBrain(cfg *config.Config, bc config.BrainConfig) time.Duration {
	if bc.Agent != nil {
		maxIter := bc.Agent.MaxIterations
		if maxIter <= 0 {
			maxIter = brain.DefaultAgentMaxIterations
		}
		// Budget the model that SURVIVES the privacy selector — the one
		// buildAgentBrain actually runs (estreno E-12): a Private brain
		// listing a fast cloud model first would otherwise derive a ceiling
		// below one real attempt of its local survivor and guillotine every
		// cold call. Mirrors brainSummary's Public-keeps-all /
		// Private-keeps-local rule; on any parse failure fall back to
		// Models[0] conservatively — the build itself fails loud later.
		var m config.ModelConfig
		if len(bc.Models) > 0 {
			m = bc.Models[0]
			if sens, err := parseSensitivity(bc.Sensitivity); err == nil {
				for _, cand := range bc.Models {
					loc, err := parseLocality(cand.Locality)
					if err != nil {
						continue
					}
					if sens == policy.Public || loc == policy.Local {
						m = cand
						break
					}
				}
			}
		}
		return deriveBrainCeiling(brainCeilingSpec{
			shape:        shapeAgent,
			perAttempt:   []time.Duration{cfg.EffectiveRequestTimeout(m)},
			agentMaxIter: maxIter,
			// The per-iteration TOOL budget (re-review follow-up F4): the
			// native lane runs up to brain.MaxNativeCallsPerTurn tools per
			// response, each bounded by its own hard timeout — a legitimate
			// multi-tool exchange must never be guillotined by a ceiling
			// that only counted model attempts (ADR-0031 Decision 2).
			perToolTimeout: agentToolBudget(bc),
			margin:         defaultCeilingMargin,
		})
	}
	perAttempt := make([]time.Duration, len(bc.Models))
	backoff := make([]time.Duration, len(bc.Models))
	for i, m := range bc.Models {
		perAttempt[i] = cfg.EffectiveRequestTimeout(m)
		backoff[i] = time.Duration(effectiveMaxRetries(bc, m)) * retry.MaxBackoffPerWait
	}
	shape := shapeFanout
	if bc.Dispatch == "sequential" {
		shape = shapeSequential
	}
	return deriveBrainCeiling(brainCeilingSpec{
		shape:         shape,
		perAttempt:    perAttempt,
		backoffBudget: backoff,
		margin:        defaultCeilingMargin,
	})
}
