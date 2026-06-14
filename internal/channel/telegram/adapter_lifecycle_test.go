// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package telegram

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/channel"
)

// --- New() validation ---

func TestNew_rejectsMissingToken(t *testing.T) {
	_, err := New(WithMode(ModePolling))
	if !errors.Is(err, ErrMissingToken) {
		t.Fatalf("err = %v, want ErrMissingToken", err)
	}
}

func TestNew_rejectsMissingMode(t *testing.T) {
	_, err := New(WithToken("test-token"))
	if !errors.Is(err, ErrInvalidMode) {
		t.Fatalf("err = %v, want ErrInvalidMode", err)
	}
}

func TestNew_rejectsInvalidModeValue(t *testing.T) {
	_, err := New(WithToken("test-token"), WithMode(Mode(99)))
	if !errors.Is(err, ErrInvalidMode) {
		t.Fatalf("err = %v, want ErrInvalidMode", err)
	}
}

func TestNew_pollingRejectsZeroCapacity(t *testing.T) {
	_, err := New(
		WithToken("test-token"),
		WithMode(ModePolling),
		WithInboundCapacity(0),
	)
	if !errors.Is(err, ErrInvalidInboundCapacity) {
		t.Fatalf("err = %v, want ErrInvalidInboundCapacity", err)
	}
}

func TestNew_pollingRejectsNegativeEnqueueTimeout(t *testing.T) {
	_, err := New(
		WithToken("test-token"),
		WithMode(ModePolling),
		WithEnqueueTimeout(-1*time.Millisecond),
	)
	if !errors.Is(err, ErrInvalidEnqueueTimeout) {
		t.Fatalf("err = %v, want ErrInvalidEnqueueTimeout", err)
	}
}

func TestNew_webhookRequiresURL(t *testing.T) {
	_, err := New(
		WithToken("test-token"),
		WithMode(ModeWebhook),
		WithListenAddr(":8443"),
		WithSecretToken("s"),
		WithTLS("c.pem", "k.pem"),
	)
	if !errors.Is(err, ErrMissingWebhookURL) {
		t.Fatalf("err = %v, want ErrMissingWebhookURL", err)
	}
}

func TestNew_webhookRequiresListenAddr(t *testing.T) {
	_, err := New(
		WithToken("test-token"),
		WithMode(ModeWebhook),
		WithWebhookURL("https://example.com/wh"),
		WithSecretToken("s"),
		WithTLS("c.pem", "k.pem"),
	)
	if !errors.Is(err, ErrMissingListenAddr) {
		t.Fatalf("err = %v, want ErrMissingListenAddr", err)
	}
}

func TestNew_webhookRequiresSecretToken(t *testing.T) {
	_, err := New(
		WithToken("test-token"),
		WithMode(ModeWebhook),
		WithWebhookURL("https://example.com/wh"),
		WithListenAddr(":8443"),
		WithTLS("c.pem", "k.pem"),
	)
	if !errors.Is(err, ErrMissingSecretToken) {
		t.Fatalf("err = %v, want ErrMissingSecretToken", err)
	}
}

func TestNew_webhookRequiresTLSConfig(t *testing.T) {
	_, err := New(
		WithToken("test-token"),
		WithMode(ModeWebhook),
		WithWebhookURL("https://example.com/wh"),
		WithListenAddr(":8443"),
		WithSecretToken("s"),
	)
	if !errors.Is(err, ErrMissingTLSConfig) {
		t.Fatalf("err = %v, want ErrMissingTLSConfig", err)
	}
}

func TestNew_webhookReverseProxyAcceptsNoTLS(t *testing.T) {
	a, err := New(
		WithToken("test-token"),
		WithMode(ModeWebhook),
		WithWebhookURL("https://example.com/wh"),
		WithListenAddr(":8443"),
		WithSecretToken("s"),
		WithReverseProxyTermination(),
		withInjectedBotForTests(stubBotClient{}),
	)
	if err != nil {
		t.Fatalf("New() err = %v, want nil", err)
	}
	if a == nil {
		t.Fatal("adapter is nil")
	}
}

