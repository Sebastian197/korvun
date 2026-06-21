// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/fanout"
	"github.com/Sebastian197/korvun/internal/policy"
)

// Compile-time assertion that *Orchestrator satisfies the Brain seam.
var _ Brain = (*Orchestrator)(nil)

// Coordinator is the dispatch seam the Orchestrator runs each request through.
// It abstracts the dispatch SHAPE — parallel fan-out (fanout.Coordinator) vs
// serial cost-saving fail-over (sequential.Coordinator) — so the binary can
// mount either from config (ADR-0017 §3). Both concrete coordinators satisfy it
// unchanged: identical Run signature, both returning *fanout.Result, so the
// policy layer downstream is agnostic to which shape produced the Result.
type Coordinator interface {
	Run(ctx context.Context, req *model.Request, models []model.Model) (*fanout.Result, error)
}

// Compile-time assertion that the parallel fan-out satisfies the seam. The
// serial sequential.Coordinator is asserted in the test package, keeping this
// production package's import graph minimal (it need not import sequential).
var _ Coordinator = (*fanout.Coordinator)(nil)

// defaultFallback is the reply sent when no usable answer exists and the
// operator did not configure one (ADR-0014 §3).
const defaultFallback = "Sorry, no answer is available right now. Please try again."

// defaultHistoryTurns is how many prior turns the Orchestrator loads when a
// conversation store is configured but the caller passes a non-positive count.
const defaultHistoryTurns = 10

// Orchestrator is the stateless Brain (ADR-0014). It translates an inbound
// Envelope to a request, fans the request out to a fixed set of models,
// applies a fixed policy to the outcomes, and translates the decision back to
// an outbound Envelope.
//
// It holds NO per-call mutable state: coord, models, policy, fallback,
// systemPrompt and logger are read-only after construction, and every per-call
// value is a local. It is therefore safe to share across the router's worker
// goroutines and agnostic to how many there are (ADR-0014 §4). Conversation
// memory (Stage 9, ADR-0018) is an injected, conversation-keyed store (the
// optional store field), NOT per-instance history — that would be shared state
// across the router's workers, i.e. a race. The store itself carries the
// concurrency contract; the Orchestrator stays stateless.
//
// models and policy are interfaces so a future SelectingBrain (per-message
// model/policy selection) can wrap this Orchestrator without changing it.
type Orchestrator struct {
	coord        Coordinator
	models       []model.Model
	policy       policy.Policy
	fallback     string
	systemPrompt string
	logger       *slog.Logger
	// store is the optional, conversation-keyed memory (ADR-0018). It holds the
	// state; the Orchestrator never does (closing ADR-0014 §4). nil = stateless,
	// behaving exactly as Stage 11. historyN is how many prior turns to load.
	store    conversation.Store
	historyN int
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

// WithConversationStore injects a conversation memory store and the number of
// prior turns to load before each dispatch (recentTurns; non-positive falls back
// to defaultHistoryTurns). The store holds all memory state — the Orchestrator
// stays stateless and safe to share across the router's workers (ADR-0014 §4,
// ADR-0018 §5). A nil store is ignored, leaving the Brain stateless (Stage 11
// behavior). The store is consulted only for envelopes that carry a conversation
// id; without one, the Brain answers statelessly rather than dropping the reply.
//
// Concurrency: the user+assistant pair of a reply is persisted atomically via
// Store.AppendTurns, so the pair stays contiguous even when two messages of the
// same conversation are handled concurrently (brainWorkers > 1). One assumption
// remains and is accepted for context memory: the order BETWEEN concurrent
// messages of the same conversation is not guaranteed without per-conversation
// worker affinity, and two concurrent messages each load history without the
// other's turn. This is fine for best-effort context; revisit if strict
// per-conversation ordering is needed (would require worker affinity).
func WithConversationStore(store conversation.Store, recentTurns int) Option {
	return func(o *Orchestrator) {
		if store == nil {
			return
		}
		o.store = store
		if recentTurns <= 0 {
			recentTurns = defaultHistoryTurns
		}
		o.historyN = recentTurns
	}
}

// NewOrchestrator constructs a stateless Brain over the given dispatch
// coordinator (fan-out or sequential — ADR-0017 §3), model set, and policy.
// models should be assembled with WithModelID so each provider receives its own
// model id (ADR-0014 §2).
func NewOrchestrator(coord Coordinator, models []model.Model, p policy.Policy, opts ...Option) *Orchestrator {
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
	key, history := o.loadHistory(ctx, env)

	req, ok := requestWithHistory(env, o.systemPrompt, history)
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
			// The (canned) fallback is NOT persisted — only real answers are.
			o.logNoAnswer(env, dec, err)
			return decisionToEnvelopes(o.fallback, env), nil
		}
		// Anything else (e.g. ErrNilResult) is mechanism misuse — a Brain bug.
		return nil, fmt.Errorf("brain: policy: %w", err)
	}

	content := dec.Response.Message.Content
	// Persist on the happy path only: the user+assistant pair as one atomic group
	// via AppendTurns, so the pair stays contiguous under concurrency (ADR-0018
	// reconciliation note). key is the empty Key when no store is configured or no
	// conversation id is present, in which case persistTurns is a no-op.
	o.persistTurns(ctx, key, latestText(env.Parts), content)
	return decisionToEnvelopes(content, env), nil
}

