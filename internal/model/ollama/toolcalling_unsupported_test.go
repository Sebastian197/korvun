// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package ollama

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Sebastian197/korvun/internal/model"
)

// RT-3 live sub-phase: the RAW wire evidence (2026-08-15, ollama 0.30.8,
// gemma3:270m, 3/3 deterministic) is:
//
//	HTTP 400
//	{"error":"registry.ollama.ai/library/gemma3:270m does not support tools"}
//
// A capability refusal is NOT a generic bad response: the caller must be able
// to degrade to the text lane on it, so it maps to its own sentinel.

func TestGenerateWithTools_doesNotSupportTools_mapsToSentinel(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"registry.ollama.ai/library/gemma3:270m does not support tools"}`))
	}))
	t.Cleanup(srv.Close)

	a := New(WithBaseURL(srv.URL))
	req := &model.Request{Model: "gemma3:270m", Messages: []model.Message{{Role: model.RoleUser, Content: "hola"}}}
	_, execErr := a.GenerateWithTools(context.Background(), req, []model.ToolSpec{{Name: "time"}})
	if !errors.Is(execErr, model.ErrToolsUnsupported) {
		t.Fatalf("err = %v, want errors.Is(_, model.ErrToolsUnsupported)", execErr)
	}
	// The generic 400 class must NOT swallow the capability signal, and vice
	// versa an unrelated 400 must stay ErrProviderResponse.
}

func TestGenerateWithTools_otherBadRequestStaysProviderResponse(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid options"}`))
	}))
	t.Cleanup(srv.Close)

	a := New(WithBaseURL(srv.URL))
	req := &model.Request{Model: "m", Messages: []model.Message{{Role: model.RoleUser, Content: "hola"}}}
	_, execErr := a.GenerateWithTools(context.Background(), req, nil)
	if errors.Is(execErr, model.ErrToolsUnsupported) {
		t.Fatalf("err = %v; an unrelated 400 must not classify as tools-unsupported", execErr)
	}
	if !errors.Is(execErr, model.ErrProviderResponse) {
		t.Fatalf("err = %v, want ErrProviderResponse", execErr)
	}
}
