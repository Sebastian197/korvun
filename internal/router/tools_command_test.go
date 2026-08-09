// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package router_test

import (
	"context"
	"testing"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/router"
)

// The /tools command (ADR-0041, spec FR-CHAT-1, mandate SP5.2): a first-token
// command on the CONFIGURED channel (the /new pattern), answered by the
// SYSTEM through the normal outbound funnel with the gatekeeper report —
// zero model/brain involvement. Marked as an ack so the console persists it
// as a SYSTEM turn.

func toolsRouter(t *testing.T, report router.ToolsReporter) (*router.Router, *fakeChannel, *fakeBrain) {
	t.Helper()
	store := conversation.NewMemStore()
	return sessionRouter(t, store, router.WithToolsCommand("tg", report))
}

func TestToolsCommand_answersWithTheGatekeeperReport(t *testing.T) {
	var askedBrain string
	r, ch, br := toolsRouter(t, func(brainName string) string {
		askedBrain = brainName
		return "Gatekeeper — brain \"b\"\n- calc: allow"
	})

	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "/tools")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	waitUntil(t, "the report to leave", func() bool { return len(ch.Sent()) == 1 })

	if askedBrain != "b" {
		t.Fatalf("reporter asked for brain %q, want the routed brain \"b\"", askedBrain)
	}
	out := ch.Sent()[0]
	if out.Channel != "tg" || out.Direction != envelope.Outbound {
		t.Fatalf("report misaddressed: %+v", out)
	}
	if len(out.Parts) != 1 || out.Parts[0].Content != "Gatekeeper — brain \"b\"\n- calc: allow" {
		t.Fatalf("report copy = %+v, want the reporter's text", out.Parts)
	}
	if out.Meta[router.MetaConversationID] != "c" {
		t.Fatalf("report lost the conversation addressing meta: %+v", out.Meta)
	}
	if out.Meta[envelope.MetaAck] != envelope.AckToolsReport {
		t.Fatalf("report not marked as a tools ack: %+v", out.Meta)
	}
	if n := len(br.Handled()); n != 0 {
		t.Fatalf("brain invoked %d times for /tools, want 0 (system response)", n)
	}
}

// On any OTHER channel the token is an ordinary message for the brain.
func TestToolsCommand_otherChannelPassesThrough(t *testing.T) {
	store := conversation.NewMemStore()
	r, ch, br := sessionRouter(t, store, router.WithToolsCommand("console", func(string) string { return "x" }))

	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "/tools")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	waitUntil(t, "the brain to handle it", func() bool { return len(br.Handled()) == 1 })
	if len(ch.Sent()) != 0 {
		t.Fatalf("system replied on a non-tools channel: %+v", ch.Sent())
	}
}

// A different first token never triggers the report.
func TestToolsCommand_otherTokenPassesThrough(t *testing.T) {
	r, _, br := toolsRouter(t, func(string) string { return "x" })

	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "tools please")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	waitUntil(t, "the brain to handle it", func() bool { return len(br.Handled()) == 1 })
}

// Without the option the token stays an ordinary message (nil reporter =
// feature off, the router's optional-seam convention).
func TestToolsCommand_absentOptionPassesThrough(t *testing.T) {
	store := conversation.NewMemStore()
	r, _, br := sessionRouter(t, store)

	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "/tools")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	waitUntil(t, "the brain to handle it", func() bool { return len(br.Handled()) == 1 })
}
