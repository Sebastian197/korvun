// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Sebastian197/korvun/internal/bus"
	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/metrics"
	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/fanout"
	"github.com/Sebastian197/korvun/internal/policy"
	"github.com/Sebastian197/korvun/internal/tool"
)

// Compile-time assertion that *AgentBrain satisfies the Brain seam (ADR-0021 §1).
var _ Brain = (*AgentBrain)(nil)

// DefaultAgentMaxIterations is the hard loop cap when the operator does not set one
// (ADR-0021 §2): an unbounded model→tool→model loop is an infinite loop burning
// cloud quota, so SOME cap is mandatory. Exported so the app's router-ceiling
// derivation can size the agent shape from the same source of truth (ADR-0031).
const DefaultAgentMaxIterations = 5

// observationPrefix marks a tool result fed back to the model (ADR-0021 §3.3).
// The parser never treats a line starting with it as a tool call; it rides as a
// user message because model.Role has no Tool role (ADR-0009).
const observationPrefix = "OBSERVATION: "

// nativeBaseInstruction is the native lane's ALWAYS-present system base
// (ADR-0042 §5, hardened after the 2026-08-09 round): tools are for when the
// request needs them, never a reflex, and tool-call syntax is never prose.
const nativeBaseInstruction = "You are a helpful assistant. Use the available tools " +
	"only when the user's request needs them; for greetings or ordinary conversation, " +
	"reply normally in the user's language without any tool. If the user asks for a " +
	"tool or action that is not in your available tools, tell them plainly it is not " +
	"available here — never invent the call. Never print tool-call " +
	"syntax or JSON as text in your reply."

// maxArgsLogRunes bounds the tool-args prefix recorded in LOCAL slog lines
// (ADR-0041 §5): args derive from the message — content — so the bounded
// debugging prefix lives ONLY in slog, never on the bus, the Activity feed,
// or a metric label (the ADR-0024 §1 metadata-only law).
const maxArgsLogRunes = 80

// shadowObservation is the honest simulation observation a shadowed tool call
// feeds back to the model (ADR-0041 §2; hardened 2026-08-09 after the live
// demo, where a small model read the softer text as a failure and offered to
// "do it manually"): unmistakably a rehearsal, never an error, and the
// manual-offer failure mode is forbidden explicitly.
func shadowObservation(name string) string {
	return fmt.Sprintf("REHEARSAL: the tool %s is in shadow mode. It was NOT executed by design "+
		"(not an error, not a failure). Tell the user plainly that the action was "+
		"simulated, not performed, and do NOT offer to do it manually.", name)
}

// deniedObservation is the observation a gate-denied tool call feeds back.
// Deliberately rule-silent toward the MODEL (ADR-0041 §2): the rule dimension
// goes to the audit surfaces, not into the conversation.
func deniedObservation(name string) string {
	return fmt.Sprintf("tool %s is not permitted here", name)
}

// boundedArgs truncates args to maxArgsLogRunes runes for slog (never bytes —
// a multibyte rune must not be split into invalid UTF-8).
func boundedArgs(args string) string {
	runes := []rune(args)
	if len(runes) <= maxArgsLogRunes {
		return args
	}
	return string(runes[:maxArgsLogRunes])
}

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
	// personaPrefix is the composed persona fragment (ComposePersona) prepended
	// BEFORE the protocol block in the seed system message (builder-canvas spec
	// FR-PERSONA-2, NC-4). Empty = today's prompt byte-for-byte.
	personaPrefix string
	logger        *slog.Logger
	metrics       metrics.Metrics
	// store is the optional, conversation-keyed memory (ADR-0018). It persists the
	// FINAL user+assistant pair only — never the tool-use trace (§6). nil =
	// stateless. historyN is how many prior turns to load.
	store    conversation.Store
	historyN int
	// now is the clock seam (fanout.CallOne latency + persisted turn timestamps).
	now func() time.Time
	// governance, when non-nil, is the policy gatekeeper over the tool
	// registry (ADR-0041 §2): SelectTools runs once per Handle (the channel
	// is per-message) and its decisions gate BOTH advertisement and
	// execution. nil = today's behavior byte-for-byte (spec AS-4).
	governance *AgentGovernance
	// audit is the optional tool-audit sink (ADR-0041 §5): every tool
	// execution, denial, and shadow rehearsal publishes one metadata-only
	// bus event. nil disables publishing at zero cost (the router's
	// EventPublisher pattern). brainName labels the events.
	audit     ToolEventPublisher
	brainName string
	// skillsBlock is the pre-composed skills section (skill.PromptBlock
	// output, ADR-0041 §6) APPENDED after the protocol block + operator
	// prompt in the seed system message — added, never reordered (the
	// persona precedent, on the other end). Empty = today's prompt
	// byte-for-byte.
	skillsBlock string
}

