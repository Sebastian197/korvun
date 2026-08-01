// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"context"
	"errors"
	"testing"

	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/fanout"
	"github.com/Sebastian197/korvun/internal/tool"
)

// SP1 RED (builder-canvas FR-PERSONA-2, NC-4 resolved): ComposePersona is a
// PURE, deterministic composer — fixed section order (display name, tone,
// language, instructions), empty fields omitted, nil/empty persona → "" (zero
// prompt noise). The persona rides the Orchestrator via WithSystemPrompt
// (the previously-unwired path) and the AgentBrain via WithAgentPersona as a
// PREFIX before the ADR-0021 §3.1 protocol block, which stays INTACT.
//
// RED note (house precedent, coordinator_carveout_test.go): Persona,
// ComposePersona and WithAgentPersona do not exist yet — the compile failure
// IS the red.

// fullPersona is the reference persona used across these tests.
func fullPersona() *Persona {
	return &Persona{
		DisplayName:  "Nova",
		Tone:         "warm, concise",
		Language:     "es-ES",
		Instructions: "Never reveal internal tooling.",
	}
}

// wantFullComposed is the EXACT composed fragment for fullPersona — the
// deterministic contract: one line per present field, in fixed order, joined
// by \n, no trailing newline.
const wantFullComposed = "You are Nova.\n" +
	"Tone: warm, concise\n" +
	"Language: es-ES\n" +
	"Never reveal internal tooling."

// TestComposePersona_deterministicFullOrder pins the exact output and its
// determinism (two calls, identical bytes).
func TestComposePersona_deterministicFullOrder(t *testing.T) {
	t.Parallel()
	got := ComposePersona(fullPersona())
	if got != wantFullComposed {
		t.Errorf("ComposePersona = %q, want %q", got, wantFullComposed)
	}
	if again := ComposePersona(fullPersona()); again != got {
		t.Errorf("ComposePersona is not deterministic: %q vs %q", again, got)
	}
}

// TestComposePersona_omitsEmptyFields: absent fields leave NO placeholder
// lines — the fragment contains exactly the present sections, order kept.
func TestComposePersona_omitsEmptyFields(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		p    Persona
		want string
	}{
		{"only display_name", Persona{DisplayName: "Nova"}, "You are Nova."},
		{"only tone", Persona{Tone: "warm, concise"}, "Tone: warm, concise"},
		{"only language", Persona{Language: "es-ES"}, "Language: es-ES"},
		{"only instructions", Persona{Instructions: "Cite sources."}, "Cite sources."},
		{
			"display_name + instructions keep order, no gap lines",
			Persona{DisplayName: "Nova", Instructions: "Cite sources."},
			"You are Nova.\nCite sources.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ComposePersona(&tc.p); got != tc.want {
				t.Errorf("ComposePersona(%+v) = %q, want %q", tc.p, got, tc.want)
			}
		})
	}
}

// TestComposePersona_nilAndEmpty: nil persona, empty persona and
// whitespace-only fields all compose to "" — zero noise in the prompt.
func TestComposePersona_nilAndEmpty(t *testing.T) {
	t.Parallel()
	if got := ComposePersona(nil); got != "" {
		t.Errorf("ComposePersona(nil) = %q, want empty", got)
	}
	if got := ComposePersona(&Persona{}); got != "" {
		t.Errorf("ComposePersona(&Persona{}) = %q, want empty", got)
	}
	ws := &Persona{DisplayName: "  ", Tone: "\n", Language: "\t", Instructions: "   "}
	if got := ComposePersona(ws); got != "" {
		t.Errorf("ComposePersona(whitespace-only) = %q, want empty", got)
	}
}

// personaRecordingCoord captures the *model.Request the Orchestrator hands the
// dispatch seam, then stops the pipeline with an error — the request is fully
// composed by then, which is all these tests assert on.
type personaRecordingCoord struct{ got *model.Request }

func (c *personaRecordingCoord) Run(_ context.Context, req *model.Request, _ []model.Model) (*fanout.Result, error) {
	c.got = req
	return nil, errors.New("recorded: stop after capture")
}

func personaEnvelope() *envelope.Envelope {
	env := envelope.New("test", envelope.Inbound, envelope.Participant{ID: "u-1"})
	env.AddText("hola")
	return env
}

// TestOrchestrator_personaReachesRequest (AS-PERSONA-2): wired via
// WithSystemPrompt(ComposePersona(p)), the request to the model carries the
// composed persona as its system message.
func TestOrchestrator_personaReachesRequest(t *testing.T) {
	t.Parallel()
	coord := &personaRecordingCoord{}
	o := NewOrchestrator(coord, nil, nil, WithSystemPrompt(ComposePersona(fullPersona())))

	if _, err := o.Handle(context.Background(), personaEnvelope()); err == nil {
		t.Fatal("Handle: want the recording coordinator's sentinel error, got nil")
	}
	if coord.got == nil {
		t.Fatal("the coordinator never saw a request")
	}
	msgs := coord.got.Messages
	if len(msgs) != 2 || msgs[0].Role != model.RoleSystem {
		t.Fatalf("messages = %+v, want [system, user]", msgs)
	}
	if msgs[0].Content != wantFullComposed {
		t.Errorf("system prompt = %q, want the composed persona %q", msgs[0].Content, wantFullComposed)
	}
}

