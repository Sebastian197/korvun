// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/model"
)

func TestNew_defaults(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "")
	a := New()
	if a.baseURL != DefaultBaseURL {
		t.Errorf("baseURL = %q, want %q", a.baseURL, DefaultBaseURL)
	}
	if a.client == nil {
		t.Error("default client is nil")
	}
	if a.timeout != 0 {
		t.Errorf("default timeout = %v, want 0", a.timeout)
	}
}

func TestDefaultBaseURL_honorsEnv(t *testing.T) {
	cases := []struct {
		env  string
		want string
	}{
		{"", DefaultBaseURL},
		{"http://1.2.3.4:11434", "http://1.2.3.4:11434"},
		{"https://api.example.com/", "https://api.example.com"},
		{"1.2.3.4:11434", "http://1.2.3.4:11434"},
		{"my-host", "http://my-host"},
	}
	for _, tc := range cases {
		t.Run(tc.env, func(t *testing.T) {
			t.Setenv("OLLAMA_HOST", tc.env)
			if got := defaultBaseURL(); got != tc.want {
				t.Errorf("defaultBaseURL() with env=%q = %q, want %q",
					tc.env, got, tc.want)
			}
		})
	}
}

func TestWithBaseURL_trimsTrailingSlash(t *testing.T) {
	a := New(WithBaseURL("http://example.com:11434/"))
	if a.baseURL != "http://example.com:11434" {
		t.Errorf("baseURL = %q", a.baseURL)
	}
}

func TestWithHTTPClient_injected(t *testing.T) {
	custom := &http.Client{Timeout: 7 * time.Second}
	a := New(WithHTTPClient(custom))
	if a.client != custom {
		t.Errorf("WithHTTPClient did not stick")
	}
}

func TestWithRequestTimeout_sticks(t *testing.T) {
	a := New(WithRequestTimeout(2 * time.Second))
	if a.timeout != 2*time.Second {
		t.Errorf("timeout = %v", a.timeout)
	}
}

func TestName(t *testing.T) {
	a := New()
	if a.Name() != ProviderName {
		t.Errorf("Name() = %q, want %q", a.Name(), ProviderName)
	}
}

func TestGenerate_happyPath(t *testing.T) {
	var gotPath, gotContentType string
	var gotBody chatRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("server decode err = %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "llama3.2",
			"message": map[string]string{
				"role":    "assistant",
				"content": "Hola, soy un modelo.",
			},
			"done": true,
		})
	}))
	t.Cleanup(srv.Close)

	a := New(WithBaseURL(srv.URL))
	req := &model.Request{
		Model: "llama3.2",
		Messages: []model.Message{
			{Role: model.RoleSystem, Content: "Eres útil."},
			{Role: model.RoleUser, Content: "¿Quién eres?"},
		},
	}

	resp, err := a.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate err = %v", err)
	}

	if gotPath != chatPath {
		t.Errorf("server saw path = %q, want %q", gotPath, chatPath)
	}
	if !strings.HasPrefix(gotContentType, "application/json") {
		t.Errorf("Content-Type = %q", gotContentType)
	}
	if gotBody.Model != "llama3.2" {
		t.Errorf("request model = %q", gotBody.Model)
	}
	if gotBody.Stream {
		t.Errorf("request stream = true, want false")
	}
	if len(gotBody.Messages) != 2 {
		t.Fatalf("request messages len = %d, want 2", len(gotBody.Messages))
	}
	if gotBody.Messages[0].Role != "system" || gotBody.Messages[0].Content != "Eres útil." {
		t.Errorf("system msg = %+v", gotBody.Messages[0])
	}
	if gotBody.Messages[1].Role != "user" || gotBody.Messages[1].Content != "¿Quién eres?" {
		t.Errorf("user msg = %+v", gotBody.Messages[1])
	}

	if resp.Provider != ProviderName {
		t.Errorf("Provider = %q", resp.Provider)
	}
	if resp.ModelName != "llama3.2" {
		t.Errorf("ModelName = %q", resp.ModelName)
	}
	if resp.Message.Role != model.RoleAssistant {
		t.Errorf("Message.Role = %v, want RoleAssistant", resp.Message.Role)
	}
	if resp.Message.Content != "Hola, soy un modelo." {
		t.Errorf("Message.Content = %q", resp.Message.Content)
	}
}

