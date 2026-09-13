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
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync/atomic"
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

// Params declares the structured fields for the native lane (the demo
// lesson): a URL and a PLAIN-TEXT message — no raw JSON composition asked
// of the model. An optional json field lets a capable model take control.
func (w *webhookCallTool) Params() []ToolParam {
	return []ToolParam{
		{Name: "url", Description: "the webhook URL, e.g. http://127.0.0.1:8765/aviso", Required: true},
		{Name: "message", Description: "the plain-text notice to send (the tool wraps it as JSON)", Required: true},
		{Name: "json", Description: "OPTIONAL: a complete JSON body; overrides message when valid"},
	}
}

// ArgsFromCall reconstructs the seam args (URL space JSON) from the fields,
// tolerantly: an explicit valid json wins; an invalid one degrades to a
// message body rather than failing; a plain message is marshaled into
// {"message": ...} so quoting is never the model's problem. Errors name the
// missing field — they become model-facing observations.
func (w *webhookCallTool) ArgsFromCall(fields map[string]any) (string, error) {
	u, _ := fields["url"].(string)
	if strings.TrimSpace(u) == "" {
		return "", fmt.Errorf("webhook_call: the url field is required")
	}
	if j, ok := fields["json"].(string); ok && strings.TrimSpace(j) != "" {
		if json.Valid([]byte(j)) {
			return u + " " + j, nil
		}
		// Tolerant degrade: broken JSON rides as a message body instead.
		fields = map[string]any{"url": u, "message": j}
	}
	m, _ := fields["message"].(string)
	if strings.TrimSpace(m) == "" {
		return "", fmt.Errorf("webhook_call: the message field is required (the plain text to send)")
	}
	body, err := json.Marshal(map[string]string{"message": m})
	if err != nil {
		return "", fmt.Errorf("webhook_call: marshal message: %w", err)
	}
	return u + " " + string(body), nil
}

// Execute parses "<url> <json-body>" from args and POSTs through the cage:
// scheme http/https only, host on the allow-list, no redirects, response
// capped, the hard timeout bounding the call, and — under the shield — every
// dial validated against private address space. Cage and shield breaches wrap
// their sentinels; a malformed payload is an ordinary tool error.
//
// EVERY refusal raised once the request body has reached the wire also wraps
// ErrEffectDelivered — an HTTP error status included, which this godoc used to
// call "an ordinary tool error". Whether the body reached the wire is not
// guessed from the error's shape: it is OBSERVED, by httptrace, and the
// observation is what the ledger closes on.
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
	// The post-delivery frontier, OBSERVED rather than inferred.
	//
	// Two cures tried to locate it by reasoning about which errors mean what,
	// and both were wrong in the same direction: the first said the frontier
	// was the response, the second said it was the end of Do's error handling.
	// http.Client.Do returns an error for ANY failure after the body is on the
	// wire — the host reads the whole POST and closes without answering, a
	// reset mid-response, a plain EOF — and each of those closed the ledger
	// FAILED over an effect that had already left.
	//
	// WroteRequest fires when a request body has been written in full, and
	// info.Err says whether that write itself failed. It cannot be reasoned
	// wrong: it is the wire reporting on itself. Redirects and retries fire it
	// once per attempt, so "fired cleanly at least once" is the question.
	var delivered atomic.Bool
	callCtx = httptrace.WithClientTrace(callCtx, &httptrace.ClientTrace{
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err == nil {
				delivered.Store(true)
			}
		},
	})
	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, u.String(), strings.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("webhook_call: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.client.Do(req)
	if err != nil {
		// A dial that never connects, the shield refusing the address, a DNS
		// failure: nothing was written, and the trace says so. A body that
		// reached the host and then lost its answer: the trace says that too,
		// and it is the SAME fact whether the failure wore a cage sentinel or
		// a bare EOF.
		if delivered.Load() {
			err = fmt.Errorf("%w: %w", ErrEffectDelivered, err)
		}
		if errors.Is(err, ErrShieldViolation) || errors.Is(err, ErrCageViolation) {
			return "", fmt.Errorf("webhook_call: %w", err)
		}
		return "", fmt.Errorf("webhook_call: request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body
	// Past here a response exists, so the body reached the host. The trace
	// agrees; this assert is here because a disagreement would mean the
	// frontier moved under us again, and that is worth a loud failure rather
	// than a quiet FAILED in somebody's ledger.
	if !delivered.Load() {
		return "", fmt.Errorf("webhook_call: a response arrived for a request the wire never reported writing: %w", ErrEffectDelivered)
	}
	if resp.StatusCode >= 400 {
		// The receiver read the body and answered. Whatever it did with the
		// POST before answering 500 is not ours to claim either way, so this
		// is a delivered request with an unusable answer — never a refusal
		// that stopped the effect.
		return "", fmt.Errorf("webhook_call: HTTP %d from %s: %w", resp.StatusCode, u.Host, ErrEffectDelivered)
	}
	// Past this line the POST has been ACCEPTED. Every error from here wraps
	// ErrEffectDelivered, because closing it as a plain failure tells the
	// operator the call did not happen. TestWebhookCall_everyBranchAfterDo
	// reads this function's AST and holds that rule by SITE.
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, w.maxBytes+1))
	if err != nil {
		return "", fmt.Errorf("webhook_call: read response: %w: %w", ErrEffectDelivered, err)
	}
	if int64(len(respBody)) > w.maxBytes {
		return "", fmt.Errorf("webhook_call: response exceeds the %d-byte cap: %w: %w", w.maxBytes, ErrEffectDelivered, ErrCageViolation)
	}
	if len(respBody) == 0 {
		return fmt.Sprintf("HTTP %d", resp.StatusCode), nil
	}
	return string(respBody), nil
}
