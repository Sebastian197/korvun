// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package groq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/model"
)

// --- Construction / API-key resolution ---

func TestNew_rejectsMissingKey(t *testing.T) {
	t.Setenv("GROQ_API_KEY", "")
	_, err := New()
	if !errors.Is(err, ErrMissingAPIKey) {
		t.Errorf("err = %v, want ErrMissingAPIKey", err)
	}
}

func TestNew_acceptsKeyFromEnv(t *testing.T) {
	t.Setenv("GROQ_API_KEY", "gsk_envkey")
	a, err := New()
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	if a.apiKey != "gsk_envkey" {
		t.Errorf("apiKey not loaded from env")
	}
}

func TestNew_acceptsKeyFromOption(t *testing.T) {
	t.Setenv("GROQ_API_KEY", "")
	a, err := New(WithAPIKey("gsk_optkey"))
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	if a.apiKey != "gsk_optkey" {
		t.Errorf("apiKey from option not stored")
	}
}

func TestNew_optionOverridesEnv(t *testing.T) {
	t.Setenv("GROQ_API_KEY", "gsk_envkey")
	a, err := New(WithAPIKey("gsk_optkey"))
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	if a.apiKey != "gsk_optkey" {
		t.Errorf("WithAPIKey did not override env: got %q, want gsk_optkey",
			a.apiKey)
	}
}

func TestNew_defaults(t *testing.T) {
	t.Setenv("GROQ_API_KEY", "gsk_k")
	a, err := New()
	if err != nil {
		t.Fatalf("New err = %v", err)
	}
	if a.baseURL != DefaultBaseURL {
		t.Errorf("baseURL = %q, want %q", a.baseURL, DefaultBaseURL)
	}
	if a.client == nil {
		t.Error("client is nil")
	}
	if a.timeout != 0 {
		t.Errorf("timeout = %v, want 0", a.timeout)
	}
}

func TestWithBaseURL_trimsTrailingSlash(t *testing.T) {
	t.Setenv("GROQ_API_KEY", "gsk_k")
	a, _ := New(WithBaseURL("https://example.com/openai/v1/"))
	if a.baseURL != "https://example.com/openai/v1" {
		t.Errorf("baseURL = %q", a.baseURL)
	}
}

func TestWithHTTPClient_sticks(t *testing.T) {
	t.Setenv("GROQ_API_KEY", "gsk_k")
	custom := &http.Client{Timeout: 7 * time.Second}
	a, _ := New(WithHTTPClient(custom))
	if a.client != custom {
		t.Error("WithHTTPClient did not stick")
	}
}

func TestWithRequestTimeout_sticks(t *testing.T) {
	t.Setenv("GROQ_API_KEY", "gsk_k")
	a, _ := New(WithRequestTimeout(3 * time.Second))
	if a.timeout != 3*time.Second {
		t.Errorf("timeout = %v", a.timeout)
	}
}

func TestName(t *testing.T) {
	t.Setenv("GROQ_API_KEY", "gsk_k")
	a, _ := New()
	if a.Name() != ProviderName {
		t.Errorf("Name() = %q, want %q", a.Name(), ProviderName)
	}
}

// --- API key never surfaces in default formatting ---

func TestAdapter_StringRedactsKey(t *testing.T) {
	t.Setenv("GROQ_API_KEY", "")
	a, _ := New(WithAPIKey("gsk_super_secret_key_should_never_leak"))
	cases := []string{
		fmt.Sprintf("%v", a),
		fmt.Sprintf("%+v", a),
		fmt.Sprintf("%#v", a),
		a.String(),
	}
	for _, got := range cases {
		if strings.Contains(got, "gsk_super_secret") {
			t.Errorf("formatting leaked API key: %q", got)
		}
	}
}

// --- Generate happy path ---

func TestGenerate_happyPath(t *testing.T) {
	var sawAuth, sawCT, sawPath string
	var gotBody chatRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		sawCT = r.Header.Get("Content-Type")
		sawPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("server decode err = %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "chatcmpl-test",
			"object": "chat.completion",
			"model":  "llama-3.1-8b-instant",
			"choices": []map[string]any{{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": "Hola desde la nube.",
				},
				"finish_reason": "stop",
			}},
		})
	}))
	t.Cleanup(srv.Close)

	t.Setenv("GROQ_API_KEY", "")
	a, err := New(WithBaseURL(srv.URL), WithAPIKey("gsk_testkey"))
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	req := &model.Request{
		Model: "llama-3.1-8b-instant",
		Messages: []model.Message{
			{Role: model.RoleSystem, Content: "Eres útil."},
			{Role: model.RoleUser, Content: "¿Quién eres?"},
		},
	}

	resp, err := a.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate err = %v", err)
	}

	if sawAuth != "Bearer gsk_testkey" {
		t.Errorf("Authorization header = %q, want %q", sawAuth, "Bearer gsk_testkey")
	}
	if !strings.HasPrefix(sawCT, "application/json") {
		t.Errorf("Content-Type = %q", sawCT)
	}
	if sawPath != chatPath {
		t.Errorf("path = %q, want %q", sawPath, chatPath)
	}
	if gotBody.Stream {
		t.Errorf("Stream = true, want false")
	}
	if len(gotBody.Messages) != 2 ||
		gotBody.Messages[0].Role != "system" ||
		gotBody.Messages[1].Role != "user" {
		t.Errorf("messages mapping wrong: %+v", gotBody.Messages)
	}

	if resp.Provider != ProviderName {
		t.Errorf("Provider = %q", resp.Provider)
	}
	if resp.ModelName != "llama-3.1-8b-instant" {
		t.Errorf("ModelName = %q", resp.ModelName)
	}
	if resp.Message.Role != model.RoleAssistant {
		t.Errorf("Role = %v", resp.Message.Role)
	}
	if resp.Message.Content != "Hola desde la nube." {
		t.Errorf("Content = %q", resp.Message.Content)
	}
}