func TestGenerate_validationFlowsThrough(t *testing.T) {
	a := New(WithBaseURL("http://127.0.0.1:1")) // unreachable, must never be called
	_, err := a.Generate(context.Background(), nil)
	if !errors.Is(err, model.ErrNilRequest) {
		t.Errorf("err = %v, want ErrNilRequest", err)
	}

	_, err = a.Generate(context.Background(), &model.Request{
		Messages: []model.Message{{Role: model.RoleUser, Content: "x"}},
	})
	if !errors.Is(err, model.ErrEmptyModel) {
		t.Errorf("err = %v, want ErrEmptyModel", err)
	}
}

func TestGenerate_wrapsTransportError(t *testing.T) {
	// Point at a port nothing is listening on.
	a := New(WithBaseURL("http://127.0.0.1:1"))
	req := &model.Request{
		Model:    "llama3.2",
		Messages: []model.Message{{Role: model.RoleUser, Content: "hola"}},
	}
	_, err := a.Generate(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, model.ErrProviderUnavailable) {
		t.Errorf("err = %v, want wrap of ErrProviderUnavailable", err)
	}
}

func TestGenerate_wrapsNon2xxStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "server kaput")
	}))
	t.Cleanup(srv.Close)

	a := New(WithBaseURL(srv.URL))
	req := &model.Request{
		Model:    "llama3.2",
		Messages: []model.Message{{Role: model.RoleUser, Content: "hola"}},
	}
	_, err := a.Generate(context.Background(), req)
	// 5xx now maps to ErrProviderUnavailable (retryable) — the non-2xx
	// classification was refined in ADR-0031 sub-phase 3 (mapHTTPError).
	if !errors.Is(err, model.ErrProviderUnavailable) {
		t.Errorf("err = %v, want wrap of ErrProviderUnavailable", err)
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("err string should mention status 500: %v", err)
	}
}

func TestGenerate_wrapsMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "not json at all")
	}))
	t.Cleanup(srv.Close)

	a := New(WithBaseURL(srv.URL))
	req := &model.Request{
		Model:    "llama3.2",
		Messages: []model.Message{{Role: model.RoleUser, Content: "hola"}},
	}
	_, err := a.Generate(context.Background(), req)
	if !errors.Is(err, model.ErrProviderResponse) {
		t.Errorf("err = %v, want wrap of ErrProviderResponse", err)
	}
}

func TestGenerate_rejectsEmptyAssistantContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "llama3.2",
			"message": map[string]string{"role": "assistant", "content": ""},
			"done":    true,
		})
	}))
	t.Cleanup(srv.Close)

	a := New(WithBaseURL(srv.URL))
	req := &model.Request{
		Model:    "llama3.2",
		Messages: []model.Message{{Role: model.RoleUser, Content: "hola"}},
	}
	_, err := a.Generate(context.Background(), req)
	if !errors.Is(err, model.ErrProviderResponse) {
		t.Errorf("err = %v, want wrap of ErrProviderResponse", err)
	}
}

func TestGenerate_respectsContextCancellation(t *testing.T) {
	// The handler holds the connection open until the request
	// context cancels OR a hard 2s safety cap fires. The safety
	// cap exists so a regression in our test does not deadlock the
	// suite — go test would only catch it at -timeout, which is
	// minutes.
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	t.Cleanup(srv.Close)

	a := New(WithBaseURL(srv.URL))
	req := &model.Request{
		Model:    "llama3.2",
		Messages: []model.Message{{Role: model.RoleUser, Content: "hola"}},
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	_, err := a.Generate(ctx, req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, model.ErrProviderUnavailable) {
		t.Errorf("err = %v, want wrap of ErrProviderUnavailable", err)
	}
}

func TestGenerate_respectsRequestTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	t.Cleanup(srv.Close)

	a := New(
		WithBaseURL(srv.URL),
		WithRequestTimeout(20*time.Millisecond),
	)
	req := &model.Request{
		Model:    "llama3.2",
		Messages: []model.Message{{Role: model.RoleUser, Content: "hola"}},
	}

	start := time.Now()
	_, err := a.Generate(context.Background(), req)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed > 1500*time.Millisecond {
		t.Errorf("Generate took %v, expected timeout near 20ms", elapsed)
	}
	if !errors.Is(err, model.ErrProviderUnavailable) {
		t.Errorf("err = %v, want wrap of ErrProviderUnavailable", err)
	}
}

// Compile-time assertion that the adapter satisfies model.Model.
var _ model.Model = (*Adapter)(nil)
