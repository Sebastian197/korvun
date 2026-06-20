// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sequential_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/fanout"
	"github.com/Sebastian197/korvun/internal/model/sequential"
	"github.com/Sebastian197/korvun/internal/policy"
)

// fakeModel is a serial-call test double. Sequential dispatch is
// single-goroutine, so the calls counter and order log need no synchronisation.
type fakeModel struct {
	name   string
	resp   *model.Response
	err    error
	panicV any
	delay  time.Duration
	calls  int
	onCall func()    // optional hook, runs at the start of Generate
	order  *[]string // optional shared call-order log
}

func (f *fakeModel) Name() string { return f.name }

func (f *fakeModel) Generate(ctx context.Context, _ *model.Request) (*model.Response, error) {
	f.calls++
	if f.order != nil {
		*f.order = append(*f.order, f.name)
	}
	if f.onCall != nil {
		f.onCall()
	}
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.panicV != nil {
		panic(f.panicV)
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

func okResp(name string) *model.Response {
	return &model.Response{
		Message:   model.Message{Role: model.RoleAssistant, Content: "from " + name},
		Provider:  name,
		ModelName: name,
	}
}

func validReq() *model.Request {
	return &model.Request{
		Model:    "m",
		Messages: []model.Message{{Role: model.RoleUser, Content: "hi"}},
	}
}

// TestRun_firstSucceeds_restNotCalled is the cost-saving heart: the first
// success stops the loop, and the remaining models never receive Generate.
func TestRun_firstSucceeds_restNotCalled(t *testing.T) {
	t.Parallel()
	m1 := &fakeModel{name: "ollama", resp: okResp("ollama")}
	m2 := &fakeModel{name: "groq", resp: okResp("groq")}

	res, err := sequential.New().Run(context.Background(), validReq(), []model.Model{m1, m2})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if len(res.Outcomes) != 1 {
		t.Fatalf("len(Outcomes) = %d, want 1 (only the first model called)", len(res.Outcomes))
	}
	if res.Outcomes[0].Provider != "ollama" || res.Outcomes[0].Err != nil {
		t.Errorf("Outcomes[0] = %+v, want ollama success", res.Outcomes[0])
	}
	if m1.calls != 1 {
		t.Errorf("m1.calls = %d, want 1", m1.calls)
	}
	if m2.calls != 0 {
		t.Errorf("m2.calls = %d, want 0 (cost saving: groq never received Generate because ollama answered)", m2.calls)
	}
}

// TestRun_failOver advances past failures and stops at the first success;
// models after the success are skipped (absent from Outcomes).
func TestRun_failOver(t *testing.T) {
	t.Parallel()
	m1 := &fakeModel{name: "ollama", err: model.ErrProviderUnavailable}
	m2 := &fakeModel{name: "groq", resp: okResp("groq")}
	m3 := &fakeModel{name: "anthropic", resp: okResp("anthropic")}

	res, err := sequential.New().Run(context.Background(), validReq(), []model.Model{m1, m2, m3})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if len(res.Outcomes) != 2 {
		t.Fatalf("len(Outcomes) = %d, want 2 (m1 failed, m2 ok, m3 skipped)", len(res.Outcomes))
	}
	if !errors.Is(res.Outcomes[0].Err, model.ErrProviderUnavailable) {
		t.Errorf("Outcomes[0].Err = %v, want ErrProviderUnavailable preserved", res.Outcomes[0].Err)
	}
	if res.Outcomes[1].Provider != "groq" || res.Outcomes[1].Err != nil {
		t.Errorf("Outcomes[1] = %+v, want groq success", res.Outcomes[1])
	}
	if m3.calls != 0 {
		t.Errorf("m3.calls = %d, want 0 (skipped after groq succeeded)", m3.calls)
	}
}

// TestRun_allFailed_reducerYieldsNoUsableOutcome: every model is called, all
// fail, Run is mechanism success ((*Result, nil)), and a reducer turns it into
// ErrNoUsableOutcome with the upstream causes preserved.
func TestRun_allFailed_reducerYieldsNoUsableOutcome(t *testing.T) {
	t.Parallel()
	m1 := &fakeModel{name: "ollama", err: model.ErrProviderUnavailable}
	m2 := &fakeModel{name: "groq", err: model.ErrAuthInvalid}

	res, err := sequential.New().Run(context.Background(), validReq(), []model.Model{m1, m2})
	if err != nil {
		t.Fatalf("Run err = %v (all-failed must be mechanism success)", err)
	}
	if len(res.Outcomes) != 2 {
		t.Fatalf("len(Outcomes) = %d, want 2 (all models called)", len(res.Outcomes))
	}

	_, perr := policy.PriorityReducer{}.Apply(context.Background(), res)
	if !errors.Is(perr, policy.ErrNoUsableOutcome) {
		t.Errorf("reducer err = %v, want ErrNoUsableOutcome", perr)
	}
	if !errors.Is(perr, model.ErrAuthInvalid) {
		t.Errorf("reducer err = %v, want joined ErrAuthInvalid cause preserved", perr)
	}
}

// TestRun_inputValidation: the shared fanout.ValidateRunInputs rejects the same
// misconfigurations with the same sentinels, returning a nil Result.
func TestRun_inputValidation(t *testing.T) {
	t.Parallel()
	good := &fakeModel{name: "ok", resp: okResp("ok")}

	tests := []struct {
		name    string
		ctx     context.Context
		req     *model.Request
		models  []model.Model
		wantErr error
		wantSub string
	}{
		{"nil ctx", nil, validReq(), []model.Model{good}, nil, "nil ctx"},
		{"no models", context.Background(), validReq(), nil, fanout.ErrNoModels, ""},
		{"nil model entry", context.Background(), validReq(), []model.Model{nil}, fanout.ErrNilModel, ""},
		{"invalid request", context.Background(), &model.Request{}, []model.Model{good}, model.ErrEmptyModel, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			res, err := sequential.New().Run(tt.ctx, tt.req, tt.models)
			if res != nil {
				t.Errorf("res = %v, want nil on validation error", res)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("err = %v, want errors.Is(%v)", err, tt.wantErr)
			}
			if tt.wantSub != "" && (err == nil || !strings.Contains(err.Error(), tt.wantSub)) {
				t.Errorf("err = %v, want contains %q", err, tt.wantSub)
			}
		})
	}
}

// TestRun_determinismCallOrder: models are called in input order, and Outcomes
// mirror that order.
func TestRun_determinismCallOrder(t *testing.T) {
	t.Parallel()
	var order []string
	m1 := &fakeModel{name: "a", err: errors.New("fail-a"), order: &order}
	m2 := &fakeModel{name: "b", err: errors.New("fail-b"), order: &order}
	m3 := &fakeModel{name: "c", resp: okResp("c"), order: &order}

	res, err := sequential.New().Run(context.Background(), validReq(), []model.Model{m1, m2, m3})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	want := []string{"a", "b", "c"}
	if !slices.Equal(order, want) {
		t.Errorf("call order = %v, want %v (input order)", order, want)
	}
	gotProviders := []string{
		res.Outcomes[0].Provider, res.Outcomes[1].Provider, res.Outcomes[2].Provider,
	}
	if !slices.Equal(gotProviders, want) {
		t.Errorf("outcome order = %v, want %v", gotProviders, want)
	}
}

// TestRun_panicPreservesSentinel: a panicking provider becomes a failed Outcome
// via the shared CallOne primitive, with the sentinel grammar (errors.Is) and
// the neutral panic prefix intact; a panic is a failure, so fail-over continues.
func TestRun_panicPreservesSentinel(t *testing.T) {
	t.Parallel()
	m1 := &fakeModel{name: "boom", panicV: model.ErrAuthInvalid}
	m2 := &fakeModel{name: "groq", resp: okResp("groq")}

	res, err := sequential.New().Run(context.Background(), validReq(), []model.Model{m1, m2})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if !errors.Is(res.Outcomes[0].Err, model.ErrAuthInvalid) {
		t.Errorf("Outcomes[0].Err = %v, want errors.Is ErrAuthInvalid (preserved via shared CallOne)", res.Outcomes[0].Err)
	}
	if !strings.Contains(res.Outcomes[0].Err.Error(), "model dispatch: provider panicked") {
		t.Errorf("Outcomes[0].Err = %q, want neutral panic prefix (not 'fanout')", res.Outcomes[0].Err.Error())
	}
	if m2.calls != 1 {
		t.Errorf("m2.calls = %d, want 1 (panic is a failure → fail over)", m2.calls)
	}
	if res.Outcomes[1].Err != nil {
		t.Errorf("Outcomes[1].Err = %v, want nil (groq succeeded)", res.Outcomes[1].Err)
	}
}

// TestRun_ctxCancelledBetweenCalls: m1 fails AND cancels the ctx during its
// own call, so the next iteration's between-calls guard (ctx.Err()) is what
// stops the loop — proven because m1 failed (the loop would otherwise advance
// to m2) yet m2 is never called. Cancelling during m1's call is the
// deterministic way to drive the between-calls guard in a single-goroutine loop.
func TestRun_ctxCancelledBetweenCalls(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	m1 := &fakeModel{name: "m1", err: errors.New("fail-1"), onCall: cancel}
	m2 := &fakeModel{name: "m2", resp: okResp("m2")}

	res, err := sequential.New().Run(ctx, validReq(), []model.Model{m1, m2})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if len(res.Outcomes) != 1 {
		t.Fatalf("len(Outcomes) = %d, want 1 (cancelled before m2)", len(res.Outcomes))
	}
	if m2.calls != 0 {
		t.Errorf("m2.calls = %d, want 0 (ctx cancelled between calls → m2 not called)", m2.calls)
	}
}

// TestRun_perModelTimeout: a slow model is bounded by the per-model timeout and
// fails; dispatch falls over to the fast model.
func TestRun_perModelTimeout(t *testing.T) {
	t.Parallel()
	slow := &fakeModel{name: "slow", resp: okResp("slow"), delay: 200 * time.Millisecond}
	fast := &fakeModel{name: "fast", resp: okResp("fast")}

	c := sequential.New(sequential.WithPerModelTimeout(10 * time.Millisecond))
	res, err := c.Run(context.Background(), validReq(), []model.Model{slow, fast})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if res.Outcomes[0].Err == nil {
		t.Errorf("Outcomes[0].Err = nil, want per-model timeout deadline error")
	}
	if res.Outcomes[1].Provider != "fast" || res.Outcomes[1].Err != nil {
		t.Errorf("Outcomes[1] = %+v, want fast success after slow timed out", res.Outcomes[1])
	}
}

// TestRun_zeroValueCoordinator: a zero-value Coordinator works one-shot (the
// lazy clock default), mirroring fanout. Concurrent reuse requires New.
func TestRun_zeroValueCoordinator(t *testing.T) {
	t.Parallel()
	var c sequential.Coordinator
	m1 := &fakeModel{name: "ollama", resp: okResp("ollama")}

	res, err := c.Run(context.Background(), validReq(), []model.Model{m1})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if len(res.Outcomes) != 1 || res.Outcomes[0].Err != nil {
		t.Errorf("Outcomes = %+v, want one success", res.Outcomes)
	}
}
