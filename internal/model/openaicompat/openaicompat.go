// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package openaicompat

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Sebastian197/korvun/internal/model"
)

const chatPath = "/chat/completions"

// maxErrorBodyBytes caps how much of a non-2xx response body is read into
// the wrapped error so a misbehaving server cannot load a giant payload
// into the error chain (the groq mold, groq.go:25).
const maxErrorBodyBytes = 1 << 10 // 1 KiB

// errRedirectRefused is returned by the CheckRedirect hook (FR-GW-7); any
// 3xx surfaces through client.Do wrapping it.
var errRedirectRefused = errors.New("endpoint redirected — refusing to follow")

// quotaExhaustedCodes is the CLOSED set partitioning 429s (FR-GW-4/H5):
// an envelope whose error.code OR error.type matches is quota/billing
// exhaustion — permanent, operator work, zero retry. Every entry is
// source-cited in the spec; extensions land here, one line each.
var quotaExhaustedCodes = map[string]bool{
	"insufficient_quota":                true,
	"credit_balance_exhausted":          true,
	"organization_spend_limit_exceeded": true,
	"project_spend_limit_exceeded":      true,
	"organization_usage_limit_exceeded": true,
}

// Adapter is the openai-compatible implementation of model.Model. Safe for
// concurrent use as long as the underlying *http.Client is.
//
// The Adapter holds the API key in an unexported field. It is never
// returned by any method, never logged, never reflected into any error
// message; String/GoString redact it from any "%v"/"%+v"/"%#v" formatting
// (the groq mold, ADR-0010 §3), and the H8 belt literally redacts it from
// every body/header-derived diagnostic.
type Adapter struct {
	baseURL   string
	apiKey    string // SECRET — never log, never expose; empty is VALID (no-auth local servers)
	authLabel string // non-secret NAME of api_key_env, for 401 diagnostics (H7)
	client    *http.Client
	timeout   time.Duration
}

// Option configures the Adapter at construction time.
type Option func(*Adapter)

// WithBaseURL sets the FULL endpoint prefix the operator declared
// (FR-GW-1 zero-magic rule). Trailing "/" is trimmed; the adapter appends
// exactly "/chat/completions" at request time.
func WithBaseURL(u string) Option {
	return func(a *Adapter) { a.baseURL = strings.TrimRight(u, "/") }
}

// WithAPIKey supplies the resolved API key. Empty is valid: no
// Authorization header is sent (no-auth local servers). There is NO
// env-var fallback in this package — the wiring resolves api_key_env.
func WithAPIKey(key string) Option {
	return func(a *Adapter) { a.apiKey = key }
}

// WithAuthLabel supplies the non-secret NAME of the env var the key was
// resolved from (H7). 401 diagnostics include it when present and stay
// generic when absent. Never key material.
func WithAuthLabel(label string) Option {
	return func(a *Adapter) { a.authLabel = label }
}

// WithHTTPClient injects a custom *http.Client (tests, shared
// transports). The adapter installs its redirect-refusing CheckRedirect
// (FR-GW-7) on a COPY, so the caller's client is never mutated.
func WithHTTPClient(c *http.Client) Option {
	return func(a *Adapter) { a.client = c }
}

// WithRequestTimeout sets a per-call deadline derived from the caller's
// ctx. Zero or negative disables the wrapper (the call is then bounded
// only by the caller's ctx) — the groq mold's semantics.
func WithRequestTimeout(d time.Duration) Option {
	return func(a *Adapter) { a.timeout = d }
}

// New builds an Adapter with the supplied options applied in order. An
// empty key is valid (FR-GW-2); base_url validity is the config layer's
// job (FR-GW-1), so New performs no network call and no URL re-check.
// The redirect-refusing CheckRedirect (FR-GW-7) is installed on a copy of
// whatever client the adapter ends up with.
func New(opts ...Option) (*Adapter, error) {
	a := &Adapter{client: &http.Client{}}
	for _, opt := range opts {
		opt(a)
	}
	guarded := *a.client
	guarded.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errRedirectRefused
	}
	a.client = &guarded
	return a, nil
}

// Name implements model.Model. Returns ProviderName ("openai-compatible").
func (a *Adapter) Name() string { return ProviderName }

// String redacts the API key from any default formatting.
func (a *Adapter) String() string {
	return fmt.Sprintf("openaicompat.Adapter{baseURL=%s, hasAPIKey=%t, authLabel=%s, hasClient=%t, timeout=%s}",
		a.baseURL, a.apiKey != "", a.authLabel, a.client != nil, a.timeout)
}

// GoString matches the same redaction for "%#v".
func (a *Adapter) GoString() string { return a.String() }

