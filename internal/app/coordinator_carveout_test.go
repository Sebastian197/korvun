// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/sequential"
	"github.com/Sebastian197/korvun/internal/policy"
)

// This file pins ADR-0031 sub-phase 2's consensus carve-out at the WIRING layer:
// buildCoordinator mounts a cancelling fan-out for a priority brain and a
// wait-all fan-out for a consensus brain. The coordinator never imports
// internal/policy — the priority-vs-consensus decision is a string the app
// passes down.
//
// RED note: buildCoordinator takes only dispatch today; these tests call it with
// the policy kind too, so the package fails to build. That compile failure IS
// the red for the new signature; GREEN threads Policy.Kind into buildCoordinator
// and maps "priority" to cancel-on-first-usable-success.

// voteModel is a deterministic model.Model for the coordinator tests: it answers
// with a fixed content after an optional delay, honoring ctx cancellation.
type voteModel struct {
	name    string
	content string
	err     error
	delay   time.Duration
}

func (m *voteModel) Name() string { return m.name }

func (m *voteModel) Generate(ctx context.Context, _ *model.Request) (*model.Response, error) {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, fmt.Errorf("vote: %w (ctx cancel)", model.ErrProviderUnavailable)
		}
	}
	if m.err != nil {
		return nil, m.err
	}
	return &model.Response{
		Message:   model.Message{Role: model.RoleAssistant, Content: m.content},
		Provider:  m.name,
		ModelName: m.name,
	}, nil
}

func voteRequest() *model.Request {
	return &model.Request{Model: "m", Messages: []model.Message{{Role: model.RoleUser, Content: "agree?"}}}
}

// TestBuildCoordinator_priorityCancelsOnFirstUsableSuccess pins that a priority
// brain wires a cancelling fan-out: a fast OK makes Run return promptly and the
// slow sibling (blocking until ctx cancel) is cancelled (ADR-0031 sub-phase 2).
func TestBuildCoordinator_priorityCancelsOnFirstUsableSuccess(t *testing.T) {
	coord := buildCoordinator("fanout", "priority")

	fast := &voteModel{name: "fast", content: "quick"}
	slow := &voteModel{name: "slow", delay: 10 * time.Second}

	start := time.Now()
	res, err := coord.Run(context.Background(), voteRequest(), []model.Model{fast, slow})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("priority fan-out did not cancel on first usable success (took %v)", time.Since(start))
	}
	if res.Outcomes[1].Err == nil {
		t.Errorf("slow outcome = %+v, want a cancellation Err (priority must cancel siblings)", res.Outcomes[1])
	}
}

// TestBuildCoordinator_consensusPreservesWaitAll is the heart of sub-phase 2:
// a consensus brain must NOT cancel. Two slow concordant votes plus one fast
// dissenting vote must still form a majority. If the wiring wrongly mounted
// cancellation, the fast "no" would cancel both slow "yes" votes, leaving a
// single success → ErrNoConsensus. This test bites that regression.
func TestBuildCoordinator_consensusPreservesWaitAll(t *testing.T) {
	coord := buildCoordinator("fanout", "consensus")

	yesA := &voteModel{name: "yesA", content: "yes", delay: 50 * time.Millisecond}
	yesB := &voteModel{name: "yesB", content: "yes", delay: 50 * time.Millisecond}
	no := &voteModel{name: "no", content: "no"} // fast dissenter

	res, err := coord.Run(context.Background(), voteRequest(), []model.Model{yesA, yesB, no})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	dec, err := policy.ConsensusReducer{}.Apply(context.Background(), res)
	if err != nil {
		t.Fatalf("consensus Apply err = %v, want a formed majority (wait-all must be preserved for consensus)", err)
	}
	if got := dec.Response.Message.Content; got != "yes" {
		t.Errorf("consensus answer = %q, want %q (the concordant majority)", got, "yes")
	}
}

// TestBuildCoordinator_sequentialUnchanged guards that the new signature still
// builds the sequential coordinator unchanged (sequential is untouched this
// sub-phase — ADR-0031).
func TestBuildCoordinator_sequentialUnchanged(t *testing.T) {
	coord := buildCoordinator("sequential", "priority")
	if _, ok := coord.(*sequential.Coordinator); !ok {
		t.Fatalf("buildCoordinator(sequential) = %T, want *sequential.Coordinator", coord)
	}
}
