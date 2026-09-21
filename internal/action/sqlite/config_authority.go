// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

const (
	configAuthorityClauseDomain = "korvun.config-authority-clause.v1"
	configAuthorityHeadDomain   = "korvun.config-authority-head.v1"
)

type configAuthorityHead struct {
	ProfileID      string   `json:"profile_id"`
	BrainPrincipal string   `json:"brain_principal_id"`
	Generation     int64    `json:"generation"`
	ClauseSet      string   `json:"clause_set_digest"`
	ClauseIDs      []string `json:"clause_ids"`
}

// ActivatedAuthorityProfile returns the profile proved by the pinned strict
// activation root. An empty result means this store instance is not armed.
func (s *Store) ActivatedAuthorityProfile() string { return s.authorityProfileID }

// SyncConfigAuthorityClauses signs one immutable generation and atomically
// advances the brain's current head. Replaying identical terms is a verified
// no-op; an empty set is a real generation that withdraws every config clause.
func (s *Store) SyncConfigAuthorityClauses(ctx context.Context, profileID,
	brainPrincipal string, clauses []action.ConfigAuthorityClause, at time.Time) (int64, error) {
	if profileID == "" || brainPrincipal == "" ||
		s.authorityActivationDigest == "" || s.authorityProfileID != profileID {
		return 0, action.ErrAuthorityEvidenceCorrupt
	}
	ordered := append([]action.ConfigAuthorityClause(nil), clauses...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ClauseID < ordered[j].ClauseID })
	ids := make([]string, len(ordered))
	seenTools := make(map[string]bool, len(ordered))
	for i, clause := range ordered {
		if err := clause.Validate(); err != nil || clause.ProfileID != profileID ||
			clause.BrainPrincipal != brainPrincipal || seenTools[clause.ToolName] {
			return 0, action.ErrAuthorityMalformed
		}
		seenTools[clause.ToolName] = true
		ids[i] = clause.ClauseID
	}
	setRaw, _ := json.Marshal(ids)
	setDigest := action.HashCanonical(string(setRaw))

	tx, err := s.beginAuthorityWrite(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.verifyAuthorityActivationTx(ctx, tx, profileID, s.authorityActivationDigest); err != nil {
		return 0, err
	}
	var generation int64
	var currentSet string
	err = tx.QueryRowContext(ctx, `SELECT generation,clause_set_digest FROM config_authority_heads WHERE profile_id=? AND brain_principal_id=?`, profileID, brainPrincipal).Scan(&generation, &currentSet)
	switch {
	case err == nil:
		if _, err := s.configAuthorityClausesTx(ctx, tx, profileID, brainPrincipal); err != nil {
			return 0, err
		}
		if currentSet == setDigest {
			if err := tx.Commit(); err != nil {
				return 0, mapAuthorityStoreError(err)
			}
			return generation, nil
		}
		generation++
	case errors.Is(err, sql.ErrNoRows):
		generation = 1
	default:
		return 0, err
	}

	for _, clause := range ordered {
		canonical := clause.CanonicalBytes()
		seal, err := s.signAuthorityTx(ctx, tx, configAuthorityClauseDomain, canonical)
		if err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO config_authority_snapshots(profile_id,brain_principal_id,generation,clause_id,canonical_clause,digest,signing_key_id,signature) VALUES(?,?,?,?,?,?,?,?)`,
			profileID, brainPrincipal, generation, clause.ClauseID, canonical,
			seal.Digest, seal.SigningKeyID, seal.Signature); err != nil {
			return 0, err
		}
		if err := s.ensureBudgetAccountTx(ctx, tx, profileID, "config",
			brainPrincipal+"\x00"+clause.ToolName, action.IntentBudgetV2{}, at); err != nil {
			return 0, err
		}
	}
	head := configAuthorityHead{profileID, brainPrincipal, generation, setDigest, ids}
	canonicalHead := mustAuthorityJSON(head)
	headSeal, err := s.signAuthorityTx(ctx, tx, configAuthorityHeadDomain, canonicalHead)
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO config_authority_heads(profile_id,brain_principal_id,generation,clause_set_digest,canonical_head,digest,signing_key_id,signature) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(profile_id,brain_principal_id) DO UPDATE SET generation=excluded.generation,clause_set_digest=excluded.clause_set_digest,canonical_head=excluded.canonical_head,digest=excluded.digest,signing_key_id=excluded.signing_key_id,signature=excluded.signature`,
		profileID, brainPrincipal, generation, setDigest, canonicalHead,
		headSeal.Digest, headSeal.SigningKeyID, headSeal.Signature); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, mapAuthorityStoreError(err)
	}
	return generation, nil
}

