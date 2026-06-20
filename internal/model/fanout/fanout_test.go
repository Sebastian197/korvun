// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package fanout

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/model"
)

// --- test helpers -----------------------------------------------------------

// fakeModel is the universal test adapter. It is concurrency-safe for
// the access patterns the fan-out applies (one Run call → one
// Generate call per model — the Coordinator does not re-enter
// Generate on the same Model within a single Run). The few fields
// the tests inspect concurrently (callCount, lastCtx) use atomics
// and a mutex.
type fakeModel struct {
	name string

	// Behaviour switches.
	response    *model.Response
	err         error
	delay       time.Duration // sleep before responding; honours ctx
	panicOnGen  any           // if non-nil, Generate panics with this value
	panicOnName any           // if non-nil, Name panics with this value

	// Observation.
	callCount int32
	mu        sync.Mutex
	lastCtx   context.Context
}

func (f *fakeModel) Name() string {
	if f.panicOnName != nil {
		panic(f.panicOnName)
	}
	return f.name
}

func (f *fakeModel) Generate(ctx context.Context, req *model.Request) (*model.Response, error) {
	atomic.AddInt32(&f.callCount, 1)
	f.mu.Lock()
	f.lastCtx = ctx
	f.mu.Unlock()
	if f.panicOnGen != nil {
		panic(f.panicOnGen)
	}
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, fmt.Errorf("fake: %w (ctx cancel)", model.ErrProviderUnavailable)
		}
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.response, nil
}

func okResponse(provider, modelName, content string) *model.Response {
	return &model.Response{
		Message:   model.Message{Role: model.RoleAssistant, Content: content},
		Provider:  provider,
		ModelName: modelName,
	}
}

func validRequest() *model.Request {
	return &model.Request{
		Model:    "test-model",
		Messages: []model.Message{{Role: model.RoleUser, Content: "ping"}},
	}
}

// --- validation pre-spawn ---------------------------------------------------

func TestRun_rejectsNilCtx(t *testing.T) {
	c := New()
	res, err := c.Run(nil, validRequest(), []model.Model{&fakeModel{name: "x"}}) //nolint:staticcheck
	if err == nil || !strings.Contains(err.Error(), "nil ctx") {
		t.Errorf("Run nil ctx err = %v, want contains 'nil ctx'", err)
	}
	if res != nil {
		t.Errorf("Run nil ctx result = %v, want nil", res)
	}
}

func TestRun_rejectsNilRequest(t *testing.T) {
	c := New()
	_, err := c.Run(context.Background(), nil, []model.Model{&fakeModel{name: "x"}})
	if !errors.Is(err, model.ErrNilRequest) {
		t.Errorf("Run nil req err = %v, want ErrNilRequest", err)
	}
}

func TestRun_rejectsInvalidRequest(t *testing.T) {
	c := New()
	bad := &model.Request{Model: "", Messages: []model.Message{{Role: model.RoleUser, Content: "x"}}}
	_, err := c.Run(context.Background(), bad, []model.Model{&fakeModel{name: "x"}})
	if !errors.Is(err, model.ErrEmptyModel) {
		t.Errorf("Run invalid req err = %v, want ErrEmptyModel", err)
	}
}

func TestRun_rejectsEmptyModels(t *testing.T) {
	c := New()
	_, err := c.Run(context.Background(), validRequest(), nil)
	if !errors.Is(err, ErrNoModels) {
		t.Errorf("Run empty models err = %v, want ErrNoModels", err)
	}
	_, err = c.Run(context.Background(), validRequest(), []model.Model{})
	if !errors.Is(err, ErrNoModels) {
		t.Errorf("Run empty models slice err = %v, want ErrNoModels", err)
	}
}

func TestRun_rejectsNilModelEntry(t *testing.T) {
	c := New()
	_, err := c.Run(context.Background(), validRequest(),
		[]model.Model{&fakeModel{name: "ok"}, nil, &fakeModel{name: "ok2"}})
	if !errors.Is(err, ErrNilModel) {
		t.Errorf("Run nil model entry err = %v, want ErrNilModel", err)
	}
}

func TestRun_rejectsCanceledCtx(t *testing.T) {
	c := New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Run(ctx, validRequest(), []model.Model{&fakeModel{name: "x"}})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run canceled ctx err = %v, want context.Canceled", err)
	}
}

