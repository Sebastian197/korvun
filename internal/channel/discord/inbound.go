// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package discord

import (
	"encoding/json"
	"strings"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
)

// messageCreate is the subset of a Discord MESSAGE_CREATE dispatch payload (the `d`
// field of an op-0 dispatch) that the v1 channel maps. Unknown fields are ignored by
// encoding/json, so Discord adding fields never breaks decoding. GuildID is absent
// on a DM. Author is a pointer so an absent author is distinguishable from an empty
// one. Attachments/embeds/components are out of v1 scope (ADR-0033 §8) and not
// modelled here.
type messageCreate struct {
	ID        string         `json:"id"`
	ChannelID string         `json:"channel_id"`
	GuildID   string         `json:"guild_id,omitempty"`   // absent => DM
	WebhookID string         `json:"webhook_id,omitempty"` // set => posted by a webhook
	Content   string         `json:"content"`
	Author    *messageAuthor `json:"author"`
}

// messageAuthor is the subset of a message author the v1 channel reads. GlobalName
// is Discord's display name (may be empty; fall back to Username). Bot marks a bot
// account.
type messageAuthor struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	GlobalName string `json:"global_name,omitempty"`
	Bot        bool   `json:"bot,omitempty"`
}

// dropReason is why an inbound MESSAGE_CREATE was NOT turned into an Envelope. It is
// a typed value (not a bare bool/error) so the Gateway loop (SP3) can count drops in
// DroppedCount AND distinguish the cause for logging without guessing. `keep` (the
// zero value) means "not dropped — the Envelope is valid".
type dropReason int

const (
	keep             dropReason = iota // mapped successfully; the Envelope is valid
	dropMalformed                      // the payload JSON could not be decoded
	dropNoChannelID                    // no channel_id — cannot key a conversation
	dropNoAuthor                       // no author — cannot attribute the message
	dropSelf                           // authored by this bot (self id) — loop prevention
	dropFromBot                        // authored by another bot — loop prevention
	dropWebhook                        // posted by a webhook (integration/bridge) — loop prevention
	dropEmptyContent                   // no text content (media-only, out of v1 scope)
)

// String names the reason for logs.
func (d dropReason) String() string {
	switch d {
	case keep:
		return "keep"
	case dropMalformed:
		return "malformed"
	case dropNoChannelID:
		return "no_channel_id"
	case dropNoAuthor:
		return "no_author"
	case dropSelf:
		return "self"
	case dropFromBot:
		return "bot"
	case dropWebhook:
		return "webhook"
	case dropEmptyContent:
		return "empty_content"
	default:
		return "unknown"
	}
}

// mapMessageCreate decodes a MESSAGE_CREATE dispatch payload and maps it to an
// inbound Envelope per ADR-0033 §4, or returns (nil, reason) explaining why it was
// dropped. It is a PURE function — no network, no state, no goroutines — so SP3 can
// call it per dispatch and count/distinguish drops via DroppedCount.
//
// Mapping: channel "discord"; conversation.id = channel_id (a DM's channel_id is
// already unique, so a DM and a guild message map to the same Envelope shape and the
// same conversation key — guild_id is not needed); sender = author.id + display name
// (global_name if set, else username); a single Text part = content.
//
// LOOP PREVENTION (copilot decision, a contract not an accident): the whole
// automaton family is dropped — this bot's own self id, every bot author
// (author.bot == true), AND every webhook-posted message (webhook_id set). Discord
// delivers other bots' messages too, and webhook messages carry author.bot == false
// yet are non-human echo sources (integrations/bridges), so two Korvun gateways —
// or a bridge repeating channel content — would otherwise answer each other forever.
// selfID is a parameter (SP3 reads it from the Ready event). Note: a HUMAN proxied
// through a webhook is dropped too; treating proxied humans as real users is
// explicitly OUT of v1 (a future opt-in if anyone asks for it).
//
// Edge validation (channel-edge security rule): a payload that is malformed, has no
// channel_id, has no author (or an author with no id), or has empty/whitespace-only
// content (a media-only or blank message, out of v1 scope) is dropped with its
// specific reason.
func mapMessageCreate(data []byte, selfID string) (*envelope.Envelope, dropReason) {
	var m messageCreate
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, dropMalformed
	}
	if m.ChannelID == "" {
		return nil, dropNoChannelID
	}
	// An absent author, or an author object with no id, is unattributable — drop it
	// rather than emit an Envelope with an empty Sender.ID (parity with telegram,
	// which refuses a message whose From has no id).
	if m.Author == nil || m.Author.ID == "" {
		return nil, dropNoAuthor
	}
	if m.Author.ID == selfID {
		return nil, dropSelf
	}
	if m.Author.Bot {
		return nil, dropFromBot
	}
	if m.WebhookID != "" {
		return nil, dropWebhook
	}
	// Empty OR whitespace-only content is a media-only / blank message (out of v1
	// scope): drop it rather than route blank text to the brain. The kept Envelope
	// carries the original content verbatim (real text is never trimmed).
	if strings.TrimSpace(m.Content) == "" {
		return nil, dropEmptyContent
	}

	name := m.Author.GlobalName
	if name == "" {
		name = m.Author.Username
	}
	env := envelope.New(ChannelName, envelope.Inbound, envelope.Participant{
		ID:   m.Author.ID,
		Name: name,
	})
	env.Meta[conversation.MetaConversationID] = m.ChannelID
	env.AddText(m.Content)
	return env, keep
}
