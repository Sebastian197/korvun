// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package openaicompat

// RED suite for SP-A of the universal model gateway. Contract:
// docs/superpowers/specs/2026-08-22-universal-model-gateway.md (FINAL).
// Table-driven over the FR-GW-4 matrix; every row cites its AS.

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Sebastian197/korvun/internal/model"
)

// chatReq is a valid two-turn request (system + user) for round-trips.
func chatReq() *model.Request {
	return &model.Request{
		Model: "test-model",
		Messages: []model.Message{
			{Role: model.RoleSystem, Content: "you are terse"},
			{Role: model.RoleUser, Content: "hello"},
		},
	}
}

// okBody is a minimal valid compat response WITHOUT usage (AS-2 tolerates
// its absence).
func okBody(content string) string {
	return fmt.Sprintf(`{"model":"served-model","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":%q}}]}`, content)
}

// newAdapter builds an adapter against base with the given extra options.
func newAdapter(t *testing.T, base string, opts ...Option) *Adapter {
	t.Helper()
	a, err := New(append([]Option{WithBaseURL(base)}, opts...)...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

// --- AS-2: round-trip -------------------------------------------------

// TestGenerate_roundTrip pins the wire (AS-2): POST to EXACTLY
// <base path>/chat/completions, body {model, messages, stream:false} with
// the system prompt as a role:"system" message, Bearer auth, and the
// response mapped to a model.Response with the assistant role, Provider
// pinned to the provider constant (FR-GW-2) and ModelName from the body.
func TestGenerate_roundTrip(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(okBody("terse answer")))
	}))
	t.Cleanup(srv.Close)

	a := newAdapter(t, srv.URL+"/v1", WithAPIKey("sk_test_key"))
	resp, err := a.Generate(context.Background(), chatReq())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if gotPath != "/v1/chat/completions" {
		t.Errorf("path = %q, want %q (zero-magic exact append, FR-GW-1)", gotPath, "/v1/chat/completions")
	}
	if gotAuth != "Bearer sk_test_key" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer sk_test_key")
	}
	if gotBody["model"] != "test-model" {
		t.Errorf("body model = %v, want test-model", gotBody["model"])
	}
	if v, present := gotBody["stream"]; !present || v != false {
		t.Errorf("body stream = %v (present=%t), want explicit false", v, present)
	}
	msgs, _ := gotBody["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("body messages len = %d, want 2", len(msgs))
	}
	first, _ := msgs[0].(map[string]any)
	if first["role"] != "system" {
		t.Errorf("messages[0].role = %v, want system (H2: system-only lane)", first["role"])
	}
	if resp.Message.Role != model.RoleAssistant || resp.Message.Content != "terse answer" {
		t.Errorf("response message = %+v, want assistant/terse answer", resp.Message)
	}
	if resp.Provider != ProviderName {
		t.Errorf("Provider = %q, want %q (the Name() pin)", resp.Provider, ProviderName)
	}
	if resp.ModelName != "served-model" {
		t.Errorf("ModelName = %q, want served-model", resp.ModelName)
	}
}

// TestGenerate_refusalIsAssistantReply pins H2 (AS-2): a non-empty
// message.refusal with empty content IS the assistant reply — a valid
// response, never an error, never a fallback trigger.
func TestGenerate_refusalIsAssistantReply(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"model":"m","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"","refusal":"I cannot help with that."}}]}`))
	}))
	t.Cleanup(srv.Close)

	a := newAdapter(t, srv.URL)
	resp, err := a.Generate(context.Background(), chatReq())
	if err != nil {
		t.Fatalf("Generate on refusal: %v (a refusal is a VALID reply, H2)", err)
	}
	if resp.Message.Role != model.RoleAssistant || resp.Message.Content != "I cannot help with that." {
		t.Errorf("response = %+v, want the refusal text as the assistant reply", resp.Message)
	}
}

// --- AS-3: the FR-GW-4 matrix, table-driven ---------------------------

// quotaCodes is the CLOSED quotaExhaustedCodes contract (FR-GW-4/H5),
// hardcoded HERE as the pin — the test defines the contract, the
// implementation must match it.
var quotaCodes = []string{
	"insufficient_quota",
	"credit_balance_exhausted",
	"organization_spend_limit_exceeded",
	"project_spend_limit_exceeded",
	"organization_usage_limit_exceeded",
}