func (s *Store) configAuthorityClausesTx(ctx context.Context, tx *sql.Tx,
	profileID, brainPrincipal string) ([]action.ConfigAuthorityClause, error) {
	var generation int64
	var setDigest string
	var canonical []byte
	var digest, keyID, signature string
	if err := tx.QueryRowContext(ctx, `SELECT generation,clause_set_digest,canonical_head,digest,signing_key_id,signature FROM config_authority_heads WHERE profile_id=? AND brain_principal_id=?`, profileID, brainPrincipal).
		Scan(&generation, &setDigest, &canonical, &digest, &keyID, &signature); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAuthorityMissing
	} else if err != nil {
		return nil, err
	}
	var head configAuthorityHead
	if err := json.Unmarshal(canonical, &head); err != nil ||
		head.ProfileID != profileID || head.BrainPrincipal != brainPrincipal ||
		head.Generation != generation || head.ClauseSet != setDigest ||
		!bytes.Equal(canonical, mustAuthorityJSON(head)) {
		return nil, action.ErrAuthorityEvidenceCorrupt
	}
	pub, _, err := publicKeyTx(ctx, tx, keyID)
	if err != nil || action.VerifyAuthorityBytes(pub, configAuthorityHeadDomain, canonical,
		action.AuthoritySignature{Digest: digest, SigningKeyID: keyID, Signature: signature}) != nil {
		return nil, action.ErrAuthorityEvidenceCorrupt
	}
	rows, err := tx.QueryContext(ctx, `SELECT clause_id,canonical_clause,digest,signing_key_id,signature FROM config_authority_snapshots WHERE profile_id=? AND brain_principal_id=? AND generation=? ORDER BY clause_id`, profileID, brainPrincipal, generation)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	clauses := make([]action.ConfigAuthorityClause, 0)
	ids := make([]string, 0)
	for rows.Next() {
		var clauseID string
		var raw []byte
		var storedDigest, storedKey, storedSignature string
		if err := rows.Scan(&clauseID, &raw, &storedDigest, &storedKey, &storedSignature); err != nil {
			return nil, err
		}
		var wire struct {
			SchemaVersion  int      `json:"schema_version"`
			ProfileID      string   `json:"profile_id"`
			BrainPrincipal string   `json:"brain_principal_id"`
			ToolName       string   `json:"tool_name"`
			Channels       []string `json:"channels"`
			CageDigest     string   `json:"cage_digest"`
		}
		if err := json.Unmarshal(raw, &wire); err != nil {
			return nil, action.ErrAuthorityEvidenceCorrupt
		}
		clause := action.ConfigAuthorityClause{
			ClauseID: clauseID, SchemaVersion: wire.SchemaVersion,
			ProfileID: wire.ProfileID, BrainPrincipal: wire.BrainPrincipal,
			ToolName: wire.ToolName, Channels: wire.Channels, CageDigest: wire.CageDigest,
		}
		pub, _, err := publicKeyTx(ctx, tx, storedKey)
		if err != nil || clause.Validate() != nil || storedDigest != clause.Digest() ||
			!bytes.Equal(raw, clause.CanonicalBytes()) ||
			action.VerifyAuthorityBytes(pub, configAuthorityClauseDomain, raw,
				action.AuthoritySignature{Digest: storedDigest, SigningKeyID: storedKey, Signature: storedSignature}) != nil {
			return nil, action.ErrAuthorityEvidenceCorrupt
		}
		clauses = append(clauses, clause)
		ids = append(ids, clauseID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	idsRaw, _ := json.Marshal(ids)
	if !sameStringSlice(ids, head.ClauseIDs) || action.HashCanonical(string(idsRaw)) != setDigest {
		return nil, action.ErrAuthorityEvidenceCorrupt
	}
	return clauses, nil
}

func (s *Store) configAuthorityClauseTx(ctx context.Context, tx *sql.Tx,
	profileID, brainPrincipal, toolName, channel string) (action.ConfigAuthorityClause, error) {
	clauses, err := s.configAuthorityClausesTx(ctx, tx, profileID, brainPrincipal)
	if err != nil {
		return action.ConfigAuthorityClause{}, err
	}
	var hits []action.ConfigAuthorityClause
	for _, clause := range clauses {
		if clause.ToolName == toolName && containsAuthorityString(clause.Channels, channel) {
			hits = append(hits, clause)
		}
	}
	if len(hits) == 0 {
		return action.ConfigAuthorityClause{}, ErrAuthorityMissing
	}
	if len(hits) != 1 {
		return action.ConfigAuthorityClause{}, ErrAuthorityAmbiguous
	}
	return hits[0], nil
}

func configAccountScope(brainPrincipal, toolName string) string {
	return strings.Join([]string{brainPrincipal, toolName}, "\x00")
}