// loadHistory derives the conversation key and loads the recent turns when a
// store is configured and the envelope carries a conversation id. Memory is an
// enhancement, never a hard dependency: a missing key or a load error degrades
// to a stateless answer (logged), it never drops the reply. It returns the empty
// Key when memory is unavailable, which persistTurns treats as a no-op.
func (o *Orchestrator) loadHistory(ctx context.Context, env *envelope.Envelope) (conversation.Key, []conversation.Turn) {
	if o.store == nil {
		return "", nil
	}
	key, err := conversation.KeyFromEnvelope(env)
	if err != nil {
		o.logger.Warn("brain: no conversation key, answering without memory",
			"envelope_id", env.ID, "channel", env.Channel, "cause", err)
		return "", nil
	}
	history, err := o.store.LoadRecent(ctx, key, o.historyN)
	if err != nil {
		o.logger.Warn("brain: load history failed, answering without memory",
			"envelope_id", env.ID, "channel", env.Channel, "cause", err)
		return key, nil
	}
	return key, history
}

// persistTurns appends the user turn and the assistant turn for a successful
// reply as ONE atomic group via AppendTurns, so the pair stays contiguous in the
// history even when two messages of the same conversation are handled
// concurrently (brainWorkers > 1). Two separate Appends would let the pairs
// interleave into a non-alternating, provider-rejected history (ADR-0018
// reconciliation note). It is a no-op when key is empty (no store / no
// conversation id). Empty-content turns are skipped: an empty turn would fail
// model.ValidateRequest when reloaded into a later request. A store error is
// logged, not propagated, so a memory write never breaks the reply path.
func (o *Orchestrator) persistTurns(ctx context.Context, key conversation.Key, userText, assistantText string) {
	if o.store == nil || key == "" {
		return
	}
	now := time.Now()
	turns := make([]conversation.Turn, 0, 2)
	if userText != "" {
		turns = append(turns, conversation.Turn{Role: conversation.RoleUser, Content: userText, Timestamp: now})
	}
	if assistantText != "" {
		turns = append(turns, conversation.Turn{Role: conversation.RoleAssistant, Content: assistantText, Timestamp: now})
	}
	if len(turns) == 0 {
		return
	}
	if _, err := o.store.AppendTurns(ctx, key, turns...); err != nil {
		o.logger.Warn("brain: append turns failed", "key", string(key), "cause", err)
	}
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
