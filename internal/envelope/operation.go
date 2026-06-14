// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package envelope

// OperationKind classifies the side-effect intent of an outbound
// Envelope.Operation. See ADR-0006 for the taxonomy (notifications,
// mutations, reads) and the per-kind contract.
type OperationKind int

const (
	// OpEditText edits the text body of a previously sent message.
	// Parts: exactly one Text Part with non-empty Content (the new body).
	// Keyboard: optional. Target via channel-specific Meta entries
	// (telegram.chat_id + telegram.message_id).
	OpEditText OperationKind = iota
	// OpEditCaption edits the caption of a previously sent media
	// message. Parts: exactly one Text Part (Content may be empty to
	// clear the caption). Keyboard: optional.
	OpEditCaption
	// OpDelete deletes a previously sent message. Parts: empty.
	// Keyboard: forbidden. Target via channel-specific Meta entries.
	OpDelete
	// OpCallbackAck acknowledges an inbound Callback so the channel
	// stops retrying. Parts: empty (silent ack) OR one Text Part with
	// non-empty Content (the toast text). Keyboard: forbidden. Target
	// via channel-specific Meta entry (e.g. telegram.callback_query_id).
	OpCallbackAck
	// OpSetReaction sets (or clears) the bot's emoji reactions on a
	// previously sent message. Parts: 0+ Text Parts, each Content one
	// emoji; empty Parts means "clear all the bot's reactions on the
	// target". Keyboard: forbidden. Target via channel-specific Meta
	// entries (e.g. telegram.chat_id + telegram.message_id).
	// See ADR-0007.
	OpSetReaction
)

// String returns the human-readable name of the operation kind.
func (k OperationKind) String() string {
	switch k {
	case OpEditText:
		return "edit_text"
	case OpEditCaption:
		return "edit_caption"
	case OpDelete:
		return "delete"
	case OpCallbackAck:
		return "callback_ack"
	case OpSetReaction:
		return "set_reaction"
	default:
		return "unknown"
	}
}

// Operation is the side-effect intent attached to an outbound
// Envelope. The Kind selects the verb; the actual data (new content,
// target identifier) rides in Envelope.Parts and Envelope.Meta per the
// contract documented on each OperationKind constant.
type Operation struct {
	Kind OperationKind `json:"kind"`
}
