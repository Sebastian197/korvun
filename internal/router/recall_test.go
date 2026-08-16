// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// SP-A red suite (minimal-memory spec 2026-08-16, FR-RECALL-1; ADR-0043 §2):
// the /recall command on the dispatch path — one quoted block into an empty
// active session, bounded newest-first source scan, fixed grammar and fixed
// honest replies. Runs on the SP2 molde: fake channel/brain, a real
// MemStore, and the injected clock; the spec constants are referenced by
// NAME (RecallTailWindow / RecallScanSessions / RecallBlockRunes), never by
// repeated literal.
package router_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/router"
)

// recallPolicy is the session policy every /recall test starts from.
func recallPolicy(max int) router.SessionPolicy {
	return router.SessionPolicy{Triggers: defaultTriggers, RecallMax: max}
}

// seedDialogue appends n alternating user/assistant turns d1..dn to k's
// active session (odd = user, even = assistant).
func seedDialogue(t *testing.T, store conversation.SessionStore, k conversation.Key, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		role := conversation.RoleUser
		if i%2 == 0 {
			role = conversation.RoleAssistant
		}
		if _, err := store.Append(context.Background(), k,
			conversation.Turn{Role: role, Content: fmt.Sprintf("d%d", i), Timestamp: time.Unix(int64(100+i), 0)}); err != nil {
			t.Fatalf("seedDialogue(%d): %v", i, err)
		}
	}
}

// archive closes k's active session by opening a fresh one.
func archive(t *testing.T, store conversation.SessionStore, k conversation.Key) {
	t.Helper()
	if _, err := store.NewSession(context.Background(), k); err != nil {
		t.Fatalf("archive: %v", err)
	}
}

// appendSystem puts one system turn (an ack) into k's active session.
func appendSystem(t *testing.T, store conversation.SessionStore, k conversation.Key, content string) {
	t.Helper()
	if _, err := store.Append(context.Background(), k,
		conversation.Turn{Role: conversation.RoleSystem, Content: content, Timestamp: time.Unix(500, 0)}); err != nil {
		t.Fatalf("appendSystem: %v", err)
	}
}

// activeTurns returns the turns of k's ACTIVE (highest-id) session.
func activeTurns(t *testing.T, store conversation.SessionStore, k conversation.Key) []conversation.Turn {
	t.Helper()
	ss, err := store.ListSessions(context.Background(), k)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(ss) == 0 {
		return nil
	}
	turns, err := store.LoadSession(context.Background(), k, ss[len(ss)-1].ID)
	if err != nil {
		t.Fatalf("LoadSession(active): %v", err)
	}
	return turns
}

// recallRouterOn is sessionRouter generalized over the channel name — the
// AS-A6 channel-agnostic assertion needs a network-channel name that is not
// the console.
func recallRouterOn(t *testing.T, name string, store conversation.SessionStore, opts ...router.Option) (*router.Router, *fakeChannel, *fakeBrain) {
	t.Helper()
	ch := newFakeChannel(name)
	br := newFakeBrain()
	base := []router.Option{router.WithSessionStore(store)}
	r := router.New(append(base, opts...)...)
	if err := r.RegisterChannel(ch); err != nil {
		t.Fatalf("RegisterChannel: %v", err)
	}
	if err := r.RegisterBrain("b", br); err != nil {
		t.Fatalf("RegisterBrain: %v", err)
	}
	if err := r.Route(name, "b"); err != nil {
		t.Fatalf("Route: %v", err)
	}
	t.Cleanup(func() { shutdown(t, r) })
	return r, ch, br
}

// errStoreDown is what the failing store returns; AS-A7's honest-error path.
var errStoreDown = errors.New("store down")

// failingRecallStore embeds a real SessionStore and fails every write —
// the AS-A7 mid-command failure.
type failingRecallStore struct {
	conversation.SessionStore
}

func (f *failingRecallStore) Append(context.Context, conversation.Key, conversation.Turn) (conversation.Turn, error) {
	return conversation.Turn{}, errStoreDown
}