// TestOrchestrator_lastSystemPromptWins pins "gana WithSystemPrompt": options
// apply in order and the LAST WithSystemPrompt is the one the request carries
// (an explicit later override beats an earlier persona-derived prompt).
func TestOrchestrator_lastSystemPromptWins(t *testing.T) {
	t.Parallel()
	coord := &personaRecordingCoord{}
	o := NewOrchestrator(coord, nil, nil,
		WithSystemPrompt(ComposePersona(fullPersona())),
		WithSystemPrompt("explicit override"),
	)

	if _, err := o.Handle(context.Background(), personaEnvelope()); err == nil {
		t.Fatal("Handle: want the recording coordinator's sentinel error, got nil")
	}
	if got := coord.got.Messages[0].Content; got != "explicit override" {
		t.Errorf("system prompt = %q, want the LAST WithSystemPrompt to win", got)
	}
}

// TestOrchestrator_noPersona_requestUnchanged is the brain-level regression
// (AS-PERSONA-1): without a persona the request is EXACTLY today's — one user
// message, no system message.
func TestOrchestrator_noPersona_requestUnchanged(t *testing.T) {
	t.Parallel()
	coord := &personaRecordingCoord{}
	o := NewOrchestrator(coord, nil, nil)

	if _, err := o.Handle(context.Background(), personaEnvelope()); err == nil {
		t.Fatal("Handle: want the recording coordinator's sentinel error, got nil")
	}
	msgs := coord.got.Messages
	if len(msgs) != 1 || msgs[0].Role != model.RoleUser {
		t.Errorf("messages = %+v, want exactly [user] (no persona → no system message)", msgs)
	}
}

// personaCaptureModel records the messages the AgentBrain sends and answers
// with a plain final reply (no TOOL: line), ending the loop on iteration 1.
type personaCaptureModel struct{ gotMessages []model.Message }

func (m *personaCaptureModel) Name() string { return "capture" }

func (m *personaCaptureModel) Generate(_ context.Context, req *model.Request) (*model.Response, error) {
	m.gotMessages = append([]model.Message(nil), req.Messages...)
	return &model.Response{
		Message:   model.Message{Role: model.RoleAssistant, Content: "done"},
		Provider:  "capture",
		ModelName: req.Model,
	}, nil
}

// TestAgentBrain_personaPrefixesProtocolIntact (AS-PERSONA-3): with a persona,
// the seed system message is EXACTLY the composed persona, a blank-line
// separator, then the UNTOUCHED ADR-0021 buildSystemPrompt output (protocol
// grammar + catalog + operator prompt, internal order intact — asserting
// byte-equality against buildSystemPrompt proves nothing was pisado).
func TestAgentBrain_personaPrefixesProtocolIntact(t *testing.T) {
	t.Parallel()
	reg := tool.Registry{}
	m := &personaCaptureModel{}
	a := NewAgentBrain(m, reg,
		WithAgentSystemPrompt("Operator rules."),
		WithAgentPersona(ComposePersona(fullPersona())),
	)

	if _, err := a.Handle(context.Background(), personaEnvelope()); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(m.gotMessages) == 0 || m.gotMessages[0].Role != model.RoleSystem {
		t.Fatalf("messages = %+v, want a leading system message", m.gotMessages)
	}
	want := wantFullComposed + "\n\n" + buildSystemPrompt(reg, "Operator rules.")
	if got := m.gotMessages[0].Content; got != want {
		t.Errorf("agent system prompt = %q, want persona prefix + intact protocol block %q", got, want)
	}
}

// TestAgentBrain_noPersona_promptUnchanged is the agent-side regression
// (AS-PERSONA-1): without a persona the seed system message is byte-identical
// to today's buildSystemPrompt output.
func TestAgentBrain_noPersona_promptUnchanged(t *testing.T) {
	t.Parallel()
	reg := tool.Registry{}
	m := &personaCaptureModel{}
	a := NewAgentBrain(m, reg, WithAgentSystemPrompt("Operator rules."))

	if _, err := a.Handle(context.Background(), personaEnvelope()); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	want := buildSystemPrompt(reg, "Operator rules.")
	if got := m.gotMessages[0].Content; got != want {
		t.Errorf("agent system prompt = %q, want today's exact %q (no persona → unchanged)", got, want)
	}
}

// TestAgentBrain_emptyPersonaPrefixIsNoise-free: an empty composed persona
// (WithAgentPersona("")) must leave the prompt byte-identical to today — no
// stray separator lines.
func TestAgentBrain_emptyPersonaPrefixNoNoise(t *testing.T) {
	t.Parallel()
	reg := tool.Registry{}
	m := &personaCaptureModel{}
	a := NewAgentBrain(m, reg,
		WithAgentSystemPrompt("Operator rules."),
		WithAgentPersona(""),
	)

	if _, err := a.Handle(context.Background(), personaEnvelope()); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	want := buildSystemPrompt(reg, "Operator rules.")
	if got := m.gotMessages[0].Content; got != want {
		t.Errorf("agent system prompt = %q, want %q (empty persona adds NOTHING)", got, want)
	}
}