func TestGenerate_errorMatrix(t *testing.T) {
	const label = "MY_COMPAT_KEY_ENV"
	const secret = "sk_TOPSECRET_must_never_appear" // #nosec G101 -- test sentinel, asserted NOT to surface

	type row struct {
		name        string
		status      int
		body        string
		header      map[string]string
		opts        []Option
		wantIs      error
		wantRate    bool // errors.As *model.RateLimitError must succeed
		wantRetryAf string
		contains    []string
		notContains []string
	}

	rows := []row{
		{
			name: "401 with auth label names the label", status: 401,
			body: `{"error":{"message":"bad key","type":"invalid_request_error","code":"invalid_api_key"}}`,
			opts: []Option{WithAPIKey(secret), WithAuthLabel(label)}, wantIs: model.ErrAuthInvalid,
			contains: []string{label}, notContains: []string{secret},
		},
		{
			name: "401 without label stays generic", status: 401,
			body: `{"error":{"message":"bad key","type":"invalid_request_error","code":"invalid_api_key"}}`,
			opts: []Option{WithAPIKey(secret)}, wantIs: model.ErrAuthInvalid,
			notContains: []string{label, secret},
		},
		{
			name: "403 is permanent and makes no auth claim", status: 403,
			body: `{"error":{"message":"region blocked","type":"forbidden","code":"unsupported_country"}}`,
			opts: []Option{WithAPIKey(secret), WithAuthLabel(label)}, wantIs: model.ErrProviderResponse,
			notContains: []string{label, secret, "authentication failed"},
		},
		{
			name: "404 steers at model_id and base_url", status: 404,
			body:   `{"error":{"message":"model not found","type":"invalid_request_error","code":"model_not_found"}}`,
			wantIs: model.ErrProviderResponse,
		},
		{
			name: "408 is retryable-unavailable", status: 408,
			body: `{"error":{"message":"timeout","type":"timeout","code":""}}`, wantIs: model.ErrProviderUnavailable,
		},
		{
			name: "409 falls in the permanent default bucket", status: 409,
			body: `{"error":{"message":"conflict","type":"conflict","code":""}}`, wantIs: model.ErrProviderResponse,
		},
		{
			name: "429 unrecognized code is a genuine rate limit", status: 429,
			body:     `{"error":{"message":"slow down","type":"rate_limit_exceeded","code":"rate_limit_exceeded"}}`,
			wantRate: true,
		},
		{
			name: "429 Retry-After seconds is carried verbatim", status: 429,
			body:   `{"error":{"message":"slow down","type":"rate_limit_exceeded","code":""}}`,
			header: map[string]string{"Retry-After": "7"}, wantRate: true, wantRetryAf: "7s",
		},
		{
			name: "429 Retry-After HTTP-date form is ignored", status: 429,
			body:   `{"error":{"message":"slow down","type":"rate_limit_exceeded","code":""}}`,
			header: map[string]string{"Retry-After": "Fri, 22 Aug 2026 12:00:00 GMT"}, wantRate: true, wantRetryAf: "0s",
		},
		{
			name: "500 is retryable-unavailable", status: 500,
			body: `{"error":{"message":"boom","type":"server_error","code":""}}`, wantIs: model.ErrProviderUnavailable,
		},
	}
	// One row PER entry of the closed quota list (H5): permanent, never a
	// rate limit, text steers at quota/credit.
	for _, code := range quotaCodes {
		rows = append(rows, row{
			name: "429 quota code " + code + " is permanent", status: 429,
			body:   fmt.Sprintf(`{"error":{"message":"out of credit","type":%q,"code":%q}}`, code, code),
			wantIs: model.ErrProviderResponse,
		})
	}

	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for k, v := range tc.header {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(srv.Close)

			a := newAdapter(t, srv.URL, tc.opts...)
			_, err := a.Generate(context.Background(), chatReq())
			if err == nil {
				t.Fatalf("Generate: nil error, want a mapped failure")
			}
			if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
				t.Errorf("errors.Is(%v) = false for err %v", tc.wantIs, err)
			}
			var rle *model.RateLimitError
			if got := errors.As(err, &rle); got != tc.wantRate {
				t.Errorf("errors.As(*RateLimitError) = %t, want %t (err %v)", got, tc.wantRate, err)
			}
			if tc.wantRate && rle != nil && tc.wantRetryAf != "" && rle.RetryAfter.String() != tc.wantRetryAf {
				t.Errorf("RetryAfter = %s, want %s", rle.RetryAfter, tc.wantRetryAf)
			}
			if tc.wantRate && errors.Is(err, model.ErrProviderResponse) {
				t.Errorf("a genuine rate limit must not be permanent: %v", err)
			}
			for _, want := range tc.contains {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err.Error(), want)
				}
			}
			for _, ban := range tc.notContains {
				if strings.Contains(err.Error(), ban) {
					t.Errorf("error %q must not contain %q", err.Error(), ban)
				}
			}
		})
	}
}