// redact is the H8 belt: every diagnostic derived from a response body or
// header passes through here BEFORE being wrapped, replacing the resolved
// key literally — a hostile server echoing the Bearer value back cannot
// plant it in an error string.
func (a *Adapter) redact(s string) string {
	if a.apiKey == "" {
		return s
	}
	return strings.ReplaceAll(s, a.apiKey, "[REDACTED]")
}

// Generate implements model.Model per the FR-GW-2/3/4/7 contract: POST
// {model, messages, stream:false} to EXACTLY baseURL+"/chat/completions",
// Bearer only with a non-empty key, the FR-GW-4 error matrix, redirect
// refusal, and the bounded, order-fixed 2xx processing.
func (a *Adapter) Generate(ctx context.Context, req *model.Request) (*model.Response, error) {
	if err := model.ValidateRequest(req); err != nil {
		return nil, err
	}

	if a.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.timeout)
		defer cancel()
	}

	payload := chatRequest{
		Model:    req.Model,
		Messages: toChatMessages(req.Messages),
		Stream:   false,
	}
	body, err := json.Marshal(&payload)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal request: %w", model.ErrProviderResponse, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+chatPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %w", model.ErrProviderResponse, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if a.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	}

	resp, err := a.client.Do(httpReq)
	if err != nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return nil, a.mapTransportError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, a.mapHTTPError(resp)
	}
	return a.decodeSuccess(resp)
}

// mapTransportError classifies a client.Do failure (FR-GW-4): redirect
// refusal (FR-GW-7) and TLS verification failure (N1) are PERMANENT and
// checked BEFORE the generic transport bucket; everything else is a
// retryable ErrProviderUnavailable. Wrapping uses %w throughout so the
// decorator's F6 check still sees context.DeadlineExceeded (N3) and
// errors.As still finds the TLS cause.
func (a *Adapter) mapTransportError(err error) error {
	if errors.Is(err, errRedirectRefused) {
		return fmt.Errorf("%w: endpoint redirected — refusing to follow", model.ErrProviderResponse)
	}
	var cve *tls.CertificateVerificationError
	if errors.As(err, &cve) {
		return fmt.Errorf("%w: tls verification failed (retrying does not repair trust): %w",
			model.ErrProviderResponse, err)
	}
	return fmt.Errorf("%w: %w", model.ErrProviderUnavailable, err)
}

// decodeSuccess reads a 2xx body under the maxResponseBytes bound and
// applies the FIXED processing order (FR-GW-3/H1): top-level error →
// finish_reason=="error" → refusal → content. Partial content behind an
// embedded error is NEVER used; the decoder DEMANDS EOF, so trailing
// garbage after the JSON document fails (H12).
func (a *Adapter) decodeSuccess(resp *http.Response) (*model.Response, error) {
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read response: %w", model.ErrProviderUnavailable, err)
	}
	if len(raw) > maxResponseBytes {
		return nil, fmt.Errorf("%w: response body exceeds the %d-byte cap (maxResponseBytes)",
			model.ErrProviderResponse, maxResponseBytes)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("%w: empty response body", model.ErrProviderResponse)
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	var apiResp chatResponse
	if err := dec.Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("%w: decode response: %s", model.ErrProviderResponse, a.redact(err.Error()))
	}
	if err := dec.Decode(new(json.RawMessage)); err != io.EOF {
		return nil, fmt.Errorf("%w: trailing data after the response document", model.ErrProviderResponse)
	}

	// (1) A 200 carrying an embedded top-level error is an error response;
	// any partial content is NEVER used (H1).
	if apiResp.Error != nil {
		return nil, fmt.Errorf("%w: error in 200 body: %s",
			model.ErrProviderResponse, a.redact(formatErrorDetail(*apiResp.Error)))
	}
	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("%w: response carried no choices", model.ErrProviderResponse)
	}
	choice := apiResp.Choices[0]
	// (2) finish_reason=="error" is the second embedded-error form (H1).
	if choice.FinishReason == "error" {
		return nil, fmt.Errorf("%w: response finished with reason %q", model.ErrProviderResponse, "error")
	}
	// (3) A non-empty refusal with empty content IS the assistant reply (H2).
	content := choice.Message.Content
	if content == "" && choice.Message.Refusal != "" {
		content = choice.Message.Refusal
	}
	// (4) Otherwise empty content is malformed (H12).
	if content == "" {
		return nil, fmt.Errorf("%w: response had empty assistant content and no refusal", model.ErrProviderResponse)
	}

	return &model.Response{
		Message: model.Message{
			Role:    model.RoleAssistant,
			Content: content,
		},
		Provider:  ProviderName,
		ModelName: apiResp.Model,
	}, nil
}

