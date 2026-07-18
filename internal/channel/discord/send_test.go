// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package discord tests — Piece 4, sub-phase 5 (outbound REST send). These exercise
// Send against a FAKE Discord REST endpoint (httptest), deterministic and network-
// free. They pin: the "Bot <token>" auth header (asserted, never logged), the exact
// body with allowed_mentions {"parse": []} (the security default: model output can
// never ping anyone), content preserved verbatim, the 2000-char rune-safe split of
// long replies (ordered, multibyte-safe, a failing part named), the house error
// grammar (429 -> RateLimitError, 401/403/404 -> named, 5xx -> Unavailable), the
// env-only token (ADR-0010), and a clean ctx-cancel mid-send.
package discord

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/model"
)

func startFakeREST(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv.URL
}

func newSendAdapter(t *testing.T, restURL string, extra ...Option) *Adapter {
	t.Helper()
	t.Setenv(gwTestTokenEnv, gwTestTokenValue)
	opts := []Option{WithTokenEnv(gwTestTokenEnv), withRESTBaseURLForTests(restURL)}
	opts = append(opts, extra...)
	a, err := New(opts...)
	if err != nil {
		t.Fatalf("New = %v", err)
	}
	return a
}

func outboundEnv(channelID, content string) *envelope.Envelope {
	e := envelope.New(ChannelName, envelope.Outbound, envelope.Participant{ID: "bot", Name: "korvun"})
	e.Meta[conversation.MetaConversationID] = channelID
	e.AddText(content)
	return e
}

func TestSend_HappyPath(t *testing.T) {
	var (
		gotPath string
		gotAuth string
		gotBody createMessageBody
		gotRaw  []byte
	)
	done := make(chan struct{})
	url := startFakeREST(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotRaw, _ = io.ReadAll(r.Body)
		_ = json.Unmarshal(gotRaw, &gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"999"}`))
		close(done)
	})

	a := newSendAdapter(t, url)
	if err := a.Send(context.Background(), outboundEnv("555", "hola 🐦 mundo")); err != nil {
		t.Fatalf("Send = %v", err)
	}
	<-done

	if gotPath != "/channels/555/messages" {
		t.Errorf("path = %q, want /channels/555/messages", gotPath)
	}
	if gotAuth != "Bot "+gwTestTokenValue { // assert WITHOUT logging the value
		t.Error("Authorization header is not \"Bot <token>\"")
	}
	if gotBody.Content != "hola 🐦 mundo" {
		t.Errorf("content = %q, want verbatim", gotBody.Content)
	}
	if !strings.Contains(string(gotRaw), `"allowed_mentions":{"parse":[]}`) {
		t.Errorf("body must carry allowed_mentions {\"parse\":[]}; got %s", gotRaw)
	}
}

func TestSend_RateLimited(t *testing.T) {
	url := startFakeREST(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "2") // header is the rounded-up integer
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"You are being rate limited.","retry_after":1.5,"global":false}`))
	})
	a := newSendAdapter(t, url)
	err := a.Send(context.Background(), outboundEnv("555", "hi"))

	var rle *model.RateLimitError
	if !errors.As(err, &rle) {
		t.Fatalf("err = %v, want *model.RateLimitError", err)
	}
	if !errors.Is(err, model.ErrRateLimited) {
		t.Error("429 error must wrap model.ErrRateLimited")
	}
	if rle.Provider != ProviderName {
		t.Errorf("Provider = %q, want %q", rle.Provider, ProviderName)
	}
	if rle.RetryAfter != 1500*time.Millisecond {
		t.Errorf("RetryAfter = %v, want 1.5s (the precise body float, not the rounded header)", rle.RetryAfter)
	}
}