func (f *failingRecallStore) AppendTurns(context.Context, conversation.Key, ...conversation.Turn) ([]conversation.Turn, error) {
	return nil, errStoreDown
}

// --- AS-A1: the quoted-block import ----------------------------------------

func TestRecall_ImportsOneQuotedBlock(t *testing.T) {
	store := conversation.NewMemStore()
	seedDialogue(t, store, key, 12)
	archive(t, store, key) // session 1 archived with d1..d12; active 2 empty
	r, ch, br := sessionRouter(t, store,
		router.WithSessionPolicy(recallPolicy(10)))

	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "/recall 4")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	waitUntil(t, "the recall ack to leave", func() bool { return len(ch.Sent()) == 1 })

	ack := ch.Sent()[0]
	if want := fmt.Sprintf(router.RecallAckFormat, 4, 1); len(ack.Parts) != 1 || ack.Parts[0].Content != want {
		t.Fatalf("ack copy = %q, want %q (names count and session)", ack.Parts[0].Content, want)
	}
	if ack.Meta[envelope.MetaAck] != envelope.AckRecall {
		t.Fatalf("ack MetaAck = %q, want %q", ack.Meta[envelope.MetaAck], envelope.AckRecall)
	}
	if n := len(br.Handled()); n != 0 {
		t.Fatalf("brain invoked %d times for /recall, want 0 (zero model involvement)", n)
	}

	turns := activeTurns(t, store, key)
	if len(turns) != 1 {
		t.Fatalf("active session holds %d turns, want exactly 1 (ONE quoted block)", len(turns))
	}
	block := turns[0]
	if block.Role != conversation.RoleUser {
		t.Fatalf("block role = %q, want %q", block.Role, conversation.RoleUser)
	}
	if want := fmt.Sprintf(router.RecallBlockHeaderFormat, 1); !strings.HasPrefix(block.Content, want) {
		t.Fatalf("block header = %q..., want prefix %q", firstLine(block.Content), want)
	}
	for _, line := range []string{"User: d9", "Assistant: d10", "User: d11", "Assistant: d12"} {
		if !strings.Contains(block.Content, line) {
			t.Fatalf("block misses line %q:\n%s", line, block.Content)
		}
	}
	if strings.Contains(block.Content, "d8") {
		t.Fatalf("block leaked a turn beyond the last k=4:\n%s", block.Content)
	}

	recent, err := store.LoadRecent(context.Background(), key, 10)
	if err != nil {
		t.Fatalf("LoadRecent: %v", err)
	}
	if len(recent) != 1 || !strings.Contains(recent[0].Content, "d12") {
		t.Fatalf("LoadRecent after recall = %v, want the quoted block visible to the next Handle", recent)
	}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// --- AS-A2: nothing to recall + the ack-only skip ---------------------------

func TestRecall_NothingToRecallOnFreshKey(t *testing.T) {
	store := conversation.NewMemStore()
	r, ch, _ := sessionRouter(t, store,
		router.WithSessionPolicy(recallPolicy(10)))

	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "/recall")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	waitUntil(t, "the fixed nothing-to-recall reply", func() bool { return len(ch.Sent()) == 1 })
	if got := ch.Sent()[0].Parts[0].Content; got != router.RecallNothingReply {
		t.Fatalf("reply = %q, want the fixed %q", got, router.RecallNothingReply)
	}
	if n := sessionCount(t, store); n != 0 {
		t.Fatalf("sessions after a nothing-reply = %d, want 0 (zero writes)", n)
	}
}

