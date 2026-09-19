// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
)

// errApprovalRowInvalid marks a row that WAS read and does not convert: a
// column holding a value of the wrong kind, or a time that does not parse.
// It is the only way a read of the approval row may be classified as corrupt
// evidence (v0.15.1 block B, P2-6): every error Scan itself returns is the
// store not answering — a driver failure, a cancelled context, a statement
// that cannot be prepared — and is classified unreadable.
//
// The split is made by construction, not by reading error text: the row is
// scanned into raw driver values, which never fails on content, and the
// conversion to typed fields happens here.
var errApprovalRowInvalid = errors.New("action/sqlite: the approval row does not convert")

// approvalColumnCount is the number of columns in approvalColumns.
const approvalColumnCount = 18

// scanRawApproval scans the approval columns followed by `extra` raw values.
// Its error is always a store failure (or sql.ErrNoRows): raw destinations
// cannot fail on content.
func scanRawApproval(row scanner, extra int) ([]any, error) {
	vals := make([]any, approvalColumnCount+extra)
	ptrs := make([]any, len(vals))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := row.Scan(ptrs...); err != nil {
		return nil, err
	}
	return vals, nil
}

// approvalRawText and approvalRawInt follow what database/sql's typed Scan
// accepted before this file existed (convertAssign): a text column takes text,
// bytes or a number; an integer column takes an integer or text that parses as
// one; NULL is accepted only where the previous destination was a NullString.
// Everything else is a row that does not convert. Two differences, declared:
// the typed Scan also took an integral REAL (3.0) into an integer field, and
// approvalRawInt names it corrupt (not moulded: the approval columns are
// INTEGER affinity, which stores such a value as an integer, so it reaches
// this reader only from a column rebuilt without that affinity); and
// approvalRawIntAsInt names a schema_version outside the int32 range corrupt
// where the typed Scan took any value that fit the platform's int.
func approvalRawText(v any, name string, nullable bool) (string, bool, error) {
	switch x := v.(type) {
	case nil:
		if nullable {
			return "", false, nil
		}
		return "", false, fmt.Errorf("%w: %s is NULL", errApprovalRowInvalid, name)
	case string:
		return x, true, nil
	case []byte:
		return string(x), true, nil
	case int64:
		return strconv.FormatInt(x, 10), true, nil
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64), true, nil
	default:
		return "", false, fmt.Errorf("%w: %s holds a %T, not text", errApprovalRowInvalid, name, v)
	}
}

func approvalRawInt(v any, name string) (int64, error) {
	switch x := v.(type) {
	case int64:
		return x, nil
	case string, []byte:
		s, _, _ := approvalRawText(x, name, false)
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("%w: %s holds text that is not an integer", errApprovalRowInvalid, name)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("%w: %s holds a %T, not an integer", errApprovalRowInvalid, name, v)
	}
}

func approvalRawTime(v any, name string, nullable bool) (time.Time, error) {
	s, present, err := approvalRawText(v, name, nullable)
	if err != nil {
		return time.Time{}, err
	}
	if !present || s == "" {
		if nullable {
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("%w: %s is empty", errApprovalRowInvalid, name)
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %s does not parse: %w", errApprovalRowInvalid, name, err)
	}
	return t, nil
}

// approvalFromRaw converts the approval columns, in approvalColumns order.
func approvalFromRaw(v []any) (action.Approval, error) {
	var (
		a   action.Approval
		err error
	)
	texts := []struct {
		dst      *string
		i        int
		name     string
		nullable bool
	}{
		{&a.ApprovalID, 0, "approval_id", false},
		{&a.ActionID, 2, "action_id", false},
		{&a.ActionDigest, 3, "action_digest", false},
		{&a.PreviewDigest, 4, "preview_digest", false},
		{&a.RequestedFrom, 5, "requested_from", false},
		{&a.Reason, 6, "reason", false},
		{&a.RiskSummary, 7, "risk_summary", false},
		{&a.PolicyDigest, 9, "policy_digest", false},
		{&a.DecisionPrincipalID, 13, "decision_principal_id", false},
		{&a.Decision, 14, "decision", false},
		{&a.Comment, 16, "comment", false},
		{&a.DecisionReceiptID, 17, "decision_receipt_id", false},
	}
	if a.SchemaVersion, err = approvalRawIntAsInt(v[1], "schema_version"); err != nil {
		return action.Approval{}, err
	}
	if a.PolicyVersion, err = approvalRawInt(v[8], "policy_version"); err != nil {
		return action.Approval{}, err
	}
	for _, t := range texts {
		if *t.dst, _, err = approvalRawText(v[t.i], t.name, t.nullable); err != nil {
			return action.Approval{}, err
		}
	}
	status, _, err := approvalRawText(v[12], "status", false)
	if err != nil {
		return action.Approval{}, err
	}
	a.Status = action.ApprovalStatus(status)
	if a.RequestedAt, err = approvalRawTime(v[10], "requested_at", false); err != nil {
		return action.Approval{}, err
	}
	if a.ExpiresAt, err = approvalRawTime(v[11], "expires_at", true); err != nil {
		return action.Approval{}, err
	}
	if a.DecisionAt, err = approvalRawTime(v[15], "decision_at", true); err != nil {
		return action.Approval{}, err
	}
	return a, nil
}

// approvalRawIntAsInt narrows a stored integer to int. A value outside the
// int32 range is corrupt evidence: no schema_version this store writes comes
// near it, and the bound keeps the conversion exact on any platform whose int
// is 32 bits wide instead of silently truncating the stored value.
func approvalRawIntAsInt(v any, name string) (int, error) {
	n, err := approvalRawInt(v, name)
	if err != nil {
		return 0, err
	}
	if n < math.MinInt32 || n > math.MaxInt32 {
		return 0, fmt.Errorf("%w: %s holds %d, outside the int32 range", errApprovalRowInvalid, name, n)
	}
	return int(n), nil
}

// classifyApprovalRead names a failed read of one approval row, with exactly
// ONE of the two classes (never both): absent, corrupt (the row was read and
// does not convert) or unreadable (the store did not answer).
func classifyApprovalRead(approvalID string, err error) error {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("action/sqlite: approval %q: %w", approvalID, ErrApprovalNotFound)
	case errors.Is(err, errApprovalRowInvalid):
		return fmt.Errorf("action/sqlite: approval %q: %w: %w", approvalID, ErrApprovalEvidenceCorrupt, err)
	default:
		return fmt.Errorf("action/sqlite: approval %q: %w: %w", approvalID, ErrApprovalUnreadable, err)
	}
}