func TestRun_doesNotSpawnOnValidationFailure(t *testing.T) {
	// If validation rejects, no Generate must have been called.
	c := New()
	tripwire := &fakeModel{name: "tripwire", panicOnGen: "Generate should NOT be called on validation failure"}
	bad := &model.Request{Model: "", Messages: []model.Message{{Role: model.RoleUser, Content: "x"}}}
	_, err := c.Run(context.Background(), bad, []model.Model{tripwire})
	if !errors.Is(err, model.ErrEmptyModel) {
		t.Fatalf("expected ErrEmptyModel, got %v", err)
	}
	if atomic.LoadInt32(&tripwire.callCount) != 0 {
		t.Errorf("tripwire.Generate called %d times on validation failure, want 0", tripwire.callCount)
	}
}

// --- happy path -------------------------------------------------------------

func TestRun_singleModelHappyPath(t *testing.T) {
	c := New()
	f := &fakeModel{name: "alpha", response: okResponse("alpha", "alpha-1", "hello")}
	res, err := c.Run(context.Background(), validRequest(), []model.Model{f})
	if err != nil {
		t.Fatalf("Run err = %v, want nil", err)
	}
	if len(res.Outcomes) != 1 {
		t.Fatalf("len(Outcomes) = %d, want 1", len(res.Outcomes))
	}
	o := res.Outcomes[0]
	if o.Provider != "alpha" {
		t.Errorf("Provider = %q, want %q", o.Provider, "alpha")
	}
	if o.Err != nil {
		t.Errorf("Err = %v, want nil", o.Err)
	}
	if o.Response == nil || o.Response.Message.Content != "hello" {
		t.Errorf("Response = %+v, want content=hello", o.Response)
	}
}

func TestRun_twoModelsHappyPath(t *testing.T) {
	c := New()
	a := &fakeModel{name: "alpha", response: okResponse("alpha", "alpha-1", "A")}
	b := &fakeModel{name: "beta", response: okResponse("beta", "beta-1", "B")}
	res, err := c.Run(context.Background(), validRequest(), []model.Model{a, b})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if res.Outcomes[0].Provider != "alpha" || res.Outcomes[1].Provider != "beta" {
		t.Errorf("providers = %q,%q want alpha,beta",
			res.Outcomes[0].Provider, res.Outcomes[1].Provider)
	}
	if res.Outcomes[0].Response.Message.Content != "A" || res.Outcomes[1].Response.Message.Content != "B" {
		t.Errorf("content mismatch: %+v", res.Outcomes)
	}
}

func TestRun_orderDeterministicAcrossRuns(t *testing.T) {
	c := New()
	mkModels := func() []model.Model {
		return []model.Model{
			&fakeModel{name: "m0", response: okResponse("m0", "m0", "0"), delay: 5 * time.Millisecond},
			&fakeModel{name: "m1", response: okResponse("m1", "m1", "1"), delay: 1 * time.Millisecond},
			&fakeModel{name: "m2", response: okResponse("m2", "m2", "2"), delay: 3 * time.Millisecond},
			&fakeModel{name: "m3", response: okResponse("m3", "m3", "3")},
			&fakeModel{name: "m4", response: okResponse("m4", "m4", "4"), delay: 2 * time.Millisecond},
		}
	}
	want := []string{"m0", "m1", "m2", "m3", "m4"}
	for run := 0; run < 50; run++ {
		res, err := c.Run(context.Background(), validRequest(), mkModels())
		if err != nil {
			t.Fatalf("run %d err = %v", run, err)
		}
		got := make([]string, len(res.Outcomes))
		for i, o := range res.Outcomes {
			got[i] = o.Provider
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("run %d position %d = %q, want %q (full: %v)", run, i, got[i], want[i], got)
			}
		}
	}
}

// --- sentinel preservation --------------------------------------------------

func TestRun_preservesErrAuthInvalid(t *testing.T) {
	c := New()
	a := &fakeModel{name: "auth-broken", err: fmt.Errorf("upstream: %w", model.ErrAuthInvalid)}
	b := &fakeModel{name: "ok", response: okResponse("ok", "ok", "x")}
	res, _ := c.Run(context.Background(), validRequest(), []model.Model{a, b})
	if !errors.Is(res.Outcomes[0].Err, model.ErrAuthInvalid) {
		t.Errorf("Outcomes[0].Err = %v, want errors.Is ErrAuthInvalid", res.Outcomes[0].Err)
	}
	if res.Outcomes[1].Err != nil {
		t.Errorf("Outcomes[1].Err = %v, want nil", res.Outcomes[1].Err)
	}
}