func TestNew_pollingHappyPath(t *testing.T) {
	a, err := New(
		WithToken("test-token"),
		WithMode(ModePolling),
		withInjectedBotForTests(stubBotClient{}),
	)
	if err != nil {
		t.Fatalf("New() err = %v, want nil", err)
	}
	if a == nil {
		t.Fatal("adapter is nil")
	}
}

// --- Name / Manifest / Receive ---

func TestAdapter_Name(t *testing.T) {
	a := newTestAdapter(t)
	if got := a.Name(); got != ChannelName {
		t.Errorf("Name() = %q, want %q", got, ChannelName)
	}
}

func TestAdapter_Manifest_capabilities(t *testing.T) {
	a := newTestAdapter(t)
	want := channel.Manifest{Text: true, Image: true, Audio: true, Video: true, Buttons: true}
	if got := a.Manifest(); got != want {
		t.Errorf("Manifest() = %+v, want %+v", got, want)
	}
}

func TestAdapter_Receive_returnsSameChannel(t *testing.T) {
	a := newTestAdapter(t)
	c1, err := a.Receive(context.Background())
	if err != nil {
		t.Fatalf("Receive() err = %v", err)
	}
	c2, err := a.Receive(context.Background())
	if err != nil {
		t.Fatalf("Receive() err = %v", err)
	}
	if c1 != c2 {
		t.Errorf("Receive returned different channels on repeated calls")
	}
}

func TestAdapter_Mode_reflectsConfig(t *testing.T) {
	a := newTestAdapter(t)
	if a.Mode() != ModePolling {
		t.Errorf("Mode() = %v, want %v", a.Mode(), ModePolling)
	}
}

// --- handleLibraryUpdate / Options coverage ---

func TestAdapter_handleLibraryUpdate_routesToDispatch(t *testing.T) {
	a := newTestAdapter(t)
	u := newTextUpdate(7, 3, "lib-handled")
	a.handleLibraryUpdate(context.Background(), nil, u)
	select {
	case env := <-a.inbound:
		if env.Parts[0].Content != "lib-handled" {
			t.Errorf("Content = %q", env.Parts[0].Content)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("handleLibraryUpdate did not produce an Envelope")
	}
}

func TestOptions_overrideDefaults(t *testing.T) {
	logger := slog.Default()
	a, err := New(
		WithToken("test-token"),
		WithMode(ModePolling),
		WithWebhookPath("/custom/wh"),
		WithReadHeaderTimeout(15*time.Second),
		WithLogger(logger),
		WithLibraryOptions(),
		withInjectedBotForTests(stubBotClient{}),
	)
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	if a.cfg.webhookPath != "/custom/wh" {
		t.Errorf("webhookPath = %q", a.cfg.webhookPath)
	}
	if a.cfg.readHeaderTimeout != 15*time.Second {
		t.Errorf("readHeaderTimeout = %v", a.cfg.readHeaderTimeout)
	}
	if a.cfg.logger != logger {
		t.Errorf("logger override did not stick")
	}
}

// --- Mode.String ---

func TestMode_String(t *testing.T) {
	cases := []struct {
		m    Mode
		want string
	}{
		{ModePolling, "polling"},
		{ModeWebhook, "webhook"},
		{Mode(0), "unknown"},
		{Mode(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.m.String(); got != tc.want {
			t.Errorf("Mode(%d).String() = %q, want %q", tc.m, got, tc.want)
		}
	}
}

// newTestAdapter builds an Adapter wired to a stub botClient,
// suitable for tests that only exercise the lifecycle-light methods
// (Name, Manifest, Receive, Mode). Inbound traffic and Send
// dispatching get their own helpers in dispatch_test.go and
// send_test.go.
func newTestAdapter(t *testing.T) *Adapter {
	t.Helper()
	a, err := New(
		WithToken("test-token"),
		WithMode(ModePolling),
		withInjectedBotForTests(stubBotClient{}),
	)
	if err != nil {
		t.Fatalf("newTestAdapter: New() err = %v", err)
	}
	return a
}
