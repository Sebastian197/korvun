// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/tool"
)

// Compile-time assertion that *AgentBrain satisfies the Brain seam — the same
// seam the Orchestrator implements, so the router and cmd/korvun mount either one
// agnostically (ADR-0021 §1).
var _ Brain = (*AgentBrain)(nil)

// scriptedModel returns a fixed sequence of replies, one per Generate call, and
// records the request it last saw. A mutex makes it safe to share, though the
// sequenced tests use it single-threaded.
type scriptedModel struct {
	name    string
	replies []string
	mu      sync.Mutex
	calls   int
	lastReq *model.Request
}

func (m *scriptedModel) Generate(_ context.Context, req *model.Request) (*model.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastReq = req
	i := m.calls
	m.calls++
	if i >= len(m.replies) {
		i = len(m.replies) - 1 // repeat the last reply if over-called
	}
	return &model.Response{
		Message:  model.Message{Role: model.RoleAssistant, Content: m.replies[i]},
		Provider: m.name,
	}, nil
}

func (m *scriptedModel) Name() string { return m.name }

// builtinRegistry is the seam-validation toolset.
func builtinRegistry() tool.Registry {
	return tool.Registry{"calc": tool.Calc(), "echo": tool.Echo(), "time": tool.Time(nil)}
}