func TestRun_preservesRateLimitErrorMetadata(t *testing.T) {
	c := New()
	rle := &model.RateLimitError{Provider: "groq", RetryAfter: 12 * time.Second}
	wrapped := fmt.Errorf("call: %w", rle)
	a := &fakeModel{name: "limited", err: wrapped}
	res, _ := c.Run(context.Background(), validRequest(), []model.Model{a})

	if !errors.Is(res.Outcomes[0].Err, model.ErrRateLimited) {
		t.Errorf("Outcomes[0].Err must satisfy errors.Is(ErrRateLimited), got %v", res.Outcomes[0].Err)
	}
	var got *model.RateLimitError
	if !errors.As(res.Outcomes[0].Err, &got) {
		t.Fatal("errors.As did not recover *RateLimitError through the fanout layer")
	}
	if got.Provider != "groq" || got.RetryAfter != 12*time.Second {
		t.Errorf("RateLimitError metadata lost: %+v", got)
	}
}

func TestRun_preservesErrProviderUnavailable(t *testing.T) {
	c := New()
	a := &fakeModel{name: "down", err: fmt.Errorf("net: %w", model.ErrProviderUnavailable)}
	res, _ := c.Run(context.Background(), validRequest(), []model.Model{a})
	if !errors.Is(res.Outcomes[0].Err, model.ErrProviderUnavailable) {
		t.Errorf("Outcomes[0].Err = %v, want errors.Is ErrProviderUnavailable", res.Outcomes[0].Err)
	}
}

func TestRun_preservesErrProviderResponse(t *testing.T) {
	c := New()
	a := &fakeModel{name: "bad-shape", err: fmt.Errorf("parse: %w", model.ErrProviderResponse)}
	res, _ := c.Run(context.Background(), validRequest(), []model.Model{a})
	if !errors.Is(res.Outcomes[0].Err, model.ErrProviderResponse) {
		t.Errorf("Outcomes[0].Err = %v, want errors.Is ErrProviderResponse", res.Outcomes[0].Err)
	}
}

func TestRun_mixedSentinelsResultReturned(t *testing.T) {
	c := New()
	models := []model.Model{
		&fakeModel{name: "auth", err: fmt.Errorf("a: %w", model.ErrAuthInvalid)},
		&fakeModel{name: "rate", err: &model.RateLimitError{Provider: "rate", RetryAfter: time.Second}},
		&fakeModel{name: "down", err: fmt.Errorf("d: %w", model.ErrProviderUnavailable)},
		&fakeModel{name: "shape", err: fmt.Errorf("s: %w", model.ErrProviderResponse)},
		&fakeModel{name: "ok", response: okResponse("ok", "ok", "yes")},
	}
	res, err := c.Run(context.Background(), validRequest(), models)
	if err != nil {
		t.Fatalf("Run-level err = %v, want nil (partial failure is not a Run error)", err)
	}
	if !errors.Is(res.Outcomes[0].Err, model.ErrAuthInvalid) {
		t.Errorf("[0] not ErrAuthInvalid: %v", res.Outcomes[0].Err)
	}
	if !errors.Is(res.Outcomes[1].Err, model.ErrRateLimited) {
		t.Errorf("[1] not ErrRateLimited: %v", res.Outcomes[1].Err)
	}
	if !errors.Is(res.Outcomes[2].Err, model.ErrProviderUnavailable) {
		t.Errorf("[2] not ErrProviderUnavailable: %v", res.Outcomes[2].Err)
	}
	if !errors.Is(res.Outcomes[3].Err, model.ErrProviderResponse) {
		t.Errorf("[3] not ErrProviderResponse: %v", res.Outcomes[3].Err)
	}
	if res.Outcomes[4].Err != nil || res.Outcomes[4].Response == nil {
		t.Errorf("[4] success path broken: %+v", res.Outcomes[4])
	}
}

// --- cancellation -----------------------------------------------------------

