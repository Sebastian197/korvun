// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package groq

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Sebastian197/korvun/internal/model"
)

const chatPath = "/chat/completions"

// maxErrorBodyBytes caps how much of a non-2xx response body is
// read into the wrapped error so a misbehaving server cannot load
// a giant payload into the error chain.
const maxErrorBodyBytes = 1 << 10 // 1 KiB

// Adapter is the Groq implementation of model.Model. Safe for
// concurrent use as long as the underlying *http.Client is.
//
// The Adapter holds the API key in an unexported field. It is
// never returned by any method, never logged, never reflected
// into any error message. The custom String / GoString methods
// below redact it from any "%v" / "%+v" / "%#v" formatting so an
// accidental log of the Adapter does not leak the secret.
type Adapter struct {
	baseURL string
	apiKey  string // SECRET — never log, never expose
	client  *http.Client
	timeout time.Duration
}

// Option configures the Adapter at construction time.
type Option func(*Adapter)

// WithBaseURL overrides the address the adapter sends chat
// completion requests to. Trailing "/" is trimmed. Defaults to
// DefaultBaseURL.
func WithBaseURL(u string) Option {
	return func(a *Adapter) {
		a.baseURL = strings.TrimRight(u, "/")
	}
}

// WithAPIKey supplies the Groq API key directly. Wins over the
// GROQ_API_KEY env var. The provided value is stored in the
// adapter's unexported field and never surfaces back.
func WithAPIKey(key string) Option {
	return func(a *Adapter) { a.apiKey = key }
}

// WithHTTPClient injects a custom *http.Client. Useful for tests
// (httptest server clients) and for callers that want to share a
// transport across providers.
func WithHTTPClient(c *http.Client) Option {
	return func(a *Adapter) { a.client = c }
}

// WithRequestTimeout sets a per-call deadline derived from the
// caller's ctx. Zero or negative disables the wrapper; the call is
// then bounded only by the caller's ctx.
func WithRequestTimeout(d time.Duration) Option {
	return func(a *Adapter) { a.timeout = d }
}

// New builds an Adapter with the supplied options applied in order.
//
// Key-resolution chain (ADR-0010 §3):
//
//  1. The GROQ_API_KEY env var is loaded as a default.
//  2. Caller-supplied options are applied in declaration order;
//     WithAPIKey wins over the env value when both are present.
//  3. If the final resolved key is empty, New returns
//     ErrMissingAPIKey. No network call is attempted.
//
// New does NOT attempt a heartbeat call against Groq; transport
// readiness is verified on the first Generate. This matches the
// fail-fast-on-config / lazy-on-network split the rest of Korvun
// uses.
func New(opts ...Option) (*Adapter, error) {
	a := &Adapter{
		baseURL: DefaultBaseURL,
		client:  &http.Client{},
	}
	if v := strings.TrimSpace(os.Getenv("GROQ_API_KEY")); v != "" {
		a.apiKey = v
	}
	for _, opt := range opts {
		opt(a)
	}
	if strings.TrimSpace(a.apiKey) == "" {
		return nil, ErrMissingAPIKey
	}
	return a, nil
}

// Name implements model.Model. Returns ProviderName ("groq").
func (a *Adapter) Name() string { return ProviderName }

// String redacts the API key from any default formatting. The
// adapter is rarely fmt'd directly, but if it is, this prevents a
// "%v" log from leaking the secret.
func (a *Adapter) String() string {
	return fmt.Sprintf("groq.Adapter{baseURL=%s, hasAPIKey=%t, timeout=%s}",
		a.baseURL, a.apiKey != "", a.timeout)
}

// GoString matches the same redaction for "%#v".
func (a *Adapter) GoString() string { return a.String() }

