// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package controlapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeReader is a static Reader for handler tests.
type fakeReader struct {
	brains   []BrainSummary
	channels []ChannelSummary
}

func (f fakeReader) BrainSummaries() []BrainSummary     { return f.brains }
func (f fakeReader) ChannelSummaries() []ChannelSummary { return f.channels }

// recordingMounter records the patterns Register mounts, so a test can assert
// the exact route set (and that every route is a read-only GET).
type recordingMounter struct{ patterns []string }

func (m *recordingMounter) Handle(pattern string, _ http.Handler) {
	m.patterns = append(m.patterns, pattern)
}

// newServer mounts the API on a real ServeMux (which satisfies Mounter) and
// serves it, so handler tests exercise the actual routing.
func newServer(r Reader) *httptest.Server {
	mux := http.NewServeMux()
	Register(mux, r)
	return httptest.NewServer(mux)
}

func u64(v uint64) *uint64 { return &v }

// TestRegister_mountsExactlyTwoReadOnlyRoutes pins the minimal cut: two GETs,
// nothing else, no mutating verb (ADR-0022 §1, §2).
func TestRegister_mountsExactlyTwoReadOnlyRoutes(t *testing.T) {
	m := &recordingMounter{}
	Register(m, fakeReader{})

	want := map[string]bool{"GET /api/brains": true, "GET /api/channels": true}
	if len(m.patterns) != len(want) {
		t.Fatalf("registered %v, want exactly %v", m.patterns, want)
	}
	for _, p := range m.patterns {
		if !want[p] {
			t.Errorf("unexpected route %q registered (cut is two read-only GETs)", p)
		}
		if !strings.HasPrefix(p, "GET ") {
			t.Errorf("route %q is not a GET — read-only invariant broken", p)
		}
	}
}

// TestBrainsHandler_returnsResolvedJSON asserts the brains endpoint serves the
// resolved summary as JSON with the right content type and status.
func TestBrainsHandler_returnsResolvedJSON(t *testing.T) {
	want := []BrainSummary{{
		Name:        "default",
		Sensitivity: "private",
		Policy:      "priority",
		Dispatch:    "fanout",
		Models:      []ModelSummary{{Provider: "ollama", ModelID: "llama3.2"}},
	}}
	srv := newServer(fakeReader{brains: want})
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/api/brains")
	if err != nil {
		t.Fatalf("GET /api/brains: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got []BrainSummary
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Name != "default" || len(got[0].Models) != 1 ||
		got[0].Models[0].Provider != "ollama" || got[0].Models[0].ModelID != "llama3.2" {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// TestChannelsHandler_droppedOmitemptyVsPresent asserts the dropped count is
// present when the channel has a counter and OMITTED otherwise.
func TestChannelsHandler_droppedOmitemptyVsPresent(t *testing.T) {
	srv := newServer(fakeReader{channels: []ChannelSummary{
		{Type: "telegram", Mode: "polling", Name: "telegram", Dropped: u64(7)},
		{Type: "webhook", Mode: "push", Name: "hook"}, // no counter
	}})
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/api/channels")
	if err != nil {
		t.Fatalf("GET /api/channels: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var raw []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(raw) != 2 {
		t.Fatalf("got %d channels, want 2", len(raw))
	}
	if _, ok := raw[0]["dropped"]; !ok {
		t.Error("channel with a counter is missing the dropped field")
	}
	if _, ok := raw[1]["dropped"]; ok {
		t.Error("channel without a counter must OMIT the dropped field (omitempty)")
	}
}

// TestWriteJSON_marshalError_returns500 covers the defensive error branch: an
// unmarshalable value yields a 500 BEFORE any 200 header is written. The summary
// types always marshal, so this is the only way to exercise the path.
func TestWriteJSON_marshalError_returns500(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, make(chan int)) // channels are not JSON-marshalable
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

// TestSummaries_carryNoSecretBearingFields is the leaf half of the secret-free
// invariant (ADR-0022 §4): the JSON shape itself names nothing secret or
// secret-referencing. The app-level test asserts no real secret leaks end to end.
func TestSummaries_carryNoSecretBearingFields(t *testing.T) {
	bs, _ := json.Marshal(BrainSummary{
		Name: "b", Sensitivity: "public", Policy: "priority", Dispatch: "fanout",
		Models: []ModelSummary{{Provider: "groq", ModelID: "llama-3.3-70b"}},
	})
	cs, _ := json.Marshal(ChannelSummary{Type: "telegram", Mode: "polling", Name: "telegram", Dropped: u64(0)})

	forbidden := []string{"token", "api_key", "apikey", "secret", "_env", "key_env"}
	for _, body := range []string{string(bs), string(cs)} {
		low := strings.ToLower(body)
		for _, f := range forbidden {
			if strings.Contains(low, f) {
				t.Errorf("summary JSON %q contains forbidden marker %q", body, f)
			}
		}
	}
}