// --- Generate validation pass-through ---

func TestGenerate_validationFlowsThrough(t *testing.T) {
	t.Setenv("GROQ_API_KEY", "gsk_k")
	a, _ := New(WithBaseURL("http://127.0.0.1:1"))
	_, err := a.Generate(context.Background(), nil)
	if !errors.Is(err, model.ErrNilRequest) {
		t.Errorf("err = %v, want ErrNilRequest", err)
	}
}

// --- Error mapping (the heart of ADR-0010 §4) ---

func TestGenerate_maps401ToAuthInvalid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"Invalid API Key","type":"invalid_request_error","code":"invalid_api_key"}}`)
	}))
	t.Cleanup(srv.Close)

	t.Setenv("GROQ_API_KEY", "gsk_k")
	a, _ := New(WithBaseURL(srv.URL))
	_, err := a.Generate(context.Background(), basicRequest())
	if !errors.Is(err, model.ErrAuthInvalid) {
		t.Errorf("err = %v, want wrap of ErrAuthInvalid", err)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("err string missing status 401: %v", err)
	}
}

func TestGenerate_maps403ToAuthInvalid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":{"message":"Forbidden"}}`)
	}))
	t.Cleanup(srv.Close)

	t.Setenv("GROQ_API_KEY", "gsk_k")
	a, _ := New(WithBaseURL(srv.URL))
	_, err := a.Generate(context.Background(), basicRequest())
	if !errors.Is(err, model.ErrAuthInvalid) {
		t.Errorf("err = %v, want wrap of ErrAuthInvalid", err)
	}
}

func TestGenerate_maps429WithRetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "42")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"Rate limit exceeded","type":"requests","code":"rate_limit_exceeded"}}`)
	}))
	t.Cleanup(srv.Close)

	t.Setenv("GROQ_API_KEY", "gsk_k")
	a, _ := New(WithBaseURL(srv.URL))
	_, err := a.Generate(context.Background(), basicRequest())
	if !errors.Is(err, model.ErrRateLimited) {
		t.Errorf("err = %v, want wrap of ErrRateLimited", err)
	}
	var rle *model.RateLimitError
	if !errors.As(err, &rle) {
		t.Fatal("errors.As did not recover *RateLimitError")
	}
	if rle.Provider != ProviderName {
		t.Errorf("RateLimitError.Provider = %q", rle.Provider)
	}
	if rle.RetryAfter != 42*time.Second {
		t.Errorf("RateLimitError.RetryAfter = %v, want 42s", rle.RetryAfter)
	}
}

func TestGenerate_maps429WithoutRetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"Rate limit exceeded"}}`)
	}))
	t.Cleanup(srv.Close)

	t.Setenv("GROQ_API_KEY", "gsk_k")
	a, _ := New(WithBaseURL(srv.URL))
	_, err := a.Generate(context.Background(), basicRequest())
	var rle *model.RateLimitError
	if !errors.As(err, &rle) {
		t.Fatal("errors.As did not recover *RateLimitError")
	}
	if rle.RetryAfter != 0 {
		t.Errorf("RetryAfter = %v, want 0 (no header)", rle.RetryAfter)
	}
}

func TestGenerate_maps5xxToProviderUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":{"message":"server kaput"}}`)
	}))
	t.Cleanup(srv.Close)

	t.Setenv("GROQ_API_KEY", "gsk_k")
	a, _ := New(WithBaseURL(srv.URL))
	_, err := a.Generate(context.Background(), basicRequest())
	if !errors.Is(err, model.ErrProviderUnavailable) {
		t.Errorf("err = %v, want wrap of ErrProviderUnavailable", err)
	}
}

func TestGenerate_maps400ToProviderResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"message":"bad model name"}}`)
	}))
	t.Cleanup(srv.Close)

	t.Setenv("GROQ_API_KEY", "gsk_k")
	a, _ := New(WithBaseURL(srv.URL))
	_, err := a.Generate(context.Background(), basicRequest())
	if !errors.Is(err, model.ErrProviderResponse) {
		t.Errorf("err = %v, want wrap of ErrProviderResponse", err)
	}
}

func TestGenerate_maps404ToProviderResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":{"message":"model not found"}}`)
	}))
	t.Cleanup(srv.Close)

	t.Setenv("GROQ_API_KEY", "gsk_k")
	a, _ := New(WithBaseURL(srv.URL))
	_, err := a.Generate(context.Background(), basicRequest())
	if !errors.Is(err, model.ErrProviderResponse) {
		t.Errorf("err = %v, want wrap of ErrProviderResponse", err)
	}
}

