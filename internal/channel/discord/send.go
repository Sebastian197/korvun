// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package discord

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

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/model"
)

// ProviderName labels this channel in the shared error grammar (e.g. the Provider of
// a RateLimitError).
const ProviderName = ChannelName

// contentLimit is Discord's per-message content cap: 2000 characters (code points).
// Longer replies are split into ordered chunks (ADR-0033 §5).
const contentLimit = 2000

// breakWindow is how far back from the limit Send looks for a newline or space to cut
// a chunk on, so a split lands at a natural boundary rather than mid-word.
const breakWindow = 256

// maxErrorBodyBytes bounds how much of an error response body is read into an error
// message, defusing a misbehaving server (parity with the model adapters).
const maxErrorBodyBytes = 2 << 10 // 2 KiB

// createMessageBody is the JSON body of POST /channels/{id}/messages. allowedMentions
// is always present with an empty parse array so model output can never ping anyone
// (ADR-0033 §6 security default).
type createMessageBody struct {
	Content         string          `json:"content"`
	AllowedMentions allowedMentions `json:"allowed_mentions"`
}

// allowedMentions with an empty, non-nil Parse marshals to {"parse":[]} — the "mention
// nobody" contract. Parse has no omitempty so an empty slice serialises as [] (not
// null or absent).
type allowedMentions struct {
	Parse []string `json:"parse"`
}

// Send delivers an outbound Envelope to Discord via REST createMessage. The
// destination channel id is the conversation.id Meta key (set by the inbound mapper,
// echoed onto the reply by the Brain — parity with how Telegram resolves its chat id).
// A reply longer than 2000 characters is split into ordered, rune-safe chunks and
// posted sequentially so the user receives the whole answer, not an error; if a chunk
// fails, Send stops and the error names which part did not go out. The bot token is
// read from the env var at send time ("Bot <token>" header), never stored, never
// logged, never in an error (ADR-0010).
func (a *Adapter) Send(ctx context.Context, env *envelope.Envelope) error {
	if env == nil {
		return ErrEmptyMessage
	}
	channelID := env.Meta[conversation.MetaConversationID]
	if channelID == "" {
		return ErrMissingChannelID
	}
	// Channel-edge input validation (CLAUDE.md): a Discord channel id is a numeric
	// snowflake. Reject anything else so it can never inject path segments / a query
	// into the request URL.
	if !isSnowflake(channelID) {
		return fmt.Errorf("%w: %q", ErrInvalidChannelID, channelID)
	}
	content := outboundText(env)
	if strings.TrimSpace(content) == "" {
		return ErrEmptyMessage
	}

	chunks := splitContent(content, contentLimit)
	for i, chunk := range chunks {
		if err := a.postMessage(ctx, channelID, chunk); err != nil {
			return fmt.Errorf("discord: send part %d/%d: %w", i+1, len(chunks), err)
		}
	}
	return nil
}

// outboundText concatenates the Text parts of an outbound Envelope (v1 replies carry a
// single Text part; joining is defensive).
func outboundText(env *envelope.Envelope) string {
	var b strings.Builder
	for _, p := range env.Parts {
		if p.Type == envelope.Text {
			b.WriteString(p.Content)
		}
	}
	return b.String()
}

// splitContent breaks s into chunks of at most limit runes, cutting at a newline or
// space within breakWindow of the limit when one exists (else a hard rune-boundary
// cut). It never splits a multibyte character because it indexes runes, not bytes.
func splitContent(s string, limit int) []string {
	runes := []rune(s)
	if len(runes) <= limit {
		return []string{s}
	}
	var chunks []string
	for len(runes) > limit {
		cut := limit
		lo := limit - breakWindow
		if lo < 1 {
			lo = 1
		}
		if br := lastBreak(runes, lo, limit); br > 0 {
			cut = br
		}
		chunks = append(chunks, string(runes[:cut]))
		runes = runes[cut:]
	}
	if len(runes) > 0 {
		chunks = append(chunks, string(runes))
	}
	return chunks
}

