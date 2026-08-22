// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package openaicompat

// GREEN-phase supplementary coverage: option seams and error-path edges
// the RED contract exercises indirectly or not at all. ADDITIVE only —
// the approved RED suite is untouched.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/model"
)

// TestGenerate_nilRequestIsValidated pins the mold's upstream validation
// seam: a nil request maps to model.ErrNilRequest without any network.
func TestGenerate_nilRequestIsValidated(t *testing.T) {
	a := newAdapter(t, "http://127.0.0.1:1")
	if _, err := a.Generate(context.Background(), nil); !errors.Is(err, model.ErrNilRequest) {
		t.Errorf("err = %v, want ErrNilRequest", err)
	}
}

// TestWithRequestTimeout_boundsTheCall pins the zero-disables option: a
// wired timeout expires against a slow server and the error chain keeps
// context.DeadlineExceeded (the N3 obligation, at the adapter's own
// timeout too).
func TestWithRequestTimeout_boundsTheCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
			return
		}
		_, _ = w.Write([]byte(okBody("too late")))
	}))
	t.Cleanup(srv.Close)

	a := newAdapter(t, srv.URL, WithRequestTimeout(30*time.Millisecond))
	_, err := a.Generate(context.Background(), chatReq())
	if !errors.Is(err, model.ErrProviderUnavailable) {
		t.Errorf("err = %v, want ErrProviderUnavailable", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want the chain to keep context.DeadlineExceeded", err)
	}
}

// TestWithHTTPClient_injectedClientIsUsed pins the injection seam against
// a TLS server: the server's own client (which trusts its certificate)
// completes the round-trip the default pool refuses.
func TestWithHTTPClient_injectedClientIsUsed(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(okBody("trusted")))
	}))
	t.Cleanup(srv.Close)

	a := newAdapter(t, srv.URL, WithHTTPClient(srv.Client()))
	resp, err := a.Generate(context.Background(), chatReq())
	if err != nil {
		t.Fatalf("Generate with injected client: %v", err)
	}
	if resp.Message.Content != "trusted" {
		t.Errorf("content = %q, want trusted", resp.Message.Content)
	}
}

// TestGenerate_429WithoutEnvelope covers the no-envelope 429: a plain
// body cannot match any quota code, so the safe default (RateLimitError)
// applies.
func TestGenerate_429WithoutEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("too many requests"))
	}))
	t.Cleanup(srv.Close)

	_, err := newAdapter(t, srv.URL).Generate(context.Background(), chatReq())
	var rle *model.RateLimitError
	if !errors.As(err, &rle) {
		t.Errorf("err = %v, want *RateLimitError (no envelope => safe default)", err)
	}
}

// TestGenerate_quotaTypeMatchesWhenCodeDiffers covers the error.type
// branch of the quota partition: an unrecognized code with a listed type
// is still permanent quota exhaustion (the spec matches EITHER field).
func TestGenerate_quotaTypeMatchesWhenCodeDiffers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"no credit","type":"insufficient_quota","code":"billing_hard_limit_reached"}}`))
	}))
	t.Cleanup(srv.Close)

	_, err := newAdapter(t, srv.URL).Generate(context.Background(), chatReq())
	if !errors.Is(err, model.ErrProviderResponse) {
		t.Errorf("err = %v, want permanent ErrProviderResponse (type matched)", err)
	}
	var rle *model.RateLimitError
	if errors.As(err, &rle) {
		t.Errorf("quota exhaustion must not carry a RateLimitError: %v", err)
	}
}

// TestGenerate_errorWithEmptyBody covers the empty non-2xx body: the
// snippet degrades to "(empty body)" and the status still maps by the
// matrix's default bucket.
func TestGenerate_errorWithEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	t.Cleanup(srv.Close)

	_, err := newAdapter(t, srv.URL).Generate(context.Background(), chatReq())
	if !errors.Is(err, model.ErrProviderResponse) {
		t.Fatalf("err = %v, want ErrProviderResponse", err)
	}
	if !strings.Contains(err.Error(), "(empty body)") {
		t.Errorf("error %q does not carry the empty-body snippet", err.Error())
	}
}

// TestGenerate_unbuildableURL covers the build-request error path: a
// base URL with a control character cannot become an *http.Request.
func TestGenerate_unbuildableURL(t *testing.T) {
	a := newAdapter(t, "http://127.0.0.1:1/\x7f\n")
	if _, err := a.Generate(context.Background(), chatReq()); !errors.Is(err, model.ErrProviderResponse) {
		t.Errorf("err = %v, want ErrProviderResponse (build request)", err)
	}
}