func TestGenerate_wrapsMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "not json at all")
	}))
	t.Cleanup(srv.Close)

	t.Setenv("GROQ_API_KEY", "gsk_k")
	a, _ := New(WithBaseURL(srv.URL))
	_, err := a.Generate(context.Background(), basicRequest())
	if !errors.Is(err, model.ErrProviderResponse) {
		t.Errorf("err = %v, want wrap of ErrProviderResponse", err)
	}
}

func TestGenerate_rejectsEmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "llama-3.1-8b-instant",
			"choices": []any{},
		})
	}))
	t.Cleanup(srv.Close)

	t.Setenv("GROQ_API_KEY", "gsk_k")
	a, _ := New(WithBaseURL(srv.URL))
	_, err := a.Generate(context.Background(), basicRequest())
	if !errors.Is(err, model.ErrProviderResponse) {
		t.Errorf("err = %v, want wrap of ErrProviderResponse", err)
	}
}

func TestGenerate_rejectsEmptyAssistantContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "llama-3.1-8b-instant",
			"choices": []map[string]any{{
				"index":   0,
				"message": map[string]string{"role": "assistant", "content": ""},
			}},
		})
	}))
	t.Cleanup(srv.Close)

	t.Setenv("GROQ_API_KEY", "gsk_k")
	a, _ := New(WithBaseURL(srv.URL))
	_, err := a.Generate(context.Background(), basicRequest())
	if !errors.Is(err, model.ErrProviderResponse) {
		t.Errorf("err = %v, want wrap of ErrProviderResponse", err)
	}
}

func TestGenerate_wrapsTransportError(t *testing.T) {
	t.Setenv("GROQ_API_KEY", "gsk_k")
	a, _ := New(WithBaseURL("http://127.0.0.1:1"))
	_, err := a.Generate(context.Background(), basicRequest())
	if !errors.Is(err, model.ErrProviderUnavailable) {
		t.Errorf("err = %v, want wrap of ErrProviderUnavailable", err)
	}
}

func TestGenerate_respectsContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	t.Cleanup(srv.Close)

	t.Setenv("GROQ_API_KEY", "gsk_k")
	a, _ := New(WithBaseURL(srv.URL))
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	_, err := a.Generate(ctx, basicRequest())
	if err == nil {
		t.Fatal("expected error")
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

	t.Setenv("GROQ_API_KEY", "gsk_k")
	a, _ := New(WithBaseURL(srv.URL), WithRequestTimeout(20*time.Millisecond))
	start := time.Now()
	_, err := a.Generate(context.Background(), basicRequest())
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

// --- The API key never appears in any error string ---

func TestGenerate_errorsDoNotLeakKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"Invalid API Key"}}`)
	}))
	t.Cleanup(srv.Close)

	secret := "gsk_TOPSECRET_must_never_appear" // #nosec G101 -- test sentinel string, asserts adapter does NOT surface it
	t.Setenv("GROQ_API_KEY", "")
	a, _ := New(WithBaseURL(srv.URL), WithAPIKey(secret))
	_, err := a.Generate(context.Background(), basicRequest())
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("error message leaked API key: %q", err.Error())
	}
}

func TestGenerate_errorsDoNotLeakKeyOnTransportError(t *testing.T) {
	secret := "gsk_TOPSECRET_must_never_appear" // #nosec G101 -- test sentinel string, asserts adapter does NOT surface it
	t.Setenv("GROQ_API_KEY", "")
	a, _ := New(WithBaseURL("http://127.0.0.1:1"), WithAPIKey(secret))
	_, err := a.Generate(context.Background(), basicRequest())
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("transport error message leaked API key: %q", err.Error())
	}
}

// --- parseRetryAfter unit cases ---

func TestParseRetryAfter(t *testing.T) {
	cases := []struct {
		raw  string
		want time.Duration
	}{
		{"", 0},
		{"   ", 0},
		{"30", 30 * time.Second},
		{" 7 ", 7 * time.Second},
		{"0", 0},
		{"-3", 0},
		{"nonsense", 0},
		{"Mon, 14 Jun 2026 18:00:00 GMT", 0}, // HTTP-date form, not supported in 4.2
	}
	for _, tc := range cases {
		if got := parseRetryAfter(tc.raw); got != tc.want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}

// --- Helpers ---

func basicRequest() *model.Request {
	return &model.Request{
		Model:    "llama-3.1-8b-instant",
		Messages: []model.Message{{Role: model.RoleUser, Content: "Hola"}},
	}
}

// Compile-time assertion that the adapter satisfies model.Model.
var _ model.Model = (*Adapter)(nil)
