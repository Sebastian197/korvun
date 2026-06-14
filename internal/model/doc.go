// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package model defines the abstraction every reasoning provider in
// Korvun talks through. A Model speaks in role-tagged conversation
// messages (system / user / assistant), not in envelope.Envelope —
// the Brain layer (internal/brain, Stage 7) is responsible for
// translating between the two domains, the same way the channel
// adapters translate native transport formats to and from Envelope.
//
// See ADR-0009 for the design rationale.
//
// Phase 4.1 ships the synchronous Generate path only. Streaming
// arrives later as a sibling StreamingModel interface; provider
// support is opt-in by satisfying that interface in addition to
// Model.
package model