func TestSend_RateLimitedHeaderFallback(t *testing.T) {
	// No retry_after in the body -> fall back to the Retry-After header (integer secs).
	url := startFakeREST(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "3")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"rate limited"}`))
	})
	a := newSendAdapter(t, url)
	err := a.Send(context.Background(), outboundEnv("555", "hi"))

	var rle *model.RateLimitError
	if !errors.As(err, &rle) {
		t.Fatalf("err = %v, want *model.RateLimitError", err)
	}
	if rle.RetryAfter != 3*time.Second {
		t.Errorf("RetryAfter = %v, want 3s (fallback to the header)", rle.RetryAfter)
	}
}

func TestSend_ErrorCodes(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   error
	}{
		{http.StatusUnauthorized, ErrSendUnauthorized},
		{http.StatusForbidden, ErrSendForbidden},
		{http.StatusNotFound, ErrChannelNotFound},
		{http.StatusInternalServerError, model.ErrProviderUnavailable},
		{http.StatusBadGateway, model.ErrProviderUnavailable},
		{http.StatusBadRequest, model.ErrProviderResponse}, // other 4xx
	} {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			url := startFakeREST(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"message":"nope"}`))
			})
			a := newSendAdapter(t, url)
			err := a.Send(context.Background(), outboundEnv("555", "hi"))
			if !errors.Is(err, tc.want) {
				t.Errorf("status %d: err = %v, want %v", tc.status, err, tc.want)
			}
		})
	}
}

func TestSend_SplitsLongContent(t *testing.T) {
	var (
		mu    sync.Mutex
		parts []string
	)
	url := startFakeREST(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var b createMessageBody
		_ = json.Unmarshal(raw, &b)
		mu.Lock()
		parts = append(parts, b.Content)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"1"}`))
	})
	a := newSendAdapter(t, url)

	// 1990 ASCII + 30 four-byte emoji = 2020 runes -> 2 chunks. A byte-based cut at
	// 2000 would land mid-emoji; a rune-based cut must not.
	content := strings.Repeat("a", 1990) + strings.Repeat("🐦", 30)
	if err := a.Send(context.Background(), outboundEnv("555", content)); err != nil {
		t.Fatalf("Send = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(parts) != 2 {
		t.Fatalf("got %d parts, want 2", len(parts))
	}
	for i, p := range parts {
		if n := utf8.RuneCountInString(p); n > 2000 {
			t.Errorf("part %d has %d runes, > 2000", i+1, n)
		}
		if !utf8.ValidString(p) {
			t.Errorf("part %d is not valid UTF-8 (a multibyte char was split)", i+1)
		}
	}
	if parts[0]+parts[1] != content {
		t.Error("parts do not reassemble to the original content (order/loss)")
	}
	if utf8.RuneCountInString(parts[0]) != 2000 {
		t.Errorf("first part = %d runes, want exactly 2000", utf8.RuneCountInString(parts[0]))
	}
}

func TestSend_PrefersBreakAtNewline(t *testing.T) {
	var (
		mu    sync.Mutex
		parts []string
	)
	url := startFakeREST(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var b createMessageBody
		_ = json.Unmarshal(raw, &b)
		mu.Lock()
		parts = append(parts, b.Content)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"1"}`))
	})
	a := newSendAdapter(t, url)

	// A newline near the 2000 boundary should be the cut point.
	head := strings.Repeat("a", 1990) + "\n" + strings.Repeat("b", 200)
	if err := a.Send(context.Background(), outboundEnv("555", head)); err != nil {
		t.Fatalf("Send = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(parts) != 2 {
		t.Fatalf("got %d parts, want 2", len(parts))
	}
	if !strings.HasSuffix(parts[0], "\n") {
		t.Errorf("first part should end at the newline; got tail %q", tail(parts[0], 5))
	}
	if strings.HasPrefix(parts[1], "\n") {
		t.Errorf("newline should stay with the first part, not lead the second")
	}
}

