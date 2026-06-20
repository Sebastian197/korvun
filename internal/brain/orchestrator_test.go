// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/fanout"
	"github.com/Sebastian197/korvun/internal/policy"
)

// quietLogger discards no-answer log output so the suite stays clean.
func quietLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

// fakePolicy returns a fixed Decision/error, to inject the policy outcomes the
// Handle error-contract split (ADR-0014 §3) must distinguish.
type fakePolicy struct {
	dec *policy.Decision
	err error
}

func (f fakePolicy) Apply(_ context.Context, _ *fanout.Result) (*policy.Decision, error) {
	return f.dec, f.err
}

// inboundText builds an inbound text Envelope addressed via a chat id in Meta.
func inboundText(channel, chatID, text string) *envelope.Envelope {
	e := envelope.New(channel, envelope.Inbound, envelope.Participant{ID: "user1", Name: "User"})
	e.AddText(text)
	e.Meta["telegram.chat_id"] = chatID
	return e
}

func okModels(ids ...string) []model.Model {
	ms := make([]model.Model, len(ids))
	for i, id := range ids {
		ms[i] = WithModelID(&recordingModel{name: id + "-provider", response: "from-" + id}, id)
	}
	return ms
}

func TestOrchestrator_Handle_success(t *testing.T) {
	t.Parallel()

	dec := &policy.Decision{
		Response: &model.Response{
			Message:  model.Message{Role: model.RoleAssistant, Content: "the answer"},
			Provider: "a-provider",
		},
	}
	o := NewOrchestrator(fanout.New(), okModels("a", "b"), fakePolicy{dec: dec}, WithLogger(quietLogger()))

	in := inboundText("telegram", "chat-42", "what is the answer?")
	out, err := o.Handle(context.Background(), in)
	if err != nil {
		t.Fatalf("Handle: unexpected error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d envelopes, want 1", len(out))
	}
	reply := out[0]
	if err := reply.Validate(); err != nil {
		t.Errorf("reply is not a valid Envelope: %v", err)
	}
	if reply.Direction != envelope.Outbound {
		t.Errorf("reply Direction = %v, want Outbound", reply.Direction)
	}
	if reply.Channel != "telegram" {
		t.Errorf("reply Channel = %q, want telegram (echoed)", reply.Channel)
	}
	if reply.Meta["telegram.chat_id"] != "chat-42" {
		t.Errorf("reply chat_id = %q, want chat-42 (echoed addressing)", reply.Meta["telegram.chat_id"])
	}
	if len(reply.Parts) != 1 || reply.Parts[0].Content != "the answer" {
		t.Errorf("reply parts = %+v, want one text part %q", reply.Parts, "the answer")
	}
}

// TestOrchestrator_Handle_realPolicy exercises the actual fan-out → policy →
// reply seam (a real Coordinator + a real PriorityReducer over fake models),
// not fakePolicy injection — the integration ADR-0014 exists to wire.
func TestOrchestrator_Handle_realPolicy(t *testing.T) {
	t.Parallel()

	t.Run("priority picks the higher-priority provider's real response", func(t *testing.T) {
		t.Parallel()
		a := &recordingModel{name: "a", response: "from-a"}
		b := &recordingModel{name: "b", response: "from-b"}
		models := []model.Model{WithModelID(a, "id-a"), WithModelID(b, "id-b")}
		o := NewOrchestrator(fanout.New(), models,
			policy.PriorityReducer{Order: []string{"b", "a"}}, // b ranks first
			WithLogger(quietLogger()))

		out, err := o.Handle(context.Background(), inboundText("telegram", "c", "hi"))
		if err != nil {
			t.Fatalf("Handle: %v", err)
		}
		if len(out) != 1 || out[0].Parts[0].Content != "from-b" {
			t.Fatalf("want reply %q (priority b), got %+v", "from-b", out)
		}
	})

	t.Run("all providers fail → real policy yields the fallback", func(t *testing.T) {
		t.Parallel()
		a := &recordingModel{name: "a", err: model.ErrProviderUnavailable}
		b := &recordingModel{name: "b", err: model.ErrAuthInvalid}
		models := []model.Model{WithModelID(a, "id-a"), WithModelID(b, "id-b")}
		o := NewOrchestrator(fanout.New(), models, policy.PriorityReducer{},
			WithFallback("none"), WithLogger(quietLogger()))

		out, err := o.Handle(context.Background(), inboundText("telegram", "c2", "hi"))
		if err != nil {
			t.Fatalf("Handle: %v", err)
		}
		if len(out) != 1 || out[0].Parts[0].Content != "none" {
			t.Fatalf("want fallback reply, got %+v", out)
		}
	})
}