// lastBreak returns the index just AFTER the last newline (preferred) or space in
// runes[lo:hi], or -1 if neither is present — so the break character stays with the
// preceding chunk.
func lastBreak(runes []rune, lo, hi int) int {
	for i := hi - 1; i >= lo; i-- {
		if runes[i] == '\n' {
			return i + 1
		}
	}
	for i := hi - 1; i >= lo; i-- {
		if runes[i] == ' ' {
			return i + 1
		}
	}
	return -1
}

// postMessage posts a single ≤2000-char chunk. It reads the bot token from the env var
// AT THIS MOMENT (never stored, never logged — ADR-0010) and sets allowed_mentions to
// the mention-nobody default.
func (a *Adapter) postMessage(ctx context.Context, channelID, content string) error {
	token := os.Getenv(a.cfg.tokenEnv)
	if token == "" {
		return fmt.Errorf("%w: %q (discord bot token)", ErrMissingToken, a.cfg.tokenEnv)
	}

	raw, err := json.Marshal(createMessageBody{
		Content:         content,
		AllowedMentions: allowedMentions{Parse: []string{}},
	})
	if err != nil {
		return fmt.Errorf("discord: marshal message: %w", err)
	}

	url := a.cfg.restBaseURL + "/channels/" + channelID + "/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("discord: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bot "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.cfg.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("discord: send aborted: %w", ctx.Err())
		}
		return fmt.Errorf("%w: %v", model.ErrProviderUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_, _ = io.Copy(io.Discard, resp.Body) // drain fully so the connection can be reused
		return nil
	}
	return mapSendError(resp)
}

// isSnowflake reports whether s is a non-empty run of ASCII digits (a Discord id).
func isSnowflake(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// mapSendError translates a non-2xx createMessage response into the house error
// grammar: 429 -> *model.RateLimitError (retry_after parsed, NO internal retry — the
// caller decides, as with the model providers); 401/403/404 -> named channel errors;
// 5xx -> model.ErrProviderUnavailable; other 4xx -> model.ErrProviderResponse. Error
// bodies are truncated; the token never appears (it is not in the response).
func mapSendError(resp *http.Response) error {
	rawBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	snippet := strings.TrimSpace(string(rawBody))

	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		rle := &model.RateLimitError{
			Provider:   ProviderName,
			RetryAfter: parseDiscordRetryAfter(rawBody, resp.Header.Get("Retry-After")),
		}
		return fmt.Errorf("discord: status 429: %s: %w", snippet, rle)
	case resp.StatusCode == http.StatusUnauthorized:
		return fmt.Errorf("%w: %s", ErrSendUnauthorized, snippet)
	case resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("%w: %s", ErrSendForbidden, snippet)
	case resp.StatusCode == http.StatusNotFound:
		return fmt.Errorf("%w: %s", ErrChannelNotFound, snippet)
	case resp.StatusCode >= 500:
		return fmt.Errorf("%w: status %d: %s", model.ErrProviderUnavailable, resp.StatusCode, snippet)
	default:
		return fmt.Errorf("%w: status %d: %s", model.ErrProviderResponse, resp.StatusCode, snippet)
	}
}

// maxRetryAfterSeconds is a sane ceiling on a 429 wait; a value above it (or a
// non-positive one) is treated as bogus and the Retry-After header is used instead.
// This also guards the float→Duration conversion against overflow from a hostile body.
const maxRetryAfterSeconds = 3600

// parseDiscordRetryAfter reads the wait before a retry from a 429: the body's
// retry_after (float seconds, precise) is preferred; the Retry-After header (rounded
// integer seconds) is the fallback.
func parseDiscordRetryAfter(body []byte, header string) time.Duration {
	var rl struct {
		RetryAfter float64 `json:"retry_after"`
	}
	if err := json.Unmarshal(body, &rl); err == nil && rl.RetryAfter > 0 && rl.RetryAfter <= maxRetryAfterSeconds {
		return time.Duration(rl.RetryAfter * float64(time.Second))
	}
	return model.ParseRetryAfter(header)
}