func TestRecall_SkipsAckOnlyArchivesAndFindsDialogue(t *testing.T) {
	store := conversation.NewMemStore()
	seedDialogue(t, store, key, 2) // session 1: d1 d2
	archive(t, store, key)         // session 2 active
	appendSystem(t, store, key, "[reset ack]")
	archive(t, store, key) // session 2 archived ACK-ONLY; session 3 active empty

	r, ch, _ := sessionRouter(t, store,
		router.WithSessionPolicy(recallPolicy(10)))
	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "/recall 2")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	waitUntil(t, "the recall ack to leave", func() bool { return len(ch.Sent()) == 1 })

	ack := ch.Sent()[0].Parts[0].Content
	if want := fmt.Sprintf(router.RecallAckFormat, 2, 1); ack != want {
		t.Fatalf("ack = %q, want %q (the ack-only session 2 is SKIPPED; dialogue is in 1)", ack, want)
	}
	block := activeTurns(t, store, key)
	if len(block) != 1 || !strings.Contains(block[0].Content, "User: d1") || !strings.Contains(block[0].Content, "Assistant: d2") {
		t.Fatalf("active session = %v, want one block quoting d1/d2", block)
	}
}

func TestRecall_DialogueBeyondScanBoundIsNothing(t *testing.T) {
	store := conversation.NewMemStore()
	seedDialogue(t, store, key, 2) // session 1: the only dialogue
	archive(t, store, key)
	// RecallScanSessions ack-only archives push the dialogue session out of
	// the bounded newest-first scan.
	for i := 0; i < router.RecallScanSessions; i++ {
		appendSystem(t, store, key, "[reset ack]")
		archive(t, store, key)
	}

	r, ch, _ := sessionRouter(t, store,
		router.WithSessionPolicy(recallPolicy(10)))
	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "/recall")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	waitUntil(t, "the fixed nothing-to-recall reply", func() bool { return len(ch.Sent()) == 1 })
	if got := ch.Sent()[0].Parts[0].Content; got != router.RecallNothingReply {
		t.Fatalf("reply = %q, want %q (dialogue beyond S=%d is honestly nothing)",
			got, router.RecallNothingReply, router.RecallScanSessions)
	}
	if turns := activeTurns(t, store, key); len(turns) != 0 {
		t.Fatalf("active session = %v, want empty (zero writes)", turns)
	}
}

// --- AS-A3: empty-session precondition --------------------------------------

func TestRecall_RefusedOnNonEmptyActive(t *testing.T) {
	store := conversation.NewMemStore()
	seedDialogue(t, store, key, 4)
	archive(t, store, key)
	seedDialogue(t, store, key, 1) // the active session now holds d1

	r, ch, _ := sessionRouter(t, store,
		router.WithSessionPolicy(recallPolicy(10)))
	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "/recall 2")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	waitUntil(t, "the fixed refusal", func() bool { return len(ch.Sent()) == 1 })

	got := ch.Sent()[0].Parts[0].Content
	if got != router.RecallRefusalReply {
		t.Fatalf("reply = %q, want the fixed %q", got, router.RecallRefusalReply)
	}
	if !strings.Contains(router.RecallRefusalReply, "/new") {
		t.Fatalf("the refusal copy must name /new: %q", router.RecallRefusalReply)
	}
	if turns := activeTurns(t, store, key); len(turns) != 1 {
		t.Fatalf("active session = %d turns, want 1 (zero writes on refusal)", len(turns))
	}
}

func TestRecall_SecondRecallRefusedByConstruction(t *testing.T) {
	store := conversation.NewMemStore()
	seedDialogue(t, store, key, 4)
	archive(t, store, key)
	r, ch, _ := sessionRouter(t, store,
		router.WithSessionPolicy(recallPolicy(10)))

	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "/recall 2")); err != nil {
		t.Fatalf("first DispatchInbound: %v", err)
	}
	waitUntil(t, "the first recall ack", func() bool { return len(ch.Sent()) == 1 })

	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "/recall 2")); err != nil {
		t.Fatalf("second DispatchInbound: %v", err)
	}
	waitUntil(t, "the second reply", func() bool { return len(ch.Sent()) == 2 })

	if got := ch.Sent()[1].Parts[0].Content; got != router.RecallRefusalReply {
		t.Fatalf("second /recall reply = %q, want the refusal %q (duplication impossible)", got, router.RecallRefusalReply)
	}
	if turns := activeTurns(t, store, key); len(turns) != 1 {
		t.Fatalf("active session = %d turns after the second /recall, want still 1 block", len(turns))
	}
}

