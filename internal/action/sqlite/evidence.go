// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Evidence persistence (Trust Layer Etapa 2, lote 3, spec FR-EVID-2):
// identity evidence lands in the SAME transaction as the attempt and its
// decision — they enter together or nothing enters at all, the Etapa-1
// record-before-effect discipline extended to identity. Evidence rows
// ride ON DELETE CASCADE with their action, so the retention cap bounds
// them too.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// StoredIdentity are the identity refs of one identified (v2) action row.
type StoredIdentity struct {
	// PrincipalID / IntentID are the row's identity references.
	PrincipalID string
	IntentID    string
	// AuthorityRefs are the grant ids the authorization walked, in order.
	AuthorityRefs []string
}

// AttemptIdentity is everything identity-related recorded WITH an attempt.
type AttemptIdentity struct {
	// PrincipalID / IntentID / AuthorityRefs land as row columns.
	PrincipalID   string
	IntentID      string
	AuthorityRefs []string
	// Evidence lands in the evidence table, same transaction.
	Evidence action.IdentityEvidence
}

// RecordAttemptIdentified persists one attempt WITH its decision, its
// identity refs and its evidence in a single transaction (FR-EVID-2).
// Same guard as RecordAttempt: only decision states may enter.
func (s *Store) RecordAttemptIdentified(ctx context.Context, env action.Envelope, d Decision, state action.State, ident AttemptIdentity) error {
	switch state {
	case action.StateDenied, action.StateShadowed, action.StateAuthorized:
	default:
		return fmt.Errorf("%w: %s", ErrNotADecisionState, state)
	}
	// Empty refs land as SQL NULL (never the JSON text "null"), so audit
	// queries render their honest dash for acts with no explaining grant.
	var refs any
	if len(ident.AuthorityRefs) > 0 {
		raw, err := json.Marshal(ident.AuthorityRefs)
		if err != nil {
			// Unreachable for a string slice; kept for honesty.
			return fmt.Errorf("action/sqlite: marshal authority refs %q: %w", env.ActionID, err)
		}
		refs = string(raw)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("action/sqlite: begin identified record: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	requestedAt := env.RequestedAt.UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO actions (action_id, schema_version, correlation_id,
		    source_kind, source_protocol, source_channel,
		    op_namespace, op_name, op_version,
		    parameters_digest, effect_class, state, requested_at,
		    principal_id, intent_id, authority_refs)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		env.ActionID, env.SchemaVersion, env.CorrelationID,
		env.Source.Kind, env.Source.Protocol, env.Source.Channel,
		env.Operation.Namespace, env.Operation.Name, env.Operation.Version,
		env.ParametersDigest, env.Effect.Class, string(state), requestedAt,
		ident.PrincipalID, ident.IntentID, refs,
	); err != nil {
		return fmt.Errorf("action/sqlite: insert identified action %q: %w", env.ActionID, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO action_decisions (action_id, outcome, rule, decided_at) VALUES (?, ?, ?, ?)`,
		env.ActionID, d.Outcome, d.Rule, requestedAt,
	); err != nil {
		return fmt.Errorf("action/sqlite: insert decision %q: %w", env.ActionID, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO evidence (evidence_id, action_id, provider, subject,
		    credential, issued_at, transport_binding, claims_digest)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ident.Evidence.EvidenceID, env.ActionID, ident.Evidence.Provider,
		ident.Evidence.Subject, string(ident.Evidence.Credential),
		ident.Evidence.IssuedAt.UTC().Format(time.RFC3339Nano),
		ident.Evidence.TransportBinding, ident.Evidence.ClaimsDigest,
	); err != nil {
		return fmt.Errorf("action/sqlite: insert evidence for %q: %w", env.ActionID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("action/sqlite: commit identified record %q: %w", env.ActionID, err)
	}
	return s.noteWrite(ctx)
}

// GetEvidence returns the identity evidence recorded with one action.
func (s *Store) GetEvidence(ctx context.Context, actionID string) (action.IdentityEvidence, error) {
	var (
		ev         action.IdentityEvidence
		credential string
		issuedAt   string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT evidence_id, provider, subject, credential, issued_at,
		        transport_binding, claims_digest
		   FROM evidence WHERE action_id = ?`, actionID,
	).Scan(&ev.EvidenceID, &ev.Provider, &ev.Subject, &credential,
		&issuedAt, &ev.TransportBinding, &ev.ClaimsDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return action.IdentityEvidence{}, fmt.Errorf("%w: evidence of %q", ErrNotFound, actionID)
	}
	if err != nil {
		return action.IdentityEvidence{}, fmt.Errorf("action/sqlite: get evidence %q: %w", actionID, err)
	}
	ev.Credential = action.CredentialType(credential)
	at, err := time.Parse(time.RFC3339Nano, issuedAt)
	if err != nil {
		return action.IdentityEvidence{}, fmt.Errorf("action/sqlite: parse issued_at of %q: %w", actionID, err)
	}
	ev.IssuedAt = at
	return ev, nil
}
