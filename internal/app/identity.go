// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Identity boot wiring derives shared requester principals, brain workloads,
// and adapter issuers from configuration. The legacy provenance registry below
// remains a compatibility profile; production adapters use opaque ingress.
package app

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/identity"
)

const (
	consoleRequesterPrincipal   = "principal_console"
	consoleResponsiblePrincipal = "principal_console_admin"
)

// phase1IdentityRegistry derives shared channel principals and fixed brain
// workloads from configuration. Sender claims never create principals.
func phase1IdentityRegistry(cfg *config.Config) identity.Registry {
	consoleCredentialRef := "console_bearer"
	if cfg.Admin != nil && cfg.Admin.TokenEnv != "" {
		consoleCredentialRef = cfg.Admin.TokenEnv
	}
	principals := map[string]identity.Principal{
		consoleRequesterPrincipal: {
			ID: consoleRequesterPrincipal, Kind: identity.PrincipalExternalSystem,
			DisplayName: "Console credential",
		},
		consoleResponsiblePrincipal: {
			ID: consoleResponsiblePrincipal, Kind: identity.PrincipalExternalSystem,
			DisplayName: "Console administrator role",
		},
	}
	registry := identity.Registry{
		Bindings: []identity.Binding{{
			ID: "binding_console", Provider: "console", Channel: "console",
			CredentialRef: consoleCredentialRef, SubjectNamespace: "local_profile",
			VerifiedSubject: "shared_console_credential",
			PrincipalID:     consoleRequesterPrincipal, Generation: 1,
			Status: identity.BindingActive,
		}},
	}
	// A channel's TYPE is its identity everywhere else in the tree — the
	// provenance registry keys `reg[ch.Type]`, the router registers by type and
	// the observability collector registers one source per type. Two channels of
	// the same type therefore share ONE binding and ONE shared-credential
	// principal, exactly as they already share their provenance row. Deriving a
	// second binding under the same id made `identity.NewResolver` refuse a
	// configuration that `config.Validate` accepts and that the tree booted
	// before this phase; the phase promises no such change, so the second
	// channel of a type is folded into the first (the twentieth pass, P1-1).
	seenChannelType := map[string]bool{}
	for _, channel := range cfg.Channels {
		if channel.Type == "console" || seenChannelType[channel.Type] {
			continue
		}
		seenChannelType[channel.Type] = true
		principalID := "principal_channel_" + channel.Type
		principals[principalID] = identity.Principal{
			ID: principalID, Kind: identity.PrincipalExternalSystem,
			DisplayName: channel.Type + " shared credential",
		}
		registry.Bindings = append(registry.Bindings, identity.Binding{
			ID: "binding_" + channel.Type, Provider: channel.Type,
			Channel: channel.Type, CredentialRef: channel.TokenEnv,
			SubjectNamespace: channel.Type + "_subject",
			VerifiedSubject:  "shared_" + channel.Type + "_credential",
			PrincipalID:      principalID,
			Generation:       1, Status: identity.BindingActive,
		})
	}
	for _, configured := range cfg.Brains {
		principalID := "principal_brain_" + configured.Name
		principals[principalID] = identity.Principal{
			ID: principalID, Kind: identity.PrincipalWorkload,
			DisplayName: configured.Name,
		}
		registry.Workloads = append(registry.Workloads, identity.Workload{
			Brain: configured.Name, PrincipalID: principalID,
			ResponsiblePrincipalID: consoleResponsiblePrincipal,
		})
	}
	for _, principal := range principals {
		registry.Principals = append(registry.Principals, principal)
	}
	sort.Slice(registry.Principals, func(i, j int) bool {
		return registry.Principals[i].ID < registry.Principals[j].ID
	})
	sort.Slice(registry.Bindings, func(i, j int) bool {
		return registry.Bindings[i].ID < registry.Bindings[j].ID
	})
	sort.Slice(registry.Workloads, func(i, j int) bool {
		return registry.Workloads[i].Brain < registry.Workloads[j].Brain
	})
	return registry
}

