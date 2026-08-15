// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package tool

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultFetchMaxBytes caps a fetched response when the operator does not set
// one. Prompt-sized, same reasoning as DefaultReadFileMaxBytes.
const DefaultFetchMaxBytes = 64 * 1024

// DefaultFetchMaxRedirects bounds the redirect chain when unset.
const DefaultFetchMaxRedirects = 3

// HTTPFetchConfig is the operator cage of the http_fetch tool (ADR-0041 §4).
type HTTPFetchConfig struct {
	// AllowHosts is the exact-host allow-list (case-insensitive; an entry
	// may pin a port — "host:port" — otherwise any port of that host
	// matches). Required non-empty; blank entries fail construction.
	AllowHosts []string
	// MaxBytes caps the response body (0 => DefaultFetchMaxBytes).
	MaxBytes int64
	// MaxRedirects bounds the redirect chain (0 => DefaultFetchMaxRedirects).
	// Every hop is re-checked against the allow-list, and under the shield
	// every hop dials through the private-address check.
	MaxRedirects int
	// PrivateOnly arms the network shield (ADR-0041 §3): the wiring sets it
	// from ToolDecision.Shield — a network tool on a Private brain. The
	// dialer then validates the RESOLVED IP of every connection.
	PrivateOnly bool
	// Timeout is the hard per-call bound (0 => DefaultFetchTimeout). Parity
	// with webhook_call's own bound (estreno E-7): without it, a slow
	// allow-listed host pinned the agent loop for the whole brain handler
	// ceiling.
	Timeout time.Duration
}

// DefaultFetchTimeout is the hard per-call bound when unset. Wider than
// webhook_call's 10s: that tool POSTs to the operator's own endpoint, while
// a page fetch over the public internet needs headroom.
const DefaultFetchTimeout = 30 * time.Second

// httpFetchTool is the caged GET-only fetcher. The http.Client is built once
// at construction (it is safe for concurrent use, honoring the Tool
// concurrency contract) with an explicit Transport: NO proxy — an
// environment proxy would carry the request around both the allow-list and
// the shield.
type httpFetchTool struct {
	timeout  time.Duration
	allow    allowList
	maxBytes int64
	client   *http.Client
}

// HTTPFetch constructs the caged http_fetch tool, failing loud at wiring on
// an empty or blank allow-list (a fetcher without a list is an open proxy).
func HTTPFetch(cfg HTTPFetchConfig) (Tool, error) {
	allow, err := newAllowList("http_fetch", cfg.AllowHosts)
	if err != nil {
		return nil, err
	}
	maxBytes := cfg.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultFetchMaxBytes
	}
	maxRedirects := cfg.MaxRedirects
	if maxRedirects <= 0 {
		maxRedirects = DefaultFetchMaxRedirects
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultFetchTimeout
	}
	f := &httpFetchTool{allow: allow, maxBytes: maxBytes, timeout: timeout}
	f.client = newCagedClient(allow, maxRedirects, cfg.PrivateOnly)
	return f, nil
}

func (*httpFetchTool) Name() string { return "http_fetch" }
func (f *httpFetchTool) Description() string {
	return "fetches a URL with HTTP GET from the operator's allow-listed hosts. args = the URL, e.g. https://host/path."
}

// Execute GETs the URL in args through the cage: scheme http/https only, host
// on the allow-list, redirects re-checked per hop, response capped, and —
// when the shield is armed — every dial validated against private address
// space. Cage/shield breaches wrap their sentinels (audited as denials); an
// HTTP error status or a network failure is an ordinary tool error.
func (f *httpFetchTool) Execute(ctx context.Context, args string) (string, error) {
	// The tool owns its per-call bound (estreno E-7): the brain's per-tool
	// timeout is optional wiring, and the handler ceiling is a last resort,
	// not a fetch budget.
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()
	raw := strings.TrimSpace(args)
	if raw == "" {
		return "", fmt.Errorf("http_fetch: a URL is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("http_fetch: invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("http_fetch: scheme %q is not permitted: %w", u.Scheme, ErrCageViolation)
	}
	if !f.allow.permits(u) {
		return "", fmt.Errorf("http_fetch: host %q is not in the allow-list: %w", u.Host, ErrCageViolation)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("http_fetch: build request: %w", err)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		// Unwrap so a shield/cage stop inside the client (dial guard,
		// redirect check) keeps its sentinel for the audit classification.
		if errors.Is(err, ErrShieldViolation) || errors.Is(err, ErrCageViolation) {
			return "", fmt.Errorf("http_fetch: %w", err)
		}
		return "", fmt.Errorf("http_fetch: request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("http_fetch: HTTP %d from %s", resp.StatusCode, u.Host)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, f.maxBytes+1))
	if err != nil {
		return "", fmt.Errorf("http_fetch: read body: %w", err)
	}
	if int64(len(body)) > f.maxBytes {
		return "", fmt.Errorf("http_fetch: response exceeds the %d-byte cap: %w", f.maxBytes, ErrCageViolation)
	}
	return string(body), nil
}

// newCagedClient builds the shared http.Client of the network tools: explicit
// no-proxy Transport, the shield's dial Control when armed, and a redirect
// policy that re-checks the allow-list and the hop cap on every hop.
func newCagedClient(allow allowList, maxRedirects int, privateOnly bool) *http.Client {
	dialer := &net.Dialer{}
	if privateOnly {
		dialer.Control = shieldControl
	}
	transport := &http.Transport{
		// Proxy deliberately nil: an environment proxy would tunnel the
		// request around the allow-list AND the shield's dial check.
		DialContext: dialer.DialContext,
	}
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return fmt.Errorf("more than %d redirects: %w", maxRedirects, ErrCageViolation)
			}
			if !allow.permits(req.URL) {
				return fmt.Errorf("redirect to %q is not in the allow-list: %w", req.URL.Host, ErrCageViolation)
			}
			return nil
		},
	}
}

// allowList is the normalized exact-host allow-list shared by the network
// tools: entries lowercase, optionally port-pinned.
type allowList []allowEntry

type allowEntry struct {
	host string
	port string // "" = any port
}

// newAllowList validates and normalizes the operator's entries.
func newAllowList(toolName string, hosts []string) (allowList, error) {
	if len(hosts) == 0 {
		return nil, fmt.Errorf("%s: a non-empty host allow-list is required", toolName)
	}
	out := make(allowList, 0, len(hosts))
	for i, raw := range hosts {
		e := strings.ToLower(strings.TrimSpace(raw))
		if e == "" {
			return nil, fmt.Errorf("%s: allow-list entry %d is blank", toolName, i)
		}
		if host, port, err := net.SplitHostPort(e); err == nil {
			out = append(out, allowEntry{host: host, port: port})
			continue
		}
		out = append(out, allowEntry{host: e})
	}
	return out, nil
}

// permits reports whether the URL's host matches an entry: hostname equal
// (case-insensitive), and the port equal when the entry pins one (an
// unpinned entry matches any port; a URL without an explicit port carries
// its scheme default).
func (l allowList) permits(u *url.URL) bool {
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	for _, e := range l {
		if e.host != host {
			continue
		}
		if e.port == "" || e.port == port {
			return true
		}
	}
	return false
}