// TestGenerate_transportRefused pins the transport row: connection
// refused ⇒ retryable ErrProviderUnavailable, and the key stays out of
// the error (the groq_test.go:458-488 pattern).
func TestGenerate_transportRefused(t *testing.T) {
	const secret = "sk_TOPSECRET_must_never_appear" // #nosec G101 -- test sentinel
	a := newAdapter(t, "http://127.0.0.1:1", WithAPIKey(secret))
	_, err := a.Generate(context.Background(), chatReq())
	if !errors.Is(err, model.ErrProviderUnavailable) {
		t.Errorf("err = %v, want ErrProviderUnavailable", err)
	}
	if err != nil && strings.Contains(err.Error(), secret) {
		t.Errorf("transport error leaked the key: %q", err.Error())
	}
}

// TestGenerate_tlsUntrustedIsPermanent pins the N1 matrix row: a TLS
// verification failure is PERMANENT (retrying does not repair trust) and
// the cause is detectable via errors.As on *tls.CertificateVerificationError.
func TestGenerate_tlsUntrustedIsPermanent(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(okBody("never trusted")))
	}))
	t.Cleanup(srv.Close)

	// Default client: the test server's self-signed cert is NOT in the pool.
	a := newAdapter(t, srv.URL)
	_, err := a.Generate(context.Background(), chatReq())
	if !errors.Is(err, model.ErrProviderResponse) {
		t.Errorf("err = %v, want permanent ErrProviderResponse (N1)", err)
	}
	var cve *tls.CertificateVerificationError
	if !errors.As(err, &cve) {
		t.Errorf("errors.As(*tls.CertificateVerificationError) = false for err %v", err)
	}
}

// TestGenerate_embeddedErrorNeverUsesContent pins H1 (AS-3): an HTTP 200
// carrying an embedded error — top-level error object, or
// finish_reason=="error" — is a permanent ErrProviderResponse and any
// partial content is NEVER used.
func TestGenerate_embeddedErrorNeverUsesContent(t *testing.T) {
	cases := []struct{ name, body string }{
		{"top-level error object", `{"error":{"message":"mid-stream failure","type":"server_error","code":""},"model":"m","choices":[{"message":{"role":"assistant","content":"partial junk"}}]}`},
		{"finish_reason error", `{"model":"m","choices":[{"finish_reason":"error","message":{"role":"assistant","content":"partial junk"}}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(srv.Close)

			resp, err := a200(t, srv.URL)
			if !errors.Is(err, model.ErrProviderResponse) {
				t.Errorf("err = %v, want permanent ErrProviderResponse (H1)", err)
			}
			if resp != nil {
				t.Errorf("resp = %+v, want nil — partial content must NEVER be used", resp)
			}
		})
	}
}

// a200 runs one Generate against base and returns the raw pair.
func a200(t *testing.T, base string) (*model.Response, error) {
	t.Helper()
	return newAdapter(t, base).Generate(context.Background(), chatReq())
}

// TestGenerate_malformed2xx pins the H12 enumeration (AS-3): each
// malformed-success shape maps to ErrProviderResponse.
func TestGenerate_malformed2xx(t *testing.T) {
	cases := []struct{ name, body string }{
		{"empty body", ""},
		{"invalid JSON", `{"model": nope`},
		{"valid JSON plus trailing garbage", okBody("x") + `{"second":"doc"}`},
		{"empty choices", `{"model":"m","choices":[]}`},
		{"empty content without refusal", `{"model":"m","choices":[{"message":{"role":"assistant","content":""}}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(srv.Close)

			resp, err := a200(t, srv.URL)
			if !errors.Is(err, model.ErrProviderResponse) {
				t.Errorf("err = %v, want ErrProviderResponse (H12: %s)", err, tc.name)
			}
			if resp != nil {
				t.Errorf("resp = %+v, want nil", resp)
			}
		})
	}
}

// TestGenerate_successBodyBound pins H3 (AS-3): a success body of exactly
// maxResponseBytes is accepted; one byte more fails naming the cap.
func TestGenerate_successBodyBound(t *testing.T) {
	frame := okBody("")
	pad := maxResponseBytes - len(frame)

	run := func(t *testing.T, extra int) (*model.Response, error) {
		t.Helper()
		body := okBody(strings.Repeat("a", pad+extra))
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(body))
		}))
		t.Cleanup(srv.Close)
		return a200(t, srv.URL)
	}

	t.Run("exactly at the limit succeeds", func(t *testing.T) {
		if _, err := run(t, 0); err != nil {
			t.Errorf("Generate at exactly maxResponseBytes: %v, want success", err)
		}
	})
	t.Run("one over the limit fails naming the cap", func(t *testing.T) {
		_, err := run(t, 1)
		if !errors.Is(err, model.ErrProviderResponse) {
			t.Fatalf("err = %v, want ErrProviderResponse naming the cap", err)
		}
		if !strings.Contains(err.Error(), fmt.Sprint(maxResponseBytes)) {
			t.Errorf("error %q does not name the cap %d", err.Error(), maxResponseBytes)
		}
	})
}

