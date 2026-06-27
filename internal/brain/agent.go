// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/metrics"
	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/fanout"
	"github.com/Sebastian197/korvun/internal/tool"
)

// Compile-time assertion that *AgentBrain satisfies the Brain seam (ADR-0021 §1).
var _ Brain = (*AgentBrain)(nil)

// defaultMaxIterations is the hard loop cap when the operator does not set one
// (ADR-0021 §2): an unbounded model→tool→model loop is an infinite loop burning
// cloud quota, so SOME cap is mandatory.
const defaultMaxIterations = 5

// observationPrefix marks a tool result fed back to the model (ADR-0021 §3.3).
// The parser never treats a line starting with it as a tool call; it rides as a
// user message because model.Role has no Tool role (ADR-0009).
const observationPrefix = "OBSERVATION: "

// AgentBrain is a stateless Brain (ADR-0014 §4) that runs a BOUNDED single-model
// tool-use loop (ADR-0021): it asks one model, and while the model requests a
// tool it executes the tool and feeds the result back as an OBSERVATION, until
// the model answers or the iteration cap is hit (§2). It is a SIBLING of the
// Orchestrator, not a wrapper of it, and it touches no other seam (decision B2).
//
// It holds NO per-call mutable state — model, tools, limits, fallback,
// systemPrompt, logger, metrics, store are read-only after construction; every
// per-call value (the running []model.Message, the iteration counter, each tool
// result) is a LOCAL in Handle. It is therefore safe to share across the router's
// N worker goroutines (§5). The injected tools MUST honor the Tool concurrency
// contract (tool.Tool godoc): N workers may call one Tool instance at once.
type AgentBrain struct {
	model        model.Model
	tools        tool.Registry
	maxIters     int
	perTool      time.Duration
	perModelCall time.Duration
	fallback     string
	systemPrompt string
	logger       *slog.Logger
	metrics      metrics.Metrics
	// store is the optional, conversation-keyed memory (ADR-0018). It persists the
	// FINAL user+assistant pair only — never the tool-use trace (§6). nil =
	// stateless. historyN is how many prior turns to load.
	store    conversation.Store
	historyN int
	// now is the clock seam (fanout.CallOne latency + persisted turn timestamps).
	now func() time.Time
}

// AgentOption configures an AgentBrain at construction.
type AgentOption func(*AgentBrain)

// WithAgentFallback overrides the reply sent when the loop yields no answer.
func WithAgentFallback(text string) AgentOption {
	return func(a *AgentBrain) {
		if text != "" {
			a.fallback = text
		}
	}
}

// WithAgentSystemPrompt sets the operator system prompt, appended AFTER the
// protocol block in the seed system message (ADR-0021 §3.1).
func WithAgentSystemPrompt(prompt string) AgentOption {
	return func(a *AgentBrain) { a.systemPrompt = prompt }
}

// WithAgentLogger sets the structured logger. A nil logger is ignored.
func WithAgentLogger(l *slog.Logger) AgentOption {
	return func(a *AgentBrain) {
		if l != nil {
			a.logger = l
		}
	}
}

// WithAgentMetrics injects the observability backend. A nil argument is ignored
// (the default stays metrics.Nop). The recorder MUST be concurrency-safe: the
// router's N workers share one AgentBrain (§5).
func WithAgentMetrics(m metrics.Metrics) AgentOption {
	return func(a *AgentBrain) {
		if m != nil {
			a.metrics = m
		}
	}
}

// WithAgentMaxIterations sets the hard loop cap (ADR-0021 §2). A non-positive
// value is ignored, leaving the default. This bound is a SAFETY invariant, not a
// tuning knob: it is what makes the loop terminate.
func WithAgentMaxIterations(n int) AgentOption {
	return func(a *AgentBrain) {
		if n > 0 {
			a.maxIters = n
		}
	}
}

// WithAgentPerToolTimeout bounds each Tool.Execute call (ADR-0021 §2), mirroring
// fanout.WithPerModelTimeout. A non-positive value leaves tools sharing the
// Handle ctx alone.
func WithAgentPerToolTimeout(d time.Duration) AgentOption {
	return func(a *AgentBrain) {
		if d > 0 {
			a.perTool = d
		}
	}
}

