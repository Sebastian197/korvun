// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// This file adds the operator-console surface of the control API
// (operator-console spec 2026-08-08, SP3: FR-API-1/1b/1c/2/3). Unlike the
// secret-free wiring reads, these endpoints carry MESSAGE CONTENT in their
// responses — so EVERY route here, reads included, sits behind the same
// bearer gate as the config mutation (the spec's posture: content leaves
// the process only authenticated, on the loopback-default admin server).
// Dependency note: this file leans on the conversation seam and the
// router's sentinel errors — the "leaf" posture of the wiring reads is
// consciously widened here, one-directionally (router never imports
// controlapi).
package controlapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/router"
)

// OperatorRouter is the seam to the router's SP2 operator surface
// (*router.Router satisfies it): the public outbound entry and the takeover
// gate. An interface so the handlers stay testable with a fake.
type OperatorRouter interface {
	DispatchOutbound(ctx context.Context, env *envelope.Envelope) error
	TakeOver(key conversation.Key)
	Release(key conversation.Key)
	TakenOver(key conversation.Key) bool
}

// Wire shapes. Content-bearing on purpose (bearer-gated); timestamps are
// RFC3339Nano, zero timestamps serialize as "".
type conversationRow struct {
	Key           string `json:"key"`
	ActiveSession int    `json:"active_session"`
	SessionCount  int    `json:"session_count"`
	LastActivity  string `json:"last_activity,omitempty"`
	LastRole      string `json:"last_role,omitempty"`
	TakenOver     bool   `json:"taken_over"`
}

type sessionRow struct {
	ID        int    `json:"id"`
	TurnCount int    `json:"turn_count"`
	First     string `json:"first,omitempty"`
	Last      string `json:"last,omitempty"`
}

type turnRow struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp,omitempty"`
	Seq       int    `json:"seq"`
}

// defaultInboxLimit bounds GET /api/conversations when no limit is given
// (FR-STORE-1 pagination-by-limit, cursors are a future additive).
const defaultInboxLimit = 50

// maxReplyBodyBytes caps the reply body — operator messages are chat-sized.
const maxReplyBodyBytes = 64 << 10 // 64 KiB

// RegisterConsole mounts the operator-console endpoints on m, every route —
// reads included — behind the bearer gate (they carry message content). Call
// it ONLY when a non-empty token is configured, and before the server starts.
func RegisterConsole(m Mounter, token string, store conversation.SessionStore, op OperatorRouter) {
	auth := bearerAuth(token)
	m.Handle("GET /api/conversations", auth(listConversationsHandler(store, op)))
	m.Handle("GET /api/conversations/{key}", auth(conversationDetailHandler(store)))
	m.Handle("GET /api/conversations/{key}/sessions", auth(listSessionsHandler(store)))
	m.Handle("GET /api/conversations/{key}/sessions/{id}", auth(sessionDetailHandler(store)))
	m.Handle("POST /api/conversations/{key}/sessions", auth(newSessionHandler(store)))
	m.Handle("POST /api/conversations/{key}/reply", auth(replyHandler(op)))
	m.Handle("POST /api/conversations/{key}/takeover", auth(takeoverHandler(op, true)))
	m.Handle("POST /api/conversations/{key}/release", auth(takeoverHandler(op, false)))
}

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// splitKey validates a path key as channel::conversation-id with BOTH halves
// non-empty (the reply path relies on this: without a conversation identity
// the operator turn could not be persisted, so the request is rejected).
func splitKey(raw string) (channelName, convID string, ok bool) {
	channelName, convID, found := strings.Cut(raw, "::")
	if !found || channelName == "" || convID == "" {
		return "", "", false
	}
	return channelName, convID, true
}

func listConversationsHandler(store conversation.SessionStore, op OperatorRouter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit := defaultInboxLimit
		if v := r.URL.Query().Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 {
				writeError(w, http.StatusBadRequest, "limit must be a positive integer")
				return
			}
			limit = n
		}
		rows, err := store.ListConversations(r.Context(), limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "listing conversations failed")
			return
		}
		out := make([]conversationRow, 0, len(rows))
		for _, c := range rows {
			out = append(out, conversationRow{
				Key:           string(c.Key),
				ActiveSession: c.ActiveSession,
				SessionCount:  c.SessionCount,
				LastActivity:  fmtTime(c.LastActivity),
				LastRole:      string(c.LastRole),
				TakenOver:     op.TakenOver(c.Key),
			})
		}
		writeJSON(w, out)
	})
}

