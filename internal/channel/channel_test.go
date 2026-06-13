// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package channel

import (
	"context"
	"errors"
	"testing"

	"github.com/Sebastian197/korvun/internal/envelope"
)

// --- mock channel for testing ---

type mockChannel struct {
	name     string
	manifest Manifest
	received []*envelope.Envelope
	sendErr  error
}

func (m *mockChannel) Name() string       { return m.name }
func (m *mockChannel) Manifest() Manifest { return m.manifest }

func (m *mockChannel) Send(_ context.Context, env *envelope.Envelope) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.received = append(m.received, env)
	return nil
}

func (m *mockChannel) Receive(_ context.Context) (<-chan *envelope.Envelope, error) {
	ch := make(chan *envelope.Envelope, 1)
	env := envelope.New(m.name, envelope.Inbound, envelope.Participant{ID: "user-1"})
	env.AddText("hello from " + m.name)
	ch <- env
	close(ch)
	return ch, nil
}

// --- Manifest tests ---

func TestManifest_capabilities(t *testing.T) {
	m := Manifest{
		Text:    true,
		Image:   true,
		Audio:   false,
		Video:   false,
		Buttons: true,
	}

	if !m.Text {
		t.Error("Manifest.Text should be true")
	}
	if !m.Image {
		t.Error("Manifest.Image should be true")
	}
	if m.Audio {
		t.Error("Manifest.Audio should be false")
	}
	if m.Video {
		t.Error("Manifest.Video should be false")
	}
	if !m.Buttons {
		t.Error("Manifest.Buttons should be true")
	}
}

// --- Channel interface tests via mock ---

func TestChannel_Send(t *testing.T) {
	ch := &mockChannel{name: "test-channel", manifest: Manifest{Text: true}}
	env := envelope.New("test-channel", envelope.Outbound, envelope.Participant{ID: "bot"})
	env.AddText("hello")

	err := ch.Send(context.Background(), env)
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	if len(ch.received) != 1 {
		t.Fatalf("received %d envelopes, want 1", len(ch.received))
	}
	if ch.received[0].Parts[0].Content != "hello" {
		t.Errorf("received content = %q, want %q", ch.received[0].Parts[0].Content, "hello")
	}
}

func TestChannel_Send_error(t *testing.T) {
	ch := &mockChannel{
		name:    "fail-channel",
		sendErr: errors.New("send failed"),
	}
	env := envelope.New("fail-channel", envelope.Outbound, envelope.Participant{ID: "bot"})
	env.AddText("hello")

	err := ch.Send(context.Background(), env)
	if err == nil {
		t.Fatal("Send() should return error")
	}
	if err.Error() != "send failed" {
		t.Errorf("error = %q, want %q", err.Error(), "send failed")
	}
}

func TestChannel_Receive(t *testing.T) {
	ch := &mockChannel{name: "test-channel", manifest: Manifest{Text: true}}

	envCh, err := ch.Receive(context.Background())
	if err != nil {
		t.Fatalf("Receive() error: %v", err)
	}

	env, ok := <-envCh
	if !ok {
		t.Fatal("Receive channel closed without sending")
	}
	if env.Channel != "test-channel" {
		t.Errorf("Channel = %q, want %q", env.Channel, "test-channel")
	}
	if len(env.Parts) == 0 || env.Parts[0].Content != "hello from test-channel" {
		t.Errorf("unexpected content: %v", env.Parts)
	}
}

func TestChannel_Name(t *testing.T) {
	ch := &mockChannel{name: "telegram"}
	if ch.Name() != "telegram" {
		t.Errorf("Name() = %q, want %q", ch.Name(), "telegram")
	}
}

func TestChannel_Manifest(t *testing.T) {
	m := Manifest{Text: true, Image: true, Audio: false, Video: false, Buttons: true}
	ch := &mockChannel{name: "telegram", manifest: m}

	got := ch.Manifest()
	if got != m {
		t.Errorf("Manifest() = %+v, want %+v", got, m)
	}
}

// --- Registry tests ---

func TestRegistry_register_and_get(t *testing.T) {
	r := NewRegistry()
	ch := &mockChannel{name: "telegram"}

	r.Register(ch)

	got, ok := r.Get("telegram")
	if !ok {
		t.Fatal("Get() returned false for registered channel")
	}
	if got.Name() != "telegram" {
		t.Errorf("Name() = %q, want %q", got.Name(), "telegram")
	}
}

func TestRegistry_get_unknown(t *testing.T) {
	r := NewRegistry()

	_, ok := r.Get("unknown")
	if ok {
		t.Error("Get() should return false for unknown channel")
	}
}

func TestRegistry_unregister(t *testing.T) {
	r := NewRegistry()
	ch := &mockChannel{name: "telegram"}

	r.Register(ch)
	r.Unregister("telegram")

	_, ok := r.Get("telegram")
	if ok {
		t.Error("Get() should return false after Unregister")
	}
}

func TestRegistry_list(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockChannel{name: "telegram"})
	r.Register(&mockChannel{name: "webhook"})

	names := r.List()
	if len(names) != 2 {
		t.Fatalf("List() len = %d, want 2", len(names))
	}

	has := make(map[string]bool)
	for _, n := range names {
		has[n] = true
	}
	if !has["telegram"] || !has["webhook"] {
		t.Errorf("List() = %v, want telegram and webhook", names)
	}
}

func TestRegistry_register_overwrites(t *testing.T) {
	r := NewRegistry()
	ch1 := &mockChannel{name: "telegram", manifest: Manifest{Text: true}}
	ch2 := &mockChannel{name: "telegram", manifest: Manifest{Text: true, Image: true}}

	r.Register(ch1)
	r.Register(ch2)

	got, ok := r.Get("telegram")
	if !ok {
		t.Fatal("Get() returned false")
	}
	if !got.Manifest().Image {
		t.Error("Register should overwrite existing channel with same name")
	}
}