// WithAgentPerModelTimeout bounds each model call inside the loop (passed to
// fanout.CallOne). A non-positive value leaves the model call sharing the Handle
// ctx alone.
func WithAgentPerModelTimeout(d time.Duration) AgentOption {
	return func(a *AgentBrain) {
		if d > 0 {
			a.perModelCall = d
		}
	}
}

// WithAgentConversationStore injects conversation memory and the number of prior
// turns to load (non-positive falls back to defaultHistoryTurns). The store holds
// all memory state — the AgentBrain stays stateless (§5). Only the FINAL pair is
// persisted, never the tool-use trace (§6). A nil store is ignored.
func WithAgentConversationStore(store conversation.Store, recentTurns int) AgentOption {
	return func(a *AgentBrain) {
		if store == nil {
			return
		}
		a.store = store
		if recentTurns <= 0 {
			recentTurns = defaultHistoryTurns
		}
		a.historyN = recentTurns
	}
}

// WithAgentClock overrides the clock (tests inject a deterministic one).
func WithAgentClock(now func() time.Time) AgentOption {
	return func(a *AgentBrain) {
		if now != nil {
			a.now = now
		}
	}
}

// NewAgentBrain constructs a stateless tool-use AgentBrain over a SINGLE model and
// an injected tool registry. The model should be assembled with WithModelID so it
// receives its own model id (ADR-0014 §2); the loop sets the placeholder Model
// that the decorator overrides on a copy.
func NewAgentBrain(m model.Model, tools tool.Registry, opts ...AgentOption) *AgentBrain {
	a := &AgentBrain{
		model:    m,
		tools:    tools,
		maxIters: defaultMaxIterations,
		fallback: defaultFallback,
		logger:   slog.Default(),
		metrics:  metrics.Nop{},
		now:      time.Now,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Handle implements Brain: seed the conversation with the protocol system prompt
// + history + the user message, then run the bounded tool-use loop until the model
// answers, the iteration cap is hit, the model call fails, or ctx is done.
//
// Error contract (mirrors ADR-0014 §3): a model-call failure, the iteration cap,
// or a cancelled ctx degrade to the fallback reply (logged), NOT a propagated
// error — the user never sees silence. Nothing to ask → clean (nil, nil).
func (a *AgentBrain) Handle(ctx context.Context, env *envelope.Envelope) ([]*envelope.Envelope, error) {
	key, history := a.loadHistory(ctx, env)

	// The seed system message carries the protocol grammar + tool catalog, then
	// the operator prompt (ADR-0021 §3.1). req.Messages is the LOOP's local
	// scratch slice — it grows with assistant:TOOL + user:OBSERVATION turns and is
	// NEVER persisted (§5, §6).
	sysPrompt := buildSystemPrompt(a.tools, a.systemPrompt)
	req, ok := requestWithHistory(env, sysPrompt, history)
	if !ok {
		return nil, nil // nothing to ask — clean no-reply (ADR-0014 §5)
	}

	a.metrics.IncMessages(env.Channel)
	userText := latestText(env.Parts)

	finalText, answered := a.runLoop(ctx, env, req)
	if !answered {
		// Iteration cap, model failure, or ctx done: a normal product outcome.
		// The user gets a fallback reply; the (canned) fallback is NOT persisted.
		return decisionToEnvelopes(a.fallback, env), nil
	}

	// Persist the FINAL user+assistant pair only (§6) — the tool-use trace stays
	// in the loop's local req.Messages and is discarded.
	a.persistPair(ctx, key, userText, finalText)
	return decisionToEnvelopes(finalText, env), nil
}

// runLoop runs the bounded model→tool→model loop. It returns the final answer and
// true, or "" and false when no answer was produced (cap hit, model failure, or
// ctx done). req is the loop's local scratch; runLoop mutates only req.Messages.
func (a *AgentBrain) runLoop(ctx context.Context, env *envelope.Envelope, req *model.Request) (string, bool) {
	for iter := 0; iter < a.maxIters; iter++ {
		if err := ctx.Err(); err != nil {
			// Total timeout / cancellation between steps (the Handle ctx is the
			// router's deadline; no separate knob — ADR-0021 §2).
			a.logger.Warn("agent: context done mid-loop",
				"envelope_id", env.ID, "channel", env.Channel, "iter", iter, "cause", err)
			return "", false
		}

		out := fanout.CallOne(ctx, req, a.model, a.perModelCall, a.now)
		a.metrics.ObserveProviderDuration(out.Provider, out.Err == nil, out.Latency)
		if out.Err != nil {
			// A model-call failure aborts the loop → fallback (ADR-0014 §3).
			// Distinct from a TOOL failure, which is an OBSERVATION (below).
			a.metrics.IncProviderFailure(out.Provider)
			a.logger.Warn("agent: model call failed",
				"envelope_id", env.ID, "channel", env.Channel, "iter", iter, "cause", out.Err)
			return "", false
		}

		content := out.Response.Message.Content
		name, args, isToolCall := parseReply(content)
		if !isToolCall {
			return content, true // final answer
		}

		// Tool call: execute, then feed the model its own request turn + the
		// result as an OBSERVATION. Appending to req.Messages keeps the trace
		// LOCAL to this Handle (§5).
		observation := a.runTool(ctx, name, args)
		req.Messages = append(req.Messages,
			model.Message{Role: model.RoleAssistant, Content: content},
			model.Message{Role: model.RoleUser, Content: observationPrefix + observation},
		)
	}

	// Cap reached with no final answer.
	a.logger.Warn("agent: iteration cap reached without an answer",
		"envelope_id", env.ID, "channel", env.Channel, "max_iters", a.maxIters)
	return "", false
}

// runTool executes the named tool and returns the OBSERVATION body. A tool error
// or an unknown tool is NOT fatal: it is returned as an observation string so the
// model can react (ADR-0021 §2). The per-tool timeout (if set) bounds Execute so a
// hung tool cannot stall the loop.
func (a *AgentBrain) runTool(ctx context.Context, name, args string) string {
	t, ok := a.tools[name]
	if !ok {
		return fmt.Sprintf("tool %q not found", name)
	}
	toolCtx := ctx
	if a.perTool > 0 {
		var cancel context.CancelFunc
		toolCtx, cancel = context.WithTimeout(ctx, a.perTool)
		defer cancel()
	}
	result, err := t.Execute(toolCtx, args)
	if err != nil {
		return fmt.Sprintf("tool %s failed: %v", name, err)
	}
	return result
}

// loadHistory derives the conversation key and loads recent turns when a store is
// configured and the envelope carries a conversation id. Memory is an enhancement,
// never a hard dependency: a missing key or a load error degrades to a stateless
// answer (logged), never dropping the reply.
//
// NOTE: this mirrors Orchestrator.loadHistory by design. AgentBrain keeps its own
// copy rather than mutating the Orchestrator (this cut adds alongside, it does not
// refactor the sibling — ADR-0021 §1); unifying the two is a deferred DRY pass.
func (a *AgentBrain) loadHistory(ctx context.Context, env *envelope.Envelope) (conversation.Key, []conversation.Turn) {
	if a.store == nil {
		return "", nil
	}
	key, err := conversation.KeyFromEnvelope(env)
	if err != nil {
		a.logger.Warn("agent: no conversation key, answering without memory",
			"envelope_id", env.ID, "channel", env.Channel, "cause", err)
		return "", nil
	}
	history, err := a.store.LoadRecent(ctx, key, a.historyN)
	if err != nil {
		a.logger.Warn("agent: load history failed, answering without memory",
			"envelope_id", env.ID, "channel", env.Channel, "cause", err)
		return key, nil
	}
	return key, history
}

// persistPair appends the FINAL user turn + assistant turn as ONE atomic group
// (ADR-0018), on a cancellation-detached context bounded by persistTimeout so the
// turn survives a graceful shutdown (ADR-0019 §6). It is a no-op when key is empty
// (no store / no conversation id). The intermediate tool-use trace is NOT passed
// here — only the final pair (ADR-0021 §6). Mirrors Orchestrator.persistTurns by
// design (see loadHistory note).
func (a *AgentBrain) persistPair(ctx context.Context, key conversation.Key, userText, assistantText string) {
	if a.store == nil || key == "" {
		return
	}
	now := a.now()
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
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), persistTimeout)
	defer cancel()
	if _, err := a.store.AppendTurns(persistCtx, key, turns...); err != nil {
		a.logger.Warn("agent: append turns failed", "key", string(key), "cause", err)
		return
	}
	a.metrics.ObserveTurnsPersisted(len(turns))
}
