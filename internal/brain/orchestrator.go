// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/fanout"
	"github.com/Sebastian197/korvun/internal/policy"
)

// Compile-time assertion that *Orchestrator satisfies the Brain seam.
var _ Brain = (*Orchestrator)(nil)

// defaultFallback is the reply sent when no usable answer exists and the
// operator did not configure one (ADR-0014 §3).
const defaultFallback = "Sorry, no answer is available right now. Please try again."

// Orchestrator is the stateless Brain (ADR-0014). It translates an inbound
// Envelope to a request, fans the request out to a fixed set of models,
// applies a fixed policy to the outcomes, and translates the decision back to
// an outbound Envelope.
//
// It holds NO per-call mutable state: coord, models, policy, fallback,
// systemPrompt and logger are read-only after construction, and every per-call
// value is a local. It is therefore safe to share across the router's worker
// goroutines and agnostic to how many there are (ADR-0014 §4). Future
// conversation memory (Stage 9) is an injected, conversation-keyed store, NOT
// fields here — per-instance history would be shared state across the router's
// workers, i.e. a race.
//
// models and policy are interfaces so a future SelectingBrain (per-message
// model/policy selection) can wrap this Orchestrator without changing it.
type Orchestrator struct {
	coord        *fanout.Coordinator
	models       []model.Model
	policy       policy.Policy
	fallback     string
	systemPrompt string
	logger       *slog.Logger
}

// Option configures an Orchestrator at construction.
type Option func(*Orchestrator)

// WithFallback overrides the reply sent when no usable answer exists.
func WithFallback(text string) Option {
	return func(o *Orchestrator) {
		if text != "" {
			o.fallback = text
		}
	}
}

// WithSystemPrompt prepends a system Message to every request.
func WithSystemPrompt(prompt string) Option {
	return func(o *Orchestrator) { o.systemPrompt = prompt }
}

// WithLogger sets the structured logger used to record no-answer outcomes.
// A nil logger is ignored (the default stays slog.Default()).
func WithLogger(l *slog.Logger) Option {
	return func(o *Orchestrator) {
		if l != nil {
			o.logger = l
		}
	}
}

// NewOrchestrator constructs a stateless Brain over the given fan-out
// coordinator, model set, and policy. models should be assembled with
// WithModelID so each provider receives its own model id (ADR-0014 §2).
func NewOrchestrator(coord *fanout.Coordinator, models []model.Model, p policy.Policy, opts ...Option) *Orchestrator {
	o := &Orchestrator{
		coord:    coord,
		models:   models,
		policy:   p,
		fallback: defaultFallback,
		logger:   slog.Default(),
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// Handle implements Brain: translate the inbound Envelope, fan the request out,
// apply the policy, and translate the decision back to an outbound reply.
//
// Error contract (ADR-0014 §3):
//   - no text to ask        → (nil, nil)         no reply, no fan-out
//   - coord.Run fails       → (nil, wrapped err) Brain misconfiguration → router hook
//   - policy no-answer       → (fallback, nil)    normal outcome → reply + logged provenance
//   - policy mechanism error → (nil, wrapped err) Brain bug → router hook
func (o *Orchestrator) Handle(ctx context.Context, env *envelope.Envelope) ([]*envelope.Envelope, error) {
	req, ok := envelopeToRequest(env, o.systemPrompt)
	if !ok {
		return nil, nil // nothing to ask — clean no-reply (ADR-0014 §5)
	}

	res, err := o.coord.Run(ctx, req, o.models)
	if err != nil {
		// nil ctx / no models / nil model / request validation: the Brain is
		// misconfigured, not "no answer". Surface to the router error hook.
		return nil, fmt.Errorf("brain: fan-out: %w", err)
	}

	dec, err := o.policy.Apply(ctx, res)
	if err != nil {
		if classifyPolicyErr(err) {
			// Providers all failed or disagreed: a normal product outcome.
			// The user gets a fallback reply; the operator gets the provenance.
			o.logNoAnswer(env, dec, err)
			return decisionToEnvelopes(o.fallback, env), nil
		}
		// Anything else (e.g. ErrNilResult) is mechanism misuse — a Brain bug.
		return nil, fmt.Errorf("brain: policy: %w", err)
	}

	return decisionToEnvelopes(dec.Response.Message.Content, env), nil
}

// logNoAnswer records a no-answer outcome with its provenance, so the operator
// can see why (which providers failed / how the vote split) even though the
// user receives only the fallback reply.
func (o *Orchestrator) logNoAnswer(in *envelope.Envelope, dec *policy.Decision, cause error) {
	considered := 0
	if dec != nil {
		considered = len(dec.Provenance.Considered)
	}
	o.logger.Warn("brain: no usable answer",
		"envelope_id", in.ID,
		"channel", in.Channel,
		"considered", considered,
		"cause", cause,
	)
}

// classifyPolicyErr reports whether a policy.Apply error is a normal "no
// answer" product outcome (fallback) versus a mechanism bug (propagate).
func classifyPolicyErr(err error) (noAnswer bool) {
	return errors.Is(err, policy.ErrNoUsableOutcome) || errors.Is(err, policy.ErrNoConsensus)
}