// --- AS-A4: grammar and clamping --------------------------------------------

func TestRecall_ClampAndBareUseConfiguredMax(t *testing.T) {
	for _, text := range []string{"/recall 99", "/recall"} {
		t.Run(text, func(t *testing.T) {
			store := conversation.NewMemStore()
			seedDialogue(t, store, key, 12)
			archive(t, store, key)
			r, ch, _ := sessionRouter(t, store,
				router.WithSessionPolicy(recallPolicy(10)))

			if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", text)); err != nil {
				t.Fatalf("DispatchInbound: %v", err)
			}
			waitUntil(t, "the recall ack", func() bool { return len(ch.Sent()) == 1 })
			if got, want := ch.Sent()[0].Parts[0].Content, fmt.Sprintf(router.RecallAckFormat, 10, 1); got != want {
				t.Fatalf("%s ack = %q, want %q (exactly recall_max)", text, got, want)
			}
		})
	}
}

func TestRecall_BadGrammarIsFixedUsageAndZeroWrites(t *testing.T) {
	for _, text := range []string{"/recall 0", "/recall abc", "/recall 3 x"} {
		t.Run(text, func(t *testing.T) {
			store := conversation.NewMemStore()
			seedDialogue(t, store, key, 4)
			archive(t, store, key)
			r, ch, br := sessionRouter(t, store,
				router.WithSessionPolicy(recallPolicy(10)))

			if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", text)); err != nil {
				t.Fatalf("DispatchInbound: %v", err)
			}
			waitUntil(t, "the fixed usage reply", func() bool { return len(ch.Sent()) == 1 })
			if got := ch.Sent()[0].Parts[0].Content; got != router.RecallUsageReply {
				t.Fatalf("%s reply = %q, want the fixed %q", text, got, router.RecallUsageReply)
			}
			if turns := activeTurns(t, store, key); len(turns) != 0 {
				t.Fatalf("%s wrote %d turns, want 0", text, len(turns))
			}
			if n := len(br.Handled()); n != 0 {
				t.Fatalf("%s reached the brain (%d), want 0", text, n)
			}
		})
	}
}

// --- AS-A5: the rune cap ----------------------------------------------------

func TestRecall_BlockTruncatesOldestLinesFirst(t *testing.T) {
	store := conversation.NewMemStore()
	// 30 long lines (~193 runes each) overflow RecallBlockRunes by design.
	long := strings.Repeat("a", 190)
	for i := 1; i <= 30; i++ {
		if _, err := store.Append(context.Background(), key,
			conversation.Turn{Role: conversation.RoleUser, Content: fmt.Sprintf("%s-%02d", long, i), Timestamp: time.Unix(int64(100+i), 0)}); err != nil {
			t.Fatalf("seed long turn %d: %v", i, err)
		}
	}
	archive(t, store, key)
	r, ch, _ := sessionRouter(t, store,
		router.WithSessionPolicy(recallPolicy(32)))

	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "/recall 30")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	waitUntil(t, "the recall ack", func() bool { return len(ch.Sent()) == 1 })

	turns := activeTurns(t, store, key)
	if len(turns) != 1 {
		t.Fatalf("active session holds %d turns, want 1", len(turns))
	}
	block := turns[0].Content
	if got := utf8.RuneCountInString(block); got > router.RecallBlockRunes {
		t.Fatalf("block = %d runes, must not exceed RecallBlockRunes = %d", got, router.RecallBlockRunes)
	}
	if want := fmt.Sprintf(router.RecallBlockHeaderTruncatedFormat, 1); !strings.HasPrefix(block, want) {
		t.Fatalf("truncated block header = %q, want %q (the header names the truncation)", firstLine(block), want)
	}
	if !strings.Contains(block, "-30") {
		t.Fatalf("newest line missing from the truncated block")
	}
	if strings.Contains(block, "-01") {
		t.Fatalf("oldest line survived the truncation, want oldest dropped first")
	}
}