func TestRun_ctxCancelMidFlight(t *testing.T) {
	c := New()
	slow := &fakeModel{name: "slow", delay: 5 * time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	res, err := c.Run(ctx, validRequest(), []model.Model{slow})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Run err = %v, want nil (mechanism returned result with per-Outcome err)", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("Run took %v, expected to return quickly after ctx cancel", elapsed)
	}
	if res.Outcomes[0].Err == nil {
		t.Errorf("Outcome.Err = nil after ctx cancel, want non-nil")
	}
}

func TestRun_perModelTimeoutFires(t *testing.T) {
	c := New(WithPerModelTimeout(40 * time.Millisecond))
	slow := &fakeModel{name: "slow", delay: 5 * time.Second}
	fast := &fakeModel{name: "fast", response: okResponse("fast", "fast", "ok")}

	start := time.Now()
	res, err := c.Run(context.Background(), validRequest(), []model.Model{slow, fast})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if elapsed > 1*time.Second {
		t.Errorf("Run took %v, want bounded by per-model timeout", elapsed)
	}
	if res.Outcomes[0].Err == nil {
		t.Errorf("slow Outcome.Err = nil, want timeout-derived error")
	}
	if res.Outcomes[1].Err != nil {
		t.Errorf("fast Outcome.Err = %v, want nil", res.Outcomes[1].Err)
	}
}

func TestRun_perModelTimeoutZero_isNoop(t *testing.T) {
	c := New(WithPerModelTimeout(0))
	if c.perModelTimeout != 0 {
		t.Errorf("perModelTimeout after WithPerModelTimeout(0) = %v, want 0 (no-op)", c.perModelTimeout)
	}
}

func TestRun_perModelTimeoutNegative_isNoop(t *testing.T) {
	c := New(WithPerModelTimeout(-1 * time.Second))
	if c.perModelTimeout != 0 {
		t.Errorf("perModelTimeout after WithPerModelTimeout(-1s) = %v, want 0 (no-op)", c.perModelTimeout)
	}
}

func TestRun_perModelTimeoutDoesNotResetPositive(t *testing.T) {
	c := New(WithPerModelTimeout(5*time.Second), WithPerModelTimeout(0))
	if c.perModelTimeout != 5*time.Second {
		t.Errorf("perModelTimeout = %v, want 5s (zero must not overwrite positive)", c.perModelTimeout)
	}
	c2 := New(WithPerModelTimeout(5*time.Second), WithPerModelTimeout(-1*time.Second))
	if c2.perModelTimeout != 5*time.Second {
		t.Errorf("perModelTimeout = %v, want 5s (negative must not overwrite positive)", c2.perModelTimeout)
	}
}

