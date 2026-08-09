// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultWebhookMaxBytes caps a webhook response when the operator does not
// set one. Prompt-sized, same reasoning as DefaultFetchMaxBytes.
const DefaultWebhookMaxBytes = 64 * 1024

// DefaultWebhookTimeout is the hard per-call bound when unset (ADR-0041 §4).
// The brain's per-tool ctx still applies; the tighter of the two wins.
const DefaultWebhookTimeout = 10 * time.Second

// WebhookCallConfig is the operator cage of the webhook_call tool (ADR-0041
// §4) — the user's no-code tool factory and the n8n door (the full n8n
// bridge stays post-beta).
type WebhookCallConfig struct {
	// AllowHosts is the exact-host allow-list, same semantics as http_fetch.
	AllowHosts []string
	// MaxBytes caps the response body (0 => DefaultWebhookMaxBytes).
	MaxBytes int64
	// Timeout is the hard per-call bound (0 => DefaultWebhookTimeout).
	Timeout time.Duration
	// PrivateOnly arms the network shield, same semantics as http_fetch.
	PrivateOnly bool
}

// webhookCallTool POSTs a JSON payload to an allow-listed endpoint. It NEVER
// follows a redirect (a redirected POST is how a listed host would smuggle
// the payload elsewhere): the caged client's hop bound is zero, so the first
// redirect dies at the cage.
type webhookCallTool struct {
	allow    allowList
	maxBytes int64
	timeout  time.Duration
	client   *http.Client
}

// WebhookCall constructs the caged webhook_call tool, failing loud at wiring
// on an empty or blank allow-list.
func WebhookCall(cfg WebhookCallConfig) (Tool, error) {
	allow, err := newAllowList("webhook_call", cfg.AllowHosts)
	if err != nil {
		return nil, err
	}
	maxBytes := cfg.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultWebhookMaxBytes
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultWebhookTimeout
	}
	return &webhookCallTool{
		allow:    allow,
		maxBytes: maxBytes,
		timeout:  timeout,
		// A zero hop bound: len(via) > 0 on the first redirect → cage.
		client: newCagedClient(allow, 0, cfg.PrivateOnly),
	}, nil
}

func (*webhookCallTool) Name() string { return "webhook_call" }
func (w *webhookCallTool) Description() string {
	return "POSTs a JSON payload to an operator allow-listed webhook. args = the URL, a space, then the JSON body, e.g. https://host/hook {\"event\":\"ping\"}."
}

// Execute parses "<url> <json-body>" from args and POSTs through the cage:
// scheme http/https only, host on the allow-list, no redirects, response
// capped, the hard timeout bounding the call, and — under the shield — every
// dial validated against private address space. Cage/shield breaches wrap
// their sentinels; a malformed payload, an HTTP error status, or a timeout
// is an ordinary tool error.
func (w *webhookCallTool) Execute(ctx context.Context, args string) (string, error) {
	parts := strings.SplitN(strings.TrimSpace(args), " ", 2)
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		return "", fmt.Errorf("webhook_call: args must be a URL, a space, then the JSON body")
	}
	rawURL, body := parts[0], strings.TrimSpace(parts[1])
	if !json.Valid([]byte(body)) {
		return "", fmt.Errorf("webhook_call: the body is not valid JSON")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("webhook_call: invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("webhook_call: scheme %q is not permitted: %w", u.Scheme, ErrCageViolation)
	}
	if !w.allow.permits(u) {
		return "", fmt.Errorf("webhook_call: host %q is not in the allow-list: %w", u.Host, ErrCageViolation)
	}

	callCtx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, u.String(), strings.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("webhook_call: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.client.Do(req)
	if err != nil {
		if errors.Is(err, ErrShieldViolation) || errors.Is(err, ErrCageViolation) {
			return "", fmt.Errorf("webhook_call: %w", err)
		}
		return "", fmt.Errorf("webhook_call: request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("webhook_call: HTTP %d from %s", resp.StatusCode, u.Host)
	}
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, w.maxBytes+1))
	if err != nil {
		return "", fmt.Errorf("webhook_call: read response: %w", err)
	}
	if int64(len(respBody)) > w.maxBytes {
		return "", fmt.Errorf("webhook_call: response exceeds the %d-byte cap: %w", w.maxBytes, ErrCageViolation)
	}
	if len(respBody) == 0 {
		return fmt.Sprintf("HTTP %d", resp.StatusCode), nil
	}
	return string(respBody), nil
}
