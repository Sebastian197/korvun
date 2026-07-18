// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package discord tests — Piece 4, sub-phase 1 (dependency + config surface +
// skeleton). These pin the constructor's env-only token resolution (ADR-0010), the
// channel.Channel surface (Name/Manifest/Mode/DroppedCount), and that the
// not-yet-built Gateway (Receive) and REST (Send) paths return explicit, honest
// errors — never silent no-ops. The Gateway/REST logic lands in SP2..SP5.
package discord

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Sebastian197/korvun/internal/channel"
)

const testTokenEnv = "KORVUN_DISCORD_TEST_TOKEN" // #nosec G101 -- env-var NAME, not a credential

// TestNew_resolvesTokenFromEnv pins the happy path: with the named env var set, New
// succeeds and the adapter exposes the channel.Channel surface. The token is
// resolved SOLELY from the environment (ADR-0010).
func TestNew_resolvesTokenFromEnv(t *testing.T) {
	t.Setenv(testTokenEnv, "a-secret-bot-token")

	a, err := New(WithTokenEnv(testTokenEnv))
	if err != nil {
		t.Fatalf("New with the env var set = %v, want nil", err)
	}
	if a.Name() != "discord" {
		t.Errorf("Name() = %q, want %q", a.Name(), "discord")
	}
	if a.Mode() != ModeGateway {
		t.Errorf("Mode() = %q, want %q", a.Mode(), ModeGateway)
	}
	if got := a.Manifest(); !got.Text || got.Image || got.Audio || got.Video || got.Buttons {
		t.Errorf("Manifest() = %+v, want text-only (v1 scope)", got)
	}
	if a.DroppedCount() != 0 {
		t.Errorf("DroppedCount() = %d, want 0 on a fresh adapter", a.DroppedCount())
	}
	var _ channel.Channel = a // compile-time: Adapter implements channel.Channel
}

// TestNew_tokenEnvUnset pins the ADR-0010 loud-and-named contract: when the named
// env var is unset, New fails with ErrMissingToken, the error NAMES the env var,
// and the (absent) token value never appears.
func TestNew_tokenEnvUnset(t *testing.T) {
	t.Setenv(testTokenEnv, "") // empty reads as unset

	_, err := New(WithTokenEnv(testTokenEnv))
	if !errors.Is(err, ErrMissingToken) {
		t.Fatalf("New with the env var unset = %v, want ErrMissingToken", err)
	}
	if !strings.Contains(err.Error(), testTokenEnv) {
		t.Errorf("error must name the env var %q; got %q", testTokenEnv, err.Error())
	}
}

// TestNew_missingTokenEnvName pins that a config without a token_env name is a loud
// error (before any env lookup).
func TestNew_missingTokenEnvName(t *testing.T) {
	if _, err := New(); !errors.Is(err, ErrMissingTokenEnv) {
		t.Fatalf("New without WithTokenEnv = %v, want ErrMissingTokenEnv", err)
	}
}

// TestNew_invalidMode pins that a non-gateway mode is rejected (the config layer
// already enforces this, but the adapter guards its own contract too).
func TestNew_invalidMode(t *testing.T) {
	t.Setenv(testTokenEnv, "tok")
	if _, err := New(WithTokenEnv(testTokenEnv), WithMode("polling")); !errors.Is(err, ErrInvalidMode) {
		t.Fatalf("New with mode=polling = %v, want ErrInvalidMode", err)
	}
}

// TestStubs_returnExplicitErrors pins that the not-yet-built paths fail honestly:
// Send (REST, SP5) and Receive (Gateway, SP3/SP4) return their explicit errors, not
// silent no-ops or a dead channel.
func TestStubs_returnExplicitErrors(t *testing.T) {
	t.Setenv(testTokenEnv, "tok")
	a, err := New(WithTokenEnv(testTokenEnv))
	if err != nil {
		t.Fatalf("New = %v, want nil", err)
	}

	if err := a.Send(context.Background(), nil); !errors.Is(err, ErrSendNotImplemented) {
		t.Errorf("Send stub = %v, want ErrSendNotImplemented", err)
	}
	ch, err := a.Receive(context.Background())
	if !errors.Is(err, ErrReceiveNotImplemented) {
		t.Errorf("Receive stub error = %v, want ErrReceiveNotImplemented", err)
	}
	if ch != nil {
		t.Errorf("Receive stub must return a nil channel alongside its error, got %v", ch)
	}
}