func TestSend_SplitFailureNamesPart(t *testing.T) {
	var n atomic.Int32
	url := startFakeREST(t, func(w http.ResponseWriter, _ *http.Request) {
		if n.Add(1) == 2 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"boom"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"1"}`))
	})
	a := newSendAdapter(t, url)

	content := strings.Repeat("a", 5000) // 3 chunks: 2000 + 2000 + 1000
	err := a.Send(context.Background(), outboundEnv("555", content))
	if err == nil {
		t.Fatal("want an error when part 2 fails")
	}
	if !strings.Contains(err.Error(), "part 2/3") {
		t.Errorf("error must name the failed part as 2/3; got %q", err.Error())
	}
	if !errors.Is(err, model.ErrProviderUnavailable) {
		t.Errorf("underlying 500 must map to ErrProviderUnavailable; got %v", err)
	}
	if got := n.Load(); got != 2 {
		t.Errorf("made %d requests; want exactly 2 (part 3 must NOT be sent after part 2 failed)", got)
	}
}

func TestSend_InvalidChannelID(t *testing.T) {
	url := startFakeREST(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	a := newSendAdapter(t, url)
	if err := a.Send(context.Background(), outboundEnv("555/../@me", "hi")); !errors.Is(err, ErrInvalidChannelID) {
		t.Fatalf("Send = %v, want ErrInvalidChannelID", err)
	}
}

func TestSplitContent(t *testing.T) {
	const limit = 2000
	for _, tc := range []struct {
		name       string
		in         string
		wantChunks int
	}{
		{"under limit", strings.Repeat("a", 1000), 1},
		{"exactly limit", strings.Repeat("a", 2000), 1},
		{"limit plus one", strings.Repeat("a", 2001), 2},
		{"no break, 5000", strings.Repeat("a", 5000), 3},
		{"all spaces", strings.Repeat(" ", 2500), 2},
		{"newline near boundary", strings.Repeat("a", 1990) + "\n" + strings.Repeat("b", 200), 2},
		{"four-byte emoji at boundary", strings.Repeat("a", 1990) + strings.Repeat("🐦", 30), 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chunks := splitContent(tc.in, limit)
			if len(chunks) != tc.wantChunks {
				t.Errorf("got %d chunks, want %d", len(chunks), tc.wantChunks)
			}
			var reassembled strings.Builder
			for i, c := range chunks {
				if n := utf8.RuneCountInString(c); n > limit {
					t.Errorf("chunk %d has %d runes, > %d", i, n, limit)
				}
				if !utf8.ValidString(c) {
					t.Errorf("chunk %d is not valid UTF-8", i)
				}
				reassembled.WriteString(c)
			}
			if reassembled.String() != tc.in {
				t.Error("chunks do not reassemble to the original (loss/dup/reorder)")
			}
		})
	}
}

func TestSend_MissingChannelID(t *testing.T) {
	url := startFakeREST(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	a := newSendAdapter(t, url)
	e := envelope.New(ChannelName, envelope.Outbound, envelope.Participant{ID: "bot"})
	e.AddText("hi") // no conversation id in Meta
	if err := a.Send(context.Background(), e); !errors.Is(err, ErrMissingChannelID) {
		t.Fatalf("Send = %v, want ErrMissingChannelID", err)
	}
}

func TestSend_EmptyContent(t *testing.T) {
	url := startFakeREST(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	a := newSendAdapter(t, url)
	if err := a.Send(context.Background(), outboundEnv("555", "   ")); !errors.Is(err, ErrEmptyMessage) {
		t.Fatalf("Send = %v, want ErrEmptyMessage", err)
	}
}

func TestSend_TokenMissingAtSend(t *testing.T) {
	url := startFakeREST(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	t.Setenv(gwTestTokenEnv, gwTestTokenValue)
	a, err := New(WithTokenEnv(gwTestTokenEnv), withRESTBaseURLForTests(url))
	if err != nil {
		t.Fatalf("New = %v", err)
	}
	if err := os.Unsetenv(gwTestTokenEnv); err != nil {
		t.Fatalf("Unsetenv = %v", err)
	}
	err = a.Send(context.Background(), outboundEnv("555", "hi"))
	if !errors.Is(err, ErrMissingToken) {
		t.Fatalf("Send = %v, want ErrMissingToken", err)
	}
	if !strings.Contains(err.Error(), gwTestTokenEnv) {
		t.Errorf("error must name the env var %q; got %q", gwTestTokenEnv, err.Error())
	}
}

func TestSend_CtxCancelAborts(t *testing.T) {
	release := make(chan struct{})
	url := startFakeREST(t, func(w http.ResponseWriter, _ *http.Request) {
		<-release // block so the request is in flight when we cancel
		w.WriteHeader(http.StatusOK)
	})
	t.Cleanup(func() { close(release) })
	a := newSendAdapter(t, url)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- a.Send(ctx, outboundEnv("555", "hi")) }()
	time.Sleep(40 * time.Millisecond) // let the request reach the server
	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Send did not return after ctx cancel")
	}
}

// tail returns the last n bytes of s for error messages.
func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
