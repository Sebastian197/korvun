// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/controlapi"
)

// statusHTTPTimeout bounds every admin-API request end to end (dial + read), so no
// status path can hang: an unreachable or wedged admin server fails within it
// rather than blocking the operator's terminal.
const statusHTTPTimeout = 5 * time.Second

// statusCmd implements `korvun status [--addr host:port]` (FR-STA-1..3): a thin
// HTTP client of the already-serving admin API (ADR-0022). It gates on /healthz
// (liveness), then GETs /api/brains + /api/channels and renders the resolved wiring
// as an aligned table. It adds NO server code and sends NO token. --addr is
// injectable so tests point it at an httptest.Server on an ephemeral port, never
// the real admin bind. It decodes into internal/controlapi's own summary types, so
// the wire contract has a single source of truth (no drift-prone local mirror).
//
// NOTE (conscious scope, not an oversight): the two independent data GETs run
// sequentially. The admin API is loopback by default (config.DefaultObservabilityAddr),
// so the round trips are sub-millisecond and parallelizing them would add
// concurrency machinery for a remote-admin case this local-first tool does not
// target. Revisit if a high-latency remote admin becomes a real use case.
func (c *cli) statusCmd(args []string) int {
	fs := flag.NewFlagSet("korvun status", flag.ContinueOnError)
	addr := fs.String("addr", config.DefaultObservabilityAddr, "host:port of the korvun admin API")
	plain, noColor, code, done := c.parseStyled(fs, args)
	if done {
		return code // -h/--help (0) or a bad flag (2), already written to the right stream
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(c.stderr, "korvun status: unexpected argument %q\nRun 'korvun help' for usage.\n", fs.Arg(0))
		return 2
	}

	// Requests carry a context so a slow status is cancellable (Ctrl-C / SIGTERM)
	// rather than blocking for the full client timeout (CLAUDE.md: context on every
	// cancellable operation). The client Timeout still bounds each request end to end.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	client := &http.Client{Timeout: statusHTTPTimeout}

	// Accept both `host:port` and a scheme-qualified `http://host:port`: strip any
	// leading scheme before composing the base, so a URL-style --addr does not become
	// `http://http://…` and misreport a live server as unreachable.
	host := strings.TrimPrefix(strings.TrimPrefix(*addr, "http://"), "https://")
	base := "http://" + host

	// Reachability gate: a dial error or a non-200 /healthz means the admin API is
	// off/unreachable — an honest failure, exit 1, never a panic or a stack trace
	// (FR-STA-2). The message is error-colored on an interactive stderr (down = error,
	// FR-STY-6), with the text carrying the meaning (color is never the only channel).
	if !adminReachable(ctx, client, base) {
		return c.statusError(fmt.Sprintf("admin API not reachable at %s — is Korvun running with observability enabled?", *addr), plain, noColor)
	}

	var brains []controlapi.BrainSummary
	if err := getJSON(ctx, client, base+"/api/brains", &brains); err != nil {
		return c.statusError(fmt.Sprintf("admin API at %s returned an unexpected response: %v", *addr, err), plain, noColor)
	}
	var channels []controlapi.ChannelSummary
	if err := getJSON(ctx, client, base+"/api/channels", &channels); err != nil {
		return c.statusError(fmt.Sprintf("admin API at %s returned an unexpected response: %v", *addr, err), plain, noColor)
	}

	c.renderStatus(*addr, brains, channels, plain, noColor)
	return 0
}

// statusError writes an honest, error-role-colored failure line to stderr (gated by
// styleEnabled — color is additive, the text carries the meaning) and returns exit
// code 1. It is the single place status emits a failure, so the unreachable and
// bad-response paths stay in lockstep.
func (c *cli) statusError(msg string, plain, noColor bool) int {
	errStyled := c.styleEnabled(c.stderr, plain, noColor)
	_, _ = fmt.Fprintln(c.stderr, c.paint(errStyled, roleError, msg))
	return 1
}

// adminReachable reports whether the admin API answers a healthy /healthz. A dial
// error (server off) or any non-200 (server unhealthy) both count as unreachable.
func adminReachable(ctx context.Context, client *http.Client, base string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/healthz", nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body) // drain so the connection can be reused
	return resp.StatusCode == http.StatusOK
}

// getJSON GETs url and decodes a 200 JSON body into out. A transport error, a
// non-200 status, or a decode error is returned so the caller can fail honestly.
func getJSON(ctx context.Context, client *http.Client, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: unexpected status %d", url, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", url, err)
	}
	return nil
}

// renderStatus writes the resolved wiring to stdout: a colored health line, then
// aligned brains and channels tables (text/tabwriter), then a warn line if any
// channel is dropping. Color is confined to the standalone header/warn lines and
// kept OUT of the tables on purpose: ANSI escapes have zero display width but
// tabwriter counts bytes, so a colored cell would misalign the columns — keeping
// them plain makes the columns byte-identical on a TTY and off it (R3/FR-STY-3).
func (c *cli) renderStatus(addr string, brains []controlapi.BrainSummary, channels []controlapi.ChannelSummary, plain, noColor bool) {
	styled := c.styleEnabled(c.stdout, plain, noColor)

	_, _ = fmt.Fprintf(c.stdout, "korvun status @ %s\n\n", addr)
	_, _ = fmt.Fprintf(c.stdout, "HEALTH  %s\n\n", c.paint(styled, roleSuccess, "up"))

	_, _ = fmt.Fprintln(c.stdout, "BRAINS")
	tw := tabwriter.NewWriter(c.stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tSENSITIVITY\tPOLICY\tDISPATCH\tMODELS")
	for _, b := range brains {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", b.Name, b.Sensitivity, b.Policy, b.Dispatch, joinModels(b.Models))
	}
	_ = tw.Flush()

	_, _ = fmt.Fprintln(c.stdout, "\nCHANNELS")
	tw = tabwriter.NewWriter(c.stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TYPE\tMODE\tNAME\tDROPPED")
	var totalDropped uint64
	for _, ch := range channels {
		dropped := "-"
		if ch.Dropped != nil {
			dropped = strconv.FormatUint(*ch.Dropped, 10)
			totalDropped += *ch.Dropped
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", ch.Type, ch.Mode, ch.Name, dropped)
	}
	_ = tw.Flush()

	if totalDropped > 0 {
		_, _ = fmt.Fprintf(c.stdout, "\n%s  %d message(s) dropped across channels\n",
			c.paint(styled, roleWarn, "warning:"), totalDropped)
	}
}

// joinModels renders a brain's surviving models as "provider/model_id" joined by
// commas, or "-" when the privacy selector left none.
func joinModels(models []controlapi.ModelSummary) string {
	if len(models) == 0 {
		return "-"
	}
	parts := make([]string, len(models))
	for i, m := range models {
		parts[i] = m.Provider + "/" + m.ModelID
	}
	return strings.Join(parts, ", ")
}