// defaultDetailTurns bounds the active-session read of the detail endpoint.
const defaultDetailTurns = 100

func conversationDetailHandler(store conversation.SessionStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := splitKey(r.PathValue("key")); !ok {
			writeError(w, http.StatusBadRequest, "key must be channel::conversation-id")
			return
		}
		n := defaultDetailTurns
		if v := r.URL.Query().Get("n"); v != "" {
			parsed, err := strconv.Atoi(v)
			if err != nil || parsed <= 0 {
				writeError(w, http.StatusBadRequest, "n must be a positive integer")
				return
			}
			n = parsed
		}
		turns, err := store.LoadRecent(r.Context(), conversation.Key(r.PathValue("key")), n)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "loading conversation failed")
			return
		}
		writeJSON(w, toTurnRows(turns))
	})
}

func listSessionsHandler(store conversation.SessionStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessions, err := store.ListSessions(r.Context(), conversation.Key(r.PathValue("key")))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "listing sessions failed")
			return
		}
		out := make([]sessionRow, 0, len(sessions))
		for _, s := range sessions {
			out = append(out, sessionRow{
				ID:        s.ID,
				TurnCount: s.TurnCount,
				First:     fmtTime(s.First),
				Last:      fmtTime(s.Last),
			})
		}
		writeJSON(w, out)
	})
}

func sessionDetailHandler(store conversation.SessionStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "session id must be a positive integer")
			return
		}
		turns, err := store.LoadSession(r.Context(), conversation.Key(r.PathValue("key")), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "loading session failed")
			return
		}
		writeJSON(w, toTurnRows(turns))
	})
}

func newSessionHandler(store conversation.SessionStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := splitKey(r.PathValue("key")); !ok {
			writeError(w, http.StatusBadRequest, "key must be channel::conversation-id")
			return
		}
		// FR-SESS-4: the console reset — same semantics as the channel
		// trigger, NO acknowledgement message.
		id, err := store.NewSession(r.Context(), conversation.Key(r.PathValue("key")))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "opening session failed")
			return
		}
		writeJSON(w, map[string]int{"session": id})
	})
}

func replyHandler(op OperatorRouter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		channelName, convID, ok := splitKey(r.PathValue("key"))
		if !ok {
			writeError(w, http.StatusBadRequest, "key must be channel::conversation-id")
			return
		}
		var body struct {
			Text string `json:"text"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxReplyBodyBytes))
		if err := dec.Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "body must be JSON with a text field")
			return
		}
		if strings.TrimSpace(body.Text) == "" {
			writeError(w, http.StatusBadRequest, "text must not be empty")
			return
		}

		env := envelope.New(channelName, envelope.Outbound,
			envelope.Participant{ID: "operator", Name: "Operator"}).AddText(body.Text)
		env.Meta[conversation.MetaConversationID] = convID

		err := op.DispatchOutbound(r.Context(), env)
		switch {
		case err == nil:
			// Accepted: the funnel is asynchronous by design (AS-5 as
			// amended) — the operator turn is persisted and the envelope is
			// queued; a delivery failure surfaces through events.
			w.WriteHeader(http.StatusAccepted)
		case errors.Is(err, router.ErrUnknownChannel):
			// The conversation's channel is not registered in this process —
			// an honest conflict, not a client typo.
			writeError(w, http.StatusConflict, "channel not registered")
		case errors.Is(err, router.ErrChannelSaturated):
			writeError(w, http.StatusServiceUnavailable, "channel outbound queue saturated")
		default:
			writeError(w, http.StatusInternalServerError, "reply failed")
		}
	})
}

func takeoverHandler(op OperatorRouter, take bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := splitKey(r.PathValue("key")); !ok {
			writeError(w, http.StatusBadRequest, "key must be channel::conversation-id")
			return
		}
		key := conversation.Key(r.PathValue("key"))
		if take {
			op.TakeOver(key)
		} else {
			op.Release(key)
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func toTurnRows(turns []conversation.Turn) []turnRow {
	out := make([]turnRow, 0, len(turns))
	for _, t := range turns {
		out = append(out, turnRow{
			Role:      string(t.Role),
			Content:   t.Content,
			Timestamp: fmtTime(t.Timestamp),
			Seq:       t.Seq,
		})
	}
	return out
}
