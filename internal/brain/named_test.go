// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"context"
	"testing"

	"github.com/Sebastian197/korvun/internal/model"
	"github.com/Sebastian197/korvun/internal/model/fanout"
)

// TestWithModelID_heterogeneousFanout is the load-bearing test for ADR-0014 §2:
// the decorator gives each provider its own model id by COPYING the shared
// request, never mutating it. Run under -race, it pins the intersection of the
// new decorator with the fan-out's shared *req (the bug class that bit 2E.8 and
// fan-out P2): a buggy decorator that wrote req.Model would (a) trip -race on
// concurrent writes and (b) hand providers nondeterministic ids.
func TestWithModelID_heterogeneousFanout(t *testing.T) {
	t.Parallel()

	// Several providers, distinct ids — widen the concurrent surface so a
	// shared-request mutation is caught.
	specs := []struct{ name, id string }{
		{"ollama", "llama3.2"},
		{"groq", "llama-3.3-70b"},
		{"together", "mixtral-8x7b"},
		{"local", "phi-4"},
	}
	recs := make([]*recordingModel, len(specs))
	models := make([]model.Model, len(specs))
	for i, s := range specs {
		recs[i] = &recordingModel{name: s.name, response: "ack"}
		models[i] = WithModelID(recs[i], s.id)
	}

	req := &model.Request{
		Model:    "placeholder",
		Messages: []model.Message{{Role: model.RoleUser, Content: "are you up?"}},
	}
	res, err := fanout.New().Run(context.Background(), req, models)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The shared request must be untouched — proof the decorator copied.
	if req.Model != "placeholder" {
		t.Errorf("shared req.Model was mutated to %q, want %q", req.Model, "placeholder")
	}

	// Each provider received its own bound id (correct binding on the happy
	// path). The mutation bug itself is caught by the req.Model=="placeholder"
	// check above plus the -race detector, not by this loop.
	for i, r := range recs {
		if r.got != specs[i].id {
			t.Errorf("provider %q saw model id %q, want %q", r.name, r.got, specs[i].id)
		}
	}

	// Attribution stays the provider name (m.Name()), never the model id; the
	// bound id surfaces only as Response.ModelName.
	gotModelName := map[string]string{}
	for _, oc := range res.Outcomes {
		if oc.Err != nil {
			t.Fatalf("outcome %q failed: %v", oc.Provider, oc.Err)
		}
		gotModelName[oc.Provider] = oc.Response.ModelName
	}
	for _, s := range specs {
		if gotModelName[s.name] != s.id {
			t.Errorf("provider %q: Response.ModelName = %q, want %q", s.name, gotModelName[s.name], s.id)
		}
		if _, leaked := gotModelName[s.id]; leaked {
			t.Errorf("model id %q leaked as a provider name in attribution", s.id)
		}
	}
}

// TestWithModelID_nameDelegates confirms attribution delegates to the wrapped
// provider, not the id.
func TestWithModelID_nameDelegates(t *testing.T) {
	t.Parallel()
	m := WithModelID(&recordingModel{name: "ollama"}, "llama3.2")
	if m.Name() != "ollama" {
		t.Errorf("Name() = %q, want ollama", m.Name())
	}
}

// recordingModel is a fake model.Model that records the Request.Model it
// observed and returns a canned response or error. Distinct instances are used
// per provider so each writes its own .got field from its own goroutine.
type recordingModel struct {
	name        string
	response    string
	err         error
	got         string          // the Request.Model this provider observed
	gotMessages []model.Message // the conversation this provider observed
}

func (m *recordingModel) Generate(_ context.Context, req *model.Request) (*model.Response, error) {
	m.got = req.Model
	m.gotMessages = req.Messages
	if m.err != nil {
		return nil, m.err
	}
	return &model.Response{
		Message:   model.Message{Role: model.RoleAssistant, Content: m.response},
		Provider:  m.name,
		ModelName: req.Model,
	}, nil
}

func (m *recordingModel) Name() string { return m.name }
