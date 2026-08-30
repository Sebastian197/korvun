// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Identity boot wiring (Trust Layer Etapa 2, lote 4, spec FR-PRIN-3,
// FR-MIG-1/2, sealed decisions 1-2): the provenance registry is derived
// from config the app already owns (channel adapters change ZERO lines),
// the root intent auto-materializes at boot — deterministic, idempotent,
// boot-fatal on failure — and each brain's governance ALLOW rows derive
// the in-memory grant that EXPLAINS its allows. Config remains the
// single source of truth; SelectTools remains THE judge.
package app

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/config"
)

// channelCredentials maps a channel type to its transport credential
// KIND (spec FR-EVID-1): the finite enum, config-pinned.
var channelCredentials = map[string]action.CredentialType{
	"telegram": action.CredentialBotTokenSession,
	"webhook":  action.CredentialInboundBearer,
	"discord":  action.CredentialGatewaySession,
}

// provenanceRegistry wires the provenance registry from config: every
// configured channel (whose NAME is its type) plus the console — the
// operator's own hands are in-process provenance, present on every boot,
// config or no config. A channel absent from this registry fails closed
// at the resolver (never an invented principal).
func provenanceRegistry(cfg *config.Config) action.ProvenanceRegistry {
	reg := action.ProvenanceRegistry{
		"console": {Class: "console", Credential: action.CredentialLoopbackInProcess},
	}
	for _, ch := range cfg.Channels {
		if credential, known := channelCredentials[ch.Type]; known {
			reg[ch.Type] = action.Provenance{Class: ch.Type, Credential: credential}
		}
	}
	return reg
}

// derivedConfigGrant derives one brain's in-memory grant from its
// governance ALLOW rows (FR-MIG-1): operations = the allowed tools,
// resources = the carried channel restrictions as coarse "channel:<name>"
// entries (or "*" when unrestricted). Shadow and deny rows grant no
// authority, and a brain with no allows derives NOTHING — its ungoverned
// flows act directly under the root's standing authority.
func derivedConfigGrant(bc config.BrainConfig) (action.AuthorityGrant, bool) {
	if bc.Agent == nil {
		return action.AuthorityGrant{}, false
	}
	var operations []string
	channels := map[string]bool{}
	for _, g := range bc.Agent.Governance {
		if g.Mode != "allow" {
			continue
		}
		operations = append(operations, g.Tool)
		for _, ch := range g.Channels {
			channels["channel:"+ch] = true
		}
	}
	if len(operations) == 0 {
		return action.AuthorityGrant{}, false
	}
	resources := []string{"*"}
	if len(channels) > 0 {
		resources = make([]string, 0, len(channels))
		for ch := range channels {
			resources = append(resources, ch)
		}
		sort.Strings(resources)
	}
	return action.DeriveConfigGrant(bc.Name, operations, resources), true
}

// StoragePath exposes the shared storage-path resolution to the CLI
// (Etapa 2, lote 5): ONE resolution for the conversation store, the
// kernel store and the operator's CLI, so "the same file" stays true by
// construction everywhere.
func StoragePath(cfg *config.Config) string { return storagePath(cfg) }

// rootIntentStore is the slice of the kernel store the boot needs.
type rootIntentStore interface {
	GetIntent(ctx context.Context, intentID string) (action.IntentContract, error)
	CreateIntent(ctx context.Context, c action.IntentContract) error
}

// ensureRootIntent materializes the root intent (sealed decision 1):
// already stored → verified no-op; absent → created. Anything else is a
// boot-fatal error — a boot that cannot state the standing authority
// must not run.
func ensureRootIntent(ctx context.Context, store rootIntentStore) error {
	_, err := store.GetIntent(ctx, action.RootIntentID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, actionsqlite.ErrNotFound) {
		return fmt.Errorf("app: read root intent: %w", err)
	}
	if err := store.CreateIntent(ctx, action.RootIntent()); err != nil {
		return fmt.Errorf("app: materialize root intent: %w", err)
	}
	return nil
}

// RecordAttemptIdentified implements brain.IdentifiedRecorder: the
// attempt, its decision, its identity refs and its evidence commit in
// ONE store transaction (FR-EVID-2 live).
func (r actionRecorder) RecordAttemptIdentified(ctx context.Context, env action.Envelope, outcome, rule string, state action.State, evidence action.IdentityEvidence) error {
	return r.store.RecordAttemptIdentified(ctx, env,
		actionsqlite.Decision{
			Outcome: outcome, Rule: rule,
			PolicyVersion: r.pin.Version, PolicyDigest: r.pin.Digest,
		}, state,
		actionsqlite.AttemptIdentity{
			PrincipalID:   env.Principal.PrincipalID,
			IntentID:      env.IntentID,
			AuthorityRefs: env.AuthorityRefs,
			Evidence:      evidence,
		})
}