// TestOrchestrator_optionGuards covers the WithFallback("") and WithLogger(nil)
// guards: both must leave the defaults intact (a nil logger would panic in
// logNoAnswer if the guard were wrong).
func TestOrchestrator_optionGuards(t *testing.T) {
	t.Parallel()
	dec := &policy.Decision{Provenance: policy.Provenance{
		Considered: []policy.Contribution{{Provider: "a-provider"}},
	}}
	o := NewOrchestrator(fanout.New(), okModels("a"),
		fakePolicy{dec: dec, err: policy.ErrNoConsensus},
		WithFallback(""), WithLogger(nil)) // both should be ignored

	out, err := o.Handle(context.Background(), inboundText("telegram", "c", "hi"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(out) != 1 || out[0].Parts[0].Content != defaultFallback {
		t.Errorf("WithFallback(\"\") should keep the default fallback, got %+v", out)
	}
}

func TestOrchestrator_Handle_systemPrompt(t *testing.T) {
	t.Parallel()

	rec := &recordingModel{name: "a-provider", response: "ok"}
	dec := &policy.Decision{Response: &model.Response{
		Message: model.Message{Role: model.RoleAssistant, Content: "ok"},
	}}
	o := NewOrchestrator(fanout.New(), []model.Model{WithModelID(rec, "a")},
		fakePolicy{dec: dec}, WithSystemPrompt("be terse"), WithLogger(quietLogger()))

	if _, err := o.Handle(context.Background(), inboundText("telegram", "c", "hi")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(rec.gotMessages) != 2 {
		t.Fatalf("provider saw %d messages, want 2 (system + user)", len(rec.gotMessages))
	}
	if rec.gotMessages[0].Role != model.RoleSystem || rec.gotMessages[0].Content != "be terse" {
		t.Errorf("system prompt not delivered: %+v", rec.gotMessages[0])
	}
}

func TestOrchestrator_Handle_noText_noReply(t *testing.T) {
	t.Parallel()

	rec := &recordingModel{name: "a-provider", response: "x"}
	o := NewOrchestrator(fanout.New(), []model.Model{WithModelID(rec, "a")},
		fakePolicy{}, WithLogger(quietLogger()))

	// A location-only inbound carries no text — nothing to ask.
	in := envelope.New("telegram", envelope.Inbound, envelope.Participant{ID: "u"})
	in.AddLocation(40.0, -3.0)

	out, err := o.Handle(context.Background(), in)
	if err != nil {
		t.Fatalf("Handle: unexpected error: %v", err)
	}
	if out != nil {
		t.Errorf("got %d envelopes, want nil (no reply)", len(out))
	}
	if rec.got != "" {
		t.Error("fan-out was invoked for a no-text Envelope; want short-circuit before Run")
	}
}

func TestOrchestrator_Handle_fanoutMisconfig_propagates(t *testing.T) {
	t.Parallel()

	// Empty model set: coord.Run returns ErrNoModels — a Brain misconfiguration
	// that must propagate to the router error hook, NOT a fallback reply.
	o := NewOrchestrator(fanout.New(), nil, fakePolicy{}, WithLogger(quietLogger()))

	out, err := o.Handle(context.Background(), inboundText("telegram", "c", "hi"))
	if !errors.Is(err, fanout.ErrNoModels) {
		t.Errorf("err = %v, want wrapped fanout.ErrNoModels", err)
	}
	if out != nil {
		t.Errorf("got %d envelopes, want nil on mechanism error", len(out))
	}
}

func TestOrchestrator_Handle_noAnswer_fallback(t *testing.T) {
	t.Parallel()

	const fallback = "no answer right now"

	tests := []struct {
		name      string
		policyErr error
	}{
		{"all providers failed", policy.ErrNoUsableOutcome},
		{"providers disagreed", policy.ErrNoConsensus},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Decision is non-nil (provenance present) even on the no-answer path.
			dec := &policy.Decision{Provenance: policy.Provenance{
				Considered: []policy.Contribution{{Provider: "a-provider"}},
			}}
			o := NewOrchestrator(fanout.New(), okModels("a", "b"),
				fakePolicy{dec: dec, err: tt.policyErr},
				WithFallback(fallback), WithLogger(quietLogger()))

			out, err := o.Handle(context.Background(), inboundText("telegram", "chat-9", "vote?"))
			if err != nil {
				t.Fatalf("no-answer must NOT propagate an error, got %v", err)
			}
			if len(out) != 1 || out[0].Parts[0].Content != fallback {
				t.Fatalf("want a single fallback reply %q, got %+v", fallback, out)
			}
			if out[0].Meta["telegram.chat_id"] != "chat-9" {
				t.Errorf("fallback reply lost the addressing chat_id")
			}
		})
	}
}

func TestOrchestrator_Handle_policyMechanismError_propagates(t *testing.T) {
	t.Parallel()

	// ErrNilResult is mechanism misuse (a Brain bug), not a no-answer outcome:
	// it must propagate, not become a fallback reply.
	o := NewOrchestrator(fanout.New(), okModels("a"),
		fakePolicy{err: policy.ErrNilResult}, WithLogger(quietLogger()))

	out, err := o.Handle(context.Background(), inboundText("telegram", "c", "hi"))
	if !errors.Is(err, policy.ErrNilResult) {
		t.Errorf("err = %v, want wrapped policy.ErrNilResult", err)
	}
	if out != nil {
		t.Errorf("got %d envelopes, want nil on mechanism error", len(out))
	}
}