// --- AS-A6: channel-agnostic ------------------------------------------------

func TestRecall_WorksOnNetworkChannel(t *testing.T) {
	store := conversation.NewMemStore()
	tgKey := conversation.Key("telegram::c")
	seedDialogue(t, store, tgKey, 2)
	archive(t, store, tgKey)
	r, ch, _ := recallRouterOn(t, "telegram", store,
		router.WithSessionPolicy(recallPolicy(10)))

	if err := r.DispatchInbound(context.Background(), mkInbound("telegram", "c", "/recall 2")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	waitUntil(t, "the recall ack on the network channel", func() bool { return len(ch.Sent()) == 1 })
	if got, want := ch.Sent()[0].Parts[0].Content, fmt.Sprintf(router.RecallAckFormat, 2, 1); got != want {
		t.Fatalf("telegram-routed /recall ack = %q, want %q (channel-agnostic reach)", got, want)
	}
}

// --- AS-A7: honest failure --------------------------------------------------

func TestRecall_StoreFailureIsHonestErrorNeverSilence(t *testing.T) {
	mem := conversation.NewMemStore()
	seedDialogue(t, mem, key, 4)
	archive(t, mem, key)
	store := &failingRecallStore{SessionStore: mem}

	var mu sync.Mutex
	var captured []router.RouterError
	r, ch, br := sessionRouter(t, store,
		router.WithSessionPolicy(recallPolicy(10)),
		router.WithErrorHandler(func(e router.RouterError) {
			mu.Lock()
			captured = append(captured, e)
			mu.Unlock()
		}))

	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "/recall 2")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	waitUntil(t, "the fixed error reply", func() bool { return len(ch.Sent()) == 1 })

	if got := ch.Sent()[0].Parts[0].Content; got != router.RecallErrorReply {
		t.Fatalf("reply = %q, want the fixed honest %q", got, router.RecallErrorReply)
	}
	if n := len(br.Handled()); n != 0 {
		t.Fatalf("store failure leaked the command to the brain (%d), want 0", n)
	}
	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, e := range captured {
		if e.Kind == router.ErrKindSession && e.Err != nil && errors.Is(e.Err, errStoreDown) {
			found = true
		}
	}
	if !found {
		t.Fatalf("no structured session error reported for the failed recall; captured = %+v", captured)
	}
	if turns := activeTurns(t, mem, key); len(turns) != 0 {
		t.Fatalf("active session = %d turns after a failed recall, want 0", len(turns))
	}
}

// --- The primary case: /recall right after a lazy expiry cut ----------------

func TestRecall_AfterIdleExpiryRecoversJustArchivedSession(t *testing.T) {
	store := conversation.NewMemStore()
	seedDialogue(t, store, key, 2) // dialogue lives in the (only) active session
	clock := newFakeClock(time.Unix(102, 0).Add(2 * time.Hour))
	r, ch, _ := sessionRouter(t, store,
		router.WithSessionPolicy(router.SessionPolicy{Triggers: defaultTriggers, IdleMin: 30, RecallMax: 10}),
		router.WithClock(clock.Now))

	// The /recall inbound itself fires the lazy idle cut FIRST (expiry runs
	// before the command by construction), then recovers from the session
	// the cut just archived.
	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "/recall")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	waitUntil(t, "the recall ack after the cut", func() bool { return len(ch.Sent()) == 1 })

	if got, want := ch.Sent()[0].Parts[0].Content, fmt.Sprintf(router.RecallAckFormat, 2, 1); got != want {
		t.Fatalf("post-expiry ack = %q, want %q (recovers the JUST-archived session)", got, want)
	}
	if n := sessionCount(t, store); n != 2 {
		t.Fatalf("sessions = %d, want 2 (the cut opened session 2)", n)
	}
	turns := activeTurns(t, store, key)
	if len(turns) != 1 || !strings.Contains(turns[0].Content, "Assistant: d2") {
		t.Fatalf("active session = %v, want one block quoting the archived dialogue", turns)
	}
}