// Generate implements model.Model. Validates the request, POSTs
// /chat/completions with stream:false and a Bearer token, decodes
// choices[0].message.content into a *model.Response.
//
// Errors map to internal/model sentinels per ADR-0010 §4:
//
//   - validation: the model.Err* sentinels from ValidateRequest.
//   - network / ctx cancel mid-flight / non-HTTP transport
//     failure: wraps model.ErrProviderUnavailable.
//   - 401 / 403: wraps model.ErrAuthInvalid.
//   - 429: wraps *model.RateLimitError (Provider: "groq",
//     RetryAfter: from the Retry-After header or 0).
//   - 5xx: wraps model.ErrProviderUnavailable.
//   - 400 / 404 / other 4xx: wraps model.ErrProviderResponse.
//   - 2xx with malformed body / empty choices / empty content:
//     wraps model.ErrProviderResponse.
//
// No retry is attempted inside this method; the caller (Brain or
// the Phase 4.3 fan-out) decides whether and when to retry.
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
	httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", model.ErrProviderUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, mapHTTPError(resp)
	}

	var apiResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("%w: decode response: %w", model.ErrProviderResponse, err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("%w: response carried no choices", model.ErrProviderResponse)
	}
	content := apiResp.Choices[0].Message.Content
	if content == "" {
		return nil, fmt.Errorf("%w: response had empty assistant content", model.ErrProviderResponse)
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

// mapHTTPError translates a non-2xx response into the right
// internal/model sentinel per ADR-0010 §4. Body reading is capped
// at maxErrorBodyBytes to defuse a misbehaving server. The
// API key is never reflected into the returned error; only the
// response-side diagnostic information (status, Groq error type /
// code / message snippet, retry-after for 429) appears.
func mapHTTPError(resp *http.Response) error {
	rawBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	snippet := decodeErrorSnippet(rawBody)

	switch {
	case resp.StatusCode == http.StatusUnauthorized,
		resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("%w: status %d: %s",
			model.ErrAuthInvalid, resp.StatusCode, snippet)
	case resp.StatusCode == http.StatusTooManyRequests:
		rle := &model.RateLimitError{
			Provider:   ProviderName,
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
		}
		return fmt.Errorf("groq: status 429: %s: %w", snippet, rle)
	case resp.StatusCode >= 500:
		return fmt.Errorf("%w: status %d: %s",
			model.ErrProviderUnavailable, resp.StatusCode, snippet)
	default:
		return fmt.Errorf("%w: status %d: %s",
			model.ErrProviderResponse, resp.StatusCode, snippet)
	}
}

// decodeErrorSnippet attempts to unmarshal the standard Groq error
// envelope and produce a one-line diagnostic. Falls back to a
// trimmed raw body when the body is not JSON-shaped.
func decodeErrorSnippet(raw []byte) string {
	if len(raw) == 0 {
		return "(empty body)"
	}
	var env errorEnvelope
	if err := json.Unmarshal(raw, &env); err == nil && env.Error.Message != "" {
		// Use only the structured fields. Never reflect headers or
		// anything outside the documented error envelope.
		return fmt.Sprintf("type=%s code=%s message=%s",
			env.Error.Type, env.Error.Code, env.Error.Message)
	}
	return strings.TrimSpace(string(raw))
}

// parseRetryAfter reads the Retry-After header value as an integer
// number of seconds and returns it as a time.Duration. Empty or
// unparseable values return zero — the consumer interprets zero as
// "no hint given".
//
// Groq's documented behaviour is to set Retry-After to a seconds
// integer on 429. The HTTP spec also allows an HTTP-date form,
// which Groq does not use today; if a future Groq behaviour
// introduces it, parseRetryAfter would need extending. For 4.2 the
// seconds-only path is sufficient and explicit.
func parseRetryAfter(raw string) time.Duration {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0
	}
	return time.Duration(n) * time.Second
}

// chatRequest is the minimal subset of the Groq /chat/completions
// request body Korvun sends. Verified against the live API
// (console.groq.com/docs/api-reference). Stream is sent
// explicitly false because the Groq default is true.
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

// chatMessage models the role-tagged conversation turn. Roles are
// lowercase ("system", "user", "assistant") — the same shape every
// OpenAI-compatible API uses, identical to what model.Role.String()
// emits.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatResponse models the fields Korvun reads from a non-streaming
// Groq /chat/completions response. The id/object/created/finish_reason
// /usage fields plus the rate-limit headers are deliberately not
// modelled here; surfacing them is observability territory.
type chatResponse struct {
	Model   string       `json:"model"`
	Choices []chatChoice `json:"choices"`
}

// chatChoice holds the structured message and the unread fields
// Korvun safely discards.
type chatChoice struct {
	Index   int         `json:"index"`
	Message chatMessage `json:"message"`
}

// errorEnvelope models the Groq error JSON. Verified against a
// live HTTPS probe (401 from a request without an Authorization
// header) on 2026-06-14: the body is
//
//	{"error":{"message":"...","type":"...","code":"..."}}
//
// All three inner fields are reflected into the diagnostic string
// for 4xx / 5xx responses; the key value is NOT.
type errorEnvelope struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// toChatMessages maps the canonical model.Message slice to the
// wire-format chatMessage slice. Role strings come from
// model.Role.String, which already produces lowercase
// "system" / "user" / "assistant".
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