// ToolEventPublisher is the narrow, best-effort audit sink the agent publishes
// tool events to (ADR-0041 §5) — the same publish-side-only view of the bus
// the router's EventPublisher carries, so the brain does not hard-depend on a
// concrete bus and tests can use a fake. Publishing MUST be non-blocking.
type ToolEventPublisher interface {
	Publish(ctx context.Context, ev bus.Event)
}

// AgentGovernance bundles the declared inputs of policy.SelectTools for one
// brain (ADR-0041 §1): the per-brain grants, the declared tool attributes,
// and the two per-brain facts the filter routes on — the brain's Sensitivity
// and its single model's Locality (both known at wiring time). Read-only
// after construction, so it is safe to share across the router's N workers.
type AgentGovernance struct {
	// Grants are the per-brain tri-state tool grants.
	Grants []policy.ToolGrant
	// Attrs are the declared per-tool attributes (house default + operator
	// override); a tool absent from the map gets zero attrs.
	Attrs map[string]policy.ToolAttrs
	// Sensitivity is the brain's declared tier (ADR-0015).
	Sensitivity policy.Sensitivity
	// Locality is where the brain's single model runs (ADR-0015).
	Locality policy.Locality
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

// WithAgentPersona sets the composed persona fragment (ComposePersona output)
// prepended as a PREFIX before the ADR-0021 protocol block in the seed system
// message, separated by one blank line (builder-canvas spec FR-PERSONA-2,
// NC-4 resolved). The protocol block itself is untouched — grammar, catalog
// and operator prompt keep their §3.1 internal order. An empty prefix leaves
// the prompt byte-identical to today.
func WithAgentPersona(prefix string) AgentOption {
	return func(a *AgentBrain) { a.personaPrefix = prefix }
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
//
// NOT wired in production since ADR-0031 sub-phase 4: the retry decorator
// (internal/model/retry) now owns the per-attempt deadline for the agent's model
// calls too — a single owner (SV3). The app no longer passes this option; it is
// retained for direct construction and tests (mirrors the adapters'
// WithRequestTimeout, kept-but-unwired per Decision 2).
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

// WithAgentSkillsBlock sets the pre-composed skills section (ADR-0041 §6,
// R-4) appended AFTER the ADR-0021 §3.1 protocol block and the operator
// prompt — the §3.1 internal order (grammar, catalog, operator) is untouched,
// mirroring how the persona rides as a prefix. Composition (budget, omission
// warnings) is the caller's: brain receives the finished block. An empty
// block leaves the prompt byte-identical to today.
func WithAgentSkillsBlock(block string) AgentOption {
	return func(a *AgentBrain) { a.skillsBlock = block }
}

// WithAgentToolAudit mounts the tool-audit sink and the brain name its events
// carry (ADR-0041 §5). A nil publisher is ignored, leaving auditing off — the
// same optionality metrics.Nop carries. The events are METADATA-ONLY by
// construction (bus.Event has no args field); the bounded args prefix lives
// exclusively in this brain's slog lines.
func WithAgentToolAudit(p ToolEventPublisher, brainName string) AgentOption {
	return func(a *AgentBrain) {
		if p != nil {
			a.audit = p
			a.brainName = brainName
		}
	}
}

// WithAgentGovernance mounts the policy gatekeeper over the tool registry
// (ADR-0041 §2). A nil argument is ignored, leaving the ungoverned default
// (every registered tool advertised and executable on every channel — spec
// AS-4). The governance value MUST NOT be mutated after construction.
func WithAgentGovernance(g *AgentGovernance) AgentOption {
	return func(a *AgentBrain) {
		if g != nil {
			a.governance = g
		}
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
		maxIters: DefaultAgentMaxIterations,
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

	// The gatekeeper decides once per Handle which tools this message may
	// see and run (ADR-0041 §2): the ADVERTISED registry feeds the system
	// prompt below, the decisions gate execution in runTool. Both are nil
	// checks away from today's behavior when no governance is mounted.
	advertised, decisions := a.effectiveTools(env)

	// Lane pick (ADR-0042 §5): a model with the native capability gets the
	// structured lane — no textual grammar, tools as specs; anything else
	// keeps today's prompt-protocol byte-for-byte.
	tcm, native := a.model.(model.ToolCallingModel)

	// The seed system message: the prompt-protocol lane carries the grammar
	// + tool catalog (ADR-0021 §3.1); the native lane drops both (the specs
	// replace them) and keeps only the operator prompt. Persona (prefix) and
	// skills (suffix) ride identically on both lanes. req.Messages is the
	// LOOP's local scratch — it grows with the lane's turn shapes and is
	// NEVER persisted (§5, §6).
	var sysPrompt string
	if native {
		// The native lane ALWAYS seeds a base instruction (2026-08-09 round
		// catch: with an empty operator prompt, a small model greeted users
		// with raw tool-call JSON). The operator prompt, when set, follows it.
		sysPrompt = nativeBaseInstruction
		if a.systemPrompt != "" {
			sysPrompt = sysPrompt + "\n\n" + a.systemPrompt
		}
	} else {
		sysPrompt = buildSystemPrompt(advertised, a.systemPrompt)
	}
	if a.personaPrefix != "" {
		// Persona rides as a PREFIX (FR-PERSONA-2).
		sysPrompt = strings.TrimSpace(a.personaPrefix + "\n\n" + sysPrompt)
	}
	if a.skillsBlock != "" {
		// Skills ride as a SUFFIX (ADR-0041 §6).
		sysPrompt = strings.TrimSpace(sysPrompt + "\n\n" + a.skillsBlock)
	}
	req, ok := requestWithHistory(env, sysPrompt, history)
	if !ok {
		return nil, nil // nothing to ask — clean no-reply (ADR-0014 §5)
	}

	a.metrics.IncMessages(env.Channel)
	// TranscriptText, not latestText: attachments persist their honest
	// markers alongside the caption (FR-ATTACH); the request keeps the
	// plain text, the history tells the whole truth.
	userText := envelope.TranscriptText(env.Parts)

	var finalText string
	var answered bool
	if native {
		finalText, answered = a.runLoopNative(ctx, env, req, tcm, advertised, decisions)
	} else {
		finalText, answered = a.runLoop(ctx, env, req, decisions)
	}
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
// decisions are this message's gate outcomes (nil = ungoverned, ADR-0041 §2).
func (a *AgentBrain) runLoop(ctx context.Context, env *envelope.Envelope, req *model.Request, decisions map[string]policy.ToolDecision) (string, bool) {
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
			if strings.TrimSpace(content) == "" {
				// An empty (non-error) model reply is not a usable answer:
				// shipping it would send the user a blank message and persist a
				// user-only turn (asymmetric memory). Degrade to the fallback,
				// the same as a model-call failure.
				a.logger.Warn("agent: empty model reply, no answer",
					"envelope_id", env.ID, "channel", env.Channel, "iter", iter)
				return "", false
			}
			return content, true // final answer
		}

		// Tool call: execute, then feed the model its own request turn + the
		// result as an OBSERVATION. Appending to req.Messages keeps the trace
		// LOCAL to this Handle (§5).
		observation := a.runTool(ctx, env, decisions, name, args)
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
//
// Under governance (decisions non-nil, ADR-0041 §2) this is the EXECUTION half
// of the two-point gate: nonexistence is checked first (honesty about a tool
// that is not there beats governance theater), then the decision — a shadowed
// call is NEVER executed and feeds the simulation observation back; a denied or
// undecided call feeds the denial observation. The bounded args prefix goes to
// LOCAL slog only (ADR-0024 §1 law).
func (a *AgentBrain) runTool(ctx context.Context, env *envelope.Envelope, decisions map[string]policy.ToolDecision, name, args string) string {
	t, ok := a.tools[name]
	if !ok {
		// A hallucinated tool name is exactly the behavior the audit
		// surfaces exist to observe (estreno E-3 / red-team): a denial with
		// its own rule, on the same grammar the cages emit.
		a.logger.Warn("agent: tool denied",
			"envelope_id", env.ID, "channel", env.Channel, "tool", name,
			"rule", "unknown_tool", "args_prefix", boundedArgs(args))
		a.auditTool(ctx, env, bus.Event{Type: bus.ToolDenied, Tool: name, Outcome: "denied", Rule: "unknown_tool"})
		return fmt.Sprintf("tool %q not found", name)
	}
	if decisions != nil {
		d, decided := decisions[name]
		switch {
		case decided && d.Mode == policy.ToolShadow:
			a.logger.Info("agent: tool shadowed",
				"envelope_id", env.ID, "channel", env.Channel, "tool", name,
				"args_prefix", boundedArgs(args))
			a.auditTool(ctx, env, bus.Event{Type: bus.ToolShadowed, Tool: name, Outcome: "shadowed"})
			return shadowObservation(name)
		case !decided || d.Mode != policy.ToolAllow:
			rule := policy.ToolRuleNotGranted
			if decided {
				rule = d.Rule
			}
			a.logger.Warn("agent: tool denied",
				"envelope_id", env.ID, "channel", env.Channel, "tool", name,
				"rule", string(rule), "args_prefix", boundedArgs(args))
			a.auditTool(ctx, env, bus.Event{Type: bus.ToolDenied, Tool: name, Outcome: "denied", Rule: string(rule)})
			return deniedObservation(name)
		}
	}
	toolCtx := ctx
	if a.perTool > 0 {
		var cancel context.CancelFunc
		toolCtx, cancel = context.WithTimeout(ctx, a.perTool)
		defer cancel()
	}
	start := a.now()
	result, err := t.Execute(toolCtx, args)
	latency := a.now().Sub(start)

	// A cage/shield breach is a DENIAL, not an executed-with-error use
	// (ADR-0041 §4/§5): the tool refused before any effect. The model still
	// receives the tool's honest error observation below; only the audit
	// classification changes.
	if rule, breached := cageRule(err); breached {
		a.logger.Warn("agent: tool denied by its cage",
			"envelope_id", env.ID, "channel", env.Channel, "tool", name,
			"rule", rule, "args_prefix", boundedArgs(args))
		a.auditTool(ctx, env, bus.Event{Type: bus.ToolDenied, Tool: name, Outcome: "denied", Rule: rule})
		return fmt.Sprintf("tool %s failed: %v", name, err)
	}

	outcome := "ok"
	if err != nil {
		outcome = "error"
	}
	a.logger.Info("agent: tool used",
		"envelope_id", env.ID, "channel", env.Channel, "tool", name,
		"outcome", outcome, "latency", latency, "args_prefix", boundedArgs(args))
	a.auditTool(ctx, env, bus.Event{Type: bus.ToolUsed, Tool: name, Outcome: outcome, Latency: latency})
	if err != nil {
		return fmt.Sprintf("tool %s failed: %v", name, err)
	}
	return result
}

// cageRule classifies a tool error as a cage or shield denial (ADR-0041 §5).
// The shield sentinel is checked FIRST: a shield stop may ride inside a cage
// wrapper and its rule is the more specific fact.
func cageRule(err error) (string, bool) {
	switch {
	case err == nil:
		return "", false
	case errors.Is(err, tool.ErrShieldViolation):
		return string(policy.ToolRulePrivateShield), true
	case errors.Is(err, tool.ErrCageViolation):
		return string(policy.ToolRuleCage), true
	default:
		return "", false
	}
}

// auditTool completes ev with the routing metadata and pushes it to the metric
// and (when mounted) the bus sink (ADR-0041 §5). Best-effort and non-blocking
// by the publisher's contract; the metric always records so an operator
// without observability wiring still gets counters through metrics.Metrics.
func (a *AgentBrain) auditTool(ctx context.Context, env *envelope.Envelope, ev bus.Event) {
	a.metrics.ObserveToolUse(ev.Tool, ev.Outcome, ev.Latency)
	if a.audit == nil {
		return
	}
	ev.Envelope = env
	ev.Channel = env.Channel
	ev.Brain = a.brainName
	a.audit.Publish(ctx, ev)
}

// effectiveTools resolves this message's gate (ADR-0041 §2): with no
// governance it returns the full registry and nil decisions (today's behavior
// byte-for-byte). With governance it runs policy.SelectTools once and returns
// the ADVERTISED registry (ToolAllow ∪ ToolShadow — shadow is announced so
// the rehearsal observes the model's real judgment) plus the decisions that
// gate execution. A misconfigured governance FAILS CLOSED (spec D-6): empty
// advertisement, non-nil empty decisions so every call is denied, logged — a
// gatekeeper that fails open is not a gatekeeper.
func (a *AgentBrain) effectiveTools(env *envelope.Envelope) (tool.Registry, map[string]policy.ToolDecision) {
	if a.governance == nil {
		return a.tools, nil
	}
	g := a.governance
	decisions, err := policy.SelectTools(g.Grants, g.Attrs, policy.ToolQuery{
		Channel:     env.Channel,
		Sensitivity: g.Sensitivity,
		Locality:    g.Locality,
	})
	if err != nil {
		a.logger.Error("agent: governance misconfigured, failing closed (deny-all)",
			"envelope_id", env.ID, "channel", env.Channel, "cause", err)
		return tool.Registry{}, map[string]policy.ToolDecision{}
	}
	advertised := make(tool.Registry, len(a.tools))
	for name, t := range a.tools {
		if d, ok := decisions[name]; ok && (d.Mode == policy.ToolAllow || d.Mode == policy.ToolShadow) {
			advertised[name] = t
		}
	}
	return advertised, decisions
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