func TestRun_callerCtxBoundsPerModelTimeout(t *testing.T) {
	// callerCtx 30ms, perModel 5s — caller wins via parent chain.
	c := New(WithPerModelTimeout(5 * time.Second))
	slow := &fakeModel{name: "slow", delay: 5 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.Run(ctx, validRequest(), []model.Model{slow})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if elapsed > 1*time.Second {
		t.Errorf("Run took %v, want bounded by caller ctx 30ms", elapsed)
	}
}

// --- panic isolation --------------------------------------------------------

func TestRun_panicInGenerateBecomesOutcome(t *testing.T) {
	c := New()
	boom := &fakeModel{name: "boom", panicOnGen: "kaboom"}
	ok := &fakeModel{name: "ok", response: okResponse("ok", "ok", "fine")}

	res, err := c.Run(context.Background(), validRequest(), []model.Model{boom, ok})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if res.Outcomes[0].Err == nil {
		t.Fatal("Outcomes[0].Err = nil after panic, want non-nil")
	}
	if !strings.Contains(res.Outcomes[0].Err.Error(), "fanout: provider panicked") {
		t.Errorf("Outcomes[0].Err = %q, want 'fanout: provider panicked' prefix", res.Outcomes[0].Err.Error())
	}
	if !strings.Contains(res.Outcomes[0].Err.Error(), "kaboom") {
		t.Errorf("Outcomes[0].Err = %q, want panic value 'kaboom'", res.Outcomes[0].Err.Error())
	}
	if res.Outcomes[0].Provider != "boom" {
		t.Errorf("Outcomes[0].Provider = %q, want 'boom' (set before Generate ran)", res.Outcomes[0].Provider)
	}
	if res.Outcomes[1].Err != nil {
		t.Errorf("Outcomes[1].Err = %v, want nil (panic must NOT contaminate other slots)", res.Outcomes[1].Err)
	}
	if res.Outcomes[1].Response == nil || res.Outcomes[1].Response.Message.Content != "fine" {
		t.Errorf("Outcomes[1].Response broken: %+v", res.Outcomes[1].Response)
	}
}

func TestRun_panicWithSentinelPreservesGrammar(t *testing.T) {
	// ADR-0011 §3 promises the upstream sentinel grammar is preserved
	// untouched through Outcome.Err. A panic whose value is itself an
	// error (e.g. an adapter that misuses a sentinel as a panic value)
	// MUST round-trip through errors.Is for the original sentinel, not
	// just stringify into the panic-prefixed message.
	c := New()
	boom := &fakeModel{name: "boom", panicOnGen: model.ErrAuthInvalid}
	res, err := c.Run(context.Background(), validRequest(), []model.Model{boom})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if res.Outcomes[0].Err == nil {
		t.Fatal("Outcomes[0].Err = nil after sentinel panic, want non-nil")
	}
	if !errors.Is(res.Outcomes[0].Err, model.ErrAuthInvalid) {
		t.Errorf("errors.Is(Outcomes[0].Err, ErrAuthInvalid) = false; want true. Got %v", res.Outcomes[0].Err)
	}
	if !strings.Contains(res.Outcomes[0].Err.Error(), "fanout: provider panicked") {
		t.Errorf("Outcomes[0].Err = %q, want 'fanout: provider panicked' prefix", res.Outcomes[0].Err.Error())
	}
}

func TestRun_panicInNameBecomesOutcome(t *testing.T) {
	c := New()
	boom := &fakeModel{name: "boom", panicOnName: errors.New("name panic")}
	res, err := c.Run(context.Background(), validRequest(), []model.Model{boom})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if res.Outcomes[0].Err == nil {
		t.Fatal("Outcomes[0].Err = nil after Name() panic, want non-nil")
	}
	if !strings.Contains(res.Outcomes[0].Err.Error(), "fanout: provider panicked") {
		t.Errorf("Outcomes[0].Err = %q, want 'fanout: provider panicked' prefix", res.Outcomes[0].Err.Error())
	}
	// Provider may legitimately be "" — Name() never returned.
}

// --- zero-value Coordinator -------------------------------------------------

func TestRun_zeroValueCoordinator(t *testing.T) {
	var c Coordinator
	f := &fakeModel{name: "x", response: okResponse("x", "x", "ok")}
	res, err := c.Run(context.Background(), validRequest(), []model.Model{f})
	if err != nil {
		t.Fatalf("zero-value Run err = %v, want nil", err)
	}
	if res.Outcomes[0].Err != nil {
		t.Errorf("zero-value Outcome.Err = %v, want nil", res.Outcomes[0].Err)
	}
}

func TestRun_zeroValueAfterFirstRunKeepsClock(t *testing.T) {
	var c Coordinator
	f := &fakeModel{name: "x", response: okResponse("x", "x", "ok")}
	if _, err := c.Run(context.Background(), validRequest(), []model.Model{f}); err != nil {
		t.Fatalf("first Run err = %v", err)
	}
	if c.now == nil {
		t.Error("c.now still nil after first Run; defense should have set it to time.Now")
	}
	if _, err := c.Run(context.Background(), validRequest(), []model.Model{f}); err != nil {
		t.Fatalf("second Run err = %v", err)
	}
}

// --- test seam for now ------------------------------------------------------

func TestRun_nowInjection_makesLatencyDeterministic(t *testing.T) {
	c := New()
	var calls int32
	c.now = func() time.Time {
		// Returns t0 for the first call, t0+250ms for the second.
		n := atomic.AddInt32(&calls, 1)
		if n%2 == 1 {
			return time.Unix(1_700_000_000, 0)
		}
		return time.Unix(1_700_000_000, 0).Add(250 * time.Millisecond)
	}
	f := &fakeModel{name: "x", response: okResponse("x", "x", "ok")}
	res, err := c.Run(context.Background(), validRequest(), []model.Model{f})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if res.Outcomes[0].Latency != 250*time.Millisecond {
		t.Errorf("Latency = %v, want 250ms (deterministic via injected clock)", res.Outcomes[0].Latency)
	}
}

// --- latency capture --------------------------------------------------------

func TestRun_latencyCapturedOnSuccess(t *testing.T) {
	c := New()
	f := &fakeModel{name: "x", response: okResponse("x", "x", "ok"), delay: 10 * time.Millisecond}
	res, err := c.Run(context.Background(), validRequest(), []model.Model{f})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if res.Outcomes[0].Latency < 10*time.Millisecond {
		t.Errorf("Latency = %v, want >= 10ms", res.Outcomes[0].Latency)
	}
	if res.Outcomes[0].Latency > 1*time.Second {
		t.Errorf("Latency = %v, suspiciously long for a 10ms fake", res.Outcomes[0].Latency)
	}
}

func TestRun_latencyCapturedOnError(t *testing.T) {
	c := New()
	f := &fakeModel{name: "x", err: errors.New("fail"), delay: 5 * time.Millisecond}
	res, err := c.Run(context.Background(), validRequest(), []model.Model{f})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if res.Outcomes[0].Latency < 5*time.Millisecond {
		t.Errorf("Latency on error = %v, want >= 5ms (still measured)", res.Outcomes[0].Latency)
	}
}

// --- reuse / concurrency ----------------------------------------------------

func TestRun_concurrentInvocationsOnSameCoordinator(t *testing.T) {
	c := New()
	ok := func(name string) *fakeModel { return &fakeModel{name: name, response: okResponse(name, name, name)} }

	var wg sync.WaitGroup
	const N = 20
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			models := []model.Model{ok(fmt.Sprintf("a%d", i)), ok(fmt.Sprintf("b%d", i))}
			res, err := c.Run(context.Background(), validRequest(), models)
			if err != nil {
				t.Errorf("concurrent run %d err = %v", i, err)
				return
			}
			if res.Outcomes[0].Provider != fmt.Sprintf("a%d", i) {
				t.Errorf("run %d Outcomes[0].Provider = %q, want a%d", i, res.Outcomes[0].Provider, i)
			}
			if res.Outcomes[1].Provider != fmt.Sprintf("b%d", i) {
				t.Errorf("run %d Outcomes[1].Provider = %q, want b%d", i, res.Outcomes[1].Provider, i)
			}
		}(i)
	}
	wg.Wait()
}