// TestGenerate_noKeySendsNoAuthorization pins N5 (AS-1, exercised for
// real): with api_key absent, a REAL request is made and it carries NO
// Authorization header — asserted by the server on the received request.
func TestGenerate_noKeySendsNoAuthorization(t *testing.T) {
	var hits int32
	var sawAuthHeader atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if _, present := r.Header["Authorization"]; present {
			sawAuthHeader.Store(true)
		}
		_, _ = w.Write([]byte(okBody("hi")))
	}))
	t.Cleanup(srv.Close)

	a := newAdapter(t, srv.URL) // no WithAPIKey
	if _, err := a.Generate(context.Background(), chatReq()); err != nil {
		t.Fatalf("Generate without key against no-auth server: %v", err)
	}
	if n := atomic.LoadInt32(&hits); n != 1 {
		t.Fatalf("server hits = %d, want 1 — the request must actually run (N5)", n)
	}
	if sawAuthHeader.Load() {
		t.Error("request carried an Authorization header; want NONE when no key is set")
	}
}

// TestGenerate_hostileEchoNeverLeaksKey pins the H8 echo rows (AS-3): a
// server that reflects the literal Bearer value back — in the JSON error
// envelope AND in a plain-text body — must not make the key appear in any
// returned error string.
func TestGenerate_hostileEchoNeverLeaksKey(t *testing.T) {
	const secret = "sk_TOPSECRET_must_never_appear" // #nosec G101 -- test sentinel

	cases := []struct {
		name    string
		handler http.HandlerFunc
		wantIs  error
	}{
		{
			name: "echo in the JSON error envelope",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(401)
				_, _ = w.Write([]byte(fmt.Sprintf(`{"error":{"message":"got header %s","type":"invalid_request_error","code":"invalid_api_key"}}`, r.Header.Get("Authorization"))))
			},
			wantIs: model.ErrAuthInvalid,
		},
		{
			name: "echo in a plain-text error body",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(400)
				_, _ = w.Write([]byte("rejected: " + r.Header.Get("Authorization")))
			},
			wantIs: model.ErrProviderResponse,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			t.Cleanup(srv.Close)

			a := newAdapter(t, srv.URL, WithAPIKey(secret))
			_, err := a.Generate(context.Background(), chatReq())
			if !errors.Is(err, tc.wantIs) {
				t.Errorf("err = %v, want %v", err, tc.wantIs)
			}
			if err != nil && strings.Contains(err.Error(), secret) {
				t.Errorf("error leaked the echoed key: %q", err.Error())
			}
		})
	}
}

// TestGenerate_redirectRefused pins FR-GW-7 (AS-6): any 3xx is refused —
// permanent error naming the refusal — and the redirect target receives
// NEITHER the body NOR the Authorization header.
func TestGenerate_redirectRefused(t *testing.T) {
	var spyHits int32
	spy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&spyHits, 1)
		_, _ = w.Write([]byte(okBody("stolen")))
	}))
	t.Cleanup(spy.Close)

	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, new(http.Request), spy.URL+"/chat/completions", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(primary.Close)

	a := newAdapter(t, primary.URL, WithAPIKey("sk_redirect_bait"))
	_, err := a.Generate(context.Background(), chatReq())
	if !errors.Is(err, model.ErrProviderResponse) {
		t.Errorf("err = %v, want permanent ErrProviderResponse", err)
	}
	if err != nil && !strings.Contains(err.Error(), "refusing to follow") {
		t.Errorf("error %q does not carry the refusal diagnostic", err.Error())
	}
	if n := atomic.LoadInt32(&spyHits); n != 0 {
		t.Errorf("spy hits = %d, want 0 — nothing may reach the redirect target", n)
	}
}

// --- Formatting hygiene (mold groq_test.go:115-128) -------------------

// TestFormatting_neverLeaksKey is the mold's formatting pin: no default
// formatting of the adapter surfaces the key. Green from the stub on —
// an anchor, not a red row.
func TestFormatting_neverLeaksKey(t *testing.T) {
	const secret = "sk_TOPSECRET_must_never_appear" // #nosec G101 -- test sentinel
	a, _ := New(WithBaseURL("http://x"), WithAPIKey(secret))
	for _, got := range []string{
		fmt.Sprintf("%v", a), fmt.Sprintf("%+v", a), fmt.Sprintf("%#v", a), a.String(),
	} {
		if strings.Contains(got, secret) {
			t.Errorf("formatting leaked the key: %q", got)
		}
	}
}
