// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Sebastian197/korvun/internal/model"
)

const chatPath = "/api/chat"

// maxErrorBodyBytes caps how much of a non-2xx response body is
// captured into the wrapped error so a misbehaving server cannot
// load a giant payload into the error chain.
const maxErrorBodyBytes = 1 << 10 // 1 KiB

// Adapter is the Ollama implementation of model.Model. It is safe
// for concurrent use as long as the underlying *http.Client is.
type Adapter struct {
	baseURL string
	client  *http.Client
	timeout time.Duration
}

// Option configures the Adapter at construction time.
type Option func(*Adapter)

// WithBaseURL overrides the address the adapter sends chat requests
// to. Trailing "/" is trimmed. If never used, the adapter falls
// back to the OLLAMA_HOST env var, then to DefaultBaseURL.
func WithBaseURL(u string) Option {
	return func(a *Adapter) {
		a.baseURL = strings.TrimRight(u, "/")
	}
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
// Defaults: baseURL from OLLAMA_HOST or DefaultBaseURL, a fresh
// *http.Client with no extra configuration, no per-call timeout.
func New(opts ...Option) *Adapter {
	a := &Adapter{
		baseURL: defaultBaseURL(),
		client:  &http.Client{},
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Name implements model.Model. Returns ProviderName ("ollama").
func (a *Adapter) Name() string { return ProviderName }

// Generate implements model.Model. Validates the request, POSTs
// /api/chat with stream:false, decodes the single-shot response,
// and wraps it into a *model.Response.
//
// Errors:
//   - validation: the model.Err* sentinels from ValidateRequest.
//   - transport failure (network down, server unreachable, ctx
//     cancelled mid-flight): wraps model.ErrProviderUnavailable.
//   - non-2xx, malformed JSON, or response with empty assistant
//     content: wraps model.ErrProviderResponse.
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

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+chatPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %w", model.ErrProviderResponse, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", model.ErrProviderUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		return nil, fmt.Errorf("%w: status %d: %s",
			model.ErrProviderResponse,
			resp.StatusCode,
			strings.TrimSpace(string(snippet)))
	}

	var apiResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("%w: decode response: %w", model.ErrProviderResponse, err)
	}

	if apiResp.Message.Content == "" {
		return nil, fmt.Errorf("%w: response had empty assistant content",
			model.ErrProviderResponse)
	}

	return &model.Response{
		Message: model.Message{
			Role:    model.RoleAssistant,
			Content: apiResp.Message.Content,
		},
		Provider:  ProviderName,
		ModelName: apiResp.Model,
	}, nil
}

// chatRequest is the minimal subset of the Ollama /api/chat
// request body Korvun sends. Verified against the v0.30.8
// api.ChatRequest struct.
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

// chatMessage models the role-tagged conversation turn. Role is
// the lowercase string every Ollama-supported model expects
// (Ollama's own UnmarshalJSON lowercases incoming values, so the
// adapter sends them already lowercase).
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatResponse models the fields Korvun reads from a non-streaming
// Ollama /api/chat response. Other fields (CreatedAt, DoneReason,
// the embedded Metrics) are ignored by encoding/json's default
// behaviour, which is exactly the forward-compat property we want.
type chatResponse struct {
	Model   string      `json:"model"`
	Message chatMessage `json:"message"`
	Done    bool        `json:"done"`
}

// toChatMessages maps the canonical model.Message slice to the
// wire-format chatMessage slice. Role strings come from
// model.Role.String, which already produces lowercase "system" /
// "user" / "assistant"; any unknown role would round-trip as
// "unknown(N)" and Ollama would reject the request — exactly the
// loud failure mode ADR-0009 §1 calls for.
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

// defaultBaseURL resolves the baseURL the adapter uses when
// WithBaseURL was not supplied. Honors the OLLAMA_HOST env var
// (used by every Ollama tool); accepts "host:port", "host", or
// a full "scheme://host[:port]". Falls back to DefaultBaseURL.
func defaultBaseURL() string {
	v := strings.TrimSpace(os.Getenv("OLLAMA_HOST"))
	if v == "" {
		return DefaultBaseURL
	}
	v = strings.TrimRight(v, "/")
	if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
		return v
	}
	return "http://" + v
}