// mapHTTPError translates a non-2xx response into the FR-GW-4 matrix.
// Body reading is capped at maxErrorBodyBytes; every diagnostic passes
// the H8 redaction belt; the key never appears — 401 names the
// WithAuthLabel label instead, when present.
func (a *Adapter) mapHTTPError(resp *http.Response) error {
	rawBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	var env errorEnvelope
	_ = json.Unmarshal(rawBody, &env)
	snippet := a.redact(decodeErrorSnippet(rawBody, env))

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		if a.authLabel != "" {
			return fmt.Errorf("%w: status 401: check %s: %s", model.ErrAuthInvalid, a.authLabel, snippet)
		}
		return fmt.Errorf("%w: status 401: %s", model.ErrAuthInvalid, snippet)
	case resp.StatusCode == http.StatusForbidden:
		// Deliberately NOT ErrAuthInvalid (H7): a 403 can be region,
		// permissions, or moderation — the text orients without asserting
		// credentials, and never names the auth label.
		return fmt.Errorf("%w: status 403 (forbidden — region, permissions, or moderation): %s",
			model.ErrProviderResponse, snippet)
	case resp.StatusCode == http.StatusNotFound:
		return fmt.Errorf("%w: status 404: check model_id and the base_url prefix: %s",
			model.ErrProviderResponse, snippet)
	case resp.StatusCode == http.StatusRequestTimeout:
		return fmt.Errorf("%w: status 408: %s", model.ErrProviderUnavailable, snippet)
	case resp.StatusCode == http.StatusTooManyRequests:
		if code, quota := quotaCode(env); quota {
			return fmt.Errorf("%w: status 429: quota/credit exhausted (%s) — add credit or raise the limit; retrying will not help: %s",
				model.ErrProviderResponse, code, snippet)
		}
		rle := &model.RateLimitError{
			Provider:   ProviderName,
			RetryAfter: model.ParseRetryAfter(resp.Header.Get("Retry-After")),
		}
		return fmt.Errorf("openai-compatible: status 429: %s: %w", snippet, rle)
	case resp.StatusCode >= 500:
		return fmt.Errorf("%w: status %d: %s", model.ErrProviderUnavailable, resp.StatusCode, snippet)
	default:
		return fmt.Errorf("%w: status %d: %s", model.ErrProviderResponse, resp.StatusCode, snippet)
	}
}

// quotaCode reports whether the structured envelope carries a
// quotaExhaustedCodes entry in error.code or error.type (H5), returning
// the matched value.
func quotaCode(env errorEnvelope) (string, bool) {
	if env.Error == nil {
		return "", false
	}
	if quotaExhaustedCodes[env.Error.Code] {
		return env.Error.Code, true
	}
	if quotaExhaustedCodes[env.Error.Type] {
		return env.Error.Type, true
	}
	return "", false
}

// decodeErrorSnippet produces a one-line diagnostic from the structured
// envelope when present, falling back to the trimmed raw body. Only the
// documented envelope fields are reflected — never headers.
func decodeErrorSnippet(raw []byte, env errorEnvelope) string {
	if env.Error != nil && env.Error.Message != "" {
		return formatErrorDetail(*env.Error)
	}
	if len(raw) == 0 {
		return "(empty body)"
	}
	return strings.TrimSpace(string(raw))
}

func formatErrorDetail(d errorDetail) string {
	return fmt.Sprintf("type=%s code=%s message=%s", d.Type, d.Code, d.Message)
}

// chatRequest is the minimal subset of the compat /chat/completions
// request body Korvun sends (FR-GW-3). Stream is sent explicitly false.
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

// chatMessage models the role-tagged conversation turn on the wire.
// Refusal only ever appears on responses; omitempty keeps requests clean.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Refusal string `json:"refusal,omitempty"`
}

// chatResponse models the fields Korvun reads from a non-streaming compat
// response, INCLUDING the embedded-error surfaces the H1 order inspects.
// usage is deliberately not modelled (tolerated-optional).
type chatResponse struct {
	Model   string       `json:"model"`
	Error   *errorDetail `json:"error"`
	Choices []chatChoice `json:"choices"`
}

// chatChoice holds the structured message plus finish_reason (H1).
type chatChoice struct {
	Index        int         `json:"index"`
	FinishReason string      `json:"finish_reason"`
	Message      chatMessage `json:"message"`
}

// errorEnvelope models the OpenAI-style error JSON, shared by the 4xx/5xx
// bodies and the embedded top-level error of a 200 (H1).
type errorEnvelope struct {
	Error *errorDetail `json:"error"`
}

type errorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// toChatMessages maps the canonical model.Message slice to the wire
// format. Role strings come from model.Role.String, which already emits
// lowercase "system"/"user"/"assistant" (H2: the system-only lane).
func toChatMessages(in []model.Message) []chatMessage {
	out := make([]chatMessage, len(in))
	for i, m := range in {
		out[i] = chatMessage{
			Role:    m.Role.String(),
			Content: m.Content,
		}
	}
	return out
}