func phase1IdentityRuntime(cfg *config.Config) (identity.Registry, *identity.Resolver, map[string]*identity.Issuer, error) {
	registry := phase1IdentityRegistry(cfg)
	resolver, err := identity.NewResolver(registry, time.Now)
	if err != nil {
		return identity.Registry{}, nil, nil, err
	}
	issuers := make(map[string]*identity.Issuer)
	newIssuer := func(channel, method, credential string) error {
		issuer, err := resolver.NewIssuer(identity.IssuerConfig{
			BindingID: "binding_" + channel, Method: method,
			CredentialClass: credential, TTL: 5 * time.Minute,
		})
		if err != nil {
			return err
		}
		issuers[channel] = issuer
		return nil
	}
	if err := newIssuer("console", "bearer", "shared_secret"); err != nil {
		return identity.Registry{}, nil, nil, err
	}
	// Same fold as phase1IdentityRegistry: one issuer per channel TYPE. A
	// second issuer over the same binding would silently supersede the first.
	issued := map[string]bool{}
	for _, channel := range cfg.Channels {
		if issued[channel.Type] {
			continue
		}
		method, credential := channel.Mode, "shared_secret"
		switch channel.Type {
		case "telegram":
			if method == "" {
				method = "polling"
			}
			credential = "bot_token_session" // #nosec G101 -- credential class, never a token value
		case "discord":
			method, credential = "gateway_session", "bot_token_session" // #nosec G101 -- credential classes
		case "webhook":
			method = "bearer"
		case "console":
			continue
		default:
			continue
		}
		if err := newIssuer(channel.Type, method, credential); err != nil {
			return identity.Registry{}, nil, nil, err
		}
		issued[channel.Type] = true
	}
	return registry, resolver, issuers, nil
}

func wireIdentitySigners(store *actionsqlite.Store, privateKey ed25519.PrivateKey) {
	store.SetIdentitySigners(
		func(e identity.Evidence) identity.SignedEvidence {
			return identity.SignEvidence(privateKey, e)
		},
		func(e identity.PrincipalEvent) identity.SignedPrincipalEvent {
			return identity.SignPrincipalEvent(privateKey, e)
		},
	)
}

// authenticatedFixtureChannel gives a channel supplied through
// WithChannelFactory an ingress capability after that channel yields an
// accepted envelope. Built-in adapters mint at their own authentication cut.
type authenticatedFixtureChannel struct {
	Channel
	issuer *identity.Issuer
}

func authenticateFixtureChannel(ch Channel, issuer *identity.Issuer) Channel {
	base := &authenticatedFixtureChannel{Channel: ch, issuer: issuer}
	dropped, hasDropped := ch.(droppedCounter)
	reconnect, hasReconnect := ch.(reconnectCounter)
	switch {
	case hasDropped && hasReconnect:
		return &authenticatedFixtureCountedChannel{
			authenticatedFixtureChannel: base, dropped: dropped, reconnect: reconnect,
		}
	case hasDropped:
		return &authenticatedFixtureDroppedChannel{
			authenticatedFixtureChannel: base, dropped: dropped,
		}
	case hasReconnect:
		return &authenticatedFixtureReconnectChannel{
			authenticatedFixtureChannel: base, reconnect: reconnect,
		}
	default:
		return base
	}
}

func (c *authenticatedFixtureChannel) Receive(ctx context.Context) (<-chan *envelope.Envelope, error) {
	in, err := c.Channel.Receive(ctx)
	if err != nil {
		return nil, err
	}
	out := make(chan *envelope.Envelope)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case env, ok := <-in:
				if !ok {
					return
				}
				if c.issuer == nil || env == nil {
					continue
				}
				ingress, err := c.issuer.Issue(env.ID, env.Sender.ID)
				if err != nil {
					continue
				}
				env.SetAuthenticatedIngress(ingress)
				select {
				case out <- env:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

type authenticatedFixtureDroppedChannel struct {
	*authenticatedFixtureChannel
	dropped droppedCounter
}

func (c *authenticatedFixtureDroppedChannel) DroppedCount() uint64 {
	return c.dropped.DroppedCount()
}

type authenticatedFixtureReconnectChannel struct {
	*authenticatedFixtureChannel
	reconnect reconnectCounter
}

func (c *authenticatedFixtureReconnectChannel) ReconnectCount() uint64 {
	return c.reconnect.ReconnectCount()
}

type authenticatedFixtureCountedChannel struct {
	*authenticatedFixtureChannel
	dropped   droppedCounter
	reconnect reconnectCounter
}

func (c *authenticatedFixtureCountedChannel) DroppedCount() uint64 {
	return c.dropped.DroppedCount()
}

func (c *authenticatedFixtureCountedChannel) ReconnectCount() uint64 {
	return c.reconnect.ReconnectCount()
}

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

// RecordAttemptAuthenticated commits the action and its Phase 1 evidence
// through the store's born-whole v2 identity door.
func (r actionRecorder) RecordAttemptAuthenticated(ctx context.Context, env action.Envelope, outcome, rule string, state action.State, evidence identity.Evidence) error {
	return r.store.RecordAttemptAuthenticated(ctx, env,
		actionsqlite.Decision{
			Outcome: outcome, Rule: rule,
			PolicyVersion: r.pin.Version, PolicyDigest: r.pin.Digest,
		}, state, evidence)
}