func TestAgentBrain_Handle_finalAnswerNoTool(t *testing.T) {
	t.Parallel()
	m := &scriptedModel{name: "m", replies: []string{"The answer is 42."}}
	a := NewAgentBrain(m, builtinRegistry(), WithAgentLogger(quietLogger()))

	out, err := a.Handle(context.Background(), inboundText("telegram", "c", "hello"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(out) != 1 || out[0].Parts[0].Content != "The answer is 42." {
		t.Fatalf("got %+v, want the model's direct answer", out)
	}
	if m.calls != 1 {
		t.Fatalf("model called %d times, want 1 (no tool round-trip)", m.calls)
	}
}

func TestAgentBrain_Handle_singleToolThenAnswer(t *testing.T) {
	t.Parallel()
	m := &scriptedModel{name: "m", replies: []string{"TOOL: calc(2+2)", "The answer is 4."}}
	a := NewAgentBrain(m, builtinRegistry(), WithAgentLogger(quietLogger()))

	out, err := a.Handle(context.Background(), inboundText("telegram", "c", "what is 2+2?"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(out) != 1 || out[0].Parts[0].Content != "The answer is 4." {
		t.Fatalf("got %+v, want the post-tool answer", out)
	}
	// The second request must carry the OBSERVATION with the calc result.
	if !requestHasObservation(m.lastReq, "OBSERVATION: 4") {
		t.Fatalf("second request missing the calc observation: %+v", m.lastReq)
	}
}

func TestAgentBrain_Handle_maxIterations(t *testing.T) {
	t.Parallel()
	// The model never gives a final answer — always asks for a tool.
	m := &scriptedModel{name: "m", replies: []string{"TOOL: echo(again)"}}
	a := NewAgentBrain(m, builtinRegistry(),
		WithAgentLogger(quietLogger()), WithAgentMaxIterations(3))

	out, err := a.Handle(context.Background(), inboundText("telegram", "c", "loop"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(out) != 1 || out[0].Parts[0].Content != defaultFallback {
		t.Fatalf("got %+v, want the fallback after the iteration cap", out)
	}
	if m.calls != 3 {
		t.Fatalf("model called %d times, want exactly maxIters=3", m.calls)
	}
}

func TestAgentBrain_Handle_toolFailureIsObservation(t *testing.T) {
	t.Parallel()
	// calc(1/0) fails; the failure must come back as an OBSERVATION and the loop
	// must continue to a final answer (ADR-0021 §2), not abort.
	m := &scriptedModel{name: "m", replies: []string{"TOOL: calc(1/0)", "Could not divide."}}
	a := NewAgentBrain(m, builtinRegistry(), WithAgentLogger(quietLogger()))

	out, err := a.Handle(context.Background(), inboundText("telegram", "c", "1/0?"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(out) != 1 || out[0].Parts[0].Content != "Could not divide." {
		t.Fatalf("got %+v, want the loop to continue past a tool failure", out)
	}
	if !requestHasObservationContaining(m.lastReq, "failed") {
		t.Fatalf("tool failure not surfaced as an OBSERVATION: %+v", m.lastReq)
	}
}

func TestAgentBrain_Handle_unknownTool(t *testing.T) {
	t.Parallel()
	m := &scriptedModel{name: "m", replies: []string{"TOOL: nope(x)", "Done."}}
	a := NewAgentBrain(m, builtinRegistry(), WithAgentLogger(quietLogger()))

	out, err := a.Handle(context.Background(), inboundText("telegram", "c", "go"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(out) != 1 || out[0].Parts[0].Content != "Done." {
		t.Fatalf("got %+v, want the loop to continue past an unknown tool", out)
	}
	if !requestHasObservationContaining(m.lastReq, "not found") {
		t.Fatalf("unknown tool not surfaced as an OBSERVATION: %+v", m.lastReq)
	}
}

func TestAgentBrain_Handle_modelFailure(t *testing.T) {
	t.Parallel()
	m := &recordingModel{name: "m", err: model.ErrProviderUnavailable}
	a := NewAgentBrain(m, builtinRegistry(), WithAgentLogger(quietLogger()))

	out, err := a.Handle(context.Background(), inboundText("telegram", "c", "hi"))
	if err != nil {
		t.Fatalf("a model failure must degrade to a fallback, not propagate: %v", err)
	}
	if len(out) != 1 || out[0].Parts[0].Content != defaultFallback {
		t.Fatalf("got %+v, want the fallback on model failure", out)
	}
}

func TestAgentBrain_Handle_nothingToAsk(t *testing.T) {
	t.Parallel()
	m := &scriptedModel{name: "m", replies: []string{"unused"}}
	a := NewAgentBrain(m, builtinRegistry(), WithAgentLogger(quietLogger()))

	// An envelope with no text (e.g. a reaction) yields a clean no-reply.
	env := inboundText("telegram", "c", "   ")
	out, err := a.Handle(context.Background(), env)
	if err != nil || out != nil {
		t.Fatalf("got (%+v, %v), want a clean no-reply (nil, nil)", out, err)
	}
	if m.calls != 0 {
		t.Fatalf("model called %d times, want 0 (nothing to ask)", m.calls)
	}
}

// TestAgentBrain_Handle_persistsOnlyFinalPair is copilot refinement #3: the loop
// message slice (assistant:TOOL + user:OBSERVATION, which grows across tool calls)
// is LOCAL to Handle and is NEVER mixed into persistence. After a Handle with TWO
// tool calls, the ConversationStore must hold EXACTLY two turns — the final user
// message and the final assistant answer — NOT the tool-use trace (ADR-0021 §6).
func TestAgentBrain_Handle_persistsOnlyFinalPair(t *testing.T) {
	t.Parallel()
	store := conversation.NewMemStore()
	m := &scriptedModel{name: "m", replies: []string{
		"TOOL: calc(2+2)",   // tool call 1
		"TOOL: echo(hi)",    // tool call 2
		"The final answer.", // final
	}}
	a := NewAgentBrain(m, builtinRegistry(),
		WithAgentLogger(quietLogger()),
		WithAgentConversationStore(store, 10))

	env := inboundConv("telegram", "chat-42", "compute please")
	if _, err := a.Handle(context.Background(), env); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	key := conversation.Key("telegram::chat-42")
	turns, err := store.LoadRecent(context.Background(), key, 100)
	if err != nil {
		t.Fatalf("LoadRecent: %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("store has %d turns, want EXACTLY 2 (final pair only, not the %d-step trace):\n%+v",
			len(turns), m.calls, turns)
	}
	if turns[0].Role != conversation.RoleUser || turns[0].Content != "compute please" {
		t.Fatalf("turn[0] = %+v, want the final user message", turns[0])
	}
	if turns[1].Role != conversation.RoleAssistant || turns[1].Content != "The final answer." {
		t.Fatalf("turn[1] = %+v, want the final assistant answer", turns[1])
	}
}

// statefulTool is a counter guarded by a mutex — the concurrency-contract probe
// (ADR-0021 §4): N workers calling ONE shared instance must not race.
type statefulTool struct {
	mu sync.Mutex
	n  int
}

func (s *statefulTool) Name() string        { return "count" }
func (s *statefulTool) Description() string { return "increments and returns a shared counter." }
func (s *statefulTool) Execute(_ context.Context, _ string) (string, error) {
	s.mu.Lock()
	s.n++
	v := s.n
	s.mu.Unlock()
	return strconv.Itoa(v), nil
}

// loopModel asks for the tool until it sees an OBSERVATION, then answers. It is
// stateless (reads only the request), so it is safe under concurrent Generate and
// deterministic regardless of interleaving — exactly what the -race test needs.
type loopModel struct{ name string }

func (m *loopModel) Name() string { return m.name }

func (m *loopModel) Generate(_ context.Context, req *model.Request) (*model.Response, error) {
	for _, msg := range req.Messages {
		if strings.HasPrefix(msg.Content, "OBSERVATION:") {
			return &model.Response{
				Message:  model.Message{Role: model.RoleAssistant, Content: "counted."},
				Provider: m.name,
			}, nil
		}
	}
	return &model.Response{
		Message:  model.Message{Role: model.RoleAssistant, Content: "TOOL: count()"},
		Provider: m.name,
	}, nil
}

// TestAgentBrain_Handle_concurrent_race is the LOAD-BEARING test (ADR-0021 §5):
// many goroutines call Handle on ONE shared AgentBrain whose registry holds a
// STATEFUL tool. It proves (under -race -count) that the Tool concurrency contract
// holds and that the per-Handle loop state (the growing message slice) never leaks
// between workers — the "intersection of two features" bug class the project has
// hit twice. Run: go test -race -run Concurrent -count=4 ./internal/brain/
func TestAgentBrain_Handle_concurrent_race(t *testing.T) {
	t.Parallel()
	counter := &statefulTool{}
	reg := tool.Registry{"count": counter}
	a := NewAgentBrain(&loopModel{name: "m"}, reg, WithAgentLogger(quietLogger()))

	const workers = 16
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			out, err := a.Handle(context.Background(),
				inboundText("telegram", "chat-"+strconv.Itoa(i), "go"))
			if err != nil {
				t.Errorf("worker %d: %v", i, err)
				return
			}
			if len(out) != 1 || out[0].Parts[0].Content != "counted." {
				t.Errorf("worker %d got %+v, want the post-tool answer", i, out)
			}
		}(i)
	}
	wg.Wait()

	// Each of the N Handles ran the tool exactly once; the shared counter saw all N.
	if counter.n != workers {
		t.Fatalf("counter = %d, want %d (every Handle invoked the shared tool once)", counter.n, workers)
	}
}

func requestHasObservation(req *model.Request, want string) bool {
	if req == nil {
		return false
	}
	for _, m := range req.Messages {
		if m.Content == want {
			return true
		}
	}
	return false
}

func requestHasObservationContaining(req *model.Request, substr string) bool {
	if req == nil {
		return false
	}
	for _, m := range req.Messages {
		if strings.HasPrefix(m.Content, "OBSERVATION:") && strings.Contains(m.Content, substr) {
			return true
		}
	}
	return false
}
