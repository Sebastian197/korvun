// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package webhook implements a generic webhook channel adapter that converts
// arbitrary JSON payloads into Envelopes using a configurable field mapping.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Sebastian197/korvun/internal/channel"
	"github.com/Sebastian197/korvun/internal/envelope"
)

// FieldMapping configures which JSON fields in the incoming payload map to
// Envelope fields.
type FieldMapping struct {
	SenderID   string `json:"sender_id"`
	SenderName string `json:"sender_name"`
	Text       string `json:"text"`
	MediaURL   string `json:"media_url"`
	MediaType  string `json:"media_type"`
}

// Adapter is a generic webhook channel that converts JSON payloads to/from
// Envelopes. It implements the channel.Channel interface.
type Adapter struct {
	name        string
	mapping     FieldMapping
	outboundURL string
	inbound     chan *envelope.Envelope
	client      *http.Client
}

// New creates a webhook adapter with the given name and field mapping.
func New(name string, mapping FieldMapping) *Adapter {
	return &Adapter{
		name:    name,
		mapping: mapping,
		inbound: make(chan *envelope.Envelope, 64),
		client:  &http.Client{},
	}
}

// Name returns the adapter's channel name.
func (a *Adapter) Name() string { return a.name }

// Manifest returns the capabilities of this webhook channel.
func (a *Adapter) Manifest() channel.Manifest {
	return channel.Manifest{
		Text:  true,
		Image: true,
		Audio: true,
		Video: true,
	}
}

// Inbound returns the read-only channel for received envelopes.
func (a *Adapter) Inbound() <-chan *envelope.Envelope {
	return a.inbound
}

// Receive returns the inbound envelope channel, satisfying the Channel
// interface.
func (a *Adapter) Receive(_ context.Context) (<-chan *envelope.Envelope, error) {
	return a.inbound, nil
}

// SetOutboundURL configures the URL for outgoing webhook POST requests.
func (a *Adapter) SetOutboundURL(url string) {
	a.outboundURL = url
}

// Send delivers an outbound Envelope as a JSON POST to the configured URL.
func (a *Adapter) Send(ctx context.Context, env *envelope.Envelope) error {
	if a.outboundURL == "" {
		return errors.New("outbound URL not configured")
	}

	payload := a.envelopeToPayload(env)
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal outbound payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.outboundURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create outbound request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("outbound request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("outbound request failed with status %d", resp.StatusCode)
	}
	return nil
}

// InboundHandler returns an http.Handler that accepts incoming webhook POST
// requests, parses the JSON payload, and converts it to an Envelope.
func (a *Adapter) InboundHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}

		var fields map[string]string
		if err := json.Unmarshal(body, &fields); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		env, err := a.payloadToEnvelope(fields)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		a.inbound <- env
		w.WriteHeader(http.StatusOK)
	})
}

func (a *Adapter) payloadToEnvelope(fields map[string]string) (*envelope.Envelope, error) {
	senderID := fields[a.mapping.SenderID]
	if senderID == "" {
		return nil, errors.New("missing sender ID")
	}

	sender := envelope.Participant{
		ID:   senderID,
		Name: fields[a.mapping.SenderName],
	}

	env := envelope.New(a.name, envelope.Inbound, sender)

	text := fields[a.mapping.Text]
	mediaURL := fields[a.mapping.MediaURL]

	if text == "" && mediaURL == "" {
		return nil, errors.New("payload has no text or media")
	}

	if text != "" {
		env.AddText(text)
	}

	if mediaURL != "" {
		mimeType := fields[a.mapping.MediaType]
		pt := mimeToPartType(mimeType)
		env.AddMedia(pt, mediaURL, mimeType)
	}

	return env, nil
}

func (a *Adapter) envelopeToPayload(env *envelope.Envelope) map[string]string {
	payload := map[string]string{
		a.mapping.SenderID:   env.Sender.ID,
		a.mapping.SenderName: env.Sender.Name,
	}

	for _, p := range env.Parts {
		switch p.Type {
		case envelope.Text:
			payload[a.mapping.Text] = p.Content
		case envelope.Image, envelope.Audio, envelope.Video, envelope.File:
			payload[a.mapping.MediaURL] = p.Source
			payload[a.mapping.MediaType] = p.MIMEType
		}
	}

	return payload
}

func mimeToPartType(mime string) envelope.PartType {
	switch {
	case strings.HasPrefix(mime, "image/"):
		return envelope.Image
	case strings.HasPrefix(mime, "audio/"):
		return envelope.Audio
	case strings.HasPrefix(mime, "video/"):
		return envelope.Video
	default:
		return envelope.File
	}
}
