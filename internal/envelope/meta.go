// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package envelope

// MetaProviderEventID is the Meta key carrying the provider-native delivery
// identifier of an inbound event: Telegram's update ID, Discord's message ID,
// a webhook caller's X-Idempotency-Key. It is the key the router's inbound
// deduplication window matches on (audit finding R-1): the at-least-once
// channels (Telegram re-delivery after a crash, Discord resume replay, a
// webhook sender's retry) may hand the same event twice, and this ID is what
// makes "the same event" recognizable.
//
// The key lives in envelope — the vocabulary both channels and the router
// already share — so no adapter needs to import the router to speak it
// (the layering lesson of audit finding A-2). An adapter that has no natural
// event ID simply does not set it; an absent or empty value means the event
// is NOT deduplicated (fail-open: missing metadata must never discard a
// message).
const MetaProviderEventID = "provider.event_id"