// --- goroutine-leak check ---------------------------------------------------

func TestRun_noGoroutineLeak(t *testing.T) {
	c := New()
	models := []model.Model{
		&fakeModel{name: "a", response: okResponse("a", "a", "x"), delay: 5 * time.Millisecond},
		&fakeModel{name: "b", response: okResponse("b", "b", "y"), delay: 10 * time.Millisecond},
		&fakeModel{name: "c", response: okResponse("c", "c", "z")},
		&fakeModel{name: "d", panicOnGen: "boom"},
	}
	// Warm-up to settle any one-time goroutines.
	if _, err := c.Run(context.Background(), validRequest(), models); err != nil {
		t.Fatalf("warm-up err = %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	runtime.GC()

	before := runtime.NumGoroutine()
	for i := 0; i < 30; i++ {
		if _, err := c.Run(context.Background(), validRequest(), models); err != nil {
			t.Fatalf("iter %d err = %v", i, err)
		}
	}
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	after := runtime.NumGoroutine()

	// Allow a small margin (test/runtime housekeeping); any real leak from
	// 30 runs over 4 goroutines each would be at least 30*4 = 120.
	if after-before > 5 {
		t.Errorf("goroutine leak: before=%d after=%d delta=%d", before, after, after-before)
	}
}

// --- New() / Options --------------------------------------------------------

func TestNew_defaultsApplied(t *testing.T) {
	c := New()
	if c.now == nil {
		t.Error("New() did not set c.now to time.Now")
	}
	if c.perModelTimeout != 0 {
		t.Errorf("New() perModelTimeout = %v, want 0", c.perModelTimeout)
	}
}

func TestWithPerModelTimeout_setsPositiveValue(t *testing.T) {
	c := New(WithPerModelTimeout(750 * time.Millisecond))
	if c.perModelTimeout != 750*time.Millisecond {
		t.Errorf("perModelTimeout = %v, want 750ms", c.perModelTimeout)
	}
}
