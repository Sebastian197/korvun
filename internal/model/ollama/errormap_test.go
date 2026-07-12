// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package ollama

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/model"
)

// mapResponse builds a minimal *http.Response for the isolated
// mapHTTPError unit tests (FR-5: the mapping is exercised directly,
// not only through Generate). retryAfter is set as the Retry-After
// header when non-empty; body is the (bounded) error snippet source.
func mapResponse(status int, retryAfter, body string) *http.Response {
	h := make(http.Header)
	if retryAfter != "" {
		h.Set("Retry-After", retryAfter)
	}
	return &http.Response{
		StatusCode: status,
		Header:     h,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// TestMapHTTPError is the table-driven heart of sub-phase 3: one case
// per FR of the spec. Assertions use errors.Is / errors.As, never
// string comparison. mapHTTPError is called directly (FR-5 isolation
// guard) — it does not exist yet, so this file is a compile-time RED.
func TestMapHTTPError(t *testing.T) {
	cases := []struct {
		name           string
		status         int
		retryAfter     string // Retry-After header, "" = absent
		wantSentinel   error  // expected via errors.Is
		wantRateLimit  bool   // expect *model.RateLimitError via errors.As
		wantRetryAfter time.Duration
		fr             string
	}{
		{"500 -> unavailable", 500, "", model.ErrProviderUnavailable, false, 0, "FR-1"},
		{"502 -> unavailable", 502, "", model.ErrProviderUnavailable, false, 0, "FR-1"},
		{"503 -> unavailable", 503, "", model.ErrProviderUnavailable, false, 0, "FR-1"},
		{"429 with header -> rate limit 7s", 429, "7", model.ErrRateLimited, true, 7 * time.Second, "FR-2"},
		{"429 without header -> rate limit 0", 429, "", model.ErrRateLimited, true, 0, "FR-2"},
		{"400 -> bad response", 400, "", model.ErrProviderResponse, false, 0, "FR-3"},
		{"404 -> bad response", 404, "", model.ErrProviderResponse, false, 0, "FR-3"},
		{"422 -> bad response", 422, "", model.ErrProviderResponse, false, 0, "FR-3"},
		{"401 -> auth invalid", 401, "", model.ErrAuthInvalid, false, 0, "FR-4"},
		{"403 -> auth invalid", 403, "", model.ErrAuthInvalid, false, 0, "FR-4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := mapHTTPError(mapResponse(tc.status, tc.retryAfter, "boom"))
			if err == nil {
				t.Fatalf("[%s] mapHTTPError(%d) = nil, want error", tc.fr, tc.status)
			}
			if !errors.Is(err, tc.wantSentinel) {
				t.Errorf("[%s] mapHTTPError(%d) = %v, want errors.Is %v",
					tc.fr, tc.status, err, tc.wantSentinel)
			}
			if tc.wantRateLimit {
				var rle *model.RateLimitError
				if !errors.As(err, &rle) {
					t.Fatalf("[%s] mapHTTPError(%d): errors.As(*RateLimitError) failed: %v",
						tc.fr, tc.status, err)
				}
				if rle.Provider != ProviderName {
					t.Errorf("[%s] RateLimitError.Provider = %q, want %q",
						tc.fr, rle.Provider, ProviderName)
				}
				if rle.RetryAfter != tc.wantRetryAfter {
					t.Errorf("[%s] RateLimitError.RetryAfter = %v, want %v",
						tc.fr, rle.RetryAfter, tc.wantRetryAfter)
				}
			}
			// The status code must appear in the diagnostic string.
			if !strings.Contains(err.Error(), http.StatusText(tc.status)) &&
				!strings.Contains(err.Error(), itoa(tc.status)) {
				t.Errorf("[%s] err string should mention status %d: %v",
					tc.fr, tc.status, err)
			}
		})
	}
}

// TestMapHTTPError_snippetBoundedNoLeak is the no-leak / bounded guard:
// even an oversized error body is capped at maxErrorBodyBytes (1 KiB)
// when reflected into the error, and nothing beyond the response-side
// diagnostic is exposed (Ollama carries no key, but the shape guard
// stays).
func TestMapHTTPError_snippetBoundedNoLeak(t *testing.T) {
	huge := strings.Repeat("A", 4096)
	err := mapHTTPError(mapResponse(http.StatusBadRequest, "", huge))
	if err == nil {
		t.Fatal("mapHTTPError(400) = nil, want error")
	}
	if got := strings.Count(err.Error(), "A"); got > maxErrorBodyBytes {
		t.Errorf("snippet not bounded: %d 'A' bytes in error, want <= %d",
			got, maxErrorBodyBytes)
	}
}

// TestGenerate_mapsStatusClasses is the httptest integration guard:
// a fake server returns each status and Generate must surface the
// right sentinel end-to-end (errors.Is / errors.As). No real network,
// no real provider.
func TestGenerate_mapsStatusClasses(t *testing.T) {
	cases := []struct {
		name           string
		status         int
		retryAfter     string
		wantSentinel   error
		wantRateLimit  bool
		wantRetryAfter time.Duration
		fr             string
	}{
		{"503 -> unavailable", 503, "", model.ErrProviderUnavailable, false, 0, "FR-1"},
		{"429 -> rate limit 5s", 429, "5", model.ErrRateLimited, true, 5 * time.Second, "FR-2"},
		{"404 -> bad response", 404, "", model.ErrProviderResponse, false, 0, "FR-3"},
		{"401 -> auth invalid", 401, "", model.ErrAuthInvalid, false, 0, "FR-4"},
		{"403 -> auth invalid", 403, "", model.ErrAuthInvalid, false, 0, "FR-4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.retryAfter != "" {
					w.Header().Set("Retry-After", tc.retryAfter)
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, "error body")
			}))
			t.Cleanup(srv.Close)

			a := New(WithBaseURL(srv.URL))
			req := &model.Request{
				Model:    "llama3.2",
				Messages: []model.Message{{Role: model.RoleUser, Content: "hola"}},
			}
			_, err := a.Generate(context.Background(), req)
			if !errors.Is(err, tc.wantSentinel) {
				t.Errorf("[%s] Generate(status %d) = %v, want errors.Is %v",
					tc.fr, tc.status, err, tc.wantSentinel)
			}
			if tc.wantRateLimit {
				var rle *model.RateLimitError
				if !errors.As(err, &rle) {
					t.Fatalf("[%s] Generate(status %d): errors.As(*RateLimitError) failed: %v",
						tc.fr, tc.status, err)
				}
				if rle.Provider != ProviderName {
					t.Errorf("[%s] RateLimitError.Provider = %q, want %q",
						tc.fr, rle.Provider, ProviderName)
				}
				if rle.RetryAfter != tc.wantRetryAfter {
					t.Errorf("[%s] RateLimitError.RetryAfter = %v, want %v",
						tc.fr, rle.RetryAfter, tc.wantRetryAfter)
				}
			}
		})
	}
}

// itoa is a tiny dependency-free int->string for the status-in-string
// assertion above (avoids pulling strconv just for a test helper).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
